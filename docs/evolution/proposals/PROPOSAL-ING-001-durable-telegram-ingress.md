# PROPOSAL-ING-001 — Ingress Telegram durável e orientado a Evidence

Authority: Non-authoritative
Status: Ready

## Problema

O ingress atual mistura captura de mensagem, cursor histórico e persistência raw de uma
forma que permite perda silenciosa, avanço de progresso sem durabilidade equivalente e
colisão entre revisões da mesma mensagem.

Além disso, `LastMessageID` não representa o mecanismo de sincronização definido pelo
protocolo Telegram.

Esta Proposal recomenda separar:

- observação recebida da fonte;
- identidade lógica de mensagem;
- estado nativo de sincronização;
- progresso de backfill histórico;
- sessão/peer state operacional.

---

## Evidence e Findings relevantes

A arqueologia e o EXP-LIMIAR-001 sustentam os seguintes pontos:

1. live updates podem ser descartados sob backpressure no desenho atual;
2. checkpoints/cursors podem ultrapassar Evidence que ainda não foi persistida;
3. `(channel_id, message_id)` é insuficiente como chave física de Evidence porque edits
   mantêm a identidade lógica da mensagem mas constituem novas observações;
4. deletes também são eventos relevantes para reconstruir projeções;
5. Telegram governa continuidade de updates por `pts/qts/seq` e channel state, não por
   maior message ID;
6. uma DurabilityBarrier linearizável é viável para impedir state advance sobre Evidence
   não durável no boundary do Limiar.

Referência experimental:
`docs/evolution/experiments/EXP-LIMIAR-001-durable-telegram-update-recovery.md`.

---

## Proposta

### 1. `EvidenceRecord`

Persistir observações de fonte de forma append-only/versionada.

Campos lógicos candidatos:

```text
id
subscription_id
acquisition
source_event_type
event_kind
source_message_id?
source_occurred_at?
received_at
payload_format
payload_schema
payload
payload_hash?
source_metadata?
```

O schema físico exato permanece fora desta Proposal.

Não criar unicidade global por `(channel_id, message_id)` na tabela de Evidence.
Replay e múltiplas revisões precisam poder coexistir.

### 2. `SourceMessageKey`

Definir identidade lógica da mensagem separadamente da Evidence:

```text
source + source scope + source message id
```

Essa chave identifica a mensagem lógica; não identifica evento, revisão ou ocorrência de
Evidence.

### 3. `SourceSyncState`

Persistir o estado operacional nativo necessário para continuidade do protocolo
Telegram, incluindo conforme aplicável `pts/qts/seq/date` e channel `pts`.

Esse state é operacional e não é identidade de domínio.

### 4. `BackfillState`

Manter progresso de histórico separado do sync live.

`LastMessageID` pode ser útil como posição de varredura de histórico, mas não como prova
de que todos os updates live até aquele ID foram duravelmente observados.

### 5. Durability boundary

Integrar o recovery manager do pacote `telegram/updates` do gotd a um
`DurableEvidenceHandler`.

A persistência de `SourceSyncState` deve ser guardada por uma `DurabilityBarrier` de
forma que falha na durabilidade de uma Evidence admitida impeça novo state write até
reconciliação/cancelamento do fluxo.

A implementação concreta precisa de contract tests contra a versão real do gotd antes de
virar produção.

### 6. Replays

O ingress assume:

```text
at-least-once observation
+
replay permitido
+
downstream idempotente
```

Não prometer exactly-once.

### 7. Deletes e edits

Edits e deletes devem produzir Evidence quando observados.

A reconstrução do estado corrente de uma mensagem pertence a uma Source Projection
derivada, não ao overwrite da Evidence original.

### 8. History snapshots

Dados obtidos por history/backfill devem ser marcados como snapshot/aquisição histórica.
Não fabricar um evento de criação ou edição que o Limiar não observou diretamente.

---

## Alternativas consideradas

### A. Manter `LastMessageID` como cursor global

Rejeitada como proposta de sync live porque não representa `pts/qts/seq`, não modela gaps
do protocolo e pode ultrapassar falhas anteriores.

### B. Reimplementar a máquina de sync do Telegram no Limiar

Rejeitada por complexidade e duplicação. O gotd já possui recovery/ordering específico do
protocolo; o Limiar deve integrar e testar esse comportamento, não recriá-lo sem
necessidade.

### C. Persistir somente estado final da mensagem

Rejeitada porque destrói a trilha de Evidence, dificulta reconstrução e conflita com
C-02/C-11.

### D. Event log append-only + state nativo separado

Recomendada. Separa verdade observada de posição operacional e permite replay.

---

## Trade-offs

### Benefícios

- elimina `LastMessageID` como falsa autoridade de sync live;
- preserva edits/deletes/replays como Evidence;
- permite reconstrução de projeções;
- aproxima a corretude do protocolo Telegram real;
- torna a perda silenciosa uma violação explícita de contrato.

### Custos

- schema de ingress fica mais rico;
- downstream precisa tolerar replay;
- sync state e backfill state deixam de ser um único cursor simples;
- barrier adiciona lifecycle/failure handling que precisa de testes concorrentes fortes.

---

## Riscos e pendências

Antes de implementação de produção:

- concluir contract tests reais do gotd descritos no EXP-LIMIAR-001;
- decidir engine/PRAGMAs de storage em ADR próprio;
- decidir payload físico/versionamento de Evidence;
- decidir session storage em conjunto com o boundary de segredos;
- definir idempotência/reconciliação quando commit de Evidence ocorre mas state write não;
- definir como updates compostos que afetam múltiplas mensagens viram uma ou mais
  projeções sem falsificar a Evidence recebida.

---

## Recomendação

Transformar os princípios desta Proposal em um ADR `Proposed` sobre Source Evidence e
Telegram Synchronization.

A implementação concreta do gotd recovery manager deve permanecer condicionada aos
contract tests pendentes; aceitar os princípios não deve ser interpretado como aceitação
prematura de toda mecânica do protótipo.
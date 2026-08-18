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

A arqueologia, a documentação primária e o EXP-LIMIAR-001 sustentam:

1. live updates podem ser descartados sob backpressure no desenho atual;
2. checkpoints/cursors podem ultrapassar Evidence que ainda não foi persistida;
3. `(channel_id, message_id)` é insuficiente como chave física de Evidence porque edits
   mantêm a identidade lógica da mensagem mas constituem novas observações;
4. uma única observação da fonte pode afetar múltiplas mensagens, como updates de delete;
5. Telegram governa continuidade por `pts/qts/seq/date` e channel state, não por maior
   message ID;
6. o protocolo possui limites de recuperação, e o gotd também documenta limitações para
   updates stateless e `ChannelDifferenceTooLong`;
7. uma DurabilityBarrier linearizável é viável como hipótese de implementação para
   impedir state advance sobre Evidence não durável no boundary do Limiar.

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

Não criar unicidade global por `(channel_id, message_id)` na Evidence. Replay, revisões e
observações compostas precisam poder coexistir.

Uma Evidence representa a observação recebida da fonte; ela não precisa corresponder
1:1 a uma `SourceMessageKey`. Uma observação pode alimentar nenhuma, uma ou várias
projeções de mensagem.

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

Definir explicitamente o boundary de admissão entre observação da fonte, persistência de
Evidence e avanço certificado do `SourceSyncState`.

A propriedade necessária é impedir:

```text
Evidence exigida pelo contrato ainda não durável
+
SourceSyncState persistido como avançado
```

`DurableEvidenceHandler` + `DurabilityBarrier/GuardedStateStorage` permanece uma
**hipótese de implementação**, não uma decisão desta Proposal. A mecânica concreta precisa
de contract tests contra a versão real do gotd antes de produção.

### 6. Replay e garantia local

Dentro do boundary controlado pelo Limiar, replay/duplicidade explícita devem ser
permitidos e reconciliáveis. Não prometer exactly-once.

Isso não é uma promessa de que o Telegram consegue recuperar indefinidamente qualquer
evento histórico. Retenção do protocolo, `differenceTooLong`,
`ChannelDifferenceTooLong` e updates stateless precisam de políticas explícitas.

### 7. Deletes, edits e updates compostos

Edits e deletes devem produzir Evidence quando observados.

Updates compostos são preservados como observação de fonte e podem afetar múltiplas
`SourceMessageKey` na projeção derivada. A Evidence original não deve ser falsificada em
vários eventos inventados apenas para facilitar persistência.

### 8. History snapshots

Dados obtidos por history/backfill devem ser marcados como snapshot/aquisição histórica.
Não fabricar um evento de criação ou edição que o Limiar não observou diretamente.

---

## Alternativas consideradas

### A. Manter `LastMessageID` como cursor global

Rejeitada como proposta de sync live porque não representa `pts/qts/seq`, não modela gaps
do protocolo e pode ultrapassar falhas anteriores.

### B. Reimplementar a máquina de sync do Telegram no Limiar

Rejeitada por complexidade e duplicação. O gotd possui recovery/ordering específico do
protocolo, mas sua integração precisa respeitar limitações documentadas e ser validada
por contract tests.

### C. Persistir somente estado final da mensagem

Rejeitada porque destrói a trilha de Evidence, dificulta reconstrução e conflita com
C-02/C-11.

### D. Event log append-only + state nativo separado

Recomendada. Separa verdade observada de posição operacional e permite replay.

---

## Trade-offs

### Benefícios

- elimina `LastMessageID` como falsa autoridade de sync live;
- preserva edits/deletes/replays e updates compostos como Evidence;
- permite reconstrução de projeções;
- aproxima a corretude do protocolo Telegram real;
- torna perda silenciosa uma violação explícita de contrato.

### Custos

- schema de ingress fica mais rico;
- downstream precisa tolerar replay;
- sync state e backfill state deixam de ser um único cursor simples;
- recovery possui casos-limite que exigem política explícita e testes concorrentes fortes.

---

## Riscos e pendências

Antes de implementação de produção:

- concluir contract tests reais do gotd descritos no EXP-LIMIAR-001;
- definir política para updates stateless e `differenceTooLong`/`ChannelDifferenceTooLong`;
- decidir engine/PRAGMAs de storage em ADR próprio;
- decidir payload físico/versionamento de Evidence;
- decidir session storage em conjunto com o boundary de segredos;
- definir idempotência/reconciliação quando commit de Evidence ocorre mas state write não;
- definir redução de updates compostos sem falsificar a Evidence recebida.

---

## Recomendação

Transformar os princípios desta Proposal em um ADR `Proposed` sobre Source Evidence e
Telegram Synchronization.

A implementação concreta de recovery permanece Candidate até os contract tests; aceitar
os princípios não deve ser interpretado como aceitação prematura da mecânica do
protótipo.
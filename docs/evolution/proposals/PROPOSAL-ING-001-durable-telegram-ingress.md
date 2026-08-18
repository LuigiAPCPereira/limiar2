# PROPOSAL-ING-001 — Ingress Telegram durável e orientado a Evidence

Authority: Non-authoritative
Status: Ready

## Outcome

Os princípios desta Proposal foram aceitos pelo ADR 016.

A mecânica concreta de integração com gotd continua Candidate e foi revisada pelo
F-ING-008; nada nesta Proposal promove o Candidate v3 para produção.

## Problema

O ingress legado mistura captura de mensagem, cursor histórico e persistência raw de uma
forma que permite perda silenciosa, avanço de progresso sem durabilidade equivalente e
colisão entre revisões da mesma mensagem.

`LastMessageID` também não representa o mecanismo de sincronização definido pelo
protocolo Telegram.

Precisamos separar:

- observação recebida da fonte;
- identidade lógica de mensagem;
- estado nativo de sincronização;
- progresso de backfill histórico;
- sessão/peer state operacional.

## Evidence e Findings relevantes

A arqueologia, EXP-LIMIAR-001 e F-ING-008 sustentam:

1. live updates podem ser descartados sob backpressure no desenho atual;
2. checkpoint/cursor pode ultrapassar Evidence ainda não durável;
3. `(channel_id, message_id)` é insuficiente como chave física de Evidence porque edits
   mantêm a mensagem lógica, mas são novas observações;
4. uma Evidence pode afetar múltiplas mensagens, como em deletes compostos;
5. Telegram governa continuidade por `pts/qts/seq/date` e channel state;
6. recovery possui limites como `DifferenceTooLong`/`ChannelDifferenceTooLong`;
7. updates reconstruídos por difference podem chegar stateless ao handler;
8. barrier linearizável é viável para impedir state persistido avançado após falha de
   Evidence;
9. o gotd v0.161.0 absorve erros em caminhos relevantes e callbacks de TooLong ocorrem
   depois do avanço de state, exigindo fail-stop externo + interceptação pré-manager.

Referências:

- `docs/evolution/experiments/EXP-LIMIAR-001-durable-telegram-update-recovery.md`;
- `docs/evolution/findings/F-ING-008-gotd-recovery-fail-stop-boundary.md`;
- ADR 016.

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

O schema físico permanece fora desta Proposal.

Não criar unicidade global por `(channel_id, message_id)`. Replay, revisões e observações
compostas precisam coexistir.

Uma Evidence não precisa corresponder 1:1 a `SourceMessageKey`: pode alimentar nenhuma,
uma ou várias projeções.

### 2. `SourceMessageKey`

Identidade lógica da mensagem:

```text
source + source scope + source message id
```

Não identifica evento, revisão ou ocorrência de Evidence.

### 3. `SourceSyncState`

Persistir state operacional nativo do Telegram, incluindo conforme aplicável
`pts/qts/seq/date` e channel `pts`.

Esse state não é identidade de domínio e não deve ser reconstruído a partir de PTS/QTS
stateless presentes em envelopes recuperados.

### 4. `BackfillState`

Histórico/backfill permanece separado do live sync.

`LastMessageID` pode existir como posição de varredura histórica, nunca como prova de
continuidade live.

### 5. Durability boundary

A propriedade obrigatória é impedir:

```text
Evidence exigida ainda não durável
+
SourceSyncState persistido como avançado
```

O Candidate v3 experimental é:

```text
Telegram RPC
    ↓
GuardedRecoveryAPI
    ↓
updates.Manager
    ↓
DurableEvidenceHandler
    ↓
Evidence Store

updates.Manager
    ↓
GuardedStateStorage

DurabilityBarrier + Supervisor
```

Esse desenho permanece **Candidate** até os contract tests reais.

### 6. `GuardedRecoveryAPI`

Bootstrap e descontinuidades precisam ser representados antes de o manager adotar o
state correspondente.

O adapter candidato intercepta:

- `UpdatesGetState` -> Evidence de adoção de baseline remoto;
- `UpdatesDifferenceTooLong` -> Evidence de descontinuidade common;
- `UpdatesChannelDifferenceTooLong` -> Evidence de descontinuidade de canal.

Se a Evidence falhar, a resposta não deve chegar ao manager e o lifecycle entra em
fail-stop.

### 7. Replay e garantia local

Dentro do boundary controlado pelo Limiar, replay/duplicidade explícita são aceitáveis e
devem ser reconciliáveis. Não prometer exactly-once.

Se Evidence foi commitada mas state não foi, repetir a observação é preferível a perder
a continuidade.

### 8. Deletes, edits e updates compostos

Edits e deletes observados produzem Evidence.

Updates compostos permanecem uma observação de fonte e podem reduzir para múltiplas
`SourceMessageKey`. Não fabricar eventos de fonte inexistentes apenas para simplificar o
schema.

### 9. History snapshots

History/backfill deve ser marcado como snapshot/aquisição histórica. Não fabricar evento
de criação/edição que não tenha sido observado diretamente.

## Alternativas consideradas

### A. `LastMessageID` como cursor live global

Rejeitada: não representa `pts/qts/seq` e pode atravessar gaps/falhas.

### B. Reimplementar sync Telegram no Limiar

Rejeitada: o gotd já possui ordering/gap recovery especializado; o Limiar deve envolver
essa máquina com boundaries próprios, não duplicá-la.

### C. `updates.Manager` + handler error como fail-stop

Rejeitada pelo F-ING-008: handler/storage errors podem ser absorvidos.

### D. Somente callbacks `OnTooLong`

Rejeitada: callbacks ocorrem depois do state advance nos caminhos relevantes.

### E. Event log append-only + state nativo separado

Recomendada nos princípios e aceita pelo ADR 016.

## Trade-offs

### Benefícios

- elimina `LastMessageID` como falsa autoridade live;
- preserva edits/deletes/replays e updates compostos;
- torna bootstrap/descontinuidades observáveis;
- permite reconstrução de projeções;
- aproxima a corretude do protocolo real;
- mantém recovery especializado no gotd.

### Custos

- ingress e storage ficam mais explícitos;
- downstream precisa tolerar replay;
- supervisor/fail-stop precisa de lifecycle testado;
- casos irrecuperáveis deixam de ser silenciosos e passam a exigir policy operacional.

## Riscos e pendências

Antes de implementação de produção:

- executar a suíte real da PR #151 com Go atual + `-race`;
- validar runtime de `ChannelDifferenceTooLong` com PTS/dialog válido;
- definir schema físico/versionamento de Evidence;
- decidir engine/PRAGMAs em ADR próprio;
- decidir session storage e boundary de segredos;
- definir idempotência/reconciliação para Evidence commitada + state antigo;
- definir lifecycle/retry do supervisor;
- reduzir updates compostos sem falsificar Evidence.

## Recomendação atual

Manter os princípios do ADR 016 como autoridade e continuar tratando
`GuardedRecoveryAPI + updates.Manager + DurableEvidenceHandler + GuardedStateStorage +
DurabilityBarrier + Supervisor` como **Candidate experimental**.

Somente após contract tests reais passarem deve existir uma Proposal/ADR que fixe essa
mecânica como implementação de produção.

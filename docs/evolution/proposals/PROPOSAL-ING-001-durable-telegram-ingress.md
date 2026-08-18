# PROPOSAL-ING-001 — Ingress Telegram durável e orientado a Evidence

Authority: Non-authoritative
Status: Ready

## Outcome

Os princípios desta Proposal foram aceitos pelo ADR 016.

A mecânica concreta de integração com gotd foi revisada pelo F-ING-008 e agora possui
contract tests reais aprovados. Ela passa de Candidate não validado para **Candidate
suportado**, sem ganhar autoridade de produção.

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
3. `(channel_id, message_id)` é insuficiente como chave física de Evidence;
4. uma Evidence pode afetar múltiplas mensagens;
5. Telegram governa continuidade por `pts/qts/seq/date` e channel state;
6. recovery possui limites como `DifferenceTooLong`/`ChannelDifferenceTooLong`;
7. updates reconstruídos por difference podem chegar stateless ao handler;
8. handler/storage errors do gotd não constituem fail-stop confiável;
9. callbacks de TooLong ocorrem depois do avanço de state;
10. o Candidate v3 passou 12 contract tests no gotd v0.161.0 com Go 1.26.6 e race detector.

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

Uma Evidence não precisa corresponder 1:1 a `SourceMessageKey`.

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

Candidate v3 suportado:

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

### 6. `GuardedRecoveryAPI`

Bootstrap e descontinuidades precisam ser representados antes de o manager adotar o
state correspondente.

Interceptações candidatas:

- `UpdatesGetState` -> Evidence de adoção de baseline remoto;
- `UpdatesDifferenceTooLong` -> Evidence de descontinuidade common;
- `UpdatesChannelDifferenceTooLong` -> Evidence de descontinuidade de canal.

Se a Evidence falhar, a resposta não chega ao manager e o lifecycle entra em fail-stop.

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

- `LastMessageID` como cursor live global — rejeitada;
- reimplementar sync Telegram no Limiar — rejeitada;
- `updates.Manager` + handler error como fail-stop — rejeitada pelo F-ING-008;
- somente callbacks `OnTooLong` — rejeitada;
- event log append-only + state nativo separado — aceita nos princípios pelo ADR 016;
- Candidate v3 acima — **suportado pelos contract tests**, ainda não autoritativo.

## Trade-offs

### Benefícios

- elimina `LastMessageID` como falsa autoridade live;
- preserva edits/deletes/replays e updates compostos;
- torna bootstrap/descontinuidades observáveis;
- permite reconstrução de projeções;
- mantém recovery especializado no gotd;
- falhas de Evidence resultam em fail-stop + replay, não avanço silencioso.

### Custos

- ingress e storage ficam mais explícitos;
- downstream precisa tolerar replay;
- supervisor/fail-stop exige lifecycle operacional;
- casos irrecuperáveis passam a exigir policy explícita.

## Riscos e pendências

Antes de implementação de produção:

- definir schema físico/versionamento de Evidence;
- decidir engine/PRAGMAs em ADR próprio;
- decidir session storage e boundary de segredos;
- definir idempotência/reconciliação para Evidence commitada + state antigo;
- definir lifecycle/retry do supervisor;
- preservar contract tests no adapter real e validar integração completa.

## Recomendação atual

Manter os princípios do ADR 016 como autoridade e considerar
`GuardedRecoveryAPI + updates.Manager + DurableEvidenceHandler + GuardedStateStorage +
DurabilityBarrier + Supervisor` um **Candidate suportado para implementação**.

O próximo passo não é implementá-lo silenciosamente. É decidir, sob a governança vigente,
se a mecânica é apenas detalhe de implementação compatível com o ADR 016 ou se merece uma
Decision complementar antes da mudança de produção.

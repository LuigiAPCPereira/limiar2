# Registro de aceitação — L3 SourceSyncState

**Data local do mantenedor:** 2026-10-01  
**Escopo:** Limiar 3.0 — L3-004 / S7 / schema físico e boundary de `SourceSyncState`.  
**Decision a formalizar:** `docs/limiar3/adr/007-source-sync-state-physical-schema.md`.

## Autorização explícita do mantenedor

Após a explicação da ADR histórica 021 e da proposta de **revisar a Decision existente,
melhorar o que fosse necessário e resolvê-la formalmente antes de implementar o storage
de sync state**, o mantenedor respondeu: **“sim vamos fazer isso”**.

Esta autorização permite promover a Decision correspondente no namespace Limiar 3 para
`Accepted`, desde que a revisão preserve as Decisions já aceitas e não transforme a
proposta histórica em authority retroativa.

## Refinamento necessário identificado na revisão

A ADR histórica 021 propunha `user_id` como chave primária do common state e
`(user_id, channel_id)` para channel state.

Depois dessa Proposal, a **L3 ADR 006 Accepted** estabeleceu que `SourceSyncState`
particiona continuidade live pela Acquisition Subscription aplicável. Portanto, copiar o
schema histórico literalmente conflitaria com uma Decision L3 superior.

A Decision L3 aceita deve reconciliar os dois contratos:

- a API upstream `github.com/gotd/td/telegram/updates.StateStorage` v0.162.0 continua
  recebendo `userID` em seus métodos;
- o adapter Limiar é instanciado/scoped para uma `subscription_id` explícita;
- a persistência física inclui `subscription_id` na chave:
  `(subscription_id, user_id)` para common state e
  `(subscription_id, user_id, channel_id)` para channel state;
- o `subscription_id` não é inferido do `userID`, channel ID, sessão ou target MCP.

## Contratos autorizados

A Decision L3 pode fechar de forma durável:

1. schema físico mínimo de common state e channel state;
2. separação entre ausência e erro de leitura;
3. `SetState` como único caminho capaz de criar/substituir common state completo;
4. setters parciais somente sobre state existente, com zero rows como erro;
5. channel state ausente distinto de `pts=0`;
6. enumeração de channels scoped por subscription + user;
7. capability/repository estreito sem expor `*sql.DB`;
8. migration SQL versionada como authority de schema;
9. integração futura por `GuardedStateStorage` + `DurabilityBarrier`, preservando
   Evidence-before-state;
10. restart/replay sem promessa de exactly-once;
11. independência de `BackfillProgress`;
12. lifecycle/limpeza de state sem transformar state operacional em Evidence.

## Limites

Esta aceitação não autoriza no mesmo ato:

- implementação do schema;
- promoção da ADR histórica 021;
- `BackfillProgress` físico;
- wiring final de `updates.Manager`;
- remoção de `NoUpdates: true`;
- Telegram real, OTP/2FA, merge ou deploy.

A implementação deve ocorrer em slice posterior, sobre a Decision L3 materializada e
validada.

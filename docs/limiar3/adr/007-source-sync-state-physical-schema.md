# L3 ADR 007 — Schema físico e boundary de SourceSyncState

Authority: Decision Record — Limiar 3.0  
Status: Accepted  
Accepted-by: Mantenedor do Limiar  
Accepted-at: 2026-10-01  
Acceptance-reference: `619db50c5a311b26221058d63bad4a2d4e90462f`

## Contexto

O Limiar 3 já aceita que:

- Evidence é append-only e deve preceder progresso certificado;
- `updates.Manager` continua sendo a authority especializada de ordering/recovery do
  Telegram;
- live Source Evidence é admitida antes do manager;
- falha de Evidence ou de state write fecha uma `DurabilityBarrier` e termina o
  lifecycle da instância;
- live sync, backfill e Evidence são authorities distintas;
- `subscription_id` identifica uma Acquisition Subscription estável e explícita.

A ADR histórica 021 propôs um schema físico para o contrato
`github.com/gotd/td/telegram/updates.StateStorage`, mas permaneceu `Proposed`.
Ela usava `user_id` como chave do common state e `(user_id, channel_id)` para channel
state.

Depois dessa Proposal, a **L3 ADR 006 Accepted** estabeleceu que `SourceSyncState`
particiona continuidade live pela Acquisition Subscription aplicável. Copiar o schema
histórico literalmente deixaria de representar essa partition.

Em 2026-10-01 o contrato upstream foi revalidado no pin atual
`github.com/gotd/td v0.162.0`: `updates.StateStorage` continua recebendo `userID`
em todas as operações, incluindo `GetState`, `SetState`, setters parciais,
`Get/SetChannelPts` e `ForEachChannels`. O upstream também documenta explicitamente
que os setters parciais devem retornar erro se o internal state do usuário não existir.

Isso permite reconciliar os dois contratos sem alterar a interface do gotd: o adapter
Limiar é scoped por `subscription_id`, enquanto o `userID` continua vindo da chamada
upstream.

## Decision

### 1. SourceSyncState é particionado por Acquisition Subscription e usuário Telegram

O common state usa identidade física equivalente a:

```sql
CREATE TABLE source_sync_state (
    subscription_id TEXT    NOT NULL CHECK(length(subscription_id) > 0),
    user_id         INTEGER NOT NULL,
    pts             INTEGER NOT NULL,
    qts             INTEGER NOT NULL,
    date            INTEGER NOT NULL,
    seq             INTEGER NOT NULL,
    PRIMARY KEY (subscription_id, user_id)
) STRICT;
```

A chave representa:

```text
Acquisition Subscription
        +
Telegram self user
        =
authority de continuidade live daquela aquisição
```

`subscription_id` não é derivado de `user_id`, session ID, channel ID,
`TelegramAuthorizationIdentity` ou target MCP.

O boundary de configuração valida a identidade conforme L3 ADR 006 antes de construir o
adapter de state. O storage não normaliza silenciosamente a identidade.

### 2. O adapter implementa StateStorage por scoping, não alterando a API upstream

A capability concreta de state é construída com uma `subscription_id` já validada.

Os métodos exigidos pelo gotd continuam com o contrato upstream baseado em `userID`.
Internamente, cada query inclui também a subscription scoped pelo adapter.

Exemplo conceitual:

```text
ScopedStateStorage(subscription_id="acq:ofertas")
    SetPts(ctx, userID=42, pts=100)

        ↓

UPDATE source_sync_state
SET pts = 100
WHERE subscription_id = "acq:ofertas"
  AND user_id = 42
```

O gotd não precisa conhecer `subscription_id` e a aplicação não precisa forká-lo.

### 3. Common state é sempre completo

Uma linha existente possui simultaneamente:

- `pts`;
- `qts`;
- `date`;
- `seq`.

Não existe common state parcialmente inicializado persistido.

`SetState` é o único método do contrato `StateStorage` autorizado a criar uma linha de
common state ausente. Ele pode inserir ou substituir atomicamente a linha completa para
`(subscription_id, user_id)`.

### 4. Setters parciais nunca fabricam state ausente

`SetPts`, `SetQts`, `SetDate`, `SetSeq` e `SetDateSeq` executam apenas
`UPDATE` sobre common state já existente.

A pós-condição é exatamente uma linha afetada.

- zero linhas afetadas → erro de state ausente;
- uma linha → sucesso;
- qualquer cardinalidade impossível/inesperada → falha fechada.

UPSERT em setter parcial é proibido.

Isso preserva o contrato upstream e impede inventar valores para campos que nunca foram
estabelecidos por `SetState`.

### 5. Ausência e falha de leitura são estados distintos

`GetState` preserva três resultados:

```text
state encontrado
state ausente
erro ao ler state
```

Erro de storage não pode ser transformado em `found=false` nem acionar bootstrap como se
nenhum state existisse.

Essa distinção é obrigatória para o fail-stop do ADR 017.

### 6. Channel state também é particionado por subscription

O channel PTS usa schema equivalente a:

```sql
CREATE TABLE source_sync_channel_state (
    subscription_id TEXT    NOT NULL CHECK(length(subscription_id) > 0),
    user_id         INTEGER NOT NULL,
    channel_id      INTEGER NOT NULL,
    pts             INTEGER NOT NULL,
    PRIMARY KEY (subscription_id, user_id, channel_id)
) STRICT;
```

`GetChannelPts` distingue linha ausente de `pts=0`.

`SetChannelPts` pode inserir ou atualizar o registro completo daquela chave composta,
porque channel state é representado integralmente pelo próprio `pts`.

### 7. Channel state não possui foreign key física para common state

Não é criada foreign key de
`(subscription_id, user_id)` em `source_sync_channel_state` para
`source_sync_state`.

A Evidence histórica não demonstra que o lifecycle físico de channel state precise ser
apagado ou invalidado junto com uma linha de common state. Impor essa relação adicionaria
acoplamento que o contrato upstream não exige.

A coerência operacional permanece responsabilidade do `updates.Manager` e do boundary
de recovery, não de cascade SQL.

### 8. Enumeração de channels é sempre scoped

`ForEachChannels(ctx, userID, fn)` enumera somente:

```text
subscription_id do adapter
+
user_id recebido do gotd
```

Não existe enumeração global de todos os usuários ou de todas as subscriptions nessa
capability.

Ordem de enumeração não é authority e não deve ser usada como cursor.

### 9. SourceSyncState é mutable operational state, não Evidence

Ao contrário de Evidence, `SourceSyncState` é uma authority operacional mutável.

Atualizações legítimas substituem o state anterior. Isso não transforma state em histórico
auditável nem em fonte de eventos observados.

Retenção/GC de state de subscriptions aposentadas fica fora desta Decision. Ausência de uso
não autoriza reciclar `subscription_id`, conforme L3 ADR 006.

### 10. Evidence-before-state continua sendo requisito do boundary, não do repository

O repository físico de state não decide se Evidence suficiente já foi persistida.

Na composição produtiva:

```text
Source/Recovery Evidence durável
        ↓
DurabilityBarrier aberta
        ↓
GuardedStateStorage
        ↓
SourceSyncState write
```

Todas as mutações de state que possam certificar progresso passam pelo
`GuardedStateStorage`.

O check da barrier e a escrita física devem ocorrer na mesma região crítica
linearizável, usando o contrato de `DurabilityBarrier.Guard` ou equivalente.

Se a escrita falhar:

- a barrier fecha antes do retorno;
- a primeira causa é preservada;
- nenhuma nova state write é autorizada naquele lifecycle;
- o supervisor encerra a instância;
- restart cria nova barrier e retoma a partir do state durável anterior.

### 11. Reads preservam erro e lifecycle

Reads não podem reinterpretar falha como ausência.

Se a barrier já estiver fechada, a integração pode recusar novas operações imediatamente;
independentemente da forma concreta, o lifecycle atual é terminal e não pode continuar
certificando progresso.

Esta Decision não obriga reads e writes a compartilhar o mesmo lock quando isso não for
necessário para a linearização Evidence-before-state. A região crítica obrigatória é a
que une check da barrier e state write.

### 12. Live sync e backfill permanecem independentes

`SourceSyncState` não contém:

- message ID como cursor de live sync;
- posição de history;
- conclusão de backfill;
- `BackfillProgress`;
- campos de projeção comercial.

`LastMessageID`, paginação histórica e backfill não substituem `pts/qts/seq/date` ou
channel PTS.

### 13. Migrations SQL versionadas são a única authority do schema

As duas tabelas entram no banco L3 definido pelo ADR histórico 019 por migration SQL
versionada.

O adapter não cria, altera ou repara schema ad hoc durante `GetState` ou `SetState`.

Nenhum `*sql.DB` global é exposto aos consumidores; o Store oferece capability estreita
de SourceSyncState.

### 14. Mesmo arquivo SQLite não funde authorities

Evidence e SourceSyncState podem residir no mesmo arquivo SQLite L3.

Isso não os transforma na mesma authority.

O baseline físico continua:

```text
Evidence durável / state antigo     -> replay seguro
Evidence durável / state novo       -> normal
Evidence ausente / state antigo     -> falha segura
Evidence ausente / state novo       -> proibido
```

Uma implementação pode usar commits separados ou uma transação que preserve o mesmo
contrato, mas não pode permitir progresso certificado sem a durabilidade exigida.

### 15. Sobreposição e múltiplas subscriptions permanecem explícitas

Duas Acquisition Subscriptions que usem o mesmo Telegram self user não compartilham linha
de `SourceSyncState` implicitamente.

Elas possuem keys distintas por `subscription_id`.

O primeiro MVP pode operar uma única subscription ativa. Suporte arquitetural a mais de
uma não exige, neste ADR, múltiplos clients, múltiplos managers ou orchestration complexa.

Se uma otimização futura desejar compartilhar uma única authority de recovery entre
subscriptions distintas, ela precisará demonstrar que não funde authorities contrariando
L3 ADR 006 e esta Decision.

### 16. Mudanças de identidade seguem L3 ADR 006

Mudança material do scope de acquisition cria nova `subscription_id` enquanto Evidence
não carregar versionamento separado da configuração.

Consequentemente, ela também cria uma nova partition lógica de SourceSyncState.

State antigo não é automaticamente copiado para a nova identity. Qualquer seed/migração de
continuidade entre identities exige operação explícita coberta por Evidence/recovery e não
é decidida aqui.

## Opções consideradas

### A. Manter somente `user_id` como primary key

Rejeitada para L3. Era suficiente na Proposal histórica 021, mas conflita com a L3 ADR 006
Accepted, que exige partition por acquisition.

### B. Alterar/forkar a interface StateStorage do gotd para receber subscription_id

Rejeitada. O upstream já fornece uma interface estreita e estável baseada em `userID`.
Scoping do adapter resolve a partition local sem fork nem wrapper invasivo.

### C. Chave somente por subscription_id

Rejeitada. O contrato upstream é explicitamente por usuário, e preservar `user_id`
mantém isolamento e rastreabilidade do self user efetivamente sincronizado.

### D. UPSERT para todos os setters

Rejeitada. Setters parciais poderiam fabricar state incompleto e violariam o contrato do
gotd.

### E. Guardar common state como BLOB opaco

Rejeitada no baseline. O contrato upstream possui campos pequenos e estáveis
`pts/qts/date/seq`; colunas explícitas tornam ausência, updates parciais e inspeção
determinística sem criar codec adicional.

### F. Foreign key/cascade entre common e channel state

Rejeitada por falta de Evidence de lifecycle acoplado e por aumentar o risco de apagar
channel continuity por uma operação de common state.

### G. Unir BackfillProgress ao mesmo state

Rejeitada. Live recovery e backfill são authorities distintas.

## Gates de implementação

Antes de chamar o storage de SourceSyncState de validado:

1. migration SQL versionada cria ambas as tabelas em banco L3 novo;
2. adapter é construído com `subscription_id` explícito e validado;
3. common state round-trip preserva `pts/qts/date/seq`;
4. ausência de common state é distinta de erro de storage;
5. cada setter parcial falha quando a linha não existe;
6. setters parciais afetam exatamente uma linha;
7. `SetState` cria/substitui somente a linha completa da partition correta;
8. `GetChannelPts` distingue ausência de `pts=0`;
9. channel state é isolado por `subscription_id + user_id + channel_id`;
10. `ForEachChannels` não vaza entre subscriptions ou usuários;
11. close/reopen preserva common e channel state;
12. schema validation/reopen detecta drift material, sem runtime repair;
13. `GuardedStateStorage` prova check + state-write linearizáveis;
14. falha de state write fecha a barrier antes de retornar;
15. erro de read não cai em bootstrap;
16. restart/replay parte do state durável antigo quando Evidence foi persistida e state não;
17. `CGO_ENABLED=0 go test ./...`, race e gates multiplataforma continuam verdes;
18. integração real mantém os gates dos ADRs históricos 017/018, incluindo TooLong,
    resync explícito e crash/restart;
19. nenhuma operação toca o banco legado in-place.

## Fora do escopo

Esta Decision não define:

- `BackfillProgress` físico;
- policy completa de recovery RPC / `EvidenceRequired(response)`;
- supervisor concreto;
- política de backoff/restart;
- UX de reset/resync;
- hot reload de subscriptions;
- migração do banco legado;
- índices adicionais sem query workload;
- retention/GC de SourceSyncState aposentado;
- Telegram real, OTP/2FA, merge ou deploy.

## Consequências

### Positivas

- o schema respeita simultaneamente a interface do gotd e a identidade L3 de acquisition;
- common state não pode nascer parcialmente;
- duas subscriptions não colidem apenas por compartilhar conta/canal;
- state read failure não vira bootstrap silencioso;
- a integração futura com `DurabilityBarrier` possui um boundary físico pequeno;
- live sync continua separado de backfill e Evidence.

### Custos

- todas as queries carregam `subscription_id` além de `user_id`;
- a mesma conta pode possuir mais de uma partition de state;
- transições entre subscription identities são explícitas, não automáticas;
- o storage precisa verificar cardinalidade de setters parciais.

## Relação com decisões e Evidence anteriores

- ADRs históricos 016–020 continuam herdados onde já documentado para L3-004;
- ADR histórico 021 permanece `Proposed` como Proposal/Evidence histórica e **não** é
  promovido retroativamente;
- L3 ADR 006 continua authority de `subscription_id`;
- esta L3 ADR 007 é a authority corrente do schema/boundary de `SourceSyncState`;
- ADR histórico 022 de `BackfillProgress` permanece Proposed;
- a `DurabilityBarrier` implementada no candidato
  `8de39c53103266fdcaf423e2898e456ac202a28a` é compatível com este boundary, mas sua
  existência não foi usada como autoridade da Decision;
- EXP-LIMIAR-009/010 e F-STO-005 permanecem Evidence/Finding, não Decisions.

## Evidência revalidada

- `github.com/gotd/td v0.162.0` é o pin atual do Limiar 3;
- a documentação upstream de `updates.StateStorage` em v0.162.0 mantém métodos por
  `userID` e exige erro nos setters parciais quando user state não existe;
- EXP-LIMIAR-009 suportou ordering físico com o lifecycle real de `updates.Manager`;
- EXP-LIMIAR-010 suportou a mesma composição sobre `ncruces/go-sqlite3`;
- F-STO-005 identificou e cobriu o caso de UPDATE com zero rows em setters parciais;
- ADR histórico 019 Accepted define SQLite/migrations/capabilities estreitas;
- ADR histórico 020 Accepted mantém Evidence fisicamente separada de SourceSyncState.

## Acceptance

O mantenedor autorizou explicitamente revisar, melhorar e resolver formalmente a ADR 021
em 2026-10-01. A autorização durável está em
`docs/limiar3/DECISION_ACCEPTANCE_2026-10-01_SOURCE_SYNC_STATE.md`, commit
`619db50c5a311b26221058d63bad4a2d4e90462f`.

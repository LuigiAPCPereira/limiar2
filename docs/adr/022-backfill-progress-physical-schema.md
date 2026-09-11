# ADR 022 — Schema físico de BackfillProgress

Authority: Decision Record
Status: Proposed

## Contexto

O ADR 016 define que backfill histórico possui lifecycle e progresso próprios e que esse progresso não certifica continuidade do live sync. O ADR 017 exige que Evidence necessária esteja durável antes de qualquer avanço certificado. O ADR 019 estabelece SQLite/ncruces, migrations SQL versionadas e capabilities estreitas como baseline do novo storage.

A evidência acumulada agora cobre também a semântica física mínima de `BackfillProgress`:

- EXP-LIMIAR-003 suportou a separação de authority entre `SourceSyncState` e `BackfillProgress` e explicitou que `Completed=true` descreve apenas cobertura histórica;
- EXP-LIMIAR-008 suportou ordering físico `Evidence -> BackfillProgress` no mesmo SQLite experimental;
- o legado mostrou `LastMessageID == 0` como sentinel de primeira execução, mas o F-ING-010 já demonstrou que esse campo mistura live e history e não pode ser promovido diretamente a nova authority;
- PR #185 validou em SQLite real que ausência pode ser distinguida de `last_message_id=0`, que o progresso pode ser isolado por `subscription_id`, que avanço monotônico e persistência sobrevivem a reopen e que regressões podem ser rejeitadas atomicamente.

A proposta abaixo congela somente o contrato físico mínimo que essas evidências suportam. Ela não decide paginação final, política de janela histórica, wiring do collector legado nem importação do cursor antigo.

## Decision proposta

### 1. BackfillProgress possui tabela própria por subscription

O novo SQLite terá uma tabela equivalente a:

```sql
CREATE TABLE backfill_progress (
    subscription_id TEXT PRIMARY KEY,
    last_message_id INTEGER NOT NULL CHECK(last_message_id >= 0),
    completed INTEGER NOT NULL DEFAULT 0 CHECK(completed IN (0, 1))
) STRICT, WITHOUT ROWID;
```

`subscription_id` identifica o lifecycle histórico cuja posição está sendo certificada. Essa authority é operacional e não substitui identidade de domínio nem `SourceSyncState`.

### 2. Ausência é diferente de posição zero

A leitura deve preservar três estados distintos:

```text
progress encontrado
progress ausente
falha de storage
```

Uma linha existente com `last_message_id = 0` não equivale a ausência. Isso evita transportar para o novo storage o sentinel implícito usado pelo legado.

### 3. Avanço de posição é monotônico

Uma operação de avanço pode criar o primeiro registro ausente ou substituir um registro existente somente quando:

```text
novo last_message_id >= last_message_id persistido
```

Uma tentativa de reduzir `last_message_id` deve falhar fechada e não modificar o registro.

A comparação e a escrita precisam ocorrer em um único boundary transacional, de modo que detecção de existência, rejeição de regressão e criação inicial não dependam de um check-then-act fora da transação.

### 4. `completed` descreve apenas lifecycle histórico

`completed = 1` significa somente que a política de backfill declarou a janela histórica correspondente coberta.

Não significa que:

- `pts/qts/seq/date` estejam contínuos;
- channel PTS esteja contínuo;
- gaps live tenham sido recuperados;
- nenhuma Evidence futura possa existir para a mesma subscription.

Nenhum consumidor pode usar `completed` para avançar, reparar ou inicializar `SourceSyncState`.

### 5. BackfillProgress permanece separado de SourceSyncState

Mesmo residindo no mesmo SQLite:

```text
Evidence         -> observação durável append-only
SourceSyncState  -> continuidade live certificada
BackfillProgress -> posição/cobertura histórica
```

Não haverá FK ou write path que transforme `BackfillProgress` em requisito de existência de `SourceSyncState`, ou vice-versa, sem evidência futura específica.

### 6. Evidence durável precede avanço de BackfillProgress

O repository físico de `BackfillProgress` não recebe autoridade para decidir quando progresso pode ser certificado.

O caller deve preservar a ordem:

```text
history obtido
    ↓
Evidence necessária durável
    ↓
BackfillProgress pode avançar
```

Se a persistência da Evidence falhar, o progresso não avança. Evidence durável com progresso antigo é aceitável e implica replay preferível a perda silenciosa.

### 7. Migrations SQL são a authority do schema

A tabela entra no novo banco por migration SQL versionada conforme ADR 019.

O adapter não cria nem repara schema ad hoc durante reads/writes.

### 8. Capability permanece estreita

Consumidores não recebem `*sql.DB`. A implementação deve expor apenas operações necessárias ao lifecycle de backfill, incluindo leitura, avanço monotônico e marcação de conclusão quando aplicável.

A API concreta pode ser refinada durante implementação, desde que não permita regressão silenciosa nem misture authority live.

## Consequências

### Positivas

- ausência não depende de sentinel numérico;
- progresso histórico deixa de compartilhar authority com live sync;
- regressões são rejeitadas explicitamente;
- posição e conclusão sobrevivem a restart;
- o contrato continua pequeno e compatível com o SQLite já adotado;
- Evidence-before-progress permanece obrigatório.

### Custos

- o write path precisa de atomicidade para avanço/criação;
- `completed` exige semântica disciplinada para não ser confundido com integridade live;
- importação do legado precisa decidir separadamente como interpretar `channels.last_message_id`;
- paginação e política de janela histórica continuam fora deste ADR.

## Fora do escopo

Este ADR não decide:

- algoritmo final de paginação/history;
- tamanho de página, limites temporais ou política de retry;
- wiring do collector legado;
- importação de `channels.last_message_id`;
- Source Admission / ADR 018;
- schema ou implementação de `SourceSyncState` / ADR 021;
- policy de `differenceTooLong`/`ChannelDifferenceTooLong`;
- projeções de mensagens;
- índices adicionais sem workload demonstrado.

## Gates antes de implementação de produção

Se este ADR for aceito:

1. adicionar migration SQL de `backfill_progress` no novo storage SQLite;
2. implementar capability estreita sem expor `*sql.DB`;
3. portar os contracts de ausência versus zero, isolamento por subscription, reopen e regressão;
4. provar atomicidade da criação/avanço sob concorrência relevante;
5. provar que falha de Evidence impede avanço de `BackfillProgress` no slice de integração;
6. manter `CGO_ENABLED=0`, race detector, Travis/CI e CodeScene verdes;
7. integrar com o lifecycle real de backfill somente em slice separado.

## Evidência

- ADR 016 — Source Evidence e sincronização Telegram;
- ADR 017 — boundary durável de recovery Telegram;
- ADR 019 — baseline de storage SQLite local;
- PROPOSAL-STO-001 — SQLite local e contratos de state/progress separados;
- EXP-LIMIAR-003 — separação de authority backfill/live;
- EXP-LIMIAR-008 — ordering físico Evidence/progress;
- F-ING-010 — colisão do cursor legado entre live e history;
- PR #185 — semântica física de BackfillProgress em SQLite.

## Escopo da proposta

`Status: Proposed` não autoriza implementação arquitetural de produção.

A promoção para `Accepted` requer autorização explícita do mantenedor conforme `docs/adr/README.md`. Até lá, os ADRs Accepted anteriores e os experimentos Supported continuam sendo authority/evidence nos seus respectivos escopos.

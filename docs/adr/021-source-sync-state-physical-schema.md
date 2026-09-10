# ADR 021 — Schema físico de SourceSyncState

Authority: Decision Record
Status: Proposed

## Contexto

Os ADRs 016 e 017 já definem que a continuidade live usa o state nativo do Telegram (`pts`, `qts`, `seq`, `date` e channel state), que `LastMessageID` não é authority de live sync e que Evidence exigida deve estar durável antes do avanço certificado de `SourceSyncState`.

O ADR 019 escolhe SQLite/ncruces, migrations SQL versionadas e repositories estreitos como baseline do novo storage. O ADR 020 materializa o schema físico de Evidence, mas deixa `SourceSyncState` explicitamente separado.

A evidência executável agora cobre os dois componentes físicos do contrato de `updates.StateStorage`:

- EXP-LIMIAR-009/010 comprovou ordering físico Evidence -> common state (`pts/qts/date/seq`) e persistência/restart;
- PR #181 comprovou channel PTS em tabela independente com chave composta `(user_id, channel_id)`, incluindo ausência distinta de `pts=0`, upsert, isolamento entre usuários/canais, enumeração por usuário e reopen físico;
- F-STO-005 + PR #182 comprovou que `SetPts`, `SetQts`, `SetDate`, `SetSeq` e `SetDateSeq` devem falhar quando o common state do usuário não existe. Um `UPDATE` que afeta zero linhas não pode ser tratado como sucesso nem convertido em UPSERT parcial.

Essa evidência é suficiente para propor o contrato físico mínimo do adapter de `SourceSyncState`, sem decidir wiring Telegram, Source Admission, BackfillProgress ou políticas ainda abertas de recovery.

## Decision proposta

### 1. Common state é uma linha completa por usuário

O novo SQLite terá uma tabela equivalente a:

```sql
CREATE TABLE source_sync_state (
    user_id INTEGER PRIMARY KEY,
    pts     INTEGER NOT NULL,
    qts     INTEGER NOT NULL,
    date    INTEGER NOT NULL,
    seq     INTEGER NOT NULL
) STRICT;
```

`user_id` pertence ao escopo operacional do `updates.Manager`; a tabela não representa identidade de domínio nem progresso de backfill.

Todos os campos do common state são obrigatórios. Não existe estado parcialmente inicializado persistido.

### 2. `SetState` estabelece ou substitui o common state completo

A operação correspondente a `updates.StateStorage.SetState` pode fazer insert/upsert da linha completa de `(pts,qts,date,seq)` para o usuário.

Ela é o caminho autorizado para criar common state ausente.

### 3. Setters parciais somente atualizam state existente

`SetPts`, `SetQts`, `SetDate`, `SetSeq` e `SetDateSeq` devem executar update de uma linha já existente e verificar o resultado da escrita.

Se nenhuma linha existir para `user_id`, a operação retorna erro e não fabrica valores para os demais campos.

UPSERT nos setters parciais é proibido porque criaria state incompleto ou inventado, contrariando o contrato validado do gotd.

O adapter pode tratar uma cardinalidade diferente de exatamente uma linha como falha fechada; a chave primária torna múltiplas linhas impossível no schema normal, mas a pós-condição permanece explícita.

### 4. Ausência e falha de leitura permanecem distintas

`GetState` deve preservar três resultados semanticamente distintos:

```text
state encontrado
state ausente
falha ao ler state
```

Erro de leitura nunca pode virar ausência/bootstrap, conforme ADR 017.

### 5. Channel PTS usa authority física independente por `(user_id, channel_id)`

O novo SQLite terá uma tabela equivalente a:

```sql
CREATE TABLE source_sync_channel_state (
    user_id    INTEGER NOT NULL,
    channel_id INTEGER NOT NULL,
    pts        INTEGER NOT NULL,
    PRIMARY KEY (user_id, channel_id)
) STRICT;
```

A chave composta é necessária para evitar colisão do mesmo channel ID entre contas e para manter isolamento entre canais do mesmo usuário.

Não é adicionada foreign key de `source_sync_channel_state.user_id` para `source_sync_state.user_id`. A evidência atual não demonstra que channel state deva depender do lifecycle físico de uma linha de common state, e impor essa relação criaria acoplamento não exigido pelo contrato do gotd.

### 6. Channel PTS ausente é distinto de PTS zero

`GetChannelPts` deve retornar ausência separadamente do valor inteiro. A existência de uma linha com `pts=0` não equivale a state ausente.

`SetChannelPts` pode inserir ou atualizar o `(user_id, channel_id)` correspondente, conforme o comportamento exercitado na PR #181.

### 7. Enumeração de channel state é sempre limitada ao usuário

A operação correspondente a `ForEachChannels(ctx, userID, fn)` consulta somente as linhas daquele `user_id` e entrega cada `(channel_id, pts)` ao callback.

Nenhuma enumeração global de todos os usuários faz parte dessa capability.

A ordem de enumeração não é authority e não deve ser usada como mecanismo de sync, salvo requisito upstream futuro explicitamente demonstrado.

### 8. SourceSyncState continua separado de Evidence e BackfillProgress

As tabelas podem residir no mesmo arquivo SQLite do ADR 019, mas não compartilham authority:

```text
Evidence         -> observação durável append-only
SourceSyncState  -> continuidade live certificada pelo recovery manager
BackfillProgress -> progresso de aquisição histórica
```

Nenhum campo de message ID é adicionado a `SourceSyncState` para substituir PTS/QTS/SEQ/DATE.

### 9. Escritas passam pelo boundary do ADR 017

Este ADR decide schema e semântica do adapter; ele não remove `GuardedStateStorage`/barrier.

Na integração real, state writes continuam condicionados à Evidence exigida já durável e ao lifecycle fail-stop definido pelo ADR 017.

O repository físico de state não recebe autoridade para decidir sozinho quando uma transição pode ser certificada.

### 10. Migrations SQL são a única authority de schema

As duas tabelas entram no novo storage por migration SQL versionada, seguindo ADR 019.

Não haverá criação/reparo ad hoc dessas tabelas pelo adapter durante `GetState`/`SetState`/channel operations.

## Consequências

### Positivas

- o adapter físico corresponde diretamente ao contrato exercitado de `updates.StateStorage`;
- common state nunca nasce parcialmente por conveniência de UPSERT;
- ausência permanece distinguível de zero e de falha de storage;
- contas e canais não colidem na authority de channel PTS;
- não há acoplamento artificial entre common state e channel state;
- o schema permanece pequeno e separado de backfill e de Evidence.

### Custos

- o adapter precisa verificar cardinalidade/ausência nas writes parciais;
- haverá duas estruturas físicas para o state nativo do Telegram;
- recovery/wiring ainda precisa preservar o boundary do ADR 017 em torno dessas capabilities;
- futuras mudanças no contrato de `gotd/td` exigem revalidação antes de alterar a semântica persistida.

## Fora do escopo

Este ADR não decide:

- Source Admission / ADR 018;
- wiring de `updates.Manager` em produção;
- `BackfillProgress` físico;
- session storage ou peer cache;
- política de `differenceTooLong`/`ChannelDifferenceTooLong` além do ADR 017;
- reset/resync UX ou política de backoff;
- migração/importação do banco legado;
- projeções de mensagens;
- índices adicionais sem evidência de query workload.

## Gates antes de implementação de produção

Se este ADR for aceito:

1. adicionar migration SQL das duas tabelas no novo storage SQLite;
2. implementar capability estreita que satisfaça o contrato necessário de `updates.StateStorage` sem expor `*sql.DB`;
3. portar para a implementação real os contracts de ausência, common state, channel PTS e reopen;
4. preservar `CGO_ENABLED=0 go test`, race detector, CodeScene e gates gerais;
5. provar que nenhum setter parcial cria state ausente;
6. provar isolamento `(user_id, channel_id)` e enumeração por usuário;
7. integrar com `GuardedStateStorage` apenas em slice separado, mantendo Evidence-before-state.

## Evidência

- ADR 016 — Source Evidence e sincronização Telegram;
- ADR 017 — Boundary durável de recovery Telegram;
- ADR 019 — baseline de storage SQLite local;
- ADR 020 — schema físico de Evidence;
- EXP-LIMIAR-009 — manager physical ordering;
- EXP-LIMIAR-010 — manager/ncruces physical ordering;
- PR #181 — physical channel PTS authority;
- F-STO-005 — partial state setters must fail if state missing;
- PR #182 — executable missing-state contract.

## Escopo da proposta

`Status: Proposed` não autoriza implementação arquitetural de produção.

A promoção para `Accepted` requer autorização explícita do mantenedor conforme `docs/adr/README.md`. Até lá, os experimentos e ADRs Accepted anteriores continuam sendo authority.
# EXP-LIMIAR-008 — Ordering físico Evidence → state/progress

Authority: Non-authoritative
Status: Supported

## Hipótese

O envelope físico de Evidence já validado pode coexistir com capabilities estreitas de `SourceSyncState` e `BackfillProgress` preservando, no mesmo SQLite experimental, a ordem mínima:

```text
commit Evidence
      ↓
commit SourceSyncState ou BackfillProgress
```

inclusive quando ocorre falha determinística imediatamente após o commit de Evidence.

## Origem

O ADR 019 continua `Proposed` e lista como gate antes de produção preservar os contracts de `Evidence -> SourceSyncState/BackfillProgress` no storage real. EXP-LIMIAR-002 já demonstrou a propriedade em um harness de durability; EXP-LIMIAR-004 e 005 materializaram um envelope físico mínimo de Evidence e enforcement append-only experimental.

O passo de maior valor que pode ser executado sem promover o ADR é integrar esses dois lados no módulo experimental atual, mantendo `SourceSyncState` e `BackfillProgress` como authorities distintas.

Este EXP não fecha o gate de integração de produção: ele reduz a incerteza antes dessa etapa e permanece fora do build/run normal do produto.

## Boundary

Arquivo executável:

`experiments/evidence-envelope/physical_ordering_test.go`

O harness adiciona somente ao banco temporário do teste:

- tabela `source_sync_state` com `subscription_id` + `pts`;
- tabela `backfill_progress` com `subscription_id` + `last_message_id`;
- capability experimental que sempre comita Evidence antes de tentar avançar state/progress.

Não altera schema, default, dependência ou código de produção.

## Cenários

### SourceSyncState

Estado inicial `pts=7`.

A operação:

1. persiste e comita Evidence;
2. injeta falha antes do update de `SourceSyncState`.

Resultado esperado após reopen:

```text
Evidence = durável
pts      = 7 (antigo)
```

O estado proibido `Evidence ausente / pts avançado` não deve ser produzido pela capability exercitada.

### BackfillProgress

Progresso inicial `last_message_id=100`.

A mesma falha entre commits deve produzir após reopen:

```text
Evidence        = durável
last_message_id = 100 (antigo)
```

### Caminho de sucesso

Sem falha injetada, state/progress só avançam depois do commit de Evidence. `PRAGMA integrity_check` deve retornar `ok`.

## Critérios de suporte

A hipótese pode mudar para `Supported` quando, no mesmo head:

1. os três cenários acima passarem;
2. reopen preservar Evidence e state/progress antigo nos casos de falha;
3. `go vet ./...` passar no módulo experimental;
4. `CGO_ENABLED=0 go test ./... -count=1` passar;
5. `go test -race ./... -count=1` passar;
6. a CI geral do repositório não revelar regressão relacionada.

## Limites epistemológicos

Este experimento não prova:

- integração com o lifecycle real do `updates.Manager`;
- crash por `SIGKILL` exatamente durante chamadas de `fsync`;
- power-loss;
- equivalência em plataformas além do runner exercitado;
- que acesso SQL bruto não possa violar ordering deliberadamente;
- schema canônico final de `SourceSyncState` ou `BackfillProgress`.

A capability é um boundary de ownership experimental, não um security boundary contra código com acesso SQL irrestrito.

## Relação com EXP-LIMIAR-007

EXP-LIMIAR-007 permanece `In Progress` e continua exigindo uma cópia real descartável do banco legado. EXP-LIMIAR-008 não substitui nem enfraquece esse gate; ele avança um gate independente enquanto a Evidence operacional externa não está disponível.

## Resultado atual

`Supported` no boundary experimental exercitado.

No head `75937e509c689e5a4d686c58f6e01e79126c256b`, os três cenários do harness passaram e os gates executáveis do módulo foram confirmados pelo workflow dedicado `EXP-LIMIAR-008`: verificação do módulo, `go vet`, testes com `CGO_ENABLED=0` e race detector. No mesmo head, a CI geral do repositório, EXP-LIMIAR-005 e EXP-LIMIAR-006 também concluíram com sucesso.

A Evidence suporta a hipótese de ordering físico dentro deste harness e destas capabilities estreitas. Ela não promove o ADR 019, não prova integração com o lifecycle real de produção e não substitui a execução do EXP-LIMIAR-007 contra uma cópia real do legado.

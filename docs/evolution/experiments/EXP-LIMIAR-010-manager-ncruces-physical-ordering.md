# EXP-LIMIAR-010 — `updates.Manager` + ncruces/go-sqlite3 ordering físico

Authority: Non-authoritative
Status: Supported

## Hipótese

A composição que permaneceu aberta após EXP-LIMIAR-008 e EXP-LIMIAR-009 pode preservar o contract aceito do ADR 017 usando diretamente o candidato físico de ADR 019:

```text
updates.Manager
      ↓
commit Evidence (ncruces/go-sqlite3)
      ↓
DurabilityBarrier
      ↓
commit SourceSyncState.pts (ncruces/go-sqlite3)
```

inclusive quando uma falha é injetada imediatamente após o commit de Evidence.

## Origem

EXP-LIMIAR-008 suportou `Evidence → SourceSyncState/BackfillProgress` no envelope físico experimental baseado em `github.com/ncruces/go-sqlite3`, mas não exercitou o lifecycle real do `updates.Manager`.

EXP-LIMIAR-009 exercitou o lifecycle real do `updates.Manager` com close + reopen físico e `DurabilityBarrier`, mas usou Tursogo de forma instrumental. O próprio EXP-009 registrou explicitamente que a combinação `updates.Manager + ncruces/go-sqlite3` permanecia sem prova.

Este experimento fecha somente essa lacuna composicional. Ele não aceita ADR 019 e não muda o storage de produção.

## Boundary

Arquivo executável:

`internal/telegram/gotd_ncruces_ordering_exp_test.go`

O arquivo possui build tag `exp010_ncruces`. Assim, a dependência candidata não entra em `go.mod`, `go.sum`, builds ou testes normais do produto. O workflow dedicado instala `github.com/ncruces/go-sqlite3/driver@v0.35.3` apenas no checkout efêmero da execução e roda o teste com a tag habilitada.

O harness reutiliza diretamente os mesmos componentes de contract test já exercitados por EXP-LIMIAR-009:

- `updates.Manager` real da versão de gotd pinada pelo produto;
- `managerPhysicalStorage`;
- `durabilityBarrier`;
- API fake e updates usados nos contracts ADR 017;
- schema mínimo de `Evidence` + `SourceSyncState`.

A única variável material trocada em relação ao EXP-009 é o driver físico: `turso` → `sqlite3` de ncruces.

## Cenários

### Caminho de sucesso

Estado inicial `pts=7`. Um update live para `pts=8` deve, após `Handle`, parada, close e reopen, resultar em:

```text
Evidence = 1 registro durável
pts      = 8
```

### Falha após Evidence

O handler comita Evidence, falha a `DurabilityBarrier` e retorna erro. O `updates.Manager` pode tentar avançar state, mas a barrier deve rejeitar a escrita. Após close + reopen:

```text
Evidence = 1 registro durável
pts      = 7
```

`PRAGMA integrity_check` deve retornar `ok`.

O estado proibido continua sendo:

```text
Evidence ausente / pts avançado
```

## Critérios de suporte

A hipótese pode mudar para `Supported` quando, no mesmo head:

1. os dois cenários passarem com `CGO_ENABLED=0`;
2. os asserts forem feitos após close + reopen do arquivo físico ncruces;
3. `PRAGMA integrity_check = ok` no cenário de falha;
4. `go vet -tags exp010_ncruces ./internal/telegram` passar após instalação efêmera do driver pinado;
5. o race detector passar nos cenários EXP-010;
6. a suíte normal sem a build tag continuar verde;
7. a CI geral não revelar regressão relacionada.

## Evidence

No head executável `91a16a7586d00e03d6f92767b1bd75c00763dcee`, os dois gates materializados para esta hipótese concluíram com sucesso:

- `EXP-LIMIAR-010 #1`: success;
- `CI #514`: success.

O workflow dedicado executou os cenários físicos com o driver ncruces instalado apenas no checkout efêmero, incluindo os gates definidos acima: `CGO_ENABLED=0`, close + reopen, `integrity_check`, vet com a build tag, race detector e suíte normal sem a tag. A CI geral no mesmo head também ficou verde.

Essa Evidence suporta a composição exercitada `updates.Manager → Evidence → DurabilityBarrier → SourceSyncState.pts` sobre `ncruces/go-sqlite3`, incluindo a falha injetada imediatamente após o commit de Evidence sem produzir o estado proibido de state avançado sem Evidence.

## Limites epistemológicos

Mesmo suportado, este experimento não prova:

- crash por `SIGKILL` exatamente durante fsync da composição real;
- power-loss;
- equivalência entre todos os VFS/plataformas;
- schema canônico final de `SourceSyncState`;
- `BackfillProgress` dentro de `updates.Manager` (não pertence ao lifecycle live);
- implementação de produção do ADR 019;
- auditoria do banco legado real exigida pelo EXP-LIMIAR-007.

## Relação com decisões

- ADR 016 e ADR 017 permanecem como contracts arquiteturais existentes.
- ADR 019 permanece `Proposed`; este resultado não o aceita automaticamente.
- EXP-LIMIAR-007 permanece `In Progress` e independente.
- Nenhum default, schema ou dependency de produção é alterado.

## Resultado atual

`Supported`.

A lacuna composicional entre EXP-LIMIAR-008 e EXP-LIMIAR-009 está fechada para o boundary exercitado. O próximo avanço deve usar esta Evidence para informar a decisão sobre o candidato físico, sem confundir suporte experimental com aceitação do ADR 019 ou implementação de produção.

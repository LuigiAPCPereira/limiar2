# EXP-LIMIAR-009 — `updates.Manager` sobre ordering físico de live sync

Authority: Non-authoritative
Status: In Progress

## Hipótese

O contract aceito do ADR 017 para live sync pode ser exercitado contra persistência SQLite física de modo que o lifecycle real do `updates.Manager` preserve:

```text
commit Evidence
      ↓
commit SourceSyncState.pts
```

inclusive quando ocorre falha imediatamente depois de Evidence ter sido comitada.

## Origem

O EXP-LIMIAR-008 suportou ordering físico entre Evidence e state/progress em um harness isolado com o envelope experimental baseado em `ncruces/go-sqlite3`.

Separadamente, os gates do ADR 017 exercitam `updates.Manager`, `DurabilityBarrier` e `GuardedStateStorage`, mas usam storage em memória.

A incerteza agora é composicional: o lifecycle real de `updates.Manager` ainda não foi exercitado contra um boundary físico persistente com reopen.

Este experimento reduz apenas essa incerteza. Ele não promove o ADR 019 e não muda storage, schema, default ou dependência de produção.

## Boundary

Arquivo executável:

`internal/telegram/gotd_physical_ordering_exp_test.go`

O teste reutiliza a versão de `gotd/td` já pinada pelo produto e uma base SQLite temporária aberta pelo driver Tursogo já presente no repositório. Isso evita adicionar dependência estrutural ou alterar o módulo de produção apenas para o experimento.

O schema temporário contém somente:

- `evidence` com payload opaco;
- `source_sync_state` com o estado mínimo necessário ao `updates.Manager`.

A capability de Evidence realiza commit explícito antes de retornar ao handler. A escrita de state continua protegida pela `DurabilityBarrier` usada pelos contracts ADR 017.

## Cenários

### Caminho de sucesso

Estado inicial:

```text
pts = 7
```

Um update live com `pts=8` deve produzir, após `updates.Manager.Handle`, parada, close e reopen:

```text
Evidence = 1 registro durável
pts      = 8
```

### Falha após commit de Evidence

O handler:

1. persiste e comita Evidence;
2. fecha a `DurabilityBarrier` com falha injetada;
3. retorna erro ao manager.

O `updates.Manager` ainda pode tentar sua transição de state, porém a barrier deve rejeitá-la. Após parada, close e reopen, o estado esperado é:

```text
Evidence = 1 registro durável
pts      = 7 (antigo)
```

O estado proibido `Evidence ausente / pts=8` não deve ocorrer.

## Critérios de suporte

A hipótese pode mudar para `Supported` quando, no mesmo head:

1. os dois cenários passarem;
2. os asserts forem feitos após close + reopen do arquivo físico;
3. `PRAGMA integrity_check` retornar `ok` no cenário de falha;
4. `go vet ./...` passar;
5. `CGO_ENABLED=0 go test ./... -count=1` passar;
6. `go test -race ./... -count=1` passar;
7. a CI geral não revelar regressão relacionada.

## Limites epistemológicos

Este experimento não prova:

- a combinação `updates.Manager + ncruces/go-sqlite3`; o EXP-LIMIAR-008 e este EXP cobrem lados diferentes da composição;
- `BackfillProgress`, porque ele não pertence ao lifecycle de live sync do `updates.Manager`;
- crash por `SIGKILL` durante a composição real;
- power-loss durante `fsync`;
- schema canônico final de `SourceSyncState`;
- implementação de produção do ADR 019.

O uso de Tursogo aqui é deliberadamente instrumental: ele já existe no módulo raiz e permite testar o lifecycle real de gotd contra persistência física sem transformar `ncruces/go-sqlite3` em dependência obrigatória de produção antes de uma Decision aceita.

## Relação com os gates atuais

- EXP-LIMIAR-007 permanece `In Progress` e continua dependendo de uma cópia real descartável do legado.
- EXP-LIMIAR-008 permanece `Supported` no boundary experimental com ncruces.
- ADR 019 permanece `Proposed`.

Se este experimento for suportado, ainda restará um gate explícito para testar a composição completa do candidato `ncruces/go-sqlite3` com os contracts ADR 016/017 antes de produção.

## Resultado atual

`In Progress` até os gates executáveis deste head concluírem.

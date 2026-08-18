# EXP-LIMIAR-002 — Durabilidade de storage e Evidence

Authority: Non-authoritative
Status: Supported with limitations

## Hipótese

É possível usar um SQLite local, acessado por `database/sql` e sem dependência de CGO no
runtime normal, para cumprir o boundary de durabilidade exigido pelos ADRs 016 e 017:

```text
Evidence durável / state antigo     -> replay seguro
Evidence durável / state novo       -> normal
Evidence não durável / state antigo -> falha segura
Evidence não durável / state novo   -> proibido
```

O experimento também compara duas engines candidatas sem transformar o resultado em
Decision arquitetural.

## Engines avaliadas

- `turso.tech/database/tursogo v0.7.2`;
- `github.com/ncruces/go-sqlite3 v0.35.3` via `database/sql`.

Ambiente das execuções:

- runner self-hosted `LuigiCachyOS`;
- Go 1.26.6 linux/amd64.

## Fase A — contrato funcional de durability

O mesmo harness foi executado contra as duas engines com:

- `PRAGMA journal_mode=WAL`;
- `PRAGMA synchronous=FULL`;
- `PRAGMA foreign_keys=ON`;
- `PRAGMA busy_timeout=5000`;
- uma única conexão aberta pelo `database/sql`.

Casos cobertos:

1. WAL e FULL com readback;
2. foreign keys habilitadas e enforcement real;
3. payload BLOB preservado byte a byte após close/reopen;
4. Evidence commitada sem avanço implícito de sync state;
5. avanço explícito de state persistido após reopen;
6. rollback não deixa Evidence parcial;
7. `PRAGMA integrity_check` retorna `ok`;
8. lote de 10.000 Evidence em transação;
9. `VACUUM INTO` gera backup reabrível com a mesma contagem;
10. Evidence já commitada permanece após falha injetada de state write, enquanto o state
    persistido permanece antigo.

Workflow experimental: `agent-runtime` run `32183259646`.

### Execução normal + race

Com CGO disponível para o próprio race detector:

```text
go test ./... -count=1 -v    PASS
go test -race ./... -count=1 PASS
```

As duas engines passaram.

### Runtime sem CGO

A suíte completa foi repetida com:

```text
CGO_ENABLED=0 go test ./... -count=1 -v
```

As duas engines passaram, incluindo o caso Evidence commitada + state write rejeitado.

O `-race` não é executável em Linux com `CGO_ENABLED=0` porque essa é uma restrição do
race detector do Go, não uma dependência das engines. A execução separou os gates:
runtime normal sem CGO e race com CGO habilitado; ambos passaram.

## Fase B — crash abrupto entre Evidence e state/progress

A Fase A usava rollback/falha injetada e reopen normal. Para aproximar o gate de
crash/restart real, a Fase B executou cada writer em processo auxiliar e aplicou
`SIGKILL` em boundaries observáveis antes/depois dos commits.

Authorities exercitadas:

- `SourceSyncState` (`pts`);
- `BackfillProgress` (`next_offset`).

Matriz esperada para cada authority:

```text
antes do commit de Evidence  -> Evidence ausente / progress antigo
após commit de Evidence       -> Evidence presente / progress antigo
antes do commit de progress   -> Evidence presente / progress antigo
após commit de progress        -> Evidence presente / progress novo
```

A matriz foi executada contra as duas engines, com três repetições por combinação:

```text
2 engines x 2 authorities x 4 boundaries x 3 repetições = 48 crashes
```

Após cada `SIGKILL`, o banco foi reaberto e verificou-se:

- contagem de Evidence;
- PTS ou BackfillProgress esperado;
- `PRAGMA integrity_check = ok`.

Execução final:

```text
Workflow run: agent-runtime#32198152076
ncruces/go-sqlite3: v0.35.3
Tursogo: v0.7.2

go test ./... -count=1 -v    PASS
go test -race ./... -count=1 PASS
```

As duas engines preservaram todos os estados esperados. Nenhum cenário produziu
`Evidence não durável / progress persistido como avançado`.

Finding detalhado:
`docs/evolution/findings/F-STO-002-process-crash-preserves-evidence-before-progress.md`.

### Limite desta fase

O teste mata o processo antes/depois de `Commit`; ele não intercepta deterministicamente
a syscall/fsync no meio do commit. Portanto, **process crash** está coberto, enquanto
**power-loss durante fsync** continua fora do que foi provado.

## Finding adicional — `synchronous=NORMAL`

O Limiar legado usa `PRAGMA synchronous=NORMAL`.

No Tursogo v0.7.2, o probe runtime aceitou o valor e o readback retornou `1`. Porém, a
matriz oficial de compatibilidade da engine documenta apenas `OFF` e `FULL` para esse
PRAGMA.

Consequência: compatibilidade observada não deve ser confundida com contrato upstream.
O candidato da rebaseline usa `FULL`.

Finding detalhado:
`docs/evolution/findings/F-STO-001-tursogo-synchronous-compatibility.md`.

## Resultado comparativo

As duas engines satisfizeram o contrato avaliado, inclusive o novo gate de process
crash/restart.

O experimento **não rejeita Tursogo por corretude funcional**. Entretanto,
`ncruces/go-sqlite3` permanece candidato preferencial para a nova base porque usa a
implementação SQLite, mantém `database/sql` e o objetivo de runtime sem CGO, enquanto o
Tursogo possui uma superfície de compatibilidade SQLite própria e parcialmente
implementada.

A diferença observada no lote de 10.000 registros favoreceu `ncruces/go-sqlite3` nesse
ambiente, mas esse número é somente diagnóstico do harness e **não** é benchmark suficiente
para decidir arquitetura.

## O que este experimento NÃO prova

- resistência real a power-loss/fsync interrompido;
- segurança sob corrupção física do filesystem;
- operação multi-processo simultânea;
- equivalência de locking/WAL em todos os sistemas operacionais;
- comportamento com bancos muito grandes;
- criptografia em repouso;
- estratégia de sessão MTProto;
- schema final de Evidence;
- codec final do payload Telegram;
- política completa de backup/restore operacional.

## Conclusão

**Supported with limitations.**

Existe pelo menos uma rota cgo-free e baseada em SQLite capaz de cumprir os contratos de
durabilidade já aceitos, incluindo restart após `SIGKILL` nos boundaries entre Evidence
e state/progress. Tanto Tursogo v0.7.2 quanto `ncruces/go-sqlite3 v0.35.3` passaram o
harness atual.

A escolha da engine, o schema físico mínimo e a política de power-loss/filesystem
permanecem fora da autoridade deste experimento e exigem Decision explícita quando
estruturais.

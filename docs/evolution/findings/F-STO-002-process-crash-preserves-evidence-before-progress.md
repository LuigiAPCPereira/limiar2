# F-STO-002 — Crash de processo preserva Evidence antes de state/progress

Authority: Non-authoritative
Status: Confirmed

## Finding

Com `WAL + synchronous=FULL`, tanto `github.com/ncruces/go-sqlite3 v0.35.3` quanto
`turso.tech/database/tursogo v0.7.2` preservaram o contrato físico esperado quando o
processo foi encerrado abruptamente por `SIGKILL` entre os boundaries de commit de
Evidence e de progresso operacional.

Para live sync, após reopen:

```text
antes do commit de Evidence  -> Evidence ausente / PTS antigo
após commit de Evidence       -> Evidence presente / PTS antigo
antes do commit de PTS        -> Evidence presente / PTS antigo
após commit de PTS             -> Evidence presente / PTS novo
```

Para backfill, o mesmo padrão foi observado substituindo PTS por `BackfillProgress`.

Nenhum cenário produziu o estado proibido:

```text
Evidence não durável / state ou progress persistido como avançado
```

## Harness

O teste utilizou bancos reais em arquivo com:

```text
SetMaxOpenConns(1)
SetMaxIdleConns(1)
PRAGMA journal_mode=WAL
PRAGMA synchronous=FULL
PRAGMA foreign_keys=ON
PRAGMA busy_timeout=5000
```

Cada cenário:

1. cria um banco limpo;
2. executa o writer em processo auxiliar;
3. espera um marcador imediatamente antes/depois do boundary de commit escolhido;
4. encerra o processo auxiliar por `SIGKILL`;
5. reabre o arquivo em novo processo/lifecycle;
6. verifica Evidence, `SourceSyncState`/`BackfillProgress` e `PRAGMA integrity_check`.

Foram usadas três repetições por combinação para reduzir dependência acidental de timing.

Matriz final:

```text
2 engines
x 2 authorities (live/backfill)
x 4 boundaries
x 3 repetições
= 48 crashes controlados
```

## Execução

Laboratório temporário: `LuigiAPCPereira/agent-runtime#21` — fechado sem merge.

Ambiente:

```text
Runner: LuigiCachyOS / actions-runner 2.336.0
Go: 1.26.6 linux/amd64
ncruces/go-sqlite3: v0.35.3
Tursogo: v0.7.2
Workflow run: 32198152076
```

Resultado:

```text
go test ./... -count=1 -v    PASS
go test -race ./... -count=1 PASS
PRAGMA integrity_check        ok em todos os reopens
```

## Interpretação

O resultado suporta o desenho em que Source Admission e backfill fazem um commit durável
de Evidence antes de permitirem a chamada separada que persiste o state/progress
correspondente.

Isso é especialmente relevante porque os ADRs 016/017 permitem e esperam o estado
`Evidence durável / state antigo`: após crash, replay é preferível a perda silenciosa.

O teste também mostra que `SourceSyncState` e `BackfillProgress` podem usar a mesma engine
física sem precisarem compartilhar authority ou transação entre si.

## Limitações

Este Finding **não** prova:

- sobrevivência a queda de energia durante o `fsync`/commit;
- corrupção física do filesystem/dispositivo;
- semântica de durability em filesystems não testados;
- acesso multi-processo simultâneo;
- comportamento em outras plataformas;
- schema físico final de Evidence;
- que Tursogo e ncruces sejam semanticamente equivalentes fora do contrato exercitado;
- escolha arquitetural de engine.

O harness mata o processo em boundaries observáveis antes/depois de `Commit`; ele não
possui hook interno da engine para interromper deterministicamente no meio da syscall de
commit.

## Consequência

O gate de **process crash/restart** para a ordem `Evidence -> state/progress` passa no
contract harness para ambas as engines avaliadas.

A escolha de engine continua sendo Decision separada. A diferença relevante permanece a
superfície de compatibilidade/documentação e os demais trade-offs, não uma falha de
corretude encontrada neste experimento.

# EXP-LIMIAR-014 — Portabilidade de compilação do novo storage SQLite

Authority: Experiment
Status: In Progress

## Pergunta

O package `internal/storage/sqlite`, no estado atual autorizado pelos ADRs 019/020, continua compilável sem CGO para alvos relevantes além do Linux X64 já exercitado, sem inferir que locking, WAL ou runtime foram validados nessas plataformas?

## Por que este experimento existe

A baseline registra Evidence operacional do novo SQLite em Linux X64, mas mantém explicitamente aberta a validação fora desse ambiente. O EXP-LIMIAR-013 demonstrou que o repositório atual passa seus gates em Linux X64 com `CGO_ENABLED=0`; isso não demonstra sequer que o boundary do novo SQLite compila para outras combinações de sistema operacional e arquitetura.

Antes de gastar runners nativos ou afirmar suporte operacional, vale separar duas perguntas diferentes:

1. o package físico do novo storage é compilável para outros targets sem CGO?;
2. o comportamento de runtime — abertura, WAL, locking, durability e backup/restore — funciona nesses targets?

Este experimento responde somente à primeira.

## Hipótese

Com Go 1.26.2 e `CGO_ENABLED=0`, os testes do package `internal/storage/sqlite` devem ser compiláveis, sem execução, para:

- `linux/arm64`;
- `windows/amd64`;
- `darwin/arm64`.

A escolha cobre uma arquitetura Linux diferente e dois sistemas operacionais relevantes, mas não declara nenhum deles como plataforma oficialmente suportada.

## Harness

O Travis recebe um terceiro job isolado, `SQLite portability / cross-build experiment`.

Para cada target, o job executa:

```sh
GOOS=<os> GOARCH=<arch> CGO_ENABLED=0 \
  go test -c -o /tmp/sqlite-<os>-<arch>.test ./internal/storage/sqlite
```

Em seguida verifica que o artefato foi materializado e não está vazio.

O job normal Linux X64/race e o job do EXP-LIMIAR-013 permanecem inalterados semanticamente. O experimento não executa binários cross-compiled, não muda código de produção, schema, migrations, defaults, dependências ou platform policy.

## Critério de suporte

A hipótese poderá ser marcada `Supported` somente se os três targets compilarem no CI real do HEAD correspondente.

Falha de um target deve ser tratada como Evidence da limitação concreta e investigada antes de qualquer correção estrutural. O experimento não autoriza adicionar build tags, trocar driver ou alterar arquitetura apenas para tornar o job verde.

## Limites

Mesmo com resultado verde, este experimento prova apenas portabilidade de compilação do package e de seus testes para os targets exercitados.

Ele **não prova**:

- execução real em ARM64, Windows ou macOS;
- semântica de filesystem ou file locking;
- comportamento WAL;
- `synchronous=FULL` ou durability física;
- backup/restore;
- performance;
- Android/Termux ou PRoot;
- compatibilidade do binário completo do Limiar nesses targets.

Esses pontos continuam exigindo Evidence nativa quando se tornarem plataformas de execução candidatas reais.

## Fora de escopo

- declarar matriz oficial de plataformas;
- alterar ADR 019 ou ADR 020;
- implementar `SourceSyncState` ou `BackfillProgress`;
- substituir Tursogo legado;
- adicionar abstrações de plataforma sem falha observada;
- transformar cross-build em prova de runtime.

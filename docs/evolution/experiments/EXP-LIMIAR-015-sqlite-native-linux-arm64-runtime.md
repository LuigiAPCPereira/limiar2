# EXP-LIMIAR-015 — Runtime nativo do novo storage SQLite em Linux ARM64

Authority: Experiment
Status: In Progress

## Pergunta

O package `internal/storage/sqlite`, já comprovadamente compilável para `linux/arm64` pelo EXP-LIMIAR-014, executa seus testes reais de storage em um host Linux ARM64 nativo com `CGO_ENABLED=0`?

## Por que este experimento existe

O EXP-LIMIAR-014 separou portabilidade de compilação de suporte operacional e demonstrou apenas que o package e seus testes podem ser cross-compiled para `linux/arm64`, `windows/amd64` e `darwin/arm64`.

A baseline continua exigindo Evidence operacional fora de Linux X64. Linux ARM64 é o próximo alvo de menor salto porque preserva o sistema operacional já exercitado e muda apenas a arquitetura, permitindo testar o driver SQLite e o filesystem em execução real antes de considerar ambientes mais diferentes.

Este experimento não declara Linux ARM64 como plataforma oficialmente suportada. Ele tenta apenas obter Evidence nativa do boundary SQLite existente.

## Hipótese

Em um runner Travis Linux ARM64 nativo, usando Go 1.26.2 e `CGO_ENABLED=0`, o package `internal/storage/sqlite` deve executar com sucesso sua suíte de testes atual.

Isso inclui, na medida em que os testes já materializados exercitam essas propriedades, abertura do banco, migrations, invariantes de Evidence, WAL/locking e backup/restore. O experimento não adiciona novos contratos de produção.

## Harness

O Travis recebe um job isolado:

```text
SQLite runtime / native Linux ARM64 experiment
arch: arm64
CGO_ENABLED=0
```

O job primeiro confirma o target efetivo do toolchain:

```sh
test "$(go env GOOS)" = "linux"
test "$(go env GOARCH)" = "arm64"
```

Em seguida executa:

```sh
go test -v -count=1 ./internal/storage/sqlite
```

Os jobs Linux X64/race, Linux X64/CGO-disabled e cross-build permanecem presentes para detectar regressões independentes do novo runner.

## Evidência

Ainda não coletada. O experimento permanece `In Progress` até que o job execute em infraestrutura ARM64 real e haja resultado observável.

## Critério de suporte

A hipótese poderá ser marcada `Supported` somente se:

1. o job estiver realmente em um runner Linux ARM64;
2. `go env GOARCH` retornar `arm64`;
3. `CGO_ENABLED=0` permanecer efetivo;
4. `go test -v -count=1 ./internal/storage/sqlite` concluir com sucesso no CI real do HEAD correspondente.

Falha de provisionamento do runner não é Evidence contra o SQLite. Falha de teste após o runner iniciar é Evidence e deve ser investigada antes de qualquer mudança estrutural.

## Limites

Mesmo com resultado verde, este experimento não prova:

- Windows ou macOS em runtime;
- Android/Termux ou PRoot;
- equivalência de durability entre filesystems/discos distintos;
- performance ou comportamento sob carga prolongada;
- suporte do binário completo do Limiar em ARM64;
- política oficial de plataformas.

Ele também não promove ADR 021/022 nem autoriza novo wiring de ingress.

## Fora de escopo

- trocar driver SQLite;
- adicionar abstrações específicas de ARM64 sem falha observada;
- alterar schema, migrations ou defaults;
- migrar o Tursogo legado;
- declarar suporte oficial a Linux ARM64 apenas por este experimento.

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

O harness conhecido como funcional confirma `GOOS=linux` e `GOARCH=arm64` com `go env` antes de executar a suíte. O próprio job declara `CGO_ENABLED=0` em seu ambiente.

Em seguida executa:

```sh
go test -v -count=1 ./internal/storage/sqlite
```

Os jobs Linux X64/race, Linux X64/CGO-disabled e cross-build permanecem presentes para detectar regressões independentes do novo runner.

## Evidência

Travis CI build `278803825`, associado ao HEAD executável `4c4dab9831cdc5702e0969bbb92b44dcb26026ba` da PR #192, executou quatro jobs em Linux Noble com Go 1.26.2 e concluiu com sucesso.

O job `SQLite runtime / native Linux ARM64 experiment` foi provisionado com `arch: arm64`, `LIMIAR_CI_MODE=sqlite-native-arm64` e `CGO_ENABLED=0` e passou executando `go test -v -count=1 ./internal/storage/sqlite` em runtime nativo Linux ARM64.

Os jobs independentes Linux X64/race, Linux X64/CGO-disabled e cross-build também permaneceram verdes no mesmo build, reduzindo a chance de o resultado ARM64 esconder regressão independente no harness compartilhado.

Essa execução constitui Evidence positiva para o boundary e ambiente efetivamente exercitados: `internal/storage/sqlite`, Linux ARM64 nativo, Noble, Go 1.26.2 e `CGO_ENABLED=0`. Porém o experimento permanece `In Progress` enquanto o HEAD final da PR não reproduzir um resultado verde com o harness estabilizado.

Um endurecimento posterior do harness adicionou diagnósticos de target, verificação explícita de `CGO_ENABLED=0` e `-timeout=5m`. O build Travis `278804127`, no HEAD `2b62e8615b6d89dcea904cc304143645ff70cf6c`, manteve verdes os três jobs independentes, mas falhou no job ARM64.

O build seguinte `278804510`, no HEAD `44d8d24c3e46d59a99854a5421726ce325ffe43a`, removeu apenas o timeout artificial e voltou a falhar somente no job ARM64. Isso falsificou a hipótese de que `-timeout=5m` explicava a regressão.

A tentativa seguinte substituiu a leitura direta da variável de shell pela verificação do modo efetivo do toolchain com `go env CGO_ENABLED`. O build Travis `278805201`, no HEAD `eddfb50b49daf74c400963ba7e0190555dde85aa`, ainda falhou exclusivamente no job ARM64, enquanto Linux X64/race, CGO-disabled e cross-build permaneceram verdes. Portanto essa alteração também não estabilizou o harness.

Como o HEAD comprovadamente verde `4c4dab9831cdc5702e0969bbb92b44dcb26026ba` já validava `GOOS` e `GOARCH` e executava a mesma suíte no mesmo tipo de runner, a próxima ação de menor escopo é restaurar exatamente esse trecho conhecido como funcional. Isso remove endurecimentos do harness que não produziram Evidence adicional confiável e evita atribuir ao storage uma falha que não foi vinculada ao SQLite.

Nenhuma das falhas posteriores é tratada como Evidence contra o storage enquanto não houver reprodução que associe a falha ao comportamento da suíte SQLite.

## Critério de promoção

O experimento pode ser promovido para `Supported` quando o HEAD final fechar verde preservando simultaneamente:

1. runner Travis Linux ARM64 nativo;
2. `go env GOOS=linux`;
3. `go env GOARCH=arm64`;
4. job configurado com `CGO_ENABLED=0`;
5. execução verde de `go test -v -count=1 ./internal/storage/sqlite`.

A execução verde anterior já demonstra que o storage pode operar nesse boundary; o gate pendente é obter novamente Evidence verde no HEAD que será integrado, sem ampliar o harness além do necessário para responder à pergunta do experimento.

## Limites

Este experimento não prova:

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

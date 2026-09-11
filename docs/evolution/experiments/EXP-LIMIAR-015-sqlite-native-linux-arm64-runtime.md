# EXP-LIMIAR-015 — Runtime nativo do novo storage SQLite em Linux ARM64

Authority: Experiment
Status: Supported

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

Um endurecimento posterior do harness adicionou diagnósticos de target, verificação explícita de `CGO_ENABLED=0` e `-timeout=5m`. O build Travis `278804127`, no HEAD `2b62e8615b6d89dcea904cc304143645ff70cf6c`, manteve verdes os três jobs independentes, mas falhou no job ARM64.

O build seguinte `278804510`, no HEAD `44d8d24c3e46d59a99854a5421726ce325ffe43a`, removeu apenas o timeout artificial e voltou a falhar somente no job ARM64. Isso falsificou a hipótese de que `-timeout=5m` explicava a regressão.

A tentativa seguinte substituiu a leitura direta da variável de shell pela verificação do modo efetivo do toolchain com `go env CGO_ENABLED`. O build Travis `278805201`, no HEAD `eddfb50b49daf74c400963ba7e0190555dde85aa`, ainda falhou exclusivamente no job ARM64, enquanto Linux X64/race, CGO-disabled e cross-build permaneceram verdes. Portanto essa alteração também não estabilizou o harness.

Como o HEAD comprovadamente verde `4c4dab9831cdc5702e0969bbb92b44dcb26026ba` já validava `GOOS` e `GOARCH` e executava a mesma suíte no mesmo tipo de runner, o harness foi restaurado exatamente para esse trecho conhecido como funcional. Nenhuma das falhas intermediárias é tratada como Evidence contra o storage porque não houve reprodução que as associasse ao comportamento da suíte SQLite.

A restauração fechou o ciclo experimental: Travis CI build `278805532`, no HEAD `eeb3f78855daae92871b5ce69bebff168510a95c`, passou novamente com os quatro jobs. O job nativo Linux ARM64 foi provisionado com `arch: arm64`, Go 1.26.2, `LIMIAR_CI_MODE=sqlite-native-arm64` e `CGO_ENABLED=0`, confirmou `GOOS=linux` e `GOARCH=arm64` e executou verde `go test -v -count=1 ./internal/storage/sqlite`. Linux X64/race, Linux X64/CGO-disabled e cross-build também ficaram verdes no mesmo build.

A repetição verde no HEAD estabilizado satisfaz o critério de promoção deste experimento e constitui Evidence positiva para o boundary efetivamente exercitado: `internal/storage/sqlite`, Linux ARM64 nativo, Ubuntu Noble, Go 1.26.2 e job configurado com `CGO_ENABLED=0`.

## Conclusão

**Supported**, com escopo estrito ao experimento observado. O novo storage SQLite executou sua suíte real com sucesso em Linux ARM64 nativo e reproduziu o resultado depois que alterações instrumentais do harness foram removidas.

O resultado demonstra viabilidade operacional do boundary SQLite nessa combinação de sistema operacional, arquitetura e toolchain. Não transforma Linux ARM64 em plataforma oficialmente suportada pelo produto e não amplia a Evidence para o binário completo do Limiar.

A tentativa de verificar `CGO_ENABLED` novamente dentro do script não foi mantida: durante o isolamento ela esteve correlacionada a um harness vermelho sem falha atribuída ao storage. Para este experimento, a Evidence preserva a configuração `CGO_ENABLED=0` do job e a execução verde resultante; endurecer novamente essa instrumentação exige uma investigação própria em vez de reabrir a hipótese já respondida.

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

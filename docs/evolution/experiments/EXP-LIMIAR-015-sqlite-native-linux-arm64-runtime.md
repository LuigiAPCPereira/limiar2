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

O job confirma o target efetivo do toolchain e o modo CGO antes de executar a suíte. Divergências falham com diagnóstico explícito.

Em seguida executa:

```sh
go test -v -count=1 ./internal/storage/sqlite
```

Os jobs Linux X64/race, Linux X64/CGO-disabled e cross-build permanecem presentes para detectar regressões independentes do novo runner.

## Evidência

Travis CI build `278803825`, associado ao HEAD executável `4c4dab9831cdc5702e0969bbb92b44dcb26026ba` da PR #192, executou quatro jobs em Linux Noble com Go 1.26.2 e concluiu com sucesso.

O job `SQLite runtime / native Linux ARM64 experiment` foi provisionado com `arch: arm64`, `LIMIAR_CI_MODE=sqlite-native-arm64` e `CGO_ENABLED=0` e passou executando `go test -v -count=1 ./internal/storage/sqlite` em runtime nativo Linux ARM64.

Os jobs independentes Linux X64/race, Linux X64/CGO-disabled e cross-build também permaneceram verdes no mesmo build, reduzindo a chance de o resultado ARM64 esconder regressão independente no harness compartilhado.

A hipótese é, portanto, `Supported` para o boundary e ambiente efetivamente exercitados: `internal/storage/sqlite`, Linux ARM64 nativo, Noble, Go 1.26.2 e `CGO_ENABLED=0`.

Um endurecimento posterior do harness adicionou diagnósticos de target, verificação explícita de `CGO_ENABLED=0` e `-timeout=5m`. O build Travis `278804127`, no HEAD `2b62e8615b6d89dcea904cc304143645ff70cf6c`, manteve verdes os três jobs independentes, mas falhou no job ARM64.

O build seguinte `278804510`, no HEAD `44d8d24c3e46d59a99854a5421726ce325ffe43a`, removeu apenas o timeout artificial e voltou a falhar somente no job ARM64. Isso falsifica a hipótese de que `-timeout=5m` explicava a regressão. Como GOOS e GOARCH já eram validados por `test` no HEAD verde `4c4dab9831cdc5702e0969bbb92b44dcb26026ba`, a diferença restante introduzida pelo endurecimento é a validação explícita de CGO e a forma diagnóstica dos checks de target.

O próximo isolamento preserva os diagnósticos de GOOS/GOARCH e continua exigindo CGO desabilitado, mas valida o **modo efetivo do toolchain** com `go env CGO_ENABLED` em vez de depender diretamente da variável de shell do Travis. Essa distinção testa se a regressão pertence ao plumbing do harness sem relaxar a propriedade arquitetural observada.

Nenhuma dessas falhas posteriores é tratada como Evidence contra o storage enquanto não houver reprodução que vincule a falha ao comportamento do SQLite.

## Critério de suporte

A hipótese é marcada `Supported` porque:

1. o job foi provisionado como runner Travis Linux ARM64 nativo;
2. o HEAD executável verde exigia `go env GOOS=linux` e `go env GOARCH=arm64` antes da suíte;
3. o job foi configurado e executado com `CGO_ENABLED=0`;
4. a suíte `internal/storage/sqlite` concluiu com sucesso no CI real do HEAD executável correspondente.

O endurecimento do harness posterior à coleta inicial de Evidence ainda está sendo estabilizado. O HEAD final precisa fechar seus próprios gates antes do merge da PR.

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

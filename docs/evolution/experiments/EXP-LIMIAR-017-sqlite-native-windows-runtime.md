# EXP-LIMIAR-017 — Runtime nativo do SQLite em Windows AMD64

Authority: Non-authoritative
Status: In Progress

## Hipótese

A suíte real de `internal/storage/sqlite` pode executar nativamente em um worker Windows AMD64 com Go 1.26.2 e `CGO_ENABLED=0`, preservando o comportamento já exercitado em Linux sem exigir mudança de schema, migration, runtime ou dependência.

## Origem

O EXP-LIMIAR-014 suportou somente cross-build para `windows/amd64`; compilação cruzada não prova comportamento de runtime.

O EXP-LIMIAR-015 fechou a mesma lacuna para Linux ARM64 por execução nativa. O EXP-LIMIAR-016 tentou obter Evidence equivalente em macOS, mas foi encerrado como `Inconclusive` porque o provider escolhido não ofereceu um worker executável para esse experimento.

A documentação atual do Travis descreve `os: windows`, Windows Server 1809, Git BASH como shell do build e Go como linguagem suportada. Portanto existe um harness executável plausível para reduzir a incerteza de Windows sem mudar produção.

## Harness

A matriz Travis adiciona um job isolado com `os: windows`, Go 1.26.2 e `CGO_ENABLED=0`. O boundary esperado exige `GOOS=windows`, `GOARCH=amd64` e executa `go test -v -count=1 ./internal/storage/sqlite`. O job não envia cobertura e não substitui os gates Linux existentes.

## Critérios de suporte

O experimento só pode mudar para `Supported` se, no mesmo HEAD:

1. um worker Windows real for provisionado;
2. `go env GOOS` retornar `windows`;
3. `go env GOARCH` retornar `amd64`;
4. o job estiver configurado com `CGO_ENABLED=0`;
5. `go test -v -count=1 ./internal/storage/sqlite` executar e passar;
6. os gates Linux existentes permanecerem verdes.

Falha de provisionamento ou de plumbing do provider deve ser separada de falha real do SQLite.

## Evidence observada

### Travis build 278807018 — HEAD `fffb28d51377d6efc5814e1213553d73605dd59e`

O provider materializou cinco jobs. Os quatro jobs Linux existentes passaram, incluindo o runtime nativo Linux ARM64. O job Windows foi provisionado e terminou `failed`.

O check agregado exposto ao GitHub não informa qual comando do job Windows falhou. Portanto esse resultado não sustenta atribuir a falha ao SQLite nem afirmar que a suíte chegou a executar.

A configuração desse HEAD ainda aplicava globalmente `addons: apt` com `gnupg`, embora esse addon pertença somente ao job Linux default que valida o binário Codecov.

### Travis build 278807191 — HEAD `eff189b332506bfa15c965b2904ddd05f907f65f`

O addon APT já estava isolado no job `Linux X64 / race`. Mesmo assim, os quatro jobs Linux passaram novamente e somente o job Windows falhou.

Esse resultado falsifica a hipótese de que o `addons: apt` global era a causa da falha Windows. Como o check agregado continua sem expor a etapa exata, a falha permanece atribuível ao lifecycle/harness até que a suíte SQLite seja demonstravelmente alcançada.

### Isolamento seguinte

No commit `935f5bbadc0ec94dde6b930022bf9bdbc6c3888c`, o job Windows passou a sobrescrever o lifecycle genérico:

- `install: skip` evita herdar o `go mod download` global;
- o próprio `script` Windows executa `go version`, confirma `GOOS` e `GOARCH`, baixa/verifica módulos e só então executa a suíte SQLite;
- o caminho Linux permanece inalterado.

A mudança reduz a superfície do harness sem alterar storage, schema, migrations, dependências ou comportamento de produção. O objetivo é separar uma falha anterior de lifecycle compartilhado de uma eventual falha real ao executar `internal/storage/sqlite` em Windows.

## Limites

Um resultado verde suporta somente o boundary observado: `internal/storage/sqlite`, Windows Server/ambiente efetivamente fornecido pelo Travis, AMD64, Go 1.26.2 e job configurado com `CGO_ENABLED=0`.

Não implica suporte oficial do produto completo a Windows, validação de CLI/collector/Telegram/dashboard, Windows ARM64, macOS, aceitação dos ADRs 021/022 ou mudança de default/arquitetura de produção.

## Resultado atual

`In Progress`.

Há Evidence repetida de provisionamento real do worker Windows e de estabilidade dos gates Linux, mas ainda não há Evidence de runtime SQLite bem-sucedido nem de falha atribuível ao storage.

## Próximo gate

Executar novamente a PR com o lifecycle Windows reduzido. Se o job passar, registrar a execução e promover este EXP para `Supported` no escopo acima. Se continuar falhando, usar o novo boundary reduzido para decidir se a falha ocorre antes da suíte; não adaptar código de produção sem Evidence de que `go test ./internal/storage/sqlite` foi alcançado e falhou.

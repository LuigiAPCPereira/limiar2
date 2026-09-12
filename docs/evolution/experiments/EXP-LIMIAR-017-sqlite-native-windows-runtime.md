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

A matriz Travis começou com um job isolado `os: windows`, Go 1.26.2 e `CGO_ENABLED=0`. O boundary esperado exige `GOOS=windows`, `GOARCH=amd64` e executa `go test -v -count=1 ./internal/storage/sqlite`. O job não envia cobertura e não substitui os gates Linux existentes.

Como o check agregado do Travis exposto ao GitHub identifica o estado de cada job, mas não a etapa interna que falhou, o harness foi posteriormente estratificado em três jobs Windows progressivos: `preflight`, `modules` e `runtime`. Cada fase repete as garantias anteriores e acrescenta somente o próximo boundary. Isso permite localizar a primeira fase falha sem depender de logs privados do provider.

## Critérios de suporte

O experimento só pode mudar para `Supported` se, no mesmo HEAD:

1. um worker Windows real for provisionado;
2. `go env GOOS` retornar `windows`;
3. `go env GOARCH` retornar `amd64`;
4. `go env CGO_ENABLED` retornar `0`;
5. módulos puderem ser baixados e verificados;
6. `go test -v -count=1 ./internal/storage/sqlite` executar e passar;
7. os gates Linux existentes permanecerem verdes.

Falha de provisionamento ou de plumbing do provider deve ser separada de falha real do SQLite.

## Evidence observada

### Travis build 278807018 — HEAD `fffb28d51377d6efc5814e1213553d73605dd59e`

O provider materializou cinco jobs. Os quatro jobs Linux existentes passaram, incluindo o runtime nativo Linux ARM64. O job Windows foi provisionado e terminou `failed`.

O check agregado exposto ao GitHub não informa qual comando do job Windows falhou. Portanto esse resultado não sustenta atribuir a falha ao SQLite nem afirmar que a suíte chegou a executar.

A configuração desse HEAD ainda aplicava globalmente `addons: apt` com `gnupg`, embora esse addon pertença somente ao job Linux default que valida o binário Codecov.

### Travis build 278807191 — HEAD `eff189b332506bfa15c965b2904ddd05f907f65f`

O addon APT já estava isolado no job `Linux X64 / race`. Mesmo assim, os quatro jobs Linux passaram novamente e somente o job Windows falhou.

Esse resultado falsifica a hipótese de que o `addons: apt` global era a causa da falha Windows. Como o check agregado continua sem expor a etapa exata, a falha permanece atribuível ao lifecycle/harness até que a suíte SQLite seja demonstravelmente alcançada.

### Travis build 278807471 — HEAD `f0702c77de1d76dcea09417b9b95bf67bedb3ad2`

O job Windows já sobrescrevia o lifecycle genérico com `install: skip` e um `script` próprio contendo somente `go version`, validações de plataforma, download/verificação de módulos e a suíte SQLite.

O resultado permaneceu idêntico: os quatro jobs Linux passaram e somente `SQLite runtime / native Windows AMD64 experiment` falhou. Isso falsifica a hipótese de que o `install` compartilhado ou os gates Linux herdados eram a causa. O check agregado ainda não informa qual comando dentro do job Windows falhou, portanto o resultado continua insuficiente para atribuir a falha ao SQLite.

## Isolamento atual

No commit `0933e39aad28018d17eae33a7a54aba83feaf786`, o único job Windows foi substituído por três fases progressivas e independentes:

- `SQLite Windows AMD64 / preflight`: executa `go version` e exige `GOOS=windows`, `GOARCH=amd64` e `CGO_ENABLED=0` efetivo;
- `SQLite Windows AMD64 / modules`: repete o preflight e acrescenta `go mod download` e `go mod verify`;
- `SQLite Windows AMD64 / runtime`: repete as duas fases anteriores e acrescenta `go test -v -count=1 ./internal/storage/sqlite`.

A intenção não é ampliar cobertura por quantidade de jobs, mas tornar observável pelo próprio status agregado qual é o primeiro boundary que falha. Se `preflight` falhar, a Evidence aponta para provisionamento/toolchain/configuração; se `preflight` passar e `modules` falhar, a incerteza fica em resolução/verificação de dependências; somente se `preflight` e `modules` passarem e `runtime` falhar haverá Evidence suficiente para investigar a suíte/storage em Windows.

Nenhuma fase altera storage, schema, migrations, dependências ou comportamento de produção.

## Limites

Um resultado verde suporta somente o boundary observado: `internal/storage/sqlite`, Windows Server/ambiente efetivamente fornecido pelo Travis, AMD64, Go 1.26.2 e `CGO_ENABLED=0` efetivo.

Não implica suporte oficial do produto completo a Windows, validação de CLI/collector/Telegram/dashboard, Windows ARM64, macOS, aceitação dos ADRs 021/022 ou mudança de default/arquitetura de produção.

## Resultado atual

`In Progress`.

Há Evidence repetida de provisionamento real do worker Windows e de estabilidade dos gates Linux. Três configurações sucessivas falharam somente no Windows, mas nenhuma delas tornou observável a etapa interna da falha. Por isso ainda não há Evidence de runtime SQLite bem-sucedido nem de falha atribuível ao storage.

## Próximo gate

Executar a PR com as três fases Windows progressivas. O primeiro job vermelho determina a próxima investigação. Não adaptar código de produção sem que `preflight` e `modules` estejam verdes e que a falha seja isolada no job `runtime` que efetivamente alcança `go test ./internal/storage/sqlite`.

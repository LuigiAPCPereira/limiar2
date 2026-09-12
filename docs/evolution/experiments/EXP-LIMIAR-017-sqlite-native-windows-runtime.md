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

Como o check agregado do Travis exposto ao GitHub identifica o estado de cada job, mas não a etapa interna que falhou, o harness foi posteriormente estratificado em fases progressivas. Cada fase repete as garantias anteriores e acrescenta somente o próximo boundary. Isso permite localizar a primeira fase falha sem depender de logs privados do provider.

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

### Travis build 278807576 — HEAD `7eafaadd6d4855170bbe5fd974fb977dc2b46c36`

A estratificação tornou o boundary finalmente observável:

- os quatro jobs Linux existentes passaram;
- `SQLite Windows AMD64 / preflight` passou;
- `SQLite Windows AMD64 / modules` passou;
- `SQLite Windows AMD64 / runtime` falhou.

Isso demonstra que o worker Windows real, Go 1.26.2, `GOOS=windows`, `GOARCH=amd64`, `CGO_ENABLED=0`, download e verificação de módulos não são o primeiro boundary falho. A falha está agora isolada na fase que acrescenta execução da suíte `internal/storage/sqlite`.

O resultado ainda não identifica qual teste ou qual operação SQLite falhou, portanto não justifica alterar produção por hipótese. A inspeção do código revela pontos potencialmente sensíveis à plataforma — em particular abertura de paths temporários Windows por URI `file:` e asserts de permissões POSIX nos testes — mas nenhum deles é tratado como causa sem execução que o isole.

### Travis build 278807630 — HEAD `352d25b1199819a19d7c3fe09ac4a7e9cd3e2571`

A segunda estratificação isolou o primeiro teste falho:

- os quatro jobs Linux existentes passaram;
- `SQLite Windows AMD64 / preflight` passou;
- `SQLite Windows AMD64 / modules` passou;
- `SQLite Windows AMD64 / pure test` passou;
- `SQLite Windows AMD64 / open baseline` falhou;
- `SQLite Windows AMD64 / runtime` falhou.

Logo, o test binary executa normalmente no worker e a primeira falha observável aparece no teste `TestOpenAppliesADR019BaselineAndMigration`, que chama `Open`, faz `os.Stat` no arquivo e então verifica permissões POSIX exatas (`0600`) antes dos demais asserts de baseline.

Esse resultado reduz o boundary, mas ainda não demonstra que `Open` em si falha. A documentação de `os.Chmod` especifica que, no Windows, somente o bit `0200` é usado para controlar o atributo read-only e os demais bits de permissão são ignorados. Portanto comparar `FileMode.Perm()` com `0600` não é uma garantia portátil equivalente à usada em Unix.

Essa diferença de semântica é Evidence para investigar o teste antes de alterar o storage. Ela não autoriza enfraquecer a garantia Unix nem declarar equivalência de segurança de ACLs Windows.

## Isolamento atual

No commit `ed14f2aa86f596c9a02d3e5a3c5cf1a427b8adea`, o harness acrescenta `SQLite Windows AMD64 / open functional`, executando `TestEvidenceAppendRoundTripAndHashAuthority` isoladamente.

Esse teste também passa por `Open`, bootstrap, WAL, migration e append real de Evidence, mas não contém o assert `Mode().Perm() == 0600`. Ele fica lado a lado com `open baseline` e `runtime` no mesmo HEAD.

A leitura do resultado é deliberadamente estreita:

- se `open functional` passar enquanto `open baseline` falhar, o assert POSIX do teste passa a ser a causa mais forte e uma correção de portabilidade do teste é justificável, preservando a garantia `0600` em Unix;
- se `open functional` também falhar, ainda existe uma falha funcional em `Open`/bootstrap/migration/append e nenhuma adaptação de teste deve mascará-la;
- somente depois que a suíte completa ficar verde o experimento pode ser promovido a `Supported`.

Nenhuma fase altera storage, schema, migrations, dependências ou comportamento de produção.

## Limites

Um resultado verde suporta somente o boundary observado: `internal/storage/sqlite`, Windows Server/ambiente efetivamente fornecido pelo Travis, AMD64, Go 1.26.2 e `CGO_ENABLED=0` efetivo.

Não implica suporte oficial do produto completo a Windows, validação de CLI/collector/Telegram/dashboard, Windows ARM64, macOS, aceitação dos ADRs 021/022 ou mudança de default/arquitetura de produção.

A semântica de `0600` usada como proteção Unix também não deve ser reinterpretada como uma garantia equivalente de ACL no Windows sem decisão e Evidence específicas.

## Resultado atual

`In Progress`.

Há Evidence de que provisionamento, toolchain/plataforma efetiva, resolução/verificação dos módulos e execução de lógica pura passam no worker Windows. O primeiro teste falho é `TestOpenAppliesADR019BaselineAndMigration`; ainda falta separar falha funcional de `Open` de incompatibilidade do assert POSIX de permissões.

## Próximo gate

Executar `open functional`, `open baseline` e `runtime` no mesmo HEAD. Se `open functional` passar e `open baseline` falhar, corrigir somente a expectativa de permissão do teste de forma específica por plataforma, mantendo `0600` como gate Unix. Se `open functional` falhar, subdividir o caminho funcional de `Open` antes de qualquer mudança de produção.

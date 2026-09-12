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

### Travis build 278807191 — HEAD `eff189b332506bfa15c965b2904ddd05f907f65f`

O addon APT já estava isolado no job `Linux X64 / race`. Mesmo assim, os quatro jobs Linux passaram novamente e somente o job Windows falhou. Isso falsificou a hipótese de que o `addons: apt` global era a causa da falha Windows.

### Travis build 278807471 — HEAD `f0702c77de1d76dcea09417b9b95bf67bedb3ad2`

O job Windows já sobrescrevia o lifecycle genérico com `install: skip` e um `script` próprio. O resultado permaneceu idêntico, falsificando a hipótese de que o `install` compartilhado ou gates Linux herdados eram a causa.

### Travis build 278807576 — HEAD `7eafaadd6d4855170bbe5fd974fb977dc2b46c36`

A estratificação tornou o boundary observável:

- quatro jobs Linux passaram;
- `preflight` passou;
- `modules` passou;
- `runtime` falhou.

Isso demonstrou que worker, Go, GOOS/GOARCH, CGO e módulos não eram o primeiro boundary falho.

### Travis build 278807630 — HEAD `352d25b1199819a19d7c3fe09ac4a7e9cd3e2571`

`pure test` passou, enquanto `open baseline` e a suíte completa falharam. O test binary portanto executava e a primeira falha aparecia ao atravessar `Open` ou seus asserts subsequentes.

### Travis build 278807734 — HEAD `43707ee8e052412cb97a3c89717b13de41cf2a05`

`open functional` também falhou, mesmo sem comparar permissões POSIX exatas. Isso falsificou a hipótese de que o assert de `0600` explicava a falha principal.

### Travis build 278807814

Abertura por filename Windows nativo passou, enquanto a URI `file:` mínima construída por `net/url`, a URI de produção e os testes que atravessam `Open` falharam. O boundary passou a ser a serialização de filename em URI.

### Travis build 278807939

A URI mínima hierárquica com separadores normalizados e `/C:/...` ainda falhou. Isso falsificou a hipótese de que apenas backslashes ou ausência do slash antes do drive explicavam a incompatibilidade.

### Travis build 278807989 — HEAD `a4d9a109e7c2b7e043c94a7c4001f19460574ec1`

O probe mínimo foi reduzido à forma documentada pelo upstream, `file:` + path Windows absoluto com separadores `/`, isto é, `file:C:/...`.

O resultado foi decisivo:

- os quatro jobs Linux existentes passaram;
- `preflight`, `modules` e `pure test` passaram;
- `native filename` passou;
- `minimal file URI` **passou**;
- `production URI` falhou;
- `open functional`, `open baseline` e a suíte completa falharam.

A diferença entre o probe verde e `databaseURI` ficou restrita à serialização do filename e aos parâmetros `_pragma`. Como o probe `file:C:/...` demonstra que o driver/VFS aceita o path absoluto Windows nessa forma, existe Evidence executável para alterar somente a serialização de drive-letter paths e então revalidar a URI de produção com os mesmos parâmetros.

### Travis build 278808046 — HEAD `e512292ba85d78145f76d252a453112afad1ee9f`

A correção mínima de serialização foi validada no runtime real:

- os quatro jobs Linux passaram;
- `preflight`, `modules`, `pure test`, `native filename` e `minimal file URI` passaram;
- `production URI` **passou**;
- `open functional` **passou**;
- `open baseline` falhou;
- a suíte Windows completa falhou.

Isso confirma que `databaseURI` deixou de ser o primeiro boundary incompatível. Como `open functional` atravessa `Open`, migrations e append real de Evidence sem comparar bits POSIX exatos, a falha restante de `open baseline` ficou concentrada em asserts posteriores à abertura.

O teste `TestOpenAppliesADR019BaselineAndMigration` exigia `info.Mode().Perm() == 0600` também no Windows. O mesmo arquivo exigia `0644` ao verificar que um SQLite não pertencente ao novo storage não havia sido mutado. Esses asserts expressam uma garantia POSIX válida para Unix, mas não uma garantia equivalente de ACL Windows. A correção de teste em `ec992652ed4c09f8cf2c93c1ee8c6729979d6fd2` mantém os asserts exatos fora de Windows e não altera `Open`, schema, migrations, PRAGMAs nem permissões de produção.

## Implementação em validação

O commit `06a08759e0542c0a9ae2159e671b9e3e601c1f64` altera somente `databaseURI`:

- preserva `url.Values` e exatamente os mesmos parâmetros `mode`/`_pragma`;
- para paths cujo volume é um drive letter (`C:` etc.), serializa `file:` + `filepath.ToSlash(path)`, produzindo `file:C:/...?...`;
- preserva a serialização anterior por `url.URL` em Unix e demais paths;
- não infere sem Evidence uma política para UNC paths.

O commit `ec992652ed4c09f8cf2c93c1ee8c6729979d6fd2` corrige somente a portabilidade dos asserts de teste de permissões: `0600` e `0644` continuam obrigatórios e verificados fora de Windows; o experimento não declara equivalência com ACLs Windows.

Nenhum schema, migration, PRAGMA, dependency, claim, WAL, baseline ou contrato de Evidence foi alterado.

## Limites

Um resultado verde suporta somente o boundary observado: `internal/storage/sqlite`, Windows Server/ambiente efetivamente fornecido pelo Travis, AMD64, Go 1.26.2 e `CGO_ENABLED=0` efetivo.

Não implica suporte oficial do produto completo a Windows, validação de CLI/collector/Telegram/dashboard, Windows ARM64, macOS, aceitação dos ADRs 021/022 ou mudança de default/arquitetura de produção.

A semântica de `0600` usada como proteção Unix também não deve ser reinterpretada como uma garantia equivalente de ACL no Windows sem decisão e Evidence específicas.

## Resultado atual

`In Progress`.

A serialização Windows de `databaseURI` já possui Evidence positiva em runtime real. A última falha conhecida da suíte está agora em validação após remover do caminho Windows somente asserts de bits POSIX que não representam uma garantia de ACL equivalente nessa plataforma.

## Próximo gate

No novo HEAD, exigir simultaneamente:

1. `production URI` verde;
2. `open functional` verde;
3. `open baseline` verde;
4. suíte completa `internal/storage/sqlite` verde;
5. gates Linux existentes verdes.

Somente se todos passarem no mesmo HEAD o EXP-LIMIAR-017 pode mudar para `Supported`. Se a suíte completa continuar vermelha, localizar o próximo teste falho antes de alterar produção.
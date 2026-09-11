# EXP-LIMIAR-017 — Runtime nativo do SQLite em Windows AMD64

Authority: Non-authoritative
Status: In Progress

## Hipótese

A suíte real de `internal/storage/sqlite` pode executar nativamente em um worker Windows AMD64 com Go 1.26.2 e `CGO_ENABLED=0`, preservando o comportamento já exercitado em Linux sem exigir mudança de schema, migration, runtime ou dependência.

## Origem

O EXP-LIMIAR-014 suportou somente cross-build para `windows/amd64`; compilação cruzada não prova comportamento de runtime.

O EXP-LIMIAR-015 fechou a mesma lacuna para Linux ARM64 por execução nativa. O EXP-LIMIAR-016 tentou obter Evidence equivalente em macOS, mas foi encerrado como `Inconclusive` porque o provider escolhido não oferece mais workers macOS.

A documentação atual do Travis descreve `os: windows`, Windows Server 1809, Git BASH como shell do build e Go como linguagem suportada. Portanto existe um harness executável plausível para reduzir a incerteza de Windows sem mudar produção.

## Harness

A matriz Travis adiciona um job isolado:

```yaml
- name: "SQLite runtime / native Windows AMD64 experiment"
  os: windows
  env: LIMIAR_CI_MODE=sqlite-native-windows CGO_ENABLED=0
```

O job confirma:

```sh
go env GOOS   == windows
go env GOARCH == amd64
```

E executa:

```sh
go test -v -count=1 ./internal/storage/sqlite
```

O job não envia cobertura e não substitui os gates Linux existentes.

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

O provider materializou cinco jobs. Os quatro jobs Linux existentes passaram, incluindo o runtime nativo Linux ARM64. O novo job `SQLite runtime / native Windows AMD64 experiment` foi provisionado como Windows e terminou `failed`.

O check agregado exposto ao GitHub não informa qual comando do job Windows falhou. Portanto esse resultado não sustenta atribuir a falha ao SQLite nem afirmar que a suíte chegou a executar.

A configuração desse HEAD ainda aplicava globalmente:

```yaml
addons:
  apt:
    packages:
      - gnupg
```

`gnupg` é necessário somente ao job Linux `default`, para verificação do binário Codecov no `after_success`. O addon APT não pertence ao boundary Windows e é uma diferença de lifecycle Linux compartilhada desnecessariamente com o experimento.

### Isolamento seguinte

No commit `e91bae9a6e790093ab7ef5ccc1f8ebb5763de554`, o addon APT foi movido para o job `Linux X64 / race`, sem alterar storage, schema, migrations, dependências ou o comando de teste Windows.

Esse ajuste testa uma hipótese de plumbing do CI: remover do worker Windows uma configuração Linux-only que não participa da hipótese experimental. Até o rerun fechar, a causa da falha original permanece desconhecida.

## Limites

Um resultado verde suporta somente o boundary observado:

- `internal/storage/sqlite`;
- Windows Server/ambiente efetivamente fornecido pelo Travis;
- AMD64;
- Go 1.26.2;
- job configurado com `CGO_ENABLED=0`.

Não implica:

- suporte oficial do produto completo a Windows;
- validação de CLI, collector, Telegram, dashboard ou lifecycle completo em Windows;
- suporte a Windows ARM64;
- generalização para macOS;
- aceitação dos ADRs 021/022;
- mudança de default ou arquitetura de produção.

## Resultado atual

`In Progress`.

Há Evidence de provisionamento real do worker Windows, mas ainda não há Evidence de runtime SQLite bem-sucedido nem de falha atribuível ao storage.

## Próximo gate

Executar novamente a PR após isolar o addon APT. Se o job Windows passar, registrar a execução e promover este EXP para `Supported` no escopo acima. Se continuar falhando, localizar a etapa exata do lifecycle antes de adaptar código de produção ou ampliar o harness.

# EXP-LIMIAR-017 — Runtime nativo do SQLite em Windows AMD64

Authority: Non-authoritative
Status: In Progress

## Hipótese

A suíte real de `internal/storage/sqlite` pode executar nativamente em um worker Windows AMD64 com Go 1.26.2 e `CGO_ENABLED=0`, preservando o comportamento já exercitado em Linux sem exigir mudança de schema, migration, runtime ou dependência.

## Origem

O EXP-LIMIAR-014 suportou somente cross-build para `windows/amd64`; compilação cruzada não prova comportamento de runtime.

O EXP-LIMIAR-015 fechou a mesma lacuna para Linux ARM64 por execução nativa. O EXP-LIMIAR-016 tentou obter Evidence equivalente em macOS, mas foi encerrado como `Inconclusive` porque o provider escolhido não oferece mais workers macOS.

A documentação atual do Travis ainda descreve `os: windows`, Windows Server 1809 e Go como linguagem suportada. Portanto existe um harness executável plausível para reduzir a incerteza de Windows sem mudar produção.

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

O harness foi materializado, mas nenhuma conclusão é válida antes de execução nativa observável no provider.

## Próximo gate

Executar a PR no Travis. Se o worker Windows iniciar e a suíte passar, registrar build/job e promover este EXP para `Supported` no escopo acima. Se falhar, separar primeiro incompatibilidade do provider/harness de falha do storage antes de adaptar qualquer código de produção.

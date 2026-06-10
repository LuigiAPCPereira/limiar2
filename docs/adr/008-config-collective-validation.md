# ADR 008 — Validação Coletiva de Configuração, Separada do Carregamento (Load)

## Status

Aceito

## Contexto

A configuração é lida de variáveis de ambiente prefixadas por `LIMIAR_`. Os operadores
se beneficiam de um feedback claro e completo: se vários valores estiverem faltando ou inválidos,
relatar apenas o primeiro erro força um ciclo frustrante de conserta-um-roda-de-novo. Nós também
queremos uma separação limpa entre "ler e aplicar padrões na config" e "decidir se
ela é utilizável", para que padrões (defaults) possam ser aplicados sem acoplamento com a validação,
e a validação possa ser executada explicitamente antes de qualquer operação de E/S.

## Decisão

Dividir a configuração em duas funções em `internal/config`:

- **`Load(v *viper.Viper)`** — faz o bind das chaves de ambiente `LIMIAR_`, extrai (unmarshals) para dentro de
  `Config`, e chama `ApplyDefaults`. Ela **não** valida.
- **`Validate()`** — verifica cada campo e retorna **todos** os problemas de uma vez,
  juntados com `errors.Join`, em vez de parar no primeiro.

`ApplyDefaults` preenche campos opcionais (`DBPath` `./limiar.db`, `LogLevel`
`info`, `LogFormat` `json`, `ShutdownTimeout` `15`, `MaxRetries` `10`,
`IOTimeout` `30s`, `DispatcherBufferSize` `256`, `DBWriterBufferSize` `512`).
Campos obrigatórios (`AppID`, `APIHash`) nunca recebem valores padrões. `Validate` impõe
intervalos (ex: `ShutdownTimeout` 1–300, `DispatcherBufferSize` 64–4096,
`DBWriterBufferSize` 128–8192) e enumerações (`LogLevel`, `LogFormat`).

O `run()` de `main.go` chama `config.Load` e em seguida `cfg.Validate()` antes de construir a
árvore de comandos — com abordagem fail-fast, antes de qualquer operação de E/S. `Config.String()` mascara `APIHash`.

## Consequências

- O operador visualiza todas as configurações erradas em uma única execução (Requisito 7.4/7.5).
- Atribuição de padrões e validação são independentes e testáveis separadamente
  (testes de propriedade cobrem "padrões aplicados", "rejeita inválidos", "coleta todos
  os erros" e "ocultação - masking").
- A validação deve ser chamada explicitamente; esquecê-la faria pular as checagens — portanto
  a raiz de composição (composition root) a chama antes de qualquer outra coisa.
- Segredos nunca vazam: o `APIHash` é mascarado pelo `String()` e redigido pelo
  logger.

## Alternativas consideradas

- **Validar dentro do Load** — acopla a aplicação de padrões à validação e dificulta
  uma lógica carregar-depois-inspecionar; rejeitada a favor do local de chamada fail-fast explícito.
- **Falhar no primeiro campo inválido** — péssima experiência para o operador; rejeitada a favor
  da agregação com `errors.Join`.
- **Biblioteca de validação baseada em Struct-tag** — uma dependência extra fora da stack
  fechada; verificações escritas manualmente mantêm o conjunto de dependências mínimo.

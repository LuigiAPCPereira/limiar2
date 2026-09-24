# Implementation Plan: Evolução do Sistema de Logging (logging-system-redesign)

## Overview

Este plano converte o design aprovado em passos incrementais de código sobre o pacote
compartilhado `internal/logger` (consumido por `cmd/limiar-collector` e
`cmd/limiar-processor`). A sequência prioriza a **correção de segurança crítica**
(`redactAttr` + reescrita do `PrettyHandler`) para que ela possa entrar cedo e de forma
independente, seguida pelo `Presenter`, pela correlação automática (`run_id`), pela
seleção de formato (`ResolveFormat`) com fiação na raiz de composição e migração dos
subcomandos da CLI, pela unificação do processor e, por fim, pelos testes e benchmarks
adicionais.

Restrições do `AGENTS.md` respeitadas em todas as tarefas: backend exclusivo `log/slog`;
nenhum import proibido (`zerolog`/`zap`/`logrus`/`sqlite`/`mattn`/`gorm`); construção
concreta de logger apenas em `internal/logger/slog.go` (`NewSlogLogger`); sem `init()` e
sem estado mutável global em `internal/`; interface `logger.Logger` com assinaturas
inalteradas; `pgregory.net/rapid` apenas em `_test.go`. Comentários de código e strings
visíveis ao usuário em pt-BR. Gates de qualidade ao final: `go build ./...`,
`go vet ./...`, `go test ./...`, `go test -race ./...`.

## Tasks

- [ ] 1. Estabelecer redação canônica e chaves de atributo estáveis
  - [ ] 1.1 Definir chaves de atributo estáveis e conjunto de chaves sensíveis
    - Criar `internal/logger/constants.go` com as constantes `attrKeyComponent`,
      `attrKeyRunID`, `attrKeyError` (valor `"erro"`, o nome pt-BR dominante no código
      existente), `attrKeyService`, `attrKeyPipelineStage`
    - Documentar (pt-BR) que `time`/`level` são geridos pelo slog e não têm constante
    - _Requirements: 5.2, 5.3, 10.2, 11.2_

  - [ ] 1.2 Implementar a função canônica `redactAttr`
    - Criar `internal/logger/redact.go` com `redactedKeys` (api_hash, apihash, session,
      token, password, auth_code), `redactedValue = "****"` e
      `redactAttr(groups []string, a slog.Attr) slog.Attr`
    - Comparação de chave **case-insensitive** via `strings.ToLower(a.Key)`; substituir o
      valor inteiro por `****` para qualquer tipo escalar, preservando a chave original;
      retornar atributos não sensíveis sem alteração
    - Remover/aposentar a antiga `redactSensitive` (substituída por `redactAttr`)
    - _Requirements: 1.1, 1.2, 1.3, 1.4, 1.5, 1.6, 1.7, 1.8_

  - [ ]* 1.3 Escrever teste de propriedade para redação universal
    - **Property 1: Redação universal de chaves sensíveis**
    - **Validates: Requirements 1.1, 1.2, 1.3, 1.4, 1.6, 1.7**
    - Em `internal/logger/redact_test.go` com `pgregory.net/rapid`, ≥100 iterações;
      gerar chave sensível em caixa variada, tipo escalar variado e profundidade de grupo
      variada; afirmar presença de `****` e ausência do valor original e de qualquer
      substring dele

  - [ ]* 1.4 Escrever teste de propriedade para preservação de chave e transparência
    - **Property 2: Preservação de chave e transparência de não sensíveis**
    - **Validates: Requirements 1.5, 1.8**
    - Em `internal/logger/redact_test.go`; afirmar chave preservada e valor de chave não
      sensível inalterado

- [ ] 2. Reescrever o PrettyHandler e integrar a redação aos formatos JSON/Text
  - [ ] 2.1 Reescrever `PrettyHandler` (correção de segurança + performance + concorrência)
    - Reescrever `internal/logger/pretty.go`: invocar `redactAttr(h.groups, a)`
      explicitamente em `Handle` e em `WithAttrs` (corrige a lacuna de redação do pretty,
      Req 1.1); `mu *sync.Mutex` compartilhado entre handlers derivados serializando uma
      **única** `Write` por registro; `bytes.Buffer` local (sem `sync.Pool`); formatação
      escalar **sem reflexão** (`strconv.AppendInt`, `.String()`, etc.); grupos em
      **flat dotted notation** (`WithGroup("auth")` + chave `api_hash` → `auth.api_hash`);
      `fieldWidth = 14` para alinhamento de coluna; usar `utf8.RuneCountInString` para
      padding rune-aware (acomodar chaves pt-BR com caracteres multibyte)
    - _Requirements: 1.1, 4.2, 6.2, 6.3, 7.1_

  - [ ] 2.2 Integrar `redactAttr` aos handlers JSON e Text via `ReplaceAttr`
    - Em `internal/logger/slog.go`, definir `opts.ReplaceAttr = redactAttr` para
      `slog.NewJSONHandler` e `slog.NewTextHandler`; remover do `NewSlogLogger` o
      `With("service", ...)` fixo (a identidade passa a ser injetada na raiz de
      composição na tarefa 6.1); manter `NewSlogLogger` como único ponto de construção
    - _Requirements: 1.2, 1.3, 8.1_

  - [ ]* 2.3 Escrever teste de tabela para descarte por nível
    - **Property 5: Descarte por nível sem formatação** [TABELA]
    - **Validates: Requirements 4.4, 6.1**
    - Em `internal/logger/pretty_test.go`; teste de tabela (4 níveis × 4 configs = 16
      casos); usar um `slog.LogValuer` instrumentado para provar que valores não são
      formatados quando o nível está abaixo do configurado

  - [ ]* 2.4 Escrever teste de propriedade para escrita única e saída contígua
    - **Property 7: Escrita única e saída contígua sob concorrência**
    - **Validates: Requirements 6.3, 6.5, 7.1**
    - Em `internal/logger/pretty_test.go`; ≥8 goroutines; writer que conta `Write` e
      verifica contiguidade/integridade de cada registro

  - [ ]* 2.5 Escrever testes de tabela do PrettyHandler
    - Em `internal/logger/pretty_test.go`; cobrir alinhamento `fieldWidth=14` (rune-aware),
      flat dotted notation de grupos e formatação de cada tipo escalar suportado
    - _Requirements: 4.2, 6.2_

  - [ ]* 2.6 Escrever teste de concorrência executável sob `-race`
    - **Validates: Requirements 7.1, 7.4**
    - Em `internal/logger/pretty_race_test.go`; ≥8 goroutines emitindo concorrentemente
      pelo mesmo logger; deve passar em `go test -race ./internal/logger`

  - [ ]* 2.7 Escrever benchmarks dos caminhos Pretty e JSON
    - **Validates: Requirements 6.4**
    - Em `internal/logger/bench_test.go`: `BenchmarkPrettyHandler` e
      `BenchmarkJSONHandler`, executáveis via `go test -bench=. -benchmem`, reportando
      allocs/op e B/op por Evento_de_Log

  - [ ]* 2.8 Escrever baseline de alocações do Pretty
    - **Validates: Requirements 6.2, 6.3**
    - Em `internal/logger/perf_test.go`: `TestPrettyAllocsBaseline` com
      `testing.AllocsPerRun` e teto constante

- [ ] 3. Checkpoint - Garantir que os testes da redação e do PrettyHandler passam
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 4. Introduzir a interface Presenter e suas implementações
  - [ ] 4.1 Definir a interface `Presenter`, `MessageType` e mapeamentos internos
    - Criar `internal/logger/presenter.go` com a interface `Presenter`
      (`Info`/`Success`/`Warning`/`Error`/`Step`), o tipo `MessageType` (MsgInfo,
      MsgSuccess, MsgWarning, MsgError, MsgStep) e os arrays internos `prefixes` e
      `colors` (emojis restritos ao `AGENTS.md`) mais `resetCode` — sem tipo `Theme`
    - _Requirements: 2.1, 3.1, 3.3, 3.7, 8.7_

  - [ ] 4.2 Implementar `TerminalPresenter` e helpers de terminal
    - Em `internal/logger/presenter.go`, implementar `NewTerminalPresenter(w io.Writer,
      useColors bool)` e o método `emit` com **uma única** `Write` por mensagem (sem
      mutex), supressão de ANSI quando `useColors=false` preservando prefixo e texto, e
      linha única terminada por exatamente um `\n`; exportar `IsTerminalWriter(w)` e
      `NoColorEnvSet()` para uso na raiz de composição
    - Asserção de compilação `var _ Presenter = (*TerminalPresenter)(nil)`
    - _Requirements: 2.3, 2.5, 3.2, 3.4, 3.5, 3.6, 4.3_

  - [ ] 4.3 Implementar `NopPresenter`
    - Em `internal/logger/nop.go`, adicionar `NopPresenter` com todos os métodos no-op e
      asserção `var _ Presenter = NopPresenter{}`
    - _Requirements: 2.1, 8.7_

  - [ ]* 4.4 Escrever teste de tabela para injetividade de prefixos
    - **Property 3: Injetividade de prefixos do TerminalPresenter** [TABELA]
    - **Validates: Requirements 3.1, 3.3, 3.6, 3.7**
    - Em `internal/logger/presenter_test.go`; teste de tabela (5 tipos, espaço finito);
      afirmar prefixos distintos por par de `MessageType`, pertencentes ao conjunto do
      `AGENTS.md`, no máximo um por mensagem

  - [ ]* 4.5 Escrever teste de tabela para supressão de ANSI
    - **Property 4: Supressão de ANSI preservando prefixo e texto** [TABELA]
    - **Validates: Requirements 3.4, 3.5**
    - Em `internal/logger/presenter_test.go`; teste de tabela (5 tipos × cor on/off);
      para cada caso, ausência de `\x1b` quando `useColors=false` e presença de prefixo
      e texto completos

  - [ ]* 4.6 Escrever testes de tabela do TerminalPresenter
    - Em `internal/logger/presenter_test.go`; cada método emite o prefixo correto;
      `NO_COLOR` suprime ANSI; saída em linha única
    - _Requirements: 2.2, 3.2_

- [ ] 5. Implementar o atributo de erro estável
  - [ ] 5.1 Implementar `errorAttr`
    - Criar `internal/logger/error.go` com `errorAttr(err error) (slog.Attr, bool)` sob a
      chave estável `erro` (o nome pt-BR dominante no código existente, garantindo
      correlação cross-stage sem transformação); retornar `ok=false` quando `err == nil`;
      preservar o texto completo da cadeia `"layer: op: cause"` sem truncar
    - _Requirements: 10.1, 10.2, 10.3_

  - [ ]* 5.2 Escrever teste de propriedade para preservação da cadeia de erro
    - **Property 10: Preservação da cadeia de erro** [PBT]
    - **Validates: Requirements 10.1, 10.2**
    - Em `internal/logger/error_test.go` com `pgregory.net/rapid`; cadeias via
      `errors.Wrap` com profundidade variável; afirmar chave `erro` e texto completo sem
      truncamento

  - [ ]* 5.3 Escrever teste unitário para erro nulo
    - Em `internal/logger/error_test.go`; `errorAttr(nil)` retorna `ok=false`
    - _Requirements: 10.3_

- [ ] 6. Gerar e vincular correlação automática (run_id) no collector
  - [ ] 6.1 Implementar `generateRunID` e fiar identidade no `Logger()` do collector
    - Em `cmd/limiar-collector/main.go`, adicionar `generateRunID()` via `crypto/rand`
      (8 bytes → 16 chars hex, sem dependência nova) e, em `provider.Logger`, vincular via
      `With` os atributos `run_id`, `service=limiar-collector`,
      `pipeline_stage=collector`
    - _Requirements: 5.1, 5.3, 11.2_

  - [ ]* 6.2 Escrever teste de propriedade para derivação e correlação automática
    - **Property 6: Derivação acumula atributos e correlação é automática**
    - **Validates: Requirements 5.1, 5.2, 5.4, 7.2**
    - Em `internal/logger/slog_test.go`; afirmar que toda derivação contém os pares
      acumulados (incluindo `run_id` e `component`) e que o pai permanece inalterado

  - [ ]* 6.3 Escrever teste de propriedade para derivação imutável sob concorrência
    - **Property 8: Derivação imutável e independente sob concorrência**
    - **Validates: Requirements 7.3**
    - Em `internal/logger/slog_test.go`; ≥8 goroutines derivando do mesmo pai; cada
      derivado contém apenas seus atributos + herdados, sem corromper pai nem irmãos

- [ ] 7. Implementar a seleção de formato por subcomando
  - [ ] 7.1 Implementar `ResolveFormat`
    - Criar `internal/logger/resolve.go` com `ResolveFormat(subcommand, envFormat string,
      logIsTTY bool) (format string, warnInvalid bool)`: env válido tem precedência; env
      inválido → `text` + `warnInvalid=true`; ausente + `run` → `json`; ausente +
      (`auth`|`channels`|`dashboard`) → `pretty` se TTY senão `text`
    - _Requirements: 9.1, 9.2, 9.3, 9.4, 9.5, 9.6_

  - [ ]* 7.2 Escrever teste de tabela para precedência e fallback de formato
    - **Property 9: Precedência de formato e fallback** [TABELA]
    - **Validates: Requirements 9.5, 9.6**
    - Em `internal/logger/resolve_test.go`; teste de tabela (~20 casos enumeráveis);
      formato válido retornado; string inválida → `text` + `warnInvalid=true`

  - [ ]* 7.3 Escrever testes de tabela do `ResolveFormat`
    - Em `internal/logger/resolve_test.go`; tabela `(subcmd, env, isTTY) → formato`
      cobrindo run/auth/channels/dashboard com e sem TTY
    - _Requirements: 9.1, 9.2, 9.3, 9.4_

- [ ] 8. Checkpoint - Garantir que Presenter, errorAttr, run_id e ResolveFormat passam
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 9. Atualizar a raiz de composição do collector
  - [ ] 9.1 Adicionar `Presenter()` à interface `cli.Provider`
    - Em `internal/cli/root.go`, acrescentar o método `Presenter() logger.Presenter` à
      interface `Provider` (a CLI passa a depender da interface `Presenter`)
    - _Requirements: 2.5, 8.7_

  - [ ] 9.2 Implementar `Presenter()` e fiar `ResolveFormat` no collector
    - Em `cmd/limiar-collector/main.go`, implementar `provider.Presenter()` usando
      `NewTerminalPresenter(os.Stderr, IsTerminalWriter(os.Stderr) && !NoColorEnvSet())`;
      usar `ResolveFormat` (com detecção de TTY do stream de log) para escolher o formato
      e emitir um Evento_de_Log Warn quando `warnInvalid` for verdadeiro
    - _Requirements: 2.3, 2.5, 9.5, 9.6_

- [ ] 10. Migrar os subcomandos da CLI de `fmt.Print*` para o Presenter
  - [ ] 10.1 Migrar `auth`
    - Em `internal/cli/auth.go`, substituir `fmt.Print*` por chamadas ao `Presenter`
      injetado; remover Eventos_de_Log Info redundantes com Mensagens_de_Apresentação
    - _Requirements: 2.2, 2.4, 4.3_

  - [ ] 10.2 Migrar `channels`
    - Em `internal/cli/channels.go`, substituir `fmt.Print*` pelo `Presenter`
    - _Requirements: 2.2, 2.4_

  - [ ] 10.3 Migrar `run`
    - Em `internal/cli/run.go`, substituir `fmt.Print*` de apresentação pelo `Presenter`
    - _Requirements: 2.2, 2.4_

  - [ ] 10.4 Migrar `dashboard` (passa a seguir detecção de TTY)
    - Em `internal/cli/dashboard.go`, substituir `fmt.Print*` pelo `Presenter` e garantir
      que o formato de log siga a detecção de TTY (pretty quando TTY, senão text)
    - _Requirements: 2.2, 2.4, 9.4_

  - [ ]* 10.5 Escrever testes de separação de streams e uso do Presenter
    - Em `internal/cli/cli_test.go`; com dois `bytes.Buffer`, confirmar Evento_de_Log em
      stdout e Mensagem_de_Apresentação em stderr; verificar que os subcomandos usam
      `Presenter` em vez de `fmt.Print*`
    - _Requirements: 2.2, 2.3_

- [ ] 11. Checkpoint - Garantir que a CLI migrada compila e os testes passam
  - Ensure all tests pass, ask the user if questions arise.

- [ ] 12. Unificar o processor com o pacote compartilhado
  - [ ] 12.1 Fiar identidade e correlação cross-stage no processor
    - Em `cmd/limiar-processor/main.go`, vincular via `With` `run_id` (mesmo
      `generateRunID`), `service=limiar-processor`, `pipeline_stage=processor`; vincular
      `canal_id`/`msg_id` via `With` no caminho de processamento de mensagem (nomes
      idênticos aos usados pelo collector para correlação sem transformação);
      substituir os `fmt.Fprint*` por `NopPresenter`/logger conforme apropriado
    - _Requirements: 5.6, 11.1, 11.2, 11.3, 11.5_

  - [ ]* 12.2 Escrever teste de unificação de atributos padronizados
    - Em `internal/logger/unification_test.go`; afirmar que collector e processor
      produzem JSON com `service`, `component`, `run_id` e `pipeline_stage` e aplicam a
      mesma política de redação
    - _Requirements: 11.2, 11.4_

- [ ] 13. Checkpoint final - Gates de qualidade
  - Ensure all tests pass, ask the user if questions arise.
  - Executar `go build ./...`, `go vet ./...`, `go test ./...`, `go test -race ./...`;
    confirmar ausência de imports proibidos (zerolog/zap/logrus/sqlite/mattn/gorm) e
    ausência de `func init(` / estado mutável global em `internal/`

## Notes

- Tarefas marcadas com `*` são opcionais (testes/benchmarks) e podem ser puladas para um
  MVP mais rápido; o código não-teste nunca é marcado como opcional.
- A correção de segurança (redação) está nas tarefas 1–2 para entrar cedo e de forma
  independente do restante.
- PBT é usado apenas para P1, P2, P6, P7, P8, P10 (espaço de entrada combinatório).
- P3, P4, P5, P9 são testes de tabela (espaço finito e pequeno).
- Chaves de atributo de domínio seguem o padrão pt-BR dominante no código: `canal_id`,
  `msg_id`, `erro`. Chaves de infra/correlação permanecem em inglês: `run_id`,
  `component`, `service`, `pipeline_stage`.
- Benchmarks (`BenchmarkPrettyHandler`/`BenchmarkJSONHandler`) e `TestPrettyAllocsBaseline`
  atendem ao Req 6.4 e residem apenas em `_test.go`.
- Comentários e strings visíveis ao usuário em pt-BR; mensagens de erro no formato
  `"layer: op: cause"`.

## Task Dependency Graph

```json
{
  "waves": [
    { "id": 0, "tasks": ["1.1", "1.2", "4.1", "4.3", "5.1", "7.1"] },
    { "id": 1, "tasks": ["2.1", "2.2", "4.2", "5.2", "7.2", "1.3"] },
    { "id": 2, "tasks": ["6.1", "9.1", "5.3", "7.3", "1.4", "4.4"] },
    { "id": 3, "tasks": ["9.2", "2.3", "4.5", "6.2", "12.1"] },
    { "id": 4, "tasks": ["10.1", "10.2", "10.3", "10.4", "2.4", "4.6", "6.3", "12.2"] },
    { "id": 5, "tasks": ["2.5", "2.6", "2.7", "2.8", "10.5"] }
  ]
}
```

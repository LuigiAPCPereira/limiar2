# Requirements Document

## Introduction

Esta especificação cobre a **evolução profissional do sistema de logging** do ecossistema Limiar
(pacote `internal/logger`), compartilhado pelo `limiar-collector` e pelo `limiar-processor`. Não se
trata de um logger greenfield: é uma reengenharia crítica de um sistema já em produção, cujo
objetivo é elevá-lo ao padrão de ferramentas de CLI modernas (Docker, Terraform, GitHub CLI, Vercel
CLI, pnpm, Bun, Stripe CLI), mantendo intactas as restrições rígidas do `AGENTS.md`.

A evolução ataca cinco eixos identificados na auditoria do código atual:

1. **Arquitetura** — acoplamento, coesão, duplicação, fronteiras de responsabilidade,
   modularidade e extensibilidade do pacote `internal/logger` e de seus pontos de uso.
2. **DX/UX do terminal** — hierarquia visual, escaneabilidade, consistência entre todos os
   subcomandos e contextos, minimalismo e eliminação de ruído (cores/ícones em excesso,
   mensagens redundantes), diferenciação clara entre os tipos de mensagem.
3. **Observabilidade** — contexto, correlação entre eventos e rastreabilidade para troubleshooting,
   inclusive entre collector e processor como partes de um pipeline unificado.
4. **Performance** — minimização de alocações e formatação supérflua, eficiência de I/O e de
   concorrência sob carga contínua de milhões de eventos.
5. **Manutenibilidade** — clareza, evolução de longo prazo e consistência entre binários.

A auditoria do estado atual identificou problemas concretos que estes requisitos endereçam:

- **Lacuna de redação no `PrettyHandler`.** A função `redactSensitive` é aplicada via
  `slog.HandlerOptions.ReplaceAttr`, que os handlers JSON e Text honram, mas o `PrettyHandler`
  customizado **não invoca** `ReplaceAttr`. Como `pretty` é o formato padrão e o formato dos
  subcomandos interativos `auth` e `channels` (que manipulam `api_hash`, `session`, `auth_code`),
  valores sensíveis podem vazar na saída pretty.
- **Dois canais de saída paralelos sem fronteira.** A apresentação ao usuário usa `fmt.Print*`
  diretamente (ex.: `auth.go`, `channels.go`, `run.go`, `dashboard.go`, processor `main.go`)
  enquanto eventos observáveis usam `log.*`. Não há separação formal entre "saída de apresentação
  ao usuário" e "log diagnóstico", o que gera inconsistência de hierarquia visual e duplicação.
- **Ícones e cores acoplados ao ponto de chamada.** Prefixos de emoji são embutidos manualmente
  em cada string de mensagem e cores ANSI ficam codificadas no `PrettyHandler`, sem semântica
  centralizada nem controle de quando suprimi-los (`NO_COLOR`, não-TTY, nível de verbosidade).
- **Alocações por registro no caminho quente.** `PrettyHandler.Handle` aloca um `strings.Builder`,
  um slice de atributos e múltiplas strings intermediárias por evento; `formatValue` usa
  `fmt.Sprintf`. Sob milhões de eventos no `run`, isso pressiona o GC.
- **Observabilidade limitada.** Não há identidade de correlação (ex.: `run_id`) que ligue os
  eventos de um mesmo ciclo de coleta, nem atributos padronizados que permitam rastrear uma
  mensagem do collector ao processor.
- **Toda saída vai para `os.Stdout`.** Apresentação, logs diagnósticos e erros compartilham o
  mesmo stream, impossibilitando redirecionamento limpo (ex.: JSON em stdout, progresso em stderr).
- **Inconsistência entre binários.** Collector e processor usam o mesmo pacote `internal/logger`
  mas com convenções divergentes de atributos, formatos e correlação.

## Glossary

- **Logging_System**: O subsistema completo de logging do ecossistema Limiar, composto pela
  interface `logger.Logger`, a interface `logger.Presenter`, e suas implementações no pacote
  `internal/logger`, compartilhado por collector e processor.
- **Logger**: A interface `logger.Logger` (Debug/Info/Warn/Error/With/WithComponent) injetada em
  todas as camadas via construtor.
- **Presenter**: A interface `logger.Presenter` (Info/Success/Warning/Error/Step) para emissão de
  Mensagens_de_Apresentação, separada do Logger diagnóstico.
- **TerminalPresenter**: A implementação concreta de `Presenter` que escreve em um `io.Writer`
  com prefixos de emoji e supressão condicional de cores ANSI.
- **SlogLogger**: A implementação concreta de `Logger` que envolve `log/slog`, instanciada
  exclusivamente em `internal/logger/slog.go` por `NewSlogLogger`.
- **PrettyHandler**: O `slog.Handler` customizado que produz saída colorida legível por humanos.
- **JSONHandler**: O `slog.Handler` da stdlib que produz uma linha JSON por evento.
- **TextHandler**: O `slog.Handler` da stdlib que produz saída `key=value` em texto simples.
- **Handler**: Qualquer implementação de `slog.Handler` usada pelo `SlogLogger`.
- **Formato_Pretty / Formato_Text / Formato_JSON**: Os três formatos de saída suportados
  (`pretty`, `text`, `json`), selecionados por `LIMIAR_LOG_FORMAT` e pelo subcomando.
- **Redaction_Function**: A lógica que substitui valores de chaves sensíveis por um marcador fixo.
- **Chave_Sensível**: Uma das chaves cujo valor nunca pode aparecer na saída: `api_hash`,
  `apihash`, `session`, `token`, `password`, `auth_code`.
- **Mensagem_de_Apresentação**: Saída destinada diretamente ao operador humano da CLI (resultado
  de comando, prompts, confirmações), distinta de um evento de log diagnóstico.
- **Evento_de_Log**: Um registro diagnóstico emitido por uma camada via `Logger`, com nível,
  mensagem e atributos chave/valor.
- **Tipo_de_Mensagem**: A classificação semântica de uma mensagem visível: info, sucesso, aviso,
  erro ou etapa-de-processamento.
- **Nível_de_Log**: Um dos níveis do `slog` (Debug, Info, Warn, Error) controlado por
  `LIMIAR_LOG_LEVEL`.
- **Correlation_ID**: Um identificador estável gerado automaticamente na raiz de composição,
  vinculado ao Logger via `With`, que liga Eventos_de_Log de uma mesma execução (chave `run_id`).
- **Componente**: O nome de subsistema vinculado a um Logger via `WithComponent` (ex.: `telegram`,
  `collector`, `dispatcher`, `processor`).
- **TTY**: Um stream de saída associado a um terminal interativo, detectado via
  `golang.org/x/term`.
- **Subcomando**: Um dos comandos da CLI do collector: `auth`, `channels`, `run`, `dashboard`.
- **Pipeline_Stage**: Atributo que identifica a etapa do pipeline Limiar (`collector` ou
  `processor`) em logs de produção (JSON).

## Requirements

### Requirement 1: Redação de dados sensíveis em todos os formatos

**User Story:** Como engenheiro de segurança, quero que valores sensíveis sejam mascarados em
qualquer formato de saída, para que credenciais nunca vazem em logs ou no terminal.

#### Acceptance Criteria

1. WHERE o Formato_Pretty está em uso, THE Logging_System SHALL aplicar a Redaction_Function a
   cada atributo, incluindo atributos aninhados em grupos, antes de escrevê-lo na saída.
2. WHERE o Formato_Text está em uso, THE Logging_System SHALL aplicar a Redaction_Function a cada
   atributo, incluindo atributos aninhados em grupos, antes de escrevê-lo na saída.
3. WHERE o Formato_JSON está em uso, THE Logging_System SHALL aplicar a Redaction_Function a cada
   atributo, incluindo atributos aninhados em grupos, antes de escrevê-lo na saída.
4. WHEN um atributo cuja chave corresponde a uma Chave_Sensível por comparação sem distinção de
   maiúsculas/minúsculas (case-insensitive) é registrado, THE Logging_System SHALL substituir o
   valor completo pelo marcador fixo de 4 caracteres `****` na saída, independentemente do tipo do
   valor (string, inteiro, float, booleano, duração, tempo).
5. WHEN um atributo cuja chave é uma Chave_Sensível é registrado, THE Logging_System SHALL
   preservar a chave original na saída.
6. FOR ALL valores de Chave_Sensível em qualquer Nível_de_Log e em qualquer formato, a saída
   produzida SHALL omitir o valor original literal e qualquer substring ou prefixo dele
   (propriedade de redação).
7. WHEN uma Chave_Sensível aparece em qualquer profundidade de aninhamento de grupos, THE
   Logging_System SHALL redigir seu valor com o mesmo marcador `****`.
8. WHEN um atributo cuja chave não é uma Chave_Sensível é registrado, THE Logging_System SHALL
   emitir seu valor sem alteração.

### Requirement 2: Separação entre apresentação ao usuário e log diagnóstico

**User Story:** Como engenheiro, quero uma fronteira formal entre saída de apresentação e log
diagnóstico, para que cada uma tenha hierarquia, destino e formatação consistentes sem duplicação.

#### Acceptance Criteria

1. THE Logging_System SHALL expor uma interface `Presenter` com métodos
   `Info`/`Success`/`Warning`/`Error`/`Step`, separada dos métodos
   `Debug`/`Info`/`Warn`/`Error` da interface `Logger` usados para emitir Eventos_de_Log.
2. WHEN uma camada de Subcomando precisa exibir um resultado de comando, prompt ou confirmação ao
   operador, THE Subcomando SHALL emitir uma Mensagem_de_Apresentação através da interface
   `Presenter` e SHALL NOT invocar `fmt.Print`/`fmt.Println`/`fmt.Fprint`/`fmt.Fprintln`
   diretamente para essa saída.
3. THE Logging_System SHALL, por padrão, escrever Eventos_de_Log em `os.Stdout` e
   Mensagens_de_Apresentação em `os.Stderr`, de modo que os dois sejam streams distintos e
   redirecionáveis de forma independente.
4. WHERE o mesmo fato já foi exibido como Mensagem_de_Apresentação dentro do mesmo Subcomando, THE
   Subcomando SHALL NOT emitir um Evento_de_Log adicional de nível Info descrevendo esse mesmo fato.
5. THE Logging_System SHALL receber o writer de Mensagens_de_Apresentação via injeção na
   construção do `TerminalPresenter`, mantendo a instanciação concreta restrita à raiz de
   composição.

### Requirement 3: Prefixos e cores centralizados

**User Story:** Como mantenedor, quero que ícones e cores sejam definidos em um único lugar
semântico, para eliminar acoplamento no ponto de chamada e ruído visual inconsistente.

#### Acceptance Criteria

1. THE `TerminalPresenter` SHALL definir internamente um mapeamento fixo que associa cada um dos
   cinco Tipos_de_Mensagem a exatamente um prefixo e exatamente um estilo, sem prefixos
   compartilhados entre tipos.
2. WHEN uma Mensagem_de_Apresentação de um Tipo_de_Mensagem é emitida, THE `TerminalPresenter`
   SHALL obter o prefixo e o estilo do mapeamento interno em vez de literais embutidos no ponto de
   chamada.
3. THE mapeamento SHALL diferenciar visualmente os cinco Tipos_de_Mensagem (info, sucesso, aviso,
   erro e etapa-de-processamento) atribuindo a cada um um prefixo distinto e, quando os códigos
   ANSI não estão suprimidos, uma cor distinta.
4. WHERE a saída não é um TTY, THE `TerminalPresenter` SHALL suprimir todas as sequências de
   escape ANSI preservando o prefixo de tipo e o texto completo da mensagem sem sequências de
   escape.
5. IF a variável de ambiente `NO_COLOR` está definida (presente com qualquer valor, inclusive
   vazio), THEN THE `TerminalPresenter` SHALL suprimir todas as sequências de escape ANSI na saída.
6. WHILE os códigos ANSI estão suprimidos, THE `TerminalPresenter` SHALL manter os cinco
   Tipos_de_Mensagem distinguíveis por seus prefixos distintos.
7. THE mapeamento SHALL restringir o conjunto de prefixos de emoji aos definidos no `AGENTS.md`
   (📡 📩 📜 🔄 ❌ ✅ 🛑 ⏰ 🌐 ⚠️) e aplicar no máximo um prefixo por mensagem.

### Requirement 4: Hierarquia visual e escaneabilidade consistentes

**User Story:** Como operador da CLI, quero saída limpa e escaneável e consistente entre todos os
subcomandos, para identificar rapidamente o estado e os resultados sem ruído.

#### Acceptance Criteria

1. THE Logging_System SHALL renderizar todas as ocorrências de um mesmo Tipo_de_Mensagem com um
   indicador de tipo idêntico (mesmo prefixo e mesmo estilo), independentemente do Subcomando que
   a emite.
2. WHILE o Formato_Pretty está ativo, THE Logging_System SHALL alinhar as chaves dos pares
   chave/valor de um Evento_de_Log em uma coluna comum de largura fixa, de modo que os valores
   correspondentes iniciem na mesma posição de coluna dentro do evento.
3. WHEN uma Mensagem_de_Apresentação de tipo info, sucesso, aviso ou erro é emitida, THE
   Logging_System SHALL produzi-la como uma única linha lógica terminada por exatamente um caractere
   de nova linha, sem caracteres de nova linha embutidos no corpo da mensagem.
4. WHERE o Nível_de_Log configurado é `info`, THE Logging_System SHALL omitir da saída todos os
   Eventos_de_Log de nível debug.
5. THE Logging_System SHALL apresentar todas as mensagens visíveis ao usuário
   (Mensagens_de_Apresentação e mensagens de Eventos_de_Log) em Português (pt-BR).

### Requirement 5: Observabilidade e correlação de eventos

**User Story:** Como engenheiro de operações, quero correlacionar os eventos de uma mesma execução
e rastrear mensagens do collector ao processor, para diagnosticar problemas em produção.

#### Acceptance Criteria

1. WHEN o Logger é construído na raiz de composição, THE Logging_System SHALL gerar
   automaticamente um Correlation_ID não vazio e vinculá-lo ao Logger sob a chave de atributo
   estável `run_id`, propagando-o a todos os Eventos_de_Log derivados sem ação do desenvolvedor.
2. WHEN um Logger possui um Componente vinculado, THE Logging_System SHALL incluir o nome do
   Componente sob a chave de atributo estável `component` em cada Evento_de_Log derivado.
3. WHERE o Formato_JSON está em uso, THE Logging_System SHALL incluir a identidade de serviço
   (`service=limiar-collector` ou `service=limiar-processor`) e o Pipeline_Stage em cada
   Evento_de_Log.
4. WHEN um Logger derivado é criado via `With` ou `WithComponent`, THE Logging_System SHALL incluir
   no Logger derivado todos os atributos do Logger pai além dos atributos adicionados, sem omitir
   nenhum.
5. WHEN um Evento_de_Log é emitido, THE Logging_System SHALL incluir um timestamp com precisão de
   pelo menos milissegundos e o Nível_de_Log do evento.
6. WHERE o Logging_System é usado pelo processor para processar uma mensagem com `canal_id` e
   `msg_id` conhecidos, THE Logging_System SHALL permitir vincular esses atributos via `With`
   para correlação com eventos do collector sobre a mesma mensagem.

### Requirement 6: Performance sob carga contínua

**User Story:** Como engenheiro de performance, quero que o caminho de logging minimize alocações e
formatação supérflua, para sustentar milhões de eventos em execução contínua.

#### Acceptance Criteria

1. WHEN um Evento_de_Log possui Nível_de_Log inferior ao Nível_de_Log configurado, THE
   Logging_System SHALL descartá-lo sem invocar a formatação da mensagem nem a formatação de
   qualquer um de seus atributos.
2. WHILE o Formato_Pretty está ativo, THE Logging_System SHALL formatar valores dos tipos escalares
   suportados (string, inteiro, float, booleano, duração, tempo) sem usar formatação baseada em
   reflexão.
3. THE Logging_System SHALL escrever cada Evento_de_Log com exatamente uma operação de escrita (uma
   única chamada `Write`) no writer de destino.
4. THE Logging_System SHALL fornecer um benchmark Go, executável via `go test -bench` com
   `-benchmem`, que reporte alocações por operação (allocs/op) e bytes por operação (B/op) por
   Evento_de_Log no caminho do Formato_Pretty e no caminho do Formato_JSON.
5. WHILE pelo menos 2 goroutines emitem Eventos_de_Log concorrentemente através do mesmo Logger,
   THE Logging_System SHALL produzir cada registro como uma sequência de bytes contígua, sem
   intercalar bytes de registros distintos.

### Requirement 7: Segurança de concorrência

**User Story:** Como engenheiro, quero garantir que o Logging_System seja seguro sob concorrência,
para que o pipeline fan-out/fan-in não corrompa a saída nem cause data races.

#### Acceptance Criteria

1. WHILE pelo menos 8 goroutines emitem Eventos_de_Log concorrentemente através do mesmo Logger,
   THE Logging_System SHALL produzir cada registro de forma íntegra, sem entrelaçamento de bytes
   entre registros e sem perda de eventos.
2. WHEN um Logger derivado é criado via `With` ou `WithComponent`, THE Logging_System SHALL manter
   inalterados os atributos e o Componente do Logger pai (derivação imutável).
3. WHILE pelo menos 8 goroutines criam Loggers derivados concorrentemente via `With` ou
   `WithComponent` a partir de um mesmo Logger pai, THE Logging_System SHALL produzir Loggers
   derivados independentes sem corromper o estado do Logger pai nem dos demais derivados.
4. WHILE executado sob o detector de corrida do Go (`go test -race`) em um teste que exercite
   emissão e derivação concorrentes a partir de pelo menos 8 goroutines, THE Logging_System SHALL
   não apresentar data races.

### Requirement 8: Conformidade arquitetural e injeção de dependência

**User Story:** Como mantenedor, quero que a evolução preserve as fronteiras arquiteturais do
`AGENTS.md`, para manter a base de código modular, testável e desacoplada.

#### Acceptance Criteria

1. THE Logging_System SHALL construir instância concreta de logger exclusivamente em
   `internal/logger/slog.go`, sendo `NewSlogLogger` o único ponto de construção.
2. THE Logging_System SHALL fazer com que cada camada receba um `logger.Logger` através de seu
   construtor (injeção de dependência), sem que a camada instancie o logger concreto por conta
   própria.
3. THE Logging_System SHALL não conter funções `init()` nem estado mutável global em qualquer
   pacote sob `internal/`, condição verificável por inspeção e pelos quality gates
   (`go build ./...`, `go vet ./...`).
4. THE Logging_System SHALL usar exclusivamente a stdlib `log/slog` como backend de logging, sem
   nenhum import de `zerolog`, `zap` ou `logrus`.
5. WHERE um chamador não fornece um Logger, THE camada SHALL usar `NopLogger`, que descarta os
   eventos sem produzir bytes na saída e sem causar panic ou desreferência de ponteiro nulo.
6. THE interface `logger.Logger` SHALL permanecer com as assinaturas exatas `Debug(msg string,
   args ...any)`, `Info(msg string, args ...any)`, `Warn(msg string, args ...any)`,
   `Error(msg string, args ...any)`, `With(args ...any) Logger` e
   `WithComponent(name string) Logger`.
7. THE camada de CLI SHALL depender da interface `logger.Presenter` para apresentação, permitindo
   substituição por implementações alternativas em testes e em contextos futuros.

### Requirement 9: Seleção de formato por subcomando

**User Story:** Como operador, quero que cada subcomando use o formato de log apropriado, para que
o `run` produza JSON estruturado e os comandos interativos produzam saída legível.

#### Acceptance Criteria

1. WHERE `LIMIAR_LOG_FORMAT` não está definida e o Subcomando é `run`, THE Logging_System SHALL
   usar o Formato_JSON.
2. WHERE `LIMIAR_LOG_FORMAT` não está definida e o Subcomando é `auth`, THE Logging_System SHALL
   usar o Formato_Pretty quando o stream de saída de log é um TTY (detectado via
   `golang.org/x/term`) e o Formato_Text caso contrário.
3. WHERE `LIMIAR_LOG_FORMAT` não está definida e o Subcomando é `channels`, THE Logging_System
   SHALL usar o Formato_Pretty quando o stream de saída de log é um TTY (detectado via
   `golang.org/x/term`) e o Formato_Text caso contrário.
4. WHERE `LIMIAR_LOG_FORMAT` não está definida e o Subcomando é `dashboard`, THE Logging_System
   SHALL usar o Formato_Pretty quando o stream de saída de log é um TTY e o Formato_Text caso
   contrário.
5. IF `LIMIAR_LOG_FORMAT` está definida com um formato válido (`pretty`, `text` ou `json`), THEN
   THE Logging_System SHALL usar o formato configurado para qualquer Subcomando, tendo precedência
   sobre os padrões por Subcomando.
6. IF `LIMIAR_LOG_FORMAT` está definida com um valor não reconhecido (diferente de `pretty`,
   `text`, `json`), THEN THE Logging_System SHALL usar o Formato_Text e emitir um Evento_de_Log de
   nível Warn indicando o valor inválido.

### Requirement 10: Formato de mensagem de erro

**User Story:** Como engenheiro de troubleshooting, quero que erros registrados sigam um formato
consistente, para localizar rapidamente a camada e a operação de origem.

#### Acceptance Criteria

1. WHEN um erro que cruzou um ou mais limites de camada (envolvido via `errors.Wrap`) é registrado
   em um Evento_de_Log, THE Logging_System SHALL preservar o texto completo da cadeia de erro no
   formato `"layer: op: cause"`, usando `": "` (dois-pontos seguido de um espaço) como separador
   entre os segmentos e sem truncar nenhum segmento da cadeia.
2. WHEN um erro é registrado como atributo de um Evento_de_Log, THE Logging_System SHALL emiti-lo
   sob a chave de atributo estável `erro`, idêntica em todos os Eventos_de_Log e em todos os
   formatos (Formato_Pretty, Formato_Text e Formato_JSON).
3. IF o valor de erro associado a um Evento_de_Log é nulo, THEN THE Logging_System SHALL omitir o
   atributo de erro da saída em vez de emitir um valor vazio ou o texto `"<nil>"`.
4. IF uma condição é recuperada por `recover()` no limite da goroutine do Dispatcher, THEN THE
   Logging_System SHALL emitir um Evento_de_Log de nível Error contendo o valor recuperado sob a
   chave de atributo `erro` e o nome do Componente de origem, sem propagar o panic e sem encerrar
   as demais goroutines de handler.

### Requirement 11: Unificação entre collector e processor

**User Story:** Como engenheiro de operações, quero que collector e processor produzam logs com a
mesma estrutura, convenções e correlação, para poder observar o pipeline Limiar como um sistema
coeso e integrado.

#### Acceptance Criteria

1. THE Logging_System SHALL ser compartilhado como um pacote único (`internal/logger`) utilizado
   por ambos os binários (`limiar-collector` e `limiar-processor`).
2. WHERE o Formato_JSON está em uso, THE Logging_System SHALL incluir em cada Evento_de_Log os
   atributos padronizados `service` (identidade do binário), `component` (subsistema),
   `run_id` (correlação de execução) e `pipeline_stage` (`collector` ou `processor`).
3. WHEN o processor processa uma mensagem com `canal_id` e `msg_id` conhecidos, THE
   Logging_System SHALL emitir esses valores como atributos nos Eventos_de_Log associados, de modo
   que possam ser correlacionados com os eventos do collector sobre a mesma mensagem.
4. THE Logging_System SHALL aplicar a mesma política de redação de Chaves_Sensíveis em ambos os
   binários, sem divergência de configuração.
5. THE Logging_System SHALL manter a mesma semântica de Níveis_de_Log e o mesmo formato de erro
   (`"layer: op: cause"`) em ambos os binários.

# AGENTS.md — Orientação para Agentes de IA e Contribuidores

Este arquivo define as regras rígidas para trabalhar no `limiar-collector`. Leia-o
antes de alterar qualquer código. As restrições aqui não são preferências de estilo —
elas são o contrato que mantém a base de código modular, testável e desacoplada
de implementações internas de terceiros.

Caminho do módulo: `github.com/limiar/collector`.

## Leitura obrigatória — sempre consulte `docs/` primeiro

Antes de fazer qualquer alteração no código, **leia a documentação relevante em `docs/`**.
Isso não é opcional. O diretório `docs/` é a fonte da verdade para a arquitetura,
decisões de design e padrões de uso de bibliotecas:

- `docs/ARCHITECTURE.md` — diagrama do pipeline, modelo de concorrência, modelo de dados
- `docs/CONTEXT.md` — visão geral do sistema, limites de escopo, roadmap de fases
- `docs/PRODUCT_BRIEF.md` — objetivos e posicionamento do produto
- `docs/specs/GOTD-TD.md` — padrões de uso e superfície da API do gotd/td
- `docs/specs/TURSOGO.md` — uso e restrições do driver Tursogo
- `docs/guidelines/` — diretrizes de extensão por camada (storage, telegram, collector)
- `docs/adr/` — registros de decisão de arquitetura (motivação para escolhas passadas)

Se uma pergunta puder ser respondida por um documento em `docs/`, use esse documento
em vez de adivinhar. Se uma alteração conflitar com o que `docs/` descreve, atualize
a documentação como parte da mesma alteração.

---

## Escopo: A Fase 1 é coleta + observação

O `limiar-collector` (Fase 1) conecta-se ao Telegram, autentica-se como um userbot,
gerencia canais monitorados, persiste os payloads **brutos (raw)** das mensagens como JSON e
fornece um dashboard leve para inspecionar os dados capturados.

**No escopo:** `auth`, `channels`, `run`, `dashboard`; persistência de sessão/peer;
captura bruta; backfill de histórico na primeira execução; retomada a partir do cursor; desligamento gracioso (graceful shutdown);
resiliência de conexão; assistente de configuração interativo; dashboard HTTP com atualizações em tempo real via SSE;
logging pretty/text/json.

**Fora do escopo (NÃO implemente):** normalização, enriquecimento, classificação semântica,
chamadas LLM, desduplicação além da persistência segura, exportação de métricas,
alertas. Estes pertencem ao `limiar-processor` e `limiar-api`.

Se uma alteração introduzir lógica de processamento, pare — é de uma fase diferente.

---

## A stack fechada

Apenas estas dependências são permitidas. **Nunca adicione uma dependência fora desta lista.**

| Funcionalidade | Permitido | Proibido |
|---------|---------|-----------|
| MTProto | `github.com/gotd/td` | GoTGProto ou qualquer outro wrapper |
| Banco de dados | `turso.tech/database/tursogo` (driver `turso`) | Drivers SQLite, `mattn`, GORM, qualquer ORM |
| CLI/config | `github.com/spf13/cobra`, `github.com/spf13/viper` | — |
| Terminal | `golang.org/x/term` (entrada mascarada no assistente/auth) | — |
| Logging | stdlib `log/slog` (por trás de `logger.Logger`) | zerolog, zap, logrus |
| HTTP | stdlib `net/http` (apenas para o dashboard) | chi, gin, echo, fiber |
| Testes de propriedade | `pgregory.net/rapid` (apenas arquivos de teste) | — |

`pgregory.net/rapid` é a única dependência permitida fora da stack de tempo de execução,
e apenas dentro de arquivos `_test.go`. `golang.org/x/term` é permitido porque
é um módulo adjacente à stdlib usado para entrada segura no terminal.

---

## Regras rígidas

1. **O Logger é instanciado apenas em `internal/logger/slog.go`.** Todas as outras
   camadas recebem um `logger.Logger` através de seu construtor. `NewSlogLogger` é o
   único lugar onde um logger concreto é criado. Chaves sensíveis (`api_hash`,
   `apihash`, `session`, `token`, `password`, `auth_code`) são redigidas (mascaradas).
2. **Sem funções `init()` e sem estado mutável global** em qualquer pacote sob `internal/`.
3. **Sem `panic()` em código de produção.** `recover()` aparece apenas no
   limite da goroutine do Dispatcher (`Dispatcher.invoke`), e toda condição
   recuperada é registrada nos logs.
4. **Um único DBWriter escreve no banco de dados.** Apenas a goroutine `Collector.dbWriter`
   emite escritas através de `*sql.DB` (fan-in via `writeCh chan WriteJob`). Nenhuma outra goroutine escreve. As consultas do Repository do dashboard são apenas de leitura (read-only).
5. **`context.Context` é o primeiro argumento de todo método de I/O.** Veja
   `Repository`, `TelegramClient`, `UpdateHandler`, etc.
6. **Erros que cruzam o limite de uma camada são envolvidos via `errors.Wrap(layer, op, err)`**
   para que as mensagens sejam lidas como `"layer: op: cause"` e a identidade do sentinel
   seja preservada para `errors.Is`. Os sentinels residem em `internal/errors/errors.go`
   (`ErrNotAuthenticated`, `ErrChannelNotFound`, `ErrSessionCorrupted`,
   `ErrDBWriteFailed`, `ErrMaxRetriesExceeded`).
7. **Os tipos do gotd/td nunca vazam para fora de `internal/telegram`.** A CLI, collector,
   e as camadas do dashboard importam apenas a facade `TelegramClient` e os modelos
   de domínio em `internal/storage`. Updates deixam o pacote telegram como `[]byte` JSON
   envolvido em uma struct `telegram.Update` (com metadados de roteamento).
8. **Todo o SQL reside em `internal/storage/repository.go`.** Placeholders são apenas `?`.
   Escritas recorrentes usam prepared statements (instruções preparadas).
9. **Dependências concretas são construídas apenas em `cmd/limiar-collector/main.go`**
   (a raiz de composição). A CLI depende da interface `cli.Provider` e nunca constrói
   as dependências concretas de storage ou telegram por conta própria. DI manual — sem framework de DI.
10. **O dashboard é apenas de leitura.** `internal/dashboard` lê apenas do Repository
    e envia eventos via um Broker. Ele nunca escreve no banco de dados e nunca importa `internal/telegram`.

---

## Subcomandos da CLI

O binário expõe **quatro** subcomandos:

| Comando | Formato de log | Propósito |
|---------|--------------|---------|
| `auth` | pretty/text | Autenticação do Telegram interativa (idempotente) |
| `channels` | pretty/text | `list`, `add <username>`, `remove <username>` |
| `run` | json | Serviço de produção do collector; flag opcional `--dashboard` |
| `dashboard` | text | Dashboard HTTP standalone para inspecionar dados capturados |

---

## Estrutura de pacotes

```
cmd/limiar-collector/main.go    — raiz de composição (único lugar onde as instâncias concretas são ligadas)
internal/
├── cli/            — Comandos Cobra (root, auth, channels, run, dashboard)
│                     Depende apenas da interface Provider
├── collector/      — Orquestração: Collector, MessageHandler, Classifier
├── config/         — Carga (Viper + .env + wizard) + Validação
├── dashboard/      — Servidor HTTP (net/http) + Broker SSE
├── errors/         — Sentinels + ajudante Wrap
├── logger/         — Interface Logger, SlogLogger, PrettyHandler, NopLogger
├── storage/        — Abertura/fechamento do BD, migrações (embed.FS), Repository (todo SQL)
└── telegram/       — Facade TelegramClient, Dispatcher, PeerStore,
                      TursoSessionStorage, ajudantes de codificação/extração
tools/
└── payload-analyzer/  — Análise offline de payload (apenas para momento de desenvolvimento)
docs/
├── ARCHITECTURE.md    — Diagrama do pipeline, modelo de concorrência, modelo de dados
├── CONTEXT.md         — Visão geral do sistema, escopo, roadmap de fases
├── PRODUCT_BRIEF.md   — Resumo do produto
├── PAYLOAD_ANALYSIS_GUIDE.md
├── payload-analysis-report.md
├── adr/               — Registros de decisão de arquitetura
├── guidelines/        — Diretrizes de extensão por camada
└── specs/             — Especificações de uso de bibliotecas (GOTD-TD.md, TURSOGO.md)
```

---

## Interfaces principais

```go
// cli.Provider — fornecimento de dependências para os comandos da CLI
type Provider interface {
    Config() *config.Config
    Logger(format string) logger.Logger
    OpenStore(ctx context.Context) (*storage.Repository, func() error, error)
    NewClient(log logger.Logger, repo *storage.Repository) telegram.TelegramClient
    NewCollector(client telegram.TelegramClient, repo *storage.Repository, log logger.Logger) *collector.Collector
}

// logger.Logger — injetado em todas as camadas
type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
    With(args ...any) Logger
    WithComponent(name string) Logger
}

// telegram.TelegramClient — Facade ocultando o gotd/td
type TelegramClient interface {
    Auth(ctx context.Context) error
    IsAuthenticated(ctx context.Context) (bool, error)
    LoadPeers(ctx context.Context) error
    AddUpdateHandler(h UpdateHandler)
    ResolveChannel(ctx context.Context, username string) (*storage.Peer, error)
    ResolveChannelChecked(ctx context.Context, username string) (*storage.Peer, error)
    FetchHistory(ctx context.Context, channelID int64, minID int64, limit int) ([]HistoryMessage, error)
    Run(ctx context.Context) error
}

// collector.Classifier — Padrão Strategy (NoopClassifier na Fase 1)
type Classifier interface {
    Classify(ctx context.Context, msg *storage.RawMessage) (*storage.RawMessage, error)
}
```

---

## Ordem de build / dependências

Os pacotes formam um grafo acíclico. A compilação ocorre de baixo para cima:

```
errors
config
logger             (← config)
storage            (← config, logger, errors)
telegram/session   (← storage, logger, errors)
telegram/peers     (← storage, logger)
telegram/auth      (← logger)
telegram/dispatcher (← logger)
telegram/client    (← config values, session, peers, auth, dispatcher, logger, errors)
collector/classifier
collector/handler  (← classifier, storage, logger, errors)
collector/collector (← telegram, storage, logger, errors)
dashboard          (← storage, logger)
cli/{root,auth,channels,run,dashboard} (← Provider interface)
cmd/limiar-collector/main.go (← tudo; liga tudo)
```

---

## Configuração

Toda a configuração é lida de variáveis de ambiente prefixadas com `LIMIAR_` (+ arquivo `.env`
opcional no diretório de trabalho (CWD)). Quando as credenciais estão ausentes e o stdin é um TTY, um
assistente interativo as coleta.

| Variável | Padrão | Valores válidos |
|----------|---------|-------------|
| `LIMIAR_APP_ID` | — (obrigatório) | inteiro diferente de zero |
| `LIMIAR_API_HASH` | — (obrigatório) | não vazio (mascarado nos logs) |
| `LIMIAR_DB_PATH` | `./limiar.db` | qualquer caminho válido |
| `LIMIAR_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LIMIAR_LOG_FORMAT` | `pretty` | `json`, `text`, `pretty` |
| `LIMIAR_SHUTDOWN_TIMEOUT` | `15` | 1–300 (segundos) |
| `LIMIAR_MAX_RETRIES` | `10` | inteiro positivo |
| `LIMIAR_IO_TIMEOUT` | `30s` | Duração do Go |
| `LIMIAR_DISPATCHER_BUFFER_SIZE` | `256` | 64–4096 |
| `LIMIAR_DB_WRITER_BUFFER_SIZE` | `512` | 128–8192 |
| `LIMIAR_HISTORY_MAX` | `5000` | 100–100000 (limite máximo de backfill por canal) |
| `LIMIAR_HISTORY_MAX_DAYS` | `30` | 1–365 (limite temporal; backfill para em mensagens mais antigas) |

---

## Modelo de concorrência

```
Telegram MTProto → Client.onUpdate → encodeUpdate → Update{ChannelID, MessageID, Payload}
                                          │
                                    Dispatcher.Dispatch
                                          │  fan-out: canal bufferizado + goroutine por handler
                                          ▼
                              MessageHandler.HandleUpdate
                                    │  filtrar por canais monitorados → Classify → writeCh
                                    ▼
                              Collector.dbWriter (único escritor, fan-in)
                                    │  SaveRawMessage + UpdateChannelLastMessage
                                    ▼
                              Tursogo DB (escritas serializadas)
                                    │
                              callback onMessage → SSE Broker → clientes do dashboard
```

- **Fan-out:** O Dispatcher fornece a cada handler um canal com buffer (padrão 256)
  drenado por uma goroutine dedicada.
- **Isolamento de Panic:** `Dispatcher.invoke` envolve cada handler com `recover()`.
- **Fan-in:** Todos os handlers alimentam `writeCh` (padrão 512). A goroutine única `dbWriter`
  elimina a concorrência de escrita.
- **Handler sem estado:** O `MessageHandler` não mantém estado mutável; sem locks.
- **Cache de peer:** O `PeerStore` protege `map[int64]*Peer` com `sync.RWMutex`.
- **Desligamento gracioso:** `signal.NotifyContext` → cancelar contexto → fechar
  `writeCh` → `WaitGroup.Wait()` → fechar BD. Imposição feita por `ShutdownTimeout`.
- **Dashboard:** SSE não bloqueante via Broker; eventos descartados para clientes lentos.

---

## Critérios de Qualidade (Quality gates)

```sh
go build ./...        # saída 0
go vet ./...          # zero problemas
go test ./...         # todos passam
go test -race ./...   # sem data races (condições de corrida)
```

O build não deve conter nenhum importe de `sqlite`, `mattn` ou `gorm`.

---

## limiar-processor (Fase 2)

O processor é o segundo binário do pipeline. Ele lê `raw_messages`
(read-only) do `limiar.db` compartilhado, normaliza payloads em uma estrutura
canônica, classifica por tipo de mensagem, e escreve em `processed_messages`.

### Estrutura de packages

```
cmd/limiar-processor/main.go      — composition root
internal/processor/
├── config.go       — env vars LIMIAR_PROCESSOR_*, sem credenciais Telegram
├── normalizer.go   — payload JSON → NormalizedMessage (Shapes A e B)
├── classify.go     — cascata de MessageType (deal_complete, commentary, etc.)
├── repository.go   — FetchUnprocessed, SaveProcessed, CountUnprocessed
└── processor.go    — loop de poll: fetch → normalize → classify → persist
```

### Regras do processor

1. **raw_messages são read-only.** O processor nunca escreve em `raw_messages`.
2. **Mesmo banco, tabelas separadas.** Processor adiciona `processed_messages` via
   migration `003_processor_tables.sql`. Tabelas do collector são intocáveis.
3. **Sem dependência do Telegram.** `internal/processor` nunca importa
   `internal/telegram`. O processor apenas lê do DB.
4. **Preços são INTEGER (centavos).** R$ 83,50 → 8350. Conversão acontece apenas nas
   boundaries (extração regex → ×100 → int64). Nunca usar float64 para preços.
5. **Processamento idempotente.** `SaveProcessed` usa `ON CONFLICT DO NOTHING` em
   `(channel_id, message_id)`. Reprocessar a mesma raw_message é seguro.
6. **Batch poll, não streaming.** O processor faz poll de `raw_messages` a cada
   `LIMIAR_PROCESSOR_POLL_INTERVAL` (default 5s) com batch size
   `LIMIAR_PROCESSOR_BATCH_SIZE` (default 50). Streaming pode ser adicionado na Fase 6.
7. **Mesma closed stack.** Nenhuma dependência nova além do que AGENTS.md permite.
   O processor reutiliza `internal/storage` (DB open/migrate), `internal/logger`,
   e `internal/errors` do módulo collector.

### Configuração do processor

| Variável | Default | Valores válidos |
|----------|---------|-----------------|
| `LIMIAR_DB_PATH` | `./limiar.db` | qualquer path válido |
| `LIMIAR_PROCESSOR_POLL_INTERVAL` | `5s` | duração Go [1s, 5m] |
| `LIMIAR_PROCESSOR_BATCH_SIZE` | `50` | 1–1000 |
| `LIMIAR_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LIMIAR_LOG_FORMAT` | `json` | `json`, `text`, `pretty` |

---

## Notas de estilo

- Mensagens de log usam prefixos de emoji para escaneabilidade: 📡 📩 📜 🔄 ❌ ✅ 🛑 ⏰ 🌐 ⚠️
- Strings visíveis ao usuário estão em Português (pt-BR).
- Comentários de código e documentação estão em Português (pt-BR).
- Todas as mensagens de erro seguem o formato `"layer: op: cause"`.

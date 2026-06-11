# ARCHITECTURE — Limiar Pipeline

O repositório contém dois binários de produção (`limiar-collector` e `limiar-processor`)
e uma ferramenta de desenvolvimento (`spike_resolve`). Ambos os binários compartilham o mesmo
banco de dados Tursogo (`limiar.db`) e reutilizam infraestrutura transversal (`logger`,
`errors`, `storage`).

A base de código é dividida horizontalmente em camadas com fronteiras rígidas:
- **`internal/cli`** — ciclo de vida dos comandos Cobra (collector)
- **`internal/collector`** — orquestração da captura com concorrência fan-out/fan-in
- **`internal/telegram`** — Fachada (Facade) rígida ocultando o gotd/td e MTProto
- **`internal/storage`** — banco de dados Tursogo, migrações, Repository (todo SQL)
- **`internal/processor`** — normalização, classificação, dedup, síntese (processor)
- **`internal/dashboard`** — servidor HTTP + Broker SSE (read-only)
- **`internal/terminal`** — entrada segura de terminal (senha mascarada)
- **`internal/logger`** — interface Logger + Presenter, implementação slog com 3 handlers
- **`internal/config`** — carga e validação de configuração (Viper + .env + wizard)
- **`internal/errors`** — sentinels + helper Wrap

Padrões de design clássicos unem as camadas: **Facade** (`telegram.TelegramClient`),
**Observer** (`telegram.UpdateHandler`, `telegram.Dispatcher`), **Strategy**
(`collector.Classifier`) e **Adapter** (`collector.MessageHandler`) — com injeção
manual de dependências conectada apenas nas raízes de composição
(`cmd/limiar-collector/main.go` e `cmd/limiar-processor/main.go`).

---

## Diagrama do Pipeline Completo

```
┌──────────────────────────────────────────────────────────────────────────┐
│            FASE 1 — limiar-collector                                       │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  Telegram MTProto                                                         │
│       │  updates / histórico                                              │
│       ▼                                                                   │
│  telegram.Client (facade gotd/td)                                         │
│       │  encodeUpdate → JSON []byte                                       │
│       ▼                                                                   │
│  telegram.Dispatcher (fan-out, buffer 256, recover por goroutine)          │
│       │                                                                   │
│       ▼                                                                   │
│  collector.MessageHandler → Classify(Noop) → writeCh ← WriteJob          │
│       │                                                                   │
│       ▼                                                                   │
│  collector.dbWriter (ÚNICO escritor, fan-in)                              │
│       │  SaveRawMessage + UpdateChannelLastMessage                         │
│       ▼                                                                   │
│  ┌─────────────────────────────────────────┐                              │
│  │  Tursogo ./limiar.db                    │                              │
│  │  raw_messages | channels | peers | sessions │                          │
│  └─────────────────────────────────────────┘                              │
└──────────────────────────────────────────────────────────────────────────┘
                              │
                              │ (mesmo arquivo .db, read-only)
                              ▼
┌──────────────────────────────────────────────────────────────────────────┐
│            FASE 2 — limiar-processor                                      │
├──────────────────────────────────────────────────────────────────────────┤
│                                                                           │
│  processor.Run (poll loop com drain mode)                                 │
│       │  FetchUnprocessed (batch)                                         │
│       ▼                                                                   │
│  processor.Normalize (Shape A/B → NormalizedMessage)                      │
│       │  texto, preços, cupons, mídia, URLs, merchants                    │
│       ▼                                                                   │
│  processor.Classify (cascata 11 tipos → MessageType)                      │
│       ▼                                                                   │
│  processor.markDuplicates (dedup cross-channel via URL hash)              │
│       ▼                                                                   │
│  processor.Synthesize (→ SynthesizedPromotion JSON)                       │
│       ▼                                                                   │
│  processor.SaveProcessedBatch (transação única)                           │
│       │                                                                   │
│       ▼                                                                   │
│  ┌─────────────────────────────────────────┐                              │
│  │  Tursogo ./limiar.db                    │                              │
│  │  processed_messages (32 colunas, 7 idx) │                              │
│  └─────────────────────────────────────────┘                              │
└──────────────────────────────────────────────────────────────────────────┘
                              │
                              │ (dashboard opcional, SSE)
                              ▼
┌──────────────────────────────────────────────────────────────────────────┐
│            DASHBOARD (embutido em ambos os binários)                       │
│  net/http + SSE Broker → navegador                                        │
│  Read-only: consulta raw_messages via Repository                          │
└──────────────────────────────────────────────────────────────────────────┘
```

---

## Composição do limiar-collector

```
┌──────────────────────────────────────────────────────────────────────┐
│                  cmd/limiar-collector/main.go                          │
│        raiz de composição — ÚNICO local de injeção                     │
│        (provider implementa cli.Provider)                              │
└───────────────────────────────────┬────────────────────────────────────┘
                                     │ NewRootCmd(provider)
          ┌──────────────┬───────────┼───────────────┬──────────────┐
          ▼              ▼           ▼               ▼              │
   ┌───────────┐  ┌──────────────┐ ┌───────────┐ ┌────────────┐   │
   │ cli/auth  │  │ cli/channels │ │  cli/run  │ │cli/dashboard│   │
   │(pretty/txt)│ │ (pretty/txt) │ │  (json)   │ │   (text)   │   │
   └─────┬─────┘  └──────┬───────┘ └─────┬─────┘ └──────┬─────┘   │
         │               │               │              │          │
         └───────────────┼───────────────┼──────────────┘          │
                         ▼               ▼                         │
┌──────────────────────────────────────────────────────────────────┘
│                         camada collector
│  Collector (orquestrador)   MessageHandler (Adapter)
│  - dbWriter goroutine        - []byte → storage.RawMessage
│  - backfill history          Classifier (Strategy → NoopClassifier)
└───────────────────────────────────┬──────────────────────────────────
                                     ▼
┌──────────────────────────────────────────────────────────────────────┐
│                    camada telegram (Facade)                            │
│  TelegramClient (interface)   Dispatcher (Observer)                    │
│  Client (impl, encapsula gotd) PeerStore (RWMutex)                     │
│  TursoSessionStorage          terminalAuthenticator (auth.UserAuth.)   │
│                          │                                             │
│                   ┌──────┴───────┐                                     │
│                   │  gotd/td     │  ← ÚNICO ponto de contato MTProto   │
│                   └──────────────┘                                     │
└───────────────────────────────────┬────────────────────────────────────┘
                                     ▼
┌──────────────────────────────────────────────────────────────────────┐
│                        camada storage                                  │
│  db.go (Open/Close/Conn)   migrations.go (embed.FS)                    │
│  repository.go (todo SQL, placeholders ?, prepared statements)         │
│                          │                                             │
│                   ┌──────┴───────┐                                     │
│                   │ Tursogo "turso"│  database/sql, sem CGO            │
│                   │  ./limiar.db │                                     │
│                   └──────────────┘                                     │
└──────────────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────────────┐
│  transversal: config (viper/env)  logger (slog)  errors (wrap)         │
│               terminal (golang.org/x/term)  dashboard (net/http+SSE)   │
└──────────────────────────────────────────────────────────────────────┘
```

---

## Composição do limiar-processor

```
┌──────────────────────────────────────────────────────────────────────┐
│                  cmd/limiar-processor/main.go                          │
│        raiz de composição — binário standalone                         │
│        flags: --dashboard, --dashboard-port                            │
└───────────────────────────────────┬────────────────────────────────────┘
                                     │
         ┌───────────────────────────┼────────────────────┐
         ▼                           ▼                    ▼
┌─────────────────┐       ┌──────────────────┐   ┌──────────────┐
│ processor.Config│       │ processor.Process │   │  dashboard   │
│ (LoadConfig +   │       │ (poll loop)       │   │ (opcional)   │
│  Validate)      │       │                   │   │              │
└─────────────────┘       └─────────┬─────────┘   └──────────────┘
                                     │
                          ┌──────────┼──────────┐
                          ▼          ▼          ▼
                   Normalize    Classify    Synthesize
                          │          │          │
                          └──────────┼──────────┘
                                     ▼
                          processor.Repository
                          (FetchUnprocessed, SaveProcessedBatch,
                           ExistsURLHashes, CountUnprocessed)
                                     │
                                     ▼
                              Tursogo ./limiar.db
```

---

## Responsabilidades das camadas

### Collector (Fase 1)

- **`cmd/limiar-collector/main.go`** — raiz de composição. A struct `provider` implementa `cli.Provider` e é o único lugar que constrói instâncias concretas de `storage`, `telegram`, `logger` e `collector`. `run()` chama `config.Load` seguido de `config.Validate` antes de construir a árvore de comandos.
- **`internal/cli`** — Árvore de comandos Cobra (`NewRootCmd`, `newAuthCmd`, `newChannelsCmd`, `newRunCmd`, `newDashboardCmd`). Depende apenas da interface `Provider`; nunca constrói dependências concretas.
- **`internal/collector`** — orquestração e adaptação. `Collector` executa a única goroutine `dbWriter`, realiza o backfill inicial e conduz `client.Run`. `MessageHandler` adapta bytes brutos do update para `storage.RawMessage`. `Classifier`/`NoopClassifier` é o Strategy plugável.
- **`internal/telegram`** — a Fachada sobre o gotd/td. `TelegramClient` expõe `Auth`, `IsAuthenticated`, `AddUpdateHandler`, `ResolveChannel`, `ResolveChannelChecked`, `FetchHistory`, `LoadPeers`, `Run`. `Client` é o único tipo que importa gotd/td. `Dispatcher` faz fan-out; `PeerStore` cache com RWMutex; `TursoSessionStorage` satisfaz `session.Storage`; `terminalAuthenticator` satisfaz `auth.UserAuthenticator`.
- **`internal/storage`** — toda a persistência do collector. `DB` (open/close/conn), `migrate` (`migrations/*.sql` embutidas) e `Repository` (todas as instruções SQL).
- **`internal/dashboard`** — servidor HTTP + Broker SSE. Read-only: consulta `raw_messages` via `storage.Repository`. Nunca escreve no BD e nunca importa `internal/telegram`.

### Processor (Fase 2)

- **`cmd/limiar-processor/main.go`** — raiz de composição standalone. Usa `flag` (não Cobra). Abre o banco, constrói `processor.Repository`, instancia `Processor`, e opcionalmente inicia o dashboard.
- **`internal/processor/config.go`** — carrega variáveis `LIMIAR_*` via Viper + `.env`.
- **`internal/processor/normalizer.go`** — `Normalize(raw *storage.RawMessage) (*NormalizedMessage, error)`. Detecta Shape A/B, extrai texto limpo, preços (centavos BRL), cupons, URLs, mídia, merchants, modifiers.
- **`internal/processor/classify.go`** — `Classify(nm *NormalizedMessage) MessageType`. Cascata de 11 tipos exclusivos por ordem de especificidade.
- **`internal/processor/synthesize.go`** — `Synthesize(nm *NormalizedMessage) SynthesizedPromotion`. Detecta merchant do domínio da URL e produz struct para o feed.
- **`internal/processor/repository.go`** — `FetchUnprocessed`, `SaveProcessedBatch`, `ExistsURLHashes`, `CountUnprocessed`. Usa prepared statements. `ON CONFLICT DO NOTHING` para idempotência.
- **`internal/processor/processor.go`** — `Processor.Run(ctx)`. Poll loop com drain mode: se batch encheu, continua imediatamente (drena backlog). Batch processing: normalize → classify → markDuplicates → save em transação única.

### Infraestrutura transversal

- **`internal/config`** — carga de configuração (Viper + `.env` + wizard interativo para credenciais ausentes). Validação de constraints em `Validate()`.
- **`internal/logger`** — subsistema de logging com três componentes:
  - **`Logger` (interface)** — `Debug`, `Info`, `Warn`, `Error`, `With`, `WithComponent`. Injetada via construtor em todas as camadas.
  - **`SlogLogger`** — implementação única. Suporta três handlers: `json` (slog.JSONHandler), `text` (slog.TextHandler), `pretty` (PrettyHandler custom).
  - **`PrettyHandler`** — handler slog custom para output legível. Formato multilinhas com tree-view (├/└), cores ANSI condicionais, padding de chave por runes. Seguro para concorrência via mutex compartilhado.
  - **`Presenter` (interface)** — mensagens de apresentação ao operador (`Info`, `Success`, `Warning`, `Error`, `Step`). `TerminalPresenter` implementa com emojis e cores ANSI condicionais. Separado do log estruturado.
  - **`ResolveFormat`** — decide formato efetivo baseado em env `LIMIAR_LOG_FORMAT` + detecção TTY.
  - **`redactAttr`** — máscara automática de chaves sensíveis em todos os formatos.
  - **`NopLogger`** — implementação no-op para testes.
- **`internal/terminal`** — `ReadPasswordMasked(fd int)`: leitura de senha com eco mascarado (`*`), suporte a backspace, Ctrl+C, Ctrl+D. Usa `golang.org/x/term` para raw mode.
- **`internal/errors`** — sentinels (`ErrNotAuthenticated`, `ErrChannelNotFound`, `ErrSessionCorrupted`, `ErrDBWriteFailed`, `ErrMaxRetriesExceeded`) + helper `Wrap(layer, op, err)` para mensagens `"layer: op: cause"` preservando `errors.Is`.

---

## Modelo de Concorrência (Collector — fan-out / fan-in)

```
                 Telegram MTProto
                       │  tg.UpdatesClass
                       ▼
              Client.onUpdate → encodeUpdate → []byte
                       │
                 Dispatcher.Dispatch
                       │  fan-out: um canal bufferizado + goroutine por handler
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
   consume(h1)    consume(h2)    consume(hN)     (recover() por goroutine)
        │              │              │
        ▼              ▼              ▼
   MessageHandler.HandleUpdate → Classify → writeCh <- WriteJob
        │              │              │
        └──────────────┼──────────────┘
                       │  fan-in: único canal WriteJob
                       ▼
              Collector.dbWriter  (ÚNICO escritor no BD)
                       │  SaveRawMessage + UpdateChannelLastMessage
                       ▼
                  Banco Tursogo (escritas serializadas)
```

Principais propriedades:

- **Fan-out:** O `Dispatcher` fornece a cada `UpdateHandler` registrado seu próprio canal bufferizado (`DispatcherBufferSize`, padrão 256) drenado por uma goroutine dedicada, para que um handler lento não bloqueie os demais.
- **Isolamento de panic:** `Dispatcher.invoke` envolve cada chamada de handler com `recover()` e registra a condição recuperada no log — um update inválido nunca derruba o processo.
- **Fan-in:** todos os handlers enviam valores `WriteJob` para um único `writeCh` (`DBWriterBufferSize`, padrão 512). A goroutine única `dbWriter` é o único escritor no `*sql.DB`, eliminando concorrência de escrita.
- **Handler stateless (sem estado):** `MessageHandler` não possui estado mutável e não adquire locks; é seguro para invocação concorrente.
- **Cache de peers:** `PeerStore` protege seu `map[int64]*storage.Peer` com um `sync.RWMutex`; leituras concorrentes não bloqueiam umas às outras.
- **Graceful shutdown:** `cli/run` constrói um `signal.NotifyContext` para `SIGTERM`/`SIGINT`. Ao cancelar, `Collector.shutdown` fecha `writeCh` e faz `wg.Wait()` para o escritor drenar; `run` impõe `ShutdownTimeout` com uma corrida usando `time.After`.

## Modelo de Concorrência (Processor)

O processor é **single-threaded** por design — sem goroutines internas:

- Poll loop sequencial: `time.After(PollInterval)` → `processBatch` → repeat.
- **Drain mode:** se `len(msgs) >= BatchSize`, o loop não espera o ticker e processa o próximo batch imediatamente (drena backlog acumulado mais rápido).
- Toda normalização/classificação é CPU-bound (regex, parsing); toda I/O (fetch, save) é em transação única por batch.
- Graceful shutdown via `signal.NotifyContext` → `ctx.Done()` no select do loop.

---

## Modelo de dados

### Tabelas (4 migrations)

**001_initial.sql** — tabelas do collector:

```sql
sessions       (id=1, data BLOB, updated_at)          -- sessão MTProto (single-row)
peers          (id PK, access_hash, type, username)    -- cache de access_hash
channels       (id PK, username UNIQUE, title, active) -- canais monitorados
raw_messages   (id AUTO, channel_id FK, message_id, payload JSON, schema_version=1)
               UNIQUE(channel_id, message_id)          -- dedup persistente segura
```

**002_dashboard_indexes.sql:**

```sql
idx_raw_messages_channel_received_at ON raw_messages(channel_id, received_at DESC)
```

**003_processor_tables.sql** — tabela do processor:

```sql
processed_messages (
    id AUTO, raw_message_id FK, channel_id, message_id,
    message_type, text_clean, text_length, media_type, photo_id,
    views, forwards, reply_to_msg_id,
    has_url, has_price, has_coupon, price_amount, price_currency,
    urgency_signals, posted_at, processed_at,
    price_original, price_discount, coupon_code,
    payment_method, shipping, installments, discount_percent,
    url_hash, merchant, product_name, synthesis,
    is_duplicate, feed_eligible,
    UNIQUE(channel_id, message_id)
)
-- 7 indexes: channel, type, posted, url_hash, merchant, feed
```

**004_dashboard_index.sql:**

```sql
idx_raw_messages_received_at ON raw_messages(received_at DESC)
```

### Structs internas (collector)

```go
type RawMessage struct {
    ID, ChannelID, MessageID int64
    Payload       []byte      // update bruto serializado como JSON
    ReceivedAt    time.Time
    SchemaVersion int         // 1 na Fase 1
}

type Channel struct {
    ID              int64
    Username, Title string
    Active          bool
    AddedAt         time.Time
    LastMessageID   int64
    LastCollectedAt time.Time
}

type Peer struct {
    ID, AccessHash int64
    Type, Username string
    UpdatedAt      time.Time
}
```

### Structs internas (processor)

```go
type NormalizedMessage struct {
    RawMessageID, MessageID, ChannelID int64
    PostedAt, ReceivedAt, ProcessedAt  time.Time
    Text          string
    TextLength    int
    MediaType     string      // "photo"|"video"|"document"|"poll"|"webpage"|"none"
    PhotoID       int64
    Views, Forwards int
    ReplyToMsgID  int64
    HasURL, HasPrice, HasCoupon bool
    PriceAmount   int64       // centavos BRL — preço final
    PriceOriginal int64       // centavos BRL — preço "De"
    PriceDiscount int         // percentual calculado
    CouponCode    string
    PaymentMethod string      // "pix" | ""
    Shipping      string      // "frete_gratis" | "frete_gratis_prime" | ""
    Installments  string      // "9x_sem_juros" | ""
    DiscountPct   int         // do texto, não-calculado
    IsCashback    bool
    URLHash       string      // SHA-256 da primeira URL normalizada
    Merchant      string      // inferido do domínio
    ProductName   string      // heurística (LLM futuro)
    IsDuplicate   bool        // mesma URL em outro canal
    FeedEligible  bool        // deal_complete ou deal_no_coupon
    UrgencySignals []string
    MessageType   string
    Synthesis     string      // JSON serializado de SynthesizedPromotion
}

type SynthesizedPromotion struct {
    Merchant        string
    PriceOriginal   int64
    PriceFinal      int64
    DiscountPercent  int
    CouponCode      string
    PaymentMethod   string
    Shipping        string
    Installments    string
    URL             string
}
```

Colunas datetime são armazenadas como texto no formato `2006-01-02 15:04:05` (UTC),
correspondendo aos padrões `datetime('now')` do schema.

---

## Interfaces principais

```go
// cli.Provider — fornecimento de dependências para os comandos da CLI
type Provider interface {
    Config() *config.Config
    Logger(format string) logger.Logger
    Presenter() logger.Presenter
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

// logger.Presenter — mensagens de apresentação ao operador humano
type Presenter interface {
    Info(text string)
    Success(text string)
    Warning(text string)
    Error(text string)
    Step(text string)
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

## Estrutura de pacotes

```
cmd/
├── limiar-collector/main.go    — raiz de composição (collector)
├── limiar-processor/main.go    — raiz de composição (processor)
└── spike_resolve/main.go       — ferramenta de dev (resolução de canais)

internal/
├── cli/            — Comandos Cobra (root, auth, channels, run, dashboard)
│                     Depende apenas da interface Provider
├── collector/      — Orquestração: Collector, MessageHandler, Classifier
├── config/         — Carga (Viper + .env + wizard) + Validação
├── dashboard/      — Servidor HTTP (net/http) + Broker SSE
├── errors/         — Sentinels + helper Wrap
├── logger/         — Interface Logger + Presenter, SlogLogger, PrettyHandler,
│                     TerminalPresenter, ResolveFormat, redactAttr, NopLogger
├── processor/      — Config, Normalize, Classify, Synthesize, Repository, Processor
├── storage/        — Abertura/fechamento do BD, migrações (embed.FS), Repository (todo SQL)
├── telegram/       — Facade TelegramClient, Dispatcher, PeerStore,
│                     TursoSessionStorage, helpers de codificação/extração
└── terminal/       — ReadPasswordMasked (golang.org/x/term, raw mode)

tools/
└── payload-analyzer/  — Análise offline de payload (apenas dev)

docs/
├── ARCHITECTURE.md    — este arquivo
├── CONTEXT.md         — visão geral do sistema, escopo, roadmap
├── PRODUCT_BRIEF.md   — resumo do produto
├── PAYLOAD_ANALYSIS_GUIDE.md
├── payload-analysis-report.md
├── adr/               — registros de decisão de arquitetura
├── guidelines/        — diretrizes de extensão por camada
└── specs/             — especificações de uso de bibliotecas (GOTD-TD.md, TURSOGO.md)
```

---

## Ordem de build / dependências

```
errors
config
logger              (← config para ParseLevel)
storage             (← config, logger, errors)
terminal            (← golang.org/x/term)
telegram/session    (← storage, logger, errors)
telegram/peers      (← storage, logger)
telegram/auth       (← logger, terminal)
telegram/dispatcher (← logger)
telegram/client     (← config values, session, peers, auth, dispatcher, logger, errors)
collector/classifier
collector/handler   (← classifier, storage, logger, errors)
collector/collector (← telegram, storage, logger, errors)
processor           (← storage, logger, errors)
dashboard           (← storage, logger)
cli/{root,auth,channels,run,dashboard} (← Provider interface)
cmd/limiar-collector/main.go (← tudo do collector; liga tudo)
cmd/limiar-processor/main.go (← processor, storage, logger, dashboard)
```

O grafo é acíclico; gotd/td é confinado ao `internal/telegram` e `database/sql` ao
`internal/storage` + `internal/processor`.

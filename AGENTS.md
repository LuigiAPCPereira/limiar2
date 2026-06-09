# AGENTS.md — Guidance for AI Agents and Contributors

This file defines the hard rules for working on `limiar-collector`. Read it
before touching any code. The constraints here are not stylistic preferences —
they are the contract that keeps the codebase modular, testable, and decoupled
from third-party internals.

Module path: `github.com/limiar/collector`.

## Required reading — always consult `docs/` first

Before making any code change, **read the relevant documentation in `docs/`**.
This is not optional. The `docs/` directory is the source of truth for
architecture, design decisions, and library usage patterns:

- `docs/ARCHITECTURE.md` — pipeline diagram, concurrency model, data model
- `docs/CONTEXT.md` — system overview, scope boundaries, phase roadmap
- `docs/PRODUCT_BRIEF.md` — product goals and positioning
- `docs/specs/GOTD-TD.md` — gotd/td usage patterns and API surface
- `docs/specs/TURSOGO.md` — Tursogo driver usage and constraints
- `docs/guidelines/` — per-layer extension guidelines (storage, telegram, collector)
- `docs/adr/` — architecture decision records (rationale for past choices)

If a question can be answered by a document in `docs/`, use that document
instead of guessing. If a change conflicts with what `docs/` describes, update
the documentation as part of the same change.

---

## Scope: Phase 1 is collection + observation

`limiar-collector` (Phase 1) connects to Telegram, authenticates as a userbot,
manages monitored channels, persists **raw** message payloads as JSON, and
provides a lightweight dashboard for inspecting captured data.

**In scope:** `auth`, `channels`, `run`, `dashboard`; session/peer persistence;
raw capture; first-run history backfill; resume from cursor; graceful shutdown;
connection resilience; interactive config wizard; HTTP dashboard with SSE
real-time updates; pretty/text/JSON logging.

**Out of scope (do NOT implement):** normalization, enrichment, semantic
classification, LLM calls, deduplication beyond safe persistence, metrics
export, alerting. These belong to `limiar-processor` and `limiar-api`.

If a change introduces processing logic, stop — it is a different phase.

---

## The closed stack

Only these dependencies are permitted. **Never add a dependency outside this
list.**

| Concern | Allowed | Forbidden |
|---------|---------|-----------|
| MTProto | `github.com/gotd/td` | GoTGProto or any other wrapper |
| Database | `turso.tech/database/tursogo` (driver `turso`) | SQLite drivers, `mattn`, GORM, any ORM |
| CLI/config | `github.com/spf13/cobra`, `github.com/spf13/viper` | — |
| Terminal | `golang.org/x/term` (masked input in wizard/auth) | — |
| Logging | stdlib `log/slog` (behind `logger.Logger`) | zerolog, zap, logrus |
| HTTP | stdlib `net/http` (dashboard only) | chi, gin, echo, fiber |
| Property tests | `pgregory.net/rapid` (test files only) | — |

`pgregory.net/rapid` is the only dependency allowed outside the runtime stack,
and only inside `_test.go` files. `golang.org/x/term` is permitted because it
is a stdlib-adjacent module used for secure terminal input.

---

## Hard rules

1. **Logger is instantiated only in `internal/logger/slog.go`.** Every other
   layer receives a `logger.Logger` via its constructor. `NewSlogLogger` is the
   sole place a concrete logger is created. Sensitive keys (`api_hash`,
   `apihash`, `session`, `token`, `password`, `auth_code`) are redacted.
2. **No `init()` functions and no global mutable state** in any `internal/`
   package.
3. **No `panic()` in production code.** `recover()` appears only at the
   Dispatcher's goroutine boundary (`Dispatcher.invoke`), and every recovered
   condition is logged.
4. **A single DBWriter writes to the database.** Only the `Collector.dbWriter`
   goroutine issues writes through the `*sql.DB` (fan-in via `writeCh chan
   WriteJob`). No other goroutine writes. The dashboard's Repository queries
   are read-only.
5. **`context.Context` is the first argument of every I/O method.** See
   `Repository`, `TelegramClient`, `UpdateHandler`, etc.
6. **Errors crossing a layer boundary are wrapped via `errors.Wrap(layer, op,
   err)`** so messages read `"layer: op: cause"` and sentinel identity is
   preserved for `errors.Is`. Sentinels live in `internal/errors/errors.go`
   (`ErrNotAuthenticated`, `ErrChannelNotFound`, `ErrSessionCorrupted`,
   `ErrDBWriteFailed`, `ErrMaxRetriesExceeded`).
7. **gotd/td types never leak past `internal/telegram`.** The CLI, collector,
   and dashboard layers import only the `TelegramClient` facade and the domain
   models in `internal/storage`. Updates leave the telegram package as `[]byte`
   JSON wrapped in a `telegram.Update` struct (with routing metadata).
8. **All SQL lives in `internal/storage/repository.go`.** Placeholders are `?`
   only. Recurring writes use prepared statements.
9. **Concrete dependencies are constructed only in
   `cmd/limiar-collector/main.go`** (the composition root). The CLI depends on
   the `cli.Provider` interface and never constructs storage or telegram
   concretes itself. Manual DI — no DI framework.
10. **The dashboard is read-only.** `internal/dashboard` only reads from the
    Repository and pushes events via a Broker. It never writes to the database
    and never imports `internal/telegram`.

---

## CLI subcommands

The binary exposes **four** subcommands:

| Command | Logger format | Purpose |
|---------|--------------|---------|
| `auth` | pretty/text | Interactive Telegram authentication (idempotent) |
| `channels` | pretty/text | `list`, `add <username>`, `remove <username>` |
| `run` | json | Production collector service; optional `--dashboard` flag |
| `dashboard` | text | Standalone HTTP dashboard for inspecting captured data |

---

## Package structure

```
cmd/limiar-collector/main.go    — composition root (only place concretes are wired)
internal/
├── cli/            — Cobra commands (root, auth, channels, run, dashboard)
│                     Depends only on Provider interface
├── collector/      — Orchestration: Collector, MessageHandler, Classifier
├── config/         — Load (Viper + .env + wizard) + Validate
├── dashboard/      — HTTP server (net/http) + SSE Broker
├── errors/         — Sentinels + Wrap helper
├── logger/         — Logger interface, SlogLogger, PrettyHandler, NopLogger
├── storage/        — DB open/close, migrations (embed.FS), Repository (all SQL)
└── telegram/       — TelegramClient facade, Dispatcher, PeerStore,
                      TursoSessionStorage, encode/extract helpers
tools/
└── payload-analyzer/  — Offline payload analysis (development-time only)
docs/
├── ARCHITECTURE.md    — Pipeline diagram, concurrency model, data model
├── CONTEXT.md         — System overview, scope, phase roadmap
├── PRODUCT_BRIEF.md   — Product brief
├── PAYLOAD_ANALYSIS_GUIDE.md
├── payload-analysis-report.md
├── adr/               — Architecture decision records
├── guidelines/        — Extension guidelines per layer
└── specs/             — Library usage specs (GOTD-TD.md, TURSOGO.md)
```

---

## Key interfaces

```go
// cli.Provider — dependency supply for CLI commands
type Provider interface {
    Config() *config.Config
    Logger(format string) logger.Logger
    OpenStore(ctx context.Context) (*storage.Repository, func() error, error)
    NewClient(log logger.Logger, repo *storage.Repository) telegram.TelegramClient
    NewCollector(client telegram.TelegramClient, repo *storage.Repository, log logger.Logger) *collector.Collector
}

// logger.Logger — injected into every layer
type Logger interface {
    Debug(msg string, args ...any)
    Info(msg string, args ...any)
    Warn(msg string, args ...any)
    Error(msg string, args ...any)
    With(args ...any) Logger
    WithComponent(name string) Logger
}

// telegram.TelegramClient — Facade hiding gotd/td
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

// collector.Classifier — Strategy pattern (NoopClassifier in Phase 1)
type Classifier interface {
    Classify(ctx context.Context, msg *storage.RawMessage) (*storage.RawMessage, error)
}
```

---

## Build / dependency order

Packages form an acyclic graph. Build bottom-up:

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
cmd/limiar-collector/main.go (← everything; wires it all)
```

---

## Configuration

All config is read from `LIMIAR_`-prefixed environment variables (+ optional
`.env` file in CWD). When credentials are missing and stdin is a TTY, an
interactive wizard collects them.

| Variable | Default | Valid values |
|----------|---------|-------------|
| `LIMIAR_APP_ID` | — (required) | non-zero integer |
| `LIMIAR_API_HASH` | — (required) | non-empty (masked in logs) |
| `LIMIAR_DB_PATH` | `./limiar.db` | any valid path |
| `LIMIAR_LOG_LEVEL` | `info` | `debug`, `info`, `warn`, `error` |
| `LIMIAR_LOG_FORMAT` | `pretty` | `json`, `text`, `pretty` |
| `LIMIAR_SHUTDOWN_TIMEOUT` | `15` | 1–300 (seconds) |
| `LIMIAR_MAX_RETRIES` | `10` | positive integer |
| `LIMIAR_IO_TIMEOUT` | `30s` | Go duration |
| `LIMIAR_DISPATCHER_BUFFER_SIZE` | `256` | 64–4096 |
| `LIMIAR_DB_WRITER_BUFFER_SIZE` | `512` | 128–8192 |
| `LIMIAR_HISTORY_MAX` | `5000` | 100–100000 (per-channel backfill ceiling) |
| `LIMIAR_HISTORY_MAX_DAYS` | `30` | 1–365 (temporal cutoff; backfill stops at older messages) |

---

## Concurrency model

```
Telegram MTProto → Client.onUpdate → encodeUpdate → Update{ChannelID, MessageID, Payload}
                                          │
                                    Dispatcher.Dispatch
                                          │  fan-out: buffered chan + goroutine per handler
                                          ▼
                              MessageHandler.HandleUpdate
                                    │  filter by monitored channels → Classify → writeCh
                                    ▼
                              Collector.dbWriter (single writer, fan-in)
                                    │  SaveRawMessage + UpdateChannelLastMessage
                                    ▼
                              Tursogo DB (serialized writes)
                                    │
                              onMessage callback → SSE Broker → dashboard clients
```

- **Fan-out:** Dispatcher gives each handler a buffered channel (default 256)
  drained by a dedicated goroutine.
- **Panic isolation:** `Dispatcher.invoke` wraps each handler with `recover()`.
- **Fan-in:** All handlers feed `writeCh` (default 512). Single `dbWriter`
  goroutine eliminates write contention.
- **Stateless handler:** `MessageHandler` holds no mutable state; no locks.
- **Peer cache:** `PeerStore` guards `map[int64]*Peer` with `sync.RWMutex`.
- **Graceful shutdown:** `signal.NotifyContext` → cancel context → close
  `writeCh` → `WaitGroup.Wait()` → close DB. Enforced by `ShutdownTimeout`.
- **Dashboard:** Non-blocking SSE via Broker; dropped events on slow clients.

---

## Quality gates

```sh
go build ./...        # exit 0
go vet ./...          # zero issues
go test ./...         # all pass
go test -race ./...   # no data races
```

The build must exclude any import of `sqlite`, `mattn`, or `gorm`.

---

## Style notes

- Log messages use emoji prefixes for scanability: 📡 📩 📜 🔄 ❌ ✅ 🛑 ⏰ 🌐 ⚠️
- User-facing strings are in Portuguese (pt-BR).
- Code comments and documentation are in Portuguese(PT-BR).
- All error messages follow `"layer: op: cause"` format.

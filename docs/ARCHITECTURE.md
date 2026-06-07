# ARCHITECTURE — limiar-collector (Phase 1)

`limiar-collector` is a standalone Go binary (module `github.com/limiar/collector`)
that captures raw Telegram messages into an embedded Tursogo database. It follows
four design patterns — **Facade** (`telegram.TelegramClient`), **Observer**
(`telegram.Dispatcher`), **Strategy** (`collector.Classifier`), and **Adapter**
(`collector.MessageHandler`) — with manual dependency injection wired only in
`cmd/limiar-collector/main.go`.

## Pipeline diagram

```
┌──────────────────────────────────────────────────────────────────────┐
│                  cmd/limiar-collector/main.go                          │
│        composition root — the ONLY place concretes are wired           │
│        (provider implements cli.Provider)                              │
└───────────────────────────────────┬────────────────────────────────────┘
                                     │ NewRootCmd(provider)
                 ┌───────────────────┼───────────────────┐
                 ▼                   ▼                   ▼
          ┌───────────┐      ┌──────────────┐     ┌───────────┐
          │ cli/auth  │      │ cli/channels │     │  cli/run  │
          │ (text log)│      │  (text log)  │     │ (json log)│
          └─────┬─────┘      └──────┬───────┘     └─────┬─────┘
                │                   │                   │
                └───────────────────┼───────────────────┘
                                    ▼
┌──────────────────────────────────────────────────────────────────────┐
│                         collector layer                                │
│  Collector (orchestrator)   MessageHandler (Adapter)                   │
│  - dbWriter goroutine        - []byte → storage.RawMessage             │
│  - backfill history          Classifier (Strategy → NoopClassifier)    │
└───────────────────────────────────┬────────────────────────────────────┘
                                     ▼
┌──────────────────────────────────────────────────────────────────────┐
│                    telegram layer (Facade)                             │
│  TelegramClient (interface)   Dispatcher (Observer)                    │
│  Client (impl, owns gotd/td)  PeerStore (RWMutex)                      │
│  TursoSessionStorage          terminalAuthenticator (auth.UserAuth.)   │
│                          │                                             │
│                   ┌──────┴───────┐                                     │
│                   │  gotd/td      │  ← ONLY contact point with MTProto  │
│                   └──────────────┘                                     │
└───────────────────────────────────┬────────────────────────────────────┘
                                     ▼
┌──────────────────────────────────────────────────────────────────────┐
│                        storage layer                                   │
│  db.go (Open/Close/Conn)   migrations.go (embed.FS)                    │
│  repository.go (ALL SQL, ? placeholders, prepared statements)          │
│                          │                                             │
│                   ┌──────┴───────┐                                     │
│                   │ Tursogo "turso"│  database/sql, no CGO              │
│                   │  ./limiar.db   │                                   │
│                   └──────────────┘                                     │
└──────────────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────────────┐
│       cross-cutting:  config (viper/env)  logger (slog)  errors        │
└──────────────────────────────────────────────────────────────────────┘
```

## Layer responsibilities

- **`cmd/limiar-collector/main.go`** — composition root. The `provider` struct
  implements `cli.Provider` and is the only place that constructs concrete
  `storage`, `telegram`, `logger`, and `collector` instances. `run()` calls
  `config.Load` then `config.Validate` before building the command tree.
- **`internal/cli`** — Cobra command tree (`NewRootCmd`, `newAuthCmd`,
  `newChannelsCmd`, `newRunCmd`). Depends only on the `Provider` interface; never
  constructs storage/telegram concretes. `auth`/`channels` use a text logger,
  `run` uses JSON.
- **`internal/collector`** — orchestration and adaptation. `Collector` runs the
  single `dbWriter` goroutine, performs first-run backfill, and drives
  `client.Run`. `MessageHandler` adapts raw update bytes into
  `storage.RawMessage`. `Classifier`/`NoopClassifier` is the pluggable Strategy.
- **`internal/telegram`** — the Facade over gotd/td. `TelegramClient` exposes
  `Auth`, `IsAuthenticated`, `AddUpdateHandler`, `ResolveChannel`,
  `FetchHistory`, `Run`. `Client` is the sole type importing gotd/td.
  `Dispatcher` fans updates out; `PeerStore` caches peers under an RWMutex;
  `TursoSessionStorage` satisfies gotd's `session.Storage`;
  `terminalAuthenticator` satisfies gotd's `auth.UserAuthenticator`;
  `encodeUpdate`/`extractMessages` serialize updates to JSON.
- **`internal/storage`** — all persistence. `DB` (open/close/conn), `migrate`
  (embedded `migrations/*.sql`), and `Repository` (every SQL statement).
- **`internal/config`**, **`internal/logger`**, **`internal/errors`** —
  cross-cutting infrastructure.

## Concurrency model (fan-out / fan-in)

```
                 Telegram MTProto
                       │  tg.UpdatesClass
                       ▼
              Client.onUpdate → encodeUpdate → []byte
                       │
                 Dispatcher.Dispatch
                       │  fan-out: one buffered chan + goroutine per handler
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
   consume(h1)    consume(h2)    consume(hN)     (recover() per goroutine)
        │              │              │
        ▼              ▼              ▼
   MessageHandler.HandleUpdate → Classify → writeCh <- WriteJob
        │              │              │
        └──────────────┼──────────────┘
                       │  fan-in: single chan WriteJob
                       ▼
              Collector.dbWriter  (the ONLY DB writer)
                       │  SaveRawMessage + UpdateChannelLastMessage
                       ▼
                  Tursogo DB (serialized writes)
```

Key properties:

- **Fan-out:** `Dispatcher` gives each registered `UpdateHandler` its own
  buffered channel (`DispatcherBufferSize`, default 256) drained by a dedicated
  goroutine, so a slow handler cannot block its siblings.
- **Panic isolation:** `Dispatcher.invoke` wraps each handler call with
  `recover()` and logs the recovered condition — a single bad update never
  crashes the process.
- **Fan-in:** all handlers send `WriteJob` values into one `writeCh`
  (`DBWriterBufferSize`, default 512). The single `dbWriter` goroutine is the
  only writer to `*sql.DB`, eliminating write contention.
- **Stateless handler:** `MessageHandler` holds no mutable state and acquires no
  locks; it is safe for concurrent invocation.
- **Peer cache:** `PeerStore` guards its `map[int64]*storage.Peer` with a
  `sync.RWMutex`; concurrent reads do not block one another.
- **Graceful shutdown:** `cli/run` builds a `signal.NotifyContext` for
  `SIGTERM`/`SIGINT`. On cancellation, `Collector.shutdown` closes `writeCh` and
  `wg.Wait()`s for the writer to drain; `run` enforces `ShutdownTimeout` with a
  `time.After` race.

## Data model

Internal structs (`internal/storage/repository.go`):

```go
type RawMessage struct {
    ID            int64
    ChannelID     int64
    MessageID     int64
    Payload       []byte   // raw gotd/td update serialized as JSON
    ReceivedAt    time.Time
    SchemaVersion int      // 1 in Phase 1
}

type Channel struct {
    ID              int64
    Username        string
    Title           string
    Active          bool
    AddedAt         time.Time
    LastMessageID   int64
    LastCollectedAt time.Time
}

type Peer struct {
    ID         int64
    AccessHash int64
    Type       string  // "channel", "user", "chat"
    Username   string
    UpdatedAt  time.Time
}
```

SQL schema (`internal/storage/migrations/001_initial.sql`):

```sql
CREATE TABLE IF NOT EXISTS sessions (
    id         INTEGER PRIMARY KEY DEFAULT 1,
    data       BLOB NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK (id = 1)                          -- single-row constraint
);

CREATE TABLE IF NOT EXISTS peers (
    id          INTEGER PRIMARY KEY,
    access_hash INTEGER NOT NULL,
    type        TEXT NOT NULL CHECK (type IN ('channel', 'user', 'chat')),
    username    TEXT,
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS channels (
    id                INTEGER PRIMARY KEY,
    username          TEXT NOT NULL UNIQUE,
    title             TEXT NOT NULL DEFAULT '',
    active            INTEGER NOT NULL DEFAULT 1,
    added_at          TEXT NOT NULL DEFAULT (datetime('now')),
    last_message_id   INTEGER NOT NULL DEFAULT 0,
    last_collected_at TEXT
);

CREATE TABLE IF NOT EXISTS raw_messages (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id     INTEGER NOT NULL REFERENCES channels(id),
    message_id     INTEGER NOT NULL,
    payload        TEXT NOT NULL,           -- JSON
    received_at    TEXT NOT NULL DEFAULT (datetime('now')),
    schema_version INTEGER NOT NULL DEFAULT 1,
    UNIQUE(channel_id, message_id)          -- safe-persistence dedup
);

CREATE INDEX IF NOT EXISTS idx_raw_messages_channel ON raw_messages(channel_id, message_id);
CREATE INDEX IF NOT EXISTS idx_channels_username    ON channels(username);
```

Datetime columns are stored as text in the layout `2006-01-02 15:04:05` (UTC),
matching the schema's `datetime('now')` defaults; `Repository.parseDBTime`
reads them back (with an RFC3339 fallback).

## Build / dependency order

```
errors → config → logger → storage
       → telegram/{session, peers, auth, dispatcher} → telegram/client
       → collector/{classifier, handler, collector}
       → cli/{root, auth, channels, run}
       → cmd/limiar-collector/main.go
```

The graph is acyclic; gotd/td is confined to `internal/telegram`, and
`database/sql` to `internal/storage`.

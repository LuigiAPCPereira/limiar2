# AGENTS.md — Guidance for AI Agents and Contributors

This file defines the hard rules for working on `limiar-collector`. Read it
before touching any code. The constraints here are not stylistic preferences —
they are the contract that keeps the codebase modular, testable, and decoupled
from third-party internals.

Module path: `github.com/limiar/collector`.

## Scope: Phase 1 is collection only

`limiar-collector` (Phase 1) connects to Telegram, authenticates as a userbot,
manages monitored channels, and persists **raw** message payloads as JSON.

**In scope:** `auth`, `channels`, `run`; session/peer persistence; raw capture;
first-run history backfill; graceful shutdown; connection resilience.

**Out of scope (do NOT implement):** normalization, enrichment, semantic
classification, LLM calls, deduplication beyond safe persistence, HTTP/REST/SSE,
health checks, metrics. These belong to `limiar-processor` and `limiar-api`.

If a change introduces processing logic, stop — it is a different phase.

## The closed stack

Only these dependencies are permitted. **Never add a dependency outside this
list.**

| Concern | Allowed | Forbidden |
|---------|---------|-----------|
| MTProto | `github.com/gotd/td` | GoTGProto or any other wrapper |
| Database | `turso.tech/database/tursogo` (driver `turso`) | SQLite drivers, `mattn`, GORM, any ORM |
| CLI/config | `github.com/spf13/cobra`, `github.com/spf13/viper` | — |
| Logging | stdlib `log/slog` (behind `logger.Logger`) | zerolog, zap, logrus |
| Property tests | `pgregory.net/rapid` (test files only) | — |

`pgregory.net/rapid` is the only dependency allowed outside the runtime stack,
and only inside `_test.go` files.

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
   WriteJob`). No other goroutine writes.
5. **`context.Context` is the first argument of every I/O method.** See
   `Repository`, `TelegramClient`, `UpdateHandler`, etc.
6. **Errors crossing a layer boundary are wrapped via `errors.Wrap(layer, op,
   err)`** so messages read `"layer: op: cause"` and sentinel identity is
   preserved for `errors.Is`. Sentinels live in `internal/errors/errors.go`
   (`ErrNotAuthenticated`, `ErrChannelNotFound`, `ErrSessionCorrupted`,
   `ErrDBWriteFailed`, `ErrMaxRetriesExceeded`).
7. **gotd/td types never leak past `internal/telegram`.** The CLI and collector
   layers import only the `TelegramClient` facade and the domain models in
   `internal/storage`. Updates leave the telegram package as `[]byte` JSON.
8. **All SQL lives in `internal/storage/repository.go`.** Placeholders are `?`
   only. Recurring writes use prepared statements.
9. **Concrete dependencies are constructed only in
   `cmd/limiar-collector/main.go`** (the composition root). The CLI depends on
   the `cli.Provider` interface and never constructs storage or telegram
   concretes itself. Manual DI — no DI framework.

## Build / dependency order

Packages form an acyclic graph. Build bottom-up:

```
errors
config
logger            (← config)
storage           (← config, logger, errors)
telegram/session  (← storage, logger, errors)
telegram/peers    (← storage, logger)
telegram/auth     (← logger)
telegram/dispatcher (← logger)
telegram/client   (← config values, session, peers, auth, dispatcher, logger, errors)
collector/classifier
collector/handler (← classifier, storage, logger, errors)
collector/collector (← telegram, storage, logger, errors)
cli/{root,auth,channels,run} (← Provider interface)
cmd/limiar-collector/main.go (← everything; wires it all)
```

## Quality gates

```sh
go build ./...        # exit 0
go vet ./...          # zero issues
go test ./...         # all pass
go test -race ./...   # no data races
```

The build must exclude any import of `sqlite`, `mattn`, or `gorm`.

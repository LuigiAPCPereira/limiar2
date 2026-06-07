# limiar-collector

Phase 1 of **Limiar**, a pipeline that monitors Brazilian Telegram promo
channels via an MTProto userbot and persists **raw** messages as JSON for later
processing.

`limiar-collector` does one job: connect to Telegram, authenticate as a userbot,
manage the list of monitored channels, and capture raw message payloads into an
embedded database. There is **zero** processing in this phase — no
normalization, classification, enrichment, or deduplication beyond safe
persistence. The goal of Phase 1 is to discover the real shape of Telegram's
data so that later phases (`limiar-processor`, `limiar-api`) can build on it.

## What it is

- A standalone Go binary (`cmd/limiar-collector`, module `github.com/limiar/collector`).
- An MTProto userbot built directly on [`gotd/td`](https://github.com/gotd/td) (no third-party wrapper).
- A raw-message collector that writes JSON payloads to an embedded
  [Tursogo](https://turso.tech) database (`turso` driver via `database/sql`, **no CGO**).

## What it is NOT

- Not a Telegram bot (it does not reply to messages or interact with users).
- Not an HTTP scraper (it speaks native MTProto).
- Not an alerting system (it is a data pipeline stage).
- Not a processor or API — normalization, classification, dedup, LLM calls,
  REST, and SSE all belong to future phases and are explicitly out of scope.

## No CGO required

Tursogo uses `purego` for FFI, so the binary builds and runs without a C
toolchain and without `CGO_ENABLED=1`. No SQLite driver, no GORM, no ORM.

## Install

```sh
go build ./...
# or build just the binary:
go build -o limiar-collector ./cmd/limiar-collector
```

## Commands

The binary exposes three subcommands. `auth` and `channels` log in **text**
format (interactive); `run` logs in **JSON** format (production service).

### `auth` — authenticate once and persist the session

Interactive, idempotent. Prompts for phone number, then login code, then the
2FA password if the account has two-factor enabled. The session is persisted to
the `sessions` table (single row). Running `auth` again when a valid session
already exists is a no-op.

```sh
LIMIAR_APP_ID=12345 LIMIAR_API_HASH=abcdef... ./limiar-collector auth
# Phone number (international format, e.g. +5511999999999): +55...
# Login code: 12345
# 2FA password: ****        (only if two-factor is enabled)
```

### `channels` — manage the monitored channel list

Requires a valid session (otherwise returns `ErrNotAuthenticated`).

```sh
./limiar-collector channels list
./limiar-collector channels add <username>      # resolves @username via MTProto, persists it
./limiar-collector channels remove <username>   # ErrChannelNotFound if absent
```

### `run` — start the collector service

Non-interactive, production-ready. Requires a valid session. Backfills the 20
most recent messages on a channel's first run, then captures live messages and
persists each raw JSON payload. Shuts down gracefully on `SIGTERM`/`SIGINT`,
draining in-flight writes within `LIMIAR_SHUTDOWN_TIMEOUT` seconds.

```sh
LIMIAR_APP_ID=12345 LIMIAR_API_HASH=abcdef... ./limiar-collector run
```

## Configuration (environment variables)

All configuration is read from `LIMIAR_`-prefixed environment variables via
Viper. `Load()` populates the struct and applies defaults; `Validate()` is
called explicitly before any I/O and reports every invalid field at once.

| Variable | Field | Default | Valid range / values |
|----------|-------|---------|----------------------|
| `LIMIAR_APP_ID` | `AppID` | — (required) | non-zero integer |
| `LIMIAR_API_HASH` | `APIHash` | — (required) | non-empty string (masked in logs) |
| `LIMIAR_DB_PATH` | `DBPath` | `./limiar.db` | any valid path |
| `LIMIAR_LOG_LEVEL` | `LogLevel` | `info` | `debug`, `info`, `warn`, `error` |
| `LIMIAR_LOG_FORMAT` | `LogFormat` | `json` | `json`, `text` |
| `LIMIAR_SHUTDOWN_TIMEOUT` | `ShutdownTimeout` | `15` | 1–300 (seconds) |
| `LIMIAR_MAX_RETRIES` | `MaxRetries` | `10` | positive integer |
| `LIMIAR_IO_TIMEOUT` | `IOTimeout` | `30s` | Go duration |
| `LIMIAR_DISPATCHER_BUFFER_SIZE` | `DispatcherBufferSize` | `256` | 64–4096 |
| `LIMIAR_DB_WRITER_BUFFER_SIZE` | `DBWriterBufferSize` | `512` | 128–8192 |

`AppID` and `APIHash` are required and never defaulted; a missing value for
either produces a validation error. The `APIHash` value is masked by
`Config.String()` and redacted from all log output.

## Stack

| Concern | Technology |
|---------|-----------|
| MTProto | `gotd/td` (no wrapper) |
| Database | `turso.tech/database/tursogo` (driver `turso`, no CGO) |
| CLI / config | `cobra` + `viper` |
| Logging | `log/slog` behind a `Logger` interface |
| Property tests | `pgregory.net/rapid` (test files only) |

## Documentation

- `docs/CONTEXT.md` — system overview, scope, phase roadmap
- `docs/ARCHITECTURE.md` — pipeline diagram, concurrency model, data model
- `docs/specs/GOTD-TD.md`, `docs/specs/TURSOGO.md` — library usage specs
- `docs/guidelines/` — extension guidelines for storage, telegram, collector
- `docs/adr/` — architecture decision records
- `AGENTS.md` — rules for AI agents and contributors

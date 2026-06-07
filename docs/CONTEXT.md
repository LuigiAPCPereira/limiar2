# CONTEXT — Limiar System Overview

## What Limiar is

Limiar is a system that monitors Brazilian Telegram promo channels via an
MTProto userbot, then normalizes, deduplicates, classifies, and serves a
standardized JSON feed to a real-time promotions website. Telegram promo
channels are rich but chaotic: duplicate messages across channels, inconsistent
formats, no categorization. Limiar turns that noise into clean, structured data.

The full system is a pipeline of independent Go binaries:

```
Telegram MTProto
       ↓
limiar-collector → connects via userbot, reads history + live events, persists RAW
       ↓
limiar-processor → normalizes, deduplicates, classifies, filters   (future)
       ↓
limiar-api       → serves REST (paginated history) + SSE (live push) (future)
       ↓
Limiar Frontend  → live promotions site                             (future)
```

This repository implements **`limiar-collector` only (Phase 1)**.

## What this repository (Phase 1) does

`limiar-collector` connects to Telegram, authenticates as a userbot once,
manages a list of monitored channels, and captures **raw** message payloads as
JSON into an embedded Tursogo database. Phase 1 exists to discover the real
shape of Telegram's data before any processing is designed.

Three CLI subcommands:

- `auth` — interactive, idempotent authentication; persists a session (text logs).
- `channels` — `list` / `add <username>` / `remove <username>`; requires a
  session (text logs).
- `run` — non-interactive collector service; captures and persists raw
  messages, graceful shutdown on `SIGTERM`/`SIGINT` (JSON logs).

## What Limiar is NOT

- Not a Telegram bot — it never replies to messages or interacts with users.
- Not an HTTP scraper — it speaks native MTProto.
- Not an alerting system — it is a data pipeline.
- Not an admin UI — configuration is via CLI and environment variables.

And specifically, **Phase 1 is NOT**:

- a processor (no normalization, enrichment, semantic classification, LLM calls);
- a deduplicator beyond safe persistence (the `UNIQUE(channel_id, message_id)`
  constraint plus `ON CONFLICT DO NOTHING`);
- an HTTP service (no REST, SSE, WebSocket, health checks, or metrics).

## Data flow (Phase 1)

```
Telegram (MTProto)
   │  updates / history
   ▼
telegram.Client (gotd/td facade)
   │  encodeUpdate → JSON []byte
   ▼
telegram.Dispatcher (Observer, fan-out, goroutine per handler)
   │  HandleUpdate(ctx, update []byte)
   ▼
collector.MessageHandler (Adapter: []byte → storage.RawMessage)
   │  Classifier.Classify (NoopClassifier pass-through)
   │  writeCh <- WriteJob
   ▼
collector.Collector.dbWriter (single goroutine, fan-in)
   │  Repository.SaveRawMessage + UpdateChannelLastMessage
   ▼
Tursogo database (./limiar.db): raw_messages, channels, peers, sessions
```

Session state and peer access hashes are persisted in the same database, so the
collector reconnects without re-authenticating and resolves channels it has
seen before.

## Phase roadmap

| Phase | Binary | What it adds |
|-------|--------|--------------|
| **1 (this repo)** | `limiar-collector` | userbot auth, channel management, raw capture |
| 2 | `limiar-processor` | normalization, dedup, rule-based classification |
| 3 | `limiar-processor` | semantic classification via batched LLM API |
| 4 | `limiar-api` | REST + SSE, frontend integration |
| 5 | `limiar-collector` | automatic channel discovery |
| 6 | `limiar` | orchestrator that spawns all binaries (`limiar run`) |

The design plants seams for later phases without implementing them: the
`Classifier` Strategy interface (currently `NoopClassifier`), the `Dispatcher`
Observer (currently one handler), and a stable raw-payload schema
(`schema_version = 1`).

# ADR 003 — Single DBWriter Goroutine via Fan-In

## Status

Accepted

## Context

Channels are monitored concurrently and updates are fanned out to handlers in
their own goroutines (Observer pattern, `Dispatcher`). If each handler wrote to
the database independently, multiple goroutines would write to the same
`*sql.DB` concurrently, risking write contention and lock errors on an embedded
store, and making write ordering and error handling hard to reason about.

## Decision

Serialize all database writes through a single DBWriter goroutine
(`Collector.dbWriter`). Producers (the stateless `MessageHandler` and the
first-run backfill) send `WriteJob` values into one buffered channel
(`writeCh`, sized by `DBWriterBufferSize`, default 512). The DBWriter drains the
channel and is the **only** component that writes to `*sql.DB`. On shutdown,
`Collector.shutdown` closes `writeCh` and `wg.Wait()`s for the writer to finish
draining.

`storage.DB.Conn()` documents this invariant: only the DBWriter may issue writes
through the returned connection.

## Consequences

- No concurrent writers; write contention is eliminated by construction.
- Writes are naturally ordered and have one place for retry/error policy
  (`writeWithRetry`, bounded by `maxWriteRetry`, logging lost messages).
- The hot path is lock-free: handlers are stateless and merely enqueue.
- Throughput is bounded by one writer; acceptable for Phase 1 volumes and
  tunable via the buffer size. A persistently slow DB will eventually exert
  backpressure through the buffered channel.

## Alternatives considered

- **Per-handler writes** — concurrent writers to one `*sql.DB`; rejected for
  contention and ordering complexity.
- **A write mutex around `Repository`** — serializes writes but spreads write
  call sites across goroutines, complicating retry/shutdown; the channel fan-in
  is cleaner and gives a single drain point.
- **A pool of writers** — unnecessary for an embedded single-file DB and
  reintroduces concurrency on the connection.

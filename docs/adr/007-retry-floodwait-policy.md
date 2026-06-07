# ADR 007 — Retry, Backoff, and Flood-Wait Policy

## Status

Accepted

## Context

A long-running collector must survive transient failures without manual
intervention: Telegram connections drop, and Telegram rate-limits clients with
`FLOOD_WAIT` signals that specify an exact wait duration. Database writes can
also fail transiently. We need a resilience policy that recovers automatically,
bounds its effort, and respects server-mandated waits, while still terminating
cleanly on context cancellation.

## Decision

**Connection backoff.** On connection loss, `Client.Run` reconnects using
exponential backoff via `CalculateBackoff`:

- base delay **1s**, multiplier **2x**, **10%** jitter, ceiling **5m**;
- bounded by `MaxRetries` — once attempts are exhausted, `CalculateBackoff`
  returns `ErrMaxRetriesExceeded` and `Run` stops reconnecting;
- the attempt counter resets to 0 on each successful connect;
- the backoff sleep selects on `ctx.Done()`, so cancellation wins immediately.

`DefaultBackoff(maxRetries)` constructs this policy; it is wired in
`main.go`'s `NewClient`.

**Flood-wait.** When Telegram signals `FLOOD_WAIT`, the client waits exactly the
duration the server specifies before retrying (no jitter applied to a
server-mandated wait).

**DB writes.** The DBWriter retries each `WriteJob` up to `maxWriteRetry` times
(`writeWithRetry`); on persistent failure it logs the lost message explicitly
and, if an `ErrCh` is present, reports an error satisfying
`errors.Is(err, ErrDBWriteFailed)`.

## Consequences

- The collector self-heals from dropped connections and rate limits without
  operator action.
- Jitter avoids thundering-herd reconnect storms; the 5m ceiling caps backoff.
- Bounded retries prevent infinite spinning; exhaustion is an explicit,
  inspectable error.
- Respecting the exact flood-wait duration keeps the userbot compliant and
  avoids escalating bans.
- All waits honor context, so shutdown stays within `ShutdownTimeout`.

## Alternatives considered

- **Fixed-interval retry** — simpler but prone to synchronized reconnect storms
  and ignores flood-wait semantics. Rejected.
- **Unbounded retries** — could spin forever on a permanent failure; rejected in
  favor of `MaxRetries`.
- **Ignoring the server's flood-wait duration** (using our own backoff instead) —
  risks bans; rejected.

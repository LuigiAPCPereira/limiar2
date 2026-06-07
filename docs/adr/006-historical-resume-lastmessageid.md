# ADR 006 — First-Run Backfill + Resume from LastMessageID

## Status

Accepted

## Context

When the collector starts monitoring a channel, two situations exist: the
channel has never been collected (no cursor), or it was collected before and we
want to avoid reprocessing what we already have. We need a deterministic cursor
to resume from, and on a brand-new channel we want some immediate history so the
downstream pipeline has data to work with rather than waiting for the next live
message.

The `channels` table carries `last_message_id` (default 0) and
`last_collected_at` as the per-channel cursor.

## Decision

On `run`, the `Collector` distinguishes first run from resume by
`Channel.LastMessageID`:

- **First run (`LastMessageID == 0`)** — `backfill` calls
  `client.FetchHistory(ctx, channelID, accessHash, historyBatchSize)` with
  `historyBatchSize = 20`, enqueues each payload as a `WriteJob`, and advances
  the channel cursor to the newest fetched message id before live capture
  begins.
- **Resume (`LastMessageID > 0`)** — `backfill` skips the channel; live capture
  continues from the existing cursor, and the DBWriter advances the cursor as new
  messages persist (`UpdateChannelLastMessage`). The
  `UNIQUE(channel_id, message_id)` constraint plus `ON CONFLICT DO NOTHING`
  guards against duplicates.

Backfill failure is logged but not fatal — live capture still starts.

## Consequences

- New channels yield immediate data (20 recent messages) without waiting for
  live traffic.
- Restarts do not reprocess history; the cursor + unique constraint make
  persistence idempotent.
- The 20-message batch is a fixed Phase 1 constant; deeper history would need
  pagination (future work).
- Inactive channels (`Active == false`) are skipped during backfill.

## Alternatives considered

- **No backfill** — new channels would sit empty until the next live message;
  poor for testing the downstream pipeline. Rejected.
- **Full history backfill** — expensive, rate-limit-prone, and unnecessary for
  shape discovery. Rejected for Phase 1.
- **Timestamp-based cursor** — message id is the natural, monotonic Telegram
  cursor and avoids clock-skew ambiguity.

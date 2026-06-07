# Guideline — Extending the Collector Layer

The collector layer (`internal/collector`) adapts raw Telegram updates into
`storage.RawMessage` records and persists them through a single DBWriter
goroutine. Phase 1 does capture only — no enrichment. Follow these rules.

## Rules

1. **`MessageHandler` stays stateless.** It holds only its `classifier`,
   `writeCh`, and `log`, acquires no locks, and is safe for concurrent
   invocation. Do not add mutable fields or per-channel state.
2. **Only the DBWriter writes.** `Collector.dbWriter` is the single goroutine
   that calls `Repository.SaveRawMessage` / `UpdateChannelLastMessage` (fan-in
   via `writeCh chan WriteJob`). Handlers and backfill **enqueue** jobs; they
   never write to the DB directly.
3. **The classifier is pluggable.** Go through the `Classifier` Strategy
   interface. Phase 1 ships `NoopClassifier` (pass-through, identity). Later
   phases swap in rule/LLM classifiers without touching the pipeline.
4. **No enrichment in Phase 1.** The handler preserves the original payload
   bytes verbatim (`Payload: update`) and stamps `SchemaVersion = 1`. No
   normalization, dedup beyond safe persistence, or semantic processing.
5. **Lost writes are logged, never silently dropped.** `writeWithRetry` retries
   up to `maxWriteRetry`, then logs an explicit error (and notifies `ErrCh` if
   set) — see Requirement 3.10.
6. **Context-first and cancellation-aware.** Sends to `writeCh` select on
   `ctx.Done()`; `shutdown` closes `writeCh` and `wg.Wait()`s the writer.
7. **Backfill is first-run only.** `backfill` skips channels with
   `LastMessageID > 0` (resume mode) and fetches `historyBatchSize` (20) for
   first-run channels — see ADR 006.
8. **Depend on the `Repository` interface**, not the concrete struct, so the
   collector stays testable with a fake.

## Correct

```go
// Stateless handler: adapt, classify via Strategy, enqueue — never write.
func (h *MessageHandler) HandleUpdate(ctx context.Context, update []byte) error {
    msg := &storage.RawMessage{
        Payload:       update,        // preserved verbatim
        ReceivedAt:    time.Now().UTC(),
        SchemaVersion: schemaVersion, // 1
        // ChannelID / MessageID from the envelope projection
    }
    classified, err := h.classifier.Classify(ctx, msg)
    if err != nil {
        return apperrors.Wrap("collector", "classify", err)
    }
    select {
    case h.writeCh <- WriteJob{Message: classified}:
        return nil
    case <-ctx.Done():
        return apperrors.Wrap("collector", "handle_update", ctx.Err())
    }
}
```

```go
// A future classifier plugs in without changing the handler or DBWriter.
type RuleClassifier struct{ /* rules */ }
func (c RuleClassifier) Classify(ctx context.Context, raw *storage.RawMessage) (*storage.RawMessage, error) {
    // Phase 2+: return a transformed copy. Phase 1 stays Noop.
    return raw, nil
}
```

## Incorrect

```go
// WRONG: handler writes to the DB directly, breaking the single-writer invariant.
func (h *MessageHandler) HandleUpdate(ctx context.Context, update []byte) error {
    return h.repo.SaveRawMessage(ctx, adapt(update)) // must enqueue to writeCh instead
}
```

```go
// WRONG: enrichment in Phase 1 (mutating/normalizing the raw payload).
msg.Payload = normalize(update)   // Phase 1 must store raw bytes unchanged
```

```go
// WRONG: spawning extra DB-writing goroutines (fan-out to multiple writers).
for _, job := range jobs {
    go c.repo.SaveRawMessage(ctx, job.Message)  // only dbWriter may write
}
```

```go
// WRONG: silently dropping a message that failed to persist.
if err := c.repo.SaveRawMessage(ctx, msg); err != nil {
    return // lost without a log — violates Requirement 3.10
}
```

## Never do

- Write to the database from anywhere but `Collector.dbWriter` (ADR 003).
- Add normalization, dedup, classification, or LLM calls in Phase 1.
- Make `MessageHandler` stateful or lock-dependent.
- Drop a failed write without logging the lost message.

# Guideline — Extending the Telegram Layer

The telegram layer (`internal/telegram`) is the **Facade** over gotd/td. It is
the only package allowed to import `github.com/gotd/td/...`. Everything it
exposes to the rest of the codebase is a domain type (`storage.Peer`,
`HistoryMessage`) or plain `[]byte`. Follow these rules when extending it.

## Rules

1. **Keep gotd/td behind the facade.** New MTProto capabilities are added as
   methods on the `TelegramClient` interface and implemented on `Client`. The
   interface must not expose any `tg.*`, `auth.*`, or `session.*` type.
2. **`Client` is the only gotd-importing type.** Helpers (`encodeUpdate`,
   `extractMessages`, `firstChannel`) may use `tg` types internally but must
   return domain types or `[]byte`.
3. **Never import gotd/td in `cli` or `collector`.** Those layers depend only on
   `TelegramClient` and `storage` models.
4. **One-shot API calls run inside `runOnce`** (`c.tg.Run(ctx, f)`); the live
   loop runs in `Run`. Both honor context cancellation.
5. **Updates leave the package as JSON.** `onUpdate` serializes via
   `encodeUpdate` and hands bytes to the `Dispatcher`; it must never block or
   crash the receive loop (encode errors are logged and swallowed).
6. **Reconnection uses `CalculateBackoff`** (1s base, 2x, 10% jitter, 5m
   ceiling, bounded by `MaxRetries`) — see ADR 007.
7. **Session storage maps "absent" to gotd's `session.ErrNotFound`** so the auth
   flow starts fresh; corruption surfaces as `ErrSessionCorrupted`.
8. **PeerStore mutations go through its RWMutex methods** (`Get`/`Set`/
   `LoadFromDB`/`FlushToDB`); never touch the map directly.

## Correct

```go
// New facade method: domain types only, context-first, wrapped errors,
// network call inside runOnce.
func (c *Client) GetChannelFull(ctx context.Context, channelID, accessHash int64) ([]byte, error) {
    var payload []byte
    err := c.runOnce(ctx, func(ctx context.Context) error {
        res, err := c.tg.API().ChannelsGetFullChannel(ctx, &tg.InputChannel{
            ChannelID: channelID, AccessHash: accessHash,
        })
        if err != nil {
            return apperrors.Wrap("telegram", "get_full_channel", err)
        }
        payload, err = json.Marshal(res)        // gotd type stays internal
        return err
    })
    return payload, err
}
```

## Incorrect

```go
// WRONG: leaking a gotd type through the interface.
type TelegramClient interface {
    Resolve(ctx context.Context, username string) (*tg.Channel, error) // exposes tg.*
}
```

```go
// WRONG: importing gotd in the collector layer.
package collector
import "github.com/gotd/td/tg"   // forbidden outside internal/telegram
```

```go
// WRONG: handling an update synchronously in onUpdate, bypassing the dispatcher.
func (c *Client) onUpdate(ctx context.Context, u tg.UpdatesClass) error {
    return c.repo.SaveRawMessage(ctx, adapt(u))  // no fan-out, no recover()
}
```

## Never do

- Add a third-party MTProto wrapper such as GoTGProto (ADR 002).
- Return or accept gotd/td types across the `TelegramClient` boundary.
- Write to the database from this layer — persistence is the storage/collector
  job; telegram only reads/writes session and peer rows via `Repository`.
- Call `panic()`; the only `recover()` in the system is the Dispatcher's.

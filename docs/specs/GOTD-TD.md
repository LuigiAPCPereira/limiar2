# Spec — gotd/td Usage

`github.com/gotd/td` is the MTProto implementation `limiar-collector` builds on.
It is used **directly**, with no third-party wrapper. Everything gotd-specific is
confined to `internal/telegram`; the `TelegramClient` facade exposes only domain
types so the rest of the codebase never imports gotd/td.

## How gotd/td is used

### Client construction (`telegram/client.go`)

`Client` is the sole type that imports gotd/td. The gotd client is built in
`NewClient` with our session storage and our dispatcher wired in as the update
handler; no network I/O occurs until `Run` or an action method is called:

```go
c.tg = telegram.NewClient(appID, appHash, telegram.Options{
    SessionStorage: session,                                  // *TursoSessionStorage
    UpdateHandler:  telegram.UpdateHandlerFunc(c.onUpdate),   // forwards to Dispatcher
})
```

### Session storage (`session.Storage`)

`TursoSessionStorage` satisfies gotd's `github.com/gotd/td/session` `Storage`
interface (compile-time asserted via `var _ gotdsession.Storage =
(*TursoSessionStorage)(nil)`):

- `LoadSession(ctx) ([]byte, error)` — returns persisted bytes, mapping our
  `storage.ErrNoSession` to gotd's `session.ErrNotFound` so the auth flow treats
  "no session" as "start fresh" rather than a hard failure.
- `StoreSession(ctx, data) error` — persists session bytes via the repository.

### Auth flow (`auth.Flow`, `auth.UserAuthenticator`)

`Client.Auth` runs gotd's flow only if not already authorized:

```go
authn := newTerminalAuthenticator(os.Stdin, os.Stdout, c.log)
flow := auth.NewFlow(authn, auth.SendCodeOptions{})
err := c.tg.Auth().IfNecessary(ctx, flow)
```

`terminalAuthenticator` satisfies `github.com/gotd/td/telegram/auth`
`UserAuthenticator`: `Phone`, `Code`, `Password` (the 2FA step), plus
`AcceptTermsOfService`/`SignUp` which reject sign-up — the userbot account must
already exist. `IsAuthenticated` calls `c.tg.Auth().Status(ctx)` and reports
`st.Authorized`.

### Update handling (`UpdateHandler`)

`Client.onUpdate(ctx, u tg.UpdatesClass)` is the gotd entrypoint. It serializes
the update with `encodeUpdate` and hands the JSON bytes to `Dispatcher.Dispatch`.
A serialization error is logged and swallowed (`return nil`) so a single bad
update never crashes the receive loop. Serialization to `RawMessage` happens
later, in the collector's adapter.

### Resolving channels (`ContactsResolveUsername`)

`Client.ResolveChannel` normalizes the username (strips `@`) and calls
`c.tg.API().ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{...})`,
extracts the first `*tg.Channel` from the resolved chats, reads its access hash
via `GetAccessHash()`, and caches a `storage.Peer{Type: "channel", ...}`.

### History backfill (`MessagesGetHistory`)

`Client.FetchHistory` calls
`c.tg.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer:
&tg.InputPeerChannel{ChannelID, AccessHash}, Limit: limit})`. `extractMessages`
handles the `*tg.MessagesChannelMessages`, `*tg.MessagesMessages`, and
`*tg.MessagesMessagesSlice` response variants, serializing each `*tg.Message`
to JSON and skipping service messages. Each result is a `HistoryMessage{MessageID,
Payload}`.

### Serialization (`encode.go`)

gotd's `tg` types are plain Go structs with exported fields, so
`encoding/json.Marshal` captures the full raw shape. That is exactly what Phase 1
needs — see ADR 005.

## Patterns adopted

- **Facade:** all gotd complexity (session, peers, reconnection, auth) sits
  behind `TelegramClient`.
- **`runOnce` for one-shot actions:** `IsAuthenticated`, `Auth`,
  `ResolveChannel`, and `FetchHistory` run inside `c.tg.Run(ctx, f)` because auth
  status and API calls require the gotd client lifecycle to be active.
- **Reconnect loop in `Run`:** on connection loss, `Run` applies exponential
  backoff (`CalculateBackoff`) and resets the attempt counter on a successful
  connect; a cancelled context yields a clean shutdown.

## What NOT to use

- **No third-party MTProto wrapper** (e.g. GoTGProto). Its peer/session storage
  is a concrete struct coupled to GORM/SQLite and cannot plug into Tursogo — see
  ADR 002.
- **Do not leak gotd/td types past `internal/telegram`.** The `TelegramClient`
  interface returns only `storage.Peer`, `HistoryMessage`, and `[]byte`. The CLI
  and collector layers must not import `github.com/gotd/td/...`.
- **Do not handle updates synchronously in `onUpdate`.** Always dispatch through
  the `Dispatcher` so fan-out and panic recovery apply.
- **Do not support sign-up.** `terminalAuthenticator.SignUp` returns an error by
  design.

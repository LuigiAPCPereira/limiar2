# ADR 002 — Use gotd/td Directly, No MTProto Wrapper

## Status

Accepted

## Context

We need an MTProto client to operate as a Telegram userbot. A common choice is a
high-level wrapper such as GoTGProto, which bundles session and peer storage.
However, GoTGProto's `PeerStorage` (and session storage) is a concrete struct
coupled to GORM/SQLite — it is not a pluggable interface. Since `limiar-collector`
uses Tursogo as its single datastore (ADR 001), a wrapper that mandates
GORM/SQLite would force a second, incompatible persistence path.

`github.com/gotd/td` exposes the right seams: `session.Storage` for session
persistence and a flexible auth flow, letting us back both with Tursogo.

## Decision

Build directly on `gotd/td`. Implement our own session storage
(`TursoSessionStorage` satisfying `session.Storage`), peer cache (`PeerStore`),
authenticator (`terminalAuthenticator` satisfying `auth.UserAuthenticator`), and
dispatcher. All gotd/td usage is confined to `internal/telegram`, behind the
`TelegramClient` facade.

## Consequences

- Session, peers, channels, and messages all persist to the same Tursogo file —
  no GORM/SQLite dependency sneaks in.
- We own the session/peer/auth code, which is more code but full control over
  storage, reconnection, and serialization.
- gotd/td types are isolated to one package; the rest of the codebase stays
  decoupled (see ADR on facade boundary and `docs/specs/GOTD-TD.md`).
- We pin the gotd/td version and adapt only `session.go`/`client.go` if its
  interfaces change.

## Alternatives considered

- **GoTGProto** — its concrete GORM/SQLite-bound storage cannot plug into
  Tursogo; rejected.
- **Other high-level wrappers** — same risk of opinionated storage and hidden
  dependencies; rejected in favor of explicit control.
- **A different language/library** — out of scope; the pipeline is Go.

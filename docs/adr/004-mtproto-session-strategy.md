# ADR 004 — MTProto Session Persisted in Tursogo

## Status

Accepted

## Context

A userbot must authenticate once and reconnect later without re-entering
credentials. gotd/td abstracts session persistence behind the
`github.com/gotd/td/session` `Storage` interface
(`LoadSession`/`StoreSession`). We need somewhere to keep that session blob.
Per ADR 001, Tursogo is the single datastore, so the session should live there
rather than in a separate file.

## Decision

Implement `TursoSessionStorage` to satisfy gotd's `session.Storage`, backing it
with the Tursogo-backed `storage.Repository`. The session is stored as a single
row in the `sessions` table, enforced by `CHECK (id = 1)` so there is never more
than one session. `SaveSession` upserts (`ON CONFLICT(id) DO UPDATE`), making
re-authentication idempotent.

`LoadSession` maps the storage-level `ErrNoSession` to gotd's
`session.ErrNotFound`, which the auth flow treats as "start fresh." A session
that cannot be decoded surfaces as `ErrSessionCorrupted`.

## Consequences

- Session, peers, channels, and messages share one transactional file; backing
  up the `.db` backs up the auth state too.
- `auth` is idempotent: running it with a valid session is a no-op, and
  `SELECT COUNT(*) FROM sessions` stays 1 (Requirement 1.4/1.5).
- The wire format is whatever gotd serializes; we store the opaque bytes and
  never inspect them. Session bytes are treated as sensitive (redacted from
  logs).

## Alternatives considered

- **File-based session storage** (gotd's `session.FileStorage`) — a second
  persistence path outside Tursogo; rejected for ops simplicity and cohesion.
- **A multi-row session table** — unnecessary; a userbot has exactly one
  session, and the single-row `CHECK` makes idempotence trivial.
- **In-memory only** — would require re-authentication on every start;
  unacceptable for a service.

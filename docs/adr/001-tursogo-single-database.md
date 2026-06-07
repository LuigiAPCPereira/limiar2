# ADR 001 — Tursogo as the Single Datastore

## Status

Accepted

## Context

`limiar-collector` must persist several kinds of state: the MTProto session, a
peer/access-hash cache, the monitored-channel list, and raw message payloads. We
wanted one embedded datastore (a single `.db` file mountable as a persistent
volume) rather than juggling multiple stores, and we wanted a portable binary
with no C toolchain requirement.

`turso.tech/database/tursogo` provides an embedded database accessible through
the standard `database/sql` interface under the driver name `turso`, using
`purego` for FFI — so **no CGO** is needed.

## Decision

Use Tursogo as the single datastore for everything: `sessions`, `peers`,
`channels`, and `raw_messages` all live in one `.db` file
(`LIMIAR_DB_PATH`, default `./limiar.db`). It is accessed only through
`database/sql` from `internal/storage`, with `?` placeholders and prepared
statements. The driver is registered with a blank import
(`_ "turso.tech/database/tursogo"`) in `storage/db.go`.

## Consequences

- One file to back up, mount, and reason about; session + peer cache + channels +
  messages share a transactional store.
- No C toolchain, no `CGO_ENABLED`; the binary is portable.
- The storage layer stays on the portable `database/sql` surface (`Exec`,
  `Query`, `QueryRow`, `Prepare`), which keeps it testable.
- We accept a dependency on Tursogo's compatibility with standard `database/sql`
  semantics; the storage layer avoids driver-specific extensions to limit risk.

## Alternatives considered

- **SQLite via `mattn/go-sqlite3`** — requires CGO, defeating the portable-binary
  goal. Explicitly forbidden by the requirements.
- **An ORM (GORM)** — adds a heavy abstraction, hides SQL, and (in the case of
  GoTGProto's storage) couples to SQLite. Forbidden.
- **Multiple stores** (e.g. file-based session + separate DB) — more moving
  parts, no transactional cohesion, harder ops.

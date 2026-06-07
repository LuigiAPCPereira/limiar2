# Spec — Tursogo Usage

`turso.tech/database/tursogo` is the embedded database `limiar-collector` uses
for every kind of persisted state: the MTProto session, the peer cache, the
monitored-channel list, and raw message payloads — all in a single `.db` file.
It is accessed exclusively through `database/sql`, and only from
`internal/storage`.

## How Tursogo is used

### Driver registration (blank import)

The driver registers itself under the name `turso` via a blank import in
`internal/storage/db.go`:

```go
import (
    "database/sql"
    _ "turso.tech/database/tursogo" // registers the "turso" driver
)

const driverName = "turso"
```

### Opening the database (`db.go`)

`storage.Open(ctx, dbPath)` opens the connection with the standard library,
verifies it with `PingContext`, and applies migrations:

```go
conn, err := sql.Open(driverName, dbPath)   // dbPath default: ./limiar.db
if err := conn.PingContext(ctx); err != nil { ... }
if err := migrate(ctx, conn); err != nil { ... }
```

`DB.Conn()` exposes the underlying `*sql.DB`; only the DBWriter goroutine issues
writes through it. `DB.Close()` closes the connection.

### Migrations (`migrations.go`)

Schema lives in `internal/storage/migrations/*.sql`, embedded with
`//go:embed migrations/*.sql` into an `embed.FS`. `migrate` reads the directory,
sorts filenames lexically, and executes each file with `db.ExecContext`. The SQL
is idempotent (`CREATE TABLE IF NOT EXISTS`), so re-running is safe — there is no
separate version table in Phase 1.

### Queries (`repository.go`)

All SQL is centralized in `Repository`. It uses the standard `database/sql`
surface only — `ExecContext`, `QueryContext`, `QueryRowContext`, `Prepare` — with
`?` as the sole placeholder token. Recurring writes are prepared statements:

```go
stmtSaveMessage // INSERT INTO raw_messages (...) VALUES (?,?,?,?,?) ON CONFLICT(channel_id, message_id) DO NOTHING
stmtSavePeer    // INSERT INTO peers (...) VALUES (?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET ...
```

`SaveSession` upserts the single session row (`ON CONFLICT(id) DO UPDATE`);
`LoadSession` maps `sql.ErrNoRows` to `ErrNoSession`. Channel and peer CRUD use
the same patterns. Datetimes are stored as UTC text in layout
`2006-01-02 15:04:05` to match the schema's `datetime('now')` defaults.

### No CGO

Tursogo uses `purego` for FFI, so the binary builds and runs without a C
toolchain and without `CGO_ENABLED=1`. No special environment variables are
needed; the binary is portable.

## What to avoid

- **No SQLite driver, no GORM, no ORM.** The build must not import `sqlite`,
  `mattn`, or `gorm`. Tursogo is the single datastore — see ADR 001.
- **No concurrent writes to `*sql.DB`.** Only the single `Collector.dbWriter`
  goroutine writes (fan-in). Reads from other goroutines are fine; writes are
  serialized through the writer. See ADR 003.
- **No placeholder style other than `?`.** Do not use `$1`/`:name` styles.
- **No raw SQL outside `repository.go`.** Other packages call `Repository`
  methods; they never touch `database/sql` directly.
- **Stick to the standard `database/sql` surface.** Use `Query`, `Exec`,
  `QueryRow`, and `Prepare`; avoid driver-specific extensions so the layer stays
  portable and testable.

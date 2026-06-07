# Guideline — Extending the Storage Layer

The storage layer (`internal/storage`) owns **all** persistence against the
embedded Tursogo database. Every SQL statement lives here; no other package
touches `database/sql`. Follow these rules when extending it.

## Rules

1. **All SQL lives in `repository.go`.** Add new queries as `Repository`
   methods. Never write SQL in `telegram`, `collector`, or `cli`.
2. **Placeholders are `?` only.** Never use `$1` or named placeholders.
3. **Recurring writes use prepared statements** created in `NewRepository` and
   closed in `Close`.
4. **`context.Context` is the first argument** of every query method; use the
   `...Context` variants (`ExecContext`, `QueryContext`, `QueryRowContext`).
5. **Wrap errors** with `apperrors.Wrap("storage", "<op>", err)` and map
   "missing row" cases to a sentinel (e.g. `sql.ErrNoRows` → `ErrNoSession` or
   `apperrors.ErrChannelNotFound`).
6. **Only the DBWriter writes at runtime.** Repository write methods exist, but
   in the `run` service they are called solely from `Collector.dbWriter`.
7. **Schema changes go in a new migration file** under `migrations/`, kept
   idempotent (`CREATE ... IF NOT EXISTS`). Files run in lexical order.
8. **Datetimes are UTC text** in layout `2006-01-02 15:04:05` (the
   `dbTimeLayout` constant), matching the schema `datetime('now')` defaults.

## Correct

```go
// New query: centralized, ?-placeholders, context-first, wrapped error.
func (r *Repository) GetChannelByUsername(ctx context.Context, username string) (*Channel, error) {
    row := r.db.QueryRowContext(ctx, `
        SELECT id, username, title, active, added_at, last_message_id, last_collected_at
        FROM channels WHERE username = ?`, username)
    ch, err := scanChannel(row)
    if stderrors.Is(err, sql.ErrNoRows) {
        return nil, apperrors.Wrap("storage", "get_channel_by_username", apperrors.ErrChannelNotFound)
    }
    if err != nil {
        return nil, err
    }
    return ch, nil
}
```

```go
// Recurring write: prepared in NewRepository, closed in Close.
saveMsg, err := db.Prepare(`
    INSERT INTO raw_messages (channel_id, message_id, payload, received_at, schema_version)
    VALUES (?, ?, ?, ?, ?)
    ON CONFLICT(channel_id, message_id) DO NOTHING`)
```

## Incorrect

```go
// WRONG: SQL outside repository.go, in the collector or telegram layer.
rows, _ := someDB.Query("SELECT * FROM channels")   // never do this here
```

```go
// WRONG: non-? placeholder.
r.db.ExecContext(ctx, `INSERT INTO peers (id) VALUES ($1)`, id)
```

```go
// WRONG: importing a SQLite/ORM driver.
import _ "github.com/mattn/go-sqlite3"
import "gorm.io/gorm"
```

```go
// WRONG: a second goroutine writing to *sql.DB concurrently with the DBWriter.
go func() { repo.SaveRawMessage(ctx, msg) }()   // breaks single-writer invariant
```

## Never do

- Add SQLite, `mattn`, GORM, or any ORM dependency (ADR 001).
- Write to `*sql.DB` from anywhere but the single DBWriter goroutine (ADR 003).
- Leak `database/sql` types or raw SQL outside `internal/storage`.
- Introduce `init()` functions or package-level mutable state.

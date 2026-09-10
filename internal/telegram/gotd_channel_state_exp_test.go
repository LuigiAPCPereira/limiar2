package telegram_test

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"sort"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
)

const channelStateSchema = `
CREATE TABLE source_sync_channel_state (
    user_id    INTEGER NOT NULL,
    channel_id INTEGER NOT NULL,
    pts        INTEGER NOT NULL,
    PRIMARY KEY (user_id, channel_id)
) STRICT;
`

type channelStateKey struct {
	userID    int64
	channelID int64
}

type channelStateRow struct {
	channelID int64
	pts       int
}

type channelStateWrite struct {
	key channelStateKey
	pts int
}

func openChannelStateDB(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite3", "file:"+path)
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	return db
}

func setChannelPts(ctx context.Context, db *sql.DB, key channelStateKey, pts int) error {
	_, err := db.ExecContext(ctx, `INSERT INTO source_sync_channel_state(user_id, channel_id, pts)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id, channel_id) DO UPDATE SET pts=excluded.pts`, key.userID, key.channelID, pts)
	return err
}

func getChannelPts(ctx context.Context, db *sql.DB, key channelStateKey) (int, bool, error) {
	var pts int
	err := db.QueryRowContext(ctx, `SELECT pts FROM source_sync_channel_state WHERE user_id=? AND channel_id=?`, key.userID, key.channelID).Scan(&pts)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return pts, err == nil, err
}

func listChannelPts(ctx context.Context, db *sql.DB, userID int64) ([]channelStateRow, error) {
	rows, err := db.QueryContext(ctx, `SELECT channel_id, pts FROM source_sync_channel_state WHERE user_id=?`, userID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var got []channelStateRow
	for rows.Next() {
		var row channelStateRow
		if err := rows.Scan(&row.channelID, &row.pts); err != nil {
			return nil, err
		}
		got = append(got, row)
	}
	return got, rows.Err()
}

func requireAbsentChannelState(t *testing.T, ctx context.Context, db *sql.DB, key channelStateKey) {
	t.Helper()
	pts, ok, err := getChannelPts(ctx, db, key)
	if err != nil {
		t.Fatalf("get absent channel state: %v", err)
	}
	if ok || pts != 0 {
		t.Fatalf("absent channel state=(pts=%d ok=%v), want zero/false", pts, ok)
	}
}

func applyChannelWrites(t *testing.T, ctx context.Context, db *sql.DB, writes []channelStateWrite) {
	t.Helper()
	for _, write := range writes {
		if err := setChannelPts(ctx, db, write.key, write.pts); err != nil {
			t.Fatal(err)
		}
	}
}

func requireChannelState(t *testing.T, ctx context.Context, db *sql.DB, want channelStateWrite) {
	t.Helper()
	pts, ok, err := getChannelPts(ctx, db, want.key)
	if err != nil {
		t.Fatalf("channel state user=%d channel=%d: %v", want.key.userID, want.key.channelID, err)
	}
	if !ok || pts != want.pts {
		t.Fatalf("channel state user=%d channel=%d: pts=%d ok=%v, want pts=%d/true", want.key.userID, want.key.channelID, pts, ok, want.pts)
	}
}

func requireUserChannels(t *testing.T, ctx context.Context, db *sql.DB, userID int64, want []channelStateRow) {
	t.Helper()
	rows, err := listChannelPts(ctx, db, userID)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].channelID < rows[j].channelID })
	if len(rows) != len(want) {
		t.Fatalf("ForEach-equivalent user=%d rows=%v, want=%v", userID, rows, want)
	}
	for i := range want {
		if rows[i] != want[i] {
			t.Fatalf("ForEach-equivalent user=%d rows=%v, want=%v", userID, rows, want)
		}
	}
}

func TestChannelPtsCompositeAuthoritySurvivesReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "channel-state.db")
	db := openChannelStateDB(t, path)
	if _, err := db.Exec(channelStateSchema); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}

	const (
		userA    int64 = 101
		userB    int64 = 202
		channelX int64 = 1001
		channelY int64 = 1002
	)
	keyAX := channelStateKey{userID: userA, channelID: channelX}
	keyAY := channelStateKey{userID: userA, channelID: channelY}
	keyBX := channelStateKey{userID: userB, channelID: channelX}

	requireAbsentChannelState(t, ctx, db, keyAX)
	applyChannelWrites(t, ctx, db, []channelStateWrite{
		{key: keyAX, pts: 11},
		{key: keyAY, pts: 21},
		{key: keyBX, pts: 31},
		{key: keyAX, pts: 12},
	})

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openChannelStateDB(t, path)
	defer func() { _ = db.Close() }()

	for _, want := range []channelStateWrite{
		{key: keyAX, pts: 12},
		{key: keyAY, pts: 21},
		{key: keyBX, pts: 31},
	} {
		requireChannelState(t, ctx, db, want)
	}
	requireUserChannels(t, ctx, db, userA, []channelStateRow{
		{channelID: channelX, pts: 12},
		{channelID: channelY, pts: 21},
	})
}

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

type channelStateRow struct {
	channelID int64
	pts       int
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

func setChannelPts(ctx context.Context, db *sql.DB, userID, channelID int64, pts int) error {
	_, err := db.ExecContext(ctx, `INSERT INTO source_sync_channel_state(user_id, channel_id, pts)
		VALUES (?, ?, ?)
		ON CONFLICT(user_id, channel_id) DO UPDATE SET pts=excluded.pts`, userID, channelID, pts)
	return err
}

func getChannelPts(ctx context.Context, db *sql.DB, userID, channelID int64) (int, bool, error) {
	var pts int
	err := db.QueryRowContext(ctx, `SELECT pts FROM source_sync_channel_state WHERE user_id=? AND channel_id=?`, userID, channelID).Scan(&pts)
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

	if pts, ok, err := getChannelPts(ctx, db, userA, channelX); err != nil || ok || pts != 0 {
		_ = db.Close()
		t.Fatalf("absent channel state=(pts=%d ok=%v err=%v), want zero/false/nil", pts, ok, err)
	}

	for _, write := range []struct {
		userID, channelID int64
		pts               int
	}{
		{userA, channelX, 11},
		{userA, channelY, 21},
		{userB, channelX, 31},
		{userA, channelX, 12},
	} {
		if err := setChannelPts(ctx, db, write.userID, write.channelID, write.pts); err != nil {
			_ = db.Close()
			t.Fatal(err)
		}
	}

	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openChannelStateDB(t, path)
	defer func() { _ = db.Close() }()

	for _, want := range []struct {
		userID, channelID int64
		pts               int
	}{
		{userA, channelX, 12},
		{userA, channelY, 21},
		{userB, channelX, 31},
	} {
		pts, ok, err := getChannelPts(ctx, db, want.userID, want.channelID)
		if err != nil || !ok || pts != want.pts {
			t.Fatalf("channel state user=%d channel=%d: pts=%d ok=%v err=%v, want pts=%d/true/nil", want.userID, want.channelID, pts, ok, err, want.pts)
		}
	}

	rows, err := listChannelPts(ctx, db, userA)
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].channelID < rows[j].channelID })
	if len(rows) != 2 || rows[0] != (channelStateRow{channelID: channelX, pts: 12}) || rows[1] != (channelStateRow{channelID: channelY, pts: 21}) {
		t.Fatalf("ForEach-equivalent user A=%v, want [{%d 12} {%d 21}]", rows, channelX, channelY)
	}
}

package storage

import (
	stderrors "errors"
	"context"
	"database/sql"
	"time"

	apperrors "github.com/limiar/collector/internal/errors"
)

// ErrNoSession indicates no session row is present in the database.
var ErrNoSession = stderrors.New("no session stored")

// dbTimeLayout is the textual datetime format used by the schema's
// datetime('now') defaults.
const dbTimeLayout = "2006-01-02 15:04:05"

// RawMessage is a captured Telegram message payload awaiting downstream
// processing. Payload holds the raw gotd/td update serialized as JSON.
type RawMessage struct {
	ID            int64
	ChannelID     int64
	MessageID     int64
	Payload       []byte
	ReceivedAt    time.Time
	SchemaVersion int
}

// Channel is a monitored Telegram channel and its collection cursor.
type Channel struct {
	ID              int64
	Username        string
	Title           string
	Active          bool
	AddedAt         time.Time
	LastMessageID   int64
	LastCollectedAt time.Time
}

// Peer is a cached Telegram peer (channel, user, or chat) with its access hash.
type Peer struct {
	ID         int64
	AccessHash int64
	Type       string
	Username   string
	UpdatedAt  time.Time
}

// Repository centralizes every SQL statement against the Tursogo database.
// Recurring writes use prepared statements. All placeholders are ?.
type Repository struct {
	db *sql.DB

	stmtSaveMessage *sql.Stmt
	stmtSavePeer    *sql.Stmt
}

// NewRepository prepares recurring statements and returns a ready Repository.
func NewRepository(db *sql.DB) (*Repository, error) {
	saveMsg, err := db.Prepare(`
		INSERT INTO raw_messages (channel_id, message_id, payload, received_at, schema_version)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(channel_id, message_id) DO NOTHING`)
	if err != nil {
		return nil, apperrors.Wrap("storage", "prepare_save_message", err)
	}
	savePeer, err := db.Prepare(`
		INSERT INTO peers (id, access_hash, type, username, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			access_hash = excluded.access_hash,
			type        = excluded.type,
			username    = excluded.username,
			updated_at  = excluded.updated_at`)
	if err != nil {
		_ = saveMsg.Close()
		return nil, apperrors.Wrap("storage", "prepare_save_peer", err)
	}
	return &Repository{db: db, stmtSaveMessage: saveMsg, stmtSavePeer: savePeer}, nil
}

// Close releases all prepared statements.
func (r *Repository) Close() error {
	return stderrors.Join(r.stmtSaveMessage.Close(), r.stmtSavePeer.Close())
}

// --- Session ---

// SaveSession upserts the single session row (id is constrained to 1).
func (r *Repository) SaveSession(ctx context.Context, data []byte) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO sessions (id, data, updated_at) VALUES (1, ?, ?)
		ON CONFLICT(id) DO UPDATE SET data = excluded.data, updated_at = excluded.updated_at`,
		data, time.Now().UTC().Format(dbTimeLayout))
	if err != nil {
		return apperrors.Wrap("storage", "save_session", err)
	}
	return nil
}

// LoadSession returns the stored session bytes, or ErrNoSession if absent.
func (r *Repository) LoadSession(ctx context.Context) ([]byte, error) {
	var data []byte
	err := r.db.QueryRowContext(ctx, `SELECT data FROM sessions WHERE id = 1`).Scan(&data)
	if stderrors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, apperrors.Wrap("storage", "load_session", err)
	}
	return data, nil
}

// CountSessions returns the number of session rows (0 or 1).
func (r *Repository) CountSessions(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		return 0, apperrors.Wrap("storage", "count_sessions", err)
	}
	return n, nil
}

// --- Peers ---

// SavePeer inserts or updates a peer by id.
func (r *Repository) SavePeer(ctx context.Context, p *Peer) error {
	_, err := r.stmtSavePeer.ExecContext(ctx,
		p.ID, p.AccessHash, p.Type, nullString(p.Username),
		time.Now().UTC().Format(dbTimeLayout))
	if err != nil {
		return apperrors.Wrap("storage", "save_peer", err)
	}
	return nil
}

// LoadPeers returns all cached peers.
func (r *Repository) LoadPeers(ctx context.Context) ([]*Peer, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, access_hash, type, username, updated_at FROM peers`)
	if err != nil {
		return nil, apperrors.Wrap("storage", "load_peers", err)
	}
	defer rows.Close()

	var peers []*Peer
	for rows.Next() {
		var (
			p        Peer
			username sql.NullString
			updated  string
		)
		if err := rows.Scan(&p.ID, &p.AccessHash, &p.Type, &username, &updated); err != nil {
			return nil, apperrors.Wrap("storage", "scan_peer", err)
		}
		p.Username = username.String
		p.UpdatedAt = parseDBTime(updated)
		peers = append(peers, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap("storage", "iterate_peers", err)
	}
	return peers, nil
}

// --- Channels ---

// AddChannel inserts a monitored channel.
func (r *Repository) AddChannel(ctx context.Context, ch *Channel) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO channels (id, username, title, active)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(username) DO UPDATE SET title = excluded.title, active = excluded.active`,
		ch.ID, ch.Username, ch.Title, boolToInt(ch.Active))
	if err != nil {
		return apperrors.Wrap("storage", "add_channel", err)
	}
	return nil
}

// RemoveChannel deletes a channel by username, returning ErrChannelNotFound if
// no such channel exists.
func (r *Repository) RemoveChannel(ctx context.Context, username string) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM channels WHERE username = ?`, username)
	if err != nil {
		return apperrors.Wrap("storage", "remove_channel", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return apperrors.Wrap("storage", "remove_channel_rows", err)
	}
	if n == 0 {
		return apperrors.Wrap("storage", "remove_channel", apperrors.ErrChannelNotFound)
	}
	return nil
}

// ListChannels returns all monitored channels ordered by id.
func (r *Repository) ListChannels(ctx context.Context) ([]*Channel, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, username, title, active, added_at, last_message_id, last_collected_at
		FROM channels ORDER BY id`)
	if err != nil {
		return nil, apperrors.Wrap("storage", "list_channels", err)
	}
	defer rows.Close()

	var channels []*Channel
	for rows.Next() {
		ch, err := scanChannel(rows)
		if err != nil {
			return nil, err
		}
		channels = append(channels, ch)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap("storage", "iterate_channels", err)
	}
	return channels, nil
}

// GetChannel returns a single channel by id, or ErrChannelNotFound.
func (r *Repository) GetChannel(ctx context.Context, id int64) (*Channel, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, username, title, active, added_at, last_message_id, last_collected_at
		FROM channels WHERE id = ?`, id)
	ch, err := scanChannel(row)
	if stderrors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.Wrap("storage", "get_channel", apperrors.ErrChannelNotFound)
	}
	if err != nil {
		return nil, err
	}
	return ch, nil
}

// UpdateChannelLastMessage advances a channel's collection cursor.
func (r *Repository) UpdateChannelLastMessage(ctx context.Context, channelID, messageID int64, collectedAt time.Time) error {
	_, err := r.db.ExecContext(ctx, `
		UPDATE channels SET last_message_id = ?, last_collected_at = ? WHERE id = ?`,
		messageID, collectedAt.UTC().Format(dbTimeLayout), channelID)
	if err != nil {
		return apperrors.Wrap("storage", "update_channel_last_message", err)
	}
	return nil
}

// --- Raw Messages ---

// SaveRawMessage persists a captured message. Duplicate (channel_id,
// message_id) pairs are ignored (safe-persistence dedup).
func (r *Repository) SaveRawMessage(ctx context.Context, msg *RawMessage) error {
	_, err := r.stmtSaveMessage.ExecContext(ctx,
		msg.ChannelID, msg.MessageID, string(msg.Payload),
		msg.ReceivedAt.UTC().Format(dbTimeLayout), msg.SchemaVersion)
	if err != nil {
		return apperrors.Wrap("storage", "save_raw_message", err)
	}
	return nil
}

// CountRawMessages returns the total number of stored raw messages.
func (r *Repository) CountRawMessages(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM raw_messages`).Scan(&n); err != nil {
		return 0, apperrors.Wrap("storage", "count_raw_messages", err)
	}
	return n, nil
}

// ListMessages returns recent messages, optionally filtered by channelID.
// Pass 0 for channelID to list all channels. Results are ordered by received_at DESC.
func (r *Repository) ListMessages(ctx context.Context, channelID int64, limit, offset int) ([]*RawMessage, error) {
	query := `SELECT id, channel_id, message_id, payload, received_at, schema_version
		FROM raw_messages`
	args := []any{}
	if channelID > 0 {
		query += ` WHERE channel_id = ?`
		args = append(args, channelID)
	}
	query += ` ORDER BY received_at DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, apperrors.Wrap("storage", "list_messages", err)
	}
	defer rows.Close()

	var messages []*RawMessage
	for rows.Next() {
		msg, err := scanMessage(rows)
		if err != nil {
			return nil, err
		}
		messages = append(messages, msg)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap("storage", "iterate_messages", err)
	}
	return messages, nil
}

// ChannelStats holds message count for a single channel.
type ChannelStats struct {
	ChannelID    int64
	Username     string
	MessageCount int64
}

// CountMessagesByChannel returns message counts grouped by channel.
func (r *Repository) CountMessagesByChannel(ctx context.Context) ([]ChannelStats, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.username, COUNT(m.id) as msg_count
		FROM channels c
		LEFT JOIN raw_messages m ON c.id = m.channel_id
		GROUP BY c.id, c.username
		ORDER BY msg_count DESC`)
	if err != nil {
		return nil, apperrors.Wrap("storage", "count_messages_by_channel", err)
	}
	defer rows.Close()

	var stats []ChannelStats
	for rows.Next() {
		var s ChannelStats
		if err := rows.Scan(&s.ChannelID, &s.Username, &s.MessageCount); err != nil {
			return nil, apperrors.Wrap("storage", "scan_channel_stats", err)
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap("storage", "iterate_channel_stats", err)
	}
	return stats, nil
}

// GetMessageByID returns a single message by its database ID.
func (r *Repository) GetMessageByID(ctx context.Context, id int64) (*RawMessage, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, channel_id, message_id, payload, received_at, schema_version
		FROM raw_messages WHERE id = ?`, id)
	msg, err := scanMessage(row)
	if stderrors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.Wrap("storage", "get_message", stderrors.New("not found"))
	}
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// --- helpers ---

// scanner abstracts *sql.Row and *sql.Rows for shared channel scanning.
type scanner interface {
	Scan(dest ...any) error
}

func scanChannel(s scanner) (*Channel, error) {
	var (
		ch          Channel
		active      int
		addedAt     string
		lastColl    sql.NullString
	)
	if err := s.Scan(&ch.ID, &ch.Username, &ch.Title, &active, &addedAt,
		&ch.LastMessageID, &lastColl); err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, apperrors.Wrap("storage", "scan_channel", err)
	}
	ch.Active = active != 0
	ch.AddedAt = parseDBTime(addedAt)
	if lastColl.Valid {
		ch.LastCollectedAt = parseDBTime(lastColl.String)
	}
	return &ch, nil
}

func scanMessage(s scanner) (*RawMessage, error) {
	var (
		msg       RawMessage
		payload   string
		received  string
	)
	if err := s.Scan(&msg.ID, &msg.ChannelID, &msg.MessageID, &payload, &received, &msg.SchemaVersion); err != nil {
		if stderrors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, apperrors.Wrap("storage", "scan_message", err)
	}
	msg.Payload = []byte(payload)
	msg.ReceivedAt = parseDBTime(received)
	return &msg, nil
}

func parseDBTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(dbTimeLayout, s)
	if err != nil {
		// Fall back to RFC3339 in case a caller stored that form.
		if t2, err2 := time.Parse(time.RFC3339, s); err2 == nil {
			return t2
		}
		return time.Time{}
	}
	return t
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}

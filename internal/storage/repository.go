package storage

import (
	"context"
	"database/sql"
	stderrors "errors"
	"time"

	apperrors "github.com/limiar/collector/internal/errors"
)

// ErrNoSession indica que não há linha (row) de sessão presente no banco de dados.
var ErrNoSession = stderrors.New("nenhuma sessão armazenada")

// dbTimeLayout é o formato de data/hora textual usado pelos padrões (defaults)
// datetime('now') do schema.
const dbTimeLayout = "2006-01-02 15:04:05"

// RawMessage é um payload de mensagem capturada do Telegram aguardando processamento
// posterior (downstream). Payload contém o update bruto do gotd/td serializado como JSON.
type RawMessage struct {
	ID            int64
	ChannelID     int64
	MessageID     int64
	Payload       []byte
	ReceivedAt    time.Time
	SchemaVersion int
}

// Channel é um canal monitorado do Telegram e seu cursor de coleta.
type Channel struct {
	ID              int64
	Username        string
	Title           string
	Active          bool
	AddedAt         time.Time
	LastMessageID   int64
	LastCollectedAt time.Time
}

// Peer é um peer do Telegram armazenado em cache (canal, usuário ou chat) com seu hash de acesso.
type Peer struct {
	ID         int64
	AccessHash int64
	Type       string
	Username   string
	UpdatedAt  time.Time
}

// Repository centraliza toda instrução SQL contra o banco de dados Tursogo.
// Escritas recorrentes usam prepared statements (instruções preparadas). Todos os marcadores (placeholders) são ?.
type Repository struct {
	db *sql.DB

	stmtSaveMessage *sql.Stmt
	stmtSavePeer    *sql.Stmt
}

// NewRepository prepara as instruções recorrentes e retorna um Repository pronto para uso.
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

// Close libera todas as instruções preparadas (prepared statements).
func (r *Repository) Close() error {
	return stderrors.Join(r.stmtSaveMessage.Close(), r.stmtSavePeer.Close())
}

// --- Session ---

// SaveSession insere ou atualiza (upsert) a única linha (row) de sessão (o id é restrito a 1).
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

// LoadSession retorna os bytes da sessão armazenada, ou ErrNoSession se estiver ausente.
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

// CountSessions retorna o número de linhas (rows) de sessão (0 ou 1).
func (r *Repository) CountSessions(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM sessions`).Scan(&n); err != nil {
		return 0, apperrors.Wrap("storage", "count_sessions", err)
	}
	return n, nil
}

// --- Peers ---

// SavePeer insere ou atualiza um peer por id.
func (r *Repository) SavePeer(ctx context.Context, p *Peer) error {
	_, err := r.stmtSavePeer.ExecContext(ctx,
		p.ID, p.AccessHash, p.Type, nullString(p.Username),
		time.Now().UTC().Format(dbTimeLayout))
	if err != nil {
		return apperrors.Wrap("storage", "save_peer", err)
	}
	return nil
}

// LoadPeers retorna todos os peers em cache.
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

// AddChannel insere um canal monitorado.
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

// RemoveChannel exclui um canal pelo username, retornando ErrChannelNotFound se
// tal canal não existir.
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

// ListChannels retorna todos os canais monitorados ordenados pelo id.
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

// GetChannel retorna um único canal pelo id, ou ErrChannelNotFound.
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

// UpdateChannelLastMessage avança o cursor de coleta de um canal.
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

// SaveRawMessage persiste uma mensagem capturada. Pares (channel_id, message_id)
// duplicados são ignorados silenciosamente via ON CONFLICT DO NOTHING. A
// flag inserted (inserida) retornada é verdadeira quando uma nova linha foi escrita e falsa quando
// a mensagem já estava presente (uma duplicata). Isso permite que os chamadores produzam
// contagens precisas de observabilidade "novo vs duplicado".
func (r *Repository) SaveRawMessage(ctx context.Context, msg *RawMessage) (inserted bool, err error) {
	res, err := r.stmtSaveMessage.ExecContext(ctx,
		msg.ChannelID, msg.MessageID, string(msg.Payload),
		msg.ReceivedAt.UTC().Format(dbTimeLayout), msg.SchemaVersion)
	if err != nil {
		return false, apperrors.Wrap("storage", "save_raw_message", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		// Erros de RowsAffected são raros e específicos do driver; trate como "desconhecido"
		// em vez de falhar o salvamento — a mensagem foi persistida de qualquer forma.
		return true, nil
	}
	return affected > 0, nil
}

// CountRawMessages retorna o número total de mensagens brutas (raw messages) armazenadas.
func (r *Repository) CountRawMessages(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM raw_messages`).Scan(&n); err != nil {
		return 0, apperrors.Wrap("storage", "count_raw_messages", err)
	}
	return n, nil
}

// ListMessages retorna mensagens recentes, opcionalmente filtradas pelo channelID.
// Passe 0 para channelID para listar todos os canais. Os resultados são ordenados por received_at DESC.
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

// ChannelStats mantém a contagem de mensagens para um único canal.
type ChannelStats struct {
	ChannelID    int64
	Username     string
	MessageCount int64
}

// CountMessagesByChannel retorna as contagens de mensagens agrupadas por canal.
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

// GetMessageByID retorna uma única mensagem pelo seu ID no banco de dados.
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

// --- Processed Messages (read-only para dashboard) ---

// ProcessedMessage representa uma mensagem normalizada e classificada pelo processor.
type ProcessedMessage struct {
	ID             int64     `json:"id"`
	RawMessageID   int64     `json:"raw_message_id"`
	ChannelID      int64     `json:"channel_id"`
	MessageID      int64     `json:"message_id"`
	MessageType    string    `json:"message_type"`
	TextClean      string    `json:"text_clean"`
	TextLength     int       `json:"text_length"`
	MediaType      string    `json:"media_type"`
	HasURL         bool      `json:"has_url"`
	HasPrice       bool      `json:"has_price"`
	HasCoupon      bool      `json:"has_coupon"`
	PriceAmount    int64     `json:"price_amount"`
	PriceCurrency  string    `json:"price_currency"`
	UrgencySignals string   `json:"urgency_signals"`
	PostedAt       time.Time `json:"posted_at"`
	ProcessedAt    time.Time `json:"processed_at"`
}

// ProcessedTypeStats contém contagem de mensagens processadas por tipo.
type ProcessedTypeStats struct {
	MessageType string `json:"message_type"`
	Count       int64  `json:"count"`
}

// ListProcessedMessages retorna mensagens processadas com paginação e filtro opcional
// por tipo e canal. Resultados ordenados por posted_at DESC.
func (r *Repository) ListProcessedMessages(ctx context.Context, channelID int64, msgType string, limit, offset int) ([]*ProcessedMessage, error) {
	query := `SELECT id, raw_message_id, channel_id, message_id, message_type,
		text_clean, text_length, media_type, has_url, has_price, has_coupon,
		price_amount, price_currency, urgency_signals, posted_at, processed_at
		FROM processed_messages`
	var conditions []string
	args := []any{}

	if channelID > 0 {
		conditions = append(conditions, "channel_id = ?")
		args = append(args, channelID)
	}
	if msgType != "" {
		conditions = append(conditions, "message_type = ?")
		args = append(args, msgType)
	}
	if len(conditions) > 0 {
		query += " WHERE " + conditions[0]
		for _, c := range conditions[1:] {
			query += " AND " + c
		}
	}
	query += " ORDER BY posted_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, apperrors.Wrap("storage", "list_processed_messages", err)
	}
	defer rows.Close()

	var msgs []*ProcessedMessage
	for rows.Next() {
		var (
			m       ProcessedMessage
			posted  string
			procAt  string
			hasURL  int
			hasPrc  int
			hasCpn  int
			price   sql.NullInt64
			urgency sql.NullString
			curr    sql.NullString
		)
		if err := rows.Scan(&m.ID, &m.RawMessageID, &m.ChannelID, &m.MessageID, &m.MessageType,
			&m.TextClean, &m.TextLength, &m.MediaType, &hasURL, &hasPrc, &hasCpn,
			&price, &curr, &urgency, &posted, &procAt); err != nil {
			return nil, apperrors.Wrap("storage", "scan_processed_message", err)
		}
		m.HasURL = hasURL != 0
		m.HasPrice = hasPrc != 0
		m.HasCoupon = hasCpn != 0
		if price.Valid {
			m.PriceAmount = price.Int64
		}
		if curr.Valid {
			m.PriceCurrency = curr.String
		}
		if urgency.Valid {
			m.UrgencySignals = urgency.String
		}
		m.PostedAt = parseDBTime(posted)
		m.ProcessedAt = parseDBTime(procAt)
		msgs = append(msgs, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap("storage", "iterate_processed_messages", err)
	}
	return msgs, nil
}

// CountProcessedByType retorna contagem de mensagens processadas agrupadas por message_type.
func (r *Repository) CountProcessedByType(ctx context.Context) ([]ProcessedTypeStats, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT message_type, COUNT(*) as cnt
		FROM processed_messages
		GROUP BY message_type
		ORDER BY cnt DESC`)
	if err != nil {
		return nil, apperrors.Wrap("storage", "count_processed_by_type", err)
	}
	defer rows.Close()

	var stats []ProcessedTypeStats
	for rows.Next() {
		var s ProcessedTypeStats
		if err := rows.Scan(&s.MessageType, &s.Count); err != nil {
			return nil, apperrors.Wrap("storage", "scan_processed_type_stats", err)
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap("storage", "iterate_processed_type_stats", err)
	}
	return stats, nil
}

// CountProcessedMessages retorna o total de mensagens processadas.
func (r *Repository) CountProcessedMessages(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processed_messages`).Scan(&n); err != nil {
		return 0, apperrors.Wrap("storage", "count_processed_messages", err)
	}
	return n, nil
}

// --- helpers ---

// scanner abstrai *sql.Row e *sql.Rows para escaneamento (scanning) compartilhado de canais.
type scanner interface {
	Scan(dest ...any) error
}

func scanChannel(s scanner) (*Channel, error) {
	var (
		ch       Channel
		active   int
		addedAt  string
		lastColl sql.NullString
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
		msg      RawMessage
		payload  string
		received string
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
		// Fallback para RFC3339 caso um chamador tenha armazenado dessa forma.
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

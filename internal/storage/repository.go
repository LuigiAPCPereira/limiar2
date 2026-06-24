package storage

import (
	"context"
	"database/sql"
	stderrors "errors"
	"time"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/model"
)

// ErrNoSession indica que não há linha (row) de sessão presente no banco de dados.
var ErrNoSession = stderrors.New("nenhuma sessão armazenada")




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
		data, time.Now().UTC().Format(model.DBTimeLayout))
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

// --- model.Peers ---

// SavePeer insere ou atualiza um peer por id.
func (r *Repository) SavePeer(ctx context.Context, p *model.Peer) error {
	_, err := r.stmtSavePeer.ExecContext(ctx,
		p.ID, p.AccessHash, p.Type, nullString(p.Username),
		time.Now().UTC().Format(model.DBTimeLayout))
	if err != nil {
		return apperrors.Wrap("storage", "save_peer", err)
	}
	return nil
}

// SavePeersBatch insere ou atualiza múltiplos peers em uma única transação.
// Significativamente mais rápido que SavePeer individual no SQLite/Turso
// (elimina N commits → 1 commit).
func (r *Repository) SavePeersBatch(ctx context.Context, peers []*model.Peer) error {
	if len(peers) == 0 {
		return nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return apperrors.Wrap("storage", "save_peers_batch_begin", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt := tx.StmtContext(ctx, r.stmtSavePeer)
	now := time.Now().UTC().Format(model.DBTimeLayout)

	for _, p := range peers {
		_, err := stmt.ExecContext(ctx,
			p.ID, p.AccessHash, p.Type, nullString(p.Username), now)
		if err != nil {
			return apperrors.Wrap("storage", "save_peers_batch_exec", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return apperrors.Wrap("storage", "save_peers_batch_commit", err)
	}
	return nil
}

// LoadPeers retorna todos os peers em cache.
func (r *Repository) LoadPeers(ctx context.Context) ([]*model.Peer, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT id, access_hash, type, username, updated_at FROM peers`)
	if err != nil {
		return nil, apperrors.Wrap("storage", "load_peers", err)
	}
	defer func() { _ = rows.Close() }()

	var peers []*model.Peer
	for rows.Next() {
		var (
			p        model.Peer
			username sql.NullString
			updated  string
		)
		if err := rows.Scan(&p.ID, &p.AccessHash, &p.Type, &username, &updated); err != nil {
			return nil, apperrors.Wrap("storage", "scan_peer", err)
		}
		p.Username = username.String
		p.UpdatedAt = model.ParseDBTime(updated)
		peers = append(peers, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap("storage", "iterate_peers", err)
	}
	return peers, nil
}

// --- model.Channels ---

// AddChannel insere um canal monitorado.
func (r *Repository) AddChannel(ctx context.Context, ch *model.Channel) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO channels (id, username, title, active)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(username) DO UPDATE SET title = excluded.title, active = excluded.active`,
		ch.ID, ch.Username, ch.Title, model.BoolToInt(ch.Active))
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
func (r *Repository) ListChannels(ctx context.Context) ([]*model.Channel, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, username, title, active, added_at, last_message_id, last_collected_at
		FROM channels ORDER BY id`)
	if err != nil {
		return nil, apperrors.Wrap("storage", "list_channels", err)
	}
	defer func() { _ = rows.Close() }()

	var channels []*model.Channel
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
func (r *Repository) GetChannel(ctx context.Context, id int64) (*model.Channel, error) {
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
		messageID, collectedAt.UTC().Format(model.DBTimeLayout), channelID)
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
func (r *Repository) SaveRawMessage(ctx context.Context, msg *model.RawMessage) (inserted bool, err error) {
	res, err := r.stmtSaveMessage.ExecContext(ctx,
		msg.ChannelID, msg.MessageID, string(msg.Payload),
		msg.ReceivedAt.UTC().Format(model.DBTimeLayout), msg.SchemaVersion)
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

// SaveRawMessageBatch persiste múltiplas mensagens capturadas em uma única transação.
// Otimização de throughput: colapsa N operações BEGIN/COMMIT (cada uma com seu fsync)
// em apenas 1 fsync, reduzindo a latência de escrita em ordens de magnitude durante
// backfill ou rajadas (bursts) de captura ao vivo. O ON CONFLICT DO NOTHING mantém a
// idempotência por (channel_id, message_id). Em caso de falha, nenhuma linha é persistida
// (rollback atômico) e inserted é retornado nil com o erro.
//
// inserted[i] indica se msgs[i] foi uma nova inserção (true) ou duplicata (false).
// totalInserted é o número de linhas efetivamente adicionadas (soma dos true).
func (r *Repository) SaveRawMessageBatch(ctx context.Context, msgs []*model.RawMessage) (inserted []bool, totalInserted int, err error) {
	if len(msgs) == 0 {
		return nil, 0, nil
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, 0, apperrors.Wrap("storage", "save_raw_message_batch_begin", err)
	}
	// Rollback é no-op após Commit bem-sucedido; garante limpeza em falhas.
	defer func() { _ = tx.Rollback() }()

	stmt := tx.StmtContext(ctx, r.stmtSaveMessage)
	inserted = make([]bool, len(msgs))
	for i, msg := range msgs {
		res, execErr := stmt.ExecContext(ctx,
			msg.ChannelID, msg.MessageID, string(msg.Payload),
			msg.ReceivedAt.UTC().Format(model.DBTimeLayout), msg.SchemaVersion)
		if execErr != nil {
			return nil, 0, apperrors.Wrap("storage", "save_raw_message_batch_exec", execErr)
		}
		affected, rowsErr := res.RowsAffected()
		if rowsErr != nil {
			// Mesma postura de SaveRawMessage: tratamos como "inserido" pois a
			// statement foi executada com sucesso; apenas a contagem é incerta.
			inserted[i] = true
			totalInserted++
			continue
		}
		if affected > 0 {
			inserted[i] = true
			totalInserted++
		}
	}
	if commitErr := tx.Commit(); commitErr != nil {
		return nil, 0, apperrors.Wrap("storage", "save_raw_message_batch_commit", commitErr)
	}
	return inserted, totalInserted, nil
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
func (r *Repository) ListMessages(ctx context.Context, channelID int64, limit, offset int) ([]*model.RawMessage, error) {
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
	defer func() { _ = rows.Close() }()

	var messages []*model.RawMessage
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


// CountMessagesByChannel retorna as contagens de mensagens agrupadas por canal.
func (r *Repository) CountMessagesByChannel(ctx context.Context) ([]model.ChannelStats, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT c.id, c.username, COUNT(m.id) as msg_count
		FROM channels c
		LEFT JOIN raw_messages m ON c.id = m.channel_id
		GROUP BY c.id, c.username
		ORDER BY msg_count DESC`)
	if err != nil {
		return nil, apperrors.Wrap("storage", "count_messages_by_channel", err)
	}
	defer func() { _ = rows.Close() }()

	var stats []model.ChannelStats
	for rows.Next() {
		var s model.ChannelStats
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
func (r *Repository) GetMessageByID(ctx context.Context, id int64) (*model.RawMessage, error) {
	row := r.db.QueryRowContext(ctx, `
		SELECT id, channel_id, message_id, payload, received_at, schema_version
		FROM raw_messages WHERE id = ?`, id)
	msg, err := scanMessage(row)
	if stderrors.Is(err, sql.ErrNoRows) {
		return nil, apperrors.Wrap("storage", "get_message", apperrors.ErrMessageNotFound)
	}
	if err != nil {
		return nil, err
	}
	return msg, nil
}

// --- helpers ---

// scanner abstrai *sql.Row e *sql.Rows para escaneamento (scanning) compartilhado de canais.
type scanner interface {
	Scan(dest ...any) error
}

func scanChannel(s scanner) (*model.Channel, error) {
	var (
		ch       model.Channel
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
	ch.AddedAt = model.ParseDBTime(addedAt)
	if lastColl.Valid {
		ch.LastCollectedAt = model.ParseDBTime(lastColl.String)
	}
	return &ch, nil
}

func scanMessage(s scanner) (*model.RawMessage, error) {
	var (
		msg      model.RawMessage
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
	msg.ReceivedAt = model.ParseDBTime(received)
	return &msg, nil
}



func nullString(s string) any {
	if s == "" {
		return nil
	}
	return s
}


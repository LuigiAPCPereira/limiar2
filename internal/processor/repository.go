// Package processor implementa o limiar-processor: normalização, classificação
// e persistência de mensagens brutas capturadas pelo limiar-collector.
package processor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/model"
)

// Repository centraliza as queries do processor contra o banco Tursogo.
// O processor lê raw_messages (read-only) e escreve em processed_messages.
type Repository struct {
	db                  *sql.DB
	stmtInsertProcessed *sql.Stmt
}

// NewRepository prepara statements recorrentes e retorna um Repository pronto.
func NewRepository(db *sql.DB) (*Repository, error) {
	stmt, err := db.Prepare(`
		INSERT INTO processed_messages (
			raw_message_id, channel_id, message_id, message_type,
			text_clean, text_length, media_type, photo_id,
			views, forwards, reply_to_msg_id,
			has_url, has_price, has_coupon, price_amount, price_currency,
			urgency_signals, posted_at, processed_at,
			price_original, price_discount, coupon_code,
			payment_method, shipping, installments, discount_percent,
			url_hash, merchant, product_name, synthesis, is_duplicate, feed_eligible,
			photo_access_hash, photo_file_ref, photo_dcid, inline_thumb
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(channel_id, message_id) DO NOTHING`)
	if err != nil {
		return nil, apperrors.Wrap("processor", "prepare_insert_processed", err)
	}
	return &Repository{db: db, stmtInsertProcessed: stmt}, nil
}

// Close libera prepared statements.
func (r *Repository) Close() error {
	return r.stmtInsertProcessed.Close()
}

// FetchUnprocessed retorna raw_messages que ainda não possuem entrada
// correspondente em processed_messages. Usa cursor baseado no último
// raw_message_id processado (evita LEFT JOIN que degrada com o tempo).
func (r *Repository) FetchUnprocessed(ctx context.Context, limit int) ([]*model.RawMessage, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, channel_id, message_id, payload, received_at, schema_version
		FROM raw_messages
		WHERE id > COALESCE(
			(SELECT MAX(raw_message_id) FROM processed_messages), 0
		)
		ORDER BY id ASC
		LIMIT ?`, limit)
	if err != nil {
		return nil, apperrors.Wrap("processor", "fetch_unprocessed", err)
	}
	defer func() { _ = rows.Close() }()

	var msgs []*model.RawMessage
	for rows.Next() {
		var (
			msg      model.RawMessage
			payload  string
			received string
		)
		if err := rows.Scan(&msg.ID, &msg.ChannelID, &msg.MessageID, &payload, &received, &msg.SchemaVersion); err != nil {
			return nil, apperrors.Wrap("processor", "scan_unprocessed", err)
		}
		msg.Payload = []byte(payload)
		msg.ReceivedAt = model.ParseDBTime(received)
		msgs = append(msgs, &msg)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap("processor", "iterate_unprocessed", err)
	}
	return msgs, nil
}

// CountUnprocessed retorna quantas raw_messages ainda não foram processadas.
// Usa COUNT total subtraído de COUNT processadas (evita LEFT JOIN).
func (r *Repository) CountUnprocessed(ctx context.Context) (int64, error) {
	var rawCount, procCount int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM raw_messages`).Scan(&rawCount); err != nil {
		return 0, apperrors.Wrap("processor", "count_raw", err)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processed_messages`).Scan(&procCount); err != nil {
		return 0, apperrors.Wrap("processor", "count_processed", err)
	}
	n := rawCount - procCount
	if n < 0 {
		n = 0
	}
	return n, nil
}

// processedInsertArgs monta a lista de argumentos para INSERT em processed_messages.
// Centraliza a conversão NormalizedMessage → []any, evitando duplicação entre
// SaveProcessed e SaveProcessedBatch.
func processedInsertArgs(msg *NormalizedMessage) []any {
	urgencyJSON, err := json.Marshal(msg.UrgencySignals)
	if err != nil {
		urgencyJSON = []byte("[]")
	}

	var priceAmount any
	if msg.PriceAmount > 0 {
		priceAmount = msg.PriceAmount
	}

	return []any{
		msg.RawMessageID,
		msg.ChannelID,
		msg.MessageID,
		msg.MessageType,
		msg.Text,
		msg.TextLength,
		msg.MediaType,
		msg.PhotoID,
		msg.Views,
		msg.Forwards,
		msg.ReplyToMsgID,
		model.BoolToInt(msg.HasURL),
		model.BoolToInt(msg.HasPrice),
		model.BoolToInt(msg.HasCoupon),
		priceAmount,
		"BRL",
		string(urgencyJSON),
		msg.PostedAt.UTC().Format(model.DBTimeLayout),
		msg.ProcessedAt.UTC().Format(model.DBTimeLayout),
		msg.PriceOriginal,
		msg.PriceDiscount,
		msg.CouponCode,
		msg.PaymentMethod,
		msg.Shipping,
		msg.Installments,
		msg.DiscountPct,
		msg.URLHash,
		msg.Merchant,
		msg.ProductName,
		msg.Synthesis,
		model.BoolToInt(msg.IsDuplicate),
		model.BoolToInt(msg.FeedEligible),
		msg.PhotoAccessHash,
		msg.PhotoFileRef,
		msg.PhotoDCID,
		msg.InlineThumb,
	}
}

// SaveProcessed persiste uma mensagem normalizada em processed_messages.
// Idempotente via ON CONFLICT DO NOTHING.
func (r *Repository) SaveProcessed(ctx context.Context, msg *NormalizedMessage) error {
	_, err := r.stmtInsertProcessed.ExecContext(ctx, processedInsertArgs(msg)...)
	if err != nil {
		return apperrors.Wrap("processor", "save_processed", fmt.Errorf("msg_id=%d: %w", msg.MessageID, err))
	}
	return nil
}

// CrossChannelDuplicates recebe pares (url_hash → channel_id) e retorna
// apenas os hashes que já existem no banco em canais DIFERENTES.
// Usa uma única query com OR conditions em vez de N queries individuais.
func (r *Repository) CrossChannelDuplicates(ctx context.Context, pairs map[string]int64) (map[string]bool, error) {
	if len(pairs) == 0 {
		return nil, nil
	}

	// Constrói query: SELECT DISTINCT url_hash FROM processed_messages
	//   WHERE (url_hash = ? AND channel_id != ?) OR (url_hash = ? AND channel_id != ?) ...
	args := make([]interface{}, 0, len(pairs)*2)
	query := "SELECT DISTINCT url_hash FROM processed_messages WHERE "
	first := true
	for hash, channelID := range pairs {
		if !first {
			query += " OR "
		}
		query += "(url_hash = ? AND channel_id != ?)"
		args = append(args, hash, channelID)
		first = false
	}

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, apperrors.Wrap("processor", "cross_channel_duplicates", err)
	}
	defer func() { _ = rows.Close() }()

	result := make(map[string]bool)
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			return result, apperrors.Wrap("processor", "scan_cross_channel_dup", err)
		}
		result[hash] = true
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap("processor", "cross_channel_duplicates_iter", err)
	}
	return result, nil
}

// SaveProcessedBatch persiste múltiplas mensagens em uma única transação.
// Significativamente mais rápido que INSERTs individuais no SQLite (10-50x).
// Mensagens com erro individual são contabilizadas em failed mas não abortam
// o batch. Erros de transação (begin/commit) são retornados via err.
func (r *Repository) SaveProcessedBatch(ctx context.Context, msgs []*NormalizedMessage) (saved, failed int, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, len(msgs), apperrors.Wrap("processor", "save_processed_batch_begin", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt := tx.StmtContext(ctx, r.stmtInsertProcessed)

	for _, msg := range msgs {
		_, execErr := stmt.ExecContext(ctx, processedInsertArgs(msg)...)
		if execErr != nil {
			failed++
			continue
		}
		saved++
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return 0, len(msgs), apperrors.Wrap("processor", "save_processed_batch_commit", commitErr)
	}
	return saved, failed, nil
}

// ═══════════════════════════════════════════════════════════════════
// Processed Messages — queries de leitura sobre dados processados.
// ═══════════════════════════════════════════════════════════════════

// ListProcessedMessages retorna mensagens processadas com paginação e filtro
// opcional por tipo e canal. Resultados ordenados por posted_at DESC.
func (r *Repository) ListProcessedMessages(ctx context.Context, channelID int64, msgType string, limit, offset int) ([]*model.ProcessedMessage, error) {
	query := `SELECT id, raw_message_id, channel_id, message_id, message_type,
		text_clean, text_length, media_type, has_url, has_price, has_coupon,
		price_amount, price_currency, urgency_signals, posted_at, processed_at,
		price_original, price_discount, coupon_code, payment_method, shipping,
		installments, discount_percent, merchant, product_name, is_duplicate
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
			query += " AND " + c // #nosec G202 — conditions are hardcoded column names, values use ?
		}
	}
	query += " ORDER BY posted_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, apperrors.Wrap("processor", "list_processed_messages", err)
	}
	defer func() { _ = rows.Close() }()

	var msgs []*model.ProcessedMessage
	for rows.Next() {
		var (
			m       model.ProcessedMessage
			posted  string
			procAt  string
			hasURL  int
			hasPrc  int
			hasCpn  int
			isDup   int
			price   sql.NullInt64
			urgency sql.NullString
			curr    sql.NullString
		)
		if err := rows.Scan(&m.ID, &m.RawMessageID, &m.ChannelID, &m.MessageID, &m.MessageType,
			&m.TextClean, &m.TextLength, &m.MediaType, &hasURL, &hasPrc, &hasCpn,
			&price, &curr, &urgency, &posted, &procAt,
			&m.PriceOriginal, &m.PriceDiscount, &m.CouponCode, &m.PaymentMethod, &m.Shipping,
			&m.Installments, &m.DiscountPct, &m.Merchant, &m.ProductName, &isDup); err != nil {
			return nil, apperrors.Wrap("processor", "scan_processed_message", err)
		}
		m.HasURL = hasURL != 0
		m.HasPrice = hasPrc != 0
		m.HasCoupon = hasCpn != 0
		m.IsDuplicate = isDup != 0
		if price.Valid {
			m.PriceAmount = price.Int64
		}
		if curr.Valid {
			m.PriceCurrency = curr.String
		}
		if urgency.Valid {
			m.UrgencySignals = urgency.String
		}
		m.PostedAt = model.ParseDBTime(posted)
		m.ProcessedAt = model.ParseDBTime(procAt)
		msgs = append(msgs, &m)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap("processor", "iterate_processed_messages", err)
	}
	return msgs, nil
}

// CountProcessedByType retorna contagem de mensagens processadas agrupadas por message_type.
func (r *Repository) CountProcessedByType(ctx context.Context) ([]model.ProcessedTypeStats, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT message_type, COUNT(*) as cnt
		FROM processed_messages
		GROUP BY message_type
		ORDER BY cnt DESC`)
	if err != nil {
		return nil, apperrors.Wrap("processor", "count_processed_by_type", err)
	}
	defer func() { _ = rows.Close() }()

	var stats []model.ProcessedTypeStats
	for rows.Next() {
		var s model.ProcessedTypeStats
		if err := rows.Scan(&s.MessageType, &s.Count); err != nil {
			return nil, apperrors.Wrap("processor", "scan_processed_type_stats", err)
		}
		stats = append(stats, s)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap("processor", "iterate_processed_type_stats", err)
	}
	return stats, nil
}

// CountProcessedMessages retorna o total de mensagens processadas.
func (r *Repository) CountProcessedMessages(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processed_messages`).Scan(&n); err != nil {
		return 0, apperrors.Wrap("processor", "count_processed_messages", err)
	}
	return n, nil
}

// GetPhotoMetadata retorna os campos MTProto de imagem para uma mensagem processada.
// Usado pelo MediaResolver para montar InputPhotoFileLocation.
func (r *Repository) GetPhotoMetadata(ctx context.Context, processedMsgID int64) (*model.PhotoMetadata, error) {
	var m model.PhotoMetadata
	err := r.db.QueryRowContext(ctx, `
		SELECT id, message_id, channel_id, photo_id, photo_access_hash, photo_file_ref, photo_dcid
		FROM processed_messages WHERE id = ?`, processedMsgID).Scan(
		&m.ID, &m.MsgID, &m.ChannelID, &m.PhotoID, &m.AccessHash, &m.FileReference, &m.DCID)
	if err != nil {
		return nil, apperrors.Wrap("processor", "get_photo_metadata", err)
	}
	return &m, nil
}

// GetPhotoID retorna o photo_id (Telegram) de uma mensagem processada.
// Retorna (0, nil) se a mensagem não existe ou não tem foto.
func (r *Repository) GetPhotoID(ctx context.Context, processedMsgID int64) (int64, error) {
	var photoID sql.NullInt64
	err := r.db.QueryRowContext(ctx, `
		SELECT photo_id FROM processed_messages WHERE id = ?`, processedMsgID).Scan(&photoID)
	if err != nil {
		if err == sql.ErrNoRows {
			return 0, nil
		}
		return 0, apperrors.Wrap("processor", "get_photo_id", err)
	}
	if !photoID.Valid {
		return 0, nil
	}
	return photoID.Int64, nil
}

// GetInlineThumb retorna o thumbnail inline (Type "i") de uma mensagem pelo photo_id.
// Retorna (nil, nil) se a mensagem não tem inline thumb ou não existe.
func (r *Repository) GetInlineThumb(ctx context.Context, photoID int64) ([]byte, error) {
	var thumb []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT inline_thumb FROM processed_messages
		WHERE photo_id = ? AND inline_thumb IS NOT NULL
		LIMIT 1`, photoID).Scan(&thumb)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, apperrors.Wrap("processor", "get_inline_thumb", err)
	}
	return thumb, nil
}

// PhotoMetadataStats agrega a cobertura de metadados de foto.
func (r *Repository) PhotoMetadataStats(ctx context.Context) (model.PhotoStats, error) {
	var s model.PhotoStats
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*),
			COUNT(CASE WHEN photo_id > 0 THEN 1 END),
			COUNT(CASE WHEN photo_id > 0
				AND photo_access_hash != 0
				AND photo_file_ref != ''
				AND photo_dcid != 0 THEN 1 END)
		FROM processed_messages`).Scan(&s.TotalProcessed, &s.WithPhoto, &s.CompleteMTProto)
	if err != nil {
		return model.PhotoStats{}, apperrors.Wrap("processor", "photo_metadata_stats", err)
	}
	return s, nil
}
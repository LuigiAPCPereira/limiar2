// Package processor implementa o limiar-processor: normalização, classificação
// e persistência de mensagens brutas capturadas pelo limiar-collector.
package processor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/storage"
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
			url_hash, merchant, product_name, synthesis, is_duplicate, feed_eligible
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
func (r *Repository) FetchUnprocessed(ctx context.Context, limit int) ([]*storage.RawMessage, error) {
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
	defer rows.Close()

	var msgs []*storage.RawMessage
	for rows.Next() {
		var (
			msg      storage.RawMessage
			payload  string
			received string
		)
		if err := rows.Scan(&msg.ID, &msg.ChannelID, &msg.MessageID, &payload, &received, &msg.SchemaVersion); err != nil {
			return nil, apperrors.Wrap("processor", "scan_unprocessed", err)
		}
		msg.Payload = []byte(payload)
		msg.ReceivedAt = storage.ParseDBTime(received)
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

// SaveProcessed persiste uma mensagem normalizada em processed_messages.
// Idempotente via ON CONFLICT DO NOTHING.
func (r *Repository) SaveProcessed(ctx context.Context, msg *NormalizedMessage) error {
	urgencyJSON, err := json.Marshal(msg.UrgencySignals)
	if err != nil {
		urgencyJSON = []byte("[]")
	}

	var priceAmount any
	if msg.PriceAmount > 0 {
		priceAmount = msg.PriceAmount
	}

	_, err = r.stmtInsertProcessed.ExecContext(ctx,
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
		storage.BoolToInt(msg.HasURL),
		storage.BoolToInt(msg.HasPrice),
		storage.BoolToInt(msg.HasCoupon),
		priceAmount,
		"BRL",
		string(urgencyJSON),
		msg.PostedAt.UTC().Format(storage.DBTimeLayout),
		msg.ProcessedAt.UTC().Format(storage.DBTimeLayout),
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
		storage.BoolToInt(msg.IsDuplicate),
		storage.BoolToInt(msg.FeedEligible),
	)
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
	defer rows.Close()

	result := make(map[string]bool)
	for rows.Next() {
		var hash string
		if err := rows.Scan(&hash); err != nil {
			continue
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
// Mensagens com erro individual são logadas mas não abortam o batch.
func (r *Repository) SaveProcessedBatch(ctx context.Context, msgs []*NormalizedMessage) (saved, failed int) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, len(msgs)
	}
	defer func() { _ = tx.Rollback() }()

	stmt := tx.StmtContext(ctx, r.stmtInsertProcessed)

	for _, msg := range msgs {
		urgencyJSON, _ := json.Marshal(msg.UrgencySignals)

		var priceAmount any
		if msg.PriceAmount > 0 {
			priceAmount = msg.PriceAmount
		}

		_, err := stmt.ExecContext(ctx,
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
			storage.BoolToInt(msg.HasURL),
			storage.BoolToInt(msg.HasPrice),
			storage.BoolToInt(msg.HasCoupon),
			priceAmount,
			"BRL",
			string(urgencyJSON),
			msg.PostedAt.UTC().Format(storage.DBTimeLayout),
			msg.ProcessedAt.UTC().Format(storage.DBTimeLayout),
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
			storage.BoolToInt(msg.IsDuplicate),
			storage.BoolToInt(msg.FeedEligible),
		)
		if err != nil {
			failed++
			continue
		}
		saved++
	}

	if err := tx.Commit(); err != nil {
		return 0, len(msgs)
	}
	return saved, failed
}


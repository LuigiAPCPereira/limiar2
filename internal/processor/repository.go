// Package processor implementa o limiar-processor: normalização, classificação
// e persistência de mensagens brutas capturadas pelo limiar-collector.
package processor

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/storage"
)

const dbTimeLayout = "2006-01-02 15:04:05"

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
			urgency_signals, posted_at, processed_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
		msg.ReceivedAt = parseDBTime(received)
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
		boolToInt(msg.HasURL),
		boolToInt(msg.HasPrice),
		boolToInt(msg.HasCoupon),
		priceAmount,
		"BRL",
		string(urgencyJSON),
		msg.PostedAt.UTC().Format(dbTimeLayout),
		msg.ProcessedAt.UTC().Format(dbTimeLayout),
	)
	if err != nil {
		return apperrors.Wrap("processor", "save_processed", fmt.Errorf("msg_id=%d: %w", msg.MessageID, err))
	}
	return nil
}

// SaveProcessedBatch persiste múltiplas mensagens em uma única transação.
// Significativamente mais rápido que INSERTs individuais no SQLite (10-50x).
// Mensagens com erro individual são logadas mas não abortam o batch.
func (r *Repository) SaveProcessedBatch(ctx context.Context, msgs []*NormalizedMessage) (saved, failed int) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, len(msgs)
	}
	defer tx.Rollback()

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
			boolToInt(msg.HasURL),
			boolToInt(msg.HasPrice),
			boolToInt(msg.HasCoupon),
			priceAmount,
			"BRL",
			string(urgencyJSON),
			msg.PostedAt.UTC().Format(dbTimeLayout),
			msg.ProcessedAt.UTC().Format(dbTimeLayout),
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

func parseDBTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	t, err := time.Parse(dbTimeLayout, s)
	if err != nil {
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

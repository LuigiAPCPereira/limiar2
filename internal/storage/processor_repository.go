// Package storage implementa acesso físico ao banco Tursogo, incluindo as
// queries usadas pelo collector, processor, dashboard e subsistema de mídia.
package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/model"
)

var processedURLRegex = regexp.MustCompile(`https?://[^\s<>"']+`)

// ProcessorRepository centraliza as queries do processor contra o banco Tursogo.
// O processor lê raw_messages (read-only) e escreve em processed_messages.
type ProcessorRepository struct {
	db                  *sql.DB
	stmtInsertProcessed *sql.Stmt
}

// NewProcessorRepository prepara statements recorrentes e retorna um repository pronto.
func NewProcessorRepository(db *sql.DB) (*ProcessorRepository, error) {
	stmt, err := db.Prepare(`
		INSERT INTO processed_messages (
			raw_message_id, channel_id, message_id, message_type,
			text_clean, text_length, media_type, photo_id,
			views, forwards, reply_to_msg_id,
			has_url, has_price, has_coupon, price_amount, price_currency,
			posted_at, processed_at,
			price_original, price_discount, coupon_code, coupon_codes,
			payment_method, shipping, installments, discount_percent, modifiers,
			url_hash, merchant, product_name, product_name_confidence, synthesis,
			virtual_currency, webpage_url, webpage_title, webpage_desc, is_promotional,
			is_duplicate, feed_eligible,
			photo_access_hash, photo_file_ref, photo_dcid, inline_thumb,
			shipping_free, installments_n, installments_value, is_recurring,
			valid_from, valid_until, flash, recurrence_pattern, recurrence_group_id, seasonal_tag
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(channel_id, message_id) DO UPDATE SET
			message_type = excluded.message_type,
			text_clean = excluded.text_clean,
			text_length = excluded.text_length,
			media_type = excluded.media_type,
			photo_id = excluded.photo_id,
			views = excluded.views,
			forwards = excluded.forwards,
			reply_to_msg_id = excluded.reply_to_msg_id,
			has_url = excluded.has_url,
			has_price = excluded.has_price,
			has_coupon = excluded.has_coupon,
			price_amount = excluded.price_amount,
			price_currency = excluded.price_currency,
			posted_at = excluded.posted_at,
			processed_at = excluded.processed_at,
			price_original = excluded.price_original,
			price_discount = excluded.price_discount,
			coupon_code = excluded.coupon_code,
			coupon_codes = excluded.coupon_codes,
			payment_method = excluded.payment_method,
			shipping = excluded.shipping,
			installments = excluded.installments,
			discount_percent = excluded.discount_percent,
			modifiers = excluded.modifiers,
			url_hash = excluded.url_hash,
			merchant = excluded.merchant,
			product_name = excluded.product_name,
			product_name_confidence = excluded.product_name_confidence,
			synthesis = excluded.synthesis,
			virtual_currency = excluded.virtual_currency,
			webpage_url = excluded.webpage_url,
			webpage_title = excluded.webpage_title,
			webpage_desc = excluded.webpage_desc,
			is_promotional = excluded.is_promotional,
			is_duplicate = excluded.is_duplicate,
			feed_eligible = excluded.feed_eligible,
			photo_access_hash = excluded.photo_access_hash,
			photo_file_ref = excluded.photo_file_ref,
			photo_dcid = excluded.photo_dcid,
			inline_thumb = excluded.inline_thumb,
			shipping_free = excluded.shipping_free,
			installments_n = excluded.installments_n,
			installments_value = excluded.installments_value,
			is_recurring = excluded.is_recurring,
			valid_from = excluded.valid_from,
			valid_until = excluded.valid_until,
			flash = excluded.flash,
			recurrence_pattern = excluded.recurrence_pattern,
			recurrence_group_id = excluded.recurrence_group_id,
			seasonal_tag = excluded.seasonal_tag`)
	if err != nil {
		return nil, apperrors.Wrap("processor", "prepare_insert_processed", err)
	}
	return &ProcessorRepository{db: db, stmtInsertProcessed: stmt}, nil
}

// Close libera prepared statements.
func (r *ProcessorRepository) Close() error {
	return r.stmtInsertProcessed.Close()
}

// FetchUnprocessed retorna raw_messages que ainda não possuem entrada
// correspondente em processed_messages. Usa cursor baseado no último
// raw_message_id processado (evita LEFT JOIN que degrada com o tempo).
func (r *ProcessorRepository) FetchUnprocessed(ctx context.Context, limit int) ([]*model.RawMessage, error) {
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
func (r *ProcessorRepository) CountUnprocessed(ctx context.Context) (int64, error) {
	var rawCount, procCount int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM raw_messages`).Scan(&rawCount); err != nil {
		return 0, apperrors.Wrap("processor", "count_raw", err)
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processed_messages`).Scan(&procCount); err != nil {
		return 0, apperrors.Wrap("processor", "count_processed", err)
	}
	n := max(rawCount-procCount, 0)
	return n, nil
}

// processedInsertArgs monta a lista de argumentos para INSERT em processed_messages.
// Centraliza a conversão NormalizedMessage → []any, evitando duplicação entre
// SaveProcessed e SaveProcessedBatch.
func dbTimeOrNil(t time.Time) any {
	if t.IsZero() {
		return nil
	}
	return t.UTC().Format(model.DBTimeLayout)
}

func encodeCoupons(coupons []model.Coupon) (string, error) {
	if len(coupons) == 0 {
		return "", nil
	}
	data, err := json.Marshal(coupons)
	if err != nil {
		return "", fmt.Errorf("coupon_codes: %w", err)
	}
	return string(data), nil
}

func encodeModifiers(modifiers []model.Modifier) (string, error) {
	if len(modifiers) == 0 {
		return "", nil
	}
	data, err := json.Marshal(modifiers)
	if err != nil {
		return "", fmt.Errorf("modifiers: %w", err)
	}
	return string(data), nil
}

func encodeVirtualCurrency(vc *model.VirtualCurrency) (string, error) {
	if vc == nil {
		return "", nil
	}
	data, err := json.Marshal(vc)
	if err != nil {
		return "", fmt.Errorf("virtual_currency: %w", err)
	}
	return string(data), nil
}

func processedInsertArgs(msg *model.NormalizedMessage) ([]any, error) {
	var priceAmount any
	if msg.PriceAmount > 0 {
		priceAmount = msg.PriceAmount
	}

	var installmentsValue any
	if msg.InstallmentsValue > 0 {
		installmentsValue = msg.InstallmentsValue
	}
	couponCodes, err := encodeCoupons(msg.CouponCodes)
	if err != nil {
		return nil, err
	}
	modifiers, err := encodeModifiers(msg.Modifiers)
	if err != nil {
		return nil, err
	}
	virtualCurrency, err := encodeVirtualCurrency(msg.VirtualCurrency)
	if err != nil {
		return nil, err
	}
	validFrom := dbTimeOrNil(msg.ValidFrom)
	validUntil := dbTimeOrNil(msg.ValidUntil)

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
		msg.PostedAt.UTC().Format(model.DBTimeLayout),
		msg.ProcessedAt.UTC().Format(model.DBTimeLayout),
		msg.PriceOriginal,
		msg.PriceDiscount,
		msg.CouponCode,
		couponCodes,
		msg.PaymentMethod,
		msg.Shipping,
		msg.Installments,
		msg.DiscountPct,
		modifiers,
		msg.URLHash,
		msg.Merchant,
		msg.ProductName,
		msg.ProductNameConfidence,
		msg.Synthesis,
		virtualCurrency,
		msg.WebpageURL,
		msg.WebpageTitle,
		msg.WebpageDesc,
		model.BoolToInt(msg.IsPromotional),
		model.BoolToInt(msg.IsDuplicate),
		model.BoolToInt(msg.FeedEligible),
		msg.PhotoAccessHash,
		msg.PhotoFileRef,
		msg.PhotoDCID,
		msg.InlineThumb,
		model.BoolToInt(msg.ShippingFree),
		msg.InstallmentsN,
		installmentsValue,
		model.BoolToInt(msg.IsRecurring),
		validFrom,
		validUntil,
		model.BoolToInt(msg.Flash),
		msg.RecurrencePattern,
		msg.RecurrenceGroupID,
		msg.SeasonalTag,
	}, nil
}

// SaveProcessed persiste uma mensagem normalizada em processed_messages.
// Idempotente via ON CONFLICT DO NOTHING.
func (r *ProcessorRepository) SaveProcessed(ctx context.Context, msg *model.NormalizedMessage) error {
	args, err := processedInsertArgs(msg)
	if err != nil {
		return apperrors.Wrap("processor", "save_processed_encode", fmt.Errorf("msg_id=%d: %w", msg.MessageID, err))
	}
	_, err = r.stmtInsertProcessed.ExecContext(ctx, args...)
	if err != nil {
		return apperrors.Wrap("processor", "save_processed", fmt.Errorf("msg_id=%d: %w", msg.MessageID, err))
	}
	return nil
}

// CrossChannelDuplicates recebe pares (url_hash → channel_id) e retorna
// apenas os hashes que já existem no banco em canais DIFERENTES.
// Usa uma única query com OR conditions em vez de N queries individuais.
func (r *ProcessorRepository) CrossChannelDuplicates(ctx context.Context, pairs map[string]int64) (map[string]bool, error) {
	if len(pairs) == 0 {
		return nil, nil
	}

	// Constrói query: SELECT DISTINCT url_hash FROM processed_messages
	//   WHERE (url_hash = ? AND channel_id != ?) OR (url_hash = ? AND channel_id != ?) ...
	args := make([]any, 0, len(pairs)*2)
	var query strings.Builder
	query.WriteString("SELECT DISTINCT url_hash FROM processed_messages WHERE ")
	first := true
	for hash, channelID := range pairs {
		if !first {
			query.WriteString(" OR ")
		}
		query.WriteString("(url_hash = ? AND channel_id != ?)")
		args = append(args, hash, channelID)
		first = false
	}

	rows, err := r.db.QueryContext(ctx, query.String(), args...)
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
// Significativamente mais rápido que INSERTs individuais no SQLite/Turso.
// A primeira falha individual aborta o lote inteiro: commit parcial avançaria
// o cursor de FetchUnprocessed e poderia perder mensagens para sempre.
func (r *ProcessorRepository) SaveProcessedBatch(ctx context.Context, msgs []*model.NormalizedMessage) (saved, failed int, err error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, len(msgs), apperrors.Wrap("processor", "save_processed_batch_begin", err)
	}
	defer func() { _ = tx.Rollback() }()

	stmt := tx.StmtContext(ctx, r.stmtInsertProcessed)

	for _, msg := range msgs {
		args, err := processedInsertArgs(msg)
		if err != nil {
			return 0, len(msgs), apperrors.Wrap("processor", "save_processed_batch_encode", fmt.Errorf("msg_id=%d: %w", msg.MessageID, err))
		}
		_, execErr := stmt.ExecContext(ctx, args...)
		if execErr != nil {
			return 0, len(msgs), apperrors.Wrap("processor", "save_processed_batch_exec", fmt.Errorf("msg_id=%d: %w", msg.MessageID, execErr))
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

func decodeCoupons(raw sql.NullString) ([]model.Coupon, error) {
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	var coupons []model.Coupon
	if err := json.Unmarshal([]byte(raw.String), &coupons); err != nil {
		return nil, fmt.Errorf("coupon_codes: %w", err)
	}
	return coupons, nil
}

func decodeModifiers(raw sql.NullString) ([]model.Modifier, error) {
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	var modifiers []model.Modifier
	if err := json.Unmarshal([]byte(raw.String), &modifiers); err != nil {
		return nil, fmt.Errorf("modifiers: %w", err)
	}
	return modifiers, nil
}

func decodeVirtualCurrency(raw sql.NullString) (*model.VirtualCurrency, error) {
	if !raw.Valid || raw.String == "" {
		return nil, nil
	}
	var vc model.VirtualCurrency
	if err := json.Unmarshal([]byte(raw.String), &vc); err != nil {
		return nil, fmt.Errorf("virtual_currency: %w", err)
	}
	return &vc, nil
}

func nullStringValue(raw sql.NullString) string {
	if !raw.Valid {
		return ""
	}
	return raw.String
}

// ListProcessedMessages retorna mensagens processadas com paginação e filtro
// opcional por tipo e canal. Resultados ordenados por posted_at DESC.
func (r *ProcessorRepository) ListProcessedMessages(ctx context.Context, channelID int64, msgType string, limit, offset int) ([]*model.ProcessedMessage, error) {
	var query strings.Builder
	query.WriteString(`SELECT id, raw_message_id, channel_id, message_id, message_type,
		text_clean, text_length, media_type, has_url, has_price, has_coupon,
		price_amount, price_currency, posted_at, processed_at,
		price_original, price_discount, coupon_code, coupon_codes, payment_method, shipping,
		installments, discount_percent, modifiers, merchant, product_name, product_name_confidence,
		virtual_currency, webpage_url, webpage_title, webpage_desc, is_promotional, is_duplicate,
		photo_id, shipping_free, installments_n, installments_value, is_recurring,
		valid_from, valid_until, flash, recurrence_pattern, recurrence_group_id, seasonal_tag
		FROM processed_messages`)
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
		query.WriteString(" WHERE " + conditions[0])
		for _, c := range conditions[1:] {
			query.WriteString(" AND " + c) // #nosec G202 — conditions are hardcoded column names, values use ?
		}
	}
	query.WriteString(" ORDER BY posted_at DESC LIMIT ? OFFSET ?")
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, apperrors.Wrap("processor", "list_processed_messages", err)
	}
	defer func() { _ = rows.Close() }()

	var msgs []*model.ProcessedMessage
	for rows.Next() {
		var (
			m               model.ProcessedMessage
			posted          string
			procAt          string
			hasURL          int
			hasPrc          int
			hasCpn          int
			isPromo         int
			isDup           int
			shipFr          int
			instN           int
			isRec           int
			price           sql.NullInt64
			curr            sql.NullString
			couponCodesText sql.NullString
			modifiersText   sql.NullString
			virtualCurrency sql.NullString
			webpageURL      sql.NullString
			webpageTitle    sql.NullString
			webpageDesc     sql.NullString
			instVal         sql.NullInt64
			validFrom       sql.NullString
			validUntil      sql.NullString
			flash           int
			recPattern      sql.NullString
			recGroup        sql.NullInt64
			seasonal        sql.NullString
		)
		if err := rows.Scan(&m.ID, &m.RawMessageID, &m.ChannelID, &m.MessageID, &m.MessageType,
			&m.TextClean, &m.TextLength, &m.MediaType, &hasURL, &hasPrc, &hasCpn,
			&price, &curr, &posted, &procAt,
			&m.PriceOriginal, &m.PriceDiscount, &m.CouponCode, &couponCodesText, &m.PaymentMethod, &m.Shipping,
			&m.Installments, &m.DiscountPct, &modifiersText, &m.Merchant, &m.ProductName, &m.ProductNameConfidence,
			&virtualCurrency, &webpageURL, &webpageTitle, &webpageDesc, &isPromo, &isDup,
			&m.PhotoID, &shipFr, &instN, &instVal, &isRec,
			&validFrom, &validUntil, &flash, &recPattern, &recGroup, &seasonal); err != nil {
			return nil, apperrors.Wrap("processor", "scan_processed_message", err)
		}
		if m.CouponCodes, err = decodeCoupons(couponCodesText); err != nil {
			return nil, apperrors.Wrap("processor", "decode_processed_message", fmt.Errorf("msg_id=%d: %w", m.MessageID, err))
		}
		if m.Modifiers, err = decodeModifiers(modifiersText); err != nil {
			return nil, apperrors.Wrap("processor", "decode_processed_message", fmt.Errorf("msg_id=%d: %w", m.MessageID, err))
		}
		if m.VirtualCurrency, err = decodeVirtualCurrency(virtualCurrency); err != nil {
			return nil, apperrors.Wrap("processor", "decode_processed_message", fmt.Errorf("msg_id=%d: %w", m.MessageID, err))
		}
		m.WebpageURL = nullStringValue(webpageURL)
		m.WebpageTitle = nullStringValue(webpageTitle)
		m.WebpageDesc = nullStringValue(webpageDesc)
		m.HasURL = hasURL != 0
		m.HasPrice = hasPrc != 0
		m.HasCoupon = hasCpn != 0
		m.IsPromotional = isPromo != 0
		m.IsDuplicate = isDup != 0
		m.ShippingFree = shipFr != 0
		m.IsRecurring = isRec != 0
		m.Flash = flash != 0
		if validFrom.Valid {
			m.ValidFrom = model.ParseDBTime(validFrom.String)
		}
		if validUntil.Valid {
			m.ValidUntil = model.ParseDBTime(validUntil.String)
		}
		if recPattern.Valid {
			m.RecurrencePattern = recPattern.String
		}
		if recGroup.Valid {
			m.RecurrenceGroupID = recGroup.Int64
		}
		if seasonal.Valid {
			m.SeasonalTag = seasonal.String
		}
		m.InstallmentsN = instN
		if instVal.Valid {
			m.InstallmentsValue = instVal.Int64
		}
		m.Url = processedURLRegex.FindString(m.TextClean)
		if m.Url == "" {
			m.Url = m.WebpageURL
		}
		if price.Valid {
			m.PriceAmount = price.Int64
		}
		if curr.Valid {
			m.PriceCurrency = curr.String
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
func (r *ProcessorRepository) CountProcessedByType(ctx context.Context) ([]model.ProcessedTypeStats, error) {
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
func (r *ProcessorRepository) CountProcessedMessages(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM processed_messages`).Scan(&n); err != nil {
		return 0, apperrors.Wrap("processor", "count_processed_messages", err)
	}
	return n, nil
}

// GetPhotoMetadata retorna os campos MTProto de imagem para uma mensagem processada.
// Quando o raw payload está disponível, ele é a fonte preferida para evitar perda
// de precisão em inteiros MTProto de 64 bits gravados antes da correção de
// DecodePayloadMap.
func (r *ProcessorRepository) GetPhotoMetadata(ctx context.Context, processedMsgID int64) (*model.PhotoMetadata, error) {
	var (
		m          model.PhotoMetadata
		rawPayload sql.NullString
	)
	err := r.db.QueryRowContext(ctx, `
		SELECT pm.id, pm.message_id, pm.channel_id,
		       pm.photo_id, pm.photo_access_hash, pm.photo_file_ref, pm.photo_dcid,
		       rm.payload
		FROM processed_messages pm
		LEFT JOIN raw_messages rm ON rm.id = pm.raw_message_id
		WHERE pm.id = ?`, processedMsgID).Scan(
		&m.ID, &m.MsgID, &m.ChannelID, &m.PhotoID, &m.AccessHash, &m.FileReference, &m.DCID, &rawPayload)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, apperrors.Wrap("processor", "get_photo_metadata", err)
	}
	if rawPayload.Valid && rawPayload.String != "" {
		payload, derr := model.DecodePayloadMap([]byte(rawPayload.String))
		if derr == nil {
			if msg, ok := model.ExtractPayloadMessage(payload); ok {
				photo := model.ExtractPhotoFields(msg)
				if photo.PhotoID != 0 {
					m.PhotoID = photo.PhotoID
					m.AccessHash = photo.AccessHash
					m.FileReference = photo.FileRef
					m.DCID = photo.DCID
				}
			}
		}
	}
	return &m, nil
}

// ListPendingPhotoMetadataForBackfill lista fotos distintas ainda ausentes no
// photo_cache. Os metadados são derivados de raw_messages quando possível para
// corrigir bancos antigos com photo_id/access_hash arredondados por float64.
func (r *ProcessorRepository) ListPendingPhotoMetadataForBackfill(ctx context.Context, limit int) ([]model.PhotoMetadata, error) {
	cached, err := r.photoCacheIDSet(ctx)
	if err != nil {
		return nil, err
	}

	rows, err := r.db.QueryContext(ctx, `
		SELECT id
		FROM processed_messages
		WHERE photo_id > 0 OR media_type = 'photo'
		ORDER BY posted_at DESC`)
	if err != nil {
		return nil, apperrors.Wrap("processor", "list_photo_metadata_for_backfill", err)
	}
	var ids []int64
	for rows.Next() {
		var processedMsgID int64
		if err := rows.Scan(&processedMsgID); err != nil {
			_ = rows.Close()
			return nil, apperrors.Wrap("processor", "scan_photo_metadata_for_backfill", err)
		}
		ids = append(ids, processedMsgID)
	}
	if err := rows.Close(); err != nil {
		return nil, apperrors.Wrap("processor", "close_photo_metadata_for_backfill", err)
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap("processor", "iterate_photo_metadata_for_backfill", err)
	}

	var photos []model.PhotoMetadata
	seen := make(map[int64]bool)
	for _, processedMsgID := range ids {
		if limit > 0 && len(photos) >= limit {
			break
		}
		meta, err := r.GetPhotoMetadata(ctx, processedMsgID)
		if err != nil {
			return nil, err
		}
		if meta == nil || meta.PhotoID == 0 || meta.AccessHash == 0 || meta.DCID == 0 {
			continue
		}
		if seen[meta.PhotoID] || cached[meta.PhotoID] {
			continue
		}
		seen[meta.PhotoID] = true
		photos = append(photos, *meta)
	}
	return photos, nil
}

func (r *ProcessorRepository) photoCacheIDSet(ctx context.Context) (map[int64]bool, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT photo_id FROM photo_cache`)
	if err != nil {
		return nil, apperrors.Wrap("processor", "list_photo_cache_ids", err)
	}
	defer func() { _ = rows.Close() }()

	cached := make(map[int64]bool)
	for rows.Next() {
		var photoID int64
		if err := rows.Scan(&photoID); err != nil {
			return nil, apperrors.Wrap("processor", "scan_photo_cache_id", err)
		}
		cached[photoID] = true
	}
	if err := rows.Err(); err != nil {
		return nil, apperrors.Wrap("processor", "iterate_photo_cache_ids", err)
	}
	return cached, nil
}

// GetPhotoID retorna o photo_id (Telegram) de uma mensagem processada.
// Retorna (0, nil) se a mensagem não existe ou não tem foto.
func (r *ProcessorRepository) GetPhotoID(ctx context.Context, processedMsgID int64) (int64, error) {
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

// GetInlineThumb retorna o thumbnail inline (Type "i") da mensagem processada.
// Retorna (nil, nil) se a mensagem não tem inline thumb ou não existe.
func (r *ProcessorRepository) GetInlineThumb(ctx context.Context, processedMsgID int64) ([]byte, error) {
	var thumb []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT inline_thumb FROM processed_messages
		WHERE id = ? AND inline_thumb IS NOT NULL`, processedMsgID).Scan(&thumb)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, apperrors.Wrap("processor", "get_inline_thumb", err)
	}
	return thumb, nil
}

// PhotoMetadataStats agrega a cobertura de metadados de foto.
func (r *ProcessorRepository) PhotoMetadataStats(ctx context.Context) (model.PhotoStats, error) {
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

// GetPhotoData retorna os bytes da foto em cache (photo_cache) pelo photo_id.
// Retorna (nil, nil) quando não há entrada no cache (sql.ErrNoRows).
// Chamadas infrequentes: usa r.db.QueryRowContext direto, sem prepared statement.
func (r *ProcessorRepository) GetPhotoData(ctx context.Context, photoID int64) ([]byte, error) {
	var data []byte
	err := r.db.QueryRowContext(ctx, `
		SELECT data FROM photo_cache WHERE photo_id = ?`, photoID).Scan(&data)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, apperrors.Wrap("processor", "get_photo_data", err)
	}
	return data, nil
}

// SavePhotoData persiste (upsert) os bytes de uma foto no cache pelo photo_id.
// O TTL fixo de 30 dias impede crescimento indefinido; acessos futuros renovam
// a expiração ao gravar novamente.
func (r *ProcessorRepository) SavePhotoData(ctx context.Context, photoID int64, data []byte) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO photo_cache (photo_id, data, expires_at) VALUES (?, ?, datetime('now', '+30 days'))
		ON CONFLICT(photo_id) DO UPDATE SET
			data = excluded.data,
			updated_at = datetime('now'),
			expires_at = datetime('now', '+30 days')`,
		photoID, data)
	if err != nil {
		return apperrors.Wrap("processor", "save_photo_data", err)
	}
	return nil
}

// CleanExpiredPhotoCache remove fotos cujo TTL expirou.
// expires_at NULL significa cache sem expiração e é preservado.
func (r *ProcessorRepository) CleanExpiredPhotoCache(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM photo_cache
		WHERE expires_at IS NOT NULL AND expires_at < datetime('now')`)
	if err != nil {
		return 0, apperrors.Wrap("processor", "clean_expired_photo_cache", err)
	}
	deleted, err := res.RowsAffected()
	if err != nil {
		return 0, apperrors.Wrap("processor", "clean_expired_photo_cache_rows", err)
	}
	return deleted, nil
}

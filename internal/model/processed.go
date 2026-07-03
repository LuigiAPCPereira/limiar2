package model

import "time"

// ProcessedMessage representa uma mensagem normalizada e classificada pelo processor.
type ProcessedMessage struct {
	ID                int64     `json:"id"`
	RawMessageID      int64     `json:"raw_message_id"`
	ChannelID         int64     `json:"channel_id"`
	MessageID         int64     `json:"message_id"`
	MessageType       string    `json:"message_type"`
	TextClean         string    `json:"text_clean"`
	TextLength        int       `json:"text_length"`
	MediaType         string    `json:"media_type"`
	PhotoID           int64     `json:"photo_id"`
	HasURL            bool      `json:"has_url"`
	Url               string    `json:"url"`
	HasPrice          bool      `json:"has_price"`
	HasCoupon         bool      `json:"has_coupon"`
	PriceAmount       int64     `json:"price_amount"`
	PriceCurrency     string    `json:"price_currency"`
	PostedAt          time.Time `json:"posted_at"`
	ProcessedAt       time.Time `json:"processed_at"`
	PriceOriginal     int64     `json:"price_original"`
	PriceDiscount     int       `json:"price_discount"`
	CouponCode        string    `json:"coupon_code"`
	PaymentMethod     string    `json:"payment_method"`
	Shipping          string    `json:"shipping"`
	Installments      string    `json:"installments"`
	DiscountPct       int       `json:"discount_percent"`
	Merchant          string    `json:"merchant"`
	ProductName       string    `json:"product_name"`
	IsDuplicate       bool      `json:"is_duplicate"`
	ShippingFree      bool      `json:"shipping_free"`
	InstallmentsN     int       `json:"installments_n"`
	InstallmentsValue int64     `json:"installments_value"`
	IsRecurring       bool      `json:"is_recurring"`
	ValidFrom         time.Time `json:"valid_from,omitempty"`
	ValidUntil        time.Time `json:"valid_until,omitempty"`
	Flash             bool      `json:"flash"`
	RecurrencePattern string    `json:"recurrence_pattern,omitempty"`
	RecurrenceGroupID int64     `json:"recurrence_group_id,omitempty"`
	SeasonalTag       string    `json:"seasonal_tag,omitempty"`
	PhotoAccessHash   int64     `json:"photo_access_hash"`
	PhotoFileRef      string    `json:"photo_file_ref"`
	PhotoDCID         int64     `json:"photo_dcid"`
	InlineThumb       []byte    `json:"inline_thumb,omitempty"`
}

// ProcessedTypeStats contém contagem de mensagens processadas por tipo.
type ProcessedTypeStats struct {
	MessageType string `json:"message_type"`
	Count       int64  `json:"count"`
}

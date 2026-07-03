package model

import "time"

// NormalizedMessage é a estrutura canônica produzida pelo Estágio 1 do
// processor. Representa qualquer payload bruto (Shape A ou B) em formato
// limpo e uniforme, pronto para classificação e persistência.
type NormalizedMessage struct {
	RawMessageID      int64
	MessageID         int64
	ChannelID         int64
	PostedAt          time.Time
	ReceivedAt        time.Time
	ProcessedAt       time.Time
	Text              string
	TextLength        int
	MediaType         string // "photo"|"video"|"document"|"poll"|"webpage"|"none"
	PhotoID           int64
	PhotoAccessHash   int64  // MTProto access_hash para download sob demanda
	PhotoFileRef      string // MTProto file_reference (base64, renovável)
	PhotoDCID         int    // MTProto data center ID
	Views             int
	Forwards          int
	ReplyToMsgID      int64
	HasURL            bool
	HasPrice          bool
	HasCoupon         bool
	PriceAmount       int64     // centavos BRL — preço final (o que o usuário paga)
	PriceOriginal     int64     // centavos BRL — preço "De" (0 se não há dual price)
	PriceDiscount     int       // percentual de desconto 0-100 (calculado se dual price)
	CouponCode        string    // código do cupom extraído (vazio se não há)
	PaymentMethod     string    // "pix" | ""
	Shipping          string    // "frete_gratis" | "frete_gratis_prime" | ""
	Installments      string    // "9x_sem_juros" | ""
	DiscountPct       int       // "20% OFF" → 20 (do texto, não-calculado)
	IsCashback        bool      // cashback mencionado
	URLHash           string    // SHA-256 da primeira URL normalizada
	Merchant          string    // merchant inferido do domínio da URL
	ProductName       string    // nome do produto (heurística)
	IsDuplicate       bool      // mesma URL já processada em outro canal
	FeedEligible      bool      // elegível para o feed (deal_complete ou deal_no_coupon)
	InlineThumb       []byte    // thumbnail inline do Telegram (Type "i", ~230 bytes, sempre disponível)
	MessageType       string    // preenchido por Classify
	Synthesis         string    // JSON serializado de SynthesizedPromotion
	ShippingFree      bool      // frete grátis (boolean)
	InstallmentsN     int       // número de parcelas (0 se não há)
	InstallmentsValue int64     // valor da parcela em centavos (0 se não há)
	IsRecurring       bool      // promoção recorrente (ex: "toda semana")
	ValidFrom         time.Time // início explícito da validade, quando o texto informa
	ValidUntil        time.Time // fim explícito da validade, quando o texto informa
	Flash             bool      // sinal temporal sem data explícita ("corre", "relâmpago")
	RecurrencePattern string    // "weekly/monday" | "monthly" | "daily" | ...
	RecurrenceGroupID int64     // hash estável para reagrupar recorrências retroativamente
	SeasonalTag       string    // "black_friday" | "christmas" | ...
}

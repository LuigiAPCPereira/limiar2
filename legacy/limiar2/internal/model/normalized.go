package model

import "time"

// Coupon representa um cupom extraído do texto da promoção.
type Coupon struct {
	Code           string `json:"code"`
	DiscountType   string `json:"discount_type"`  // "code_only" | "percent" | "fixed_brl"
	DiscountValue  int64  `json:"discount_value"` // centavos ou percentual inteiro
	RequiresAction bool   `json:"requires_action"`
}

// Modifier representa uma condição comercial determinística da promoção.
type Modifier struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

// VirtualCurrency representa moedas/cashback em moedas de marketplace.
type VirtualCurrency struct {
	Platform string `json:"platform"`
	Amount   int64  `json:"amount"`
	CapBRL   int64  `json:"cap_brl"`
	Type     string `json:"type"`
}

// WebpageInfo preserva os metadados Media.Webpage do payload bruto.
type WebpageInfo struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// NormalizedMessage é a estrutura canônica produzida pelo Estágio 1 do
// processor. Representa qualquer payload bruto (Shape A ou B) em formato
// limpo e uniforme, pronto para classificação e persistência.
type NormalizedMessage struct {
	RawMessageID          int64
	MessageID             int64
	ChannelID             int64
	PostedAt              time.Time
	ReceivedAt            time.Time
	ProcessedAt           time.Time
	Text                  string
	TextLength            int
	MediaType             string // "photo"|"video"|"document"|"poll"|"webpage"|"none"
	PhotoID               int64
	PhotoAccessHash       int64  // MTProto access_hash para download sob demanda
	PhotoFileRef          string // MTProto file_reference (base64, renovável)
	PhotoDCID             int    // MTProto data center ID
	Views                 int
	Forwards              int
	ReplyToMsgID          int64
	HasURL                bool
	HasPrice              bool
	HasCoupon             bool
	PriceAmount           int64            // centavos BRL — preço final (o que o usuário paga)
	PriceOriginal         int64            // centavos BRL — preço "De" (0 se não há dual price)
	PriceDiscount         int              // percentual de desconto 0-100 (calculado se dual price)
	CouponCode            string           // código do cupom extraído (vazio se não há)
	CouponCodes           []Coupon         // múltiplos cupons estruturados
	PaymentMethod         string           // "pix" | ""
	Shipping              string           // "frete_gratis" | "frete_gratis_prime" | ""
	Installments          string           // "9x_sem_juros" | ""
	DiscountPct           int              // "20% OFF" → 20 (do texto, não-calculado)
	IsCashback            bool             // cashback mencionado
	Modifiers             []Modifier       // modifiers estruturados
	URLHash               string           // SHA-256 da primeira URL normalizada
	Merchant              string           // merchant inferido do domínio da URL
	ProductName           string           // nome do produto (heurística)
	ProductNameConfidence float64          // score 0.0–1.0 do CRE
	IsDuplicate           bool             // mesma URL já processada em outro canal
	FeedEligible          bool             // elegível para o feed (deal_complete ou deal_no_coupon)
	InlineThumb           []byte           // thumbnail inline do Telegram (Type "i", ~230 bytes, sempre disponível)
	VirtualCurrency       *VirtualCurrency // moedas Shopee/AliExpress (nil se não há)
	WebpageURL            string           // Media.Webpage.URL
	WebpageTitle          string           // Media.Webpage.Title
	WebpageDesc           string           // Media.Webpage.Description
	IsPromotional         bool             // deal_* → true
	CanonicalURL          string           // URL canônica resolvida sem tracking/affiliate
	URLTitle              string           // <title> HTML da URL resolvida
	URLResolved           bool             // true quando a resolução HTTP/cache foi bem-sucedida
	MessageType           string           // preenchido por Classify
	Synthesis             string           // JSON serializado de SynthesizedPromotion
	ShippingFree          bool             // frete grátis (boolean)
	InstallmentsN         int              // número de parcelas (0 se não há)
	InstallmentsValue     int64            // valor da parcela em centavos (0 se não há)
	IsRecurring           bool             // promoção recorrente (ex: "toda semana")
	ValidFrom             time.Time        // início explícito da validade, quando o texto informa
	ValidUntil            time.Time        // fim explícito da validade, quando o texto informa
	Flash                 bool             // sinal temporal sem data explícita ("corre", "relâmpago")
	RecurrencePattern     string           // "weekly/monday" | "monthly" | "daily" | ...
	RecurrenceGroupID     int64            // hash estável para reagrupar recorrências retroativamente
	SeasonalTag           string           // "black_friday" | "christmas" | ...
}

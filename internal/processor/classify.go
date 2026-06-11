package processor

import "regexp"

// MessageType classifica cada mensagem em um cluster exclusivo.
// A taxonomia é baseada na análise de 6674 mensagens reais (§6.5 da ideação).
type MessageType string

const (
	TypeDealComplete   MessageType = "deal_complete"
	TypeDealNoCoupon   MessageType = "deal_no_coupon"
	TypeDealNoPrice    MessageType = "deal_no_price"
	TypeCouponOnly     MessageType = "coupon_only"
	TypeCategoryHeader MessageType = "category_header"
	TypeCouponExpired  MessageType = "coupon_expired"
	TypeCommentary     MessageType = "commentary"
	TypeVideo          MessageType = "video"
	TypeDocument       MessageType = "document"
	TypePoll           MessageType = "poll"
	TypeAdminMeta      MessageType = "admin_meta"
)

var (
	reAdminMeta = regexp.MustCompile(`(?i)(regras_grupo|cupons_hoje|grupos_whatsapp|canal_telegram` +
		`|entre\s+no\s+grupo|grupo\s+de\s+ofertas|grupo\s+do\s+whatsapp` +
		`|canal\s+de\s+ofertas|salvando\s+o\s+bolso|quase\s+\d+\s*mil` +
		`|pessoas\s+somando|noss[oa]\s+grupo|noss[oa]\s+canal)`)
	reCouponHeader = regexp.MustCompile(`(?i)^(?:🎟️?\s*)?(?:cup[ao]m|cupons|c[oó]digo|code)\b`)
)

// Classify aplica a cascata de checks para determinar o MessageType.
// A ordem importa: checks mais específicos primeiro, genéricos por último.
func Classify(nm *NormalizedMessage) MessageType {
	text := nm.Text

	// 1. Admin meta (regras do grupo, links de whatsapp, etc.)
	if reAdminMeta.MatchString(text) {
		return TypeAdminMeta
	}

	// 2. Cupom expirado (menciona esgotado/acabou, sem URL de produto)
	if isExpired(text) && !nm.HasURL {
		return TypeCouponExpired
	}

	// 3. Cupom sem produto (distribuição de cupom genérico, ex: "Cupom Shopee R$10 OFF - CODE")
	if nm.HasCoupon && nm.HasURL && reCouponHeader.MatchString(text) {
		return TypeCouponOnly
	}

	// 4. Deals (com URL de produto)
	if nm.HasURL {
		switch {
		case nm.HasPrice && nm.HasCoupon:
			return TypeDealComplete
		case nm.HasPrice:
			return TypeDealNoCoupon
		default:
			return TypeDealNoPrice
		}
	}

	// 4. Media types (check before category_header — short texts with media
	// should not be classified as headers)
	switch nm.MediaType {
	case "video":
		return TypeVideo
	case "document":
		return TypeDocument
	case "poll":
		return TypePoll
	}

	// 5. Category header (texto curto, sem URL/preço/esgotado)
	if len(text) < 50 && !nm.HasURL && !nm.HasPrice && !isExpired(text) {
		return TypeCategoryHeader
	}

	// 6. Fallback
	return TypeCommentary
}

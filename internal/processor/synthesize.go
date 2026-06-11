package processor

import (
	"net/url"
	"regexp"
	"strings"
	"unicode/utf8"
)

// SynthesizedPromotion é o resumo estruturado de uma promoção, pronto para
// consumo pelo limiar-api e frontend. Produzido pelo Estágio 3 do pipeline.
type SynthesizedPromotion struct {
	ProductName     string `json:"product_name,omitempty"`
	Merchant        string `json:"merchant,omitempty"`
	PriceOriginal   int64  `json:"price_original,omitempty"`
	PriceFinal      int64  `json:"price_final,omitempty"`
	DiscountPercent int    `json:"discount_percent,omitempty"`
	CouponCode      string `json:"coupon_code,omitempty"`
	PaymentMethod   string `json:"payment_method,omitempty"`
	Shipping        string `json:"shipping,omitempty"`
	Installments    string `json:"installments,omitempty"`
	URL             string `json:"url,omitempty"`
}

// merchantDomains mapeia sufixos de domínio → nome do merchant.
// A busca é feita por sufixo (hasSuffix) para cobrir subdomínios.
var merchantDomains = []struct {
	suffix   string
	merchant string
}{
	{"meli.la", "mercadolivre"},
	{"mercadolivre.com", "mercadolivre"},
	{"mercadolivre.com.br", "mercadolivre"},
	{"shopee.com.br", "shopee"},
	{"amzn.to", "amazon"},
	{"amazon.com.br", "amazon"},
	{"amzn.divulgador.link", "amazon"},
	{"amzlink.to", "amazon"},
	{"aliexpress.com", "aliexpress"},
	{"magazineluiza.onelink.me", "magalu"},
	{"divulgador.magalu.com", "magalu"},
	{"magalu.com", "magalu"},
	{"tidd.ly", "tiddly"},
	{"bit.ly", "bitly"},
	{"eioferta.com.br", "eioferta"},
}

// detectMerchant infere o merchant a partir da primeira URL no texto.
func detectMerchant(text string) string {
	raw := reURL.FindString(text)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Host)
	for _, d := range merchantDomains {
		if strings.HasSuffix(host, d.suffix) || host == d.suffix {
			return d.merchant
		}
	}
	return ""
}

// reEmojiStrip remove sequências de emojis e símbolos decorativos.
var reEmojiStrip = mustCompileEmoji()

func mustCompileEmoji() *regexp.Regexp {
	return regexp.MustCompile(
		`[\x{1F300}-\x{1F9FF}\x{2600}-\x{26FF}\x{2700}-\x{27BF}` +
			`\x{FE00}-\x{FE0F}\x{1FA00}-\x{1FA6F}\x{1FA70}-\x{1FAFF}` +
			`\x{200D}\x{20E3}\x{FE0F}]+`)
}

// extractProductName heurística para nome do produto:
// 1. Primeira linha do texto limpo
// 2. Remover emojis, URLs, preços, cupons
// 3. Trim e truncar a 80 caracteres
// 4. Se vazio → segunda linha
func extractProductName(text string) string {
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		name := cleanProductName(line)
		if name != "" {
			return name
		}
	}
	return ""
}

func cleanProductName(line string) string {
	// Remover URLs
	s := reURL.ReplaceAllString(line, "")
	// Remover preços
	s = rePrice.ReplaceAllString(s, "")
	// Remover cupons
	s = reCoupon.ReplaceAllString(s, "")
	// Remover emojis
	s = reEmojiStrip.ReplaceAllString(s, "")
	// Remover modificadores comuns
	s = rePix.ReplaceAllString(s, "")
	s = reFreteG.ReplaceAllString(s, "")
	s = reInstall.ReplaceAllString(s, "")
	// Limpar pontuação residual e whitespace
	s = strings.Map(func(r rune) rune {
		if r == '|' || r == '•' || r == '·' || r == '*' {
			return ' '
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	// Truncar a 80 runes
	if utf8.RuneCountInString(s) > 80 {
		runes := []rune(s)
		s = string(runes[:80])
		s = strings.TrimSpace(s)
	}
	return s
}

// Synthesize produz um SynthesizedPromotion a partir de uma NormalizedMessage.
func Synthesize(nm *NormalizedMessage) SynthesizedPromotion {
	firstURL := reURL.FindString(nm.Text)
	return SynthesizedPromotion{
		ProductName:     extractProductName(nm.Text),
		Merchant:        detectMerchant(nm.Text),
		PriceOriginal:   nm.PriceOriginal,
		PriceFinal:      nm.PriceAmount,
		DiscountPercent: nm.PriceDiscount,
		CouponCode:      nm.CouponCode,
		PaymentMethod:   nm.PaymentMethod,
		Shipping:        nm.Shipping,
		Installments:    nm.Installments,
		URL:             firstURL,
	}
}

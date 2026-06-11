package processor

import (
	"net/url"
	"strings"
)

// SynthesizedPromotion é o resumo estruturado de uma promoção, pronto para
// consumo pelo limiar-api e frontend. Produzido pelo Estágio 3 do pipeline.
// ProductName NÃO é extraído aqui — requer LLM (Fase 3).
type SynthesizedPromotion struct {
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
	{"magazinevoce.com.br", "magalu"},
	{"magalu.com", "magalu"},
	{"shein.com", "shein"},
	{"terabyteshop.com.br", "terabyte"},
	{"natura.divulgador.link", "natura"},
	{"steampowered.com", "steam"},
	{"epicgames.com", "epicgames"},
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

// Synthesize produz um SynthesizedPromotion a partir de uma NormalizedMessage.
func Synthesize(nm *NormalizedMessage) SynthesizedPromotion {
	firstURL := reURL.FindString(nm.Text)
	return SynthesizedPromotion{
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

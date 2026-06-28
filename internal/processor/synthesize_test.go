package processor

import "testing"

func TestDetectMerchant_Extended(t *testing.T) {
	tests := []struct {
		name string
		text string
		want string
	}{
		{"mercadolivre .com.br", "https://www.mercadolivre.com.br/produto", "mercadolivre"},
		{"amazon .com.br", "https://www.amazon.com.br/dp/B123", "amazon"},
		{"amazon divulgador", "https://amzn.divulgador.link/abc", "amazon"},
		{"amazon amzlink", "https://amzlink.to/abc", "amazon"},
		{"magalu divulgador", "https://divulgador.magalu.com/prod", "magalu"},
		{"magalu domain", "https://www.magalu.com/produto/123", "magalu"},
		{"natura", "https://natura.divulgador.link/abc", "natura"},
		{"steam", "https://store.steampowered.com/app/123", "steam"},
		{"epicgames", "https://store.epicgames.com/p/game", "epicgames"},
		{"tiddly", "https://tidd.ly/abc123", "tiddly"},
		{"bitly", "https://bit.ly/3xyzABC", "bitly"},
		{"eioferta", "https://www.eioferta.com.br/prod", "eioferta"},
		{"empty string", "", ""},
		{"multiple URLs returns first", "https://amzn.to/abc e https://s.shopee.com.br/xyz", "amazon"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := detectMerchant(tt.text)
			if got != tt.want {
				t.Errorf("detectMerchant(%q) = %q, want %q", tt.text, got, tt.want)
			}
		})
	}
}

func TestSynthesize_Complete(t *testing.T) {
	nm := &NormalizedMessage{
		Text:          "Promoção! https://amzn.to/xyz R$ 199,90 no Pix. Cupom: PROMO20",
		PriceAmount:   19990,
		PriceOriginal: 29990,
		PriceDiscount: 33,
		CouponCode:    "PROMO20",
		PaymentMethod: "pix",
		Shipping:      "frete_gratis",
		Installments:  "12x_sem_juros",
	}

	syn := Synthesize(nm)

	if syn.Merchant != "amazon" {
		t.Errorf("Merchant = %q, want amazon", syn.Merchant)
	}
	if syn.PriceFinal != 19990 {
		t.Errorf("PriceFinal = %d, want 19990", syn.PriceFinal)
	}
	if syn.PriceOriginal != 29990 {
		t.Errorf("PriceOriginal = %d, want 29990", syn.PriceOriginal)
	}
	if syn.DiscountPercent != 33 {
		t.Errorf("DiscountPercent = %d, want 33", syn.DiscountPercent)
	}
	if syn.CouponCode != "PROMO20" {
		t.Errorf("CouponCode = %q, want PROMO20", syn.CouponCode)
	}
	if syn.PaymentMethod != "pix" {
		t.Errorf("PaymentMethod = %q, want pix", syn.PaymentMethod)
	}
	if syn.Shipping != "frete_gratis" {
		t.Errorf("Shipping = %q, want frete_gratis", syn.Shipping)
	}
	if syn.Installments != "12x_sem_juros" {
		t.Errorf("Installments = %q, want 12x_sem_juros", syn.Installments)
	}
	if syn.URL != "https://amzn.to/xyz" {
		t.Errorf("URL = %q, want https://amzn.to/xyz", syn.URL)
	}
}

func TestSynthesize_NoURL(t *testing.T) {
	nm := &NormalizedMessage{
		Text:        "Promoção sem link R$ 50",
		PriceAmount: 5000,
	}

	syn := Synthesize(nm)

	if syn.URL != "" {
		t.Errorf("URL = %q, want empty", syn.URL)
	}
	if syn.Merchant != "" {
		t.Errorf("Merchant = %q, want empty", syn.Merchant)
	}
	if syn.PriceFinal != 5000 {
		t.Errorf("PriceFinal = %d, want 5000", syn.PriceFinal)
	}
}

func TestSynthesize_ZeroValues(t *testing.T) {
	nm := &NormalizedMessage{
		Text: "texto simples sem promoção",
	}

	syn := Synthesize(nm)

	if syn.Merchant != "" {
		t.Errorf("Merchant = %q, want empty", syn.Merchant)
	}
	if syn.PriceFinal != 0 {
		t.Errorf("PriceFinal = %d, want 0", syn.PriceFinal)
	}
	if syn.PriceOriginal != 0 {
		t.Errorf("PriceOriginal = %d, want 0", syn.PriceOriginal)
	}
	if syn.URL != "" {
		t.Errorf("URL = %q, want empty", syn.URL)
	}
}

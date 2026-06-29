package processor

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/limiar/collector/internal/model"
)

func TestNormalize_ShapeA(t *testing.T) {
	payload := map[string]any{
		"ID":       float64(12345),
		"Message":  "🔥 Celular Samsung R$ 1.234,56\nhttps://amzn.to/abc\nCupom: SAMSUNG10",
		"PeerID":   map[string]any{"ChannelID": float64(999)},
		"Date":     float64(1717891200),
		"Media":    nil,
		"Views":    float64(500),
		"Forwards": float64(10),
	}
	raw := makeRaw(t, 1, 999, 12345, payload)

	nm, err := Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize falhou: %v", err)
	}

	if nm.MessageID != 12345 {
		t.Errorf("MessageID = %d, quer 12345", nm.MessageID)
	}
	if nm.ChannelID != 999 {
		t.Errorf("ChannelID = %d, quer 999", nm.ChannelID)
	}
	if nm.PostedAt.Unix() != 1717891200 {
		t.Errorf("PostedAt = %v, quer unix 1717891200", nm.PostedAt)
	}
	if nm.MediaType != "none" {
		t.Errorf("MediaType = %q, quer 'none'", nm.MediaType)
	}
	if nm.Views != 500 {
		t.Errorf("Views = %d, quer 500", nm.Views)
	}
	if !nm.HasURL {
		t.Error("HasURL = false, quer true")
	}
	if !nm.HasPrice {
		t.Error("HasPrice = false, quer true")
	}
	if nm.PriceAmount != 123456 {
		t.Errorf("PriceAmount = %d, quer 123456 (centavos de R$ 1.234,56)", nm.PriceAmount)
	}
	if !nm.HasCoupon {
		t.Error("HasCoupon = false, quer true")
	}
	if nm.CouponCode != "SAMSUNG10" {
		t.Errorf("CouponCode = %q, quer SAMSUNG10", nm.CouponCode)
	}
	if nm.Merchant != "amazon" {
		t.Errorf("Merchant = %q, quer amazon", nm.Merchant)
	}
	if nm.URLHash == "" {
		t.Error("URLHash vazio, esperava hash")
	}
}

func TestNormalize_ShapeB(t *testing.T) {
	payload := map[string]any{
		"Updates": []any{
			map[string]any{
				"Message": map[string]any{
					"ID":      float64(54321),
					"Message": "Fone bluetooth R$ 29,90 https://s.shopee.com.br/xyz",
					"PeerID":  map[string]any{"ChannelID": float64(777)},
					"Date":    float64(1717977600),
					"Media": map[string]any{
						"Photo": map[string]any{"ID": float64(888)},
					},
				},
			},
		},
	}
	raw := makeRaw(t, 2, 777, 54321, payload)

	nm, err := Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize falhou: %v", err)
	}

	if nm.MessageID != 54321 {
		t.Errorf("MessageID = %d, quer 54321", nm.MessageID)
	}
	if nm.ChannelID != 777 {
		t.Errorf("ChannelID = %d, quer 777", nm.ChannelID)
	}
	if nm.MediaType != "photo" {
		t.Errorf("MediaType = %q, quer 'photo'", nm.MediaType)
	}
	if nm.PhotoID != 888 {
		t.Errorf("PhotoID = %d, quer 888", nm.PhotoID)
	}
	if !nm.HasURL {
		t.Error("HasURL = false, quer true")
	}
	if !nm.HasPrice {
		t.Error("HasPrice = false, quer true")
	}
	if nm.PriceAmount != 2990 {
		t.Errorf("PriceAmount = %d, quer 2990 (centavos de R$ 29,90)", nm.PriceAmount)
	}
	if nm.Merchant != "shopee" {
		t.Errorf("Merchant = %q, quer shopee", nm.Merchant)
	}
}

func TestNormalize_TextCleaning(t *testing.T) {
	text := "Linha 1\n\n\n\n\nLinha 2\u00a0com\u00a0NBSP"
	payload := map[string]any{
		"ID":      float64(1),
		"Message": text,
		"PeerID":  map[string]any{"ChannelID": float64(1)},
		"Date":    float64(0),
	}
	raw := makeRaw(t, 10, 1, 1, payload)

	nm, err := Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize falhou: %v", err)
	}

	expected := "Linha 1\n\nLinha 2 com NBSP"
	if nm.Text != expected {
		t.Errorf("Text = %q, quer %q", nm.Text, expected)
	}
}

func TestNormalize_ReplyTo(t *testing.T) {
	payload := map[string]any{
		"ID":      float64(100),
		"Message": "Esgotado!",
		"PeerID":  map[string]any{"ChannelID": float64(1)},
		"Date":    float64(1717891200),
		"ReplyTo": map[string]any{"ReplyToMsgID": float64(99)},
	}
	raw := makeRaw(t, 20, 1, 100, payload)

	nm, err := Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize falhou: %v", err)
	}
	if nm.ReplyToMsgID != 99 {
		t.Errorf("ReplyToMsgID = %d, quer 99", nm.ReplyToMsgID)
	}
}


func TestNormalize_MediaTypes(t *testing.T) {
	tests := []struct {
		name  string
		media any
		want  string
	}{
		{"nil media", nil, "none"},
		{"video", map[string]any{"Video": map[string]any{"ID": float64(1)}}, "video"},
		{"document", map[string]any{"Document": map[string]any{"ID": float64(1)}}, "document"},
		{"poll", map[string]any{"Poll": map[string]any{"ID": float64(1)}}, "poll"},
		{"webpage", map[string]any{"Webpage": map[string]any{"URL": "https://x.com"}}, "webpage"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			payload := map[string]any{
				"ID":      float64(1),
				"Message": "test",
				"PeerID":  map[string]any{"ChannelID": float64(1)},
				"Date":    float64(0),
				"Media":   tt.media,
			}
			raw := makeRaw(t, 1, 1, 1, payload)
			nm, err := Normalize(raw)
			if err != nil {
				t.Fatalf("Normalize falhou: %v", err)
			}
			if nm.MediaType != tt.want {
				t.Errorf("MediaType = %q, quer %q", nm.MediaType, tt.want)
			}
		})
	}
}

func TestNormalize_UnknownShape(t *testing.T) {
	payload := map[string]any{"Foo": "bar"}
	raw := makeRaw(t, 1, 1, 1, payload)

	_, err := Normalize(raw)
	if err == nil {
		t.Fatal("esperava erro para shape desconhecido")
	}
}

// --- Fase 2: Dual Price ---

func TestDualPrice(t *testing.T) {
	tests := []struct {
		name      string
		text      string
		wantOrig  int64
		wantFinal int64
		wantDisc  int
		wantHas   bool
	}{
		{
			"De por Pix",
			"De R$ 429 por R$ 208,92 no Pix",
			42900, 20892, 51, true,
		},
		{
			"De: por:",
			"De: R$ 1.048 por R$ 478 no Pix",
			104800, 47800, 54, true,
		},
		{
			"OFF em",
			"R$ 50 OFF em R$ 250: CODE123",
			25000, 20000, 20, true,
		},
		{
			"single price",
			"R$ 99,90",
			0, 9990, 0, true,
		},
		{
			"no price",
			"sem preço aqui",
			0, 0, 0, false,
		},
		{
			"De por à vista",
			"De R$ 200 à vista por R$ 150",
			20000, 15000, 25, true,
		},
		{
			"POR REAIS sem R$",
			"POR: 425 REAIS",
			0, 42500, 0, true,
		},
		{
			"por apenas sem R$",
			"por apenas 99,90",
			0, 9990, 0, true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nm := &NormalizedMessage{}
			extractPrices(tt.text, nm)
			if nm.HasPrice != tt.wantHas {
				t.Errorf("HasPrice = %v, quer %v", nm.HasPrice, tt.wantHas)
			}
			if nm.PriceOriginal != tt.wantOrig {
				t.Errorf("PriceOriginal = %d, quer %d", nm.PriceOriginal, tt.wantOrig)
			}
			if nm.PriceAmount != tt.wantFinal {
				t.Errorf("PriceAmount = %d, quer %d", nm.PriceAmount, tt.wantFinal)
			}
			if nm.PriceDiscount != tt.wantDisc {
				t.Errorf("PriceDiscount = %d, quer %d", nm.PriceDiscount, tt.wantDisc)
			}
		})
	}
}

// --- Fase 2: Coupon ---

func TestCouponExtraction(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		wantHas  bool
		wantCode string
	}{
		{"Cupom com espaço", "Cupom VEMPRAMAZON", true, "VEMPRAMAZON"},
		{"Cupom com dois-pontos", "Cupom: AEBR1", true, "AEBR1"},
		{"cupom minúsculo", "cupom CORREPRAPROMO", true, "CORREPRAPROMO"},
		{"Código", "Código: MEGABR08", true, "MEGABR08"},
		{"code", "Code DESCONTO20", true, "DESCONTO20"},
		{"sem cupom", "promoção sem código", false, ""},
		{"cupom com newline", "Cupom:\nRESGATE", true, "RESGATE"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nm := &NormalizedMessage{}
			extractCoupon(tt.text, nm)
			if nm.HasCoupon != tt.wantHas {
				t.Errorf("HasCoupon = %v, quer %v", nm.HasCoupon, tt.wantHas)
			}
			if nm.CouponCode != tt.wantCode {
				t.Errorf("CouponCode = %q, quer %q", nm.CouponCode, tt.wantCode)
			}
		})
	}
}

// --- Fase 2: Modifiers ---

func TestModifiers(t *testing.T) {
	tests := []struct {
		name       string
		text       string
		wantPay    string
		wantShip   string
		wantInst   string
		wantCash   bool
		wantDiscPc int
	}{
		{
			"Pix",
			"R$ 99 no Pix",
			"pix", "", "", false, 0,
		},
		{
			"Frete grátis",
			"Produto com frete grátis",
			"", "frete_gratis", "", false, 0,
		},
		{
			"Frete grátis Prime",
			"Frete grátis Prime para membros",
			"", "frete_gratis_prime", "", false, 0,
		},
		{
			"Parcelamento",
			"em até 12x sem juros",
			"", "", "12x_sem_juros", false, 0,
		},
		{
			"Cashback",
			"Com cashback de 5%",
			"", "", "", true, 0,
		},
		{
			"Desconto percentual",
			"20% OFF no produto",
			"", "", "", false, 20,
		},
		{
			"Combo Pix + frete",
			"R$ 50 no Pix com frete grátis",
			"pix", "frete_gratis", "", false, 0,
		},
		{
			"Sem modifiers",
			"Produto simples R$ 100",
			"", "", "", false, 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nm := &NormalizedMessage{}
			extractModifiers(tt.text, nm)
			if nm.PaymentMethod != tt.wantPay {
				t.Errorf("PaymentMethod = %q, quer %q", nm.PaymentMethod, tt.wantPay)
			}
			if nm.Shipping != tt.wantShip {
				t.Errorf("Shipping = %q, quer %q", nm.Shipping, tt.wantShip)
			}
			if nm.Installments != tt.wantInst {
				t.Errorf("Installments = %q, quer %q", nm.Installments, tt.wantInst)
			}
			if nm.IsCashback != tt.wantCash {
				t.Errorf("IsCashback = %v, quer %v", nm.IsCashback, tt.wantCash)
			}
			if nm.DiscountPct != tt.wantDiscPc {
				t.Errorf("DiscountPct = %d, quer %d", nm.DiscountPct, tt.wantDiscPc)
			}
		})
	}
}

// --- Fase 2: Merchant Detection ---

func TestDetectMerchant(t *testing.T) {
	tests := []struct {
		text string
		want string
	}{
		{"https://meli.la/abc123", "mercadolivre"},
		{"https://amzn.to/4kT8xyz", "amazon"},
		{"https://s.shopee.com.br/produto", "shopee"},
		{"https://a.aliexpress.com/item", "aliexpress"},
		{"https://magazineluiza.onelink.me/abc", "magalu"},
		{"https://onelink.shein.com/item", "shein"},
		{"https://www.terabyteshop.com.br/produto", "terabyte"},
		{"https://www.magazinevoce.com.br/prod", "magalu"},
		{"https://unknown-site.com/prod", ""},
		{"sem url aqui", ""},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := detectMerchant(tt.text)
			if got != tt.want {
				t.Errorf("detectMerchant(%q) = %q, quer %q", tt.text, got, tt.want)
			}
		})
	}
}

// --- Fase 2: URL Hash ---

func TestURLHash(t *testing.T) {
	// Mesma URL normalizada → mesmo hash
	h1 := computeURLHash("https://amzn.to/abc?tag=test")
	h2 := computeURLHash("https://amzn.to/abc?ref=x")
	if h1 == "" {
		t.Fatal("hash vazio para URL válida")
	}
	// Com tracking params diferentes, o hash deve ser igual se a base URL é a mesma
	// (tag e ref são removidos na normalização)
	if h1 != h2 {
		t.Errorf("hashes deveriam ser iguais para mesma URL base: %q vs %q", h1, h2)
	}

	// Sem URL → hash vazio
	h3 := computeURLHash("sem url aqui")
	if h3 != "" {
		t.Errorf("hash deveria ser vazio para texto sem URL: %q", h3)
	}
}

// --- Fase 2: parseBRL ---

func TestParseBRL(t *testing.T) {
	tests := []struct {
		input string
		want  int64
	}{
		{"1.234,56", 123456},
		{"83", 8300},
		{"99,90", 9990},
		{"0,01", 1},
		{"1.000", 100000},
		{"", 0},
		{"abc", 0},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseBRL(tt.input)
			if got != tt.want {
				t.Errorf("parseBRL(%q) = %d, quer %d", tt.input, got, tt.want)
			}
		})
	}
}

// --- Fase 2: Synthesize ---

func TestSynthesize(t *testing.T) {
	nm := &NormalizedMessage{
		Text:          "🔥 Fone Bluetooth R$ 29,90 no Pix\nhttps://s.shopee.com.br/xyz\nCupom: FONE5",
		PriceAmount:   2990,
		PriceOriginal: 5990,
		PriceDiscount: 50,
		CouponCode:    "FONE5",
		PaymentMethod: "pix",
		URLHash:       "abc123",
	}

	syn := Synthesize(nm)

	if syn.Merchant != "shopee" {
		t.Errorf("Merchant = %q, quer shopee", syn.Merchant)
	}
	if syn.PriceFinal != 2990 {
		t.Errorf("PriceFinal = %d, quer 2990", syn.PriceFinal)
	}
	if syn.PriceOriginal != 5990 {
		t.Errorf("PriceOriginal = %d, quer 5990", syn.PriceOriginal)
	}
	if syn.CouponCode != "FONE5" {
		t.Errorf("CouponCode = %q, quer FONE5", syn.CouponCode)
	}
	if syn.PaymentMethod != "pix" {
		t.Errorf("PaymentMethod = %q, quer pix", syn.PaymentMethod)
	}
	if syn.URL != "https://s.shopee.com.br/xyz" {
		t.Errorf("URL = %q, quer https://s.shopee.com.br/xyz", syn.URL)
	}
}

// makeRaw é um helper que cria um model.RawMessage com payload JSON.
func makeRaw(t *testing.T, id, channelID, messageID int64, payload map[string]any) *model.RawMessage {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return &model.RawMessage{
		ID:            id,
		ChannelID:     channelID,
		MessageID:     messageID,
		Payload:       data,
		ReceivedAt:    time.Now().UTC(),
		SchemaVersion: 1,
	}
}

func TestExtractPhotoMetadata(t *testing.T) {
	tests := []struct {
		name     string
		msg      map[string]any
		wantID   int64
		wantHash int64
		wantRef  string
		wantDC   int
	}{
		{
			name:     "sem media",
			msg:      map[string]any{"ID": float64(1)},
			wantID:   0,
			wantHash: 0,
			wantRef:  "",
			wantDC:   0,
		},
		{
			name: "media sem photo",
			msg: map[string]any{
				"Media": map[string]any{"Webpage": map[string]any{"URL": "https://x.com"}},
			},
			wantID: 0, wantHash: 0, wantRef: "", wantDC: 0,
		},
		{
			name: "photo completo",
			msg: map[string]any{
				"Media": map[string]any{
					"Photo": map[string]any{
						"ID":            float64(4985843018),
						"AccessHash":    float64(-7524180780),
						"FileReference": "AlCZaRIAAXQAaiW+TljSnSegJJ4ybQVsJymwX1s=",
						"DCID":          float64(2),
					},
				},
			},
			wantID:   4985843018,
			wantHash: -7524180780,
			wantRef:  "AlCZaRIAAXQAaiW+TljSnSegJJ4ybQVsJymwX1s=",
			wantDC:   2,
		},
		{
			name: "photo sem file_reference",
			msg: map[string]any{
				"Media": map[string]any{
					"Photo": map[string]any{
						"ID":         float64(123),
						"AccessHash": float64(456),
						"DCID":       float64(4),
					},
				},
			},
			wantID: 123, wantHash: 456, wantRef: "", wantDC: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id, hash, ref, dc := extractPhotoMetadata(tt.msg)
			if id != tt.wantID {
				t.Errorf("PhotoID = %d, want %d", id, tt.wantID)
			}
			if hash != tt.wantHash {
				t.Errorf("AccessHash = %d, want %d", hash, tt.wantHash)
			}
			if ref != tt.wantRef {
				t.Errorf("FileRef = %q, want %q", ref, tt.wantRef)
			}
			if dc != tt.wantDC {
				t.Errorf("DCID = %d, want %d", dc, tt.wantDC)
			}
		})
	}
}

func TestNewExtractors(t *testing.T) {
	tests := []struct {
		name              string
		text              string
		wantOrig          int64
		wantAmount        int64
		wantInstallments  int
		wantInstallmentsV float64
		wantShippingFree  bool
		wantDiscountPct   int
	}{
		{
			name:       "Dual price DE POR",
			text:       "DE R$ 689 POR R$ 440",
			wantOrig:   68900,
			wantAmount: 44000,
			wantDiscountPct: 36,
		},
		{
			name:       "Dual price com barra",
			text:       "DE 3.429 | POR 1.531",
			wantOrig:   342900,
			wantAmount: 153100,
			wantDiscountPct: 55,
		},
		{
			name:       "Dual price invertido ignorado",
			text:       "De R$ 179 por R$ 236,58",
			wantOrig:   0,
			wantAmount: 17900, // Fallback extracts first price
		},
		{
			name:       "Dual price era agora",
			text:       "Era R$ 500, agora R$ 350",
			wantOrig:   50000,
			wantAmount: 35000,
			wantDiscountPct: 30,
		},
		{
			name:       "Preco com REAIS",
			text:       "A PARTIR DE: 61 REAIS",
			wantOrig:   0,
			wantAmount: 6100,
		},
		{
			name:       "Multiplos preços com REAIS pega o menor",
			text:       "PRETO: 169 REAIS / BRANCO: 179 REAIS",
			wantAmount: 16900,
		},
		{
			name:              "Parcelamento com valor",
			text:              "10x sem juros de R$ 44",
			wantAmount:        4400,
			wantInstallments:  10,
			wantInstallmentsV: 44.0,
		},
		{
			name:              "Parcelamento com valor alt",
			text:              "6x R$ 73,00",
			wantAmount:        7300,
			wantInstallments:  6,
			wantInstallmentsV: 73.0,
		},
		{
			name:              "Parcelamento com valor alt 2",
			text:              "em 12x de R$ 99,90",
			wantAmount:        9990,
			wantInstallments:  12,
			wantInstallmentsV: 99.9,
		},
		{
			name:              "Parcelamento sem valor",
			text:              "8X SEM JUROS",
			wantInstallments:  8,
			wantInstallmentsV: 0.0,
		},
		{
			name:             "Frete gratis 1",
			text:             "Com frete grátis",
			wantShippingFree: true,
		},
		{
			name:             "Frete gratis 2",
			text:             "frete free pra todo brasil",
			wantShippingFree: true,
		},
		{
			name:             "Frete gratis 3",
			text:             "sem frete",
			wantShippingFree: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			nm := &NormalizedMessage{Text: tt.text}
			extractPrices(tt.text, nm)
			extractModifiers(tt.text, nm)

			if nm.PriceOriginal != tt.wantOrig {
				t.Errorf("PriceOriginal = %v, quer %v", nm.PriceOriginal, tt.wantOrig)
			}
			if nm.PriceAmount != tt.wantAmount {
				t.Errorf("PriceAmount = %v, quer %v", nm.PriceAmount, tt.wantAmount)
			}
			if nm.InstallmentsN != tt.wantInstallments {
				t.Errorf("InstallmentsN = %v, quer %v", nm.InstallmentsN, tt.wantInstallments)
			}
			if nm.InstallmentsValue != tt.wantInstallmentsV {
				t.Errorf("InstallmentsValue = %v, quer %v", nm.InstallmentsValue, tt.wantInstallmentsV)
			}
			if nm.ShippingFree != tt.wantShippingFree {
				t.Errorf("ShippingFree = %v, quer %v", nm.ShippingFree, tt.wantShippingFree)
			}
			if nm.DiscountPct != tt.wantDiscountPct {
				t.Errorf("DiscountPct = %v, quer %v", nm.DiscountPct, tt.wantDiscountPct)
			}
		})
	}
}

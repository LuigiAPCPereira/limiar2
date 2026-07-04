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

func TestNormalize_Sprint1ExtractionFields(t *testing.T) {
	payload := map[string]any{
		"ID":      float64(12346),
		"Message": "Produto teste R$ 99 no pix\nCupom: AEBR2 ou IFPL90V1",
		"PeerID":  map[string]any{"ChannelID": float64(999)},
		"Date":    float64(1717891200),
		"Media": map[string]any{
			"Webpage": map[string]any{
				"URL":         "https://www.amazon.com.br/produto",
				"Title":       "Produto Teste - Amazon",
				"Description": "Descrição gold standard",
			},
		},
	}
	raw := makeRaw(t, 11, 999, 12346, payload)

	nm, err := Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize falhou: %v", err)
	}

	if nm.CouponCode != "AEBR2" {
		t.Fatalf("CouponCode = %q, quer primeiro cupom AEBR2", nm.CouponCode)
	}
	if len(nm.CouponCodes) != 2 {
		t.Fatalf("len(CouponCodes) = %d, quer 2: %#v", len(nm.CouponCodes), nm.CouponCodes)
	}
	if nm.WebpageURL != "https://www.amazon.com.br/produto" {
		t.Errorf("WebpageURL = %q", nm.WebpageURL)
	}
	if nm.WebpageTitle != "Produto Teste - Amazon" {
		t.Errorf("WebpageTitle = %q", nm.WebpageTitle)
	}
	if nm.WebpageDesc != "Descrição gold standard" {
		t.Errorf("WebpageDesc = %q", nm.WebpageDesc)
	}
	if len(nm.Modifiers) == 0 || nm.Modifiers[0].Type != "payment" || nm.Modifiers[0].Value != "pix" {
		t.Fatalf("Modifiers = %#v, quer payment/pix", nm.Modifiers)
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

func TestNormalize_UrgencySignals(t *testing.T) {
	payload := map[string]any{
		"ID":      float64(200),
		"Message": "🏃 Corre! Frete grátis, envio nacional",
		"PeerID":  map[string]any{"ChannelID": float64(1)},
		"Date":    float64(0),
	}
	raw := makeRaw(t, 30, 1, 200, payload)

	nm, err := Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize falhou: %v", err)
	}

	// UrgencySignals foi removido do schema, então não verificamos mais
	// Apenas verificamos que a normalização funcionou
	if nm.Text == "" {
		t.Fatal("Texto não foi extraído")
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
			applyCoupons(tt.text, nm)
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
			applyModifiers(tt.text, nm)
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

func TestNormalize_PreservesLargePhotoMetadataIDs(t *testing.T) {
	const (
		photoID    int64 = 4985843018296396847
		accessHash int64 = -7524180780117537531
	)

	raw := makeRaw(t, 1, 1352231186, 95232, map[string]any{
		"ID":      int64(95232),
		"Message": "produto com foto",
		"Media": map[string]any{
			"Photo": map[string]any{
				"ID":            photoID,
				"AccessHash":    accessHash,
				"FileReference": "AlCZaRIAAXQAaiW+TljSnSegJJ4ybQVsJymwX1s=",
				"DCID":          1,
			},
		},
	})

	got, err := Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	if got.PhotoID != photoID {
		t.Errorf("PhotoID = %d, want %d", got.PhotoID, photoID)
	}
	if got.PhotoAccessHash != accessHash {
		t.Errorf("PhotoAccessHash = %d, want %d", got.PhotoAccessHash, accessHash)
	}
}

// --- Regressão: extração temporal (valid_until / flash / recurrence / seasonal) ---
//
// Contrato (analisado pelo TemporalAudit): Normalize extrai 4 famílias de
// sinais temporais do texto em campos ATÔMICOS de NormalizedMessage:
//   - ValidUntil/ValidFrom (time.Time): data explícita no texto
//   - Flash (bool): sinal temporal sem data ("corre", "relâmpago", "24h")
//   - RecurrencePattern (string): "weekly/monday", "monthly", "daily", ...
//   - SeasonalTag (string): "black_friday", "christmas", ...
//
// Precedência (data explícita > flash > recorrência > sazonal) reflete-se em
// campo de forma ÚNICA e inequívoca: data explícita SUPRIME flash ("se tem
// data explícita, não é flash"). Os demais sinais coexistem como contexto.
//
// Datas de teste usam ano explícito (DD/MM/YYYY) para não depender da
// heurística de inferência de ano (que tornaria o teste flaky por data corrente).
func TestNormalize_TemporalExtraction(t *testing.T) {
	tests := []struct {
		name           string
		text           string
		wantValidFrom  string // "2006-01-02" ou "" (zero)
		wantValidUntil string
		wantFlash      bool
		wantPattern    string
		wantSeasonal   string
		wantRecurring  bool
	}{
		{
			name:           "data_explicita_valid_until",
			text:           "Promoção válida até 15/07/2026, aproveite!",
			wantValidUntil: "2026-07-15",
		},
		{
			name:           "data_explicita_range",
			text:           "Oferta válida de 10/06/2026 até 20/06/2026",
			wantValidFrom:  "2026-06-10",
			wantValidUntil: "2026-06-20",
		},
		{
			name:      "flash_puro",
			text:      "Oferta relâmpago! Corre que acaba logo, tempo limitado",
			wantFlash: true,
		},
		{
			name:          "recorrente",
			text:          "Toda segunda-feira tem promoção nova no canal",
			wantPattern:   "weekly/monday",
			wantRecurring: true,
		},
		{
			name:         "sazonal",
			text:         "Black friday imperdível, confiram o link",
			wantSeasonal: "black_friday",
		},
		{
			// Precedência confirmada: data explícita SUPRIME flash. Mesmo com
			// palavras flash ("corre", "relâmpago"), a data vence → Flash = false.
			name:           "precedencia_data_suprime_flash",
			text:           "Válido até 15/07/2026, corre! Oferta relâmpago.",
			wantValidUntil: "2026-07-15",
			wantFlash:      false,
		},
		{
			// Recorrência COEXISTE com data explícita (ortogonal): uma oferta
			// pode ser recorrente E ter prazo. ValidUntil + RecurrencePattern
			// ambos populados; Flash permanece false (sem palavras flash).
			name:           "recorrencia_coexiste_com_data",
			text:           "Toda quarta-feira até 30/11/2026 tem promoção",
			wantValidUntil: "2026-11-30",
			wantPattern:    "weekly/wednesday",
			wantRecurring:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := makeRaw(t, 1, 1, 1, map[string]any{"ID": int64(1), "Message": tt.text})
			got, err := Normalize(raw)
			if err != nil {
				t.Fatalf("Normalize: %v", err)
			}
			if dayOnly(got.ValidFrom) != tt.wantValidFrom {
				t.Errorf("ValidFrom = %q; quer %q", dayOnly(got.ValidFrom), tt.wantValidFrom)
			}
			if dayOnly(got.ValidUntil) != tt.wantValidUntil {
				t.Errorf("ValidUntil = %q; quer %q", dayOnly(got.ValidUntil), tt.wantValidUntil)
			}
			if got.Flash != tt.wantFlash {
				t.Errorf("Flash = %v; quer %v", got.Flash, tt.wantFlash)
			}
			if got.RecurrencePattern != tt.wantPattern {
				t.Errorf("RecurrencePattern = %q; quer %q", got.RecurrencePattern, tt.wantPattern)
			}
			if got.SeasonalTag != tt.wantSeasonal {
				t.Errorf("SeasonalTag = %q; quer %q", got.SeasonalTag, tt.wantSeasonal)
			}
			if got.IsRecurring != tt.wantRecurring {
				t.Errorf("IsRecurring = %v; quer %v", got.IsRecurring, tt.wantRecurring)
			}
		})
	}
}

// dayOnly formata time.Time como "YYYY-MM-DD", ou "" se zero. Centraliza a
// comparação de datas extraídas ignorando horas/minutos (que não são assertados).
func dayOnly(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02")
}

// --- Sprint 2: Candidate Ranking Engine (ProductName) ---
//
// Contrato (docs/specs/EXTRACTION-CRE.md §2, ADR 014): o CRE determinístico
// extrai ProductName de (1) linha limpa próxima ao preço no texto e (2) do
// título da webpage (Media.Webpage.Title), atribuindo ProductNameConfidence em
// [0.0, 1.0]. Threshold 0.45: abaixo dele o ProductName fica vazio (não inventa
// nome). Os testes abaixo cobrem as três famílias de comportamento pelo caminho
// público Normalize.

// TestNormalize_CRE_TextHeuristic defende o caso dominante do corpus: o nome do
// produto é a linha limpa imediatamente acima do preço. O CRE deve extraí-la e
// atribuir confidence >= 0.45.
func TestNormalize_CRE_TextHeuristic(t *testing.T) {
	payload := map[string]any{
		"ID":      float64(50001),
		"Message": "PARCELADO🔥🔥🔥🔥\n\nAnker Caixa de Som Soundcore Select 4 go\n\nPOR: 154 REAIS E FRETE GRÁTIS PRIME\nhttps://amzn.to/abc",
		"PeerID":  map[string]any{"ChannelID": float64(999)},
		"Date":    float64(1717891200),
		"Media":   nil,
	}
	raw := makeRaw(t, 1, 999, 50001, payload)

	nm, err := Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize falhou: %v", err)
	}

	if nm.ProductName != "Anker Caixa de Som Soundcore Select 4 go" {
		t.Errorf("ProductName = %q, quer \"Anker Caixa de Som Soundcore Select 4 go\"", nm.ProductName)
	}
	if nm.ProductNameConfidence < 0.45 {
		t.Errorf("ProductNameConfidence = %v, quer >= 0.45", nm.ProductNameConfidence)
	}
}

// TestNormalize_CRE_WebpageTitle prova que, quando o texto só traz preço e link,
// o título da webpage (Media.Webpage.Title) é a fonte de ProductName — com o
// sufixo de merchant ("- Amazon.com.br") normalizado para fora.
func TestNormalize_CRE_WebpageTitle(t *testing.T) {
	payload := map[string]any{
		"ID":      float64(50002),
		"Message": "R$ 7.599,99 à vista\nhttps://amzn.to/xyz",
		"PeerID":  map[string]any{"ChannelID": float64(999)},
		"Date":    float64(1717891200),
		"Media": map[string]any{
			"Webpage": map[string]any{
				"URL":   "https://www.amazon.com.br/iphone-16/dp/B0DGFP9XQN",
				"Title": "Apple iPhone 16 (128 GB) - Amazon.com.br",
			},
		},
	}
	raw := makeRaw(t, 2, 999, 50002, payload)

	nm, err := Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize falhou: %v", err)
	}

	if nm.ProductName != "Apple iPhone 16 (128 GB)" {
		t.Errorf("ProductName = %q, quer \"Apple iPhone 16 (128 GB)\"", nm.ProductName)
	}
}

// TestNormalize_CRE_NoProduct_MetaCoupon defende o invariante "não inventa nome":
// um meta-anúncio de cupom genérico, sem produto real, deve deixar ProductName
// vazio e ProductNameConfidence abaixo do threshold 0.45.
func TestNormalize_CRE_NoProduct_MetaCoupon(t *testing.T) {
	payload := map[string]any{
		"ID":      float64(50003),
		"Message": "NOVO CUPOM AMAZON\nCupom: AMAZON10\nVálido só hoje",
		"PeerID":  map[string]any{"ChannelID": float64(999)},
		"Date":    float64(1717891200),
		"Media":   nil,
	}
	raw := makeRaw(t, 3, 999, 50003, payload)

	nm, err := Normalize(raw)
	if err != nil {
		t.Fatalf("Normalize falhou: %v", err)
	}

	if nm.ProductName != "" {
		t.Errorf("ProductName = %q, quer vazio (sem produto real)", nm.ProductName)
	}
	if nm.ProductNameConfidence >= 0.45 {
		t.Errorf("ProductNameConfidence = %v, quer < 0.45 (abaixo do threshold)", nm.ProductNameConfidence)
	}
}

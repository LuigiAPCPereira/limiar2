package processor

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/limiar/collector/internal/storage"
)

func TestNormalize_ShapeA(t *testing.T) {
	payload := map[string]any{
		"ID":      float64(12345),
		"Message": "🔥 Celular Samsung R$ 1.234,56\nhttps://amzn.to/abc\nCupom: SAMSUNG10",
		"PeerID":  map[string]any{"ChannelID": float64(999)},
		"Date":    float64(1717891200),
		"Media":   nil,
		"Views":   float64(500),
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

	if len(nm.UrgencySignals) != 3 {
		t.Fatalf("UrgencySignals = %v, quer 3 sinais", nm.UrgencySignals)
	}
	want := map[string]bool{"corre": true, "frete_gratis": true, "envio_nacional": true}
	for _, s := range nm.UrgencySignals {
		if !want[s] {
			t.Errorf("sinal inesperado: %q", s)
		}
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

func TestNormalize_PriceExtraction(t *testing.T) {
	tests := []struct {
		text      string
		wantOK    bool
		wantCents int64
	}{
		{"R$ 1.234,56", true, 123456},
		{"R$1234", true, 123400},
		{"R$ 83", true, 8300},
		{"R$99,90", true, 9990},
		{"sem preço", false, 0},
		{"R$ 0,01", true, 1},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			ok, cents := extractPrice(tt.text)
			if ok != tt.wantOK {
				t.Errorf("hasPrice = %v, quer %v", ok, tt.wantOK)
			}
			if cents != tt.wantCents {
				t.Errorf("cents = %d, quer %d", cents, tt.wantCents)
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

// makeRaw é um helper que cria um storage.RawMessage com payload JSON.
func makeRaw(t *testing.T, id, channelID, messageID int64, payload map[string]any) *storage.RawMessage {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	return &storage.RawMessage{
		ID:            id,
		ChannelID:     channelID,
		MessageID:     messageID,
		Payload:       data,
		ReceivedAt:    time.Now().UTC(),
		SchemaVersion: 1,
	}
}

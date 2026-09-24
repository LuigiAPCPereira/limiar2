package processor

import (
	"testing"

	"github.com/limiar/collector/internal/model"
)

func TestExtractModifiers(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []model.Modifier
	}{
		{
			name: "pix entre parenteses",
			text: "POR: 99 (pix)",
			want: []model.Modifier{{Type: "payment", Value: "pix"}},
		},
		{
			name: "selecione pix na pag",
			text: "SELECIONE PIX NA PAG",
			want: []model.Modifier{{Type: "payment", Value: "pix"}},
		},
		{
			name: "no pix",
			text: "Oferta no pix",
			want: []model.Modifier{{Type: "payment", Value: "pix"}},
		},
		{
			name: "apenas aplicativo",
			text: "APENAS PELO APLICATIVO",
			want: []model.Modifier{{Type: "app_only", Value: "true"}},
		},
		{
			name: "somente app",
			text: "somente no app",
			want: []model.Modifier{{Type: "app_only", Value: "true"}},
		},
		{
			name: "pela web",
			text: "Oferta válida pela web",
			want: []model.Modifier{{Type: "web_only", Value: "true"}},
		},
		{
			name: "somente site",
			text: "somente no site",
			want: []model.Modifier{{Type: "web_only", Value: "true"}},
		},
		{
			name: "moedas quantidade",
			text: "1853 MOEDAS",
			want: []model.Modifier{{Type: "discount", Value: "moedas"}},
		},
		{
			name: "cashback moedas shopee",
			text: "50% cashback em Moedas Shopee",
			want: []model.Modifier{{Type: "cashback", Value: "moedas_shopee"}, {Type: "discount", Value: "50_percent"}},
		},
		{
			name: "combo pix frete parcelamento",
			text: "R$ 99 no pix com frete grátis em até 10x sem juros",
			want: []model.Modifier{{Type: "payment", Value: "pix"}, {Type: "shipping", Value: "frete_gratis"}, {Type: "installments", Value: "10x_sem_juros"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractModifiers(tt.text)
			assertModifiersEqual(t, got, tt.want)
		})
	}
}

func assertModifiersEqual(t *testing.T, got, want []model.Modifier) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("len(modifiers) = %d (%#v), want %d (%#v)", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("modifier[%d] = %#v, want %#v; all got=%#v", i, got[i], want[i], got)
		}
	}
}

func TestExtractVirtualCurrency(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		merchant string
		want     *model.VirtualCurrency
	}{
		{
			name:     "moedas shopee por merchant",
			text:     "Ganhe 1853 MOEDAS",
			merchant: "shopee",
			want:     &model.VirtualCurrency{Platform: "shopee", Amount: 1853, Type: "discount"},
		},
		{
			name:     "cashback moedas shopee",
			text:     "50% cashback em Moedas Shopee",
			merchant: "shopee",
			want:     &model.VirtualCurrency{Platform: "shopee", Amount: 0, Type: "cashback"},
		},
		{
			name:     "sem moedas",
			text:     "Oferta sem saldo virtual",
			merchant: "shopee",
			want:     nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractVirtualCurrency(tt.text, tt.merchant)
			if got == nil || tt.want == nil {
				if got != tt.want {
					t.Fatalf("extractVirtualCurrency() = %#v, want %#v", got, tt.want)
				}
				return
			}
			if *got != *tt.want {
				t.Fatalf("extractVirtualCurrency() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

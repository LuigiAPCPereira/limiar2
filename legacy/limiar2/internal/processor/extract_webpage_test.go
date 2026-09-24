package processor

import (
	"testing"

	"github.com/limiar/collector/internal/model"
)

func TestExtractWebpageInfo(t *testing.T) {
	tests := []struct {
		name string
		msg  map[string]any
		want model.WebpageInfo
	}{
		{
			name: "webpage completo",
			msg: map[string]any{
				"Media": map[string]any{
					"Webpage": map[string]any{
						"URL":         "https://www.amazon.com.br/produto",
						"Title":       "Apple iPhone 16 (128 GB)",
						"Description": "Oferta por tempo limitado",
					},
				},
			},
			want: model.WebpageInfo{URL: "https://www.amazon.com.br/produto", Title: "Apple iPhone 16 (128 GB)", Description: "Oferta por tempo limitado"},
		},
		{
			name: "sem webpage",
			msg: map[string]any{
				"Media": map[string]any{
					"Photo": map[string]any{"ID": float64(1)},
				},
			},
			want: model.WebpageInfo{},
		},
		{
			name: "webpage parcial titulo vazio",
			msg: map[string]any{
				"Media": map[string]any{
					"Webpage": map[string]any{
						"URL":   "https://s.shopee.com.br/produto",
						"Title": "",
					},
				},
			},
			want: model.WebpageInfo{URL: "https://s.shopee.com.br/produto"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractWebpageInfo(tt.msg)
			if got != tt.want {
				t.Fatalf("extractWebpageInfo() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

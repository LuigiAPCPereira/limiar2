package processor

import "testing"

func TestClassify(t *testing.T) {
	tests := []struct {
		name string
		nm   *NormalizedMessage
		want MessageType
	}{
		{
			name: "deal_complete: URL + preço + cupom",
			nm: &NormalizedMessage{
				Text:     "🔥 Fone R$ 29,90 https://amzn.to/abc Cupom: ABCDE",
				HasURL:   true,
				HasPrice: true,
				HasCoupon: true,
			},
			want: TypeDealComplete,
		},
		{
			name: "deal_no_coupon: URL + preço, sem cupom",
			nm: &NormalizedMessage{
				Text:     "🔥 Fone R$ 29,90 https://amzn.to/abc",
				HasURL:   true,
				HasPrice: true,
			},
			want: TypeDealNoCoupon,
		},
		{
			name: "deal_no_price: URL sem preço",
			nm: &NormalizedMessage{
				Text:   "Confira https://amzn.to/abc",
				HasURL: true,
			},
			want: TypeDealNoPrice,
		},
		{
			name: "category_header: texto curto",
			nm: &NormalizedMessage{
				Text: "🎧 Eletrônicos",
			},
			want: TypeCategoryHeader,
		},
		{
			name: "coupon_expired: esgotado sem URL",
			nm: &NormalizedMessage{
				Text: "⚠️ Esgotado pessoal!",
			},
			want: TypeCouponExpired,
		},
		{
			name: "admin_meta: regras_grupo",
			nm: &NormalizedMessage{
				Text: "📋 regras_grupo: não spam",
			},
			want: TypeAdminMeta,
		},
		{
			name: "video: media video",
			nm: &NormalizedMessage{
				Text:      "Vídeo review do produto",
				MediaType: "video",
			},
			want: TypeVideo,
		},
		{
			name: "document: media document",
			nm: &NormalizedMessage{
				Text:      "Manual do produto",
				MediaType: "document",
			},
			want: TypeDocument,
		},
		{
			name: "poll: media poll",
			nm: &NormalizedMessage{
				Text:      "Qual categoria?",
				MediaType: "poll",
			},
			want: TypePoll,
		},
		{
			name: "commentary: texto livre",
			nm: &NormalizedMessage{
				Text: "Barato demais gente, precinho! Aproveitem antes que acabe, tá valendo muito a pena comprar agora",
			},
			want: TypeCommentary,
		},
		{
			name: "expired com URL não é coupon_expired",
			nm: &NormalizedMessage{
				Text:     "Esgotado mas ainda tem https://amzn.to/abc",
				HasURL:   true,
				HasPrice: true,
				HasCoupon: true,
			},
			want: TypeDealComplete,
		},
		{
			name: "coupon_only: cupom genérico sem produto",
			nm: &NormalizedMessage{
				Text:      "Cupom Shopee\n\nR$10 OFF em R$40 - TORCIDAAFILIADAAF\n\nResgate aqui\nhttps://s.shopee.com.br/abc",
				HasURL:    true,
				HasPrice:  true,
				HasCoupon: true,
			},
			want: TypeCouponOnly,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Classify(tt.nm)
			if got != tt.want {
				t.Errorf("Classify = %q, quer %q", got, tt.want)
			}
		})
	}
}

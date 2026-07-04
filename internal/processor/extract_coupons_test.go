package processor

import (
	"reflect"
	"testing"

	"github.com/limiar/collector/internal/model"
)

func TestExtractCoupons(t *testing.T) {
	tests := []struct {
		name string
		text string
		want []model.Coupon
	}{
		{
			name: "cupom simples",
			text: "cupom: TOCAPROGOL",
			want: []model.Coupon{{Code: "TOCAPROGOL", DiscountType: "code_only"}},
		},
		{
			name: "multiplos cupons com ou",
			text: "Cupom: AEBR2 ou IFPL90V1",
			want: []model.Coupon{{Code: "AEBR2", DiscountType: "code_only"}, {Code: "IFPL90V1", DiscountType: "code_only"}},
		},
		{
			name: "multiplos cupons com soma",
			text: "Cupom: NOTE600 + POUPE",
			want: []model.Coupon{{Code: "NOTE600", DiscountType: "code_only"}, {Code: "POUPE", DiscountType: "code_only"}},
		},
		{
			name: "cupom valor fixo",
			text: "Cupom de R$ 30 OFF",
			want: []model.Coupon{{DiscountType: "fixed_brl", DiscountValue: 3000}},
		},
		{
			name: "cupom percentual",
			text: "Cupom de 15% OFF",
			want: []model.Coupon{{DiscountType: "percent", DiscountValue: 15}},
		},
		{
			name: "requires action",
			text: "Resgate o cupom no anúncio",
			want: []model.Coupon{{RequiresAction: true}},
		},
		{
			name: "sem cupom",
			text: "Texto sem menção promocional de cupom",
			want: []model.Coupon{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractCoupons(tt.text)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("extractCoupons(%q) = %#v, want %#v", tt.text, got, tt.want)
			}
		})
	}
}

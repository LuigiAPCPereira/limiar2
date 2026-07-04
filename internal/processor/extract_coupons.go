package processor

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/limiar/collector/internal/model"
)

var (
	reCouponChain    = regexp.MustCompile(`(?i)(?:cup[ao]m|c[oó]digo|code)[\s:\n]+([A-Za-z0-9_]{3,25}(?:(?:\s+\bou\b\s+|\s*\+\s*)[A-Za-z0-9_]{3,25})*)`)
	reCouponFixedBRL = regexp.MustCompile(`(?i)cupom\s+de\s+R\$\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?|[0-9]+(?:,[0-9]{2})?)`)
	reCouponPercent  = regexp.MustCompile(`(?i)cupom\s+de\s+(\d{1,3})\s*%`)
	reCouponAction   = regexp.MustCompile(`(?i)resgate.*(?:an[úu]ncio|app|loja)`)
	reCouponSplit    = regexp.MustCompile(`(?i)(?:\s+\bou\b\s+|\s*\+\s*)`)
)

// extractCoupons enumera todos os cupons explícitos sem perder cadeias "A ou B".
func extractCoupons(text string) []model.Coupon {
	coupons := make([]model.Coupon, 0)
	seen := make(map[string]bool)
	requiresAction := reCouponAction.MatchString(text)

	for _, match := range reCouponChain.FindAllStringSubmatch(text, -1) {
		for _, rawCode := range reCouponSplit.Split(match[1], -1) {
			code := strings.ToUpper(strings.TrimSpace(rawCode))
			if code == "" || seen[code] {
				continue
			}
			seen[code] = true
			coupons = append(coupons, model.Coupon{Code: code, DiscountType: "code_only", RequiresAction: requiresAction})
		}
	}

	if match := reCouponFixedBRL.FindStringSubmatch(text); match != nil {
		coupons = append(coupons, model.Coupon{DiscountType: "fixed_brl", DiscountValue: parseBRL(match[1]), RequiresAction: requiresAction})
	}
	if match := reCouponPercent.FindStringSubmatch(text); match != nil {
		value, _ := strconv.ParseInt(match[1], 10, 64)
		if value > 0 && value <= 100 {
			coupons = append(coupons, model.Coupon{DiscountType: "percent", DiscountValue: value, RequiresAction: requiresAction})
		}
	}
	if len(coupons) == 0 && requiresAction {
		coupons = append(coupons, model.Coupon{RequiresAction: true})
	}
	return coupons
}

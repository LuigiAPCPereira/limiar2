package processor

import (
	"regexp"
	"strings"
	"unicode"
)

var reProductNameSKUTrailing = regexp.MustCompile(`\s*[-–—|]\s*[A-Z0-9]{3,}(?:-[A-Z0-9]+)+\s*$`)

// reProductNamePriceTrailing remove preços/cupons que vazam para o nome do
// produto quando a fonte é uma única linha ("Kit ... - R$55 pix", "... Cupom: XPTO").
var reProductNamePriceTrailing = regexp.MustCompile(`(?i)\s*[-–—|•·:]\s*(?:por[:\s]*)?R?\$\s*\d+(?:[.,]\d+)?(?:\s*/\s*\d+x)?(?:\s+pix)?\s*$`)
var reProductNameCouponTrailing = regexp.MustCompile(`(?i)\s*[-–—|•·:]\s*(?:cupom[:\s]*)?[A-Z0-9]{3,}(?:\s*\+\s*[A-Z0-9]{3,})?\s*$`)

func normalizeProductName(name string) string {
	name = cleanCandidateLine(name)
	name = removeProductNameMerchantSuffix(name)
	name = removeProductNameSKUTrailing(name)
	name = removeProductNamePriceTrailing(name)
	name = removeProductNameCouponTrailing(name)
	name = removeProductNamePromoNoise(name)
	name = cleanProductNameDelimiters(name)
	if isAllCapsProductName(name) {
		name = titleCaseProductName(name)
	}
	return cleanProductNameDelimiters(name)
}

func removeProductNameMerchantSuffix(name string) string {
	for _, re := range productNameMerchantSuffixRegexps() {
		name = re.ReplaceAllString(name, "")
	}
	return strings.TrimSpace(name)
}

func removeProductNameSKUTrailing(name string) string {
	return strings.TrimSpace(reProductNameSKUTrailing.ReplaceAllString(name, ""))
}

func removeProductNamePriceTrailing(name string) string {
	return strings.TrimSpace(reProductNamePriceTrailing.ReplaceAllString(name, ""))
}

func removeProductNameCouponTrailing(name string) string {
	return strings.TrimSpace(reProductNameCouponTrailing.ReplaceAllString(name, ""))
}

func removeProductNamePromoNoise(name string) string {
	for _, re := range productNamePromoNoiseReplaceRegexps() {
		name = re.ReplaceAllString(name, " ")
	}
	return strings.TrimSpace(name)
}

func cleanProductNameDelimiters(name string) string {
	name = reCollapseProductNameSpace.ReplaceAllString(name, " ")
	name = strings.TrimSpace(name)
	name = strings.Trim(name, " -–—|:•·")
	return strings.TrimSpace(name)
}

func isAllCapsProductName(name string) bool {
	var letters int
	var lower int
	for _, r := range name {
		if !unicode.IsLetter(r) {
			continue
		}
		letters++
		if unicode.IsLower(r) {
			lower++
		}
	}
	return letters > 1 && lower == 0
}

func titleCaseProductName(name string) string {
	words := strings.Fields(name)
	for i, word := range words {
		words[i] = titleCaseProductNameWord(word)
	}
	return strings.Join(words, " ")
}

func titleCaseProductNameWord(word string) string {
	lower := strings.ToLower(word)
	if isProductNameStopword(lower) {
		return lower
	}
	if brand, ok := canonicalProductNameBrand(lower); ok {
		return brand
	}
	letters := []rune(lower)
	for i, r := range letters {
		if unicode.IsLetter(r) {
			letters[i] = unicode.ToUpper(r)
			break
		}
	}
	return string(letters)
}

func canonicalProductNameBrand(lower string) (string, bool) {
	for _, brand := range productNameBrands() {
		if strings.ToLower(brand) == lower {
			return brand, true
		}
	}
	return "", false
}

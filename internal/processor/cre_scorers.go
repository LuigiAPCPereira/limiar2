package processor

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

func scorePosition(c Candidate, _ *NormalizedMessage) float64 {
	switch c.Detail {
	case "line:0":
		return 0.25
	case "line:1":
		return 0.15
	case "line:0-1":
		return 0.10
	default:
		return 0
	}
}

func scoreCapitalization(c Candidate, _ *NormalizedMessage) float64 {
	words := productNameWords(c.Text)
	if len(words) == 0 {
		return 0
	}
	var titleWords int
	var considered int
	for _, word := range words {
		lower := strings.ToLower(word)
		if isProductNameStopword(lower) {
			continue
		}
		considered++
		if startsUpperAndHasLower(word) {
			titleWords++
		}
	}
	if considered == 0 {
		return 0
	}
	if float64(titleWords)/float64(considered) >= 0.60 {
		return 0.20
	}
	return 0
}

func scoreTechDensity(c Candidate, _ *NormalizedMessage) float64 {
	matches := 0
	for _, re := range productNameTechTokenRegexps() {
		if re.MatchString(c.Text) {
			matches++
		}
	}
	if matches == 0 {
		return 0
	}
	if matches >= 4 {
		return 0.20
	}
	return 0.05 * float64(matches)
}

func scoreBrandMatch(c Candidate, _ *NormalizedMessage) float64 {
	for _, brand := range productNameBrands() {
		if containsProductNamePhrase(c.Text, brand) {
			return 0.20
		}
	}
	return 0
}

func scoreProximity(c Candidate, msg *NormalizedMessage) float64 {
	if c.Detail == "near_price" {
		return 0.10
	}
	if msg != nil && (msg.HasPrice || msg.HasCoupon) && c.Detail == "line:0-1" {
		return 0.05
	}
	return 0
}

func scoreMetaPenalty(c Candidate, _ *NormalizedMessage) float64 {
	for _, noise := range productNamePromoNoise() {
		if containsProductNamePhrase(c.Text, noise) {
			return -0.15
		}
	}
	return 0
}

func productNameWords(text string) []string {
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	words := make([]string, 0, len(fields))
	for _, field := range fields {
		if field != "" {
			words = append(words, field)
		}
	}
	return words
}

func startsUpperAndHasLower(word string) bool {
	var sawFirst bool
	var firstUpper bool
	var hasLower bool
	for _, r := range word {
		if unicode.IsLetter(r) {
			if !sawFirst {
				sawFirst = true
				firstUpper = unicode.IsUpper(r)
			} else if unicode.IsLower(r) {
				hasLower = true
			}
		}
	}
	return firstUpper && hasLower
}

func isProductNameStopword(word string) bool {
	switch word {
	case "a", "as", "o", "os", "de", "da", "das", "do", "dos", "e", "em", "com", "para", "por", "sem":
		return true
	default:
		return false
	}
}

func containsProductNamePhrase(text, phrase string) bool {
	text = strings.ToLower(text)
	phrase = strings.ToLower(strings.TrimSpace(phrase))
	if phrase == "" {
		return false
	}
	start := 0
	for {
		idx := strings.Index(text[start:], phrase)
		if idx < 0 {
			return false
		}
		idx += start
		end := idx + len(phrase)
		if productNameBoundaryBefore(text, idx) && productNameBoundaryAfter(text, end) {
			return true
		}
		_, size := utf8.DecodeRuneInString(text[idx:])
		if size == 0 {
			return false
		}
		start = idx + size
	}
}

func productNameBoundaryBefore(text string, idx int) bool {
	if idx <= 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(text[:idx])
	return isProductNameBoundary(r)
}

func productNameBoundaryAfter(text string, idx int) bool {
	if idx >= len(text) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(text[idx:])
	return isProductNameBoundary(r)
}

func isProductNameBoundary(r rune) bool {
	return !unicode.IsLetter(r) && !unicode.IsDigit(r)
}

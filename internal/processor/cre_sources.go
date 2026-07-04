package processor

import (
	"regexp"
	"strings"
	"unicode"
)

var reCollapseProductNameSpace = regexp.MustCompile(`\s+`)

func webpageTitleCandidates(title string) []Candidate {
	title = strings.TrimSpace(title)
	if title == "" {
		return nil
	}
	return []Candidate{
		{
			Text:           title,
			Source:         productNameSourceWebpageTitle,
			Detail:         "webpage",
			BaseConfidence: 0.60,
		},
	}
}

func textHeuristicCandidates(text string) []Candidate {
	lines := splitProductNameLines(text)
	candidates := []Candidate{}
	if len(lines) == 0 {
		return candidates
	}

	// Linhas de conteúdo = linhas não-meta, na ordem original.
	// Candidatos são gerados a partir dessas linhas, não das linhas cruas.
	contentLines := make([]string, 0, len(lines))
	for _, line := range lines {
		if isMetaOnlyProductNameCandidate(line) {
			continue
		}
		contentLines = append(contentLines, line)
	}
	if len(contentLines) == 0 {
		return candidates
	}

	candidates = appendProductNameTextCandidate(candidates, contentLines[0], "line:0", 0.30)
	if len(contentLines) > 1 {
		candidates = appendProductNameTextCandidate(candidates, contentLines[0]+" "+contentLines[1], "line:0-1", 0.20)
	}

	// near_price: a linha imediatamente antes do preço, se for uma linha de conteúdo.
	if priceLine := productNamePriceLine(lines); priceLine > 0 {
		nearLine := lines[priceLine-1]
		if !isMetaOnlyProductNameCandidate(nearLine) {
			candidates = appendProductNameTextCandidate(candidates, nearLine, "near_price", 0.25)
		}
	}

	return candidates
}

func splitProductNameLines(text string) []string {
	lines := []string{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func cleanCandidateLine(line string) string {
	line = strings.TrimSpace(line)
	line = strings.TrimFunc(line, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '(' && r != ')'
	})
	line = reCollapseProductNameSpace.ReplaceAllString(line, " ")
	return strings.TrimSpace(line)
}

func appendProductNameTextCandidate(candidates []Candidate, raw, detail string, baseConfidence float64) []Candidate {
	line := cleanCandidateLine(raw)
	if line == "" || isMetaOnlyProductNameCandidate(line) {
		return candidates
	}
	return append(candidates, Candidate{
		Text:           line,
		Source:         productNameSourceTextHeuristic,
		Detail:         detail,
		BaseConfidence: baseConfidence,
	})
}

func isMetaOnlyProductNameCandidate(line string) bool {
	if !hasProductNamePromoNoise(line) {
		return false
	}
	lower := strings.ToLower(line)
	if rePrice.MatchString(line) || rePriceReais.MatchString(line) || rePriceAlt.MatchString(line) || strings.Contains(lower, "por:") {
		return false
	}
	stripped := cleanProductNameDelimiters(removeProductNamePromoNoise(line))
	return len(productNameWords(stripped)) <= 2
}

func hasProductNamePromoNoise(line string) bool {
	for _, noise := range productNamePromoNoise() {
		if containsProductNamePhrase(line, noise) {
			return true
		}
	}
	return false
}

func productNamePriceLine(lines []string) int {
	for i, line := range lines {
		if rePrice.MatchString(line) || rePriceReais.MatchString(line) || strings.Contains(strings.ToLower(line), "por:") {
			return i
		}
	}
	return -1
}

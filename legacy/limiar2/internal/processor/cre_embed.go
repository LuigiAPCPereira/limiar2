package processor

import (
	"embed"
	"regexp"
	"strings"
	"sync"
)

//go:embed data/*.txt
var creDataFS embed.FS

var (
	creBrandsOnce                   sync.Once
	creTechTokensOnce               sync.Once
	creMerchantSuffixesOnce         sync.Once
	crePromoNoiseOnce               sync.Once
	creTechTokenRegexpsOnce         sync.Once
	creMerchantSuffixRegexpsOnce    sync.Once
	crePromoNoiseReplaceRegexpsOnce sync.Once

	creBrands                   []string
	creTechTokens               []string
	creMerchantSuffixes         []string
	crePromoNoise               []string
	creTechTokenRegexps         []*regexp.Regexp
	creMerchantSuffixRegexps    []*regexp.Regexp
	crePromoNoiseReplaceRegexps []*regexp.Regexp
)

func loadEmbedList(name string) []string {
	raw, err := creDataFS.ReadFile("data/" + name)
	if err != nil {
		return nil
	}
	lines := []string{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

func productNameBrands() []string {
	creBrandsOnce.Do(func() {
		creBrands = loadEmbedList("brands.txt")
	})
	return creBrands
}

func productNameTechTokens() []string {
	creTechTokensOnce.Do(func() {
		creTechTokens = loadEmbedList("tech_tokens.txt")
	})
	return creTechTokens
}

func productNameMerchantSuffixes() []string {
	creMerchantSuffixesOnce.Do(func() {
		creMerchantSuffixes = loadEmbedList("merchant_suffixes.txt")
	})
	return creMerchantSuffixes
}

func productNamePromoNoise() []string {
	crePromoNoiseOnce.Do(func() {
		crePromoNoise = loadEmbedList("promo_noise.txt")
	})
	return crePromoNoise
}

func productNameTechTokenRegexps() []*regexp.Regexp {
	creTechTokenRegexpsOnce.Do(func() {
		for _, token := range productNameTechTokens() {
			re, err := regexp.Compile(token)
			if err != nil {
				continue
			}
			creTechTokenRegexps = append(creTechTokenRegexps, re)
		}
	})
	return creTechTokenRegexps
}

func productNameMerchantSuffixRegexps() []*regexp.Regexp {
	creMerchantSuffixRegexpsOnce.Do(func() {
		for _, suffix := range productNameMerchantSuffixes() {
			suffix = strings.TrimSpace(suffix)
			if suffix == "" {
				continue
			}
			creMerchantSuffixRegexps = append(creMerchantSuffixRegexps, regexp.MustCompile(`(?i)\s*[-–—|]\s*`+regexp.QuoteMeta(suffix)+`\s*$`))
		}
	})
	return creMerchantSuffixRegexps
}

func productNamePromoNoiseReplaceRegexps() []*regexp.Regexp {
	crePromoNoiseReplaceRegexpsOnce.Do(func() {
		for _, noise := range productNamePromoNoise() {
			noise = strings.TrimSpace(noise)
			if noise == "" {
				continue
			}
			crePromoNoiseReplaceRegexps = append(crePromoNoiseReplaceRegexps, regexp.MustCompile(`(?i)(^|\s+)`+regexp.QuoteMeta(noise)+`($|\s+)`))
		}
	})
	return crePromoNoiseReplaceRegexps
}

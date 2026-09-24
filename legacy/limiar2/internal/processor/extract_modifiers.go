package processor

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/limiar/collector/internal/model"
)

var (
	rePixExpanded    = regexp.MustCompile(`(?i)\b(?:no\s+)?pix\b|selecione\s+pix|pix\s+na\s+pag`)
	reAppOnly        = regexp.MustCompile(`(?i)apenas\s+(?:pelo\s+)?aplicativo|somente\s+(?:no\s+)?(?:aplicativo|app)|pelo\s+app`)
	reWebOnly        = regexp.MustCompile(`(?i)apenas\s+(?:pelo\s+)?site|somente\s+(?:no\s+)?site|pela\s+web|no\s+site`)
	reMoedasAmount   = regexp.MustCompile(`(?i)(\d+)\s*moedas?(?:\s*no\s+app)?`)
	reMoedasMention  = regexp.MustCompile(`(?i)moedas?`)
	reMoedasCashback = regexp.MustCompile(`(?i)cash\s*back\s+em\s+moedas?|cashback\s+em\s+moedas?|moedas?.*cash\s*back|moedas?.*cashback`)
	reVirtualCapBRL  = regexp.MustCompile(`(?i)limite[^\n]*R\$\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?|[0-9]+(?:,[0-9]{2})?)`)
	rePlatformShopee = regexp.MustCompile(`(?i)shopee`)
	rePlatformAli    = regexp.MustCompile(`(?i)ali\s*express|aliexpress`)
)

// extractModifiers retorna condições comerciais normalizadas na ordem de leitura do processor.
func extractModifiers(text string) []model.Modifier {
	modifiers := make([]model.Modifier, 0)
	add := func(typ, value string) {
		for _, m := range modifiers {
			if m.Type == typ && m.Value == value {
				return
			}
		}
		modifiers = append(modifiers, model.Modifier{Type: typ, Value: value})
	}

	if rePixExpanded.MatchString(text) {
		add("payment", "pix")
	}
	if reFretePrime.MatchString(text) {
		add("shipping", "frete_gratis_prime")
	} else if reFreteG.MatchString(text) {
		add("shipping", "frete_gratis")
	}
	if match := reInstall.FindStringSubmatch(text); match != nil {
		add("installments", match[1]+"x_sem_juros")
	}
	if reMoedasCashback.MatchString(text) {
		platform := inferVirtualCurrencyPlatform(text, "")
		if platform != "" {
			add("cashback", "moedas_"+platform)
		} else {
			add("cashback", "moedas")
		}
	} else if reCashback.MatchString(text) {
		add("cashback", "cashback")
	}
	if match := reDiscPct.FindStringSubmatch(text); match != nil {
		n, _ := strconv.Atoi(match[1])
		if n > 0 && n <= 100 {
			add("discount", strconv.Itoa(n)+"_percent")
		}
	} else if reMoedasMention.MatchString(text) {
		add("discount", "moedas")
	}
	if reAppOnly.MatchString(text) {
		add("app_only", "true")
	}
	if reWebOnly.MatchString(text) {
		add("web_only", "true")
	}
	if reRecurring.MatchString(text) {
		add("recurring", "true")
	}
	return modifiers
}

// extractVirtualCurrency captura moedas de marketplaces sem depender de I/O.
func extractVirtualCurrency(text, merchant string) *model.VirtualCurrency {
	hasMoedas := reMoedasMention.MatchString(text)
	if !hasMoedas {
		return nil
	}
	vc := &model.VirtualCurrency{
		Platform: inferVirtualCurrencyPlatform(text, merchant),
		Type:     "discount",
	}
	if reMoedasCashback.MatchString(text) {
		vc.Type = "cashback"
	}
	if match := reMoedasAmount.FindStringSubmatch(text); match != nil {
		amount, _ := strconv.ParseInt(match[1], 10, 64)
		vc.Amount = amount
	}
	if match := reVirtualCapBRL.FindStringSubmatch(text); match != nil {
		vc.CapBRL = parseBRL(match[1])
	}
	return vc
}

func inferVirtualCurrencyPlatform(text, merchant string) string {
	m := strings.ToLower(strings.TrimSpace(merchant))
	if strings.Contains(m, "shopee") || rePlatformShopee.MatchString(text) {
		return "shopee"
	}
	if strings.Contains(m, "aliexpress") || rePlatformAli.MatchString(text) {
		return "aliexpress"
	}
	return ""
}

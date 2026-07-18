package processor

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/limiar/collector/internal/model"
)

// NormalizedMessage preserva a API pública do pacote processor enquanto o modelo
// canônico vive em internal/model para permitir persistência em internal/storage.
type NormalizedMessage = model.NormalizedMessage

// --- Estágio 1: Normalização de texto ---

var (
	reWhitespace  = regexp.MustCompile(`\n{3,}`)
	reMultiSpace  = regexp.MustCompile(` {2,}`)
	reSmartQuotes = strings.NewReplacer(
		"\u201c", `"`, "\u201d", `"`,
		"\u2018", `'`, "\u2019", `'`,
		"\u2013", "-", "\u2014", "-",
		"\u00a0", " ",
	)
)

// cleanText normaliza whitespace e pontuação tipográfica.
// Preserva emojis e formatação original relevante.
func cleanText(s string) string {
	s = reSmartQuotes.Replace(s)
	s = reWhitespace.ReplaceAllString(s, "\n\n")
	s = reMultiSpace.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// --- Estágio 2: Pré-processamento (extração de sinais) ---

var (
	reURL   = regexp.MustCompile(`https?://[^\s<>"']+`)
	rePrice = regexp.MustCompile(`R\$\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?|[0-9]+(?:,[0-9]{2})?)`)
	// rePriceReais catches "A PARTIR DE: 61 REAIS", "A APARTIR DE: 64 REAIS", "620 REAIS em 8X"
	rePriceReais = regexp.MustCompile(`(?i)\b([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?|[0-9]+(?:,[0-9]{2})?)\s*reais\b`)
	// rePriceAlt catches "POR: 425 REAIS", "POR 65,47", "por apenas 99,90" without R$
	rePriceAlt = regexp.MustCompile(`(?i)(?:por|only|apenas)[:\s]+R?\$?\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?)\s*(?:reais|REAIS)?`)
	Test__ = regexp.MustCompile(`(?i)(?:cup[ao]m|c[oó]digo|code)[\s:\n]+([A-Za-z0-9_]{3,25})`)

	// Dual price: "De R$ 429 por R$ 208,92", "De: R$ 429 | Por: R$ 208", "R$ 50 OFF em R$ 250"
	reDualPrice = regexp.MustCompile(
		`(?i)(?:de[:\s]+R\$\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?)[^\n]*?(?:por|à\s*vista|no\s*pix)[:\s]+R\$\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?))` +
			`|(?:R\$\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?)\s*OFF\s+em\s+R\$\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?))`)

	// Modifiers
	_ = regexp.MustCompile(`(?i)no\s*pix|via\s*pix|pagamento\s+pix|[àa]\s+vista\s+no\s+pix`)
	reFretePrime = regexp.MustCompile(`(?i)frete\s*gr[áa]tis\s*prime|prime.*frete\s*gr[áa]tis`)
	reFreteG     = regexp.MustCompile(`(?i)frete\s*gr[áa]tis`)
	reInstall    = regexp.MustCompile(`(?i)(\d+)x\s*(?:sem\s*juros|s/\s*juros)`)
	reCashback   = regexp.MustCompile(`(?i)cash\s*back|cashback`)
	reDiscPct    = regexp.MustCompile(`(?i)(\d+)\s*%\s*(?:de\s+)?(?:desconto|off|desc|cash\s*back|cashback)`)
	reRecurring  = regexp.MustCompile(`(?i)toda\s+(?:semana|segunda(?:-feira)?|terça(?:-feira)?|terca(?:-feira)?|quarta(?:-feira)?|quinta(?:-feira)?|sexta(?:-feira)?|sábado|sabado|domingo)|todo\s+(?:m[eê]s|dia)|recorrente|mensal|di[áa]rio|semanal|quinzenal|a\s+cada\s+15\s+dias`)

	// Urgency
	reExpired  = regexp.MustCompile(`(?i)(esgotado|acabou|encerrado|expirado)`)
	_ = regexp.MustCompile(`(?i)\b(corre|corram)\b`)
	_ = regexp.MustCompile(`(?i)[úu]ltima[s]?\s*unidade|acabando|esgotando`)
	_ = regexp.MustCompile(`(?i)envio\s+(nacional|do\s+brasil)`)
	// Temporalidade de promoções. Datas explícitas vencem sinais flash.
	reDateDMY           = regexp.MustCompile(`\b([0-3]?\d)/([01]?\d)(?:/(\d{2,4}))?\b`)
	reFlashTemporal     = regexp.MustCompile(`(?i)\b(corre|corram|rel[âa]mpago|tempo\s+limitado|s[oó]\s+hoje|apenas\s+hoje|24h|12h|oferta\s+do\s+dia|[úu]ltima[s]?\s+unidade[s]?|acabando|esgotando|agora)\b`)
	reRecurringWeekday  = regexp.MustCompile(`(?i)\btod[ao]\s+(segunda|terça|terca|quarta|quinta|sexta|s[áa]bado|sabado|domingo)(?:-feira)?\b`)
	reRecurringMonthly  = regexp.MustCompile(`(?i)\b(todo\s+m[eê]s|mensal)\b`)
	reRecurringDaily    = regexp.MustCompile(`(?i)\b(todo\s+dia|di[áa]rio)\b`)
	reRecurringBiweekly = regexp.MustCompile(`(?i)\b(a\s+cada\s+15\s+dias|quinzenal)\b`)
	reRecurringWeekly   = regexp.MustCompile(`(?i)\b(toda\s+semana|semanal)\b`)
	reSeasonBlackFriday = regexp.MustCompile(`(?i)\bblack\s+friday\b`)
	reSeasonChristmas   = regexp.MustCompile(`(?i)\b(natal|christmas)\b`)
	reSeasonMothersDay  = regexp.MustCompile(`(?i)\bdia\s+das\s+m[ãa]es\b`)
	reSeasonFathersDay  = regexp.MustCompile(`(?i)\bdia\s+dos\s+pais\b`)
	reSeasonConsumer    = regexp.MustCompile(`(?i)\bsemana\s+do\s+consumidor\b`)
	reSeasonBackSchool  = regexp.MustCompile(`(?i)\bvolta\s+[àa]s\s+aulas\b`)
)

// Normalize transforma um payload bruto (Shape A ou B) em NormalizedMessage.
func Normalize(raw *model.RawMessage) (*NormalizedMessage, error) {
	payload, err := model.DecodePayloadMap(raw.Payload)
	if err != nil {
		return nil, fmt.Errorf("processor: normalize: unmarshal payload: %w", err)
	}

	msg, err := extractMessage(payload)
	if err != nil {
		return nil, fmt.Errorf("processor: normalize: %w", err)
	}

	nm := &NormalizedMessage{
		RawMessageID: raw.ID,
		ReceivedAt:   raw.ReceivedAt,
		ProcessedAt:  time.Now().UTC(),
	}

	nm.MessageID = model.JSONToInt64(msg["ID"])
	nm.ChannelID = extractChannelID(msg)
	nm.PostedAt = extractDate(msg)
	nm.Text = cleanText(model.JSONToString(msg["Message"]))
	nm.TextLength = utf8.RuneCountInString(nm.Text)
	nm.MediaType = extractMediaType(msg)
	nm.PhotoID, nm.PhotoAccessHash, nm.PhotoFileRef, nm.PhotoDCID = extractPhotoMetadata(msg)
	nm.InlineThumb = extractInlineThumb(msg)
	nm.Views = model.JSONToInt(msg["Views"])
	nm.Forwards = model.JSONToInt(msg["Forwards"])
	nm.ReplyToMsgID = extractReplyTo(msg)

	// Extração de sinais
	webpage := extractWebpageInfo(msg)
	nm.WebpageURL = webpage.URL
	nm.WebpageTitle = webpage.Title
	nm.WebpageDesc = webpage.Description
	nm.HasURL = hasURL(nm.Text)
	nm.URLHash = computeURLHash(nm.Text)

	// Fallback: URL de Media.Webpage quando texto não tem URL.
	if !nm.HasURL && nm.WebpageURL != "" {
		nm.HasURL = true
		nm.URLHash = computeURLHash(nm.WebpageURL)
	}

	extractPrices(nm.Text, nm)
	applyCoupons(nm.Text, nm)
	applyModifiers(nm.Text, nm)
	pn := ExtractProductName(nm)
	nm.ProductName = pn.Name
	nm.ProductNameConfidence = pn.Confidence
	extractTemporalSignals(nm.Text, nm.PostedAt, nm)

	// Estágio 3: Síntese
	syn := Synthesize(nm)
	nm.Merchant = syn.Merchant
	// Modifiers preservam o sinal bruto de moedas/cashback; VirtualCurrency
	// extrai plataforma/valor/teto após a síntese do merchant.
	nm.VirtualCurrency = extractVirtualCurrency(nm.Text, nm.Merchant)
	if nm.RecurrencePattern != "" {
		nm.RecurrenceGroupID = recurrenceGroupID(nm)
	}
	synJSON, err := json.Marshal(syn)
	if err == nil {
		nm.Synthesis = string(synJSON)
	}

	return nm, nil
}

// extractPrices detecta preço dual (De X por Y) ou preço único.
// Se dual: PriceOriginal = X, PriceAmount = Y, PriceDiscount = calculado.
// Se único: PriceAmount = primeiro preço encontrado, PriceOriginal = 0.
func extractPrices(text string, nm *NormalizedMessage) {
	if dual := reDualPrice.FindStringSubmatch(text); dual != nil {
		var origRaw, finalRaw string
		if dual[1] != "" && dual[2] != "" {
			origRaw, finalRaw = dual[1], dual[2]
		} else if dual[3] != "" && dual[4] != "" {
			// "R$ X OFF em R$ Y" → original é Y, desconto é X, final = Y - X
			discountRaw := dual[3]
			origRaw = dual[4]
			origCents := parseBRL(origRaw)
			discCents := parseBRL(discountRaw)
			if origCents > 0 {
				nm.HasPrice = true
				nm.PriceOriginal = origCents
				if discCents > 0 && discCents < origCents {
					nm.PriceAmount = origCents - discCents
				} else {
					nm.PriceAmount = origCents
				}
				if nm.PriceOriginal > 0 && nm.PriceAmount > 0 {
					nm.PriceDiscount = int(math.Round((1.0 - float64(nm.PriceAmount)/float64(nm.PriceOriginal)) * 100))
				}
				return
			}
		}
		if origRaw != "" && finalRaw != "" {
			nm.HasPrice = true
			nm.PriceOriginal = parseBRL(origRaw)
			nm.PriceAmount = parseBRL(finalRaw)
			if nm.PriceOriginal > 0 && nm.PriceAmount > 0 {
				nm.PriceDiscount = max(int(math.Round((1.0-float64(nm.PriceAmount)/float64(nm.PriceOriginal))*100)), 0)
			}
			return
		}
	}

	// Preço único (R$)
	if match := rePrice.FindStringSubmatch(text); match != nil {
		nm.HasPrice = true
		nm.PriceAmount = parseBRL(match[1])
		return
	}

	// Fallback: preço sem R$ ("POR: 425 REAIS", "por apenas 99,90")
	if match := rePriceAlt.FindStringSubmatch(text); match != nil {
		nm.HasPrice = true
		nm.PriceAmount = parseBRL(match[1])
	}

	// Fallback: "XX REAIS" sem R$ - captura todos e seleciona o menor
	if matches := rePriceReais.FindAllStringSubmatch(text, -1); len(matches) > 0 {
		var minPrice int64
		for _, m := range matches {
			price := parseBRL(m[1])
			if price > 0 && (minPrice == 0 || price < minPrice) {
				minPrice = price
			}
		}
		if minPrice > 0 {
			nm.HasPrice = true
			nm.PriceAmount = minPrice
		}
	}
}

// applyCoupons mantém campos flat legados e preenche cupons estruturados.
func applyCoupons(text string, nm *NormalizedMessage) {
	nm.CouponCodes = extractCoupons(text)
	nm.HasCoupon = len(nm.CouponCodes) > 0
	nm.CouponCode = ""
	for _, coupon := range nm.CouponCodes {
		if coupon.Code != "" {
			nm.CouponCode = coupon.Code
			return
		}
	}
}

// applyModifiers mantém campos flat legados e preenche modifiers estruturados.
func applyModifiers(text string, nm *NormalizedMessage) {
	nm.Modifiers = extractModifiers(text)
	for _, modifier := range nm.Modifiers {
		switch modifier.Type {
		case "payment":
			if modifier.Value == "pix" {
				nm.PaymentMethod = "pix"
			}
		case "shipping":
			nm.Shipping = modifier.Value
			if modifier.Value == "frete_gratis" || modifier.Value == "frete_gratis_prime" {
				nm.ShippingFree = true
			}
		case "installments":
			nm.Installments = modifier.Value
			if n, ok := parseInstallmentsCount(modifier.Value); ok {
				nm.InstallmentsN = n
				if nm.PriceAmount > 0 {
					nm.InstallmentsValue = nm.PriceAmount / int64(n)
				}
			}
		case "cashback":
			nm.IsCashback = true
		case "discount":
			if pct, ok := parseDiscountPercentModifier(modifier.Value); ok {
				nm.DiscountPct = pct
			}
		case "recurring":
			nm.IsRecurring = true
		}
	}
}

func parseInstallmentsCount(value string) (int, bool) {
	raw, _, ok := strings.Cut(value, "x_")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	return n, err == nil && n > 0
}

func parseDiscountPercentModifier(value string) (int, bool) {
	raw, _, ok := strings.Cut(value, "_percent")
	if !ok {
		return 0, false
	}
	n, err := strconv.Atoi(raw)
	return n, err == nil && n > 0 && n <= 100
}

func extractTemporalSignals(text string, postedAt time.Time, nm *NormalizedMessage) {
	nm.ValidFrom, nm.ValidUntil = extractExplicitValidity(text, postedAt)
	nm.RecurrencePattern = extractRecurrencePattern(text)
	nm.SeasonalTag = extractSeasonalTag(text)
	if nm.RecurrencePattern != "" {
		nm.IsRecurring = true
	}
	if nm.ValidFrom.IsZero() && nm.ValidUntil.IsZero() && reFlashTemporal.MatchString(text) {
		nm.Flash = true
	}
}

func extractExplicitValidity(text string, postedAt time.Time) (time.Time, time.Time) {
	matches := reDateDMY.FindAllStringSubmatch(text, -1)
	if len(matches) == 0 {
		return time.Time{}, time.Time{}
	}
	dates := make([]time.Time, 0, len(matches))
	for _, match := range matches {
		dt, ok := parseTemporalDate(match[1], match[2], match[3], postedAt)
		if ok {
			dates = append(dates, dt)
		}
	}
	if len(dates) == 0 {
		return time.Time{}, time.Time{}
	}
	if len(dates) >= 2 {
		return dates[0], dates[1]
	}
	return time.Time{}, dates[0]
}

func parseTemporalDate(dayRaw, monthRaw, yearRaw string, postedAt time.Time) (time.Time, bool) {
	day, err := strconv.Atoi(dayRaw)
	if err != nil {
		return time.Time{}, false
	}
	month, err := strconv.Atoi(monthRaw)
	if err != nil {
		return time.Time{}, false
	}
	year := 0
	if yearRaw != "" {
		year, err = strconv.Atoi(yearRaw)
		if err != nil {
			return time.Time{}, false
		}
		if year < 100 {
			year += 2000
		}
	} else if !postedAt.IsZero() {
		year = postedAt.UTC().Year()
	} else {
		year = time.Now().UTC().Year()
	}
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, false
	}
	dt := time.Date(year, time.Month(month), day, 23, 59, 59, 0, time.UTC)
	if dt.Day() != day || int(dt.Month()) != month {
		return time.Time{}, false
	}
	if yearRaw == "" && !postedAt.IsZero() && dt.Before(postedAt.UTC().AddDate(0, 0, -1)) {
		dt = dt.AddDate(1, 0, 0)
	}
	return dt, true
}

func extractRecurrencePattern(text string) string {
	if match := reRecurringWeekday.FindStringSubmatch(text); match != nil {
		switch strings.ToLower(match[1]) {
		case "segunda":
			return "weekly/monday"
		case "terça", "terca":
			return "weekly/tuesday"
		case "quarta":
			return "weekly/wednesday"
		case "quinta":
			return "weekly/thursday"
		case "sexta":
			return "weekly/friday"
		case "sábado", "sabado":
			return "weekly/saturday"
		case "domingo":
			return "weekly/sunday"
		}
	}
	switch {
	case reRecurringMonthly.MatchString(text):
		return "monthly"
	case reRecurringDaily.MatchString(text):
		return "daily"
	case reRecurringBiweekly.MatchString(text):
		return "biweekly"
	case reRecurringWeekly.MatchString(text):
		return "weekly"
	case reRecurring.MatchString(text):
		return "recurring"
	default:
		return ""
	}
}

func extractSeasonalTag(text string) string {
	switch {
	case reSeasonBlackFriday.MatchString(text):
		return "black_friday"
	case reSeasonChristmas.MatchString(text):
		return "christmas"
	case reSeasonMothersDay.MatchString(text):
		return "mothers_day"
	case reSeasonFathersDay.MatchString(text):
		return "fathers_day"
	case reSeasonConsumer.MatchString(text):
		return "consumer_week"
	case reSeasonBackSchool.MatchString(text):
		return "back_to_school"
	default:
		return ""
	}
}

func recurrenceGroupID(nm *NormalizedMessage) int64 {
	if nm.RecurrencePattern == "" {
		return 0
	}
	key := strings.ToLower(strings.Join([]string{
		nm.RecurrencePattern,
		nm.Merchant,
		nm.CouponCode,
		nm.URLHash,
	}, "|"))
	h := fnv.New64a()
	_, _ = h.Write([]byte(key))
	return int64(h.Sum64() & 0x7fffffffffffffff)
}

// computeURLHash retorna SHA-256 da primeira URL normalizada no texto.
func computeURLHash(text string) string {
	u := reURL.FindString(text)
	if u == "" {
		return ""
	}
	normalized := normalizeURL(u)
	h := sha256.Sum256([]byte(normalized))
	return hex.EncodeToString(h[:])
}

// normalizeURL lowercases o host, remove trailing slash e query params de tracking.
func normalizeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return strings.ToLower(strings.TrimRight(raw, "/"))
	}
	u.Host = strings.ToLower(u.Host)
	u.Fragment = ""
	q := u.Query()
	for _, k := range []string{"tag", "ref", "ref_", "campaign", "utm_source", "utm_medium", "utm_campaign", "utm_content", "utm_term"} {
		q.Del(k)
	}
	u.RawQuery = q.Encode()
	result := u.String()
	return strings.TrimRight(result, "/")
}

// --- Helpers de payload ---

func extractMessage(payload map[string]any) (map[string]any, error) {
	if updates, ok := payload["Updates"]; ok {
		arr, ok := updates.([]any)
		if !ok || len(arr) == 0 {
			return nil, fmt.Errorf("shape B: Updates não é array ou vazio")
		}
		first, ok := arr[0].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("shape B: Updates[0] não é objeto")
		}
		if inner, ok := first["Message"].(map[string]any); ok {
			return inner, nil
		}
		return first, nil
	}
	if _, hasID := payload["ID"]; hasID {
		return payload, nil
	}
	return nil, fmt.Errorf("shape desconhecido: nem Shape A (ID) nem Shape B (Updates)")
}

func extractChannelID(msg map[string]any) int64 {
	peerID, ok := msg["PeerID"].(map[string]any)
	if !ok {
		return 0
	}
	return model.JSONToInt64(peerID["ChannelID"])
}

func extractDate(msg map[string]any) time.Time {
	switch v := msg["Date"].(type) {
	case float64:
		if v == 0 {
			return time.Time{}
		}
		return time.Unix(int64(v), 0).UTC()
	case json.Number:
		n, err := v.Int64()
		if err != nil || n == 0 {
			return time.Time{}
		}
		return time.Unix(n, 0).UTC()
	}
	return time.Time{}
}

func extractMediaType(msg map[string]any) string {
	media, ok := msg["Media"]
	if !ok || media == nil {
		return "none"
	}
	mediaMap, ok := media.(map[string]any)
	if !ok {
		return "none"
	}
	if _, ok := mediaMap["Photo"]; ok {
		return "photo"
	}
	if _, ok := mediaMap["Video"]; ok {
		return "video"
	}
	if _, ok := mediaMap["Document"]; ok {
		return "document"
	}
	if _, ok := mediaMap["Poll"]; ok {
		return "poll"
	}
	if _, ok := mediaMap["Webpage"]; ok {
		return "webpage"
	}
	return "none"
}

// extractPhotoMetadata extrai os campos MTProto necessários para download
// sob demanda via upload.GetFile (ADR 011). Delega para model.ExtractPhotoFields
// (função compartilhada com telegram/media.go).
func extractPhotoMetadata(msg map[string]any) (int64, int64, string, int) {
	pf := model.ExtractPhotoFields(msg)
	return pf.PhotoID, pf.AccessHash, pf.FileRef, pf.DCID
}

// extractInlineThumb extrai o thumbnail inline (Type "i") de Media.Photo.Sizes.
// O Telegram inclui um thumbnail compacto (~230 bytes, base64) em cada mensagem
// com foto. Este thumbnail é sempre disponível (não depende de file_reference
// MTProto) e serve como fallback universal para o frontend (ADR 011 v2).
func extractInlineThumb(msg map[string]any) []byte {
	media, ok := msg["Media"].(map[string]any)
	if !ok {
		return nil
	}
	photo, ok := media["Photo"].(map[string]any)
	if !ok {
		return nil
	}
	sizes, ok := photo["Sizes"].([]any)
	if !ok {
		return nil
	}
	for _, s := range sizes {
		size, ok := s.(map[string]any)
		if !ok {
			continue
		}
		if size["Type"] == "i" {
			if b64, ok := size["Bytes"].(string); ok && b64 != "" {
				decoded, err := base64.StdEncoding.DecodeString(b64)
				if err == nil {
					return decoded
				}
			}
		}
	}
	return nil
}

func extractReplyTo(msg map[string]any) int64 {
	reply, ok := msg["ReplyTo"].(map[string]any)
	if !ok {
		return 0
	}
	return model.JSONToInt64(reply["ReplyToMsgID"])
}

func hasURL(text string) bool {
	return reURL.MatchString(text)
}

func isExpired(text string) bool {
	return reExpired.MatchString(text)
}

// --- Conversão de tipos ---

// parseBRL converte "1.234,56" ou "83" ou "99,90" em centavos (int64).
func parseBRL(raw string) int64 {
	raw = strings.ReplaceAll(raw, ".", "")
	raw = strings.ReplaceAll(raw, ",", ".")
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return 0
	}
	return int64(math.Round(f * 100))
}

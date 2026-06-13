package processor

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/limiar/collector/internal/storage"
)

// NormalizedMessage é a estrutura canônica produzida pelo Estágio 1 do
// processor. Representa qualquer payload bruto (Shape A ou B) em formato
// limpo e uniforme, pronto para classificação e persistência.
type NormalizedMessage struct {
	RawMessageID   int64
	MessageID      int64
	ChannelID      int64
	PostedAt       time.Time
	ReceivedAt     time.Time
	ProcessedAt    time.Time
	Text           string
	TextLength     int
	MediaType      string // "photo"|"video"|"document"|"poll"|"webpage"|"none"
	PhotoID         int64
	PhotoAccessHash int64  // MTProto access_hash para download sob demanda
	PhotoFileRef    string // MTProto file_reference (base64, renovável)
	PhotoDCID       int    // MTProto data center ID
	Views          int
	Forwards       int
	ReplyToMsgID   int64
	HasURL         bool
	HasPrice       bool
	HasCoupon      bool
	PriceAmount    int64  // centavos BRL — preço final (o que o usuário paga)
	PriceOriginal  int64  // centavos BRL — preço "De" (0 se não há dual price)
	PriceDiscount  int    // percentual de desconto 0-100 (calculado se dual price)
	CouponCode     string // código do cupom extraído (vazio se não há)
	PaymentMethod  string // "pix" | ""
	Shipping       string // "frete_gratis" | "frete_gratis_prime" | ""
	Installments   string // "9x_sem_juros" | ""
	DiscountPct    int    // "20% OFF" → 20 (do texto, não-calculado)
	IsCashback     bool   // cashback mencionado
	URLHash        string // SHA-256 da primeira URL normalizada
	Merchant       string // merchant inferido do domínio da URL
	ProductName    string // nome do produto (heurística)
	IsDuplicate    bool   // mesma URL já processada em outro canal
	FeedEligible   bool   // elegível para o feed (deal_complete ou deal_no_coupon)
	UrgencySignals []string
	MessageType    string // preenchido por Classify
	Synthesis      string // JSON serializado de SynthesizedPromotion
}

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
	reURL    = regexp.MustCompile(`https?://[^\s<>"']+`)
	rePrice  = regexp.MustCompile(`R\$\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?|[0-9]+(?:,[0-9]{2})?)`)
	// rePriceAlt catches "POR: 425 REAIS", "POR 65,47", "por apenas 99,90" without R$
	rePriceAlt = regexp.MustCompile(`(?i)(?:por|only|apenas)[:\s]+R?\$?\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?)\s*(?:reais|REAIS)?`)
	reCoupon = regexp.MustCompile(`(?i)(?:cup[ao]m|c[oó]digo|code)[\s:\n]+([A-Za-z0-9_]{3,25})`)

	// Dual price: "De R$ 429 por R$ 208,92", "De: R$ 429 | Por: R$ 208", "R$ 50 OFF em R$ 250"
	reDualPrice = regexp.MustCompile(
		`(?i)(?:de[:\s]+R\$\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?)[^\n]*?(?:por|à\s*vista|no\s*pix)[:\s]+R\$\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?))` +
			`|(?:R\$\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?)\s*OFF\s+em\s+R\$\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?))`)

	// Modifiers
	rePix       = regexp.MustCompile(`(?i)no\s*pix|via\s*pix|pagamento\s+pix|[àa]\s+vista\s+no\s+pix`)
	reFretePrime = regexp.MustCompile(`(?i)frete\s*gr[áa]tis\s*prime|prime.*frete\s*gr[áa]tis`)
	reFreteG    = regexp.MustCompile(`(?i)frete\s*gr[áa]tis`)
	reInstall   = regexp.MustCompile(`(?i)(\d+)x\s*(?:sem\s*juros|s/\s*juros)`)
	reCashback  = regexp.MustCompile(`(?i)cash\s*back|cashback`)
	reDiscPct   = regexp.MustCompile(`(?i)(\d+)\s*%\s*(?:de\s+)?(?:desconto|off|desc)`)

	// Urgency
	reExpired  = regexp.MustCompile(`(?i)(esgotado|acabou|encerrado|expirado)`)
	reCorre    = regexp.MustCompile(`(?i)\b(corre|corram)\b`)
	reUltima   = regexp.MustCompile(`(?i)[úu]ltima[s]?\s*unidade|acabando|esgotando`)
	reNacional = regexp.MustCompile(`(?i)envio\s+(nacional|do\s+brasil)`)
)

// Normalize transforma um payload bruto (Shape A ou B) em NormalizedMessage.
func Normalize(raw *storage.RawMessage) (*NormalizedMessage, error) {
	var payload map[string]any
	if err := json.Unmarshal(raw.Payload, &payload); err != nil {
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

	nm.MessageID = toInt64(msg["ID"])
	nm.ChannelID = extractChannelID(msg)
	nm.PostedAt = extractDate(msg)
	nm.Text = cleanText(toString(msg["Message"]))
	nm.TextLength = utf8.RuneCountInString(nm.Text)
	nm.MediaType = extractMediaType(msg)
	nm.PhotoID, nm.PhotoAccessHash, nm.PhotoFileRef, nm.PhotoDCID = extractPhotoMetadata(msg)
	nm.Views = toInt(msg["Views"])
	nm.Forwards = toInt(msg["Forwards"])
	nm.ReplyToMsgID = extractReplyTo(msg)

	// Extração de sinais
	nm.HasURL = hasURL(nm.Text)
	nm.URLHash = computeURLHash(nm.Text)

	// 2.4: Fallback — URL de Media.Webpage quando texto não tem URL
	if !nm.HasURL {
		if wpURL := extractWebpageURL(msg); wpURL != "" {
			nm.HasURL = true
			nm.URLHash = computeURLHash(wpURL)
		}
	}

	extractPrices(nm.Text, nm)
	extractCoupon(nm.Text, nm)
	extractModifiers(nm.Text, nm)
	nm.UrgencySignals = extractUrgency(nm.Text)

	// Estágio 3: Síntese
	syn := Synthesize(nm)
	nm.Merchant = syn.Merchant
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
				nm.PriceDiscount = int(math.Round((1.0 - float64(nm.PriceAmount)/float64(nm.PriceOriginal)) * 100))
				if nm.PriceDiscount < 0 {
					nm.PriceDiscount = 0
				}
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
}

// extractCoupon extrai código de cupom e seta HasCoupon + CouponCode.
func extractCoupon(text string, nm *NormalizedMessage) {
	if match := reCoupon.FindStringSubmatch(text); match != nil {
		nm.HasCoupon = true
		nm.CouponCode = strings.ToUpper(match[1])
	}
}

// extractModifiers detecta Pix, frete grátis, parcelamento, cashback, % desconto.
func extractModifiers(text string, nm *NormalizedMessage) {
	if rePix.MatchString(text) {
		nm.PaymentMethod = "pix"
	}
	if reFretePrime.MatchString(text) {
		nm.Shipping = "frete_gratis_prime"
	} else if reFreteG.MatchString(text) {
		nm.Shipping = "frete_gratis"
	}
	if match := reInstall.FindStringSubmatch(text); match != nil {
		nm.Installments = match[1] + "x_sem_juros"
	}
	if reCashback.MatchString(text) {
		nm.IsCashback = true
	}
	if match := reDiscPct.FindStringSubmatch(text); match != nil {
		n, _ := strconv.Atoi(match[1])
		if n > 0 && n <= 100 {
			nm.DiscountPct = n
		}
	}
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
	return toInt64(peerID["ChannelID"])
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
// sob demanda via upload.GetFile (ADR 011). Retorna (0, 0, "", 0) se a
// mensagem não contém Media.Photo.
//
// NOTA: internal/telegram/media.go possui uma cópia independente desta função
// (photoMetaFromPayload) porque telegram não pode importar processor (dep. reversa).
// Se o schema do payload mudar, atualize AMBAS.
func extractPhotoMetadata(msg map[string]any) (int64, int64, string, int) {
	media, ok := msg["Media"].(map[string]any)
	if !ok {
		return 0, 0, "", 0
	}
	photo, ok := media["Photo"].(map[string]any)
	if !ok {
		return 0, 0, "", 0
	}
	photoID := toInt64(photo["ID"])
	accessHash := toInt64(photo["AccessHash"])
	fileRef, _ := photo["FileReference"].(string)
	dcid := toInt(photo["DCID"])
	return photoID, accessHash, fileRef, dcid
}

func extractReplyTo(msg map[string]any) int64 {
	reply, ok := msg["ReplyTo"].(map[string]any)
	if !ok {
		return 0
	}
	return toInt64(reply["ReplyToMsgID"])
}

// extractWebpageURL extrai URL de Media.Webpage quando presente.
func extractWebpageURL(msg map[string]any) string {
	media, ok := msg["Media"].(map[string]any)
	if !ok {
		return ""
	}
	wp, ok := media["Webpage"].(map[string]any)
	if !ok {
		return ""
	}
	u, _ := wp["URL"].(string)
	return u
}

func hasURL(text string) bool {
	return reURL.MatchString(text)
}

func isExpired(text string) bool {
	return reExpired.MatchString(text)
}

func extractUrgency(text string) []string {
	var signals []string
	if reCorre.MatchString(text) {
		signals = append(signals, "corre")
	}
	if reFreteG.MatchString(text) {
		signals = append(signals, "frete_gratis")
	}
	if reUltima.MatchString(text) {
		signals = append(signals, "ultima_unidade")
	}
	if reNacional.MatchString(text) {
		signals = append(signals, "envio_nacional")
	}
	return signals
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

func toInt64(v any) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case json.Number:
		i, _ := n.Int64()
		return i
	case int64:
		return n
	case int:
		return int64(n)
	}
	return 0
}

func toInt(v any) int {
	return int(toInt64(v))
}

func toString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case json.Number:
		return s.String()
	}
	return ""
}

package processor

import (
	"encoding/json"
	"fmt"
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
	PhotoID        int64
	Views          int
	Forwards       int
	ReplyToMsgID   int64
	HasURL         bool
	HasPrice       bool
	HasCoupon      bool
	PriceAmount    int64 // centavos de BRL
	UrgencySignals []string
	MessageType    string // preenchido por Classify
}

// reWhitespace colapsa 3+ newlines consecutivos em 2.
var reWhitespace = regexp.MustCompile(`\n{3,}`)

// Normalize transforma um payload bruto (Shape A ou B) em NormalizedMessage.
// Shape A: payload direto do tg.Message (history backfill)
// Shape B: wrapper tg.Updates com Updates[0].Message (live capture)
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
	nm.PhotoID = extractPhotoID(msg)
	nm.Views = toInt(msg["Views"])
	nm.Forwards = toInt(msg["Forwards"])
	nm.ReplyToMsgID = extractReplyTo(msg)

	nm.HasURL = hasURL(nm.Text)
	nm.HasPrice, nm.PriceAmount = extractPrice(nm.Text)
	nm.HasCoupon = hasCoupon(nm.Text)
	nm.UrgencySignals = extractUrgency(nm.Text)

	return nm, nil
}

// extractMessage detecta o shape e retorna o map da mensagem real.
func extractMessage(payload map[string]any) (map[string]any, error) {
	// Shape B: wrapper com campo "Updates"
	if updates, ok := payload["Updates"]; ok {
		arr, ok := updates.([]any)
		if !ok || len(arr) == 0 {
			return nil, fmt.Errorf("shape B: Updates não é array ou vazio")
		}
		first, ok := arr[0].(map[string]any)
		if !ok {
			return nil, fmt.Errorf("shape B: Updates[0] não é objeto")
		}
		// Updates[0] pode ter "Message" diretamente ou ser o próprio update
		if inner, ok := first["Message"].(map[string]any); ok {
			return inner, nil
		}
		return first, nil
	}

	// Shape A: payload direto do tg.Message
	if _, hasID := payload["ID"]; hasID {
		return payload, nil
	}

	return nil, fmt.Errorf("shape desconhecido: nem Shape A (ID) nem Shape B (Updates)")
}

// extractChannelID extrai PeerID.ChannelID do payload.
func extractChannelID(msg map[string]any) int64 {
	peerID, ok := msg["PeerID"].(map[string]any)
	if !ok {
		return 0
	}
	return toInt64(peerID["ChannelID"])
}

// extractDate converte o campo Date (Unix timestamp int) para time.Time.
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

// extractMediaType detecta o tipo de media presente na mensagem.
func extractMediaType(msg map[string]any) string {
	media, ok := msg["Media"]
	if !ok || media == nil {
		return "none"
	}
	mediaMap, ok := media.(map[string]any)
	if !ok {
		return "none"
	}
	// tg types usam campo "_" ou nomes específicos
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

// extractPhotoID extrai Media.Photo.ID quando presente.
func extractPhotoID(msg map[string]any) int64 {
	media, ok := msg["Media"].(map[string]any)
	if !ok {
		return 0
	}
	photo, ok := media["Photo"].(map[string]any)
	if !ok {
		return 0
	}
	return toInt64(photo["ID"])
}

// extractReplyTo extrai ReplyTo.ReplyToMsgID quando presente.
func extractReplyTo(msg map[string]any) int64 {
	reply, ok := msg["ReplyTo"].(map[string]any)
	if !ok {
		return 0
	}
	return toInt64(reply["ReplyToMsgID"])
}

// cleanText normaliza whitespace: NBSP → espaço, 3+ newlines → 2.
// Preserva emojis e formatação original.
func cleanText(s string) string {
	s = strings.ReplaceAll(s, "\u00a0", " ")
	s = reWhitespace.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// --- Detecções via regex ---

var (
	reURL     = regexp.MustCompile(`https?://[^\s<>"']+`)
	rePrice   = regexp.MustCompile(`R\$\s*([0-9]{1,3}(?:\.[0-9]{3})+(?:,[0-9]{2})?|[0-9]+(?:,[0-9]{2})?)`)
	reCoupon  = regexp.MustCompile(`(?i)cupom[:\s]+([A-Z0-9_]{3,25})`)
	reExpired = regexp.MustCompile(`(?i)(esgotado|acabou|encerrado|expirado)`)
	reCorre   = regexp.MustCompile(`(?i)\b(corre|corram)\b`)
	reFreteG  = regexp.MustCompile(`(?i)frete\s+gr[áa]tis`)
	reUltima  = regexp.MustCompile(`(?i)[úu]ltima[s]?\s*unidade|acabando|esgotando`)
	reNacional = regexp.MustCompile(`(?i)envio\s+(nacional|do\s+brasil)`)
)

func hasURL(text string) bool {
	return reURL.MatchString(text)
}

// extractPrice retorna (true, centavos) se um preço BRL é encontrado.
func extractPrice(text string) (bool, int64) {
	match := rePrice.FindStringSubmatch(text)
	if match == nil {
		return false, 0
	}
	// match[1] = "1.234,56" ou "83" ou "99,90"
	raw := match[1]
	raw = strings.ReplaceAll(raw, ".", "")
	raw = strings.ReplaceAll(raw, ",", ".")
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return false, 0
	}
	return true, int64(f * 100)
}

func hasCoupon(text string) bool {
	return reCoupon.MatchString(text)
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

// --- Helpers de conversão de tipo ---

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

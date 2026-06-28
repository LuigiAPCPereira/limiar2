package model

import "encoding/json"

// BoolToInt converte bool para int (1/0) para persistência no SQLite/Turso.
func BoolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// JSONToInt64 converte um valor de payload JSON desserializado (any) para int64.
// Suporta float64 (padrão json.Unmarshal), json.Number, int64 e int.
func JSONToInt64(v any) int64 {
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

// JSONToInt converte um valor de payload JSON desserializado (any) para int.
func JSONToInt(v any) int {
	return int(JSONToInt64(v))
}

// JSONToString converte um valor de payload JSON desserializado (any) para string.
func JSONToString(v any) string {
	switch s := v.(type) {
	case string:
		return s
	case json.Number:
		return s.String()
	}
	return ""
}

// PhotoFields contém os campos MTProto de foto extraídos de um payload JSON.
type PhotoFields struct {
	PhotoID    int64
	AccessHash int64
	FileRef    string
	DCID       int
}

// ExtractPhotoFields navega msg["Media"]["Photo"] e extrai os campos MTProto
// necessários para download. Retorna zero-value se a mensagem não contém foto.
// Compartilhado por processor/normalizer.go e telegram/media.go para evitar
// duplicação da navegação de payload (anteriormente documentada como "atualize AMBAS").
func ExtractPhotoFields(msg map[string]any) PhotoFields {
	media, ok := msg["Media"].(map[string]any)
	if !ok {
		return PhotoFields{}
	}
	photo, ok := media["Photo"].(map[string]any)
	if !ok {
		return PhotoFields{}
	}
	return PhotoFields{
		PhotoID:    JSONToInt64(photo["ID"]),
		AccessHash: JSONToInt64(photo["AccessHash"]),
		FileRef:    JSONToString(photo["FileReference"]),
		DCID:       JSONToInt(photo["DCID"]),
	}
}

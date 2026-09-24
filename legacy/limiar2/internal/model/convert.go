package model

import (
	"bytes"
	"encoding/json"
)

// DecodePayloadMap desserializa payloads brutos preservando números como
// json.Number. IDs MTProto são inteiros de 64 bits e não cabem com segurança no
// mantissa de float64; usar json.Unmarshal direto em map[string]any arredonda
// photo_id/access_hash e faz imagens de produtos diferentes colidirem.
func DecodePayloadMap(data []byte) (map[string]any, error) {
	var payload map[string]any
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// ExtractPayloadMessage localiza o objeto Message em payloads brutos Shape A ou
// Shape B. Retorna false quando o payload não contém uma mensagem Telegram.
func ExtractPayloadMessage(payload map[string]any) (map[string]any, bool) {
	if updates, ok := payload["Updates"]; ok {
		arr, ok := updates.([]any)
		if !ok || len(arr) == 0 {
			return nil, false
		}
		first, ok := arr[0].(map[string]any)
		if !ok {
			return nil, false
		}
		if inner, ok := first["Message"].(map[string]any); ok {
			return inner, true
		}
		return first, true
	}
	if _, hasID := payload["ID"]; hasID {
		return payload, true
	}
	return nil, false
}

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

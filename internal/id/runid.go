// Package id fornece helpers para geração de identificadores únicos.
package id

import (
	"crypto/rand"
	"encoding/hex"
)

// NewRunID produz 8 bytes aleatórios formatados como hex (16 chars).
// Usa crypto/rand para garantir unicidade entre execuções.
// Em caso de falha catastrófica do gerador de números aleatórios,
// retorna "0000000000000000" como fallback.
func NewRunID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}

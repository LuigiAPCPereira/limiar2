package processor

import (
	"strings"
	"testing"
	"unicode"
)

// Testes RED da normalização pós-extração (Stage 3) do CRE — Sprint 2.
//
// API planejada (símbolo ainda inexistente):
//
//	func normalizeProductName(name string) string
//
// Limpa o nome escolhido antes de persistir: remove sufixo de merchant, SKU
// trailing, ruído de promoção residual e corrige ALL CAPS para casing legível.

// TestNormalizeProductNameRemoveSufixoMerchant verifica a remoção do sufixo do
// merchant colado ao nome extraído do título da página.
func TestNormalizeProductNameRemoveSufixoMerchant(t *testing.T) {
	got := normalizeProductName("Apple iPhone 16 (128 GB) - Amazon.com.br")
	const want = "Apple iPhone 16 (128 GB)"
	if got != want {
		t.Fatalf("normalizeProductName = %q, quer %q", got, want)
	}
}

// TestNormalizeProductNameRemoveSKUTrailing verifica a remoção do SKU trailing
// no final do nome.
func TestNormalizeProductNameRemoveSKUTrailing(t *testing.T) {
	got := normalizeProductName("Teclado Mecânico - XYZ-BLU-L-2024")
	const want = "Teclado Mecânico"
	if got != want {
		t.Fatalf("normalizeProductName = %q, quer %q", got, want)
	}
}

// TestNormalizeProductNameRemovePromoNoise verifica a remoção de ruído de
// promoção residual colado ao nome.
func TestNormalizeProductNameRemovePromoNoise(t *testing.T) {
	got := normalizeProductName("Fone JBL FRETE GRÁTIS")
	const want = "Fone JBL"
	if got != want {
		t.Fatalf("normalizeProductName = %q, quer %q", got, want)
	}
}

// TestNormalizeProductNameCorrigeAllCaps verifica que um nome inteiramente em
// CAIXA ALTA é convertido para casing legível (Title Case). Não pinamos a
// string exata para não acoplar ao tratamento de stopwords (de/da/e); pinamos
// o contrato observável: deixou de ser ALL CAPS e a primeira letra é
// maiúscula. Um scorer/normalizer que ignora ALL CAPS deixaria a saída
// idêntica à entrada.
func TestNormalizeProductNameCorrigeAllCaps(t *testing.T) {
	const in = "CAIXA DE SOM BLUETOOTH"
	got := normalizeProductName(in)

	if got == in {
		t.Fatalf("normalizeProductName não alterou o ALL CAPS: %q", got)
	}
	if strings.ToUpper(got) == got {
		t.Fatalf("normalizeProductName manteve ALL CAPS: %q, quer casing legível", got)
	}
	if !primeiraLetraMaiuscula(got) {
		t.Fatalf("normalizeProductName = %q, quer primeira letra maiúscula", got)
	}
}

// primeiraLetraMaiuscula informa se o primeiro rune do nome é uma letra
// maiúscula (lida via unicode para cobrir acentos).
func primeiraLetraMaiuscula(s string) bool {
	for _, r := range s {
		return unicode.IsUpper(r)
	}
	return false
}

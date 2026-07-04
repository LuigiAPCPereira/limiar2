package processor

import (
	"testing"

	"github.com/limiar/collector/internal/model"
)

// Testes RED do Candidate Ranking Engine (CRE) — Sprint 2 (ProductName).
//
// Estes testes definem o contrato público do CRE ANTES da implementação.
// A API planejada (símbolos ainda inexistentes) mora em internal/processor:
//
//	type ProductNameResult struct {
//	    Name         string
//	    Confidence   float64 // 0.0–1.0
//	    Source       string  // "webpage_title" | "text_heuristic" | "url_title"
//	    SourceDetail string
//	}
//
//	func ExtractProductName(msg *model.NormalizedMessage) ProductNameResult
//
//	const productNameConfidenceThreshold = 0.45
//
// Threshold 0.45: abaixo dele o nome fica vazio, mas o score é registrado
// para a Fase 3 (LLM) saber quando intervir. CPU-only, sem URL resolver.

// TestExtractProductNameTextoHeuristico verifica que o CRE escolhe a linha do
// produto mesmo quando a linha 0 é um meta-anúncio (PARCELADO + emojis) e o
// preço aparece depois. O vencedor deve vir da fonte text_heuristic com
// confiança acima do threshold — a meta-penalty derruba a linha 0 e o bônus de
// marca/posição eleva a linha do produto.
func TestExtractProductNameTextoHeuristico(t *testing.T) {
	msg := &model.NormalizedMessage{
		Text: "PARCELADO🔥🔥🔥🔥\n" +
			"Anker Caixa de Som Soundcore Select 4 go\n" +
			"R$ 154,90 e frete grátis",
	}

	got := ExtractProductName(msg)

	const want = "Anker Caixa de Som Soundcore Select 4 go"
	if got.Name != want {
		t.Fatalf("ExtractProductName.Name = %q, quer %q", got.Name, want)
	}
	if got.Source != "text_heuristic" {
		t.Fatalf("ExtractProductName.Source = %q, quer %q", got.Source, "text_heuristic")
	}
	if got.Confidence < productNameConfidenceThreshold {
		t.Fatalf("ExtractProductName.Confidence = %v, quer >= %v", got.Confidence, productNameConfidenceThreshold)
	}
}

// TestExtractProductNamePrefereWebpageTitle verifica que, quando há
// WebpageTitle (gold standard), ele vence sobre texto genérico. O sufixo do
// merchant ("- Amazon.com.br") deve ser removido na normalização (Stage 3).
func TestExtractProductNamePrefereWebpageTitle(t *testing.T) {
	msg := &model.NormalizedMessage{
		Text:         "Promoção imperdível, confira agora.",
		WebpageTitle: "Apple iPhone 16 (128 GB) - Amazon.com.br",
	}

	got := ExtractProductName(msg)

	const want = "Apple iPhone 16 (128 GB)"
	if got.Name != want {
		t.Fatalf("ExtractProductName.Name = %q, quer %q (sufixo do merchant removido)", got.Name, want)
	}
	if got.Source != "webpage_title" {
		t.Fatalf("ExtractProductName.Source = %q, quer %q", got.Source, "webpage_title")
	}
	if got.Confidence < productNameConfidenceThreshold {
		t.Fatalf("ExtractProductName.Confidence = %v, quer >= %v", got.Confidence, productNameConfidenceThreshold)
	}
}

// TestExtractProductNameMetaAnuncioAbaixoThreshold verifica que um texto que só
// contém meta-anúncio/cupom (sem produto real) produz nome vazio, mas a
// confiança registra um score positivo abaixo do threshold (sinal para a Fase 3
// de que aquela mensagem precisa de LLM).
func TestExtractProductNameMetaAnuncioAbaixoThreshold(t *testing.T) {
	msg := &model.NormalizedMessage{
		Text: "NOVO CUPOM DA SEMANA\nAproveite a oferta exclusiva",
	}

	got := ExtractProductName(msg)

	if got.Name != "" {
		t.Fatalf("ExtractProductName.Name = %q, quer vazio (abaixo do threshold)", got.Name)
	}
	if got.Confidence <= 0 {
		t.Fatalf("ExtractProductName.Confidence = %v, quer > 0 (score deve ser registrado)", got.Confidence)
	}
	if got.Confidence >= productNameConfidenceThreshold {
		t.Fatalf("ExtractProductName.Confidence = %v, quer < %v (abaixo do threshold)", got.Confidence, productNameConfidenceThreshold)
	}
}

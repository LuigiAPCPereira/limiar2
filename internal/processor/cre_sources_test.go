package processor

import (
	"strings"
	"testing"
)

// Testes RED das fontes de candidatos (Stage 1) do CRE — Sprint 2.
//
// API planejada (símbolos ainda inexistentes):
//
//	type Candidate struct {
//	    Text           string
//	    Source         string  // "webpage_title" | "text_heuristic" | "url_title"
//	    Detail         string  // "line:0", "line:0-1", "near_price", "webpage"
//	    BaseConfidence float64
//	}
//
//	func textHeuristicCandidates(text string) []Candidate
//	func webpageTitleCandidates(title string) []Candidate

// detalhesTextHeuristicValidos é o conjunto de Detail reconhecidos para a fonte
// text_heuristic. Garante que cada candidato carrega um detalhe verificável
// (rastreabilidade do CRE: saber de onde veio cada nome candidato).
var detalhesTextHeuristicValidos = map[string]bool{
	"line:0":     true,
	"line:0-1":   true,
	"near_price": true,
}

// TestTextHeuristicCandidatesGeraCandidatos verifica que a fonte gera
// candidatos a partir das primeiras linhas não vazias e da linha próxima ao
// preço, cada um com Source "text_heuristic" e Detail verificável.
func TestTextHeuristicCandidatesGeraCandidatos(t *testing.T) {
	// Primeira linha é um nome de produto legítimo (não meta-anúncio); a última
	// linha traz o preço em formato BRL canônico.
	text := "Monitor Gamer SuperFrame Enterprise 34 Pol\n" +
		"Painel Curvo 3440x1440 144Hz\n" +
		"R$ 1.299,90 à vista no PIX"

	cands := textHeuristicCandidates(text)

	if len(cands) < 2 {
		t.Fatalf("textHeuristicCandidates gerou %d candidatos, quer >= 2 (primeiras linhas + near_price)", len(cands))
	}

	temLinha0 := false
	temNearPrice := false
	for i, c := range cands {
		if c.Source != "text_heuristic" {
			t.Fatalf("candidato %d Source = %q, quer %q", i, c.Source, "text_heuristic")
		}
		if c.Text == "" {
			t.Fatalf("candidato %d com Text vazio (fonte não deve emitir candidato sem texto)", i)
		}
		if !detalhesTextHeuristicValidos[c.Detail] {
			t.Fatalf("candidato %d Detail = %q, não é um detalhe reconhecido", i, c.Detail)
		}
		if c.Detail == "line:0" {
			temLinha0 = true
			if !strings.Contains(c.Text, "Monitor") {
				t.Fatalf("candidato line:0 Text = %q, deveria conter a primeira linha 'Monitor ...'", c.Text)
			}
		}
		if c.Detail == "near_price" {
			temNearPrice = true
		}
	}
	if !temLinha0 {
		t.Fatalf("nenhum candidato com Detail 'line:0' foi gerado a partir da primeira linha")
	}
	if !temNearPrice {
		t.Fatalf("nenhum candidato com Detail 'near_price' foi gerado apesar de haver preço no texto")
	}
}

// TestWebpageTitleCandidatesVazio verifica que título vazio não produz
// candidato (sem gold standard disponível).
func TestWebpageTitleCandidatesVazio(t *testing.T) {
	cands := webpageTitleCandidates("")
	if len(cands) != 0 {
		t.Fatalf("webpageTitleCandidates(\"\") = %d candidatos, quer 0", len(cands))
	}
}

// TestWebpageTitleCandidatesPresente verifica que título presente produz
// exatamente 1 candidato gold standard com Source "webpage_title",
// Detail "webpage" e BaseConfidence 0.60.
func TestWebpageTitleCandidatesPresente(t *testing.T) {
	const title = "Apple iPhone 16 (128 GB) - Amazon.com.br"

	cands := webpageTitleCandidates(title)
	if len(cands) != 1 {
		t.Fatalf("webpageTitleCandidates gerou %d candidatos, quer exatamente 1", len(cands))
	}
	c := cands[0]
	if c.Source != "webpage_title" {
		t.Fatalf("Source = %q, quer %q", c.Source, "webpage_title")
	}
	if c.Detail != "webpage" {
		t.Fatalf("Detail = %q, quer %q", c.Detail, "webpage")
	}
	if c.BaseConfidence != 0.60 {
		t.Fatalf("BaseConfidence = %v, quer 0.60 (gold standard direto do <title>)", c.BaseConfidence)
	}
	if c.Text == "" {
		t.Fatalf("Text vazio, quer o título informado")
	}
}

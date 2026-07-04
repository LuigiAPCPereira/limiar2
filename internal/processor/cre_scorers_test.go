package processor

import (
	"testing"

	"github.com/limiar/collector/internal/model"
)

// Testes RED dos scorers (Stage 2) do CRE — Sprint 2.
//
// API planejada (símbolos ainda inexistentes). Cada scorer recebe um candidato
// e a mensagem normalizada e devolve um ajuste de confiança (bônus >= 0 ou
// penalidade <= 0). O score final é BaseConfidence + somatório, clampado em
// [0.0, 1.0]:
//
//	func scorePosition(c Candidate, msg *model.NormalizedMessage) float64
//	func scoreBrandMatch(c Candidate, msg *model.NormalizedMessage) float64
//	func scoreTechDensity(c Candidate, msg *model.NormalizedMessage) float64
//	func scoreMetaPenalty(c Candidate, msg *model.NormalizedMessage) float64

// msgDeTeste é uma mensagem mínima, read-only, para exercitar scorers que
// dependem apenas do candidato (Detail/Text). Scorer puro não a muta.
func msgDeTeste() *model.NormalizedMessage { return &model.NormalizedMessage{} }

// TestScorePositionLinha0VsLinha1 verifica que a linha 0 recebe bônus maior que
// a linha 1, e que ambos somam positivo (posição alta no texto é sinal de nome
// de produto). Um scorer que ignora o Detail deixaria os dois iguais.
func TestScorePositionLinha0VsLinha1(t *testing.T) {
	linha0 := scorePosition(Candidate{Text: "Anker Soundcore", Detail: "line:0"}, msgDeTeste())
	linha1 := scorePosition(Candidate{Text: "Anker Soundcore", Detail: "line:1"}, msgDeTeste())

	if linha0 <= 0 {
		t.Fatalf("scorePosition(line:0) = %v, quer > 0 (bônus total)", linha0)
	}
	if linha1 <= 0 {
		t.Fatalf("scorePosition(line:1) = %v, quer > 0 (bônus parcial)", linha1)
	}
	if linha0 <= linha1 {
		t.Fatalf("scorePosition(line:0) = %v não é maior que line:1 = %v", linha0, linha1)
	}
}

// TestScoreBrandMatchMarcasConhecidas verifica que candidatos com marca
// conhecida (Anker, Apple, HyperX) recebem bônus positivo, e candidatos sem
// marca conhecida recebem bônus zero (sem inventar sinal).
func TestScoreBrandMatchMarcasConhecidas(t *testing.T) {
	tests := []struct {
		name     string
		texto    string
		querZero bool
	}{
		{"marca Anker", "Anker Caixa de Som Soundcore Select 4 go", false},
		{"marca Apple", "Apple iPhone 16 (128 GB)", false},
		{"marca HyperX", "HyperX Cloud II Headset Gamer", false},
		{"sem marca conhecida", "Caixa de Som Genérica Modelo X", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scoreBrandMatch(Candidate{Text: tt.texto, Detail: "line:0"}, msgDeTeste())
			if tt.querZero {
				if got != 0 {
					t.Fatalf("scoreBrandMatch(%q) = %v, quer 0 (sem marca conhecida)", tt.texto, got)
				}
				return
			}
			if got <= 0 {
				t.Fatalf("scoreBrandMatch(%q) = %v, quer > 0 (marca conhecida)", tt.texto, got)
			}
		})
	}
}

// TestScoreTechDensityTokensTecnicos verifica que densidade de tokens técnicos
// (GB, Hz, RAM, pol) rende bônus positivo, e texto sem especificações técnicas
// rende bônus zero.
func TestScoreTechDensityTokensTecnicos(t *testing.T) {
	comSpecs := scoreTechDensity(Candidate{Text: "Smartphone 256GB 8GB RAM 120Hz 6.7pol"}, msgDeTeste())
	semSpecs := scoreTechDensity(Candidate{Text: "Caixa de Som Portátil sem especificações"}, msgDeTeste())

	if comSpecs <= 0 {
		t.Fatalf("scoreTechDensity(com specs) = %v, quer > 0", comSpecs)
	}
	if semSpecs != 0 {
		t.Fatalf("scoreTechDensity(sem specs) = %v, quer 0", semSpecs)
	}
	if comSpecs <= semSpecs {
		t.Fatalf("scoreTechDensity com specs (%v) não superou sem specs (%v)", comSpecs, semSpecs)
	}
}

// TestScoreMetaPenaltyMetaAnuncio verifica que texto de meta-anúncio (CUPOM,
// PARCELADO) sofre penalidade negativa, e texto limpo de produto não sofre
// penalidade (zero).
func TestScoreMetaPenaltyMetaAnuncio(t *testing.T) {
	tests := []struct {
		name           string
		texto          string
		querPenalidade bool
	}{
		{"CUPOM", "CUPOM EXCLUSIVO DA SEMANA", true},
		{"PARCELADO", "PARCELADO SEM JUROS NO CARTÃO", true},
		{"texto limpo de produto", "Anker Caixa de Som Soundcore Select 4 go", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scoreMetaPenalty(Candidate{Text: tt.texto, Detail: "line:0"}, msgDeTeste())
			if tt.querPenalidade {
				if got >= 0 {
					t.Fatalf("scoreMetaPenalty(%q) = %v, quer < 0 (penalidade)", tt.texto, got)
				}
				return
			}
			if got != 0 {
				t.Fatalf("scoreMetaPenalty(%q) = %v, quer 0 (sem meta-anúncio)", tt.texto, got)
			}
		})
	}
}

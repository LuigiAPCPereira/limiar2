package processor

const (
	productNameConfidenceThreshold = 0.45

	productNameSourceWebpageTitle = "webpage_title"
	productNameSourceTextHeuristic = "text_heuristic"
	productNameSourceURLTitle     = "url_title"
)

// ProductNameResult é o resultado determinístico do CRE para nome de produto.
type ProductNameResult struct {
	Name         string  `json:"name"`
	Confidence   float64 `json:"confidence"`
	Source       string  `json:"source"`
	SourceDetail string  `json:"source_detail,omitempty"`
}

// Candidate representa um candidato a ProductName gerado por uma fonte do CRE.
type Candidate struct {
	Text           string
	Source         string
	Detail         string
	BaseConfidence float64
}

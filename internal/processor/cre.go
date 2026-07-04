package processor

// ExtractProductName executa o Candidate Ranking Engine determinístico do
// ProductName. É CPU-only e não faz resolução de URL nem chamadas externas.
func ExtractProductName(msg *NormalizedMessage) ProductNameResult {
	if msg == nil {
		return ProductNameResult{}
	}

	candidates := []Candidate{}
	candidates = append(candidates, webpageTitleCandidates(msg.WebpageTitle)...)
	candidates = append(candidates, textHeuristicCandidates(msg.Text)...)
	if len(candidates) == 0 {
		return ProductNameResult{}
	}

	best := candidates[0]
	bestScore := scoreProductNameCandidate(best, msg)
	for _, candidate := range candidates[1:] {
		score := scoreProductNameCandidate(candidate, msg)
		if score > bestScore {
			best = candidate
			bestScore = score
		}
	}

	result := ProductNameResult{
		Confidence:   bestScore,
		Source:       best.Source,
		SourceDetail: best.Detail,
	}
	if bestScore < productNameConfidenceThreshold {
		return result
	}
	result.Name = normalizeProductName(best.Text)
	if result.Name == "" {
		return ProductNameResult{
			Confidence:   bestScore,
			Source:       best.Source,
			SourceDetail: best.Detail,
		}
	}
	return result
}

func scoreProductNameCandidate(c Candidate, msg *NormalizedMessage) float64 {
	score := c.BaseConfidence
	score += scorePosition(c, msg)
	score += scoreCapitalization(c, msg)
	score += scoreTechDensity(c, msg)
	score += scoreBrandMatch(c, msg)
	score += scoreProximity(c, msg)
	score += scoreMetaPenalty(c, msg)
	return clampProductNameScore(score)
}

func clampProductNameScore(score float64) float64 {
	if score < 0 {
		return 0
	}
	if score > 1 {
		return 1
	}
	return score
}

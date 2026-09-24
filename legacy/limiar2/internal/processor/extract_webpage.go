package processor

import "github.com/limiar/collector/internal/model"

// extractWebpageInfo extrai metadados Media.Webpage sem depender de URL no texto.
func extractWebpageInfo(msg map[string]any) model.WebpageInfo {
	media, ok := msg["Media"].(map[string]any)
	if !ok {
		return model.WebpageInfo{}
	}
	wp, ok := media["Webpage"].(map[string]any)
	if !ok {
		return model.WebpageInfo{}
	}
	return model.WebpageInfo{
		URL:         model.JSONToString(wp["URL"]),
		Title:       model.JSONToString(wp["Title"]),
		Description: model.JSONToString(wp["Description"]),
	}
}

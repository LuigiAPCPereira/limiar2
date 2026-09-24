package processor

import (
	"html"
	"regexp"
	"strings"
)

const maxURLTitleLength = 200

var reHTMLTitle = regexp.MustCompile(`(?is)<title\b[^>]*>(.*?)</title>`)

// ExtractHTMLTitle extrai o conteúdo textual de <title> de um HTML parcial.
// Retorna string vazia quando não há title e limita o resultado para persistência.
func ExtractHTMLTitle(data []byte) string {
	match := reHTMLTitle.FindSubmatch(data)
	if len(match) < 2 {
		return ""
	}
	title := html.UnescapeString(string(match[1]))
	title = strings.Join(strings.Fields(title), " ")
	if len([]rune(title)) <= maxURLTitleLength {
		return title
	}
	runes := []rune(title)
	return string(runes[:maxURLTitleLength])
}

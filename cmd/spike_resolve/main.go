// Command spike_resolve é um script descartável para validar resolução de URLs
// shortened de merchants (Shopee, Amazon) via HTTP HEAD com follow redirects.
// Uso: go run ./cmd/spike_resolve [--db path] [--limit N] [--urls url1,url2,...]
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "turso.tech/database/tursogo"
)

var (
	reURL     = regexp.MustCompile(`https?://[^\s<>"'\)]+`)
	reASIN    = regexp.MustCompile(`/dp/([A-Z0-9]{10})`)
	reShopee  = regexp.MustCompile(`shopee\.com\.br`)
	reAmazon  = regexp.MustCompile(`amazon\.com\.br`)
	reKabum   = regexp.MustCompile(`kabum\.com\.br`)
	reMagalu  = regexp.MustCompile(`magazineluiza\.com\.br`)
	reML      = regexp.MustCompile(`mercadolivre\.com\.br`)
	reAliExp  = regexp.MustCompile(`aliexpress\.com`)

	trackingParams = []string{
		"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content",
		"fbclid", "gclid", "dclid", "msclkid",
		"tag", "linkId", "linkCode", "ref_", "camp", "creative",
		"affiliate_id", "sub_id", "mmp_pid",
		"aff_id", "aff_platform", "sk", "dp", "af", "cv", "cn", "btsid",
		"matt_tool", "matt_word", "matt_campaign",
	}
)

type result struct {
	original   string
	canonical  string
	merchant   string
	statusCode int
	hops       int
	duration   time.Duration
	err        error
}

func main() {
	dbPath := flag.String("db", "./limiar.db", "caminho do limiar.db")
	limit := flag.Int("limit", 100, "máximo de URLs para testar")
	urlsFlag := flag.String("urls", "", "URLs separadas por vírgula (override do DB)")
	workers := flag.Int("workers", 5, "concorrência de resolução")
	flag.Parse()

	var urls []string
	if *urlsFlag != "" {
		urls = strings.Split(*urlsFlag, ",")
	} else {
		urls = extractURLsFromDB(*dbPath, *limit)
	}

	if len(urls) == 0 {
		fmt.Println("❌ Nenhuma URL encontrada. Use --urls ou tenha um limiar.db com mensagens.")
		return
	}

	fmt.Printf("🔍 Testando %d URLs (%d workers)...\n\n", len(urls), *workers)

	results := resolveAll(urls, *workers)
	printReport(results)
}

func extractURLsFromDB(dbPath string, limit int) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	db, err := sql.Open("turso", dbPath)
	if err != nil {
		fmt.Printf("❌ Erro ao abrir DB: %v\n", err)
		return nil
	}
	defer db.Close()

	rows, err := db.QueryContext(ctx, `SELECT payload FROM raw_messages ORDER BY id DESC LIMIT 500`)
	if err != nil {
		fmt.Printf("❌ Erro na query: %v\n", err)
		return nil
	}
	defer rows.Close()

	seen := make(map[string]struct{})
	var urls []string

	for rows.Next() && len(urls) < limit {
		var payload string
		if err := rows.Scan(&payload); err != nil {
			continue
		}
		// Unescape JSON escape sequences so \n, \t become real characters
		// and don't get captured by the URL regex as part of the URL.
		unescaped := strings.ReplaceAll(payload, `\n`, "\n")
		unescaped = strings.ReplaceAll(unescaped, `\t`, "\t")
		unescaped = strings.ReplaceAll(unescaped, `\"`, `"`)
		for _, found := range reURL.FindAllString(unescaped, -1) {
			cleaned := cleanURL(found)
			if cleaned == "" {
				continue
			}
			if isShortened(cleaned) {
				if _, ok := seen[cleaned]; !ok {
					seen[cleaned] = struct{}{}
					urls = append(urls, cleaned)
					if len(urls) >= limit {
						break
					}
				}
			}
		}
	}

	// Se não achou shortened, pega URLs de merchant direto
	if len(urls) == 0 {
		for rows.Next() && len(urls) < limit {
			var payload string
			if err := rows.Scan(&payload); err != nil {
				continue
			}
			for _, found := range reURL.FindAllString(payload, -1) {
				cleaned := cleanURL(found)
				if cleaned == "" {
					continue
				}
				if isMerchantURL(cleaned) {
					if _, ok := seen[cleaned]; !ok {
						seen[cleaned] = struct{}{}
						urls = append(urls, cleaned)
						if len(urls) >= limit {
							break
						}
					}
				}
			}
		}
	}

	return urls
}

var shortenedDomains = []string{
	"s.shopee.com.br", "amzn.to", "amzn.divulgador.link", "amzlink.to",
	"tidd.ly", "meli.la", "s.click.aliexpress.com",
	"magazineluiza.onelink.me", "divulgador.magalu.com",
	"bit.ly",
}

func isShortened(u string) bool {
	parsed, err := url.Parse(u)
	if err != nil {
		return false
	}
	for _, d := range shortenedDomains {
		if parsed.Hostname() == d || strings.HasSuffix(parsed.Hostname(), "."+d) {
			return true
		}
	}
	return false
}

func isMerchantURL(u string) bool {
	return reShopee.MatchString(u) || reAmazon.MatchString(u) ||
		reKabum.MatchString(u) || reMagalu.MatchString(u) ||
		reML.MatchString(u) || reAliExp.MatchString(u)
}

func resolveAll(urls []string, workers int) []result {
	jobs := make(chan string, len(urls))
	results := make([]result, len(urls))
	var idx int64

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for u := range jobs {
				i := atomic.AddInt64(&idx, 1) - 1
				results[i] = resolve(u)
			}
		}()
	}

	for _, u := range urls {
		jobs <- u
	}
	close(jobs)
	wg.Wait()

	return results[:idx]
}

func resolve(rawURL string) result {
	start := time.Now()

	client := &http.Client{
		Timeout: 10 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("max redirects exceeded")
			}
			return nil
		},
	}

	req, err := http.NewRequest("HEAD", rawURL, nil)
	if err != nil {
		return result{original: rawURL, err: err, duration: time.Since(start)}
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")

	resp, err := client.Do(req)
	if err != nil {
		// HEAD pode ser bloqueado; tenta GET
		req2, _ := http.NewRequest("GET", rawURL, nil)
		req2.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/125.0.0.0 Safari/537.36")
		resp, err = client.Do(req2)
		if err != nil {
			return result{original: rawURL, err: err, duration: time.Since(start)}
		}
	}
	defer resp.Body.Close()

	finalURL := resp.Request.URL.String()
	cleaned := stripTracking(finalURL)
	merchant := identifyMerchant(cleaned)
	hops := countHops(rawURL, resp)

	return result{
		original:   rawURL,
		canonical:  cleaned,
		merchant:   merchant,
		statusCode: resp.StatusCode,
		hops:       hops,
		duration:   time.Since(start),
	}
}

func stripTracking(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := u.Query()
	changed := false
	for _, p := range trackingParams {
		if q.Has(p) {
			q.Del(p)
			changed = true
		}
	}
	if changed {
		u.RawQuery = q.Encode()
	}
	return u.String()
}

func identifyMerchant(u string) string {
	switch {
	case reShopee.MatchString(u):
		return "shopee"
	case reAmazon.MatchString(u):
		if m := reASIN.FindStringSubmatch(u); len(m) > 1 {
			return "amazon:" + m[1]
		}
		return "amazon"
	case reKabum.MatchString(u):
		return "kabum"
	case reMagalu.MatchString(u):
		return "magalu"
	case reML.MatchString(u):
		return "mercadolivre"
	case reAliExp.MatchString(u):
		return "aliexpress"
	default:
		host := ""
		if parsed, err := url.Parse(u); err == nil {
			host = parsed.Hostname()
		}
		return "other:" + host
	}
}

func countHops(original string, resp *http.Response) int {
	_ = original
	_ = resp
	// Count via resp.Request chain is not straightforward in Go.
	// We approximate by checking if the final URL differs from original.
	return 0 // Simplified for spike.
}

func cleanURL(u string) string {
	// Strip trailing punctuation and non-URL characters (emoji, whitespace)
	u = strings.TrimRight(u, ".,;:!?)\n\r\t ")
	// Remove any non-ASCII trailing characters (emoji etc.)
	for len(u) > 0 {
		last := u[len(u)-1]
		if last > 127 || last < 33 {
			u = u[:len(u)-1]
		} else {
			break
		}
	}
	// Validate it's still a parseable URL
	if _, err := url.Parse(u); err != nil || !strings.HasPrefix(u, "http") {
		return ""
	}
	return u
}

func printReport(results []result) {
	var (
		resolved   int
		failed     int
		byMerchant = make(map[string]int)
		byStatus   = make(map[int]int)
		totalDur   time.Duration
	)

	fmt.Println("─── Resultados ───────────────────────────────────────────────────")
	for i, r := range results {
		if r.err != nil {
			failed++
			fmt.Printf("%3d ❌ %s\n     erro: %v\n", i+1, r.original, r.err)
			continue
		}
		resolved++
		totalDur += r.duration
		byMerchant[r.merchant]++
		byStatus[r.statusCode]++

		status := "✅"
		if r.statusCode >= 400 {
			status = "⚠️"
		}

		fmt.Printf("%3d %s %s\n", i+1, status, r.merchant)
		fmt.Printf("     orig:  %s\n", truncate(r.original, 90))
		if r.canonical != r.original {
			fmt.Printf("     canon: %s\n", truncate(r.canonical, 90))
		}
		fmt.Printf("     http=%d  dur=%s\n", r.statusCode, r.duration.Round(time.Millisecond))
		fmt.Println()
	}

	fmt.Println("─── Resumo ───────────────────────────────────────────────────────")
	fmt.Printf("Total: %d | Resolvidas: %d | Falhas: %d\n", len(results), resolved, failed)
	if resolved > 0 {
		fmt.Printf("Tempo médio: %s\n", (totalDur / time.Duration(resolved)).Round(time.Millisecond))
	}
	fmt.Println("\nPor merchant:")
	for m, c := range byMerchant {
		fmt.Printf("  %-20s %d\n", m, c)
	}
	fmt.Println("\nPor status code:")
	for s, c := range byStatus {
		fmt.Printf("  %d: %d\n", s, c)
	}
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

// Suppress unused import warnings for json (used in future extensions).
var _ = json.Marshal

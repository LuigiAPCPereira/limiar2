package processor

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/model"
)

func TestCanonicalizeURL(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{
			name: "amazon removes affiliate and tracking params",
			raw:  "HTTPS://WWW.AMAZON.COM.BR/Produto?tag=afiliado-20&utm_source=telegram&color=azul&b=2&a=1#cupom",
			want: "https://www.amazon.com.br/Produto?a=1&b=2&color=azul",
		},
		{
			name: "shopee removes app tracking and preserves useful variation",
			raw:  "https://s.shopee.com.br/item?spm=a&algo_pvid=1&variation=blue",
			want: "https://s.shopee.com.br/item?variation=blue",
		},
		{
			name: "shopee removes uls tracking from resolved campaign URLs",
			raw:  "https://shopee.com.br/m/7-7?uls_trackid=51abc&item=123",
			want: "https://shopee.com.br/m/7-7?item=123",
		},
		{
			name: "mercado livre removes click ids and preserves quantity",
			raw:  "https://produto.mercadolivre.com.br/item?fbclid=abc&quantity=1",
			want: "https://produto.mercadolivre.com.br/item?quantity=1",
		},
		{
			name: "magalu removes campaign params leaving clean URL",
			raw:  "https://www.magazineluiza.com.br/produto/p?gclid=abc&utm_campaign=promo",
			want: "https://www.magazineluiza.com.br/produto/p",
		},
		{
			name: "aliexpress removes affiliate params and preserves sku",
			raw:  "https://pt.aliexpress.com/item/1.html?aff_id=abc&mmp_sub1=x&sku_id=120000&camp=foo",
			want: "https://pt.aliexpress.com/item/1.html?sku_id=120000",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CanonicalizeURL(tt.raw)
			if err != nil {
				t.Fatalf("CanonicalizeURL(%q) error = %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("CanonicalizeURL(%q) = %q, want %q", tt.raw, got, tt.want)
			}
		})
	}
}

func TestCanonicalizeURL_InvalidURL(t *testing.T) {
	if _, err := CanonicalizeURL(":// sem scheme"); err == nil {
		t.Fatal("CanonicalizeURL invalid URL returned nil error")
	}
}

func TestExtractHTMLTitle(t *testing.T) {
	longTitle := strings.Repeat("A", 260)
	tests := []struct {
		name string
		html string
		want string
	}{
		{name: "plain title", html: `<html><head><title>Produto X - Loja</title></head></html>`, want: "Produto X - Loja"},
		{name: "collapses whitespace", html: "<title> Produto\n\t X    -   Loja </title>", want: "Produto X - Loja"},
		{name: "unescapes entities", html: `<title>Fone &amp; Caixa &lt;Oferta&gt;</title>`, want: "Fone & Caixa <Oferta>"},
		{name: "missing title", html: `<html><body>sem title</body></html>`, want: ""},
		{name: "truncates long title", html: `<title>` + longTitle + `</title>`, want: strings.Repeat("A", 200)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ExtractHTMLTitle([]byte(tt.html))
			if got != tt.want {
				t.Fatalf("ExtractHTMLTitle() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestURLResolver_ResolvesRedirectAndCachesTitle(t *testing.T) {
	store := newFakeURLResolutionStore()
	var finalHits int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/start":
			http.Redirect(w, r, "/final?utm_source=telegram&sku=123", http.StatusFound)
		case "/final":
			finalHits++
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write([]byte(`<html><head><title>Produto Resolvido &amp; Loja</title></head><body>ok</body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	resolver := NewURLResolver(store, server.Client(), logger.NopLogger{})
	ctx := context.Background()
	original := server.URL + "/start?tag=afiliado"

	first, err := resolver.Resolve(ctx, original)
	if err != nil {
		t.Fatalf("Resolve first: %v", err)
	}
	if first.Unresolved {
		t.Fatalf("Resolve first unresolved: %#v", first)
	}
	wantCanonical := server.URL + "/final?sku=123"
	if first.CanonicalURL != wantCanonical {
		t.Fatalf("CanonicalURL = %q, want %q", first.CanonicalURL, wantCanonical)
	}
	if first.Title != "Produto Resolvido & Loja" {
		t.Fatalf("Title = %q", first.Title)
	}
	if finalHits != 1 {
		t.Fatalf("finalHits after first resolve = %d, want 1", finalHits)
	}

	second, err := resolver.Resolve(ctx, original)
	if err != nil {
		t.Fatalf("Resolve cache hit: %v", err)
	}
	if second.CanonicalURL != first.CanonicalURL || second.Title != first.Title {
		t.Fatalf("cache hit = %#v, want %#v", second, first)
	}
	if finalHits != 1 {
		t.Fatalf("cache hit performed HTTP request; finalHits = %d, want 1", finalHits)
	}
}

func TestURLResolver_CacheHitCanonicalizesStaleCanonicalURL(t *testing.T) {
	store := newFakeURLResolutionStore()
	original := "https://s.shopee.com.br/1qaAJDNDFK"
	store.items[original] = model.URLResolution{
		OriginalURL:  original,
		CanonicalURL: "https://shopee.com.br/m/7-7?uls_trackid=51abc&item=123",
		Merchant:     "shopee",
		Title:        "",
		Unresolved:   false,
	}

	resolver := NewURLResolver(store, nil, logger.NopLogger{})
	got, err := resolver.Resolve(context.Background(), original)
	if err != nil {
		t.Fatalf("Resolve cache hit: %v", err)
	}
	const want = "https://shopee.com.br/m/7-7?item=123"
	if got.CanonicalURL != want {
		t.Fatalf("CanonicalURL = %q, want %q", got.CanonicalURL, want)
	}
	if store.items[original].CanonicalURL != want {
		t.Fatalf("stored CanonicalURL = %q, want refreshed %q", store.items[original].CanonicalURL, want)
	}
}

func TestURLResolver_ForbiddenFallsBackAsUnresolved(t *testing.T) {
	store := newFakeURLResolutionStore()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "blocked", http.StatusForbidden)
	}))
	defer server.Close()

	resolver := NewURLResolver(store, server.Client(), logger.NopLogger{})
	got, err := resolver.Resolve(context.Background(), server.URL+"/blocked?utm_source=x")
	if err != nil {
		t.Fatalf("Resolve forbidden returned error: %v", err)
	}
	if !got.Unresolved {
		t.Fatalf("Unresolved = false, want true: %#v", got)
	}
	if got.CanonicalURL != server.URL+"/blocked" {
		t.Fatalf("CanonicalURL = %q, want original without tracking", got.CanonicalURL)
	}
	if got.Title != "" {
		t.Fatalf("Title = %q, want empty", got.Title)
	}
}

func TestURLResolver_FreshUnresolvedCacheHitDoesNotRetry(t *testing.T) {
	store := newFakeURLResolutionStore()
	serverHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHits++
		_, _ = w.Write([]byte(`<html><head><title>Produto</title></head></html>`))
	}))
	defer server.Close()

	original := server.URL + "/temporario"
	store.items[original] = model.URLResolution{
		OriginalURL:  original,
		CanonicalURL: original,
		ResolvedAt:   time.Now().UTC(),
		Unresolved:   true,
	}

	resolver := NewURLResolver(store, server.Client(), logger.NopLogger{})
	got, err := resolver.Resolve(context.Background(), original)
	if err != nil {
		t.Fatalf("Resolve fresh unresolved cache hit: %v", err)
	}
	if !got.Unresolved {
		t.Fatalf("Unresolved = false, want cached unresolved: %#v", got)
	}
	if serverHits != 0 {
		t.Fatalf("serverHits = %d, want 0 for fresh unresolved cache", serverHits)
	}
}

func TestURLResolver_StaleUnresolvedCacheHitRetries(t *testing.T) {
	store := newFakeURLResolutionStore()
	serverHits := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		serverHits++
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><title>Produto Retentado</title></head><body>ok</body></html>`))
	}))
	defer server.Close()

	original := server.URL + "/temporario?utm_source=telegram"
	store.items[original] = model.URLResolution{
		OriginalURL:  original,
		CanonicalURL: server.URL + "/temporario",
		ResolvedAt:   time.Now().Add(-48 * time.Hour).UTC(),
		Unresolved:   true,
	}

	resolver := NewURLResolver(store, server.Client(), logger.NopLogger{})
	got, err := resolver.Resolve(context.Background(), original)
	if err != nil {
		t.Fatalf("Resolve stale unresolved cache hit: %v", err)
	}
	if got.Unresolved {
		t.Fatalf("Unresolved = true, want retried resolved result: %#v", got)
	}
	if got.Title != "Produto Retentado" {
		t.Fatalf("Title = %q, want retry result title", got.Title)
	}
	if serverHits != 1 {
		t.Fatalf("serverHits = %d, want 1 retry", serverHits)
	}
}

func TestURLResolver_TimeoutFallsBackAsUnresolved(t *testing.T) {
	store := newFakeURLResolutionStore()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
	}))
	defer server.Close()

	client := server.Client()
	client.Timeout = 10 * time.Millisecond
	resolver := NewURLResolver(store, client, logger.NopLogger{})
	got, err := resolver.Resolve(context.Background(), server.URL+"/slow")
	if err != nil {
		t.Fatalf("Resolve timeout returned error: %v", err)
	}
	if !got.Unresolved {
		t.Fatalf("Unresolved = false, want true: %#v", got)
	}
}

type fakeURLResolutionStore struct {
	items map[string]model.URLResolution
	err   error
}

func newFakeURLResolutionStore() *fakeURLResolutionStore {
	return &fakeURLResolutionStore{items: make(map[string]model.URLResolution)}
}

func (f *fakeURLResolutionStore) GetURLResolution(_ context.Context, originalURL string) (model.URLResolution, bool, error) {
	if f.err != nil {
		return model.URLResolution{}, false, f.err
	}
	r, ok := f.items[originalURL]
	return r, ok, nil
}

func (f *fakeURLResolutionStore) SaveURLResolution(_ context.Context, r model.URLResolution) error {
	if f.err != nil {
		return f.err
	}
	if r.OriginalURL == "" {
		return errors.New("missing original url")
	}
	f.items[r.OriginalURL] = r
	return nil
}

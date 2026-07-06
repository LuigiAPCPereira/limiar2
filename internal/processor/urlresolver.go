package processor

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/model"
)

const (
	urlResolverTimeout              = 5 * time.Second
	urlResolverMaxRedirects         = 10
	urlResolverReadLimit            = 256 * 1024
	urlResolverUnresolvedRetryAfter = 24 * time.Hour
	urlResolverUserAgent            = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126 Safari/537.36"
)

type URLResolutionStore interface {
	GetURLResolution(ctx context.Context, originalURL string) (model.URLResolution, bool, error)
	SaveURLResolution(ctx context.Context, r model.URLResolution) error
}

// URLResolver resolve URLs com cache persistente antes de fazer rede.
// Erros HTTP/rede viram resolução unresolved para não bloquear o processor.
type URLResolver struct {
	store  URLResolutionStore
	client *http.Client
	log    logger.Logger
}

func NewURLResolver(store URLResolutionStore, client *http.Client, log logger.Logger) *URLResolver {
	if client == nil {
		client = &http.Client{Timeout: urlResolverTimeout}
	} else {
		copyClient := *client
		client = &copyClient
		if client.Timeout == 0 {
			client.Timeout = urlResolverTimeout
		}
	}
	if client.CheckRedirect == nil {
		client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			if len(via) >= urlResolverMaxRedirects {
				return fmt.Errorf("processor: url_resolver: redirects excederam %d", urlResolverMaxRedirects)
			}
			return nil
		}
	}
	if log == nil {
		log = logger.NopLogger{}
	}
	return &URLResolver{store: store, client: client, log: log}
}

func (r *URLResolver) Resolve(ctx context.Context, originalURL string) (model.URLResolution, error) {
	originalURL = strings.TrimSpace(originalURL)
	if originalURL == "" {
		return model.URLResolution{}, nil
	}
	if r.store != nil {
		cached, ok, err := r.store.GetURLResolution(ctx, originalURL)
		if err != nil {
			return model.URLResolution{}, err
		}
		if ok && !shouldRetryUnresolved(cached) {
			return r.refreshCachedCanonical(ctx, cached)
		}
	}

	result := unresolvedURLResolution(originalURL)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, originalURL, nil)
	if err != nil {
		return r.save(ctx, result)
	}
	req.Header.Set("User-Agent", urlResolverUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := r.client.Do(req)
	if err != nil {
		r.log.Warn("⚠️ Resolução de URL falhou", "url", originalURL, "erro", err)
		return r.save(ctx, result)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode >= http.StatusBadRequest {
		return r.save(ctx, result)
	}

	finalURL := originalURL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	canonical, err := CanonicalizeURL(finalURL)
	if err != nil {
		canonical = result.CanonicalURL
	}
	limited := io.LimitReader(resp.Body, urlResolverReadLimit)
	body, readErr := io.ReadAll(limited)
	if readErr != nil {
		r.log.Warn("⚠️ Leitura da URL resolvida falhou", "url", originalURL, "erro", readErr)
		return r.save(ctx, result)
	}

	result.CanonicalURL = canonical
	result.Merchant = detectMerchant(canonical)
	result.Title = ExtractHTMLTitle(body)
	result.Unresolved = false
	return r.save(ctx, result)
}

func (r *URLResolver) refreshCachedCanonical(ctx context.Context, cached model.URLResolution) (model.URLResolution, error) {
	if cached.CanonicalURL == "" {
		return cached, nil
	}
	canonical, err := CanonicalizeURL(cached.CanonicalURL)
	if err != nil || canonical == cached.CanonicalURL {
		return cached, nil
	}
	cached.CanonicalURL = canonical
	if cached.Merchant == "" {
		cached.Merchant = detectMerchant(canonical)
	}
	return r.save(ctx, cached)
}

func shouldRetryUnresolved(cached model.URLResolution) bool {
	if !cached.Unresolved {
		return false
	}
	if cached.ResolvedAt.IsZero() {
		return true
	}
	return time.Since(cached.ResolvedAt) >= urlResolverUnresolvedRetryAfter
}

func (r *URLResolver) save(ctx context.Context, result model.URLResolution) (model.URLResolution, error) {
	if r.store == nil || result.OriginalURL == "" {
		return result, nil
	}
	if err := r.store.SaveURLResolution(ctx, result); err != nil {
		return model.URLResolution{}, err
	}
	return result, nil
}

func unresolvedURLResolution(originalURL string) model.URLResolution {
	canonical, err := CanonicalizeURL(originalURL)
	if err != nil {
		canonical = strings.TrimSpace(originalURL)
	}
	return model.URLResolution{
		OriginalURL:  originalURL,
		CanonicalURL: canonical,
		Merchant:     detectMerchant(canonical),
		ResolvedAt:   time.Now().UTC(),
		Unresolved:   true,
	}
}

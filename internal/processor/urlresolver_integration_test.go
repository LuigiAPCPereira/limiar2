package processor

import (
	"context"
	"testing"
	"time"

	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/model"
)

func TestProcessorProcessBatchResolvesURLsWhenEnabled(t *testing.T) {
	repo := &urlResolverProcessorStore{
		raw: []*model.RawMessage{makeURLResolverRawMessage(t, 1, "Produto R$ 99 https://loja.example/oferta?utm_source=telegram")},
		resolution: model.URLResolution{
			OriginalURL:  "https://loja.example/oferta?utm_source=telegram",
			CanonicalURL: "https://loja.example/oferta",
			Merchant:     "loja",
			Title:        "Produto Resolvido",
			ResolvedAt:   time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC),
			Unresolved:   false,
		},
	}
	proc := NewProcessor(repo, &Config{BatchSize: 10, PollInterval: time.Second, ResolveURLs: true}, logger.NopLogger{})

	proc.processBatch(context.Background())

	if len(repo.saved) != 1 {
		t.Fatalf("saved len = %d, want 1", len(repo.saved))
	}
	saved := repo.saved[0]
	if saved.CanonicalURL != "https://loja.example/oferta" {
		t.Fatalf("CanonicalURL = %q", saved.CanonicalURL)
	}
	if saved.URLTitle != "Produto Resolvido" {
		t.Fatalf("URLTitle = %q", saved.URLTitle)
	}
	if !saved.URLResolved {
		t.Fatal("URLResolved = false, want true")
	}
}

func TestProcessorProcessBatchDoesNotResolveURLsByDefault(t *testing.T) {
	repo := &urlResolverProcessorStore{
		raw: []*model.RawMessage{makeURLResolverRawMessage(t, 1, "Produto R$ 99 https://loja.example/oferta?utm_source=telegram")},
		resolution: model.URLResolution{
			OriginalURL:  "https://loja.example/oferta?utm_source=telegram",
			CanonicalURL: "https://loja.example/oferta",
			Title:        "Produto Resolvido",
		},
	}
	proc := NewProcessor(repo, &Config{BatchSize: 10, PollInterval: time.Second}, logger.NopLogger{})

	proc.processBatch(context.Background())

	if repo.resolveCalls != 0 {
		t.Fatalf("ResolveURL calls = %d, want 0", repo.resolveCalls)
	}
	if len(repo.saved) != 1 {
		t.Fatalf("saved len = %d, want 1", len(repo.saved))
	}
	if repo.saved[0].CanonicalURL != "" || repo.saved[0].URLTitle != "" || repo.saved[0].URLResolved {
		t.Fatalf("URL fields unexpectedly set: %#v", repo.saved[0])
	}
}

func TestProcessorProcessBatchResolveLimitPersistsAcrossBatches(t *testing.T) {
	firstURL := "https://loja.example/primeira"
	secondURL := "https://loja.example/segunda"
	repo := &urlResolverProcessorStore{
		rawBatches: [][]*model.RawMessage{
			{makeURLResolverRawMessage(t, 1, "Primeira oferta R$ 10 "+firstURL)},
			{makeURLResolverRawMessage(t, 2, "Segunda oferta R$ 20 "+secondURL)},
		},
		resolutions: map[string]model.URLResolution{
			firstURL: {
				OriginalURL:  firstURL,
				CanonicalURL: firstURL,
				Merchant:     "loja",
				Title:        "Primeira",
				ResolvedAt:   time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC),
			},
			secondURL: {
				OriginalURL:  secondURL,
				CanonicalURL: secondURL,
				Merchant:     "loja",
				Title:        "Segunda",
				ResolvedAt:   time.Date(2026, 7, 5, 10, 0, 0, 0, time.UTC),
			},
		},
	}
	proc := NewProcessor(repo, &Config{BatchSize: 1, PollInterval: time.Second, ResolveURLs: true, ResolveURLsLimit: 1}, logger.NopLogger{})

	proc.processBatch(context.Background())
	proc.processBatch(context.Background())

	if repo.resolveCalls != 1 {
		t.Fatalf("ResolveURL calls = %d, want 1 across processor run", repo.resolveCalls)
	}
	if len(repo.saved) != 2 {
		t.Fatalf("saved len = %d, want 2", len(repo.saved))
	}
	if repo.saved[0].CanonicalURL == "" {
		t.Fatal("first batch was not URL-enriched")
	}
	if repo.saved[1].CanonicalURL != "" || repo.saved[1].URLResolved {
		t.Fatalf("second batch exceeded global URL limit: %#v", repo.saved[1])
	}
}

func TestNewProcessorCreatesReusableURLResolverWhenEnabled(t *testing.T) {
	repo := &urlResolverProcessorStore{}
	proc := NewProcessor(repo, &Config{BatchSize: 1, PollInterval: time.Second, ResolveURLs: true}, logger.NopLogger{})

	if proc.urlResolver == nil {
		t.Fatal("urlResolver = nil, want reusable resolver when ResolveURLs is enabled")
	}
}

func makeURLResolverRawMessage(t *testing.T, id int64, text string) *model.RawMessage {
	t.Helper()
	payload := `{"ID":101,"Message":` + quoteJSONString(text) + `,"PeerID":{"ChannelID":1},"Date":1717891200}`
	return &model.RawMessage{ID: id, ChannelID: 1, MessageID: 101, Payload: []byte(payload), ReceivedAt: time.Now(), SchemaVersion: 1}
}

func quoteJSONString(s string) string {
	b := make([]byte, 0, len(s)+2)
	b = append(b, '"')
	for _, r := range s {
		switch r {
		case '\\', '"':
			b = append(b, '\\', byte(r))
		case '\n':
			b = append(b, '\\', 'n')
		default:
			b = append(b, string(r)...)
		}
	}
	b = append(b, '"')
	return string(b)
}

type urlResolverProcessorStore struct {
	raw          []*model.RawMessage
	rawBatches   [][]*model.RawMessage
	fetchCalls   int
	resolution   model.URLResolution
	resolutions  map[string]model.URLResolution
	saved        []*NormalizedMessage
	resolveCalls int
}

func (s *urlResolverProcessorStore) FetchUnprocessed(context.Context, int) ([]*model.RawMessage, error) {
	if s.rawBatches != nil {
		if s.fetchCalls >= len(s.rawBatches) {
			return nil, nil
		}
		batch := s.rawBatches[s.fetchCalls]
		s.fetchCalls++
		return batch, nil
	}
	return s.raw, nil
}
func (s *urlResolverProcessorStore) CountUnprocessed(context.Context) (int64, error) { return 0, nil }
func (s *urlResolverProcessorStore) SaveProcessed(context.Context, *NormalizedMessage) error {
	return nil
}
func (s *urlResolverProcessorStore) SaveProcessedBatch(_ context.Context, msgs []*NormalizedMessage) (int, int, error) {
	s.saved = append(s.saved, msgs...)
	return len(msgs), 0, nil
}
func (s *urlResolverProcessorStore) CrossChannelDuplicates(context.Context, map[string]int64) (map[string]bool, error) {
	return map[string]bool{}, nil
}
func (s *urlResolverProcessorStore) CleanExpiredPhotoCache(context.Context) (int64, error) {
	return 0, nil
}
func (s *urlResolverProcessorStore) GetURLResolution(_ context.Context, originalURL string) (model.URLResolution, bool, error) {
	s.resolveCalls++
	if s.resolutions != nil {
		r, ok := s.resolutions[originalURL]
		return r, ok, nil
	}
	if originalURL == s.resolution.OriginalURL {
		return s.resolution, true, nil
	}
	return model.URLResolution{}, false, nil
}
func (s *urlResolverProcessorStore) SaveURLResolution(context.Context, model.URLResolution) error {
	return nil
}

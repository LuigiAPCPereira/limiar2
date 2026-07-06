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
	resolution   model.URLResolution
	saved        []*NormalizedMessage
	resolveCalls int
}

func (s *urlResolverProcessorStore) FetchUnprocessed(context.Context, int) ([]*model.RawMessage, error) {
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
	if originalURL == s.resolution.OriginalURL {
		return s.resolution, true, nil
	}
	return model.URLResolution{}, false, nil
}
func (s *urlResolverProcessorStore) SaveURLResolution(context.Context, model.URLResolution) error {
	return nil
}

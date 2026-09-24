package processor

import (
	"context"
	"testing"
	"time"

	"github.com/limiar/collector/internal/model"
)

func TestProcessorRepository_URLResolutionRoundTrip(t *testing.T) {
	repo, _, cleanup, err := openTempProcessorRepo()
	if err != nil {
		t.Fatalf("openTempProcessorRepo: %v", err)
	}
	defer cleanup()
	ctx := context.Background()

	resolvedAt := time.Date(2026, 7, 5, 12, 30, 0, 0, time.UTC)
	want := model.URLResolution{
		OriginalURL:  "https://www.amazon.com.br/produto?tag=afiliado-20&utm_source=telegram",
		CanonicalURL: "https://www.amazon.com.br/produto",
		Merchant:     "amazon",
		Title:        "Produto Amazon",
		ResolvedAt:   resolvedAt,
		Unresolved:   false,
	}
	if err := repo.SaveURLResolution(ctx, want); err != nil {
		t.Fatalf("SaveURLResolution: %v", err)
	}

	got, ok, err := repo.GetURLResolution(ctx, want.OriginalURL)
	if err != nil {
		t.Fatalf("GetURLResolution: %v", err)
	}
	if !ok {
		t.Fatalf("GetURLResolution ok = false, want true")
	}
	if got.OriginalURL != want.OriginalURL || got.CanonicalURL != want.CanonicalURL || got.Merchant != want.Merchant || got.Title != want.Title || got.Unresolved != want.Unresolved {
		t.Fatalf("URLResolution = %#v, want %#v", got, want)
	}
	if !got.ResolvedAt.Equal(resolvedAt) {
		t.Fatalf("ResolvedAt = %s, want %s", got.ResolvedAt, resolvedAt)
	}
}

func TestSaveProcessed_RoundTripsURLResolverFields(t *testing.T) {
	repo, db, cleanup, err := openTempProcessorRepo()
	if err != nil {
		t.Fatalf("openTempProcessorRepo: %v", err)
	}
	defer cleanup()
	ctx := context.Background()

	mustExec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.DB().ExecContext(ctx, q, args...); err != nil {
			t.Fatalf("seed exec %q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO channels (id, username, title, active) VALUES (1, 'c1', 'C', 1)`)
	mustExec(`INSERT INTO raw_messages (id, channel_id, message_id, payload) VALUES (1, 1, 101, '{}')`)

	postedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	msg := &NormalizedMessage{
		RawMessageID: 1,
		ChannelID:    1,
		MessageID:    101,
		MessageType:  "deal_complete",
		Text:         "Produto R$ 99 https://amzn.to/oferta",
		PostedAt:     postedAt,
		ProcessedAt:  postedAt,
		HasURL:       true,
		CanonicalURL: "https://www.amazon.com.br/produto",
		URLTitle:     "Produto Resolvido - Amazon",
		URLResolved:  true,
	}
	if err := repo.SaveProcessed(ctx, msg); err != nil {
		t.Fatalf("SaveProcessed: %v", err)
	}

	got, err := repo.ListProcessedMessages(ctx, 0, "", 10, 0)
	if err != nil {
		t.Fatalf("ListProcessedMessages: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("len(got) = %d, want 1", len(got))
	}
	pm := got[0]
	if pm.CanonicalURL != msg.CanonicalURL {
		t.Fatalf("CanonicalURL = %q, want %q", pm.CanonicalURL, msg.CanonicalURL)
	}
	if pm.URLTitle != msg.URLTitle {
		t.Fatalf("URLTitle = %q, want %q", pm.URLTitle, msg.URLTitle)
	}
	if !pm.URLResolved {
		t.Fatal("URLResolved = false, want true")
	}
}

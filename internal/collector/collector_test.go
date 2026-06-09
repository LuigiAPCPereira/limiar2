package collector_test

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/limiar/collector/internal/collector"
	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

// newRepo opens a real Tursogo-backed repository in a temp dir for handler
// tests that need a live channels table.
func newRepo(t *testing.T) *storage.Repository {
	t.Helper()
	ctx := context.Background()
	dir, err := os.MkdirTemp("", "limiar-collector-")
	if err != nil {
		t.Fatalf("mkdtemp: %v", err)
	}
	db, err := storage.Open(ctx, filepath.Join(dir, "t.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	repo, err := storage.NewRepository(db.Conn())
	if err != nil {
		t.Fatalf("repo: %v", err)
	}
	t.Cleanup(func() {
		_ = repo.Close()
		_ = db.Close()
		_ = os.RemoveAll(dir)
	})
	return repo
}

// --- fakes for collector orchestration tests ---

type fakeRepo struct {
	mu       sync.Mutex
	channels []*storage.Channel
	saved    []*storage.RawMessage
	cursors  map[int64]int64
}

func newFakeRepo(channels ...*storage.Channel) *fakeRepo {
	return &fakeRepo{channels: channels, cursors: make(map[int64]int64)}
}

func (f *fakeRepo) ListChannels(_ context.Context) ([]*storage.Channel, error) {
	return f.channels, nil
}

func (f *fakeRepo) SaveRawMessage(_ context.Context, msg *storage.RawMessage) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.saved = append(f.saved, msg)
	return true, nil
}

func (f *fakeRepo) UpdateChannelLastMessage(_ context.Context, channelID, messageID int64, _ time.Time) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cursors[channelID] = messageID
	return nil
}

func (f *fakeRepo) savedCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.saved)
}

// fakeClient implements telegram.TelegramClient. Run blocks until ctx is
// cancelled, simulating the live capture loop.
type fakeClient struct {
	history    []telegram.HistoryMessage
	runStarted chan struct{}
}

func (c *fakeClient) Auth(context.Context) error                    { return nil }
func (c *fakeClient) IsAuthenticated(context.Context) (bool, error) { return true, nil }
func (c *fakeClient) LoadPeers(context.Context) error               { return nil }
func (c *fakeClient) AddUpdateHandler(telegram.UpdateHandler)       {}

func (c *fakeClient) ResolveChannel(context.Context, string) (*storage.Peer, error) {
	return &storage.Peer{ID: 1, Type: "channel"}, nil
}

func (c *fakeClient) ResolveChannelChecked(context.Context, string) (*storage.Peer, error) {
	return &storage.Peer{ID: 1, Type: "channel"}, nil
}

func (c *fakeClient) FetchHistory(_ context.Context, _ int64, minID int64, limit int) ([]telegram.HistoryMessage, error) {
	var out []telegram.HistoryMessage
	for _, m := range c.history {
		if m.MessageID > minID {
			out = append(out, m)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

// FetchHistoryWithOffset returns up to limit messages with id < offsetID
// (newest-first when offsetID == 0), sorted descending to mirror Telegram's
// messages.getHistory ordering.
func (c *fakeClient) FetchHistoryWithOffset(_ context.Context, _ int64, offsetID int64, limit int) ([]telegram.HistoryMessage, error) {
	var out []telegram.HistoryMessage
	for _, m := range c.history {
		if offsetID == 0 || m.MessageID < offsetID {
			out = append(out, m)
		}
	}
	// Sort descending (newest first) like the real API.
	sort.Slice(out, func(i, j int) bool { return out[i].MessageID > out[j].MessageID })
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func (c *fakeClient) Run(ctx context.Context) error {
	if c.runStarted != nil {
		close(c.runStarted)
	}
	<-ctx.Done()
	return nil
}

// Feature: limiar-collector, Property 16: Shutdown Within Timeout.
// When the root context is cancelled, Collector.Run returns promptly.
func TestProperty16ShutdownWithinTimeout(t *testing.T) {
	repo := newFakeRepo()
	started := make(chan struct{})
	client := &fakeClient{runStarted: started}
	col := collector.NewCollector(client, repo, collector.NoopClassifier{}, nil, 256, 3, 5000, 30)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- col.Run(ctx) }()

	<-started // ensure live capture is running
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error on cancel: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not shut down within timeout after cancel")
	}
}

// First run (last_message_id == 0) backfills history before live capture.
func TestFirstRunBackfillsHistory(t *testing.T) {
	ch := &storage.Channel{ID: 1, Username: "promos", Active: true, LastMessageID: 0}
	repo := newFakeRepo(ch)
	history := make([]telegram.HistoryMessage, 20)
	for i := range history {
		history[i] = telegram.HistoryMessage{MessageID: int64(i + 1), Payload: []byte(`{}`)}
	}
	started := make(chan struct{})
	client := &fakeClient{history: history, runStarted: started}
	col := collector.NewCollector(client, repo, collector.NoopClassifier{}, nil, 256, 3, 5000, 30)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- col.Run(ctx) }()
	<-started
	cancel()
	<-done

	if repo.savedCount() != 20 {
		t.Fatalf("expected 20 backfilled messages, got %d", repo.savedCount())
	}
	if repo.cursors[1] != 20 {
		t.Fatalf("expected cursor advanced to 20, got %d", repo.cursors[1])
	}
}

// Resume mode (last_message_id > 0) backfills only messages newer than the cursor.
func TestResumeBackfillFetchesOnlyNewMessages(t *testing.T) {
	ch := &storage.Channel{ID: 1, Username: "promos", Active: true, LastMessageID: 99}
	repo := newFakeRepo(ch)
	// History contains one old message (id=1) and one new message (id=101).
	history := []telegram.HistoryMessage{
		{MessageID: 1, Payload: []byte(`{}`)},
		{MessageID: 101, Payload: []byte(`{}`)},
	}
	started := make(chan struct{})
	client := &fakeClient{history: history, runStarted: started}
	col := collector.NewCollector(client, repo, collector.NoopClassifier{}, nil, 256, 3, 5000, 30)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- col.Run(ctx) }()
	<-started
	cancel()
	<-done

	if repo.savedCount() != 1 {
		t.Fatalf("resume backfill should fetch only new messages, got %d", repo.savedCount())
	}
	if repo.cursors[1] != 101 {
		t.Fatalf("expected cursor advanced to 101, got %d", repo.cursors[1])
	}
}

// Regression: backfill must respect the temporal cutoff (LIMIAR_HISTORY_MAX_DAYS).
// Messages older than now-historyMaxDays are skipped, and pagination stops when
// the page straddles the cutoff. The cursor advances to the newest seen id.
func TestBackfillRespectsTemporalCutoff(t *testing.T) {
	ch := &storage.Channel{ID: 1, Username: "promos", Active: true, LastMessageID: 0}
	repo := newFakeRepo(ch)
	now := time.Now()
	history := []telegram.HistoryMessage{
		{MessageID: 10, Date: now.Add(-24 * time.Hour), Payload: []byte(`{}`)},    // 1 day old — keep
		{MessageID: 20, Date: now.Add(-3 * 24 * time.Hour), Payload: []byte(`{}`)}, // 3 days old — keep
		{MessageID: 30, Date: now.Add(-15 * 24 * time.Hour), Payload: []byte(`{}`)}, // 15 days old — skip
		{MessageID: 40, Date: now.Add(-60 * 24 * time.Hour), Payload: []byte(`{}`)}, // 60 days old — skip
	}
	started := make(chan struct{})
	client := &fakeClient{history: history, runStarted: started}
	// historyMaxDays=7 → only messages <= 7 days old are persisted.
	col := collector.NewCollector(client, repo, collector.NoopClassifier{}, nil, 256, 3, 5000, 7)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- col.Run(ctx) }()
	<-started
	cancel()
	<-done

	if repo.savedCount() != 2 {
		t.Fatalf("expected 2 messages within the 7-day window, got %d", repo.savedCount())
	}
	// Cursor tracks the scan frontier (newest seen, id=40), not the newest
	// persisted. This is intentional: messages 30 and 40 were filtered by the
	// temporal cutoff but there is no point re-walking them on the next run.
	if repo.cursors[1] != 40 {
		t.Fatalf("expected cursor advanced to 40 (newest seen), got %d", repo.cursors[1])
	}
}

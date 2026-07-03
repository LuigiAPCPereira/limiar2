package dashboard

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/limiar/collector/internal/media"
	"github.com/limiar/collector/internal/model"
	"github.com/limiar/collector/internal/processor"
)

// --- Mock implementations ---

type mockRepo struct {
	channels       []*model.Channel
	channelsErr    error
	messages       []*model.RawMessage
	messagesErr    error
	messageByID    *model.RawMessage
	messageByIDErr error
	stats          []model.ChannelStats
	statsErr       error
	countRaw       int64
	countRawErr    error
}

func (m *mockRepo) ListChannels(_ context.Context) ([]*model.Channel, error) {
	return m.channels, m.channelsErr
}

func (m *mockRepo) ListMessages(_ context.Context, _ int64, _, _ int) ([]*model.RawMessage, error) {
	return m.messages, m.messagesErr
}

func (m *mockRepo) GetMessageByID(_ context.Context, _ int64) (*model.RawMessage, error) {
	return m.messageByID, m.messageByIDErr
}

func (m *mockRepo) CountMessagesByChannel(_ context.Context) ([]model.ChannelStats, error) {
	return m.stats, m.statsErr
}

func (m *mockRepo) CountRawMessages(_ context.Context) (int64, error) {
	return m.countRaw, m.countRawErr
}

type mockProcessed struct {
	msgs     []*model.ProcessedMessage
	msgsErr  error
	stats    []model.ProcessedTypeStats
	statsErr error
	count    int64
	countErr error
}

func (m *mockProcessed) ListProcessedMessages(_ context.Context, _ int64, _ string, _, _ int) ([]*model.ProcessedMessage, error) {
	return m.msgs, m.msgsErr
}

func (m *mockProcessed) CountProcessedByType(_ context.Context) ([]model.ProcessedTypeStats, error) {
	return m.stats, m.statsErr
}

func (m *mockProcessed) CountProcessedMessages(_ context.Context) (int64, error) {
	return m.count, m.countErr
}

func (m *mockProcessed) GetPhotoMetadata(_ context.Context, _ int64) (*model.PhotoMetadata, error) {
	return nil, nil
}

func (m *mockProcessed) GetPhotoID(_ context.Context, _ int64) (int64, error) {
	return 0, nil
}

func (m *mockProcessed) GetInlineThumb(_ context.Context, _ int64) ([]byte, error) {
	return nil, nil
}

func (m *mockProcessed) GetPhotoData(_ context.Context, _ int64) ([]byte, error) {
	return nil, nil
}

func (m *mockProcessed) SavePhotoData(_ context.Context, _ int64, _ []byte) error {
	return nil
}

func (m *mockProcessed) PhotoMetadataStats(_ context.Context) (model.PhotoStats, error) {
	return model.PhotoStats{}, nil
}

// Compile-time check.
var _ processor.ProcessedReader = (*mockProcessed)(nil)

// --- Tests ---

func newTestServer(repo *mockRepo, proc *mockProcessed, broker *Broker) *Server {
	var pr processor.ProcessedReader
	if proc != nil {
		pr = proc
	}
	return NewServer(repo, pr, nil, nil, 8080, broker)
}

func TestHandleHealthz_OK(t *testing.T) {
	repo := &mockRepo{countRaw: 42}
	srv := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	srv.handleHealthz(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["status"] != "ok" {
		t.Errorf("status = %v, want ok", body["status"])
	}
	if body["db"] != true {
		t.Errorf("db = %v, want true", body["db"])
	}
	if body["raw_messages"] != float64(42) {
		t.Errorf("raw_messages = %v, want 42", body["raw_messages"])
	}
}

func TestHandleHealthz_Degraded(t *testing.T) {
	repo := &mockRepo{countRawErr: context.DeadlineExceeded}
	srv := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()

	srv.handleHealthz(w, req)

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["status"] != "degraded" {
		t.Errorf("status = %v, want degraded", body["status"])
	}
	if body["db"] != false {
		t.Errorf("db = %v, want false", body["db"])
	}
}

func TestHandleChannels_OK(t *testing.T) {
	repo := &mockRepo{
		stats: []model.ChannelStats{
			{ChannelID: 123, Username: "test_channel", MessageCount: 10},
		},
	}
	srv := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
	w := httptest.NewRecorder()

	srv.handleChannels(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestHandleChannels_Error(t *testing.T) {
	repo := &mockRepo{statsErr: context.DeadlineExceeded}
	srv := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/channels", nil)
	w := httptest.NewRecorder()

	srv.handleChannels(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleMessages_Defaults(t *testing.T) {
	repo := &mockRepo{
		messages: []*model.RawMessage{
			{ID: 1, ChannelID: 100, MessageID: 1, Payload: []byte(`{}`), ReceivedAt: time.Now()},
		},
	}
	srv := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/messages", nil)
	w := httptest.NewRecorder()

	srv.handleMessages(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleMessages_WithParams(t *testing.T) {
	repo := &mockRepo{messages: []*model.RawMessage{}}
	srv := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/messages?channel_id=100&limit=10&offset=5", nil)
	w := httptest.NewRecorder()

	srv.handleMessages(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleMessages_Error(t *testing.T) {
	repo := &mockRepo{messagesErr: context.DeadlineExceeded}
	srv := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/messages", nil)
	w := httptest.NewRecorder()

	srv.handleMessages(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleMessage_OK(t *testing.T) {
	repo := &mockRepo{
		messageByID: &model.RawMessage{ID: 5, ChannelID: 100, MessageID: 42, Payload: []byte(`{}`), ReceivedAt: time.Now()},
	}
	srv := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/message/5", nil)
	w := httptest.NewRecorder()

	srv.handleMessage(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleMessage_InvalidID(t *testing.T) {
	repo := &mockRepo{}
	srv := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/message/abc", nil)
	w := httptest.NewRecorder()

	srv.handleMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestHandleMessage_NotFound(t *testing.T) {
	repo := &mockRepo{messageByIDErr: context.DeadlineExceeded}
	srv := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/message/999", nil)
	w := httptest.NewRecorder()

	srv.handleMessage(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHandleProcessed_NilReader(t *testing.T) {
	repo := &mockRepo{}
	srv := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/processed", nil)
	w := httptest.NewRecorder()

	srv.handleProcessed(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	// Should return empty array
	if body := w.Body.String(); body != "[]\n" {
		t.Errorf("body = %q, want empty array", body)
	}
}

func TestHandleProcessed_WithReader(t *testing.T) {
	proc := &mockProcessed{
		msgs: []*model.ProcessedMessage{
			{ID: 1, ChannelID: 100, MessageID: 42, MessageType: "deal_complete"},
		},
	}
	repo := &mockRepo{}
	srv := newTestServer(repo, proc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/processed?channel_id=100&type=deal_complete&limit=10&offset=0", nil)
	w := httptest.NewRecorder()

	srv.handleProcessed(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestHandleProcessed_Error(t *testing.T) {
	proc := &mockProcessed{msgsErr: context.DeadlineExceeded}
	repo := &mockRepo{}
	srv := newTestServer(repo, proc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/processed", nil)
	w := httptest.NewRecorder()

	srv.handleProcessed(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleProcessedStats_NilReader(t *testing.T) {
	repo := &mockRepo{}
	srv := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/processed/stats", nil)
	w := httptest.NewRecorder()

	srv.handleProcessedStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["total"] != float64(0) {
		t.Errorf("total = %v, want 0", body["total"])
	}
}

func TestHandleProcessedStats_WithReader(t *testing.T) {
	proc := &mockProcessed{
		stats: []model.ProcessedTypeStats{{MessageType: "deal_complete", Count: 5}},
		count: 5,
	}
	repo := &mockRepo{}
	srv := newTestServer(repo, proc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/processed/stats", nil)
	w := httptest.NewRecorder()

	srv.handleProcessedStats(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if body["total"] != float64(5) {
		t.Errorf("total = %v, want 5", body["total"])
	}
}

func TestHandleProcessedStats_Error(t *testing.T) {
	proc := &mockProcessed{statsErr: context.DeadlineExceeded}
	repo := &mockRepo{}
	srv := newTestServer(repo, proc, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/processed/stats", nil)
	w := httptest.NewRecorder()

	srv.handleProcessedStats(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusInternalServerError)
	}
}

func TestHandleIndex_Root(t *testing.T) {
	repo := &mockRepo{}
	srv := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	srv.handleIndex(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html; charset=utf-8" {
		t.Errorf("Content-Type = %q, want text/html; charset=utf-8", ct)
	}
}

func TestHandleIndex_NotFound(t *testing.T) {
	repo := &mockRepo{}
	srv := newTestServer(repo, nil, nil)

	req := httptest.NewRequest(http.MethodGet, "/unknown", nil)
	w := httptest.NewRecorder()

	srv.handleIndex(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// flusherRecorder wraps httptest.ResponseRecorder to also satisfy http.Flusher.
type flusherRecorder struct {
	*httptest.ResponseRecorder
}

func (f *flusherRecorder) Flush() {}

func TestHandleEvents_Subscribe(t *testing.T) {
	broker := NewBroker()
	repo := &mockRepo{}
	srv := newTestServer(repo, nil, broker)

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodGet, "/api/events", nil).WithContext(ctx)
	w := &flusherRecorder{httptest.NewRecorder()}

	done := make(chan struct{})
	go func() {
		defer close(done)
		srv.handleEvents(w, req)
	}()

	// Give the handler time to subscribe and start listening
	time.Sleep(20 * time.Millisecond)

	// Publish an event
	broker.Publish(Event{Type: "new_message", Data: []byte(`{"id":1}`)})
	// Wait briefly for the handler to write
	time.Sleep(50 * time.Millisecond)

	// Cancel to exit the SSE handler
	cancel()
	<-done

	if ct := w.Header().Get("Content-Type"); ct != "text/event-stream" {
		t.Errorf("Content-Type = %q, want text/event-stream", ct)
	}
	body := w.Body.String()
	if body == "" {
		t.Error("expected SSE event in body")
	}
}

// noFlusherWriter is an http.ResponseWriter that does NOT implement http.Flusher.
type noFlusherWriter struct {
	code   int
	header http.Header
}

func (n *noFlusherWriter) Header() http.Header         { return n.header }
func (n *noFlusherWriter) Write(_ []byte) (int, error) { return 0, nil }
func (n *noFlusherWriter) WriteHeader(code int)        { n.code = code }

func TestHandleEvents_NoFlusher(t *testing.T) {
	broker := NewBroker()
	repo := &mockRepo{}
	srv := newTestServer(repo, nil, broker)

	req := httptest.NewRequest(http.MethodGet, "/api/events", nil)
	w := &noFlusherWriter{header: make(http.Header)}

	srv.handleEvents(w, req)

	if w.code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", w.code, http.StatusInternalServerError)
	}
}

func TestHandleMessages_LimitClamping(t *testing.T) {
	repo := &mockRepo{messages: []*model.RawMessage{}}
	srv := newTestServer(repo, nil, nil)

	tests := []struct {
		name  string
		query string
	}{
		{"negative limit", "/api/messages?limit=-1"},
		{"zero limit", "/api/messages?limit=0"},
		{"excessive limit", "/api/messages?limit=200"},
		{"negative offset", "/api/messages?offset=-5"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.query, nil)
			w := httptest.NewRecorder()
			srv.handleMessages(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
			}
		})
	}
}

func TestNewServer_NilLogger(t *testing.T) {
	repo := &mockRepo{}
	srv := NewServer(repo, nil, nil, nil, 8080, nil)
	if srv == nil {
		t.Fatal("NewServer returned nil")
	}
	if srv.log == nil {
		t.Fatal("expected NopLogger, got nil")
	}
}

// --- Spies de mídia para defender o invariante read-only do dashboard ---

// spyMediaRepo é um media.Repository que registra escritas em photo_cache.
// Usado para provar que o caminho HTTP do dashboard nunca persiste: nenhuma
// requisição GET /api/media/{id} deve chamar SavePhotoData.
type spyMediaRepo struct {
	photoID   int64
	photoData []byte
	meta      *model.PhotoMetadata
	thumb     []byte
	saveCalls int
}

func (r *spyMediaRepo) GetPhotoID(context.Context, int64) (int64, error) {
	return r.photoID, nil
}
func (r *spyMediaRepo) GetPhotoData(context.Context, int64) ([]byte, error) {
	return r.photoData, nil
}
func (r *spyMediaRepo) GetPhotoMetadata(context.Context, int64) (*model.PhotoMetadata, error) {
	return r.meta, nil
}
func (r *spyMediaRepo) GetInlineThumb(context.Context, int64) ([]byte, error) {
	return r.thumb, nil
}
func (r *spyMediaRepo) SavePhotoData(_ context.Context, _ int64, _ []byte) error {
	r.saveCalls++
	return nil
}

// spyMediaClient é um media.Client que registra chamadas de DownloadPhoto.
// Usado para provar que o dashboard não dispara download MTProto em requests
// HTTP (AGENTS.md §8: "Dashboard → Telegram" é proibido).
type spyMediaClient struct {
	downloadCalls int
	data          []byte
	err           error
}

func (c *spyMediaClient) DownloadPhoto(context.Context, media.PhotoDownloadRequest) ([]byte, error) {
	c.downloadCalls++
	return c.data, c.err
}
func (c *spyMediaClient) DownloadPhotoBatch(_ context.Context, reqs []media.PhotoDownloadRequest, h func(int64, []byte, error)) error {
	for _, req := range reqs {
		c.downloadCalls++
		h(req.PhotoID, c.data, c.err)
	}
	return nil
}

var (
	_ media.Repository = (*spyMediaRepo)(nil)
	_ media.Client     = (*spyMediaClient)(nil)
)

// TestHandleMedia_ReadOnlyResolverNeverWrites defende o invariante arquitetural
// de que o dashboard é read-only (AGENTS.md §8): uma requisição
// GET /api/media/{id} deve servir mídia pela cadeia do resolver SEM disparar
// download MTProto (DownloadPhoto) e SEM persistir em photo_cache (SavePhotoData).
//
// O resolver é injetado SEM client (client == nil), que é o modo read-only
// correto do dashboard. O CENÁRIO É PROPOSITALMENTE ARMADO: photo_cache ausente
// (L2 miss) + metadados MTProto completos. Com client == nil o resolver cai em
// inline_thumb (L4) e nada escreve; se o dashboard fosse (incorretamente) wired
// com um resolver de client on-demand, este cenário dispararia L3 → DownloadPhoto
// + SavePhotoData, e o teste FALHARIA em repo.saveCalls != 0.
func TestHandleMedia_ReadOnlyResolverNeverWrites(t *testing.T) {
	// Stripped thumbnail válido para thumbnail.Expand: exige len >= 3 e data[0] == 0x01.
	strippedThumb := []byte{0x01, 0x00, 0x00}
	repo := &spyMediaRepo{
		photoID:   42,
		photoData: nil, // L2 miss: força a cadeia além de photo_cache
		// Metadados completos: um client on-demand faria L3 aqui.
		meta:  &model.PhotoMetadata{PhotoID: 42, AccessHash: 123, DCID: 4},
		thumb: strippedThumb,
	}
	// client == nil é o contrato read-only do dashboard.
	resolver := media.NewResolver(repo, media.NewCache(8, 0), nil, nil)

	srv := NewServer(&mockRepo{}, &mockProcessed{}, resolver, nil, 8080, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/media/100", nil)
	w := httptest.NewRecorder()
	srv.handleMedia(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%q", w.Code, w.Body.String())
	}
	if got := w.Header().Get("Content-Type"); got != "image/jpeg" {
		t.Errorf("Content-Type = %q, want image/jpeg", got)
	}
	if w.Body.Len() == 0 {
		t.Error("corpo da resposta vazio; esperado JPEG expandido do inline_thumb")
	}
	if repo.saveCalls != 0 {
		t.Errorf("SavePhotoData chamado %dx; GET /api/media deve ser read-only (AGENTS.md §8 proíbe Dashboard → escrita em banco)", repo.saveCalls)
	}
}

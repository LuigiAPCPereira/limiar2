package dashboard

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"strconv"
	"time"

	"github.com/gotd/td/telegram/thumbnail"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/media"
	"github.com/limiar/collector/internal/model"
	"github.com/limiar/collector/internal/processor"
)

// Server é um dashboard HTTP minimalista para inspecionar mensagens capturadas.
// Quando o broker não é nil, o endpoint SSE /api/events é habilitado para
// atualizações em tempo real.
type Server struct {
	repo          dashboardRepo
	processed     processor.ProcessedReader
	mediaResolver *media.Resolver
	log           logger.Logger
	port          int
	broker        *Broker
	startedAt     time.Time
}

// dashboardRepo define as queries de leitura do storage consumidas pelo dashboard.
type dashboardRepo interface {
	ListChannels(ctx context.Context) ([]*model.Channel, error)
	ListMessages(ctx context.Context, channelID int64, limit, offset int) ([]*model.RawMessage, error)
	GetMessageByID(ctx context.Context, id int64) (*model.RawMessage, error)
	CountMessagesByChannel(ctx context.Context) ([]model.ChannelStats, error)
	CountRawMessages(ctx context.Context) (int64, error)
}

// NewServer constrói um servidor de dashboard na porta fornecida. broker pode ser nil
// para desabilitar o SSE (o dashboard fará fallback para polling).
// processed pode ser nil para desabilitar as queries de mensagens processadas
// (usado pelo limiar-collector standalone que não tem acesso ao processor).
// mediaResolver pode ser nil para desabilitar o endpoint de imagens.
func NewServer(repo dashboardRepo, processed processor.ProcessedReader, mediaResolver *media.Resolver, log logger.Logger, port int, broker *Broker) *Server {
	if log == nil {
		log = logger.NopLogger{}
	}
	return &Server{repo: repo, processed: processed, mediaResolver: mediaResolver, log: log, port: port, broker: broker, startedAt: time.Now()}
}

// ListenAndServe inicia o servidor HTTP e bloqueia até que ctx seja cancelado.
func (s *Server) ListenAndServe(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/api/channels", s.handleChannels)
	mux.HandleFunc("/api/messages", s.handleMessages)
	mux.HandleFunc("/api/message/", s.handleMessage)
	mux.HandleFunc("/api/processed", s.handleProcessed)
	mux.HandleFunc("/api/processed/stats", s.handleProcessedStats)
	if s.mediaResolver != nil {
		mux.HandleFunc("/api/media/", s.handleMedia)
	}
	if s.broker != nil {
		mux.HandleFunc("/api/events", s.handleEvents)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", s.port)

	secureMux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; font-src 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		mux.ServeHTTP(w, r)
	})

	srv := &http.Server{
		Addr:              addr,
		Handler:           secureMux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		MaxHeaderBytes:    1 << 20, // 1MB limite para cabeçalhos para evitar DoS
	}

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("🌐 Dashboard iniciado", "porta", s.port, "url", fmt.Sprintf("http://localhost:%d", s.port))
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if r.URL.Path == "/" {
		data, err := distFS.ReadFile("dist/index.html")
		if err != nil {
			s.log.Error("Erro ao servir index do dashboard", "erro", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(data)
		return
	}
	distFileServer.ServeHTTP(w, r)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	count, err := s.repo.CountRawMessages(r.Context())
	status := "ok"
	dbStatus := true
	if err != nil {
		status = "degraded"
		dbStatus = false
	}
	s.writeJSON(w, map[string]any{
		"status":         status,
		"db":             dbStatus,
		"raw_messages":   count,
		"uptime_seconds": time.Since(s.startedAt).Seconds(),
	})
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	stats, err := s.repo.CountMessagesByChannel(r.Context())
	if err != nil {
		s.log.Error("Erro interno no dashboard", "erro", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, stats)
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	channelID, _ := strconv.ParseInt(r.URL.Query().Get("channel_id"), 10, 64)
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	msgs, err := s.repo.ListMessages(r.Context(), channelID, limit, offset)
	if err != nil {
		s.log.Error("Erro interno no dashboard", "erro", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, msgs)
}

func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	idStr := r.URL.Path[len("/api/message/"):]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	msg, err := s.repo.GetMessageByID(r.Context(), id)
	if err != nil {
		s.log.Error("Erro ao buscar mensagem por ID", "erro", err)
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}
	s.writeJSON(w, msg)
}

func (s *Server) handleProcessed(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if s.processed == nil {
		s.writeJSON(w, []any{})
		return
	}
	channelID, _ := strconv.ParseInt(r.URL.Query().Get("channel_id"), 10, 64)
	msgType := r.URL.Query().Get("type")
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	if offset < 0 {
		offset = 0
	}

	msgs, err := s.processed.ListProcessedMessages(r.Context(), channelID, msgType, limit, offset)
	if err != nil {
		s.log.Error("Erro interno no dashboard", "erro", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	s.writeJSON(w, msgs)
}

func (s *Server) handleProcessedStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	if s.processed == nil {
		s.writeJSON(w, map[string]any{"total": 0, "by_type": []any{}})
		return
	}
	stats, err := s.processed.CountProcessedByType(r.Context())
	if err != nil {
		s.log.Error("Erro interno no dashboard", "erro", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	total, totalErr := s.processed.CountProcessedMessages(r.Context())
	if totalErr != nil {
		s.log.Warn("Erro ao contar mensagens processadas", "erro", totalErr)
	}
	s.writeJSON(w, map[string]any{
		"total":   total,
		"by_type": stats,
	})
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch, err := s.broker.Subscribe()
	if err != nil {
		http.Error(w, "too many subscribers", http.StatusServiceUnavailable)
		return
	}
	defer s.broker.Unsubscribe(ch)

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			_, _ = fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.Data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func (s *Server) handleMedia(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	idStr := r.URL.Path[len("/api/media/"):]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	data, source, err := s.mediaResolver.ResolveImage(r.Context(), id)
	if err != nil {
		s.log.Error("Erro ao resolver imagem", "id", id, "erro", err)
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
		return
	}

	jpeg := data
	if source == "inline-thumb" {
		expanded, err := thumbnail.Expand(data)
		if err != nil {
			s.log.Warn("Erro ao expandir inline_thumb", "processed_msg_id", id, "erro", err)
			http.NotFound(w, r)
			return
		}
		jpeg = expanded
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	w.Header().Set("X-Source", source)
	_, _ = w.Write(jpeg)
}

func (s *Server) writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		s.log.Error("Erro interno no dashboard", "erro", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

//go:embed all:dist
var distFS embed.FS

var distFileServer http.Handler

func init() {
	sub, err := fs.Sub(distFS, "dist")
	if err != nil {
		panic(fmt.Sprintf("dashboard: fs.Sub: %v", err))
	}
	distFileServer = http.FileServer(http.FS(sub))
}

package dashboard

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/storage"
)

// Server é um dashboard HTTP minimalista para inspecionar mensagens capturadas.
// Quando o broker não é nil, o endpoint SSE /api/events é habilitado para
// atualizações em tempo real.
type Server struct {
	repo   *storage.Repository
	log    logger.Logger
	port   int
	broker *Broker
}

// NewServer constrói um servidor de dashboard na porta fornecida. broker pode ser nil
// para desabilitar o SSE (o dashboard fará fallback para polling).
func NewServer(repo *storage.Repository, log logger.Logger, port int, broker *Broker) *Server {
	if log == nil {
		log = logger.NopLogger{}
	}
	return &Server{repo: repo, log: log, port: port, broker: broker}
}

// ListenAndServe inicia o servidor HTTP e bloqueia até que ctx seja cancelado.
func (s *Server) ListenAndServe(ctx context.Context) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", s.handleIndex)
	mux.HandleFunc("/api/channels", s.handleChannels)
	mux.HandleFunc("/api/messages", s.handleMessages)
	mux.HandleFunc("/api/message/", s.handleMessage)
	if s.broker != nil {
		mux.HandleFunc("/api/events", s.handleEvents)
	}

	addr := fmt.Sprintf("127.0.0.1:%d", s.port)
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.log.Info("🌐 Dashboard iniciado", "porta", s.port, "url", fmt.Sprintf("http://localhost:%d", s.port))
		errCh <- srv.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		return srv.Shutdown(context.Background())
	case err := <-errCh:
		return err
	}
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, indexHTML)
}

func (s *Server) handleChannels(w http.ResponseWriter, r *http.Request) {
	stats, err := s.repo.CountMessagesByChannel(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, stats)
}

func (s *Server) handleMessages(w http.ResponseWriter, r *http.Request) {
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
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, msgs)
}

func (s *Server) handleMessage(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Path[len("/api/message/"):]
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	msg, err := s.repo.GetMessageByID(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	writeJSON(w, msg)
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	ch := s.broker.Subscribe()
	defer s.broker.Unsubscribe(ch)

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event.Type, event.Data)
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}

//go:embed index.html
var indexHTML string

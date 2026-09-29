package mcptelegram

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/LuigiAPCPereira/limiar2/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	defaultHistoryPageSize = 20
	observationKind        = "telegram_realtime_not_source_evidence"
)

var ErrInvalidConfig = errors.New("mcp telegram: configuração inválida")

type Target struct {
	Name string
	Ref  telegram.PeerRef
}

type Config struct {
	Targets            []Target
	MaxHistoryPageSize int
}

type Adapter struct {
	query              telegram.TelegramQuery
	targets            []Target
	targetByName       map[string]Target
	maxHistoryPageSize int
	server             *mcp.Server
}

type targetsInput struct{}

type targetOutput struct {
	Name string `json:"name"`
}

type targetsOutput struct {
	Observation string         `json:"observation"`
	Targets     []targetOutput `json:"targets"`
}

type historyInput struct {
	Target string `json:"target" jsonschema:"nome do target do Telegram configurado"`
	Limit  int    `json:"limit,omitempty" jsonschema:"mensagens nesta página; omita para usar o tamanho padrão da página"`
	Cursor string `json:"cursor,omitempty" jsonschema:"next_cursor opaco retornado por uma chamada anterior de telegram.history"`
}

type historyOutput struct {
	Observation    string          `json:"observation"`
	Target         string          `json:"target"`
	RequestedLimit int             `json:"requested_limit"`
	Messages       []messageOutput `json:"messages"`
	NextCursor     string          `json:"next_cursor,omitempty"`
}

type messageOutput struct {
	ID            int64  `json:"id"`
	PeerKind      string `json:"peer_kind"`
	PeerID        int64  `json:"peer_id"`
	Date          string `json:"date"`
	Kind          string `json:"kind"`
	UpstreamType  string `json:"upstream_type"`
	Text          string `json:"text,omitempty"`
	EditedAt      string `json:"edited_at,omitempty"`
	GroupedID     int64  `json:"grouped_id,omitempty"`
	MediaKind     string `json:"media_kind,omitempty"`
	ServiceAction string `json:"service_action,omitempty"`
}

func New(query telegram.TelegramQuery, cfg Config) (*Adapter, error) {
	if query == nil {
		return nil, fmt.Errorf("%w: TelegramQuery ausente", ErrInvalidConfig)
	}
	if cfg.MaxHistoryPageSize < 1 || cfg.MaxHistoryPageSize > telegram.MaxHistoryPageSize {
		return nil, fmt.Errorf(
			"%w: o tamanho máximo da página de histórico deve ficar entre 1 e %d",
			ErrInvalidConfig,
			telegram.MaxHistoryPageSize,
		)
	}

	a := &Adapter{
		query:              query,
		targets:            make([]Target, 0, len(cfg.Targets)),
		targetByName:       make(map[string]Target, len(cfg.Targets)),
		maxHistoryPageSize: cfg.MaxHistoryPageSize,
	}

	for _, configured := range cfg.Targets {
		name := strings.TrimSpace(configured.Name)
		ref := strings.TrimSpace(configured.Ref.Value)
		if name == "" || ref == "" {
			return nil, fmt.Errorf("%w: nome e referência do target não podem estar vazios", ErrInvalidConfig)
		}
		if _, exists := a.targetByName[name]; exists {
			return nil, fmt.Errorf("%w: target duplicado %q", ErrInvalidConfig, name)
		}
		target := Target{Name: name, Ref: telegram.PeerRef{Value: ref}}
		a.targets = append(a.targets, target)
		a.targetByName[name] = target
	}

	a.server = a.newMCPServer()
	return a, nil
}

func (a *Adapter) Server() *mcp.Server {
	if a == nil {
		return nil
	}
	return a.server
}

func (a *Adapter) HTTPServer(address string) (*http.Server, error) {
	if a == nil || a.server == nil {
		return nil, fmt.Errorf("%w: adapter inválido", ErrInvalidConfig)
	}
	if err := validateLoopbackAddress(address); err != nil {
		return nil, err
	}

	streamable := mcp.NewStreamableHTTPHandler(
		func(*http.Request) *mcp.Server { return a.server },
		&mcp.StreamableHTTPOptions{
			Stateless:                    true,
			PropagateRequestCancellation: true,
			MaxRequestBodyBytes:          mcp.DefaultMaxRequestBodyBytes,
		},
	)
	protection := http.NewCrossOriginProtection()

	return &http.Server{
		Addr:    address,
		Handler: protection.Handler(streamable),
	}, nil
}

func (a *Adapter) newMCPServer() *mcp.Server {
	server := mcp.NewServer(
		&mcp.Implementation{Name: "limiar-telegram-realtime", Version: "l3-003"},
		&mcp.ServerOptions{
			Capabilities: &mcp.ServerCapabilities{
				Tools: &mcp.ToolCapabilities{},
			},
		},
	)

	openWorld := true
	annotations := &mcp.ToolAnnotations{
		IdempotentHint: true,
		OpenWorldHint:  &openWorld,
		ReadOnlyHint:   true,
	}

	mcp.AddTool(server, &mcp.Tool{
		Name:        "telegram.targets",
		Title:       "Targets do Telegram",
		Description: "List the Targets do Telegram configured for this MCP read scope. Use these names when calling telegram.history.",
		Annotations: annotations,
	}, a.handleTargets)

	mcp.AddTool(server, &mcp.Tool{
		Name:  "telegram.history",
		Title: "Histórico do Telegram",
		Description: fmt.Sprintf(
			"Lê uma página limitada de mensagens do Telegram de um target configurado. limit usa %d por padrão e pode variar de 1 a %d. Para ler mais, chame novamente com next_cursor; uma única chamada não é ilimitada.",
			min(defaultHistoryPageSize, a.maxHistoryPageSize),
			a.maxHistoryPageSize,
		),
		Annotations: annotations,
	}, a.handleHistory)

	return server
}

func (a *Adapter) handleTargets(
	context.Context,
	*mcp.CallToolRequest,
	targetsInput,
) (*mcp.CallToolResult, targetsOutput, error) {
	out := targetsOutput{
		Observation: observationKind,
		Targets:     make([]targetOutput, 0, len(a.targets)),
	}
	for _, target := range a.targets {
		out.Targets = append(out.Targets, targetOutput{Name: target.Name})
	}
	return nil, out, nil
}

func (a *Adapter) handleHistory(
	ctx context.Context,
	_ *mcp.CallToolRequest,
	in historyInput,
) (*mcp.CallToolResult, historyOutput, error) {
	targetName := strings.TrimSpace(in.Target)
	target, ok := a.targetByName[targetName]
	if !ok {
		return nil, historyOutput{}, fmt.Errorf("target do Telegram %q não está configurado", targetName)
	}

	limit := in.Limit
	if limit == 0 {
		limit = min(defaultHistoryPageSize, a.maxHistoryPageSize)
	}
	if limit < 1 || limit > a.maxHistoryPageSize {
		return nil, historyOutput{}, fmt.Errorf(
			"o limite do histórico do Telegram deve ficar entre 1 e %d; use next_cursor para páginas adicionais",
			a.maxHistoryPageSize,
		)
	}

	descriptor, err := a.query.ResolvePeer(ctx, target.Ref)
	if err != nil {
		return nil, historyOutput{}, safeTelegramError(err)
	}
	page, err := a.query.History(ctx, descriptor.Key, telegram.HistoryRequest{
		Limit:  limit,
		Cursor: in.Cursor,
	})
	if err != nil {
		return nil, historyOutput{}, safeTelegramError(err)
	}

	out := historyOutput{
		Observation:    observationKind,
		Target:         target.Name,
		RequestedLimit: limit,
		Messages:       make([]messageOutput, 0, len(page.Messages)),
		NextCursor:     page.NextCursor,
	}
	for _, message := range page.Messages {
		out.Messages = append(out.Messages, mapMessage(message))
	}
	return nil, out, nil
}

func mapMessage(message telegram.Message) messageOutput {
	out := messageOutput{
		ID:            message.ID,
		PeerKind:      string(message.Peer.Kind),
		PeerID:        message.Peer.ID,
		Kind:          string(message.Kind),
		UpstreamType:  message.UpstreamType,
		Text:          message.Text,
		GroupedID:     message.GroupedID,
		MediaKind:     message.MediaKind,
		ServiceAction: message.ServiceAction,
	}
	if !message.Date.IsZero() {
		out.Date = message.Date.UTC().Format(time.RFC3339)
	}
	if !message.EditedAt.IsZero() {
		out.EditedAt = message.EditedAt.UTC().Format(time.RFC3339)
	}
	return out
}

func safeTelegramError(err error) error {
	if err == nil {
		return nil
	}
	switch {
	case errors.Is(err, context.Canceled):
		return context.Canceled
	case errors.Is(err, context.DeadlineExceeded):
		return context.DeadlineExceeded
	case errors.Is(err, telegram.ErrInvalidCursor):
		return errors.New("cursor do Telegram inválido")
	case errors.Is(err, telegram.ErrPeerNotResolved):
		return errors.New("peer do Telegram não resolvido")
	case errors.Is(err, telegram.ErrUnsupportedPeer):
		return errors.New("tipo de peer do Telegram não suportado")
	case errors.Is(err, telegram.ErrInvalidQuery):
		return errors.New("consulta Telegram inválida")
	case errors.Is(err, telegram.ErrInvalidUpstreamMessage):
		return errors.New("Telegram retornou uma mensagem inválida")
	}

	var operationErr *telegram.OperationError
	if errors.As(err, &operationErr) {
		if operationErr.RetryAfter > 0 {
			return fmt.Errorf("telegram %s; tente novamente após %s", operationErr.Kind, operationErr.RetryAfter)
		}
		return fmt.Errorf("telegram %s", operationErr.Kind)
	}
	return errors.New("requisição ao Telegram falhou")
}

func validateLoopbackAddress(address string) error {
	host, port, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("%w: endereço de escuta deve usar host:porta", ErrInvalidConfig)
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("%w: listener MCP deve usar um endereço IP de loopback", ErrInvalidConfig)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 0 || portNumber > 65535 {
		return fmt.Errorf("%w: porta de escuta inválida", ErrInvalidConfig)
	}
	return nil
}

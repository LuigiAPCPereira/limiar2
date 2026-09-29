package mcptelegram

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/limiar2/internal/telegram"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fakeQuery struct {
	resolveCalls int
	historyCalls int

	lastRef     telegram.PeerRef
	lastKey     telegram.PeerKey
	lastRequest telegram.HistoryRequest

	resolveErr error
	historyErr error
	page       telegram.MessagePage
}

func (f *fakeQuery) ResolvePeer(_ context.Context, ref telegram.PeerRef) (telegram.PeerDescriptor, error) {
	f.resolveCalls++
	f.lastRef = ref
	if f.resolveErr != nil {
		return telegram.PeerDescriptor{}, f.resolveErr
	}
	return telegram.PeerDescriptor{
		Key: telegram.PeerKey{Kind: telegram.PeerKindChannel, ID: 42},
	}, nil
}

func (f *fakeQuery) History(
	_ context.Context,
	key telegram.PeerKey,
	req telegram.HistoryRequest,
) (telegram.MessagePage, error) {
	f.historyCalls++
	f.lastKey = key
	f.lastRequest = req
	if f.historyErr != nil {
		return telegram.MessagePage{}, f.historyErr
	}
	return f.page, nil
}

func TestServerDiscoveryExposesOnlyReadOnlyTracerTools(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t, &fakeQuery{})
	session := connectTestClient(t, adapter.Server())

	var names []string
	for tool, err := range session.Tools(context.Background(), nil) {
		if err != nil {
			t.Fatal(err)
		}
		names = append(names, tool.Name)
		if tool.Annotations == nil || !tool.Annotations.ReadOnlyHint || !tool.Annotations.IdempotentHint {
			t.Fatalf("tool %q annotations=%+v, esperado read-only/idempotent", tool.Name, tool.Annotations)
		}
	}
	sort.Strings(names)
	esperado := []string{"telegram.history", "telegram.targets"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("tools=%v, esperado %v", names, want)
	}
}

func TestTargetsReturnsConfiguredNamesWithoutPeerReferences(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t, &fakeQuery{})
	session := connectTestClient(t, adapter.Server())

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{Name: "telegram.targets"})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("telegram.targets retornou erro da tool: %s", resultText(result))
	}

	var out targetsOutput
	decodeStructured(t, result, &out)
	if out.Observation != observationKind {
		t.Fatalf("observation=%q", out.Observation)
	}
	if len(out.Targets) != 1 || out.Targets[0].Name != "phones" {
		t.Fatalf("targets=%+v", out.Targets)
	}

	payload, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(payload), "@phones") {
		t.Fatalf("referência de peer vazou in targets output: %s", payload)
	}
}

func TestHistoryPassesRequestedPageAndPreservesCursor(t *testing.T) {
	t.Parallel()

	fake := &fakeQuery{
		page: telegram.MessagePage{
			Messages: []telegram.Message{{
				ID:           101,
				Peer:         telegram.PeerKey{Kind: telegram.PeerKindChannel, ID: 42},
				Date:         time.Unix(1_700_000_000, 0).UTC(),
				Kind:         telegram.MessageKindRegular,
				UpstreamType: "message",
				Text:         "Galaxy S26 R$ 3.999",
				MediaKind:    "messageMediaPhoto",
			}},
			NextCursor: "tg-history-v1:101:1700000000",
		},
	}
	adapter := newTestAdapter(t, fake)
	session := connectTestClient(t, adapter.Server())

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "telegram.history",
		Arguments: map[string]any{
			"target": "phones",
			"limit":  20,
			"cursor": "opaque-previous",
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("telegram.history retornou erro da tool: %s", resultText(result))
	}

	if fake.resolveCalls != 1 || fake.historyCalls != 1 {
		t.Fatalf("calls resolve=%d history=%d", fake.resolveCalls, fake.historyCalls)
	}
	if fake.lastRef.Value != "@phones" {
		t.Fatalf("resolved ref=%q", fake.lastRef.Value)
	}
	if fake.lastRequest.Limit != 20 || fake.lastRequest.Cursor != "opaque-previous" {
		t.Fatalf("history request=%+v", fake.lastRequest)
	}

	var out historyOutput
	decodeStructured(t, result, &out)
	if out.RequestedLimit != 20 || out.NextCursor != "tg-history-v1:101:1700000000" {
		t.Fatalf("history output=%+v", out)
	}
	if len(out.Messages) != 1 || out.Messages[0].Text != "Galaxy S26 R$ 3.999" {
		t.Fatalf("messages=%+v", out.Messages)
	}
	if out.Messages[0].Date != "2023-11-14T22:13:20Z" {
		t.Fatalf("message date=%q", out.Messages[0].Date)
	}
}

func TestHistoryDefaultsToTwentyButAllowsUpToConfiguredPageCeiling(t *testing.T) {
	t.Parallel()

	fake := &fakeQuery{}
	adapter := newTestAdapter(t, fake)
	session := connectTestClient(t, adapter.Server())

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "telegram.history",
		Arguments: map[string]any{"target": "phones"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("histórico padrão retornou erro: %s", resultText(result))
	}
	if fake.lastRequest.Limit != 20 {
		t.Fatalf("default limit=%d, esperado 20", fake.lastRequest.Limit)
	}

	result, err = session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "telegram.history",
		Arguments: map[string]any{
			"target": "phones",
			"limit":  telegram.MaxHistoryPageSize,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError {
		t.Fatalf("histórico máximo retornou erro: %s", resultText(result))
	}
	if fake.lastRequest.Limit != telegram.MaxHistoryPageSize {
		t.Fatalf("max limit=%d", fake.lastRequest.Limit)
	}
}

func TestHistoryRejectsUnboundedSingleCallBeforeTelegram(t *testing.T) {
	t.Parallel()

	fake := &fakeQuery{}
	adapter := newTestAdapter(t, fake)
	session := connectTestClient(t, adapter.Server())

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name: "telegram.history",
		Arguments: map[string]any{
			"target": "phones",
			"limit":  telegram.MaxHistoryPageSize + 1,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("limite acima do teto da página foi aceito, esperado tool error")
	}
	if fake.resolveCalls != 0 || fake.historyCalls != 0 {
		t.Fatalf("TelegramQuery called for rejected limit: resolve=%d history=%d", fake.resolveCalls, fake.historyCalls)
	}
	if !strings.Contains(resultText(result), "next_cursor") {
		t.Fatalf("erro não explica a paginação: %q", resultText(result))
	}
}

func TestHistoryRejectsUnknownTargetBeforeTelegram(t *testing.T) {
	t.Parallel()

	fake := &fakeQuery{}
	adapter := newTestAdapter(t, fake)
	session := connectTestClient(t, adapter.Server())

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "telegram.history",
		Arguments: map[string]any{"target": "not-configured", "limit": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("target desconhecido foi aceito, esperado tool error")
	}
	if fake.resolveCalls != 0 || fake.historyCalls != 0 {
		t.Fatalf("TelegramQuery called for unknown target: resolve=%d history=%d", fake.resolveCalls, fake.historyCalls)
	}
}

func TestHistoryDoesNotLeakUnexpectedTelegramErrors(t *testing.T) {
	t.Parallel()

	fake := &fakeQuery{historyErr: errors.New("SESSION_BYTES=secret-value")}
	adapter := newTestAdapter(t, fake)
	session := connectTestClient(t, adapter.Server())

	result, err := session.CallTool(context.Background(), &mcp.CallToolParams{
		Name:      "telegram.history",
		Arguments: map[string]any{"target": "phones", "limit": 1},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.IsError {
		t.Fatal("falha upstream inesperada foi tratada como sucesso")
	}
	text := resultText(result)
	if strings.Contains(text, "SESSION_BYTES") || strings.Contains(text, "secret-value") {
		t.Fatalf("erro upstream bruto vazou: %q", text)
	}
	if text != "requisição ao Telegram falhou" {
		t.Fatalf("erro sanitizado=%q", text)
	}
}

func TestHTTPServerIsLoopbackOnlyAndRejectsCrossOrigin(t *testing.T) {
	t.Parallel()

	adapter := newTestAdapter(t, &fakeQuery{})

	for _, address := range []string{
		"0.0.0.0:8080",
		"192.168.1.10:8080",
		"localhost:8080",
		":8080",
	} {
		if _, err := adapter.HTTPServer(address); !errors.Is(err, ErrInvalidConfig) {
			t.Fatalf("HTTPServer(%q) error=%v, esperado ErrInvalidConfig", address, err)
		}
	}

	server, err := adapter.HTTPServer("127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	if server.Addr != "127.0.0.1:0" {
		t.Fatalf("server address=%q", server.Addr)
	}

	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1/mcp", strings.NewReader(`{}`))
	req.Header.Set("Origin", "https://evil.example")
	recorder := httptest.NewRecorder()
	server.Handler.ServeHTTP(recorder, req)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("status cross-origin=%d, esperado 403", recorder.Code)
	}
}

func TestConfigRejectsDuplicateTargetsAndInvalidPageCeiling(t *testing.T) {
	t.Parallel()

	query := &fakeQuery{}
	if _, err := New(query, Config{
		MaxHistoryPageSize: telegram.MaxHistoryPageSize + 1,
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("erro no teto da página=%v", err)
	}
	if _, err := New(query, Config{
		MaxHistoryPageSize: 10,
		Targets: []Target{
			{Name: "phones", Ref: telegram.PeerRef{Value: "@one"}},
			{Name: " phones ", Ref: telegram.PeerRef{Value: "@two"}},
		},
	}); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("erro de target duplicado=%v", err)
	}
}

func newTestAdapter(t *testing.T, query telegram.TelegramQuery) *Adapter {
	t.Helper()
	adapter, err := New(query, Config{
		MaxHistoryPageSize: telegram.MaxHistoryPageSize,
		Targets: []Target{{
			Name: "phones",
			Ref:  telegram.PeerRef{Value: "@phones"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return adapter
}

func connectTestClient(t *testing.T, server *mcp.Server) *mcp.ClientSession {
	t.Helper()
	ctx := context.Background()
	serverTransport, clientTransport := mcp.NewInMemoryTransports()

	serverSession, err := server.Connect(ctx, serverTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = serverSession.Close() })

	client := mcp.NewClient(
		&mcp.Implementation{Name: "limiar-test-client", Version: "test"},
		&mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}},
	)
	clientSession, err := client.Connect(ctx, clientTransport, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = clientSession.Close() })
	return clientSession
}

func decodeStructured(t *testing.T, result *mcp.CallToolResult, out any) {
	t.Helper()
	payload, err := json.Marshal(result.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(payload, out); err != nil {
		t.Fatal(err)
	}
}

func resultText(result *mcp.CallToolResult) string {
	if result == nil || len(result.Content) == 0 {
		return ""
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		return ""
	}
	return text.Text
}

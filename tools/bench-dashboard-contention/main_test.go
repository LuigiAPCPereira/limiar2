package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseConfigDefaultsAndFlags(t *testing.T) {
	cfg, err := parseConfig(nil)
	if err != nil {
		t.Fatalf("parseConfig(nil) returned error: %v", err)
	}

	if cfg.bin != "./limiar" {
		t.Fatalf("default bin = %q, want ./limiar", cfg.bin)
	}
	if cfg.dbPath != "./limiar.db" {
		t.Fatalf("default db path = %q, want ./limiar.db", cfg.dbPath)
	}
	if cfg.dashboardURL != "http://127.0.0.1:8080" {
		t.Fatalf("default dashboard url = %q", cfg.dashboardURL)
	}
	if cfg.mode != "in-process" {
		t.Fatalf("default mode = %q, want in-process", cfg.mode)
	}
	if cfg.scenario != "all" {
		t.Fatalf("default scenario = %q, want all", cfg.scenario)
	}
	if cfg.concurrency != 1 {
		t.Fatalf("default concurrency = %d, want 1", cfg.concurrency)
	}
	if cfg.duration != 120*time.Second {
		t.Fatalf("default duration = %s, want 120s", cfg.duration)
	}
	if cfg.interval != 250*time.Millisecond {
		t.Fatalf("default interval = %s, want 250ms", cfg.interval)
	}
	if cfg.timeout != 2*time.Second {
		t.Fatalf("default timeout = %s, want 2s", cfg.timeout)
	}

	cfg, err = parseConfig([]string{
		"--bin", "/tmp/limiar-bench",
		"--db", "/tmp/limiar.db",
		"--dashboard-url", "http://127.0.0.1:9090",
		"--scenario", "heavy",
		"--mode", "external",
		"--concurrency", "4",
		"--duration", "30s",
		"--interval", "10ms",
		"--timeout", "750ms",
	})
	if err != nil {
		t.Fatalf("parseConfig(flags) returned error: %v", err)
	}
	if cfg.bin != "/tmp/limiar-bench" || cfg.dbPath != "/tmp/limiar.db" || cfg.dashboardURL != "http://127.0.0.1:9090" {
		t.Fatalf("paths not parsed correctly: %+v", cfg)
	}
	if cfg.scenario != "heavy" || cfg.mode != "external" || cfg.concurrency != 4 || cfg.duration != 30*time.Second || cfg.interval != 10*time.Millisecond || cfg.timeout != 750*time.Millisecond {
		t.Fatalf("runtime flags not parsed correctly: %+v", cfg)
	}

	invalidCases := [][]string{
		{"--scenario", "unknown"},
		{"--mode", "invalid"},
		{"--concurrency", "0"},
		{"--duration", "0s"},
		{"--interval", "-1ms"},
		{"--timeout", "0s"},
	}
	for _, args := range invalidCases {
		if _, err := parseConfig(args); err == nil {
			t.Fatalf("parseConfig(%v) returned nil error", args)
		}
	}
}

func TestHelpTextContainsDocumentedFlags(t *testing.T) {
	help := helpText()
	for _, flag := range []string{
		"--bin",
		"--db",
		"--dashboard-url",
		"--mode",
		"--scenario",
		"--concurrency",
		"--duration",
		"--interval",
		"--timeout",
		"baseline|healthz|channels|light|heavy|stats|all",
	} {
		if !strings.Contains(help, flag) {
			t.Fatalf("help text missing %q:\n%s", flag, help)
		}
	}
}

func TestEndpointSpecsExposeIsolatedLightEndpoints(t *testing.T) {
	tests := []struct {
		name     string
		expected []string
	}{
		{name: "healthz", expected: []string{"/healthz"}},
		{name: "channels", expected: []string{"/api/channels"}},
		{name: "stats", expected: []string{"/api/processed/stats"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			endpoints := endpointSpecs(tt.name)
			if len(endpoints) != len(tt.expected) {
				t.Fatalf("endpointSpecs(%q) returned %d endpoints, want %d", tt.name, len(endpoints), len(tt.expected))
			}
			for i, endpoint := range endpoints {
				if endpoint.Path != tt.expected[i] {
					t.Fatalf("endpointSpecs(%q)[%d] = %q, want %q", tt.name, i, endpoint.Path, tt.expected[i])
				}
			}
		})
	}
}

func TestFriendlyLockErrorExplainsTursoSingleProcess(t *testing.T) {
	err := friendlyOpenError(
		"./limiar.db",
		"errors: storage: ping: turso: error: Locking error: Failed locking file './limiar.db'. File is locked by another process",
	)
	if err == nil {
		t.Fatalf("friendlyOpenError returned nil")
	}
	msg := err.Error()
	for _, want := range []string{
		"Tursogo não permite múltiplos processos",
		"pare o limiar",
		"./limiar.db",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("friendly error missing %q: %s", want, msg)
		}
	}
}

func TestParseReprocessOutputPortugueseSummary(t *testing.T) {
	processed, failed, ok := parseReprocessOutput("✅ Reprocessamento concluído: 31832 processadas, 0 falharam, 2m11.706s")
	if !ok {
		t.Fatalf("parseReprocessOutput did not match portuguese summary")
	}
	if processed != 31832 || failed != 0 {
		t.Fatalf("parseReprocessOutput = processed %d failed %d, want 31832/0", processed, failed)
	}

	if _, _, ok := parseReprocessOutput("processor started without final summary"); ok {
		t.Fatalf("parseReprocessOutput matched unrelated output")
	}
}

func TestRunHTTPWorkerRecordsStatusLatencyAndErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/ok":
			_, _ = w.Write([]byte("ok"))
		case "/fail":
			http.Error(w, "boom", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	metrics := newHTTPMetrics()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()

	err := runHTTPWorker(ctx, server.Client(), server.URL, []endpointSpec{
		{Path: "/ok"},
		{Path: "/fail"},
	}, time.Millisecond, metrics)
	if err != nil {
		t.Fatalf("runHTTPWorker returned error: %v", err)
	}

	snapshot := metrics.snapshot()
	if snapshot.Requests == 0 {
		t.Fatalf("requests = 0")
	}
	if snapshot.StatusCodes[http.StatusOK] == 0 {
		t.Fatalf("status 200 count = 0, statuses=%v", snapshot.StatusCodes)
	}
	if snapshot.StatusCodes[http.StatusInternalServerError] == 0 {
		t.Fatalf("status 500 count = 0, statuses=%v", snapshot.StatusCodes)
	}
	if snapshot.Errors == 0 {
		t.Fatalf("errors = 0, want failed HTTP status counted as error")
	}
	if snapshot.P50 <= 0 || snapshot.P95 <= 0 || snapshot.Max <= 0 {
		t.Fatalf("latencies not recorded: p50=%s p95=%s max=%s", snapshot.P50, snapshot.P95, snapshot.Max)
	}
}

func TestSummarizeScenarioReportsBaselineDelta(t *testing.T) {
	baseline := scenarioResult{
		Name: "baseline",
		Reprocess: reprocessMetrics{
			Duration:   120 * time.Second,
			Processed:  31832,
			Failed:     0,
			Throughput: 265.3,
			RSSMaxKB:   135 * 1024,
		},
	}
	result := scenarioResult{
		Name: "heavy",
		Reprocess: reprocessMetrics{
			Duration:   150 * time.Second,
			Processed:  31832,
			Failed:     0,
			Throughput: 212.2,
			RSSMaxKB:   142 * 1024,
		},
		HTTP: httpMetricsSnapshot{
			Requests:        1250,
			Errors:          3,
			P50:             12 * time.Millisecond,
			P95:             80 * time.Millisecond,
			Max:             220 * time.Millisecond,
			SlowestEndpoint: "/api/messages",
			StatusCodes:     map[int]int{http.StatusOK: 1247, http.StatusInternalServerError: 3},
		},
	}

	out := summarizeScenario(result, &baseline)
	for _, want := range []string{
		"Scenario: heavy",
		"Reprocess duration: 2m30s",
		"Processed: 31832",
		"Failures: 0",
		"Throughput: 212.2 msg/s",
		"HTTP requests: 1250",
		"HTTP errors: 3",
		"HTTP p50: 12ms",
		"HTTP p95: 80ms",
		"HTTP max: 220ms",
		"Slowest endpoint: /api/messages",
		"RSS max: 142 MB",
		"Conclusion: heavy dashboard load added +25.0% wall time vs baseline",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("summary missing %q:\n%s", want, out)
		}
	}
}

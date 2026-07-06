package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/limiar/collector/internal/dashboard"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/media"
	"github.com/limiar/collector/internal/processor"
	"github.com/limiar/collector/internal/storage"
)

const (
	defaultBin          = "./limiar"
	defaultDBPath       = "./limiar.db"
	defaultDashboardURL = "http://127.0.0.1:8080"
	defaultScenario     = "all"
	defaultMode         = "in-process"
	defaultConcurrency  = 1
	defaultDuration     = 120 * time.Second
	defaultInterval     = 250 * time.Millisecond
	defaultTimeout      = 2 * time.Second
	failureThreshold    = 0.05
	reprocessBatchSize  = 100
)

type config struct {
	bin          string
	dbPath       string
	dashboardURL string
	scenario     string
	mode         string
	concurrency  int
	duration     time.Duration
	interval     time.Duration
	timeout      time.Duration
	dryRun       bool
}

type endpointSpec struct {
	Path      string
	PageLimit int
}

type reprocessMetrics struct {
	Duration   time.Duration
	Processed  int
	Failed     int
	Throughput float64
	RSSMaxKB   int64
}

type scenarioResult struct {
	Name      string
	Reprocess reprocessMetrics
	HTTP      httpMetricsSnapshot
}

type httpMetrics struct {
	mu              sync.Mutex
	requests        int
	errors          int
	statusCodes     map[int]int
	latencies       []time.Duration
	endpointMax     map[string]time.Duration
	slowestEndpoint string
}

type httpMetricsSnapshot struct {
	Requests        int
	Errors          int
	StatusCodes     map[int]int
	P50             time.Duration
	P95             time.Duration
	Max             time.Duration
	SlowestEndpoint string
}

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if wantsHelp(args) {
		_, _ = fmt.Fprint(stdout, helpText())
		return nil
	}

	cfg, err := parseConfig(args)
	if err != nil {
		_, _ = fmt.Fprint(stderr, helpText())
		return err
	}

	planned := scenariosFor(cfg.scenario)
	if cfg.dryRun {
		printDryRun(stdout, cfg, planned)
		return nil
	}

	switch cfg.mode {
	case "in-process":
		return runInProcess(context.Background(), stdout, cfg, planned)
	case "external":
		return runExternal(context.Background(), stdout, cfg, planned)
	default:
		return fmt.Errorf("mode inválido %q", cfg.mode)
	}
}

type reprocessRunner func(context.Context, config) (reprocessMetrics, error)

func runExternal(ctx context.Context, stdout io.Writer, cfg config, planned []string) error {
	if cfg.scenario != "baseline" {
		if err := waitDashboard(ctx, cfg); err != nil {
			return err
		}
	}
	return runScenarioSet(ctx, stdout, cfg, planned, runReprocessExternal)
}

func runInProcess(ctx context.Context, stdout io.Writer, cfg config, planned []string) error {
	db, err := storage.Open(ctx, cfg.dbPath, logger.NopLogger{})
	if err != nil {
		return friendlyOpenError(cfg.dbPath, err.Error())
	}
	defer func() { _ = db.Close() }()

	collectorRepo, err := storage.NewRepository(db.DB())
	if err != nil {
		return fmt.Errorf("benchmark: criar collector repository: %w", err)
	}
	defer func() { _ = collectorRepo.Close() }()

	procRepo, err := storage.NewProcessorRepository(db.DB())
	if err != nil {
		return fmt.Errorf("benchmark: criar processor repository: %w", err)
	}
	defer func() { _ = procRepo.Close() }()

	var stopDashboard context.CancelFunc
	defer func() {
		if stopDashboard != nil {
			stopDashboard()
		}
	}()

	startDashboard := func() error {
		if stopDashboard != nil {
			return nil
		}
		stop, err := startInProcessDashboard(ctx, cfg, collectorRepo, procRepo)
		if err != nil {
			return err
		}
		stopDashboard = stop
		return nil
	}

	runner := func(ctx context.Context, cfg config) (reprocessMetrics, error) {
		return runReprocessInProcess(ctx, cfg, collectorRepo, procRepo)
	}

	return runScenarioSetWithHook(ctx, stdout, cfg, planned, runner, startDashboard)
}

func runScenarioSet(ctx context.Context, stdout io.Writer, cfg config, planned []string, runner reprocessRunner) error {
	return runScenarioSetWithHook(ctx, stdout, cfg, planned, runner, nil)
}

func runScenarioSetWithHook(
	ctx context.Context,
	stdout io.Writer,
	cfg config,
	planned []string,
	runner reprocessRunner,
	beforeDashboardScenario func() error,
) error {
	var baseline *scenarioResult
	results := make([]scenarioResult, 0, len(planned))
	for _, name := range planned {
		if name != "baseline" && beforeDashboardScenario != nil {
			if err := beforeDashboardScenario(); err != nil {
				return err
			}
		}
		result, err := runScenario(ctx, cfg, name, runner)
		if err != nil {
			return err
		}
		_, _ = fmt.Fprintln(stdout, summarizeScenario(result, baseline))
		results = append(results, result)
		if name == "baseline" {
			copyResult := result
			baseline = &copyResult
		}
	}

	_, _ = fmt.Fprint(stdout, summarizeTable(results, baseline))
	return nil
}

func parseConfig(args []string) (config, error) {
	cfg := config{
		bin:          defaultBin,
		dbPath:       defaultDBPath,
		dashboardURL: defaultDashboardURL,
		scenario:     defaultScenario,
		mode:         defaultMode,
		concurrency:  defaultConcurrency,
		duration:     defaultDuration,
		interval:     defaultInterval,
		timeout:      defaultTimeout,
	}

	fs := flag.NewFlagSet("bench-dashboard-contention", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&cfg.bin, "bin", cfg.bin, "caminho do binário limiar")
	fs.StringVar(&cfg.dbPath, "db", cfg.dbPath, "caminho do banco limiar.db")
	fs.StringVar(&cfg.dashboardURL, "dashboard-url", cfg.dashboardURL, "URL base do dashboard já iniciado")
	fs.StringVar(&cfg.mode, "mode", cfg.mode, "modo: in-process|external")
	fs.StringVar(&cfg.scenario, "scenario", cfg.scenario, "cenário: baseline|light|heavy|stats|all")
	fs.IntVar(&cfg.concurrency, "concurrency", cfg.concurrency, "número de workers HTTP")
	fs.DurationVar(&cfg.duration, "duration", cfg.duration, "duração máxima da carga HTTP")
	fs.DurationVar(&cfg.interval, "interval", cfg.interval, "intervalo entre requests por worker")
	fs.DurationVar(&cfg.timeout, "timeout", cfg.timeout, "timeout por request HTTP")
	fs.BoolVar(&cfg.dryRun, "dry-run", false, "imprime o plano sem executar reprocessamento ou HTTP")

	if err := fs.Parse(args); err != nil {
		return config{}, err
	}
	if fs.NArg() != 0 {
		return config{}, fmt.Errorf("argumentos posicionais não suportados: %s", strings.Join(fs.Args(), " "))
	}
	if !validScenario(cfg.scenario) {
		return config{}, fmt.Errorf("scenario inválido %q", cfg.scenario)
	}
	if cfg.mode != "in-process" && cfg.mode != "external" {
		return config{}, fmt.Errorf("mode inválido %q", cfg.mode)
	}
	if cfg.bin == "" {
		return config{}, fmt.Errorf("bin não pode ficar vazio")
	}
	if cfg.dbPath == "" {
		return config{}, fmt.Errorf("db não pode ficar vazio")
	}
	if cfg.dashboardURL == "" {
		return config{}, fmt.Errorf("dashboard-url não pode ficar vazio")
	}
	if _, err := url.ParseRequestURI(cfg.dashboardURL); err != nil {
		return config{}, fmt.Errorf("dashboard-url inválida: %w", err)
	}
	if cfg.concurrency <= 0 {
		return config{}, fmt.Errorf("concurrency deve ser maior que zero")
	}
	if cfg.duration <= 0 {
		return config{}, fmt.Errorf("duration deve ser maior que zero")
	}
	if cfg.interval <= 0 {
		return config{}, fmt.Errorf("interval deve ser maior que zero")
	}
	if cfg.timeout <= 0 {
		return config{}, fmt.Errorf("timeout deve ser maior que zero")
	}
	return cfg, nil
}

func helpText() string {
	return `Benchmark de contenção do dashboard embutido.

Uso:
  go run ./tools/bench-dashboard-contention [flags]

Modo padrão: --mode in-process. A ferramenta abre o banco uma vez, inicia o
dashboard dentro do próprio processo quando necessário, e roda o reprocessamento
no mesmo processo. Isso evita o erro de lock do Tursogo: o driver não permite
múltiplos processos abrindo o mesmo arquivo .db.

Use --mode external apenas se o dashboard passado em --dashboard-url NÃO mantiver
este mesmo banco aberto. Com o limiar já rodando contra o mesmo limiar.db, o modo
external falha por lock do Tursogo.

Flags:
  --bin ./limiar                         caminho do binário limiar
  --db ./limiar.db                       caminho do banco; repassado como LIMIAR_DB_PATH
  --dashboard-url http://127.0.0.1:8080  URL base do dashboard já iniciado
  --mode in-process                    in-process|external
  --scenario baseline|healthz|healthz-lite|channels|light|heavy|stats|all
  --concurrency 1                        número de workers HTTP
  --duration 120s                        duração máxima da carga HTTP
  --interval 250ms                       intervalo entre requests por worker
  --timeout 2s                           timeout por request HTTP
  --dry-run                              imprime cenários, endpoints e comando sem executar
  --help                                 mostra esta ajuda

Cenários:
  baseline  reprocessamento sem carga HTTP
  healthz   /healthz isolado
  healthz-lite /healthz-lite isolado sem consulta ao banco
  channels  /api/channels isolado
  light     /healthz, /api/processed/stats, /api/channels
  heavy     /api/messages?limit=100&offset=N, /api/processed?limit=100&offset=N
  stats     /api/processed/stats em loop; endpoint caro existe hoje no dashboard
  all       baseline, light, heavy e stats nessa ordem

Importante:
  Pare qualquer limiar run, limiar processor run ou limiar dashboard que esteja
  usando o mesmo --db antes de usar o modo in-process.
`
}

func wantsHelp(args []string) bool {
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			return true
		}
	}
	return false
}

func validScenario(name string) bool {
	switch name {
	case "baseline", "healthz", "healthz-lite", "channels", "light", "heavy", "stats", "all":
		return true
	default:
		return false
	}
}

func scenariosFor(name string) []string {
	if name == "all" {
		return []string{"baseline", "light", "heavy", "stats"}
	}
	return []string{name}
}

func printDryRun(w io.Writer, cfg config, scenarios []string) {
	_, _ = fmt.Fprintf(w, "Dry run: dashboard contention benchmark\n")
	_, _ = fmt.Fprintf(w, "Mode: %s\n", cfg.mode)
	if cfg.mode == "external" {
		_, _ = fmt.Fprintf(w, "Command: LIMIAR_DB_PATH=%s %s processor reprocess --all\n", cfg.dbPath, cfg.bin)
	} else {
		fmt.Fprintf(w, "Command: in-process reprocess using %s\n", cfg.dbPath)
	}
	fmt.Fprintf(w, "Dashboard URL: %s\n", cfg.dashboardURL)
	fmt.Fprintf(w, "Concurrency: %d\nDuration: %s\nInterval: %s\nTimeout: %s\n", cfg.concurrency, cfg.duration, cfg.interval, cfg.timeout)
	for _, scenario := range scenarios {
		fmt.Fprintf(w, "Scenario %s endpoints: %s\n", scenario, endpointList(endpointSpecs(scenario)))
	}
}

func endpointList(endpoints []endpointSpec) string {
	if len(endpoints) == 0 {
		return "none"
	}
	parts := make([]string, 0, len(endpoints))
	for _, endpoint := range endpoints {
		parts = append(parts, endpoint.Path)
	}
	return strings.Join(parts, ", ")
}

func startInProcessDashboard(
	ctx context.Context,
	cfg config,
	collectorRepo *storage.Repository,
	procRepo *storage.ProcessorRepository,
) (context.CancelFunc, error) {
	port, err := dashboardPort(cfg.dashboardURL)
	if err != nil {
		return nil, err
	}

	serverCtx, stop := context.WithCancel(ctx)
	imageCache := media.NewCache(500, 30*time.Minute)
	mediaResolver := media.NewResolver(procRepo, imageCache, nil, logger.NopLogger{})
	srv := dashboard.NewServer(collectorRepo, procRepo, mediaResolver, logger.NopLogger{}, port, nil)

	errCh := make(chan error, 1)
	go func() {
		errCh <- srv.ListenAndServe(serverCtx)
	}()

	deadline := time.Now().Add(cfg.timeout)
	for {
		select {
		case err := <-errCh:
			stop()
			return nil, fmt.Errorf("dashboard in-process encerrou antes de ficar pronto: %w", err)
		default:
		}
		if err := waitDashboard(ctx, cfg); err == nil {
			return stop, nil
		}
		if time.Now().After(deadline) {
			stop()
			return nil, fmt.Errorf("dashboard in-process não ficou pronto em %s", cfg.timeout)
		}
		if !sleepWithContext(ctx, 50*time.Millisecond) {
			stop()
			return nil, ctx.Err()
		}
	}
}

func dashboardPort(rawURL string) (int, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return 0, fmt.Errorf("dashboard-url inválida: %w", err)
	}
	if u.Scheme != "http" {
		return 0, fmt.Errorf("dashboard-url deve usar http no modo in-process")
	}
	portText := u.Port()
	if portText == "" {
		return 0, fmt.Errorf("dashboard-url deve incluir porta explícita no modo in-process")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("porta inválida em dashboard-url: %q", portText)
	}
	return port, nil
}

func friendlyOpenError(dbPath string, errText string) error {
	if isTursoLockError(errText) {
		return fmt.Errorf("banco %s está bloqueado: Tursogo não permite múltiplos processos abrindo o mesmo .db; pare o limiar que já está rodando nesse banco e execute o benchmark sozinho: %s", dbPath, errText)
	}
	return fmt.Errorf("abrir banco %s: %s", dbPath, errText)
}

func friendlyReprocessError(dbPath string, errText string) error {
	if isTursoLockError(errText) {
		return fmt.Errorf("reprocessamento externo falhou porque o banco %s está bloqueado: Tursogo não permite múltiplos processos no mesmo .db; use o modo padrão --mode in-process e pare o limiar que já está rodando nesse banco, ou use --mode external apenas com outro banco/dashboard: %s", dbPath, errText)
	}
	return fmt.Errorf("%s", errText)
}

func isTursoLockError(text string) bool {
	lower := strings.ToLower(text)
	return strings.Contains(lower, "locking error") ||
		strings.Contains(lower, "file is locked by another process") ||
		strings.Contains(lower, "failed locking file")
}

func waitDashboard(ctx context.Context, cfg config) error {
	client := &http.Client{Timeout: cfg.timeout}
	requestURL := strings.TrimRight(cfg.dashboardURL, "/") + "/healthz"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return fmt.Errorf("dashboard readiness: criar request: %w", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("dashboard não respondeu em %s: %w", requestURL, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= http.StatusBadRequest {
		return fmt.Errorf("dashboard readiness %s retornou status %d", requestURL, resp.StatusCode)
	}
	return nil
}

func runScenario(ctx context.Context, cfg config, name string, runner reprocessRunner) (scenarioResult, error) {
	result := scenarioResult{Name: name}
	endpoints := endpointSpecs(name)
	client := &http.Client{Timeout: cfg.timeout}
	metrics := newHTTPMetrics()

	loadCtx, stopLoad := context.WithCancel(ctx)
	var wg sync.WaitGroup
	if len(endpoints) > 0 {
		loadCtx, stopLoad = context.WithTimeout(ctx, cfg.duration)
		for i := range cfg.concurrency {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				_ = runHTTPWorker(loadCtx, client, cfg.dashboardURL, endpoints, cfg.interval, metrics)
			}(i)
		}
	}

	reprocess, err := runner(ctx, cfg)
	stopLoad()
	wg.Wait()
	result.Reprocess = reprocess
	result.HTTP = metrics.snapshot()

	if err != nil {
		return result, err
	}
	if result.Reprocess.Failed > 0 {
		return result, fmt.Errorf("scenario %s: reprocessamento teve %d falhas", name, result.Reprocess.Failed)
	}
	if result.HTTP.Requests > 0 {
		ratio := float64(result.HTTP.Errors) / float64(result.HTTP.Requests)
		if ratio > failureThreshold {
			return result, fmt.Errorf("scenario %s: taxa de erro HTTP %.1f%% excedeu 5%%", name, ratio*100)
		}
	}
	return result, nil
}

func endpointSpecs(name string) []endpointSpec {
	switch name {
	case "healthz":
		return []endpointSpec{{Path: "/healthz"}}
	case "healthz-lite":
		return []endpointSpec{{Path: "/healthz-lite"}}
	case "channels":
		return []endpointSpec{{Path: "/api/channels"}}
	case "light":
		return []endpointSpec{
			{Path: "/healthz"},
			{Path: "/api/processed/stats"},
			{Path: "/api/channels"},
		}
	case "heavy":
		return []endpointSpec{
			{Path: "/api/messages", PageLimit: 100},
			{Path: "/api/processed", PageLimit: 100},
		}
	case "stats":
		return []endpointSpec{{Path: "/api/processed/stats"}}
	default:
		return nil
	}
}

func runReprocessExternal(ctx context.Context, cfg config) (reprocessMetrics, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer

	cmd := exec.CommandContext(ctx, cfg.bin, "processor", "reprocess", "--all")
	cmd.Env = append(os.Environ(), "LIMIAR_DB_PATH="+cfg.dbPath)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return reprocessMetrics{}, fmt.Errorf("reprocess: iniciar comando: %w", err)
	}

	stopRSS := make(chan struct{})
	rssDone := make(chan int64, 1)
	go sampleRSS(cmd.Process.Pid, stopRSS, rssDone)

	err := cmd.Wait()
	close(stopRSS)
	rssMaxKB := <-rssDone
	duration := time.Since(start)

	output := stdout.String() + "\n" + stderr.String()
	processed, failed, ok := parseReprocessOutput(output)
	if !ok && err == nil {
		return reprocessMetrics{Duration: duration, RSSMaxKB: rssMaxKB}, fmt.Errorf("reprocess: resumo final não encontrado na saída")
	}
	metrics := reprocessMetrics{
		Duration:  duration,
		Processed: processed,
		Failed:    failed,
		RSSMaxKB:  rssMaxKB,
	}
	if duration > 0 && processed > 0 {
		metrics.Throughput = float64(processed) / duration.Seconds()
	}
	if err != nil {
		msg := fmt.Sprintf("reprocess: comando falhou: %v\n%s", err, trimOutput(output))
		return metrics, friendlyReprocessError(cfg.dbPath, msg)
	}
	return metrics, nil
}

func runReprocessInProcess(
	ctx context.Context,
	cfg config,
	collectorRepo *storage.Repository,
	procRepo *storage.ProcessorRepository,
) (reprocessMetrics, error) {
	start := time.Now()
	stopRSS := make(chan struct{})
	rssDone := make(chan int64, 1)
	go sampleRSS(os.Getpid(), stopRSS, rssDone)

	processed, failed, err := reprocessAllInProcess(ctx, collectorRepo, procRepo)
	close(stopRSS)
	rssMaxKB := <-rssDone
	duration := time.Since(start)

	metrics := reprocessMetrics{
		Duration:  duration,
		Processed: processed,
		Failed:    failed,
		RSSMaxKB:  rssMaxKB,
	}
	if duration > 0 && processed > 0 {
		metrics.Throughput = float64(processed) / duration.Seconds()
	}
	if err != nil {
		return metrics, fmt.Errorf("reprocess in-process: %w", err)
	}
	return metrics, nil
}

func reprocessAllInProcess(
	ctx context.Context,
	collectorRepo *storage.Repository,
	procRepo *storage.ProcessorRepository,
) (totalProcessed int, totalFailed int, err error) {
	offset := 0
	for {
		if ctx.Err() != nil {
			return totalProcessed, totalFailed, nil
		}
		msgs, err := collectorRepo.ListMessages(ctx, 0, reprocessBatchSize, offset)
		if err != nil {
			return totalProcessed, totalFailed, err
		}
		if len(msgs) == 0 {
			break
		}

		normalized := make([]*processor.NormalizedMessage, 0, len(msgs))
		for _, raw := range msgs {
			nm, err := processor.Normalize(raw)
			if err != nil {
				totalFailed++
				continue
			}
			nm.MessageType = string(processor.Classify(nm))
			nm.IsPromotional = processor.IsPromotionalMessageType(nm.MessageType)
			normalized = append(normalized, nm)
		}

		saved, saveFailed, err := procRepo.SaveProcessedBatch(ctx, normalized)
		if err != nil {
			return totalProcessed, totalFailed, err
		}
		totalProcessed += saved
		totalFailed += saveFailed

		offset += len(msgs)
		if len(msgs) < reprocessBatchSize {
			break
		}
	}
	return totalProcessed, totalFailed, nil
}

var reprocessSummaryRE = regexp.MustCompile(`(?m)(\d+)\s+processadas,\s+(\d+)\s+falharam`)

func parseReprocessOutput(text string) (processed int, failed int, ok bool) {
	match := reprocessSummaryRE.FindStringSubmatch(text)
	if len(match) != 3 {
		return 0, 0, false
	}
	processed, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, 0, false
	}
	failed, err = strconv.Atoi(match[2])
	if err != nil {
		return 0, 0, false
	}
	return processed, failed, true
}

func trimOutput(output string) string {
	output = strings.TrimSpace(output)
	if len(output) <= 4000 {
		return output
	}
	return output[len(output)-4000:]
}

func sampleRSS(pid int, stop <-chan struct{}, done chan<- int64) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	var maxRSS int64
	for {
		if rss, err := readRSSKB(pid); err == nil && rss > maxRSS {
			maxRSS = rss
		}
		select {
		case <-stop:
			done <- maxRSS
			return
		case <-ticker.C:
		}
	}
}

func readRSSKB(pid int) (int64, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return 0, err
	}
	for _, line := range strings.Split(string(data), "\n") {
		if !strings.HasPrefix(line, "VmRSS:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			return 0, fmt.Errorf("rss inválido: %q", line)
		}
		return strconv.ParseInt(fields[1], 10, 64)
	}
	return 0, fmt.Errorf("VmRSS não encontrado")
}

func newHTTPMetrics() *httpMetrics {
	return &httpMetrics{
		statusCodes: make(map[int]int),
		latencies:   []time.Duration{},
		endpointMax: make(map[string]time.Duration),
	}
}

func runHTTPWorker(ctx context.Context, client *http.Client, baseURL string, endpoints []endpointSpec, interval time.Duration, metrics *httpMetrics) error {
	if len(endpoints) == 0 {
		return nil
	}
	baseURL = strings.TrimRight(baseURL, "/")
	var requestIndex int
	for {
		for _, endpoint := range endpoints {
			select {
			case <-ctx.Done():
				return nil
			default:
			}
			requestPath := endpoint.requestPath(requestIndex)
			start := time.Now()
			statusCode, err := doGET(ctx, client, baseURL+requestPath)
			metrics.record(requestPathWithoutOffset(requestPath), statusCode, time.Since(start), err)
			requestIndex++

			if !sleepWithContext(ctx, interval) {
				return nil
			}
		}
	}
}

func (e endpointSpec) requestPath(requestIndex int) string {
	if e.PageLimit <= 0 {
		return e.Path
	}
	offset := (requestIndex * e.PageLimit) % 10000
	separator := "?"
	if strings.Contains(e.Path, "?") {
		separator = "&"
	}
	return fmt.Sprintf("%s%slimit=%d&offset=%d", e.Path, separator, e.PageLimit, offset)
}

func requestPathWithoutOffset(path string) string {
	if i := strings.IndexByte(path, '?'); i >= 0 {
		return path[:i]
	}
	return path
}

func doGET(ctx context.Context, client *http.Client, requestURL string) (int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, requestURL, nil)
	if err != nil {
		return 0, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)
	if resp.StatusCode >= http.StatusBadRequest {
		return resp.StatusCode, fmt.Errorf("http status %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
}

func sleepWithContext(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

func (m *httpMetrics) record(endpoint string, statusCode int, latency time.Duration, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.requests++
	if statusCode > 0 {
		m.statusCodes[statusCode]++
	}
	if err != nil {
		m.errors++
	}
	m.latencies = append(m.latencies, latency)
	if latency > m.endpointMax[endpoint] {
		m.endpointMax[endpoint] = latency
	}
	if m.slowestEndpoint == "" || latency >= m.endpointMax[m.slowestEndpoint] {
		m.slowestEndpoint = endpoint
	}
}

func (m *httpMetrics) snapshot() httpMetricsSnapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	latencies := append([]time.Duration(nil), m.latencies...)
	sort.Slice(latencies, func(i, j int) bool { return latencies[i] < latencies[j] })
	statusCodes := make(map[int]int, len(m.statusCodes))
	for code, count := range m.statusCodes {
		statusCodes[code] = count
	}
	return httpMetricsSnapshot{
		Requests:        m.requests,
		Errors:          m.errors,
		StatusCodes:     statusCodes,
		P50:             percentile(latencies, 0.50),
		P95:             percentile(latencies, 0.95),
		Max:             percentile(latencies, 1.0),
		SlowestEndpoint: m.slowestEndpoint,
	}
}

func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	if p >= 1 {
		return sorted[len(sorted)-1]
	}
	idx := int(float64(len(sorted)-1) * p)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
}

func summarizeScenario(result scenarioResult, baseline *scenarioResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Scenario: %s\n", result.Name)
	fmt.Fprintf(&b, "Reprocess duration: %s\n", result.Reprocess.Duration.Round(time.Millisecond))
	fmt.Fprintf(&b, "Processed: %d\n", result.Reprocess.Processed)
	fmt.Fprintf(&b, "Failures: %d\n", result.Reprocess.Failed)
	fmt.Fprintf(&b, "Throughput: %.1f msg/s\n", result.Reprocess.Throughput)
	fmt.Fprintf(&b, "HTTP requests: %d\n", result.HTTP.Requests)
	fmt.Fprintf(&b, "HTTP errors: %d\n", result.HTTP.Errors)
	fmt.Fprintf(&b, "HTTP p50: %s\n", result.HTTP.P50.Round(time.Millisecond))
	fmt.Fprintf(&b, "HTTP p95: %s\n", result.HTTP.P95.Round(time.Millisecond))
	fmt.Fprintf(&b, "HTTP max: %s\n", result.HTTP.Max.Round(time.Millisecond))
	fmt.Fprintf(&b, "Slowest endpoint: %s\n", valueOrUnknown(result.HTTP.SlowestEndpoint))
	fmt.Fprintf(&b, "RSS max: %s\n", formatRSS(result.Reprocess.RSSMaxKB))
	fmt.Fprintf(&b, "Status codes: %s\n", formatStatusCodes(result.HTTP.StatusCodes))
	fmt.Fprintf(&b, "Conclusion: %s\n\n", conclusion(result, baseline))
	return b.String()
}

func conclusion(result scenarioResult, baseline *scenarioResult) string {
	if baseline == nil || baseline.Reprocess.Duration <= 0 || result.Name == "baseline" {
		return "baseline registrado para comparação"
	}
	delta := (float64(result.Reprocess.Duration) - float64(baseline.Reprocess.Duration)) / float64(baseline.Reprocess.Duration) * 100
	return fmt.Sprintf("%s dashboard load added %+0.1f%% wall time vs baseline", result.Name, delta)
}

func summarizeTable(results []scenarioResult, baseline *scenarioResult) string {
	var b strings.Builder
	fmt.Fprintln(&b, "Final table:")
	fmt.Fprintln(&b, "Scenario | Reprocess | Processed | Failures | Throughput | HTTP req | HTTP err | HTTP p50 | HTTP p95 | RSS max | Delta")
	fmt.Fprintln(&b, "--- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---:")
	for _, result := range results {
		delta := "n/a"
		if baseline != nil && baseline.Reprocess.Duration > 0 && result.Name != "baseline" {
			pct := (float64(result.Reprocess.Duration) - float64(baseline.Reprocess.Duration)) / float64(baseline.Reprocess.Duration) * 100
			delta = fmt.Sprintf("%+.1f%%", pct)
		}
		fmt.Fprintf(&b, "%s | %s | %d | %d | %.1f msg/s | %d | %d | %s | %s | %s | %s\n",
			result.Name,
			result.Reprocess.Duration.Round(time.Millisecond),
			result.Reprocess.Processed,
			result.Reprocess.Failed,
			result.Reprocess.Throughput,
			result.HTTP.Requests,
			result.HTTP.Errors,
			result.HTTP.P50.Round(time.Millisecond),
			result.HTTP.P95.Round(time.Millisecond),
			formatRSS(result.Reprocess.RSSMaxKB),
			delta)
	}
	return b.String()
}

func valueOrUnknown(value string) string {
	if value == "" {
		return "unknown"
	}
	return value
}

func formatRSS(kb int64) string {
	if kb <= 0 {
		return "unknown"
	}
	return fmt.Sprintf("%d MB", (kb+1023)/1024)
}

func formatStatusCodes(statusCodes map[int]int) string {
	if len(statusCodes) == 0 {
		return "none"
	}
	codes := make([]int, 0, len(statusCodes))
	for code := range statusCodes {
		codes = append(codes, code)
	}
	sort.Ints(codes)
	parts := make([]string, 0, len(codes))
	for _, code := range codes {
		parts = append(parts, fmt.Sprintf("%d=%d", code, statusCodes[code]))
	}
	return strings.Join(parts, ",")
}

// Command limiar é o orquestrador unificado do pipeline Limiar. Este binário
// é o ponto de entrada de produção: compõe Collector, Processor e Dashboard
// como goroutines dentro de um único processo, compartilhando um *sql.DB.
//
// Os binários standalone (limiar-collector, limiar-processor) permanecem para
// desenvolvimento e debug — não representam o modo operacional de produção.
//
// Este arquivo é a raiz de composição (composition root): é o ÚNICO lugar
// onde as dependências concretas são construídas e conectadas.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/viper"
	"golang.org/x/sync/errgroup"

	"github.com/limiar/collector/internal/collector"
	"github.com/limiar/collector/internal/config"
	"github.com/limiar/collector/internal/dashboard"
	"github.com/limiar/collector/internal/id"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/processor"
	"github.com/limiar/collector/internal/model"
	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "limiar:", err)
		os.Exit(1)
	}
}

func run() error {
	// --- Configuração ---
	cfg, err := config.Load(viper.New())
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	procCfg, err := processor.LoadConfig(viper.New())
	if err != nil {
		return err
	}
	if err := procCfg.Validate(); err != nil {
		return err
	}

	// --- Logger ---
	runID := id.NewRunID()
	format := logger.ResolveFormat(cfg.LogFormat, logger.IsTerminalWriter(os.Stdout))
	log := logger.NewSlogLogger(os.Stdout, logger.ParseLevel(cfg.LogLevel), format).
		With("run_id", runID, "service", "limiar", "pipeline_stage", "orchestrator")
	presenter := logger.NewTerminalPresenter(os.Stderr,
		logger.IsTerminalWriter(os.Stderr) && !logger.NoColorEnvSet())

	// --- Sinal de desligamento ---
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	// --- Banco (único *sql.DB para todo o pipeline) ---
	db, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	// --- Repositories ---
	collectorRepo, err := storage.NewRepository(db.DB())
	if err != nil {
		return fmt.Errorf("limiar: criar collector repository: %w", err)
	}
	defer func() { _ = collectorRepo.Close() }()

	procRepo, err := processor.NewRepository(db.DB())
	if err != nil {
		return fmt.Errorf("limiar: criar processor repository: %w", err)
	}
	defer func() { _ = procRepo.Close() }()

	// --- Componentes ---
	// Collector
	session := telegram.NewTursoSessionStorage(collectorRepo, log.WithComponent("session"))
	peers := telegram.NewPeerStore(collectorRepo, log.WithComponent("peers"))
	dispatcher := telegram.NewDispatcher(cfg.DispatcherBufferSize, log.WithComponent("dispatcher"))
	backoff := telegram.DefaultBackoff(cfg.MaxRetries)
	client := telegram.NewClient(
		cfg.AppID, cfg.APIHash, cfg.IOTimeout, backoff,
		session, peers, dispatcher, log.WithComponent("telegram"),
	)
	col := collector.NewCollector(
		client, collectorRepo, collector.NoopClassifier{}, log.WithComponent("collector"),
		cfg.DBWriterBufferSize, cfg.MaxRetries, cfg.HistoryMax, cfg.HistoryMaxDays,
	)

	// Processor
	proc := processor.NewProcessor(procRepo, procCfg, log.WithComponent("processor"))

	// Dashboard (SSE broker para mensagens em tempo real do collector)
	broker := dashboard.NewBroker()
	dashRepo, err := storage.NewRepository(db.DB())
	if err != nil {
		return fmt.Errorf("limiar: criar dashboard repository: %w", err)
	}
	defer func() { _ = dashRepo.Close() }()
	srv := dashboard.NewServer(dashRepo, log.WithComponent("dashboard"), 8080, broker)

	col.SetOnMessage(func(msg *model.RawMessage) {
		data, err := json.Marshal(msg)
		if err != nil {
			return
		}
		broker.Publish(dashboard.Event{Type: "message", Data: data})
	})

	// --- Goroutines via errgroup ---
	// errgroup.WithContext: se qualquer goroutine retornar erro, o contexto
	// derivado é cancelado, sinalizando as demais para encerrar.
	g, gctx := errgroup.WithContext(ctx)

	log.Info("🚀 Limiar orquestrador iniciado",
		"collector", "ativo",
		"processor", "ativo",
		"dashboard", "http://localhost:8080")
	presenter.Info("Limiar orquestrador iniciado")
	presenter.Step("Dashboard: http://localhost:8080")
	presenter.Step("Pressione Ctrl+C para encerrar")

	g.Go(func() error {
		return col.Run(gctx)
	})

	g.Go(func() error {
		return proc.Run(gctx)
	})

	g.Go(func() error {
		return srv.ListenAndServe(gctx)
	})

	// g.Wait() bloqueia até todas as goroutines encerrarem.
	// Retorna o primeiro erro não-nil (se houver).
	if err := g.Wait(); err != nil {
		log.Error("🛑 Pipeline encerrou com erro", "erro", err)
		return err
	}

	log.Info("✅ Limiar encerrado gracefulmente")
	return nil
}


// Command limiar-processor é o binário de processamento do pipeline Limiar.
// Ele lê raw_messages do banco compartilhado (limiar.db), normaliza, classifica
// e persiste em processed_messages. Não interage com o Telegram.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/viper"

	"github.com/limiar/collector/internal/dashboard"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/processor"
	"github.com/limiar/collector/internal/storage"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "limiar-processor:", err)
		os.Exit(1)
	}
}

func run() error {
	var (
		withDashboard bool
		dashboardPort int
	)
	flag.BoolVar(&withDashboard, "dashboard", false, "Iniciar dashboard web")
	flag.IntVar(&dashboardPort, "dashboard-port", 8080, "Porta do dashboard")
	flag.Parse()

	cfg, err := processor.LoadConfig(viper.New())
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	format, _ := logger.ResolveFormat("processor", cfg.LogFormat, logger.IsTerminalWriter(os.Stdout))
	log := logger.NewSlogLogger(os.Stdout, logger.ParseLevel(cfg.LogLevel), format).
		With("run_id", generateRunID(), "service", "limiar-processor", "pipeline_stage", "processor")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	db, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("abrir banco: %w", err)
	}

	repo, err := processor.NewRepository(db.DB())
	if err != nil {
		_ = db.Close()
		return fmt.Errorf("criar repository: %w", err)
	}

	proc := processor.NewProcessor(repo, cfg, log.WithComponent("processor"))

	if withDashboard {
		dashRepo, err := storage.NewRepository(db.DB())
		if err != nil {
			_ = repo.Close()
			_ = db.Close()
			return fmt.Errorf("criar dashboard repository: %w", err)
		}
		srv := dashboard.NewServer(dashRepo, log.WithComponent("dashboard"), dashboardPort, nil)
		go func() { _ = srv.ListenAndServe(ctx) }()
		log.Info("🌐 Dashboard disponível", "porta", dashboardPort)
	}

	runErr := make(chan error, 1)
	go func() { runErr <- proc.Run(ctx) }()

	select {
	case err := <-runErr:
		_ = repo.Close()
		_ = db.Close()
		log.Info("✅ Processor encerrado")
		return err
	case <-ctx.Done():
		log.Info("🛑 Sinal de desligamento recebido")
		err := <-runErr
		_ = repo.Close()
		_ = db.Close()
		log.Info("✅ Processor encerrado gracefulmente")
		return err
	}
}

// generateRunID produz 8 bytes aleatórios formatados como hex (16 chars).
func generateRunID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}

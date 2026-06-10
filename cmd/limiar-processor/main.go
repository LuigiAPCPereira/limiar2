// Command limiar-processor é o binário de processamento do pipeline Limiar.
// Ele lê raw_messages do banco compartilhado (limiar.db), normaliza, classifica
// e persiste em processed_messages. Não interage com o Telegram.
package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/viper"

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
	cfg, err := processor.LoadConfig(viper.New())
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	log := logger.NewSlogLogger(os.Stdout, logger.ParseLevel(cfg.LogLevel), cfg.LogFormat)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	db, err := storage.Open(ctx, cfg.DBPath)
	if err != nil {
		return fmt.Errorf("abrir banco: %w", err)
	}

	repo, err := processor.NewRepository(db.Conn())
	if err != nil {
		_ = db.Close()
		return fmt.Errorf("criar repository: %w", err)
	}

	proc := processor.NewProcessor(repo, cfg, log.WithComponent("processor"))

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
		// Run retorna quando ctx é cancelado
		err := <-runErr
		_ = repo.Close()
		_ = db.Close()
		log.Info("✅ Processor encerrado gracefulmente")
		return err
	}
}

// Command limiar-collector is the Phase 1 Telegram collector. This file is the
// composition root: it is the ONLY place where concrete storage, telegram,
// logger, and collector dependencies are constructed and wired together.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/viper"

	"github.com/limiar/collector/internal/cli"
	"github.com/limiar/collector/internal/collector"
	"github.com/limiar/collector/internal/config"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

// provider is the concrete cli.Provider: it owns all dependency construction.
type provider struct {
	cfg *config.Config
}

// Config returns the validated configuration.
func (p *provider) Config() *config.Config { return p.cfg }

// Logger builds a SlogLogger at the configured level in the requested format.
func (p *provider) Logger(format string) logger.Logger {
	return logger.NewSlogLogger(os.Stdout, logger.ParseLevel(p.cfg.LogLevel), format)
}

// OpenStore opens the Tursogo database and returns a Repository with a closer
// that releases prepared statements and the connection.
func (p *provider) OpenStore(ctx context.Context) (*storage.Repository, func() error, error) {
	db, err := storage.Open(ctx, p.cfg.DBPath)
	if err != nil {
		return nil, nil, err
	}
	repo, err := storage.NewRepository(db.Conn())
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	closeFn := func() error {
		repoErr := repo.Close()
		dbErr := db.Close()
		if repoErr != nil {
			return repoErr
		}
		return dbErr
	}
	return repo, closeFn, nil
}

// NewClient assembles the Telegram facade: session storage, peer store, and
// dispatcher, all backed by repo and log.
func (p *provider) NewClient(log logger.Logger, repo *storage.Repository) telegram.TelegramClient {
	session := telegram.NewTursoSessionStorage(repo, log)
	peers := telegram.NewPeerStore(repo, log)
	dispatcher := telegram.NewDispatcher(p.cfg.DispatcherBufferSize, log.WithComponent("dispatcher"))
	backoff := telegram.DefaultBackoff(p.cfg.MaxRetries)
	return telegram.NewClient(
		p.cfg.AppID, p.cfg.APIHash, p.cfg.IOTimeout, backoff,
		session, peers, dispatcher, log.WithComponent("telegram"),
	)
}

// NewCollector assembles the collector with a NoopClassifier (Phase 1).
func (p *provider) NewCollector(client telegram.TelegramClient, repo *storage.Repository, log logger.Logger) *collector.Collector {
	return collector.NewCollector(
		client, repo, collector.NoopClassifier{}, log.WithComponent("collector"),
		p.cfg.DBWriterBufferSize, p.cfg.MaxRetries, p.cfg.HistoryMax, p.cfg.HistoryMaxDays,
	)
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "limiar-collector:", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(viper.New())
	if err != nil {
		return err
	}
	if err := cfg.Validate(); err != nil {
		return err
	}

	root := cli.NewRootCmd(&provider{cfg: cfg})
	return root.ExecuteContext(context.Background())
}

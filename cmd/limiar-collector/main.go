// Command limiar-collector é o coletor do Telegram da Fase 1. Este arquivo é a
// raiz de composição (composition root): é o ÚNICO lugar onde as dependências concretas
// de storage, telegram, logger e collector são construídas e conectadas entre si.
package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
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

// provider é a implementação concreta de cli.Provider: ele possui toda a construção de dependências.
type provider struct {
	cfg *config.Config
}

// Config retorna a configuração validada.
func (p *provider) Config() *config.Config { return p.cfg }

// Logger constrói um SlogLogger no nível configurado e no formato solicitado.
// Vincula automaticamente run_id, service e pipeline_stage para correlação.
func (p *provider) Logger(format string) logger.Logger {
	log := logger.NewSlogLogger(os.Stdout, logger.ParseLevel(p.cfg.LogLevel), format)
	return log.With("run_id", generateRunID(), "service", "limiar-collector", "pipeline_stage", "collector")
}

// Presenter constrói um TerminalPresenter escrevendo em stderr.
// Cores são ativadas quando stderr é TTY e NO_COLOR não está definido.
func (p *provider) Presenter() logger.Presenter {
	useColors := logger.IsTerminalWriter(os.Stderr) && !logger.NoColorEnvSet()
	return logger.NewTerminalPresenter(os.Stderr, useColors)
}

// OpenStore abre o banco de dados Tursogo e retorna um Repository com um fechador (closer)
// que libera as instruções preparadas (prepared statements) e a conexão.
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

// NewClient monta a fachada do Telegram: armazenamento de sessão, armazenamento de peers e
// dispatcher, todos apoiados pelo repositório (repo) e pelo logger (log).
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

// NewCollector monta o coletor com um NoopClassifier (Fase 1).
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

// generateRunID produz 8 bytes aleatórios formatados como hex (16 chars).
// Usa crypto/rand para garantir unicidade entre execuções.
func generateRunID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return "0000000000000000"
	}
	return hex.EncodeToString(b)
}

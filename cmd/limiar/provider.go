package main

import (
	"context"
	"os"
	"time"

	"github.com/limiar/collector/internal/cli"
	"github.com/limiar/collector/internal/collector"
	"github.com/limiar/collector/internal/config"
	"github.com/limiar/collector/internal/id"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/media"
	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

// asserção em tempo de compilação de que *provider satisfaz cli.Provider.
var _ cli.Provider = (*provider)(nil)

// provider é a implementação concreta de cli.Provider: possui toda a
// construção de dependências concretas num único lugar (AGENTS.md §14.5).
type provider struct {
	cfg       *config.Config
	log       logger.Logger
	presenter logger.Presenter
}

// newProvider constrói o provider com logger e presenter já resolvidos.
func newProvider(cfg *config.Config) *provider {
	format := logger.ResolveFormat(cfg.LogFormat, logger.IsTerminalWriter(os.Stdout))
	log := logger.NewSlogLogger(os.Stdout, logger.ParseLevel(cfg.LogLevel), format).
		With("run_id", id.NewRunID(), "service", "limiar", "pipeline_stage", "unified")

	useColors := logger.IsTerminalWriter(os.Stderr) && !logger.NoColorEnvSet()
	presenter := logger.NewTerminalPresenter(os.Stderr, useColors)

	return &provider{cfg: cfg, log: log, presenter: presenter}
}

// Config retorna a configuração unificada validada.
func (p *provider) Config() *config.Config { return p.cfg }

// Logger retorna o logger compartilhado da sessão.
func (p *provider) Logger() logger.Logger { return p.log }

// Presenter retorna o presenter para mensagens ao operador.
func (p *provider) Presenter() logger.Presenter { return p.presenter }

// OpenStore abre o banco de dados Tursogo e retorna um Repository com uma
// função de fechamento que libera prepared statements e a conexão.
func (p *provider) OpenStore(ctx context.Context) (*storage.Repository, func() error, error) {
	db, err := storage.Open(ctx, p.cfg.DBPath, p.log.WithComponent("storage"))
	if err != nil {
		return nil, nil, err
	}
	repo, err := storage.NewRepository(db.DB())
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

// newTelegramClient monta a facade *telegram.Client. Construção concreta
// num único lugar (AGENTS.md §14.5).
func (p *provider) newTelegramClient(repo *storage.Repository) *telegram.Client {
	log := p.log
	session := telegram.NewTursoSessionStorage(repo, log.WithComponent("session"))
	peers := telegram.NewPeerStore(repo, log.WithComponent("peers"))
	dispatcher := telegram.NewDispatcher(p.cfg.DispatcherBufferSize, log.WithComponent("dispatcher"))
	backoff := telegram.DefaultBackoff(p.cfg.MaxRetries)
	return telegram.NewClient(
		p.cfg.AppID, p.cfg.APIHash, p.cfg.IOTimeout, backoff,
		session, peers, dispatcher, log.WithComponent("telegram"),
	)
}

// NewTelegramClient constrói a fachada do Telegram vinculada ao repositório.
func (p *provider) NewTelegramClient(repo *storage.Repository) telegram.TelegramClient {
	return p.newTelegramClient(repo)
}

// NewMediaClient envolve a facade telegram.Client em telegram.MediaClient.
func (p *provider) NewMediaClient(repo *storage.Repository) media.Client {
	return telegram.NewMediaClient(
		p.newTelegramClient(repo),
		p.log.WithComponent("media-client"),
	)
}

// NewImageCache constrói o cache in-memory compartilhado entre Collector e Dashboard.
// 500 entradas, TTL 30min (ADR 011 v2).
func (p *provider) NewImageCache() *media.Cache {
	return media.NewCache(500, 30*time.Minute)
}

// NewCollector monta o coletor com um NoopClassifier (Fase 1).
func (p *provider) NewCollector(client telegram.TelegramClient, repo *storage.Repository) *collector.Collector {
	log := p.log
	return collector.NewCollector(
		client, repo, collector.NoopClassifier{}, log.WithComponent("collector"),
		p.cfg.DBWriterBufferSize, p.cfg.MaxRetries, p.cfg.HistoryMax, p.cfg.HistoryMaxDays,
	)
}

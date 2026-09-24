// Package cli constrói a árvore de comandos do Cobra para o binário unificado `limiar`.
// Ele nunca constrói as dependências concretas de storage ou telegram por conta própria:
// toda essa construção é fornecida por um Provider implementado em
// cmd/limiar/provider.go. Isso mantém a ligação de dependências em um único lugar.
package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/limiar/collector/internal/collector"
	"github.com/limiar/collector/internal/config"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/media"
	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

// Provider fornece dependências em tempo de execução para os comandos da CLI.
// A implementação concreta reside em cmd/limiar/provider.go; a CLI depende
// apenas desta interface para que nenhuma construção concreta de
// storage/telegram aconteça neste pacote.
type Provider interface {
	// Config retorna a configuração unificada validada.
	Config() *config.Config
	// Logger retorna o logger compartilhado da sessão.
	Logger() logger.Logger
	// Presenter retorna o presenter para mensagens ao operador.
	Presenter() logger.Presenter
	// OpenStore abre o banco de dados e retorna um Repository junto com uma função de fechamento.
	OpenStore(ctx context.Context) (*storage.Repository, func() error, error)
	// NewTelegramClient constrói a fachada do Telegram vinculada ao repositório fornecido.
	NewTelegramClient(repo *storage.Repository) telegram.TelegramClient
	// NewCollector constrói o coletor vinculado ao cliente, repositório fornecido.
	NewCollector(client telegram.TelegramClient, repo *storage.Repository) *collector.Collector
	// NewMediaClient constrói o MediaClient concreto usado por `media resolve`.
	NewMediaClient(repo *storage.Repository) media.Client
	// NewImageCache constrói o cache in-memory compartilhado entre Collector e Dashboard.
	NewImageCache() *media.Cache
}

// NewRootCmd constrói o comando raiz do binário unificado `limiar` e anexa todos os
// subcomandos (run, collector, processor, stats, auth, channels, dashboard, media),
// cada um injetado através de p.
func NewRootCmd(p Provider) *cobra.Command {
	root := &cobra.Command{
		Use:           "limiar",
		Short:         "Limiar: pipeline de captura e processamento de promoções do Telegram",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newRunCmd(p),
		newCollectorCmd(p),
		newProcessorCmd(p),
		newStatsCmd(p),
		newAuthCmd(p),
		newChannelsCmd(p),
		newDashboardCmd(p),
		newMediaCmd(p),
	)
	return root
}

// Package cli constrói a árvore de comandos do Cobra para o limiar-collector. Ele nunca
// constrói as dependências concretas de storage ou telegram por conta própria: toda
// essa construção é fornecida por um Provider implementado em
// cmd/limiar-collector/main.go. Isso mantém a ligação de dependências em um único lugar.
package cli

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/limiar/collector/internal/collector"
	"github.com/limiar/collector/internal/config"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

// Provider fornece dependências em tempo de execução para os comandos da CLI. A implementação
// concreta reside em main.go; a CLI depende apenas desta interface para que
// nenhuma construção concreta de storage/telegram aconteça neste pacote.
type Provider interface {
	// Config retorna a configuração validada.
	Config() *config.Config
	// Logger constrói um Logger no formato fornecido ("json" ou "text").
	Logger(format string) logger.Logger
	// OpenStore abre o banco de dados e retorna um Repository junto com uma função de fechamento.
	OpenStore(ctx context.Context) (*storage.Repository, func() error, error)
	// NewClient constrói a fachada (facade) do Telegram vinculada ao repositório e logger fornecidos.
	NewClient(log logger.Logger, repo *storage.Repository) telegram.TelegramClient
	// NewCollector constrói o coletor vinculado ao cliente, repositório e logger fornecidos.
	NewCollector(client telegram.TelegramClient, repo *storage.Repository, log logger.Logger) *collector.Collector
}

// NewRootCmd constrói o comando raiz e anexa os subcomandos auth, channels e run,
// todos injetados através de p.
func NewRootCmd(p Provider) *cobra.Command {
	root := &cobra.Command{
		Use:           "limiar-collector",
		Short:         "Limiar Phase 1 collector: captura mensagens brutas de canais do Telegram",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(
		newAuthCmd(p),
		newChannelsCmd(p),
		newRunCmd(p),
		newDashboardCmd(p),
	)
	return root
}

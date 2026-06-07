// Package cli builds the Cobra command tree for limiar-collector. It never
// constructs concrete storage or telegram dependencies itself: all such
// construction is supplied by a Provider implemented in
// cmd/limiar-collector/main.go. This keeps dependency wiring in a single place.
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

// Provider supplies runtime dependencies to the CLI commands. The concrete
// implementation lives in main.go; the CLI depends only on this interface so
// no concrete storage/telegram construction happens in this package.
type Provider interface {
	// Config returns the validated configuration.
	Config() *config.Config
	// Logger builds a Logger in the given format ("json" or "text").
	Logger(format string) logger.Logger
	// OpenStore opens the database and returns a Repository plus a close func.
	OpenStore(ctx context.Context) (*storage.Repository, func() error, error)
	// NewClient builds the Telegram facade bound to the given repo and logger.
	NewClient(log logger.Logger, repo *storage.Repository) telegram.TelegramClient
	// NewCollector builds the collector bound to the given client, repo, and logger.
	NewCollector(client telegram.TelegramClient, repo *storage.Repository, log logger.Logger) *collector.Collector
}

// NewRootCmd builds the root command and attaches the auth, channels, and run
// subcommands, all wired through p.
func NewRootCmd(p Provider) *cobra.Command {
	root := &cobra.Command{
		Use:           "limiar-collector",
		Short:         "Limiar Phase 1 collector: capture raw Telegram channel messages",
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

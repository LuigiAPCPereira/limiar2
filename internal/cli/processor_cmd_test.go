package cli

import (
	"context"
	"io"
	"testing"

	"github.com/limiar/collector/internal/collector"
	"github.com/limiar/collector/internal/config"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/media"
	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

func TestReprocessCommandURLResolverFlagsAreSafeByDefault(t *testing.T) {
	cmd := newReprocessCmd(testProvider{cfg: &config.Config{}})

	resolveURLs := cmd.Flags().Lookup("resolve-urls")
	if resolveURLs == nil {
		t.Fatal("--resolve-urls flag missing")
	}
	if resolveURLs.DefValue != "false" {
		t.Fatalf("--resolve-urls default = %q, want false", resolveURLs.DefValue)
	}

	limit := cmd.Flags().Lookup("resolve-urls-limit")
	if limit == nil {
		t.Fatal("--resolve-urls-limit flag missing")
	}
	if limit.DefValue != "0" {
		t.Fatalf("--resolve-urls-limit default = %q, want 0", limit.DefValue)
	}
}

type testProvider struct {
	cfg *config.Config
}

func (p testProvider) Config() *config.Config {
	if p.cfg != nil {
		return p.cfg
	}
	return &config.Config{}
}

func (testProvider) Logger() logger.Logger { return logger.NopLogger{} }
func (testProvider) Presenter() logger.Presenter {
	return logger.NewTerminalPresenter(io.Discard, false)
}
func (testProvider) OpenStore(context.Context) (*storage.Repository, func() error, error) {
	panic("OpenStore must not be called by flag tests")
}
func (testProvider) NewTelegramClient(*storage.Repository) telegram.TelegramClient {
	panic("NewTelegramClient must not be called by flag tests")
}
func (testProvider) NewCollector(telegram.TelegramClient, *storage.Repository) *collector.Collector {
	panic("NewCollector must not be called by flag tests")
}
func (testProvider) NewMediaClient(*storage.Repository) media.Client {
	panic("NewMediaClient must not be called by flag tests")
}
func (testProvider) NewImageCache() *media.Cache {
	panic("NewImageCache must not be called by flag tests")
}

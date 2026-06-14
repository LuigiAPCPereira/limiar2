package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/media"
)

// newMediaCmd constrói o subcomando `media`: diagnóstico e validação do subsistema
// de resolução de imagens (ADR 011/012).
//
//	media          — smoke test da fundação de metadados MTProto (Fase A).
//	media resolve  — baixa a imagem de uma mensagem via MTProto e salva no disco
//	                 (valida o fluxo L1→L2→L3; implementa o item 6 / Wave 4 prep).
//
// `media` (smoke) não requer sessão autenticada (só lê o banco). `media resolve`
// requer sessão autenticada (limiar-collector auth) e dispara download MTProto real.
func newMediaCmd(p Provider) *cobra.Command {
	var msgID int64

	cmd := &cobra.Command{
		Use:   "media",
		Short: "Subsistema de mídia: smoke test e resolução (download) de imagens",
		Long: `Subsistema de resolução de imagens (ADR 011).

Sem argumentos, executa um smoke test da fundação de metadados MTProto (Fase A):
valida, contra o banco, se o processor extraiu photo_id/access_hash/file_ref/dcid.

Subcomandos:
  resolve   Baixa a imagem de uma mensagem via MTProto (upload.GetFile) e salva no
            disco — valida o fluxo L1→L2→L3 do MediaResolver. Requer sessão
            autenticada. (Wave 4: o endpoint HTTP envolverá esse mesmo resolver.)`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runMediaSmoke(cmd, p, msgID)
		},
	}

	cmd.Flags().Int64Var(&msgID, "msg-id", 0,
		"ID da mensagem processada para inspecionar os metadados MTProto (smoke; 0 = só estatísticas)")
	cmd.AddCommand(newResolveCmd(p))
	return cmd
}

// runMediaSmoke valida a fundação de metadados contra o banco (não dispara download).
func runMediaSmoke(cmd *cobra.Command, p Provider, msgID int64) error {
	ctx := cmd.Context()

	cfg := p.Config()
	format, _ := logger.ResolveFormat("media", cfg.LogFormat, logger.IsTerminalWriter(os.Stdout))
	log := p.Logger(format)
	presenter := p.Presenter()

	repo, closeStore, err := p.OpenStore(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = closeStore() }()

	presenter.Info("📷 Smoke test do subsistema de mídia")

	stats, err := repo.PhotoMetadataStats(ctx)
	if err != nil {
		return apperrors.Wrap("cli", "media_stats", err)
	}

	presenter.Step(fmt.Sprintf("Mensagens processadas: %d", stats.TotalProcessed))
	presenter.Step(fmt.Sprintf("Com foto (photo_id > 0): %d", stats.WithPhoto))
	if stats.WithPhoto > 0 {
		pct := 100 * float64(stats.CompleteMTProto) / float64(stats.WithPhoto)
		presenter.Step(fmt.Sprintf("Metadados MTProto completos: %d (%.1f%% das com foto)",
			stats.CompleteMTProto, pct))
	}

	if msgID > 0 {
		meta, err := repo.GetPhotoMetadata(ctx, msgID)
		if err != nil {
			presenter.Warning(fmt.Sprintf("msg-id %d: %v", msgID, err))
		} else {
			complete := meta.PhotoID > 0 && meta.AccessHash != 0 &&
				meta.FileReference != "" && meta.DCID != 0
			presenter.Step(fmt.Sprintf(
				"msg-id %d: photo_id=%d access_hash=%d dcid=%d fileref=%d bytes (completo=%v)",
				msgID, meta.PhotoID, meta.AccessHash, meta.DCID, len(meta.FileReference), complete))
		}
	}

	log.Info("📷 media smoke concluído",
		"total", stats.TotalProcessed, "com_foto", stats.WithPhoto,
		"mtproto_completo", stats.CompleteMTProto)
	presenter.Info("ℹ️  Download MTProto ao vivo: `media resolve` (ou Wave 4 — ADR 012)")
	return nil
}

// newResolveCmd constrói `media resolve`: baixa a imagem de uma mensagem e salva
// no disco. Tenta primeiro web scraping da página pública do canal (t.me/s/username)
// que não precisa de file_reference MTProto. Se falhar (canal privado ou mensagem
// não encontrada na página), cai para o resolver MTProto (cache L1 + L2 + L3).
func newResolveCmd(p Provider) *cobra.Command {
	var msgID int64
	var outPath string

	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Baixar a imagem de uma mensagem e salvar no disco",
		Long: `Baixa a imagem da mensagem processada informada e salva no disco.

Estratégia (em ordem):
  1. Web scraping da página pública do canal (t.me/s/username) — rápido,
     sem file_reference, funciona para canais públicos.
  2. MTProto via MediaResolver (cache L1 + download L2 + renovação L3) —
     para canais privados ou quando o scraping falha.

O --msg-id é o processed_messages.id (PK), não o message_id do Telegram.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if msgID <= 0 {
				return fmt.Errorf("--msg-id é obrigatório (use um processed_messages.id > 0)")
			}
			if outPath == "" {
				return fmt.Errorf("--out é obrigatório (caminho do JPEG de saída)")
			}

			cfg := p.Config()
			format, _ := logger.ResolveFormat("media", cfg.LogFormat, logger.IsTerminalWriter(os.Stdout))
			log := p.Logger(format)
			presenter := p.Presenter()

			repo, closeStore, err := p.OpenStore(ctx)
			if err != nil {
				return err
			}
			defer func() { _ = closeStore() }()

			// Busca metadados da mensagem para obter channel_id e message_id.
			meta, err := repo.GetPhotoMetadata(ctx, msgID)
			if err != nil {
				return apperrors.Wrap("cli", "get_metadata", err)
			}
			if meta.PhotoID == 0 {
				return fmt.Errorf("mensagem %d não possui foto", msgID)
			}

			// Busca o username do canal para scraping.
			username, err := repo.GetChannelUsername(ctx, meta.ChannelID)
			if err != nil {
				log.Debug("canal não encontrado para scraping", "channel_id", meta.ChannelID)
			}

			mediaClient := p.NewMediaClient(log, repo)

			// Estratégia 1: web scraping (não precisa de MTProto).
			if username != "" {
				presenter.Info(fmt.Sprintf("🔎 Tentando scraping de @%s (msg %d)...", username, meta.MsgID))
				photoURL, scrapeErr := mediaClient.ScrapePhotoURL(ctx, username, meta.MsgID)
				if scrapeErr == nil {
					presenter.Step(fmt.Sprintf("📥 Baixando %d bytes via HTTP...", len(photoURL)))
					data, dlErr := downloadHTTP(ctx, photoURL)
					if dlErr == nil {
						if wErr := os.WriteFile(outPath, data, 0o644); wErr != nil {
							return apperrors.Wrap("cli", "write_file", wErr)
						}
						presenter.Success(fmt.Sprintf("✅ %d bytes salvos em %s (via scraping)", len(data), outPath))
						log.Info("📷 media resolve OK (scraping)", "msg_id", msgID, "bytes", len(data))
						return nil
					}
					log.Warn("download HTTP falhou, caindo para MTProto", "erro", dlErr)
				} else {
					log.Debug("scraping falhou, caindo para MTProto", "erro", scrapeErr)
				}
			}

			// Estratégia 2: MTProto via MediaResolver.
			cache := media.NewImageCache(200, 30*time.Minute)
			resolver := media.NewMediaResolver(mediaClient, repo, cache, log)

			presenter.Info(fmt.Sprintf("🔎 Resolvendo via MTProto (msg-id %d)...", msgID))
			data, err := resolver.ResolveImage(ctx, msgID)
			if err != nil {
				if errors.Is(err, apperrors.ErrNoPhoto) {
					return fmt.Errorf("mensagem %d não possui foto", msgID)
				}
				return apperrors.Wrap("cli", "media_resolve", err)
			}

			if err := os.WriteFile(outPath, data, 0o644); err != nil {
				return apperrors.Wrap("cli", "write_file", err)
			}
			presenter.Success(fmt.Sprintf("✅ %d bytes salvos em %s (via MTProto)", len(data), outPath))
			log.Info("📷 media resolve OK (MTProto)", "msg_id", msgID, "bytes", len(data))
			return nil
		},
	}

	cmd.Flags().Int64Var(&msgID, "msg-id", 0, "ID da mensagem processada (obrigatório)")
	cmd.Flags().StringVar(&outPath, "out", "", "arquivo de saída JPEG (obrigatório)")
	return cmd
}

// downloadHTTP baixa o conteúdo de uma URL via HTTP GET.
func downloadHTTP(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Limiar/1.0)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
}

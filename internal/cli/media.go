package cli

import (
	"encoding/base64"
	"errors"
	"fmt"
	"github.com/spf13/cobra"
	"os"
	"sync/atomic"
	"time"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/media"
	"github.com/limiar/collector/internal/storage"
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
	cmd.AddCommand(newBackfillCmd(p))
	return cmd
}

// runMediaSmoke valida a fundação de metadados contra o banco (não dispara download).
func runMediaSmoke(cmd *cobra.Command, p Provider, msgID int64) error {
	ctx := cmd.Context()

	cfg := p.Config()
	log := p.Logger()
	presenter := p.Presenter()

	store, err := storage.Open(ctx, cfg.DBPath, log)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	procRepo, err := storage.NewProcessorRepository(store.DB())
	if err != nil {
		return err
	}
	defer func() { _ = procRepo.Close() }()

	presenter.Info("📷 Smoke test do subsistema de mídia")

	stats, err := procRepo.PhotoMetadataStats(ctx)
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
		meta, err := procRepo.GetPhotoMetadata(ctx, msgID)
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

// newResolveCmd constrói `media resolve`: resolve a imagem de uma mensagem e salva
// no disco. O MediaResolver tenta em ordem: cache L1 → photo_cache L2 →
// download MTProto L3 → inline_thumb L4.
func newResolveCmd(p Provider) *cobra.Command {
	var msgID int64
	var outPath string

	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Baixar a imagem de uma mensagem e salvar no disco",
		Long: `Baixa a imagem da mensagem processada informada e salva no disco.

O MediaResolver tenta automaticamente (em ordem):
  L1: Cache in-memory (LRU + TTL 30min)
  L2: photo_cache persistido no banco
  L3: Download MTProto via upload.GetFile, com renovação de file_reference
  L4: inline_thumb do payload Telegram

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
			log := p.Logger()
			presenter := p.Presenter()

			store, err := storage.Open(ctx, cfg.DBPath, log)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			procRepo, err := storage.NewProcessorRepository(store.DB())
			if err != nil {
				return err
			}
			defer func() { _ = procRepo.Close() }()

			collectorRepo, err := storage.NewRepository(store.DB())
			if err != nil {
				return err
			}
			defer func() { _ = collectorRepo.Close() }()

			cache := media.NewCache(200, 30*time.Minute)
			mediaClient := p.NewMediaClient(collectorRepo)
			resolver := media.NewResolver(procRepo, cache, mediaClient, log.WithComponent("media-resolver"))

			presenter.Info(fmt.Sprintf("🔎 Resolvendo imagem (msg-id %d)...", msgID))
			data, source, err := resolver.ResolveImage(ctx, msgID)
			if err != nil {
				if errors.Is(err, apperrors.ErrNoPhoto) {
					return fmt.Errorf("mensagem %d não possui foto", msgID)
				}
				return apperrors.Wrap("cli", "media_resolve", err)
			}

			// #nosec G306
			if err := os.WriteFile(outPath, data, 0o600); err != nil {
				return apperrors.Wrap("cli", "write_file", err)
			}
			if err := os.Chmod(outPath, 0o600); err != nil {
				return apperrors.Wrap("cli", "chmod", err)
			}
			presenter.Success(fmt.Sprintf("✅ %d bytes salvos em %s (source: %s)", len(data), outPath, source))
			log.Info("📷 media resolve OK", "msg_id", msgID, "bytes", len(data), "source", source)
			return nil
		},
	}

	cmd.Flags().Int64Var(&msgID, "msg-id", 0, "ID da mensagem processada (obrigatório)")
	cmd.Flags().StringVar(&outPath, "out", "", "arquivo de saída JPEG (obrigatório)")
	return cmd
}

// newBackfillCmd constrói `media backfill`: baixa thumbnails (800x800) de todas as
// fotos distintas com metadata MTProto e persiste em photo_cache. Requer sessão
// autenticada. Após o backfill, o dashboard serve imagens em alta resolução mesmo
// no modo `processor run` (sem conexão Telegram ativa).
func newBackfillCmd(p Provider) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "backfill",
		Short: "Baixar thumbnails de todas as fotos e persistir no banco",
		Long: "Baixa a variante \"x\" (800x800) de cada foto distinta com metadata MTProto\n" +
			"via upload.GetFile e persiste em photo_cache. Requer sessão autenticada\n" +
			"(`limiar auth`). Após o backfill, o dashboard serve imagens em alta resolução\n" +
			"mesmo sem conexão Telegram ativa (`processor run`).",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cfg := p.Config()
			log := p.Logger()

			store, err := storage.Open(ctx, cfg.DBPath, log)
			if err != nil {
				return err
			}
			defer func() { _ = store.Close() }()

			procRepo, err := storage.NewProcessorRepository(store.DB())
			if err != nil {
				return apperrors.Wrap("cli", "new_processor_repo", err)
			}
			defer func() { _ = procRepo.Close() }()

			// Lista fotos com metadados MTProto derivados do raw payload quando
			// possível. Isso corrige bancos antigos cujos photo_id/access_hash foram
			// arredondados por decode JSON via float64. O limite é aplicado no
			// repositório para evitar varrer o banco inteiro em smoke tests.
			photos, err := procRepo.ListPendingPhotoMetadataForBackfill(ctx, limit)
			if err != nil {
				return apperrors.Wrap("cli", "backfill_query", err)
			}

			if len(photos) == 0 {
				log.Info("📷 Backfill: nenhuma foto pendente — photo_cache completo")
				return nil
			}

			log.Info("📷 Backfill iniciado", "fotos", len(photos))

			collectorRepo, err := storage.NewRepository(store.DB())
			if err != nil {
				return apperrors.Wrap("cli", "backfill_new_collector_repo", err)
			}
			defer func() { _ = collectorRepo.Close() }()

			mediaClient := p.NewMediaClient(collectorRepo)

			// Monta requests com channel_id/message_id para renovação de file_reference.
			reqs := make([]media.PhotoDownloadRequest, len(photos))
			for i, pm := range photos {
				fileRef, _ := base64.StdEncoding.DecodeString(pm.FileReference)
				reqs[i] = media.PhotoDownloadRequest{
					PhotoID:       pm.PhotoID,
					AccessHash:    pm.AccessHash,
					FileReference: fileRef,
					DCID:          pm.DCID,
					ChannelID:     pm.ChannelID,
					MessageID:     pm.MsgID,
				}
			}

			var saved, failed, processed atomic.Int64
			total := int64(len(reqs))

			err = mediaClient.DownloadPhotoBatch(ctx, reqs, func(photoID int64, data []byte, dlErr error) {
				done := processed.Add(1)
				if dlErr != nil {
					failed.Add(1)
					log.Debug("backfill: download falhou", "photo_id", photoID, "erro", dlErr)
				} else if saveErr := procRepo.SavePhotoData(ctx, photoID, data); saveErr != nil {
					failed.Add(1)
					log.Debug("backfill: save falhou", "photo_id", photoID, "erro", saveErr)
				} else {
					saved.Add(1)
				}
				if done%200 == 0 || done == total {
					log.Info("📷 Backfill progresso",
						"processadas", done, "total", total,
						"ok", saved.Load(), "falhou", failed.Load())
				}
			})

			log.Info("📷 Backfill concluído",
				"total", total, "salvas", saved.Load(), "falharam", failed.Load())
			if err != nil {
				return apperrors.Wrap("cli", "backfill_batch", err)
			}
			return nil
		},
	}

	cmd.Flags().IntVar(&limit, "limit", 0, "Máximo de fotos a baixar (0 = todas)")
	return cmd
}

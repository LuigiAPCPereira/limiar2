package media

import (
	"context"
	"encoding/base64"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/model"
)

// Resolver resolve imagens de mensagens usando a cadeia de fallback (ADR 011 v3):
//
//	L1: cache in-memory (full-res)
//	L2: repo.GetPhotoData (photo_cache — full-res persistida de download anterior)
//	L3: client.DownloadPhoto (MTProto on-demand, apenas quando client != nil)
//	L4: repo.GetInlineThumb (thumbnail inline do DB)
//	 → ErrNoPhoto
type Resolver struct {
	repo   Repository
	cache  *Cache
	client Client // pode ser nil (desabilita download on-demand — modo sem Telegram)
	log    logger.Logger
}

// NewResolver constrói o resolver com suas dependências.
// cache pode ser nil (desabilita a camada L1).
// client pode ser nil (desabilita o download on-demand L3 — usado no modo
// `processor run` sem acesso ao Telegram).
// log pode ser nil (usa NopLogger).
func NewResolver(repo Repository, cache *Cache, client Client, log logger.Logger) *Resolver {
	if log == nil {
		log = logger.NopLogger{}
	}
	return &Resolver{repo: repo, cache: cache, client: client, log: log}
}

// ResolveImage retorna os bytes da foto da mensagem processada informada.
// processedMsgID é o PK de processed_messages (não o photo_id do Telegram).
//
// Cadeia de fallback (ADR 011 v3):
//  1. Cache in-memory (L1) — full-res se já resolvida nesta sessão
//  2. photo_cache no DB (L2) — full-res persistida de download anterior
//  3. Download MTProto on-demand (L3) — somente se client configurado
//  4. inline_thumb no DB (L4) — thumbnail inline (~230 bytes, quase sempre disponível)
//  5. ErrNoPhoto — mensagem sem foto
//
// Retorna (data, source, error) onde source indica a origem dos bytes:
// "cache", "photo-cache", "downloaded" ou "inline-thumb".
func (r *Resolver) ResolveImage(ctx context.Context, processedMsgID int64) ([]byte, string, error) {
	// Preferir metadados derivados do raw payload: bancos processados antes da
	// correção de DecodePayloadMap podem ter photo_id arredondado por float64.
	meta, metaErr := r.repo.GetPhotoMetadata(ctx, processedMsgID)
	if metaErr != nil {
		r.log.Debug("media: get_photo_metadata falhou; usando photo_id persistido", "processed_msg_id", processedMsgID, "erro", metaErr)
	}
	var photoID int64
	if meta != nil && meta.PhotoID != 0 {
		photoID = meta.PhotoID
	} else {
		var err error
		photoID, err = r.repo.GetPhotoID(ctx, processedMsgID)
		if err != nil {
			return nil, "", apperrors.Wrap("media", "get_photo_id", err)
		}
		if photoID == 0 {
			return nil, "", apperrors.ErrNoPhoto
		}
	}

	// L1: cache in-memory (full-res).
	if r.cache != nil {
		if data, ok := r.cache.Get(photoID); ok {
			r.log.Debug("media: cache hit", "photo_id", photoID, "bytes", len(data))
			return data, "cache", nil
		}
	}

	// L2: photo_cache no DB (full-res persistida).
	data, err := r.repo.GetPhotoData(ctx, photoID)
	if err != nil {
		return nil, "", apperrors.Wrap("media", "get_photo_data", err)
	}
	if data != nil {
		r.log.Debug("media: photo-cache hit", "photo_id", photoID, "bytes", len(data))
		if r.cache != nil {
			r.cache.Put(photoID, data)
		}
		return data, "photo-cache", nil
	}

	// L3: download MTProto on-demand (apenas se client configurado).
	if r.client != nil {
		if downloaded, ok := r.tryDownload(ctx, processedMsgID, photoID, meta); ok {
			return downloaded, "downloaded", nil
		}
	}

	// L4: inline_thumb no DB (thumbnail inline da mensagem processada).
	thumb, err := r.repo.GetInlineThumb(ctx, processedMsgID)
	if err != nil {
		return nil, "", apperrors.Wrap("media", "get_inline_thumb", err)
	}
	if thumb != nil {
		r.log.Debug("media: inline thumb hit", "processed_msg_id", processedMsgID, "photo_id", photoID, "bytes", len(thumb))
		return thumb, "inline-thumb", nil
	}

	return nil, "", apperrors.ErrNoPhoto
}

// tryDownload tenta o download MTProto on-demand. Retorna (bytes, true) em caso
// de sucesso; (nil, false) caso contrário (client ausente, metadados incompletos
// ou falha de rede), para que o caller continue a cadeia de fallback. Bytes
// bem-sucedidos são promovidos ao cache L1 e persistidos em photo_cache de forma
// best-effort: falhas de persistência são apenas logadas, sem impedir o retorno
// da imagem ao cliente.
func (r *Resolver) tryDownload(ctx context.Context, processedMsgID, photoID int64, meta *model.PhotoMetadata) ([]byte, bool) {
	if meta == nil {
		var err error
		meta, err = r.repo.GetPhotoMetadata(ctx, processedMsgID)
		if err != nil {
			r.log.Debug("media: get_photo_metadata falhou", "photo_id", photoID, "erro", err)
			return nil, false
		}
	}
	if meta == nil || meta.AccessHash == 0 || meta.DCID == 0 {
		return nil, false
	}

	req := PhotoDownloadRequest{
		PhotoID:    meta.PhotoID,
		AccessHash: meta.AccessHash,
		DCID:       meta.DCID,
		ChannelID:  meta.ChannelID,
		MessageID:  meta.MsgID,
	}
	// FileReference é persistido como base64 no DB; decodifica para bytes MTProto.
	if meta.FileReference != "" {
		if ref, derr := base64.StdEncoding.DecodeString(meta.FileReference); derr == nil {
			req.FileReference = ref
		}
	}

	data, derr := r.client.DownloadPhoto(ctx, req)
	if derr != nil {
		r.log.Warn("media: download on-demand falhou", "photo_id", photoID, "erro", derr)
		return nil, false
	}
	if len(data) == 0 {
		return nil, false
	}

	r.log.Debug("media: download on-demand ok", "photo_id", photoID, "bytes", len(data))
	if r.cache != nil {
		r.cache.Put(photoID, data)
	}
	if serr := r.repo.SavePhotoData(ctx, photoID, data); serr != nil {
		r.log.Warn("media: save_photo_data falhou (best-effort)", "photo_id", photoID, "erro", serr)
	}
	return data, true
}

// PutCache armazena bytes no cache. Usado pelo Collector para entregar
// imagens full-res baixadas proativamente.
func (r *Resolver) PutCache(photoID int64, data []byte) {
	if r.cache != nil {
		r.cache.Put(photoID, data)
	}
}

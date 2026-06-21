package media

import (
	"context"
	"fmt"

	"golang.org/x/sync/singleflight"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
)

// Resolver resolve imagens de mensagens usando a cadeia de fallback:
//
//	1. cache.Get(photoID)    → full-res do Collector (se disponível)
//	2. repo.GetInlineThumb   → thumbnail inline do DB (sempre disponível)
//	3. ErrNoPhoto            → 404
//
type Resolver struct {
	repo  Repository
	cache *Cache
	sf    singleflight.Group // coalesce chamadas concorrentes ao mesmo photoID
	log   logger.Logger
}

// NewResolver constrói o resolver com suas dependências.
// cache pode ser nil (desabilita cache, só DB).
// log pode ser nil (usa NopLogger).
func NewResolver(repo Repository, cache *Cache, log logger.Logger) *Resolver {
	if log == nil {
		log = logger.NopLogger{}
	}
	return &Resolver{repo: repo, cache: cache, log: log}
}

// ResolveImage retorna os bytes da foto da mensagem processada informada.
// processedMsgID é o PK de processed_messages (não o photo_id do Telegram).
//
// Cadeia de fallback (ADR 011 v2):
//  1. Cache in-memory (LRU + TTL) — full-res se Collector já baixou
//  2. DB inline_thumb — thumbnail inline (~230 bytes, sempre disponível)
//  3. ErrNoPhoto — mensagem sem foto
//
// Retorna (data, source, error) onde source indica a origem dos bytes:
// "collector-cache" ou "inline-thumb".
func (r *Resolver) ResolveImage(ctx context.Context, processedMsgID int64) ([]byte, string, error) {
	// Busca photo_id do Telegram a partir do processed_messages.id.
	photoID, err := r.repo.GetPhotoID(ctx, processedMsgID)
	if err != nil {
		return nil, "", apperrors.Wrap("media", "get_photo_id", err)
	}
	if photoID == 0 {
		return nil, "", apperrors.ErrNoPhoto
	}

	// L1: cache in-memory (full-res do Collector).
	if r.cache != nil {
		if data, ok := r.cache.Get(photoID); ok {
			r.log.Debug("media: cache hit", "photo_id", photoID, "bytes", len(data))
			return data, "collector-cache", nil
		}
	}

	// L2: DB inline_thumb com singleflight — coalesce chamadas concorrentes
	// ao mesmo photoID em uma única query.
	key := fmt.Sprintf("thumb:%d", photoID)
	v, err, _ := r.sf.Do(key, func() (any, error) {
		return r.repo.GetInlineThumb(ctx, photoID)
	})
	if err != nil {
		return nil, "", apperrors.Wrap("media", "get_inline_thumb", err)
	}
	thumb := v.([]byte) // safe: GetInlineThumb retorna []byte ou nil
	if thumb != nil {
		r.log.Debug("media: inline thumb hit", "photo_id", photoID, "bytes", len(thumb))
		return thumb, "inline-thumb", nil
	}

	// L3: não encontrado.
	return nil, "", apperrors.ErrNoPhoto
}

// PutCache armazena bytes no cache. Usado pelo Collector para entregar
// imagens full-res baixadas proativamente.
func (r *Resolver) PutCache(photoID int64, data []byte) {
	if r.cache != nil {
		r.cache.Put(photoID, data)
	}
}

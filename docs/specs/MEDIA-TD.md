# Design — Subsistema de Resolução de Imagens (MEDIA-TD v2)

> **ADR:** [011-mtproto-image-resolver.md](../adr/011-mtproto-image-resolver.md)
> **Depende de:** ADR 009 (orquestrador), ADR 002 (gotd/td direto)
> **Supercede:** MEDIA-TD v1 (proxy MTProto sob demanda)

---

## 1. Visão geral

O subsistema de imagens usa uma abordagem de duas camadas:

1. **Inline Thumbnail** (source of truth universal): thumbnail compacto (~230 bytes)
   incluído pelo Telegram no payload de cada mensagem com foto. Extraído pelo
   Processor e persistido no DB. 100% de cobertura, latência sub-ms.

2. **Download Proativo** (upgrade opcional): full-res (800px) baixado pelo Collector
   no momento da chegada da mensagem (janela onde `file_reference` MTProto é válido).
   Armazenado no cache in-memory compartilhado.

```
┌─ Collector ──────────────────────────────────────┐
│  Mensagem chega (file_reference fresco)          │
│  → SaveRawMessage (payload JSON com inline thumb)│
│  → DownloadPhoto (full-res, assíncrono)          │
│  → cache.Put(photoID, fullResBytes)              │
└────────────────────────┬─────────────────────────┘
                         │ cache compartilhado (mesmo processo)
┌─ Processor ────────────┼─────────────────────────┐
│  Normaliza payload     │                         │
│  → extractInlineThumb  │  ← Type "i", ~230 bytes│
│  → INSERT inline_thumb │  ← BLOB no DB          │
└────────────────────────┼─────────────────────────┘
                         │
┌─ API (Wave 4) ─────────┼─────────────────────────┐
│  GET /media/{photo_id} │                         │
│  1. cache.Get → 200   │  X-Source: cache        │
│  2. DB inline → 200   │  X-Source: inline-thumb │
│  3. 404               │                         │
└──────────────────────────────────────────────────┘
```

---

## 2. Componentes

### 2.1 ImageCache (`internal/media/cache.go`)

```go
import "github.com/hashicorp/golang-lru/v2/expirable"

type ImageCache struct {
    lru *expirable.LRU[int64, []byte]
}

func NewImageCache(size int, ttl time.Duration) *ImageCache
func (c *ImageCache) Get(photoID int64) ([]byte, bool)
func (c *ImageCache) Put(photoID int64, data []byte)
func (c *ImageCache) Len() int
```

**Limites:** 500 entradas, TTL 30min. Thread-safe.

### 2.2 MediaResolver (`internal/media/resolver.go`)

```go
type MediaResolver struct {
    repo  MediaRepository
    cache *ImageCache
    log   logger.Logger
}

func (r *MediaResolver) ResolveImage(ctx context.Context, processedMsgID int64) ([]byte, string, error)
func (r *MediaResolver) PutCache(photoID int64, data []byte)
```

**Cadeia de fallback:**
1. `cache.Get(photoID)` → `("collector-cache", data)`
2. `repo.GetInlineThumb(photoID)` → `("inline-thumb", data)`
3. `ErrNoPhoto` → 404

### 2.3 MediaClient (`internal/media/media.go`)

```go
type MediaClient interface {
    DownloadPhoto(ctx context.Context, req PhotoDownloadRequest) ([]byte, error)
}
```

Implementação concreta em `internal/telegram/media.go` sobre gotd/td.
Usado apenas pelo Collector para download proativo.

### 2.4 MediaRepository (`internal/media/media.go`)

```go
type MediaRepository interface {
    GetPhotoID(ctx context.Context, processedMsgID int64) (int64, error)
    GetInlineThumb(ctx context.Context, photoID int64) ([]byte, error)
}
```

Satisfeito por `*storage.Repository`.

---

## 3. Extração no Normalizer

```go
// internal/processor/normalizer.go

func extractInlineThumb(msg map[string]any) []byte {
    media := msg["Media"].(map[string]any)
    photo := media["Photo"].(map[string]any)
    for _, size := range photo["Sizes"].([]any) {
        if size["Type"] == "i" {
            decoded, _ := base64.StdEncoding.DecodeString(size["Bytes"].(string))
            return decoded
        }
    }
    return nil
}
```

Campo `InlineThumb []byte` no `NormalizedMessage`. Persistido como BLOB.

---

## 4. Download Proativo no Collector

```go
// internal/collector/collector.go

func (c *Collector) proactiveDownload(ctx context.Context, msg *storage.RawMessage) {
    req, err := telegram.ExtractPhotoRequest(msg.Payload)
    if err != nil || req == nil {
        return
    }
    go func() {
        data, err := c.mediaClient.DownloadPhoto(ctx, *req)
        if err != nil {
            return
        }
        c.imageCache.Put(req.PhotoID, data)
    }()
}
```

Executado após cada save bem-sucedido no dbWriter. Assíncrono (goroutine).

---

## 5. Schema — Migration 007

```sql
ALTER TABLE processed_messages ADD COLUMN inline_thumb BLOB;

CREATE INDEX IF NOT EXISTS idx_processed_inline_thumb
    ON processed_messages(photo_id) WHERE inline_thumb IS NOT NULL;
```

---

## 6. Edge cases

| Caso | Comportamento |
|------|---------------|
| Mensagem sem foto | `InlineThumb = nil`. Resolver retorna `ErrNoPhoto` |
| Foto sem Type "i" | `InlineThumb = nil`. Resolver retorna `ErrNoPhoto` |
| Download proativo falha | Cache não populado. Fallback para inline thumb |
| Cache miss + inline thumb | Retorna inline thumb (source: "inline-thumb") |
| Restart do processo | Cache vazio. Inline thumb como fallback universal |
| Forward de outro canal | `photo_id` é global. Funciona cross-canal |

---

## 7. Constraints não-negociáveis

- **Zero escrita em disco** para imagens
- **Collector é único ponto** de download full-res
- **Inline thumb sempre disponível** no DB
- **Única dependência externa**: `github.com/hashicorp/golang-lru/v2/expirable`

# Design — Subsistema de Resolução de Imagens (MEDIA-TD)

> **ADR:** [011-mtproto-image-resolver.md](../adr/011-mtproto-image-resolver.md)
> **Depende de:** ADR 009 (orquestrador), ADR 002 (gotd/td direto)

---

## 1. Visão geral

O subsistema de imagens permite que o `limiar-api` sirva fotos das mensagens via proxy
MTProto, sem depender de URLs HTTP efêmeras do Telegram. O frontend usa
`<img src="/api/media/{msg_id}/photo">` e o endpoint faz stream dos bytes sob demanda.

```
Frontend → limiar-api → MediaResolver → gotd/td (MTProto)
                              │
                         LRU+TTL cache
                         (in-memory, 200 max, 30min TTL)
                              │
                         singleflight.Group
                         (dedup de requests simultâneas)
```

---

## 2. Componentes

### 2.1 PhotoMetadata (processor)

Extraída pelo normalizer do payload bruto. Persistida em `processed_messages`.

```go
// internal/processor/normalizer.go

type PhotoMetadata struct {
    PhotoID       int64  `json:"photo_id"`
    AccessHash    int64  `json:"access_hash"`
    FileReference string `json:"file_ref"` // base64
    DCID          int    `json:"dcid"`
}
```

### 2.2 MediaResolver (novo pacote `internal/media`)

```go
// internal/media/resolver.go

// MediaResolver baixa imagens via MTProto com cache in-memory e singleflight.
type MediaResolver struct {
    client    MediaClient        // interface sobre gotd/td
    repo      MediaRepository    // leitura/escrita de photo metadata
    cache     *ImageCache        // LRU + TTL
    group     singleflight.Group // dedup de requests concorrentes
}

// MediaClient abstrai as chamadas MTProto necessárias.
type MediaClient interface {
    // DownloadPhoto baixa a imagem usando os campos MTProto.
    DownloadPhoto(ctx context.Context, req PhotoDownloadRequest) ([]byte, error)

    // RenewFileReference renova via messages.getMessages(msg_id).
    RenewFileReference(ctx context.Context, channelID, msgID int64) (*PhotoMetadata, error)

    // RefetchFromChannel re-coleta a mensagem do canal (hard renew).
    RefetchFromChannel(ctx context.Context, channelID, msgID int64) (*PhotoMetadata, error)
}

// MediaRepository abstrai o acesso ao banco para metadados de foto.
type MediaRepository interface {
    GetPhotoMetadata(ctx context.Context, msgID int64) (*PhotoMetadata, error)
    UpdateFileReference(ctx context.Context, msgID int64, fileRef string) error
    UpdatePhotoMetadata(ctx context.Context, msgID int64, meta *PhotoMetadata) error
}

type PhotoDownloadRequest struct {
    PhotoID       int64
    AccessHash    int64
    FileReference []byte // decoded from base64
    DCID          int
}
```

### 2.3 ImageCache (LRU + TTL)

```go
// internal/media/cache.go

type ImageCache struct {
    mu      sync.RWMutex
    entries map[int64]*cacheEntry
    order   []int64 // LRU order (oldest first)
    maxLen  int
    ttl     time.Duration
}

type cacheEntry struct {
    data      []byte
    fetchedAt time.Time
}

// NewImageCache cria cache com limites. Default: 200 entradas, 30min TTL.
func NewImageCache(maxLen int, ttl time.Duration) *ImageCache { ... }

// Get retorna os bytes se presentes e dentro do TTL.
func (c *ImageCache) Get(photoID int64) ([]byte, bool) { ... }

// Put armazena bytes e evicta LRU se necessário.
func (c *ImageCache) Put(photoID int64, data []byte) { ... }
```

**Limites:**
- `maxLen = 200` → ~10MB max (200 × 50KB JPEG 800px)
- `ttl = 30 * time.Minute`
- Evicção: LRU quando atinge `maxLen`, TTL no `Get`

---

## 3. Fluxo de resolução

```go
func (r *MediaResolver) ResolveImage(ctx context.Context, msgID int64, channelID int64) ([]byte, error) {
    // Chave do singleflight: photo_id (não msg_id — mesma foto pode estar em múltiplas msgs)
    meta, err := r.repo.GetPhotoMetadata(ctx, msgID)
    if err != nil {
        return nil, fmt.Errorf("media: get metadata: %w", err)
    }
    if meta.PhotoID == 0 {
        return nil, ErrNoPhoto
    }

    // Singleflight: dedup por photo_id
    key := strconv.FormatInt(meta.PhotoID, 10)
    result, err, _ := r.group.Do(key, func() (any, error) {
        // L1: Cache check
        if data, ok := r.cache.Get(meta.PhotoID); ok {
            return data, nil
        }

        // L2: MTProto download
        data, err := r.client.DownloadPhoto(ctx, PhotoDownloadRequest{
            PhotoID:       meta.PhotoID,
            AccessHash:    meta.AccessHash,
            FileReference: decodeBase64(meta.FileReference),
            DCID:          meta.DCID,
        })
        if err == nil {
            r.cache.Put(meta.PhotoID, data)
            return data, nil
        }

        // L3: File reference expired?
        if isFileRefExpired(err) {
            return r.renewAndRetry(ctx, meta, msgID, channelID)
        }

        return nil, fmt.Errorf("media: download failed: %w", err)
    })

    if err != nil {
        return nil, err
    }
    return result.([]byte), nil
}
```

### 3.1 Renovação L3

```go
func (r *MediaResolver) renewAndRetry(ctx context.Context, meta *PhotoMetadata, msgID, channelID int64) ([]byte, error) {
    // L3 Soft: renew via messages.getMessages
    renewed, err := r.client.RenewFileReference(ctx, channelID, msgID)
    if err == nil {
        // Atualiza no banco
        _ = r.repo.UpdateFileReference(ctx, msgID, renewed.FileReference)
        // Retry com novo file_reference
        data, err := r.client.DownloadPhoto(ctx, PhotoDownloadRequest{
            PhotoID:       renewed.PhotoID,
            AccessHash:    renewed.AccessHash,
            FileReference: decodeBase64(renewed.FileReference),
            DCID:          renewed.DCID,
        })
        if err == nil {
            r.cache.Put(renewed.PhotoID, data)
            return data, nil
        }
    }

    // L3 Hard: refetch from channel
    refetched, err := r.client.RefetchFromChannel(ctx, channelID, msgID)
    if err != nil {
        return nil, fmt.Errorf("media: hard renew failed: %w", err)
    }
    // Atualiza todos os campos no banco
    _ = r.repo.UpdatePhotoMetadata(ctx, msgID, refetched)
    // Retry final
    data, err := r.client.DownloadPhoto(ctx, PhotoDownloadRequest{
        PhotoID:       refetched.PhotoID,
        AccessHash:    refetched.AccessHash,
        FileReference: decodeBase64(refetched.FileReference),
        DCID:          refetched.DCID,
    })
    if err != nil {
        return nil, fmt.Errorf("media: retry after hard renew failed: %w", err)
    }
    r.cache.Put(refetched.PhotoID, data)
    return data, nil
}
```

---

## 4. Singleflight — por que é crítico

### Cenário sem singleflight

```
User A: GET /api/media/95605/photo  ─→  upload.GetFile (200ms)  ─→  200 OK
User B: GET /api/media/95605/photo  ─→  upload.GetFile (200ms)  ─→  200 OK
                                                                  ↑ 2 downloads MTProto
                                                                    para a MESMA imagem
```

### Cenário com singleflight

```
User A: GET /api/media/95605/photo  ─→  upload.GetFile (200ms)  ─→  200 OK
User B: GET /api/media/95605/photo  ─→  bloqueia ─→ reutiliza ─→  200 OK
                                                                  ↑ 1 download MTProto
                                                                    compartilhado
```

`singleflight.Group.Do(key, fn)` garante que chamadas concorrentes com a mesma `key`
executam `fn` apenas uma vez. As demais bloqueiam e recebem o mesmo `(result, error)`.

A chave é `photo_id` (não `msg_id`) porque a mesma foto pode aparecer em múltiplas
mensagens (forwards, cross-channel).

---

## 5. API Endpoint (Fase 4)

```go
// internal/api/media.go

func (s *Server) handleMediaPhoto(w http.ResponseWriter, r *http.Request) {
    msgID, err := parseMsgID(r.URL.Path)
    if err != nil {
        http.Error(w, "invalid message id", http.StatusBadRequest)
        return
    }

    data, err := s.resolver.ResolveImage(r.Context(), msgID, 0)
    if err != nil {
        if errors.Is(err, media.ErrNoPhoto) {
            http.Error(w, "no photo", http.StatusNotFound)
            return
        }
        http.Error(w, "image unavailable", http.StatusServiceUnavailable)
        return
    }

    w.Header().Set("Content-Type", "image/jpeg")
    w.Header().Set("Cache-Control", "public, max-age=1800") // 30min
    w.Header().Set("Content-Length", strconv.Itoa(len(data)))
    w.Write(data)
}
```

---

## 6. Schema — Migration 007

```sql
-- migrations/007_media_metadata.sql
-- Metadados MTProto para resolução de imagens sob demanda.
-- Ver ADR 011.

ALTER TABLE processed_messages ADD COLUMN photo_access_hash INTEGER DEFAULT 0;
ALTER TABLE processed_messages ADD COLUMN photo_file_ref TEXT DEFAULT '';
ALTER TABLE processed_messages ADD COLUMN photo_dcid INTEGER DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_processed_media
    ON processed_messages(photo_id) WHERE photo_id > 0;
```

---

## 7. Extração no Normalizer

```go
// internal/processor/normalizer.go

func extractPhotoMetadata(msg map[string]any) (int64, int64, string, int) {
    media, ok := msg["Media"].(map[string]any)
    if !ok {
        return 0, 0, "", 0
    }
    photo, ok := media["Photo"].(map[string]any)
    if !ok {
        return 0, 0, "", 0
    }
    photoID := toInt64(photo["ID"])
    accessHash := toInt64(photo["AccessHash"])
    fileRef, _ := photo["FileReference"].(string)
    dcid := toInt(photo["DCID"])
    return photoID, accessHash, fileRef, dcid
}
```

---

## 8. Edge cases

| Caso | Comportamento |
|------|---------------|
| Mensagem sem foto | `PhotoMetadata{}` vazio. Endpoint retorna 404 |
| Forward de outro canal | `photo_id` + `access_hash` são globais. Funciona |
| `file_reference` expirado | L3 soft → L3 hard. Transparente para o frontend |
| Canal deletado | L3 hard falha. Log warning. Endpoint retorna 503 |
| Imagem >10MB | Download limita ao Size "x" (800px, ~50KB). Não baixa full resolution |
| Cold start (cache vazio) | Requests iniciais têm latência MTProto (~200-500ms). Singleflight dedup |
| Restart do processo | Cache esvazia. Comportamento idêntico ao cold start |
| `photo_id = 0` | Mensagem sem mídia. Skipped no normalizer |

---

## 9. Ordem de implementação

### Fase A — Metadados (processor, sem MTProto)

1. Migration 007
2. `extractPhotoMetadata` no normalizer
3. Repository: INSERT com novas colunas + `GetPhotoMetadata` + `UpdateFileReference` + `UpdatePhotoMetadata`
4. Reprocessar (backfill dos 10.000+ payloads)

### Fase B — Resolver (requer MTProto, no orquestrador)

5. Pacote `internal/media` com `MediaResolver`, `ImageCache`, `singleflight`
6. `MediaClient` implementation sobre `gotd/td`
7. Testes com mock de `MediaClient`

### Fase C — API endpoint (Fase 4)

8. `GET /api/media/{msg_id}/photo` no `limiar-api`
9. Integração com o orquestrador

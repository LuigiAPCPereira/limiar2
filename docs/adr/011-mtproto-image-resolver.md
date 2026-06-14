# ADR 011 — Subsistema de Resolução de Imagens (v2)

## Status

**Supercedido** (v1 supercedida por v2)

## Contexto (v1 — supercedida)

A abordagem original (proxy MTProto sob demanda com L1→L2→L3→L4) falhou porque
`FILE_REFERENCE_EXPIRED` é **irrecuperável** para mensagens antigas via MTProto.
O `file_reference` do Telegram expira em horas/dias e não pode ser renovado para
mensagens que não aparecem em updates recentes da sessão.

## Decisão (v2 — Inline Thumbnail + Download Proativo)

### Inline Thumbnail — Source of Truth Universal

O Telegram inclui um thumbnail inline compacto (~230 bytes, base64) no payload de
**cada mensagem com foto** (campo `Media.Photo.Sizes[]` onde `Type == "i"`). Este
thumbnail é **sempre disponível** — não depende de `file_reference` MTProto.

- Extraído pelo Processor do `raw_messages.payload`
- Persistido em `processed_messages.inline_thumb` (BLOB)
- Cobertura: **100%** das mensagens com foto
- Latência: sub-ms (query indexada)
- Zero download necessário

### Download Proativo — Full-res Opcional

O Collector baixa a imagem full-res (size "x", 800px) **no momento da chegada**
da mensagem — a única janela onde `file_reference` MTProto é válido.

- Executado em goroutine assíncrona (não bloqueia dbWriter)
- Resultado armazenado no cache in-memory compartilhado
- Disponível para o frontend via `MediaResolver.ResolveImage()`
- **Opcional**: se falhar, o inline thumb serve como fallback universal

### Cache In-Memory — hashicorp/golang-lru

```go
expirable.NewLRU[int64, []byte](500, nil, 30*time.Minute)
```

- LRU bounded (500 entradas, ~25MB a 50KB/imagem)
- TTL 30 minutos
- Compartilhado entre Collector, Processor e API (mesmo processo)
- Thread-safe (sync.Mutex interno)

### API de Mídia — Fallback em Cadeia

```
GET /api/v1/media/{photo_id}?size=full
1. cache.Get(photoID)    → 200 image/jpeg + X-Source: collector-cache
2. DB inline_thumb       → 200 image/jpeg + X-Source: inline-thumb
3. 404
```

O frontend mostra o inline thumb instantaneamente; faz upgrade lazy para full-res
no clique/hover (se disponível no cache).

## Schema

```sql
-- Migration 007
ALTER TABLE processed_messages ADD COLUMN inline_thumb BLOB;
CREATE INDEX IF NOT EXISTS idx_processed_inline_thumb
    ON processed_messages(photo_id) WHERE inline_thumb IS NOT NULL;
```

## Consequências

### Positivas
- **100% de cobertura**: inline thumb sempre disponível para qualquer mensagem com foto
- **Latência sub-ms**: query indexada, zero download
- **Zero gestão de disco**: tudo in-memory
- **Simplicidade radical**: cadeia cache → DB → 404 (vs L1→L2→L3→L4 anterior)
- **Sem dependência de sessão MTProto ativa**: inline thumb funciona mesmo sem collector

### Negativas
- Inline thumb é ~230 bytes (thumbnail compacto, não full-res)
- Full-res depende de download proativo no momento da chegada (janela finita)
- Restart do processo esvazia o cache (cold start: inline thumb como fallback)

## Dependências

- `github.com/hashicorp/golang-lru/v2/expirable` — única dependência externa nova
- Zero escrita em disco para imagens (constraint não-negociável)

## O que foi removido (v1 → v2)

- L2: Download MTProto sob demanda (`upload.GetFile`)
- L3: Renovação de `file_reference` (soft + hard)
- L4: Scraping da página pública do canal
- `ScrapePhotoURL`, `DownloadHTTP`, `RefetchAndDownload`, `RenewFileReference`
- `singleflight.Group` (desnecessário com download proativo)

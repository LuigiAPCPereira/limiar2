# Design — Subsistema de Resolução de Imagens (MEDIA-TD v3)

> **ADR:** [011-mtproto-image-resolver.md](../adr/011-mtproto-image-resolver.md)
> **Depende de:** ADR 009 (orquestrador), ADR 012 (sessão MTProto persistente)
> **Supercede:** MEDIA-TD v2 (inline thumbnail + cache RAM apenas)

---

## 1. Visão geral

O subsistema de imagens resolve uma imagem pública a partir de
`processed_messages.id`, não de `photo_id`. Isso evita que dois registros distintos
com metadados corrompidos ou arredondados compartilhem a mesma imagem por acidente.

Fluxo efetivo:

```text
Collector
  SaveRawMessageBatch(raw payload) ──commit──┐
  ExtractPhotoRequest(raw payload)           │
  DownloadPhoto(size "x", ~800px)            │
  imageCache.Put(photo_id, jpeg)             │
                                             ▼
Processor
  DecodePayloadMap(UseNumber)
  ExtractPhotoFields(raw payload)
  ExtractInlineThumb(Type "i")
  SaveProcessedBatch(processed_messages + inline_thumb + metadados MTProto)
  CleanExpiredPhotoCache()
                                             ▼
Dashboard / API interna (request HTTP read-only)
  GET /api/media/{processed_messages.id}
  1. L1 cache RAM compartilhado por photo_id exato
  2. L2 photo_cache persistido por photo_id exato
  3. L3 inline_thumb da mensagem processada
  4. 404 ErrNoPhoto

Operações autenticadas (Collector proativo, `limiar media resolve`, `limiar media backfill`)
  Download MTProto por metadados exatos do raw payload
  Preenche cache RAM e/ou photo_cache antes do dashboard servir a imagem

O thumbnail inline garante cobertura. A imagem preferencial no dashboard é o JPEG
800px já presente no cache RAM ou no `photo_cache`; downloads MTProto acontecem no
Collector ou em comandos operacionais, não durante requests HTTP.

---

## 2. Componentes

### 2.1 Cache (`internal/media/cache.go`)

```go
type Cache struct {
    lru *expirable.LRU[int64, []byte]
}

func NewCache(size int, ttl time.Duration) *Cache
func (c *Cache) Get(photoID int64) ([]byte, bool)
func (c *Cache) Put(photoID int64, data []byte)
func (c *Cache) Len() int
```

Default no provider:

- 500 entradas;
- TTL de 30 minutos;
- thread-safe via `expirable.LRU`;
- compartilhado entre Collector e Dashboard Resolver no processo `limiar run`.

### 2.2 Resolver (`internal/media/resolver.go`)

Contrato:

```go
func (r *Resolver) ResolveImage(ctx context.Context, processedMsgID int64) ([]byte, string, error)
func (r *Resolver) PutCache(photoID int64, data []byte)
```

Cadeia de fallback:

1. `repo.GetPhotoMetadata(processedMsgID)` para obter `photo_id` exato.
2. `cache.Get(photoID)` → `source="cache"`.
3. `repo.GetPhotoData(photoID)` → `source="photo-cache"`.
4. Se `client != nil`, `client.DownloadPhoto(req)` → salva L1/L2 best-effort e `source="downloaded"`.
5. `repo.GetInlineThumb(processedMsgID)` → expande thumbnail e `source="inline-thumb"`.
6. Sem dados → `ErrNoPhoto`.

No dashboard, `client` é nil. Portanto request HTTP nunca abre MTProto nem grava
`photo_cache`; a camada L3 de download só vale para Collector/CLI.

Regra crítica: quando o raw payload está disponível, o resolver usa o `photo_id`
extraído dele. A coluna `processed_messages.photo_id` é fallback para registros
legados sem payload relível.

### 2.3 Client (`internal/media/media.go`)

```go
type Client interface {
    DownloadPhoto(ctx context.Context, req PhotoDownloadRequest) ([]byte, error)
    DownloadPhotoBatch(ctx context.Context, reqs []PhotoDownloadRequest, handler func(photoID int64, data []byte, err error)) error
}
```

Implementação concreta: `internal/telegram.MediaClient`, usando gotd/td sem vazar
tipos `tg.*` para fora de `internal/telegram`.

### 2.4 Repository (`internal/media/media.go`)

```go
type Repository interface {
    GetPhotoID(ctx context.Context, processedMsgID int64) (int64, error)
    GetInlineThumb(ctx context.Context, processedMsgID int64) ([]byte, error)
    GetPhotoData(ctx context.Context, photoID int64) ([]byte, error)
    SavePhotoData(ctx context.Context, photoID int64, data []byte) error
    GetPhotoMetadata(ctx context.Context, processedMsgID int64) (*model.PhotoMetadata, error)
}
```

`GetPhotoMetadata` relê `raw_messages.payload` quando possível para recuperar
`photo_id`, `access_hash`, `file_reference` e `dc_id` como inteiros de 64 bits
sem arredondamento.

---

## 3. Extração no Processor

O Processor extrai somente dados determinísticos do payload bruto:

- `InlineThumb []byte` de `Media.Photo.Sizes[]` com `Type == "i"`;
- `PhotoID`, `PhotoAccessHash`, `PhotoFileRef`, `PhotoDCID`;
- campos temporais explícitos (`ValidFrom`, `ValidUntil`, `Flash`,
  `RecurrencePattern`, `SeasonalTag`) para melhorar o feed sem chamar LLM.

`model.DecodePayloadMap` usa `json.Decoder.UseNumber()` para preservar IDs MTProto
maiores que 53 bits. Nunca use `json.Unmarshal` direto para payloads brutos que
contêm `photo_id` ou `access_hash`.

---

## 4. Download proativo no Collector

O Collector só baixa bytes de mídia; ele não interpreta conteúdo da mensagem.

Pontos de execução:

1. `dbWriter.flush` commita o lote em `SaveRawMessageBatch`.
2. Para cada mensagem realmente inserida, chama `proactiveDownload`.
3. `proactiveDownload` extrai `PhotoDownloadRequest` do payload bruto.
4. Baixa via `media.Client.DownloadPhoto`.
5. Popula o `media.Cache` compartilhado por `photo_id`.

Duplicatas (`ON CONFLICT DO NOTHING`) não disparam download. Mensagens sem foto não
disparam download. Se `SetMediaDownload` não foi chamado, o caminho é no-op.

---

## 5. photo_cache e backfill

`photo_cache` guarda JPEGs 800px no banco:

```sql
CREATE TABLE IF NOT EXISTS photo_cache (
    photo_id INTEGER PRIMARY KEY,
    data BLOB NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT
);
```

`SavePhotoData` grava `expires_at = datetime('now', '+30 days')`.
`CleanExpiredPhotoCache` remove apenas linhas com `expires_at` vencido; linhas
legadas com `expires_at NULL` permanecem.

`limiar media backfill` usa `ListPendingPhotoMetadataForBackfill` e
`DownloadPhotoBatch` para popular `photo_cache` em lote. O lote usa uma única
conexão MTProto, limite baixo de concorrência de download e handler serial para
persistência.

---

## 6. Schema

Banco novo: schema consolidado em `internal/storage/migrations/001_initial.sql`.

Campos de mídia em `processed_messages`:

```sql
photo_id INTEGER NOT NULL DEFAULT 0,
photo_access_hash INTEGER NOT NULL DEFAULT 0,
photo_file_ref TEXT NOT NULL DEFAULT '',
photo_dcid INTEGER NOT NULL DEFAULT 0,
inline_thumb BLOB
```

Índices:

```sql
idx_processed_has_photo ON processed_messages(photo_id) WHERE photo_id > 0;
idx_photo_cache_expires ON photo_cache(expires_at) WHERE expires_at IS NOT NULL;
```

Migrações antigas `002_*` a `010_*` foram removidas junto com bancos locais de
desenvolvimento. Bancos legados com `schema_migrations` dessas versões são
compatibilizados na abertura do banco: `storage.migrate` adiciona apenas colunas
faltantes do schema consolidado e `007_legacy_schema_compat.sql` registra o marco
atual sem reexecutar `001_initial.sql`.

---

## 7. Edge cases

| Caso | Comportamento |
|------|---------------|
| Mensagem sem foto | `InlineThumb = nil`; resolver retorna `ErrNoPhoto` se L1/L2 e inline thumb também não têm dados |
| Foto sem `Type == "i"` | Pode aparecer se o Collector/CLI já baixou para cache; sem cache retorna `ErrNoPhoto` |
| Download proativo falha | L1 não popula; dashboard tenta L2 e inline thumb; CLI/backfill pode popular L2 depois |
| Cache RAM vazio após restart | Resolver do dashboard tenta `photo_cache` e inline thumb, sem download on-demand |
| `FILE_REFERENCE_EXPIRED` | `internal/telegram.MediaClient` tenta renovar via `ChannelID`/`MessageID` quando possível |
| Payload legado com `photo_id` arredondado | `GetPhotoMetadata` usa raw payload exato; coluna processada é fallback |
| Produto com foto duplicada por raw duplicado | `processed_messages.id` isola inline thumb por mensagem |

---

## 8. Constraints não-negociáveis

- Zero arquivos de imagem soltos no filesystem.
- Bytes persistentes de imagem ficam apenas em Turso (`photo_cache`) ou no payload/processado.
- Collector/MediaClient é o único ponto de download full-res via MTProto.
- Dashboard não importa `internal/telegram`.
- Processor não depende de sessão Telegram e não chama LLM.
- Placeholders SQL continuam sendo `?`.

# ADR 011 — Subsistema de Resolução de Imagens

## Status

**Aceito** — v3 substitui a v2 inline-only.

## Contexto

Mensagens promocionais do Telegram carregam fotos com dois caminhos distintos:

1. `Media.Photo.Sizes[Type=="i"]`: thumbnail inline compacto, embutido no payload bruto.
2. `upload.GetFile`: download MTProto de uma variante maior (`ThumbSize == "x"`, ~800px), dependente de `photo_id`, `access_hash`, `dc_id` e `file_reference`.

A v2 resolveu a cobertura usando `inline_thumb`, mas a qualidade ficou baixa. Além disso,
bancos antigos tiveram `photo_id`/`access_hash` arredondados quando JSON foi decodificado
via `float64`; isso podia fazer produtos diferentes colidirem no mesmo identificador de foto.

## Decisão

Usar fallback em quatro camadas, sempre endereçando a imagem pública por
`processed_messages.id`:

```text
GET /api/media/{processed_messages.id}  (dashboard/API interna read-only)
1. L1 cache RAM compartilhado        → JPEG 800px baixado nesta sessão
2. L2 photo_cache no Turso            → JPEG 800px persistido em BLOB
3. L3 processed_messages.inline_thumb → thumbnail inline expandido para JPEG
4. ErrNoPhoto                         → 404

Download MTProto (`upload.GetFile`) não ocorre durante request HTTP. Ele popula
L1/L2 pelo Collector proativo ou por comandos operacionais autenticados
(`limiar media resolve`, `limiar media backfill`).
```

### Inline thumbnail

- Extraído pelo Processor de `raw_messages.payload`.
- Persistido em `processed_messages.inline_thumb`.
- Servido por `processed_messages.id`, nunca por `photo_id`, para não misturar produtos.
- É fallback universal, mas não é a imagem preferencial por qualidade.

### Cache RAM compartilhado

- `media.Cache` usa LRU bounded + TTL de 30 minutos.
- No `limiar run`, o mesmo cache é injetado no Collector e no Dashboard Resolver.
- O download proativo do Collector popula este cache após commit do batch.

### photo_cache persistido

- Tabela `photo_cache(photo_id PRIMARY KEY, data BLOB, updated_at, expires_at)`.
- `SavePhotoData` grava JPEG 800px com TTL de 30 dias (`expires_at`).
- `CleanExpiredPhotoCache` remove apenas linhas vencidas; `expires_at NULL` nunca expira.
- O cache persistido evita voltar para inline thumbnail após restart.

### Preservação de IDs MTProto

- `model.DecodePayloadMap` usa `json.Decoder.UseNumber()`.
- `ProcessorRepository.GetPhotoMetadata` prefere reler `raw_messages.payload` para
  obter `photo_id`, `access_hash`, `file_reference` e `dc_id` exatos.
- O Resolver consulta `photo_cache` com o `photo_id` exato dos metadados; só usa o
  valor persistido na coluna como fallback.

## Schema

O schema novo é consolidado em `internal/storage/migrations/001_initial.sql`.

```sql
processed_messages (
    ..., photo_id, photo_access_hash, photo_file_ref, photo_dcid, inline_thumb,
    ...
)

photo_cache (
    photo_id   INTEGER PRIMARY KEY,
    data       BLOB NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT
)
```

Índices relevantes:

```sql
idx_processed_has_photo ON processed_messages(photo_id) WHERE photo_id > 0;
idx_photo_cache_expires ON photo_cache(expires_at) WHERE expires_at IS NOT NULL;
```

## Consequências

### Positivas

- Dashboard deixa de depender do thumbnail inline quando há cache disponível.
- Imagens reais sobrevivem a restart via `photo_cache`.
- Produtos não compartilham imagem por colisão de `photo_id` arredondado quando o raw payload está disponível.
- Sem arquivos soltos de imagem no filesystem.

### Negativas

- `photo_cache` aumenta o tamanho do `limiar.db`; TTL/cleanup mitigam crescimento.
- Download MTProto pode falhar nos caminhos autenticados (Collector/CLI); nesses casos o dashboard cai para `photo_cache` ou `inline_thumb`.
- `inline_thumb` continua sendo fallback de baixa qualidade quando L1/L2 não têm dados.

## Constraints

- Nenhum tipo do gotd/td vaza para fora de `internal/telegram`.
- Collector não interpreta conteúdo; ele só usa metadados MTProto para baixar bytes de mídia.
- Processor permanece determinístico; extração de thumbnail/metadados vem do payload bruto.
- Dashboard é read-only: não abre MTProto e não persiste `photo_cache` em request HTTP.
- Nenhuma imagem persistente é gravada fora do banco Turso.

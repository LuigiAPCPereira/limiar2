# Ideação — limiar-processor (Fase 2)

**Data:** 2026-06-09 (atualizado após análise de 6674 mensagens)
**Status:** Ideação refinada com base em dados reais — pronto para Sprint 1
**Base de dados:** 6674 mensagens, 16 canais, 70 dias (2026-03-30 → 2026-06-08)
**Pré-requisitos cumpridos:** ✅ Análise de payloads completa (Opção A + B + C executadas)

---

## 1. Visão Geral

O `limiar-processor` é o segundo binário do pipeline Limiar. Ele consome
mensagens brutas persistidas pelo `limiar-collector` (tabela `raw_messages`),
transforma-as em dados estruturados, e produz uma saída limpa pronta para
consumo pelo `limiar-api`.

**Linguagem:** Go  
**Binário separado:** `cmd/limiar-processor/main.go`  
**Banco:** Mesmo arquivo `limiar.db` (Tursogo), tabelas novas  
**Deployment:** A definir (pode rodar no mesmo servidor ou separado)

---

## 2. Pipeline de Estágios (Ordem de Execução)

```
raw_messages (collector)
       │
       ▼
┌─────────────────────┐
│ 1. NORMALIZAÇÃO     │  Extrair mensagem de wrappers, limpar campos,
│                     │  produzir estrutura canônica
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│ 2. RESOLUÇÃO URLs   │  Seguir redirects, limpar tracking params,
│                     │  identificar merchant, obter URL canônica
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│ 3. RE-AFILIAÇÃO     │  Gerar link de afiliado próprio para cada
│                     │  merchant (Playwright/API conforme plataforma)
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│ 4. IMAGENS          │  Extrair inline thumb + metadados MTProto
│                     │  para resolver via cache/download autenticado
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│ 5. DEDUPLICAÇÃO     │  Agrupar por produto canônico, manter
│                     │  melhor oferta, detectar duplicatas cross-channel
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│ 6. PRICE TRACKING   │  Registrar variações de preço ao longo
│                     │  do tempo por produto
└─────────┬───────────┘
          │
          ▼
┌─────────────────────┐
│ 7. CLASSIFICAÇÃO    │  Categorizar produto, determinar se é
│                     │  promocional (regex → LLM em fases futuras)
└─────────┬───────────┘
          │
          ▼
processed_messages + products + price_history
```

---

## 3. Estágio 1: Normalização

### Objetivo

Transformar qualquer payload bruto (Shape A: mensagem direta, Shape B: wrapper
Updates) em uma estrutura canônica única.

### Regras

1. **Detectar wrapper:** Se payload tem campo `Updates`, extrair
   `Updates[0].Message` como mensagem real
2. **Validar campos obrigatórios:** `ID`, `PeerID.ChannelID`, `Date`, `Message`
3. **Limpar texto:**
   - Normalizar whitespace (3+ newlines → 2, NBSP → SPACE)
   - Preservar emojis (são features úteis para classificação)
   - Manter formatação original (negrito, itálico via Entities)
4. **Converter tipos:**
   - `Date` (Unix int) → ISO8601 UTC
   - `PeerID.ChannelID` → int64
   - `Entities` null → array vazio
5. **Descartar campos irrelevantes:** Flags, bitmasks, campos de UI do Telegram
   (ver lista completa no payload-analysis-report.md, seção 4.3)

### Saída do estágio

```go
type NormalizedMessage struct {
    RawMessageID    int64     // FK para raw_messages.id
    MessageID       int64     // ID original do Telegram
    ChannelID       int64     // PeerID.ChannelID
    ChannelUsername string    // lookup em channels table
    PostedAt        time.Time // Date convertido
    ReceivedAt      time.Time // raw_messages.received_at
    Text            string    // Message limpo
    TextLength      int
    MediaType       string    // "photo" | "poll" | "video" | "document" | "webpage" | "none"
    PhotoID         int64     // Media.Photo.ID (se aplicável)
    PhotoSizes      []PhotoSize
    Entities        []Entity  // Entities parseadas (offset, length, type, url)
    Views           int
    Forwards        int
    GroupedID       int64     // Para detecção de álbuns (sempre 0 nos dados atuais)
    ReplyToMsgID    int64     // ReplyTo.ReplyToMsgID (0 se null)
    WebpageURL      string    // Media.Webpage.URL (se MediaType == "webpage")
    WebpageTitle    string    // Media.Webpage.Title
    WebpageDesc     string    // Media.Webpage.Description
}

// MessageType — classificador exclusivo (cada mensagem pertence a 1 tipo).
// Ver §6.5 para definição e cascata.
type MessageType string

const (
    TypeDealComplete    MessageType = "deal_complete"
    TypeDealNoCoupon    MessageType = "deal_no_coupon"
    TypeDealNoPrice     MessageType = "deal_no_price"
    TypeCategoryHeader  MessageType = "category_header"
    TypeCouponExpired   MessageType = "coupon_expired"
    TypeCommentary      MessageType = "commentary"
    TypeVideo           MessageType = "video"
    TypeDocument        MessageType = "document"
    TypePoll            MessageType = "poll"
    TypeAdminMeta       MessageType = "admin_meta"
)
```

---

## 4. Estágio 2: Resolução de URLs

### Objetivo

Transformar URLs de afiliados de terceiros em URLs canônicas de produto.

### Fluxo

```
URL original (shortened/affiliate)
  → HTTP HEAD/GET com follow redirects (max 10 hops)
  → URL final (página do produto)
  → Remover parâmetros de tracking (UTM, affiliate IDs, session tokens)
  → URL canônica limpa
```

### Merchants prioritizados (baseado em análise de 6674 mensagens, 2026-06-09)

Prioridade calibrada por **volume real de URLs** e **facilidade de resolução**:

| Prio | Merchant | Domínios shortened | Domínio canônico | Volume | Redirects | Dificuldade |
|------|----------|-------------------|------------------|--------|-----------|-------------|
| **P1** | Shopee | `s.shopee.com.br` | `shopee.com.br` | 2805 (47%) | 1 | Fácil — params claros (`mmp_pid`, `utm_*`) |
| **P1** | Amazon | `amzn.to`, `amzn.divulgador.link`, `amzlink.to` | `amazon.com.br` | 2347 (39%) | 1-2 | Fácil — **ASIN extraível do path** (`/dp/{ASIN}`) |
| **P2** | Kabum (AWIN) | `tidd.ly` | `kabum.com.br` | 51 (1%) | 2 | Médio — AWIN affiliate network |
| **P3** | Mercado Livre | `meli.la`, `mercadolivre.com/sec/*` | `mercadolivre.com.br` | 2190 (36%) | 1 | **Difícil** — `meli.la` cai em perfil de afiliado, não produto |
| **P3** | AliExpress | `s.click.aliexpress.com` | `aliexpress.com` | 51 (1%) | 1 | **Difícil** — cai em landing genérica de moedas, não produto |
| **P4** | Magalu | `magazineluiza.onelink.me`, `divulgador.magalu.com` | `magazineluiza.com.br` | ~200 (3%) | 1-2 | **Muito difícil** — **HTTP 403** anti-bot, precisa Playwright |
| — | Cross-platform | `bit.ly`, `chat.whatsapp.com` | varia | ~90 | 2 | Fácil (não-merchant, apenas cross-promo) |

**Notas:**
- Volumes somam >100% porque uma mensagem pode ter múltiplas URLs (ex.: cupom + produto)
- `meli.la` requer investigação adicional: talvez seja necessário extrair o product_id do path ou usar outro signal
- Magalu P4: adiar para Sprint 5+; usar Playwright headless com User-Agent real
- `amzn.divulgador.link` e `amzlink.to` resolvem para a mesma estrutura Amazon (`/dp/{ASIN}?tag=...`), tags de afiliado diferentes

### Parâmetros a remover (limpeza de tracking)

```
# Genéricos
utm_source, utm_medium, utm_campaign, utm_term, utm_content
fbclid, gclid, dclid, msclkid

# AliExpress
aff_id, aff_platform, sk, dp, af, cv, cn, btsid

# Amazon
tag, linkId, linkCode, ref_, camp, creative

# Mercado Livre
matt_tool, matt_word, matt_campaign

# Shopee
affiliate_id, sub_id
```

### Considerações técnicas

- **Rate limiting:** Merchants podem bloquear requests excessivos. Implementar
  backoff exponencial e cache agressivo de URLs já-resolvidas.
- **Timeout:** Max 5s por resolução. Se falhar, manter URL original e marcar
  como `unresolved`.
- **User-Agent:** Usar User-Agent de browser real para evitar bloqueios.
- **Cache:** URLs **canônicas** (produto final) são estáveis — cachear
  indefinidamente em tabela `url_resolutions(original_url, canonical_url,
  merchant, resolved_at)`. No entanto, **URLs shortened diferentes** podem
  apontar pro mesmo produto (ex.: um canal usa `amzn.to/X` e outro usa
  `amzlink.to/Y` para o mesmo ASIN). Por isso a dedup é feita no hash da
  **canonical_url**, não da original.

---

## 5. Estágio 3: Re-afiliação

### Objetivo

Substituir o link de afiliado do canal original por um link de afiliado próprio
do Limiar, garantindo rastreabilidade e monetização.

### Fluxo

```
URL canônica do produto
  → Identificar merchant (pelo domínio)
  → Usar driver específico do merchant para gerar link de afiliado
  → URL final com affiliate ID do Limiar
```

### Drivers por merchant

#### Mercado Livre

**Método:** Automação de browser (Playwright/Rod/Chromedp)  
**Motivo:** ML não oferece API pública para geração de links de afiliado  
**Fluxo:**
1. Autenticar no painel de afiliados do Mercado Livre (sessão persistida)
2. Navegar ao gerador de links
3. Colar URL canônica do produto
4. Extrair link gerado
5. Cachear resultado

**Considerações:**
- Sessão do ML expira → precisa de re-auth automático
- Rate limit: não gerar mais de X links/minuto para não triggerar captcha
- Fallback: se automação falhar, marcar como `pending_affiliation` e retentear

#### Amazon Associates

**Método:** Verificar se há API oficial; se não, construção manual da URL  
**Formato típico:** `https://www.amazon.com.br/dp/{ASIN}?tag={AFFILIATE_TAG}`  
**Considerações:**
- Extrair ASIN da URL canônica
- Concatenar `?tag=SEU_TAG` é rastreável pela Amazon? Validar.
- Se não for rastreável, usar automação similar ao ML

#### AliExpress

**Método:** Verificar API do programa de afiliados AliExpress  
**Considerações:**
- AliExpress tem API de geração de links (Portals API)
- Se disponível, preferir API sobre automação
- Verificar limites de chamadas

#### Shopee

**Método:** A definir — verificar programa de afiliados Shopee Brasil  
**Considerações:**
- Shopee muda de políticas frequentemente
- Verificar se permite geração programática de links

### Configuração

```env
# Credenciais de afiliados (novas variáveis de ambiente)
LIMIAR_ML_AFFILIATE_USER=...
LIMIAR_ML_AFFILIATE_PASS=...
LIMIAR_AMAZON_AFFILIATE_TAG=...
LIMIAR_ALIEXPRESS_APP_KEY=...
LIMIAR_ALIEXPRESS_APP_SECRET=...
LIMIAR_SHOPEE_AFFILIATE_ID=...
```

### Processamento assíncrono

A re-afiliação é o estágio mais lento (especialmente ML com Playwright). O deal
deve aparecer no feed com URL canônica imediatamente e ser atualizado com URL
de afiliado conforme disponível:

```
Estado 1: deal aparece com canonical_url (imediato)
Estado 2: deal atualizado com affiliate_url (segundos/minutos depois)
```

Isso evita que o pipeline inteiro fique bloqueado esperando o Playwright.

---

## 6. Estágio 4: Imagens via MTProto + cache local

> Atualização: a proposta antiga de CDN público (`telesco.pe`/`og:image`) foi
> superada por [MEDIA-TD v3](MEDIA-TD.md) e ADR 011. O projeto não faz scraping de
> páginas públicas do Telegram para resolver imagens.

### Objetivo

Entregar imagem de produto com qualidade aceitável sem duplicar fotos entre
produtos e sem criar arquivos soltos no filesystem.

### Como funciona

- O Processor extrai `inline_thumb` e metadados MTProto (`photo_id`,
  `access_hash`, `file_reference`, `dc_id`) do payload bruto.
- O Collector baixa proativamente uma variante ~800px enquanto o
  `file_reference` ainda está fresco e popula o cache RAM compartilhado.
- O Resolver do dashboard recebe `processed_messages.id` e tenta, nessa ordem:
  cache RAM, `photo_cache`, `inline_thumb`, 404. Download MTProto sob demanda é restrito ao Collector/CLI.
- `photo_cache` persiste bytes JPEG no Turso com TTL de 30 dias.

### Por que não CDN público

- Scraping de preview público misturava responsabilidades de fase e dependeria de
  HTML externo instável.
- Canais privados ou previews indisponíveis quebrariam a resolução.
- O payload MTProto já contém os metadados necessários para download autenticado.
- Endereçar por `processed_messages.id` evita colisão visual entre produtos que
  tinham `photo_id` arredondado em bancos antigos.

### Implementação atual

```go
// GET /api/media/{processed_messages.id}
data, source, err := resolver.ResolveImage(ctx, processedMessageID)
```

`source` pode ser `cache`, `photo-cache`, `downloaded` ou `inline-thumb`.

### Dados de suporte (análise de 6674 mensagens)

- **5613 mensagens (93.5%) têm Media.Photo** — quase todas as promoções têm imagem
- **362 mensagens (6%) têm Media=null** — mensagens puramente textuais, sem imagem
- **Aspect ratios dominantes:**
  - 1:1 quadrado (85% das fotos) — placeholder quadrado no frontend
  - 16:9 (1.77, ~8%) — landscape
  - 1.9:1 (~7%) — wide banner
- **Dimensões disponíveis no payload:**
  - 320x320 (thumbnail "m")
  - 800x800 (média "x")
  - 1024x1024, 1200x1200, 1280x1280 (alta "y")

---

## 6.5. Taxonomia de Mensagens (validada em 6674 mensagens)

### Objetivo

Classificar cada mensagem em um dos tipos abaixo para que estágios posteriores
saibam como tratar (processar, ignorar, agregar). A taxonomia é **exclusiva**
(cada mensagem pertence a 1 cluster) — implementada como cascata de checks.

### Clusters identificados

| Cluster | Qtd | % | Definição (cascata) | Tratamento |
|---------|-----|---|---------------------|------------|
| **deal_complete** | 3491 | 58.2% | tem URL + preço + cupom | Pipeline completo (normalização → dedup → price) |
| **deal_no_coupon** | 1250 | 20.8% | tem URL + preço, sem cupom | Pipeline completo, `coupons: null` |
| **deal_no_price** | 1047 | 17.4% | tem URL, sem preço (landing page) | Pipeline parcial (sem price tracking) |
| **category_header** | 173 | 2.9% | texto < 50 chars, sem URL, sem preço, sem "esgotado" | Ignorar para feed; usar como contexto de thread |
| **coupon_expired** | 112 | 1.9% | menciona "esgotado/acabou/encerrado", sem URL | Sinalizar promoção original (via ReplyTo) como `expired: true` |
| **commentary** | 97 | 1.6% | texto livre, sem URL/preço/esgotado | Ignorar (ex.: "barato", "precinho demais") |
| **video** | 7 | 0.1% | Media.Video presente | Pipeline com `media_type: video` |
| **document** | 7 | 0.1% | Media.Document presente | Pipeline com `media_type: document` |
| **poll** | 1 | ~0% | Media.Poll presente | Ignorar (não-promoção) |
| **admin_meta** | ~18 | ~0.3% | regras_grupo, cupons_hoje, grupos_whatsapp, canal_telegram | Ignorar (meta-promoção do canal) |

### Cascata de classificação

```go
func Classify(m NormalizedMessage) MessageType {
    hasURL := HasURL(m.Text)
    hasPrice := HasPrice(m.Text)
    hasCoupon := HasCoupon(m.Text)
    isExpired := IsExpiredMention(m.Text)
    isCategoryHeader := len(m.Text) < 50 && !hasURL && !hasPrice && !isExpired
    isAdmin := IsAdminMeta(m.Text)

    switch {
    case isAdmin:
        return TypeAdminMeta
    case isExpired && !hasURL:
        return TypeCouponExpired
    case hasURL && hasPrice && hasCoupon:
        return TypeDealComplete
    case hasURL && hasPrice:
        return TypeDealNoCoupon
    case hasURL:
        return TypeDealNoPrice
    case isCategoryHeader:
        return TypeCategoryHeader
    case m.MediaType == "video":
        return TypeVideo
    case m.MediaType == "document":
        return TypeDocument
    case m.MediaType == "poll":
        return TypePoll
    default:
        return TypeCommentary
    }
}
```

### Dados de suporte (análise 2026-06-09)

- **ReplyTo chains:** 72 msgs com ReplyTo, todas com `ReplyToMsgID` válido
  (nunca `ReplyToPeerID` isolado). ReplyTo aponta pra mensagem anterior no
  mesmo canal. **Use ReplyTo para propagar `expired: true` de
  `coupon_expired` → deal original.**
- **GroupedID (álbuns):** ZERO ocorrências em 6003 payloads diretos. Processor
  **não precisa** lidar com álbuns (pelo menos nesse dataset).
- **Idioma:** 95% PT-BR puro, ~1% com títulos em EN (produto importado),
  ~0% ES. Não precisa de detector de idioma na Fase 2.

### Signals de urgência (badges pro frontend)

Detectar via regex, popular campo `urgency_signals: []string`:

| Signal | Regex | Volume | Badge sugerido |
|--------|-------|--------|----------------|
| "corre" / "corram" | `\bcorre\|\bcorram` | 194 | 🏃 Corra |
| "última unidade" / "acabando" | `última[s]? unidade\|acabando\|esgotando` | 17 | ⚠️ Últimas |
| "frete grátis" | `frete grátis\|frete gratis` | 297 | 🚚 Frete grátis |
| "envio nacional" / "envio do brasil" | `envio nacional\|envio do brasil` | 87 | 🇧🇷 Nacional |
| "tempo limitado" | `tempo limitado\|por tempo limitado` | 0 | (não detectado) |

---

## 7. Estágio 5: Deduplicação Inteligente

### Objetivo

Identificar o mesmo produto aparecendo em múltiplos canais e agrupar ofertas,
mantendo a melhor.

### Níveis de deduplicação

#### Nível 1: Mesma mensagem (já no collector)

- Chave: `(channel_id, message_id)` — constraint UNIQUE
- Implementado no collector, não precisa re-implementar

#### Nível 2: Mesma URL canônica (cross-channel)

- Chave: `SHA-256(canonical_url)`
- Mensagens de canais diferentes apontando pro mesmo produto
- **Regra:** Agrupar sob entidade `Product`, manter todas as ofertas mas
  destacar a de menor preço

#### Nível 3: Mesmo produto, URLs diferentes

- **Cenário:** Mesmo produto vendido no ML e na Amazon (URLs diferentes)
- **Abordagem futura:** Matching por nome de produto (fuzzy, embeddings)
- **Fase 2:** Não implementar, apenas preparar a interface

### Entidade Product

```go
type Product struct {
    ID              int64
    CanonicalURL    string    // URL limpa do produto (chave de dedup)
    URLHash         string    // SHA-256 da canonical_url (índice)
    Merchant        string    // "mercadolivre", "amazon", "aliexpress", "shopee"
    Name            string    // Extraído do texto (best-effort)
    CurrentPrice    int64     // Preço mais recente (centavos)
    LowestPrice     int64     // Menor preço histórico (centavos)
    AffiliateURL    string    // URL com affiliate ID do Limiar
    ImageRef        int64     // processed_messages.id usado por /api/media/{id}
    FirstSeenAt     time.Time
    LastSeenAt      time.Time
    OfferCount      int       // Quantas vezes apareceu (cross-channel)
}
```

### Fluxo de deduplicação

```
Mensagem normalizada
  → Resolver URL canônica
  → Calcular hash da URL
  → Buscar Product por hash
  → Se existe: atualizar preço, incrementar offer_count, linkar mensagem
  → Se não existe: criar Product novo, linkar mensagem
```

### Regra de "melhor oferta"

Quando múltiplas mensagens apontam pro mesmo produto:
- **Menor preço** ganha destaque principal
- **Todas as ofertas** ficam visíveis (pode ser útil saber que ML tem por X e
  Amazon por Y)
- **Mais recente** desempata se preços forem iguais

---

## 8. Estágio 6: Price Tracking

### Objetivo

Registrar variações de preço de um produto ao longo do tempo, permitindo ao
frontend mostrar histórico de preços e detectar se uma "promoção" é realmente
um bom preço.

### Modelo de dados

```go
type PricePoint struct {
    ID          int64
    ProductID   int64     // FK para products.id
    Price       int64     // Preço detectado (centavos)
    Currency    string    // "BRL"
    ChannelID   int64     // Canal de onde veio
    MessageID   int64     // Mensagem de onde extraiu
    DetectedAt  time.Time // Quando foi visto
}
```

### Tabela SQL

```sql
CREATE TABLE price_history (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id  INTEGER NOT NULL REFERENCES products(id),
    price       INTEGER NOT NULL,
    currency    TEXT NOT NULL DEFAULT 'BRL',
    channel_id  INTEGER NOT NULL,
    message_id  INTEGER NOT NULL,
    detected_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(product_id, channel_id, message_id)
);

CREATE INDEX idx_price_history_product ON price_history(product_id, detected_at);
```

### Funcionalidades derivadas

1. **Menor preço histórico:** `SELECT MIN(price) FROM price_history WHERE product_id = ?`
2. **Preço médio:** Para determinar se oferta atual é boa
3. **Tendência:** Preço subindo ou descendo ao longo do tempo
4. **Alerta de preço baixo:** Se preço atual < menor histórico, é destaque

---

## 9. Modelo de Dados (Tabelas Novas)

> **DECISÃO (2026-06-09):** Preços são armazenados como `INTEGER` (centavos de BRL).
> Exemplo: R$ 83,50 → 8350. Conversão float→int apenas nas boundaries (input do
> texto, output pro frontend). Motivo: float64 acumula erro de arredondamento em
> comparações e agregações (MIN, AVG). Padrão fintech. Os campos afetados são
> `products.current_price`, `products.lowest_price`, `price_history.price`, e
> `processed_messages.price_amount` — todos `INTEGER` no schema abaixo.

O processor adiciona 4 tabelas ao `limiar.db`. As tabelas do collector
(`raw_messages`, `channels`, `peers`, `sessions`) permanecem intocáveis.

```sql
-- Mensagens processadas (saída do pipeline completo)
CREATE TABLE processed_messages (
    id               INTEGER PRIMARY KEY AUTOINCREMENT,
    raw_message_id   INTEGER NOT NULL REFERENCES raw_messages(id),
    product_id       INTEGER REFERENCES products(id),
    channel_id       INTEGER NOT NULL,
    message_id       INTEGER NOT NULL,
    message_type     TEXT NOT NULL DEFAULT 'deal_complete',
        -- Ver §6.5: deal_complete | deal_no_coupon | deal_no_price |
        -- category_header | coupon_expired | commentary | video |
        -- document | poll | admin_meta
    text_clean       TEXT NOT NULL,
    text_length      INTEGER NOT NULL DEFAULT 0,
    media_type       TEXT NOT NULL DEFAULT 'none',
    image_url        TEXT,
    views            INTEGER NOT NULL DEFAULT 0,
    forwards         INTEGER NOT NULL DEFAULT 0,
    reply_to_msg_id  INTEGER NOT NULL DEFAULT 0,
        -- Para propagação de expired=true em coupon_expired
    webpage_url      TEXT,
    webpage_title    TEXT,
    webpage_desc     TEXT,
        -- Preenchidos quando Media.Webpage presente (0.3% das mensagens)
    has_price        INTEGER NOT NULL DEFAULT 0,
    has_coupon       INTEGER NOT NULL DEFAULT 0,
    price_amount     INTEGER,
    price_currency   TEXT DEFAULT 'BRL',
    coupons          TEXT,
        -- JSON array de Coupon structs: [{"code":"X","discount_type":"code_only","discount_value":0,"requires_action":false}]
    virtual_currency TEXT,
        -- JSON VirtualCurrency: {"platform":"aliexpress","amount":90,"cap_brl":0,"type":"discount"}
    urgency_signals  TEXT,
        -- JSON array de strings: ["frete_gratis", "corre", "ultima_unidade"]
    expires_at       TEXT,
        -- Preenchido se texto menciona data/hora específica
    is_promotional   INTEGER NOT NULL DEFAULT 1,
    processed_at     TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(channel_id, message_id)
);

-- Produtos (entidade de deduplicação)
CREATE TABLE products (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    canonical_url   TEXT NOT NULL,
    url_hash        TEXT NOT NULL UNIQUE,  -- SHA-256
    merchant        TEXT NOT NULL,
    name            TEXT NOT NULL DEFAULT '',
    current_price   INTEGER,
    lowest_price    INTEGER,
    affiliate_url   TEXT,
    image_url       TEXT,
    offer_count     INTEGER NOT NULL DEFAULT 1,
    first_seen_at   TEXT NOT NULL DEFAULT (datetime('now')),
    last_seen_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_products_merchant ON products(merchant);
CREATE INDEX idx_products_url_hash ON products(url_hash);

-- Histórico de preços
CREATE TABLE price_history (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    product_id  INTEGER NOT NULL REFERENCES products(id),
    price       INTEGER NOT NULL,
    currency    TEXT NOT NULL DEFAULT 'BRL',
    channel_id  INTEGER NOT NULL,
    message_id  INTEGER NOT NULL,
    detected_at TEXT NOT NULL DEFAULT (datetime('now')),
    UNIQUE(product_id, channel_id, message_id)
);

CREATE INDEX idx_price_history_product ON price_history(product_id, detected_at);

-- Cache de resolução de URLs
CREATE TABLE url_resolutions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    original_url  TEXT NOT NULL UNIQUE,
    canonical_url TEXT NOT NULL,
    merchant      TEXT NOT NULL,
    resolved_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_url_resolutions_canonical ON url_resolutions(canonical_url);

-- Cache persistente de fotos baixadas via MTProto (JPEG ~800px)
CREATE TABLE photo_cache (
    photo_id   INTEGER PRIMARY KEY,
    data       BLOB NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    expires_at TEXT
);

CREATE INDEX idx_photo_cache_expires
    ON photo_cache(expires_at)
    WHERE expires_at IS NOT NULL;
```

### Relação entre tabelas

```
raw_messages (collector, read-only)
     │ 1:1
     ▼
processed_messages
     │ N:1
     ▼
products
     │ 1:N
     ▼
price_history
```

---

## 10. Decisões Arquiteturais Pendentes

### 10.1 Processamento: Batch ou Streaming?

**Opção A — Batch (poll):**
- Processor roda periodicamente (cron ou ticker interno)
- Query: `SELECT * FROM raw_messages WHERE id > last_processed_id`
- Simples, mas introduz latência (intervalo de poll)

**Opção B — Streaming (event-driven):**
- Collector notifica processor via mecanismo compartilhado (file watcher,
  Unix socket, ou shared channel se no mesmo processo)
- Latência mínima, mas mais complexo

**Recomendação:** Começar com batch (poll a cada 5s). Complexidade de streaming
não justifica para MVP.

### 10.2 Playwright: Embed ou sidecar?

**Opção A — Embed (rod/chromedp):**
- Go library que controla Chrome/Chromium headless
- Sem processo externo, deploy mais simples
- `github.com/go-rod/rod` ou `github.com/chromedp/chromedp`

**Opção B — Sidecar (Playwright service):**
- Processo Node.js/Python separado com API HTTP
- Processor chama via HTTP para gerar links
- Mais flexível, mas mais infra

**Recomendação:** Rod (Go native) para manter a stack Go-only. Adicionar
`github.com/go-rod/rod` à closed stack do processor.

### 10.3 Extração de preço e cupom (calibrado em 6674 mensagens, 2026-06-09)

**Decisão finalizada.** Regex puro na Fase 2; templates por canal e LLM ficam para Fase 3.

#### Preços

**Distribuição real (6450 menções de `R$` extraídas):**
- p50 = R$ 83, p95 = R$ 1499, p99 = R$ 3607, max = R$ 40000
- 55% abaixo de R$ 100 (produtos baratos predominam)
- 3% acima de R$ 2000 (eletrônicos caros, nichos)

**Regex calibrado:**
```
R\$\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?)
```
Captura `R$ 1.234,56`, `R$1234`, `R$ 83`, `R$99,90`. Conversão: remover `.`, substituir `,` por `.`, `strconv.ParseFloat`.

#### Cupons

**Padrões identificados (4246 menções, ~71% das mensagens):**

| Padrão | Exemplo | Regex |
|--------|---------|-------|
| Único | `🎟  Cupom: 6DO6` | `(?i)cupom[:\s]+([A-Z0-9_]{3,25})` |
| Múltiplos (OU) | `Cupom: AEBR2 ou IFPL90V1 ou MARCABR02` | split por ` ou ` após match |
| Concatenado (+) | `Cupom: AEBR2 + FIFINE0601 + 954 Moedas` | split por ` \+ ` |
| Percentual | `Cupom de 15% OFF` | `(?i)cupom\s+de\s+(\d+)%` |
| Valor fixo | `cupom de R$ 30 OFF` | `(?i)cupom\s+de\s+R\$\s*(\d+)` |
| Sem código | `Resgate o cupom no anúncio` | flag `coupon_requires_action: true` |
| Especificador | `Use o Cupom: PROMONETS ou XETPROMOCOES` | case-insensitive, extrair todos |

**Schema de saída:**
```go
type Coupon struct {
    Code           string  // "AEBR2", "6DO6", ""
    DiscountType   string  // "percent" | "fixed_brl" | "code_only"
    DiscountValue  float64 // 15.0 para %, 30.0 para R$30, 0.0 para code_only
    RequiresAction bool    // true se "resgate no anúncio"
}
```

#### Moedas virtuais (Shopee e AliExpress)

**Descoberta-chave:** tanto Shopee quanto AliExpress usam "moedas" como desconto
adicional no app. **Não é o mesmo que cupom** — é camada separada.

- **AliExpress:** 92 menções — formato: `Cupom: XXX + N Moedas no app`
- **Shopee:** 36 menções — formato: `50% de cashback em Moedas Shopee` ou
  `70% de cashback em Moedas Shopee nas compras acima de R$0 (limite de 1.000 moedas:R$10)`

**Regex calibrado:**
```
(?i)(\d+)\s*moedas?(\s*no\s*app)?
```

**Schema de saída:**
```go
type VirtualCurrency struct {
    Platform string  // "aliexpress" | "shopee"
    Amount   int64   // 90, 588, 954
    CapBRL   float64 // 10.0 se "limite de 1.000 moedas:R$10" (Shopee); 0.0 senão
    Type     string  // "discount" | "cashback" (Shopee é cashback, AliExpress é desconto direto)
}
```

**Nota:** detectar plataforma pelo domínio da URL na mensagem
(`s.click.aliexpress.com` → AliExpress; `s.shopee.com.br` → Shopee).

#### Descontos (OFF / %)

- 1686 mensagens com "OFF" (predominante)
- 1354 mensagens com "%" (ex.: "30% OFF")
- 4747 mensagens com "R$" (preço direto)

**Regex combinado:**
```
(?i)(\d+)\s*%\s*(?:de\s+)?(?:desconto\s+)?off
```
Captura "30% OFF", "15% de desconto", "50% off".

#### Estratégia de implementação

1. Aplicar todos os regex em cascata no texto limpo
2. Dedup por código (mesmo cupom pode ser mencionado múltiplas vezes)
3. Se `coupon_requires_action: true` e sem código detectado, marcar flag
4. Logar mensagens onde regex falha (para refinar em Fase 3)

---

## 11. Configuração Esperada

```env
# Existentes (herdadas do collector)
LIMIAR_DB_PATH=./limiar.db
LIMIAR_LOG_LEVEL=info
LIMIAR_LOG_FORMAT=json

# Novas para o processor
LIMIAR_PROCESSOR_POLL_INTERVAL=5s          # Intervalo de poll para novas mensagens
LIMIAR_PROCESSOR_BATCH_SIZE=50             # Mensagens por batch
LIMIAR_PROCESSOR_RESOLVE_URLS=false        # Resolver URLs no processor/reprocess (opt-in)
LIMIAR_PROCESSOR_RESOLVE_URLS_LIMIT=0      # Limite por execução; 0 = sem limite quando habilitado

# Credenciais de afiliados
LIMIAR_AFFILIATE_ML_USER=...               # Login do painel ML afiliados
LIMIAR_AFFILIATE_ML_PASS=...               # Senha (masked em logs)
LIMIAR_AFFILIATE_AMAZON_TAG=...            # Tag Amazon Associates
LIMIAR_AFFILIATE_ALIEXPRESS_APP_KEY=...    # App key AliExpress
LIMIAR_AFFILIATE_ALIEXPRESS_SECRET=...     # Secret (masked em logs)
LIMIAR_AFFILIATE_SHOPEE_ID=...             # ID Shopee afiliados
```

---

## 12. Restrições e Regras (Extensão do AGENTS.md)

O processor segue as mesmas regras fundamentais do collector, com extensões:

1. **Raw messages são read-only.** O processor nunca escreve em `raw_messages`.
2. **Mesmo banco, tabelas separadas.** Processor adiciona tabelas, não modifica
   as existentes.
3. **Sem CGO.** Se usar rod/chromedp, verificar que não requer CGO.
4. **Logger injected.** Mesma interface `logger.Logger` do collector.
5. **Credenciais mascaradas.** Affiliate passwords/secrets nunca aparecem em logs.
6. **Idempotência.** Reprocessar a mesma raw_message produz o mesmo resultado
   (ou atualiza se preço mudou).
7. **Falha parcial é ok.** Se resolução de URL falha, a mensagem é processada
   sem URL resolvida e marcada para retry.

---

## 13. O que NÃO Fazer na Fase 2

- **Não implementar classificação semântica (LLM)** — isso é Fase 3
- **Não implementar REST/SSE** — isso é Fase 4 (limiar-api)
- **Não resolver deduplicação fuzzy por nome** — apenas por URL canônica
- **Não hospedar arquivos soltos de imagem** — bytes persistentes ficam no Turso (`photo_cache`)
- **Não construir UI de admin** — processor é headless, observável via logs

---

## 14. Sequência de Implementação Sugerida

```
Sprint 1: Fundação
├── Estrutura do binário (cmd/limiar-processor/main.go)
├── Tabelas novas (migrations)
├── Estágio 1: Normalização (wrapper detection, field cleanup)
└── Testes com payloads reais exportados

Sprint 2: URLs
├── Estágio 2: Resolução de URLs (HTTP follow redirects, cache)
├── Estágio 4: Imagens via MTProto/cache local (MEDIA-TD v3)
└── Tabela url_resolutions + products (criação básica)

Sprint 3: Deduplicação + Preço
├── Estágio 5: Deduplicação por URL hash
├── Estágio 6: Price tracking (inserir price_history)
├── Lógica de "melhor oferta"
└── Testes de dedup cross-channel

Sprint 4: Afiliados
├── Estágio 3: Re-afiliação (driver ML com rod)
├── Drivers Amazon/AliExpress/Shopee (conforme APIs disponíveis)
├── Processamento assíncrono de affiliate URLs
└── Retry + fallback para falhas de geração
```

---

## 14.5. Métricas para o Frontend (Fase 4 — limiar-api)

### Dados calibrados em 6674 mensagens (2026-06-09)

Estas métricas informam decisões de UX/display no frontend. Não afetam o
processor diretamente, mas o processor já deve produzir os campos necessários.

#### Truncagem de texto

| Percentil | Chars | Decisão de UI |
|-----------|-------|---------------|
| p50 | 175 | Mostrar completo em card |
| p95 | 350 | Mostrar completo, sem "Leia mais" |
| p99 | 507 | Card padrão (com "Leia mais" opcional) |
| max | 1076 | Modal / página de detalhe |

**Recomendação:** truncar em 300 chars com "..." em card de feed; mostrar
completo em view de detalhe.

#### Emojis como filtros rápidos

| Emoji | Frequência | Filtro sugerido |
|-------|------------|-----------------|
| 🔥 | 2864 (47%) | "Hot deals" |
| 🎟 | 1119 (18%) | "Com cupom" |
| ✅ | 511 (8%) | "Verificado" |
| ⭐ | 327 (5%) | "Destaque" |
| 🚀 | 28 | "Novo / lançamento" |

**Recomendação:** chip filter no topo do feed com esses 4 emojis principais.

#### Picos de atividade (horário do dia)

| Hora | Mensagens | % do total |
|------|-----------|------------|
| 21h | 5099 | 85% |
| 20h | 581 | 9.7% |
| 12-14h | 142-143 | 2.4% |
| 1-7h | ~1 | <0.1% |

**Recomendação:** badge "🔥 pico agora" no frontend quando hora atual = 20-22h;
scheduler de notificação push às 20:30.

#### Aspect ratio das imagens

- 1:1 quadrado (85%) → placeholder `aspect-ratio: 1/1` por default
- 16:9 landscape (8%) → adaptação CSS via `object-fit: contain`
- 1.9:1 wide banner (7%) → raro, aceita overflow

**Recomendação:** CSS grid com slot 1:1; lazy-load com blur placeholder.

#### Distribuição de preços (para histograma no frontend)

- R$ 0-100: 55% (cor verde, "barato")
- R$ 100-500: 33% (cor amarela, "médio")
- R$ 500-2000: 9% (cor laranja, "premium")
- R$ 2000+: 3% (cor vermelha, "luxo")

**Recomendação:** filtro por faixa de preço no sidebar do feed.

---

## 15. Perguntas para Resolver Antes de Implementar

### ✅ Respondidas durante análise de 6674 mensagens (2026-06-09)

1. ~~**Dados suficientes?**~~ — Sim, 6674 msgs em 70 dias, 16 canais, distribuição real.
   Regex de preço/cupom calibrado em §10.3.
2. ~~**CDN do Telegram é estável?**~~ — A proposta de CDN público foi superada; usar MTProto + cache local. Ver §6 e MEDIA-TD v3.
3. ~~**Entities como fonte de URLs?**~~ — Não. Só 30 entities com URL vs 5788 no texto.
   Regex no texto é fonte primária; Entities é backup.
4. ~~**GroupedID / álbuns?**~~ — Zero ocorrências. Não precisa lidar.
5. ~~**Mensagens com Media=null são erro?**~~ — Não. São mensagens textuais puras (6%,
   com URLs e cupons).

### ❓ Pendentes

1. **`meli.la` não leva ao produto** — a URL cai em página do afiliado, não do
   produto. Como extrair o product_id? Opções:
   - Scraping da página do afiliado para achar o produto linkado
   - Extrair do contexto da mensagem (se há nome do produto)
   - Marcar como `unresolved` até ter solução melhor
2. **AliExpress `s.click.aliexpress.com` cai em landing genérica** — mesmo
   problema do `meli.la`. Pode ser necessário login + scraping para achar o
   produto real, ou usar URL interna do Telegram (`Webpage.URL`) quando
   disponível como fonte canônica.
3. **Magalu bloqueia com HTTP 403** — Playwright com User-Agent real resolve?
   Ou precisa de proxy residential IP? Testar antes de implementar.
4. **Contas de afiliado criadas?** Necessário para testar re-afiliação (Sprint 4)
5. **Rod funciona sem CGO?** Validar que `go-rod/rod` compila pure Go
6. **Rate limits dos merchants?** Testar quantas URLs podem ser resolvidas por
   minuto sem bloqueio (Shopee P1, Amazon P1 primeiro)
7. **Shopee cashback de moedas: como modelar?** Em §10.3 propus
   `VirtualCurrency` separado do `Coupon`. Faz sentido ou consolidar?
8. **`coupon_expired` deve atualizar o Product original?** Propus propagar
   `expired: true` via ReplyTo. Mas ReplyTo pode ser ausente — nesses casos,
   usar fuzzy-match por cupom code? (ex.: se cupom `6DO6` foi mencionado em
   mensagem anterior, marcar aquela como expired).

---

**Fim do documento de ideação.**

Este documento deve ser consultado pelo agente que iniciar a implementação da
Fase 2. Ele não é uma especificação finalizada — é um registro de decisões,
ideias e restrições discutidas durante a fase de coleta de dados.

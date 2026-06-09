# Ideação — limiar-processor (Fase 2)

**Data:** 2026-06-08  
**Status:** Rascunho de ideias — aguardando mais dados do collector antes de implementar  
**Pré-requisitos:** Análise de payloads expandida (meta: 10.000+ mensagens, 10+ canais, 30 dias)

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
│ 4. IMAGENS          │  Construir URL da imagem via CDN público
│                     │  do Telegram (telesco.pe), sem hosting
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
    RawMessageID   int64     // FK para raw_messages.id
    MessageID      int64     // ID original do Telegram
    ChannelID      int64     // PeerID.ChannelID
    ChannelUsername string   // lookup em channels table
    PostedAt       time.Time // Date convertido
    ReceivedAt     time.Time // raw_messages.received_at
    Text           string    // Message limpo
    TextLength     int
    MediaType      string    // "photo" | "poll" | "video" | "document" | "none"
    PhotoID        int64     // Media.Photo.ID (se aplicável)
    PhotoSizes     []PhotoSize
    Entities       []Entity  // Entities parseadas (offset, length, type, url)
    Views          int
    Forwards       int
    GroupedID      int64     // Para detecção de álbuns
}
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

### Merchants prioritários (baseado nos dados coletados)

| Merchant | Domínios shortened | Domínio canônico |
|----------|-------------------|------------------|
| AliExpress | `s.click.aliexpress.com` | `aliexpress.com` / `pt.aliexpress.com` |
| Mercado Livre | `meli.la` | `mercadolivre.com.br` |
| Amazon | `amzn.to`, `amzn.divulgador.link` | `amazon.com.br` |
| Shopee | `s.shopee.com.br` | `shopee.com.br` |

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
- **Cache:** URLs resolvidas são imutáveis — cachear indefinidamente em tabela
  `url_resolutions(original_url, canonical_url, merchant, resolved_at)`.

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

## 6. Estágio 4: Imagens via CDN Público do Telegram

### Objetivo

Obter URLs de imagem de alta qualidade sem hospedar arquivos, usando o CDN
público do Telegram (`cdn1.telesco.pe`).

### Como funciona

Canais públicos do Telegram expõem preview de mensagens em
`https://t.me/{channel_username}/{message_id}`. O embed serve a imagem
original via CDN:

```
https://cdn1.telesco.pe/file/{file_token}.jpg
```

### Referência: Como o RSSHub faz

O RSSHub usa exatamente essa técnica para gerar feeds RSS de canais do
Telegram. Ele acessa o preview público da mensagem e extrai a URL da imagem
do CDN. A imagem é servida em resolução completa (não thumbnail).

### Implementação proposta

```go
// Dado channel_username e message_id:
// 1. GET https://t.me/{channel_username}/{message_id}
// 2. Parsear HTML, extrair <meta property="og:image" content="...">
// 3. URL extraída = imagem em alta resolução no CDN do Telegram

func ResolveImageURL(ctx context.Context, channelUsername string, messageID int64) (string, error) {
    url := fmt.Sprintf("https://t.me/%s/%d", channelUsername, messageID)
    // GET + parse og:image meta tag
    // Retorna URL do cdn1.telesco.pe
}
```

### Vantagens

- **Sem hosting de imagens** — CDN do Telegram serve o arquivo
- **Sem autenticação** — canais públicos permitem acesso direto
- **Alta qualidade** — imagem original, não thumbnail
- **Sem File Reference expirando** — URL do CDN é estável para canais públicos

### Limitações

- **Canal privado:** Se um canal mudar para privado, URLs quebram. Mitigação:
  canais de promoção são quase sempre públicos.
- **Rate limiting do t.me:** Não abusar de requests ao t.me. Cachear URLs de
  imagem uma vez resolvidas (imutáveis por mensagem).
- **Disponibilidade:** Dependência do CDN do Telegram. Se cair, imagens ficam
  indisponíveis (aceitável para MVP).

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
    CurrentPrice    float64   // Preço mais recente
    LowestPrice     float64   // Menor preço histórico
    AffiliateURL    string    // URL com affiliate ID do Limiar
    ImageURL        string    // URL do CDN do Telegram
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
    Price       float64   // Preço detectado
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
    price       REAL NOT NULL,
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
    text_clean       TEXT NOT NULL,
    text_length      INTEGER NOT NULL DEFAULT 0,
    media_type       TEXT NOT NULL DEFAULT 'none',
    image_url        TEXT,
    views            INTEGER NOT NULL DEFAULT 0,
    forwards         INTEGER NOT NULL DEFAULT 0,
    has_price        INTEGER NOT NULL DEFAULT 0,
    has_coupon       INTEGER NOT NULL DEFAULT 0,
    price_amount     REAL,
    price_currency   TEXT DEFAULT 'BRL',
    coupons          TEXT,  -- JSON array: ["CUPOM1", "CUPOM2"]
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
    current_price   REAL,
    lowest_price    REAL,
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
    price       REAL NOT NULL,
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

### 10.3 Extração de preço/cupom

**Decisão adiada.** Aguardar mais dados do collector (10.000+ mensagens) para
entender melhor os padrões. Opções:

- Regex puro (simples, mas frágil)
- Templates por canal (cada canal tem um formato previsível)
- LLM local/API (mais robusto, mas mais lento e caro) — Fase 3

---

## 11. Configuração Esperada

```env
# Existentes (herdadas do collector)
LIMIAR_DB_PATH=./limiar.db
LIMIAR_LOG_LEVEL=info
LIMIAR_LOG_FORMAT=json

# Novas para o processor
LIMIAR_PROCESSOR_POLL_INTERVAL=5s          # Intervalo de poll para novas mensagens
LIMIAR_PROCESSOR_URL_TIMEOUT=5s            # Timeout para resolução de URLs
LIMIAR_PROCESSOR_URL_MAX_REDIRECTS=10      # Max redirects ao resolver URL
LIMIAR_PROCESSOR_BATCH_SIZE=50             # Mensagens por batch

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
- **Não hospedar imagens** — apenas referenciar CDN do Telegram
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
├── Estágio 4: Imagens via CDN (og:image extraction)
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

## 15. Perguntas para Resolver Antes de Implementar

1. **Dados suficientes?** Coletar 10.000+ mensagens antes de finalizar regex
   de extração de preço/cupom
2. **Contas de afiliado criadas?** Necessário para testar re-afiliação
3. **Rod funciona sem CGO?** Validar que `go-rod/rod` compila pure Go
4. **Rate limits dos merchants?** Testar quantas URLs podem ser resolvidas por
   minuto sem bloqueio
5. **CDN do Telegram é estável?** Monitorar se URLs do `cdn1.telesco.pe`
   expiram ou mudam

---

**Fim do documento de ideação.**

Este documento deve ser consultado pelo agente que iniciar a implementação da
Fase 2. Ele não é uma especificação finalizada — é um registro de decisões,
ideias e restrições discutidas durante a fase de coleta de dados.

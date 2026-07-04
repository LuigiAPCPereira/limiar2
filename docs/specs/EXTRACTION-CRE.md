# Spec — Extração Determinística e Candidate Ranking Engine

> **ADR:** [014-deterministic-extraction-and-cre.md](../adr/014-deterministic-extraction-and-cre.md)
> **Baseline:** Análise de 31.832 mensagens (2026-07-03)
> **Literatura:** AVE (KDD 2016, OpenTag 2018), Two-Stage Retrieval (Mono-Duo BERT,
> FlashRank, Learning to Rank), Product Normalization (Affiliate.com, Superclean)
> **Fase:** 2 (limiar-processor)
> **Status:** Pronto para implementação

---

## Visão geral

Este spec detalha três sprints de melhoria da extração no `limiar-processor`,
baseados em evidência medida no banco de produção e em literatura acadêmica de
Attribute Value Extraction e Two-Stage Retrieval. O objetivo é maximizar a
extração determinística (regex + scoring) antes de qualquer invocação de LLM,
seguindo os invariantes do projeto.

```
raw_messages (read-only)
    │
    ▼
Normalize (existente)
    │
    ├─► extractCoupons     (Sprint 1 — enumeração)
    ├─► extractModifiers   (Sprint 1 — extração estruturada)
    ├─► extractWebpage     (Sprint 1 — gold standard para CRE)
    │
    ▼
CandidateRankingEngine    (Sprint 2 — ranking de candidatos)
    │
    │  ┌─────────────────────────────────────────────┐
    │  │ STAGE 1: Candidate Generation               │
    │  │   Webpage.Title → TextHeuristic → [URLTitle]│
    │  ├─────────────────────────────────────────────┤
    │  │ STAGE 2: Scoring                            │
    │  │   Position + Capitaliz + TechDensity +      │
    │  │   BrandMatch + Proximity - MetaPenalty      │
    │  ├─────────────────────────────────────────────┤
    │  │ STAGE 3: Normalization (pós-extração)       │
    │  │   Remove merchant suffix, SKU, promo noise  │
    │  └─────────────────────────────────────────────┘
    │
    ▼
URL Resolver              (Sprint 3 — canonicalização + <title>)
    │  limpa affiliate links, resolve redirects, cacheia
    ▼
SaveProcessedBatch
```

---

## Gap analysis: schema atual vs spec PROCESSOR-IDEATION

### Tabelas inexistentes (previstas no spec §9, ausentes no banco)

| Tabela | Previsão | Quando entra |
|--------|----------|--------------|
| `products` | Entidade de deduplicação (canonical_url, url_hash, merchant, name, current/lowest_price, affiliate_url, offer_count) | Sprint 3+ (quando dedup evoluir de URL-hash pra entidade Product) |
| `price_history` | Tracking de preço por produto (product_id, price, channel_id, message_id, detected_at) | Sprint 3+ (depende de `products`) |
| `url_resolutions` | Cache de URL resolution (original, canonical, merchant, title, resolved_at) | **Sprint 3** (explicitamente) |

### Colunas previstas no spec §9 mas inexistentes em `processed_messages`

| Coluna spec original | Status atual | Ação |
|----------------------|--------------|------|
| `webpage_url` | Não existe. Normalizer extrai (linha 560-571) mas só usa pra URL hash, não persiste. | **Sprint 1** — adicionar |
| `webpage_title` | Não existe. É a fonte gold standard do CRE. | **Sprint 1** — adicionar |
| `webpage_desc` | Não existe. Contexto extra do Webpage. | **Sprint 1** — adicionar |
| `coupons` (JSON array) | Não existe. Hoje só `coupon_code` (single). | **Sprint 1** — adicionar como `coupon_codes` |
| `virtual_currency` (JSON) | Não existe. Moedas Shopee/AliExpress. | **Sprint 1** — adicionar |
| `image_url` | Não aplicável. Resolvido via `photo_cache` + `inline_thumb` (ADR 011). | Nenhuma ação |
| `is_promotional` | Não existe. Hoje `feed_eligible` cobre parcialmente. | **Sprint 1** — adicionar |
| `expires_at` | Coberto por `valid_until`. | Nenhuma ação |

### Campos novos identificados pela análise (não no spec original)

| Campo | Motivo | Sprint |
|-------|--------|--------|
| `product_name_confidence REAL` | CRE precisa persistir score para Fase 3 (LLM) saber quando intervir | 2 |
| `modifiers` (JSON array) | Modifiers estruturados (app_only, web_only, pix, moedas, etc.) | 1 |
| `canonical_url TEXT` | URL limpa sem affiliate tracking, servida ao frontend | 3 |
| `url_title TEXT` | `<title>` da página resolvida, fonte complementar do CRE | 3 |
| `url_resolved INTEGER` | Flag indicando se a URL foi resolvida com sucesso | 3 |

---

## Princípios de engenharia (aplicáveis aos três sprints)

Todo código produzido segue:

1. **Alta coesão, baixa acoplamento**: cada extractor é uma função pura
   que recebe texto (e contexto mínimo) e retorna structs de domínio. Não
   muta estado externo, não conhece banco, não conhece telegram.
2. **Extensível sem refatoração grande**: novos padrões de regex são
   adicionados como dados (slices de struct ou arquivos embed), não como
   novos `if`. Adicionar um modificador ou marca = adicionar uma entrada
   num arquivo de dados.
3. **Limpo e legível**: nomes descritivos, funções curtas, comentários
   explicando o "porquê" (não o "o quê"). Zero magic numbers — constantes
   nomeadas.
4. **Testável**: cada extractor tem tabela de testes (`[]struct{ in, want }`)
   com casos reais do corpus. Testes de regressão para cada bug corrigido.
5. **Determinístico**: mesmo input, mesmo output. Sem `time.Now()` em
   extração (apenas em `processed_at`).
6. **Sem abstração prematura**: não criar interfaces sem 2+ implementações.
   Começar com funções concretas. Extrair interface quando a segunda fonte
   de dados real surgir (ex: URL Resolver no Sprint 3).
7. **Sem nova dependência**: regex + scoring é stdlib. Sprint 3 usa `net/http`
   da stdlib.

---

## Sprint 1 — Correções de Extração + Webpage + Schema

### Objetivo

Corrigir sub-extração medida no banco, adicionar dados estruturados que não
estão sendo persistidos (Webpage.Title/URL/Desc, VirtualCurrency), e preparar
o schema para o CRE do Sprint 2.

### Problemas medidos (baseline)

| Problema | Medido | Meta |
|----------|--------|------|
| Cupons múltiplos perdidos | ~22% das msgs com cupom | 0% perdido |
| PIX sub-extraído | 24% loss (1.305 msgs) | < 5% |
| VirtualCurrency | 100% loss (1.699 msgs) | detectado quando presente |
| App-only / Web-only | 100% loss (1.288 msgs) | detected quando presente |
| Webpage.Title não persistido | 0.3% coverage, 0% persistido | 100% persistido quando presente |
| `product_name` vazio | 100% | (Sprint 2) |

### 1.1 Cupons múltiplos

#### Onde: `internal/processor/normalizer.go` → função `extractCoupon`

Hoje usa `FindStringSubmatch` (first-only). Depois: função `extractCoupons`
(plural) que retorna `[]model.Coupon`:

```go
// Coupon representa um cupom extraído do texto da promoção.
type Coupon struct {
    Code           string `json:"code"`
    DiscountType   string `json:"discount_type"`   // "code_only"|"percent"|"fixed_brl"
    DiscountValue  int64  `json:"discount_value"`  // centavos ou %
    RequiresAction bool   `json:"requires_action"` // "resgate no anúncio"
}
```

#### Lógica

1. `reCoupon.FindAllStringSubmatch` (ALL matches, não first).
2. Para cada match, verificar se o código capturado é seguido de " ou "
   ou " + " e fazer split.
3. Detectar cupons de valor: `(?i)cupom\s+de\s+R\$\s*(\d+)` (fixed_brl) e
   `(?i)cupom\s+de\s+(\d+)%` (percent).
4. Detectar requires_action: `(?i)resgate.*(?:anúncio|app|loja)` → sem
   código, flag only.
5. Dedup por código (case-insensitive).
6. `coupon_code` (campo existente) mantém o primeiro código para
   compatibilidade com dashboard/query existentes.

### 1.2 Modifiers estruturados

```go
// Modifier representa uma condição comercial da promoção.
type Modifier struct {
    Type  string `json:"type"`  // "payment"|"shipping"|"installments"|"cashback"|"discount"|"app_only"|"web_only"|"recurring"
    Value string `json:"value"` // valor normalizado
}
```

#### Novos padrões de regex (baseados na análise)

```go
// PIX — cobre os 24% perdidos: "(pix)", "SELECIONE PIX NA PAG", "pix na pag"
rePixExtra = regexp.MustCompile(`(?i)\b(?:no\s+)?pix\b|selecione\s+pix|pix\s+na\s+pag`)

// VirtualCurrency (Moedas Shopee/AliExpress) — spec §10.3
reMoedas   = regexp.MustCompile(`(?i)(\d+)\s*moedas?(?:\s*no\s+app)?`)

// App-only / Web-only
reAppOnly  = regexp.MustCompile(`(?i)apenas\s+(?:pelo\s+)?aplicativo|somente\s+(?:no\s+)?(?:aplicativo|app)|pelo\s+app`)
reWebOnly  = regexp.MustCompile(`(?i)apenas\s+(?:pelo\s+)?site|somente\s+(?:no\s+)?site|pela\s+web|no\s+site`)
```

### 1.3 VirtualCurrency

```go
// VirtualCurrency representa moedas/cashback em moedas (Shopee/AliExpress).
type VirtualCurrency struct {
    Platform string `json:"platform"` // "aliexpress"|"shopee"
    Amount   int64  `json:"amount"`   // 90, 588, 954
    CapBRL   int64  `json:"cap_brl"`  // 1000 se "limite de 1.000 moedas:R$10"
    Type     string `json:"type"`     // "discount"|"cashback"
}
```

Plataforma inferida do merchant já detectado (`shopee` ou `aliexpress`).

### 1.4 Webpage.Title/URL/Desc (preparação para CRE)

O normalizer já extrai `Webpage.URL` (linha 560-571) mas só usa para
computar URL hash quando texto não tem URL. Agora vamos persistir os três
campos do Webpage como fonte gold standard para o CRE.

```go
type WebpageInfo struct {
    URL         string `json:"url"`
    Title       string `json:"title"`
    Description string `json:"description"`
}
```

Extrair do payload bruto `Media.Webpage.{URL,Title,Description}`.
Persistir mesmo se texto já tem URL — Webpage.Title é independente e vale
ouro para o CRE.

### 1.5 Migração de Schema — 008 (Sprint 1)

> **Estratégia de migration:** Este projeto usa schema consolidado em
> `001_initial.sql` (banco novo nasce completo) + reparo condicional em Go
> via `currentSchemaColumns` (banco legado recebe colunas faltantes via
> `PRAGMA table_info`). O arquivo `008_*.sql` é um **marcador** que apenas
> garante que a versão é registrada em `schema_migrations`. Isso evita o
> erro `duplicate column name` do Turso/SQLite ao rodar `ALTER TABLE ADD
> COLUMN` em banco que já tem a coluna via `001`. Ver diretriz completa em
> `docs/guidelines/STORAGE.md` §"Estratégia de Migrations".

#### Arquivo SQL (marcador)

`internal/storage/migrations/008_extraction_enhancements.sql`:

```sql
-- 008_extraction_enhancements.sql
-- Sprint 1: cupons múltiplos, modifiers estruturados, virtual_currency,
-- webpage fields, is_promotional.
--
-- O schema novo já nasce completo em 001_initial.sql. Bancos antigos recebem
-- colunas ausentes pelo reparo condicional em internal/storage/migrations.go,
-- pois Turso não aceita ALTER TABLE ADD COLUMN duplicado.
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

#### Colunas adicionadas (fisicamente, via Go)

As colunas são declaradas em DOIS lugares (ambos obrigatórios):

1. **`001_initial.sql`** — para bancos novos (schema consolidado completo):

```sql
coupon_codes       TEXT NOT NULL DEFAULT '',
modifiers          TEXT NOT NULL DEFAULT '',
virtual_currency   TEXT NOT NULL DEFAULT '',
webpage_url        TEXT NOT NULL DEFAULT '',
webpage_title      TEXT NOT NULL DEFAULT '',
webpage_desc       TEXT NOT NULL DEFAULT '',
is_promotional     INTEGER NOT NULL DEFAULT 0,
```

2. **`currentSchemaColumns` em `migrations.go`** — para bancos legados
   (reparo condicional via `PRAGMA table_info`):

```go
{table: "processed_messages", name: "coupon_codes", definition: "TEXT NOT NULL DEFAULT ''"},
{table: "processed_messages", name: "modifiers", definition: "TEXT NOT NULL DEFAULT ''"},
{table: "processed_messages", name: "virtual_currency", definition: "TEXT NOT NULL DEFAULT ''"},
{table: "processed_messages", name: "webpage_url", definition: "TEXT NOT NULL DEFAULT ''"},
{table: "processed_messages", name: "webpage_title", definition: "TEXT NOT NULL DEFAULT ''"},
{table: "processed_messages", name: "webpage_desc", definition: "TEXT NOT NULL DEFAULT ''"},
{table: "processed_messages", name: "is_promotional", definition: "INTEGER NOT NULL DEFAULT 0"},
```

### 1.6 Atualização do model

`internal/model/normalized.go` — adicionar campos:

```go
CouponCodes      []Coupon         // múltiplos cupons estruturados
Modifiers        []Modifier       // modifiers estruturados
VirtualCurrency  *VirtualCurrency // moedas Shopee/AliExpress (nil se não há)
WebpageURL       string           // Media.Webpage.URL
WebpageTitle     string           // Media.Webpage.Title (gold standard CRE)
WebpageDesc      string           // Media.Webpage.Description
IsPromotional    bool             // deal_* → true
```

`internal/model/processed.go` — adicionar campos correspondentes com
JSON tags para o dashboard.

### 1.7 Estrutura de código modular

```
internal/processor/
├── normalizer.go         (existente — orquestra, delega)
├── extract_coupons.go    (NOVO — extractCoupons + structs + regex)
├── extract_modifiers.go  (NOVO — extractModifiers + structs + regex)
├── extract_webpage.go    (NOVO — extractWebpageInfo do payload)
├── extract_coupons_test.go
├── extract_modifiers_test.go
└── extract_webpage_test.go
```

### 1.8 Verificação do Sprint 1

```
go build ./...
go vet ./...
go test ./internal/processor/...
go test -race ./...
```

Re-processar banco e medir:
- `% msgs com coupon_codes != ''` > 62.1% (baseline atual de coupon_code)
- `% msgs com payment_method='pix'` sobe de 12.9% para > 16%
- `% msgs com modifiers != ''` > 0% (hoje é 0)
- `% msgs com webpage_title != ''` ≈ 0.3% (mas 100% persistido quando existe)

---

## Sprint 2 — Candidate Ranking Engine (ProductName)

### Objetivo

Extrair `product_name` deterministicamente com score de confiança, sem
invocar LLM. A heurística é a fonte primária; LLM vira fallback (Fase 3).

### Fundamentação em literatura

O design do CRE é fundamentado em três corpos de pesquisa:

1. **Attribute Value Extraction (AVE)** — papers de e-commerce (KDD 2016
   "Attribute Extraction from Product Titles in eCommerce", OpenTag 2018,
   "Bootstrapped NER for Product Attribute Extraction" ACL 2011) validam
   que extração de atributos de títulos de produto é um problema real e
   que regras/regex são a base mesmo de sistemas que depois evoluem pra
   CRF/transformers. O paper do Walmart (2016) diz: "We used a distant
   supervision approach to build our initial training data set. For each
   attribute we built regex based rules to programatically annotate product
   titles."

2. **Two-Stage Retrieval** — o padrão da indústria (Mono-Duo BERT, FlashRank,
   "Learning to Rank in the Age of Muppets") é: Stage 1 (candidate generation,
   broad recall) → Stage 2 (scoring/reranking, precise). "Learning to Rank"
   lista 4 categorias de features que mapeiam direto pros nossos scorers:
   term-based → TechDensity, score-based → BrandMatch+Capitalization,
   proximity-based → Position+Proximity.

3. **Product Name Normalization** — blogs de affiliate.com e Superclean
   validam que normalização pós-extração é crítica: merchant suffixes,
   promo noise, SKU codes, casing inconsistente e separators precisam ser
   removidos do nome extraído. Sem isso, o nome vem sujo.

### 2.1 Arquitetura do CRE — 3 Estágios

O CRE é um pipeline de 3 estágios, inspirado no padrão multi-stage retrieval.
Cada estágio tem responsabilidade única e é testável isoladamente.

```
INPUT: NormalizedMessage (texto limpo, posted_at, webpage_title, ...)
    │
    ▼
┌─────────────────────────────────────────────────────────────┐
│ STAGE 1: CANDIDATE GENERATION                               │
│                                                             │
│ Sources (coletam candidatos em paralelo conceitual):        │
│  ┌─────────────────┐  ┌──────────────────┐                 │
│  │ WebpageTitleSrc │  │ TextHeuristicSrc │                 │
│  │ (grátis, 0.3%)  │  │ (texto, ~70-80%) │                 │
│  └─────────────────┘  └──────────────────┘                 │
│  ┌─────────────────┐                                        │
│  │ URLTitleSrc     │  ← Sprint 3 (ainda não existe)        │
│  └─────────────────┘                                        │
│                                                             │
│ Cada fonte retorna []Candidate:                             │
│   { Text, Source, Detail, BaseConfidence }                 │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ STAGE 2: SCORING                                            │
│                                                             │
│ Para cada candidato, scorers ADICIONAM ou SUBTRAEM da       │
│ BaseConfidence. Score final é clamp([0.0, 1.0]).            │
│                                                             │
│  + PositionScorer      (0.25, linha 0?)                     │
│  + CapitalizationScorer (0.20, Title Case?)                 │
│  + TechDensityScorer   (0.20, specs técnicas?)              │
│  + BrandMatchScorer    (0.20, marca da lista embed?)        │
│  + ProximityScorer     (0.10, perto de preço/cupom?)        │
│  - MetaPenaltyScorer   (0.15, meta-anúncio?)                │
│                                                             │
│ Vencedor = candidato com maior score                        │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
┌─────────────────────────────────────────────────────────────┐
│ STAGE 3: NORMALIZATION (pós-extração)                       │
│                                                             │
│ Limpa o nome escolhido antes de persistir:                  │
│  - Remove merchant suffix (" - Amazon.com.br",              │
│    " | Shopee Brasil", " — Mercado Livre")                  │
│  - Remove SKU code trailing ("XYZ-BLU-L-2024")              │
│  - Remove promo noise residual ("BESTSELLER!", "FRETE       │
│    GRÁTIS", "OFERTA")                                       │
│  - Normaliza casing (Title Case quando apropriado)          │
│  - Collapse whitespace e delimitadores                      │
│                                                             │
│ Output: nome limpo                                          │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            ▼
THRESHOLD CHECK:
  score >= 0.45 → product_name = nome limpo, confidence = score
  score <  0.45 → product_name = "", confidence = score (pra LLM)
```

### 2.2 Modelo de dados

```go
// ProductNameResult é o resultado da extração de ProductName pelo CRE.
type ProductNameResult struct {
    Name         string  `json:"name"`
    Confidence   float64 `json:"confidence"`             // 0.0–1.0
    Source       string  `json:"source"`                 // "webpage_title"|"text_heuristic"|"url_title"
    SourceDetail string  `json:"source_detail,omitempty"`  // "line:0", "webpage", etc.
}

// Candidate é um candidato a ProductName gerado por uma fonte.
type Candidate struct {
    Text          string  // texto bruto do candidato
    Source        string  // origem: "webpage_title"|"text_heuristic"|"url_title"
    Detail        string  // detalhe: "line:0", "line:0-1", "near_price", "webpage"
    BaseConfidence float64 // confiança inicial da fonte
}
```

### 2.3 Stage 1 — Fontes de candidatos

Cada fonte é uma função pura que recebe `NormalizedMessage` e retorna
`[]Candidate`.

#### Fonte 1: WebpageTitleSource

Lê `NormalizedMessage.WebpageTitle` (persistido no Sprint 1).
Coverage esperada: 0.3% (96 msgs). Gold standard quando existe.
BaseConfidence: 0.60 (direto do `<title>` da página).

#### Fonte 2: TextHeuristicSource

A fonte principal. Estratégia: dividir o texto em linhas, gerar candidatos
de múltiplas linhas iniciais.

```go
func textHeuristicCandidates(text string) []Candidate {
    lines := splitLines(text)
    var cands []Candidate
    // Candidato A: primeira linha limpa
    if len(lines) > 0 {
        cands = append(cands, Candidate{
            Text: cleanLine(lines[0]),
            Source: "text_heuristic",
            Detail: "line:0",
            BaseConfidence: 0.30,
        })
    }
    // Candidato B: primeira + segunda linha (caso nome ocupe 2 linhas)
    if len(lines) > 1 {
        cands = append(cands, Candidate{
            Text: cleanLine(lines[0] + " " + lines[1]),
            Source: "text_heuristic",
            Detail: "line:0-1",
            BaseConfidence: 0.20,
        })
    }
    // Candidato C: linha imediatamente antes do preço
    if priceLine := lineBeforePrice(lines); priceLine >= 0 && priceLine > 0 {
        cands = append(cands, Candidate{
            Text: cleanLine(lines[priceLine-1]),
            Source: "text_heuristic",
            Detail: "near_price",
            BaseConfidence: 0.25,
        })
    }
    return cands
}
```

`cleanLine` remove emojis de borda, modificadores isolados ("PARCELADO",
"NOVO CUPOM") e whitespace.

#### Fonte 3: URLTitleSource (Sprint 3)

Lê do cache `url_resolutions.title` quando disponível.
BaseConfidence: 0.55.

### 2.4 Stage 2 — Scorers

Cada scorer é uma função `(Candidate, NormalizedMessage) float64` que retorna
um ajuste (positivo ou negativo). O score final é clampado em [0.0, 1.0].

| Scorer | Peso | Sinal | O que mede |
|--------|------|-------|------------|
| PositionScorer | 0.25 | + | Candidato na linha 0? (bônus total); linha 1 (parcial) |
| CapitalizationScorer | 0.20 | + | Title Case > 60% das palavras (excluindo stopwords) |
| TechDensityScorer | 0.20 | + | Densidade de tokens técnicos (GB, Hz, RAM, pol) acima de threshold |
| BrandMatchScorer | 0.20 | + | Contém marca da lista embed (`data/brands.txt`) |
| ProximityScorer | 0.10 | + | Candidato perto de preço/cupom no texto |
| MetaPenaltyScorer | 0.15 | - | Contém stopword de meta-anúncio (CUPOM, NOVO, PROMO, etc.) |

Score final = clamp(BaseConfidence + somatório_ajustes, 0.0, 1.0)

#### Exemplo de scoring

Texto:
```
PARCELADO🔥🔥🔥🔥

Anker Caixa de Som Soundcore Select 4 go

POR: 154 REAIS E FRETE GRÁTIS PRIME
```

Candidato A (line:0): "PARCELADO🔥🔥🔥🔥", base 0.30
- Position: +0.25 (linha 0)
- Capitalization: +0.00 (ALL CAPS, não Title Case)
- TechDensity: +0.00
- BrandMatch: +0.00
- Proximity: +0.00
- MetaPenalty: -0.15 ("PARCELADO")
- Score: 0.30 + 0.25 - 0.15 = 0.40

Candidato B (line:1): "Anker Caixa de Som Soundcore Select 4 go", base 0.20
- Position: +0.15 (linha 1, parcial)
- Capitalization: +0.20 (Title Case)
- TechDensity: +0.05 (densidade baixa mas "Select 4 go" é modelo)
- BrandMatch: +0.20 ("Anker" na lista)
- Proximity: +0.05 (próximo ao preço)
- MetaPenalty: -0.00
- Score: 0.20 + 0.15 + 0.20 + 0.05 + 0.20 + 0.05 = 0.85

Vencedor: B (0.85). Após Stage 3 normalization → "Anker Caixa de Som
Soundcore Select 4 go".

### 2.5 Stage 3 — Normalization (pós-extração)

O nome escolhido no Stage 2 pode vir sujo (merchant suffix, SKU, promo
residual). O Stage 3 limpa antes de persistir.

Inspiração: literatura de product name normalization (Affiliate.com,
Superclean, productlasso.com) valida que esses passos são essenciais.

Operações:
1. **Remove merchant suffix**: regex de sufixos comuns
   (`\s*[-–|]\s*(Amazon\.com\.br|Shopee Brasil|Mercado Livre|...)`).
   Lista embed em `data/merchant_suffixes.txt`.
2. **Remove SKU trailing**: regex `[-–|]\s*[A-Z0-9]{3,}-[A-Z0-9-]+$`
   (ex.: " - XYZ-BLU-L-2024").
3. **Remove promo noise residual**: palavras isoladas de promoção
   (BESTSELLER, FRETE GRÁTIS, OFERTA, IMPERDÍVEL). Lista embed em
   `data/promo_noise.txt`.
4. **Normaliza casing**: se > 60% Title Case, manter; se ALL CAPS,
   converter pra Title Case (provável nome); se all lowercase, capitalizar
   primeira letra de cada palavra.
5. **Collapse whitespace**: múltiplos espaços → 1, trim bordas.

### 2.6 Threshold

```go
const productNameConfidenceThreshold = 0.45
```

Se o melhor candidato tiver score < threshold → `product_name` fica vazio,
`product_name_confidence` guarda o score (para análise e para Fase 3 saber
que precisa de LLM).

### 2.7 Dados embed (listas extensíveis sem recompilar)

As listas de marcas, tokens técnicos, merchant suffixes e promo noise
vivem como arquivos de dados embed (mesmo padrão das migrations SQL em
`internal/storage/migrations/`).

Estrutura:

```
internal/processor/
└── data/
    ├── brands.txt              # uma marca por linha
    ├── tech_tokens.txt         # um token/regex por linha
    ├── merchant_suffixes.txt   # um sufixo por linha
    └── promo_noise.txt         # uma palavra/expressão por linha
```

Carregamento via `embed.FS`:

```go
//go:embed data/*.txt
var creDataFS embed.FS

func loadEmbedList(name string) []string {
    raw, err := creDataFS.ReadFile("data/" + name)
    if err != nil {
        return nil
    }
    var lines []string
    for _, line := range strings.Split(string(raw), "\n") {
        line = strings.TrimSpace(line)
        if line != "" && !strings.HasPrefix(line, "#") {
            lines = append(lines, line)
        }
    }
    return lines
}
```

Vantagens:
- **Extensível sem mudar lógica**: adicionar marca = editar `.txt`, não
  editar código Go. Recompila no próximo build (embed é compile-time).
- **Versionado com o código**: fica no git, revisável em PR.
- **Sem arquivo solto em runtime**: embed embute no binário, não cria
  dependência de filesystem externo (alinhado com ADR 011 — "zero arquivos
  soltos").
- **Mesmo padrão do projeto**: as migrations SQL já usam `embed.FS`.

### 2.8 Migração de Schema — 009 (Sprint 2)

> Segue a mesma estratégia do Sprint 1 (ver §1.5): schema consolidado em
> `001_initial.sql` + reparo condicional em `migrations.go` + marcador SQL
> `009_*.sql`.

#### Arquivo SQL (marcador)

`internal/storage/migrations/009_cre_confidence.sql`:

```sql
-- 009_cre_confidence.sql
-- Sprint 2: Candidate Ranking Engine — persistir confidence score de
-- product_name para que a Fase 3 (LLM) saiba quais mensagens precisam
-- de inferência (confidence abaixo do threshold).
CREATE TABLE IF NOT EXISTS schema_migrations (
    version    TEXT PRIMARY KEY,
    applied_at TEXT NOT NULL DEFAULT (datetime('now'))
);
```

#### Coluna adicionada (fisicamente, via Go)

Declarada em DOIS lugares (ambos obrigatórios):

1. **`001_initial.sql`**:

```sql
product_name_confidence REAL NOT NULL DEFAULT 0.0,
```

2. **`currentSchemaColumns` em `migrations.go`**:

```go
{table: "processed_messages", name: "product_name_confidence", definition: "REAL NOT NULL DEFAULT 0.0"},
```

### 2.9 Atualização do model

`internal/model/normalized.go`:
```go
ProductNameConfidence float64 // score 0.0–1.0 do CRE
```

`internal/model/processed.go`: campo correspondente.

### 2.10 Estrutura de código modular

```
internal/processor/
├── cre.go                 (NOVO — engine: Extract, orquestra 3 estágios)
├── cre_types.go           (NOVO — Candidate, ProductNameResult)
├── cre_sources.go         (NOVO — Stage 1: WebpageTitle, TextHeuristic, URLTitle)
├── cre_scorers.go         (NOVO — Stage 2: Position, Capitalization, TechDensity, BrandMatch, Proximity, MetaPenalty)
├── cre_normalize.go       (NOVO — Stage 3: normalização pós-extração)
├── cre_data.go            (NOVO — embed.FS + loadEmbedList)
├── data/
│   ├── brands.txt         (NOVO — marcas conhecidas)
│   ├── tech_tokens.txt    (NOVO — tokens técnicos para TechDensity)
│   ├── merchant_suffixes.txt (NOVO — sufixos de merchant pra remover)
│   └── promo_noise.txt    (NOVO — palavras de promoção residual)
├── cre_test.go
├── cre_sources_test.go
├── cre_scorers_test.go
└── cre_normalize_test.go
```

### 2.11 Integração no pipeline

Em `Normalize()` (normalizer.go), após `extractModifiers`:

```go
// Estágio: Candidate Ranking Engine
pnResult := ExtractProductName(nm)
nm.ProductName = pnResult.Name
nm.ProductNameConfidence = pnResult.Confidence
```

Não há mudança na concorrência do processor (single-threaded, determinístico).
CRE é CPU-only, sem rede.

### 2.12 Verificação do Sprint 2

```
go build ./...
go vet ./...
go test ./internal/processor/...
```

Re-processar banco e medir:
- `% de deal_* com product_name != ''` ≥ 50%
- `% com product_name_confidence >= threshold` ≈ cobertura total
- Amostragem manual de 50 registros: precisão ≥ 80%

---

## Sprint 3 — URL Resolver (Estágio 2)

### Objetivo

Resolver URLs canônicas (follow redirects, remove tracking params), extrair
`<title>` como fonte complementar para o CRE, e limpar affiliate links do
campo `url` servido ao frontend.

### Escopo

O URL Resolver é o subsistema de rede do processor. É a peça mais cara e
complexa. Entrou como complemento, não como pré-requisito.

Funcionalidades:
1. **Canonicalização**: HEAD/GET com follow redirects (max 10), remove
   tracking params (UTM, tag, affiliate IDs), lowercase host.
2. **Title extraction**: extrai `<title>` da página final via regex (não
   DOM parser — leve e suficiente).
3. **Affiliate cleaning**: remove params de afiliado (`tag`, `ref_`,
   `affiliate_id`, `mmp_pid`, etc.) da URL servida.
4. **Cache persistente**: `url_resolutions(original, canonical, merchant, title, resolved_at)`.
5. **(Futuro) Re-afiliação**: estudo separado. O resolver prepara o terreno
   (canonical URL limpa) para futura substituição por link de afiliado próprio.

### Constraints de rede

- **Rate limiting**: cada merchant tem limite. Backoff exponencial + cache
  agressivo (URLs canônicas são estáveis e cacheadas indefinidamente).
- **Timeout**: 5s por resolução. Se falha, mantém URL original, marca
  `url_resolved = false`.
- **User-Agent**: usar UA de browser real para evitar bloqueios.
- **Fallback gracioso**: se resolução falha (timeout, 403, rate limit), o
  pipeline NÃO bloqueia. A mensagem é processada com URL original.
- **Anti-bot**: Magalu (P4) bloqueia com 403. Marcar como `unresolved`,
  adiar para solução futura (Playwright/Rod — exige avaliação contra
  closed stack do AGENTS.md §11).

### Modelo de dados

```go
// URLResolution é o resultado da resolução de uma URL.
type URLResolution struct {
    OriginalURL  string
    CanonicalURL string
    Merchant     string
    Title        string // <title> da página final
    ResolvedAt   time.Time
    Unresolved   bool   // true se falhou (403, timeout, etc.)
}
```

### Migração de Schema — 010 (Sprint 3)

Arquivo: `internal/storage/migrations/010_url_resolver.sql`

```sql
-- 010_url_resolver.sql
-- Sprint 3: URL Resolver — cache persistente de resoluções e colunas
-- de URL canônica/title/resolved em processed_messages.

CREATE TABLE IF NOT EXISTS url_resolutions (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    original_url  TEXT NOT NULL UNIQUE,
    canonical_url TEXT NOT NULL DEFAULT '',
    merchant      TEXT NOT NULL DEFAULT '',
    title         TEXT NOT NULL DEFAULT '',
    unresolved    INTEGER NOT NULL DEFAULT 0,
    resolved_at   TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_url_resolutions_canonical
    ON url_resolutions(canonical_url) WHERE canonical_url != '';
```

Evolução de `processed_messages` (registradas em
`currentSchemaColumns` em `migrations.go`):

```go
{table: "processed_messages", name: "canonical_url", definition: "TEXT NOT NULL DEFAULT ''"},
{table: "processed_messages", name: "url_title", definition: "TEXT NOT NULL DEFAULT ''"},
{table: "processed_messages", name: "url_resolved", definition: "INTEGER NOT NULL DEFAULT 0"},
```

Atualizar `001_initial.sql` com as mesmas colunas para bancos novos.

### Integração no pipeline

O URL Resolver roda DEPOIS do CRE, como um estágio que enriquece
mensagens com `url_title` e `canonical_url`. Não substitui nenhuma fonte do
CRE — adiciona uma fonte nova (URLTitleSource) que consulta o cache.

```
Normalize
    │
    ▼
CRE (Webpage.Title + TextHeuristic)  → product_name (imediato)
    │
    ▼
URL Resolver (cached)                → url_title, canonical_url
    │                                   ↳ feed de volta para CRE se product_name vazio
    ▼
SaveProcessedBatch
```

### Estrutura de código modular

```
internal/processor/
├── urlresolver.go        (NOVO — Resolver, URLResolution)
├── urlresolver_test.go
├── cre_sources.go        (atualizado — URLTitleSource lê do cache)
└── normalizer.go         (atualizado — chama resolver quando disponível)

internal/storage/
└── processor_repository.go (atualizado — SaveURLResolution, GetURLResolution, ...)
```

### Verificação do Sprint 3

```
go build ./...
go vet ./...
go test ./internal/processor/...
go test -race ./...
```

Re-processar banco e medir:
- `% de msgs com canonical_url != ''` (URL efetivamente resolvida)
- `% de msgs com url_title != ''` (title extraído)
- `% de msgs com url_resolved = 1`
- Tempo de processamento não aumenta significativamente (cache + async)
- Rate limiting: nenhum IP block reportado em Shopee/Amazon
- Affiliate tracking params ausentes em 100% das canonical_url

---

## Migrations posteriores (fora destes sprints)

As tabelas `products` e `price_history` (PROCESSOR-IDEATION §9) NÃO entram
nestes sprints. Elas dependem de dedup evoluindo de URL-hash simples para
entidade `Product` com price tracking. São marcadas para sprint futuro
pós-Sprint 3, quando a deduplicação for aprimorada.

---

## Ordem de execução e dependências

```
Sprint 1 (cupons + modifiers + webpage + schema)
    │  não bloqueia nada, alto impacto, baixo custo
    ▼
Sprint 2 (CRE / ProductName)
    │  peça substantiva, dependência: Sprint 1 pronto (webpage_title)
    ▼
Sprint 3 (URL Resolver)
       dependência: Sprint 2 pronto
       prioridade: reavaliar após medir cobertura do Sprint 2
```

### Critério de prioridade do Sprint 3

Após Sprint 2, re-processar o banco e medir `% deal_* com product_name`.
Se ≥ 70%: Sprint 3 fica lower priority (CRE já cobre bem).
Se 50-70%: Sprint 3 entra como complemento de title.
Se < 50%: Sprint 3 ganha alta prioridade.

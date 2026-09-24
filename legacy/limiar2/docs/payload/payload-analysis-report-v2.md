# Relatório de Análise de Payloads v2

**Data:** 2026-06-09
**Base:** 6674 mensagens persistidas em `raw_messages`
**Canais analisados:** 16
**Janela temporal:** 70 dias (2026-03-30 → 2026-06-08)
**Ferramenta:** `tools/payload-analyzer/main.go` + scripts ad-hoc (jq + curl)
**Relatório anterior:** `docs/payload-analysis-report.md` (239 msgs, 2026-06-07)

---

## 1. Sumário Executivo

Análise de 6674 mensagens revelou um dataset **saudável e diversificado**, com
padrões estáveis em múltiplas dimensões (textuais, temporais, de merchant, de
media). O relatório anterior (239 msgs) estava enviesado pela amostra pequena —
várias de suas "descobertas" não se confirmaram (ex.: "GroupedID sempre
presente" → na verdade é sempre zero; "AliExpress domina merchants" → na
verdade Shopee domina).

**Principais descobertas:**

1. **Shopee é o merchant #1** (2805 URL mentions, 47%) — não AliExpress
2. **CDN do Telegram via `t.me` preview funciona** mas com cache de 3h + ETag
3. **Só 30 entities têm URL** (vs 5788 URLs no texto) — regex é fonte primária
4. **6% das mensagens têm Media=null** — são textuais puras, não erro
5. **Sistema de "moedas virtuais" em 2 plataformas**: AliExpress (92 msgs) e Shopee (36 msgs)
6. **21 URLs aparecem em múltiplos canais** — dedup cross-channel validada
7. **Taxonomia de 10 clusters** cobre 100% das mensagens com regras claras

---

## 2. Distribuição por Canal

| Canal | Mensagens | % do total | Janela temporal |
|-------|-----------|------------|-----------------|
| `jptechofertasgerais` | 5027 | 75.3% | 24h (adicionado recentemente) |
| `lobaopromo` | 409 | 6.1% | 28h |
| `LaPromotion` | 177 | 2.7% | 20h |
| `xetdaspromocoes` | 171 | 2.6% | 28h |
| `gatunopromos` | 168 | 2.5% | 28h |
| `TJGOFERTASs` | 154 | 2.3% | 22h |
| `iuriindica` | 122 | 1.8% | 28h |
| `EconomizandocomJP` | 111 | 1.7% | 24h |
| `Fraguas84Oficial` | 70 | 1.0% | 22h |
| `urubupromo` | 54 | 0.8% | 21h |
| `ENVOLTOTECH` | 49 | 0.7% | 22h |
| `descontogamerfb` | 46 | 0.7% | 21h |
| `jptechnotebooks` | 37 | 0.6% | 21h |
| `ToptechPROMO` | 33 | 0.5% | 20h |
| `garimposdodepinho` | 25 | 0.4% | 20h |
| `TopTechPromoCelular` | 20 | 0.3% | pontual |

**Observação:** `jptechofertasgerais` domina o volume (75%) porque é um canal
"firehose" (~5000 msgs/dia). Foi adicionado com `history_max=10000` antes do
corte temporal existir, e Ctrl+C interrompeu em ~5000. Não é um bug — é o
comportamento real do canal.

---

## 3. Shape dos Payloads

### 3.1 Wrapper detection

- **Direto (mensagem única):** 6003 payloads (89.9%)
- **Wrapper Updates:** 671 envelopes (10.1%)

**Descoberta:** todos os 671 envelopes wrapper têm **exatamente 1 mensagem**
(`Updates[0].Message`). Nunca múltiplas mensagens no mesmo envelope.
Simplifica drasticamente a lógica do estágio de normalização.

### 3.2 Campos top-level

48 campos em média por payload direto. Frequência:

- 100%: `Date` (sempre presente)
- ~90%: campos estruturais (`ID`, `PeerID`, `Message`, `Media`, `Entities`, etc.)
- ~10%: campos de wrapper (`Updates`, `Seq`, `Chats`, `Users`)

### 3.3 Field types

| Campo | Tipos observados |
|-------|------------------|
| `ReplyTo` | `map`, `null` |
| `Media` | `map`, `null` |
| `Entities` | `array`, `null` |
| `Chats` | `array`, `null` |

---

## 4. Análise de URLs (5788 mensagens, 96.4%)

### 4.1 Top domains

| Domínio | Contagem | Merchant | Redirects | Dificuldade |
|---------|----------|----------|-----------|-------------|
| `s.shopee.com.br` | 2805 | **Shopee** | 1 | Fácil |
| `meli.la` | 2054 | Mercado Livre | 1 | **Difícil** ⚠️ |
| `amzn.to` | 1439 | Amazon | 1 | Fácil |
| `www.amazon.com.br` | 908 | Amazon (direto) | 0 | Fácil |
| `t.me` | 175 | Telegram (cross-promo) | 0 | N/A |
| `mercadolivre.com` | 136 | Mercado Livre | 0 | Fácil |
| `magazineluiza.onelink.me` | 133 | Magalu | 1-2 | **Muito difícil** (403) |
| `bit.ly` | 68 | Genérico | 2 | Fácil |
| `eioferta.com.br` | 56 | Agregador | ? | Médio |
| `divulgador.magalu.com` | 54 | Magalu | 1-2 | **Muito difícil** (403) |
| `s.click.aliexpress.com` | 51 | AliExpress | 1 | **Difícil** ⚠️ |
| `tidd.ly` | 51 | Kabum (AWIN) | 2 | Médio |

### 4.2 Investigação de redirects (curl -Ls)

Testado em 1 URL real por domínio:

| Domínio | Destino final | Nota |
|---------|---------------|------|
| `s.shopee.com.br/X` | `shopee.com.br/m/loja` + `mmp_pid` (affiliate) | Params claros, fácil limpeza |
| `amzn.to/X` | `amazon.com.br/dp/{ASIN}?tag=xxx` | **ASIN extraível do path** |
| `amzn.divulgador.link/X` | `amazon.com.br/dp/{ASIN}?tag=yyy` | Mesma estrutura |
| `amzlink.to/X` | `amazon.com.br/dp/{ASIN}?tag=zzz` | Mesma estrutura |
| `meli.la/X` | `mercadolivre.com.br/social/{user}` | ⚠️ **Perfil do afiliado, não produto** |
| `s.click.aliexpress.com/X` | Landing de moedas AliExpress | ⚠️ **Não é produto específico** |
| `magazineluiza.onelink.me/X` | **HTTP 403** | Anti-bot ativo |
| `divulgador.magalu.com/X` | **HTTP 403** | Anti-bot ativo |
| `tidd.ly/X` | `kabum.com.br/produto/{id}` | 2 redirects, AWIN network |
| `bit.ly/X` | `whatsapp.com/channel/...` | Cross-platform promo |

### 4.3 Cross-channel dedup

**21 URLs aparecem em múltiplos canais.** Exemplos:

- `amzn.to/3FWxvh6`: 2 canais, 602 ocorrências (produto viral)
- `mercadolivre.com/sec/2YgfcPA`: 2 canais, 118 ocorrências
- `s.shopee.com.br/1BJkqQVhXr`: 2 canais, 44 ocorrências

**Implicação:** dedup por URL canônica (não original) é mandatório.

---

## 5. Análise de Texto

### 5.1 Distribuição de tamanho

| Percentil | Chars |
|-----------|-------|
| min | 0 (1 mensagem) |
| p50 | 175 |
| avg | 192 |
| p95 | 350 |
| p99 | 507 |
| max | 1076 |

### 5.2 Sinais de conteúdo

| Sinal | Regex | Contagem | % |
|-------|-------|----------|---|
| URL no texto | `https?://` | 5788 | 96.4% |
| Preço (R$) | `R\$` | 4747 | 79.1% |
| Cupom | `cupom\|cupão\|coupon` | 4246 | 70.7% |
| "OFF" | `\bOFF\b` | 1686 | 28.1% |
| Percentual | `\d+%` | 1354 | 22.6% |
| Frete grátis | `frete grátis\|gratis` | 297 | 4.9% |
| Envio nacional | `envio nacional\|envio do brasil` | 87 | 1.4% |
| Corre/corra | `\bcorre\|\bcorram` | 194 | 3.2% |
| Última unidade | `última[s]? unidade\|acabando\|esgotando` | 121 | 2.0% |

### 5.3 Preços extraídos

Distribuição de 6450 preços (`R$ X.XXX,XX`):

| Faixa | Contagem | % |
|-------|----------|---|
| R$ 0-100 | 3565 | 55.3% |
| R$ 100-500 | 2113 | 32.8% |
| R$ 500-2000 | 586 | 9.1% |
| R$ 2000+ | 186 | 2.9% |

Estatísticas: **p50 = R$ 83**, **p95 = R$ 1499**, **p99 = R$ 3607**, max = R$ 40000.

---

## 6. Análise de Cupons (4246 menções, 70.7%)

### 6.1 Padrões identificados

| Padrão | Exemplo | Frequência |
|--------|---------|------------|
| Único simples | `🎟  Cupom: 6DO6` | comum |
| Múltiplos (OU) | `Cupom: AEBR2 ou IFPL90V1 ou MARCABR02` | comum |
| Concatenado (+) | `Cupom: AEBR2 + FIFINE0601 + 954 Moedas no app` | comum |
| Percentual | `Cupom de 15% OFF` | ~5% |
| Valor fixo | `cupom de R$ 30 OFF` | raro |
| Sem código | `Resgate o cupom no anúncio` | ~2% |
| Especificador | `Use o Cupom: PROMONETS ou XETPROMOCOES` | raro |
| Expirado | `Cupom Amazon ULTIMO6DO6 Esgotado` | ~3% |

### 6.2 Moedas virtuais (Shopee + AliExpress)

**Descoberta-chave:** ambos os merchants usam "moedas" como desconto adicional,
mas com semânticas diferentes:

- **AliExpress (92 mensagens):** `Cupom: XXX + N Moedas no app`
  - Moedas são **desconto direto** no checkout
  - Valores típicos: 90, 588, 954 moedas
- **Shopee (36 mensagens):** `50% de cashback em Moedas Shopee`
  - Moedas são **cashback** (crédito futuro, não desconto imediato)
  - Limites de cap: "limite de 1.000 moedas:R$10"
  - Janelas temporais: "Resgate as 23:00 hoje para usar AMANHÃ"

**Recomendação:** modelar como `VirtualCurrency` separado do `Coupon`
(ver PROCESSOR-IDEATION.md §10.3).

---

## 7. Análise de Media (tipos e dimensões)

### 7.1 Tipos de Media

| Tipo | Contagem | % |
|------|----------|---|
| Photo | 5613 | 93.5% |
| **null** | 362 | 6.0% |
| Webpage (link preview) | 20 | 0.3% |
| Video | 7 | 0.1% |
| Document | 7 | 0.1% |
| Poll | 1 | ~0% |

### 7.2 Media=null — NÃO É ERRO

362 mensagens sem `Media` mas com conteúdo válido:
- 62% têm URL no texto
- 75% têm menção a cupom
- 100% têm Views (campo estrutural sempre presente)
- São mensagens **puramente textuais** (cupons, avisos, headers de categoria)

### 7.3 Aspect ratios de Photo

| Dimensões | Tipo | Contagem | Aspect |
|-----------|------|----------|--------|
| 320x320 | `m` (thumb) | 3133 | 1:1 |
| 800x800 | `x` (médio) | 2201 | 1:1 |
| 1024x1024 | `y` (alto) | 477 | 1:1 |
| 1000x1000 | `y` | 389 | 1:1 |
| 450x450 | `x` | 364 | 1:1 |
| 1200x1200 | `y` | 303 | 1:1 |
| 320x168 | `m` | 185 | 1.9:1 |
| 600x315 | `x` | 171 | 1.9:1 |
| 1280x1280 | `y` | 136 | 1:1 |
| 320x180 | `m` | 83 | 16:9 |

**Dominância:** 85% quadrado (1:1); 8% 16:9; 7% 1.9:1.

### 7.4 Video media (7 mensagens)

- Formato: `video/mp4`
- Duração: 21-58s
- Tamanho: 4-48MB
- Codecs: `h264`
- Alguns sem som (`Nosound: true`)

### 7.5 Document media (7 mensagens)

- Arquivos tipo `IMG_7866.MOV`, `IMG_6316.MP4` (vídeos reclassificados)
- Tamanhos similares a Video

### 7.6 Webpage media (20 mensagens)

Link previews estruturados (ouro para o processor):
- `URL`: `s.shopee.com.br/X`
- `Title`: "Espaço Tecnologia | Shopee 2026"
- `Description`: "Conheça o Espaço Tecnologia Shopee..."
- `SiteName`: "s.shopee.com.br"
- `Type`: "photo"
- `Photo`: múltiplos tamanhos (320x320 → 1280x1280)

---

## 8. Análise de Entities

### 8.1 Tipos detectados

| Assinatura | Contagem | Interpretação |
|------------|----------|---------------|
| `{Offset, Length}` | 12804 | Formatação (bold, italic, etc.) |
| `{DocumentID, Offset, Length}` | 100 | Mention de usuário |
| `{Offset, Length, URL}` | 30 | Hyperlink clicável |
| `{Language, Offset, Length}` | 8 | Code block com linguagem |

### 8.2 Entities como fonte de URL — NÃO USAR

**Só 30 entities têm URL vs 5788 URLs no texto.** Entities do Telegram é uma
fonte incompleta para extração de URLs. Regex no texto é fonte primária;
Entities é apenas cross-check/backup.

---

## 9. Análise Temporal

### 9.1 Janela temporal

- **Primeira mensagem:** 2026-03-30 17:17:18
- **Última mensagem:** 2026-06-08 22:05:56
- **Span:** 70 dias

### 9.2 Distribuição por hora do dia

| Hora | Mensagens | % |
|------|-----------|---|
| 00 | 225 | 3.7% |
| 01 | 8 | 0.1% |
| 03 | 108 | 1.8% |
| 04-07 | ~2 | <0.1% |
| 10-11 | 52 | 0.9% |
| 12-14 | 397 | 6.6% |
| 15 | 14 | 0.2% |
| 18 | 81 | 1.3% |
| 20 | 581 | 9.7% |
| **21** | **5099** | **84.9%** |
| 22-23 | 101 | 1.7% |

**Pico dominante:** 21h com 85% do volume (jptechofertasgerais firehose).

### 9.3 Implicações

- Processor pode batch-rodar a cada 5 min sem perder picos
- Frontend pode destacar "horários quentes" (20-22h)
- Scheduler de push notification às 20:30

---

## 10. ReplyTo e Threads

- **72 mensagens com ReplyTo** (1.2%)
- 100% têm `ReplyToMsgID` válido
- Nunca `ReplyToPeerID` isolado
- Amostras: "Precinho demais!!!", "É hora de tentar", "Esse produto é vendido pela magalu galera"

**Uso:** ReplyTo é contexto/conversa, não promoção nova. Processor pode usar
ReplyTo para propagar `expired: true` de mensagens "cupom esgotado" para a
promoção original.

---

## 11. Mensagens Administrativas (não-promoção)

| Categoria | Contagem |
|-----------|----------|
| Regras de grupo | 1 |
| "Cupons de hoje" | 1 |
| Grupos WhatsApp (cross-promo) | 11 |
| Convite para canal Telegram | 5 |
| **Total admin_meta** | ~18 |

**Tratamento:** classificar como `message_type: admin_meta` e excluir do feed
principal (mas manter em histórico para auditoria).

---

## 12. Agrupamentos (GroupedID)

**ZERO ocorrências de GroupedID não-nulo** nos 6003 payloads diretos.

O relatório anterior (239 msgs) dizia que GroupedID estava sempre presente —
foi um artefato da amostra pequena. Na verdade, não há álbuns no dataset.
Processor não precisa lidar com álbuns.

---

## 13. Taxonomia de Mensagens (clusters)

### 13.1 Classificação em clusters (mutuamente exclusivos)

| Cluster | Qtd | % | Definição |
|---------|-----|---|-----------|
| **deal_complete** | 3491 | 58.2% | URL + preço + cupom |
| **deal_no_coupon** | 1250 | 20.8% | URL + preço, sem cupom |
| **deal_no_price** | 1047 | 17.4% | URL, sem preço |
| **category_header** | 173 | 2.9% | Texto curto (<50 chars), sem URL/preço |
| **coupon_expired** | 112 | 1.9% | Menciona "esgotado/acabou/encerrado" |
| **commentary** | 97 | 1.6% | Texto livre sem signals |
| **video** | 7 | 0.1% | Media.Video |
| **document** | 7 | 0.1% | Media.Document |
| **poll** | 1 | ~0% | Media.Poll |
| **admin_meta** | ~18 | ~0.3% | Regras, convites |
| **Total** | 6003 | 100% | — |

### 13.2 Cascata de classificação

```go
func Classify(m NormalizedMessage) MessageType {
    hasURL := HasURL(m.Text)
    hasPrice := HasPrice(m.Text)
    hasCoupon := HasCoupon(m.Text)
    isExpired := IsExpiredMention(m.Text)
    isCategoryHeader := len(m.Text) < 50 && !hasURL && !hasPrice && !isExpired
    isAdmin := IsAdminMeta(m.Text)

    switch {
    case isAdmin:                          return TypeAdminMeta
    case isExpired && !hasURL:             return TypeCouponExpired
    case hasURL && hasPrice && hasCoupon:  return TypeDealComplete
    case hasURL && hasPrice:               return TypeDealNoCoupon
    case hasURL:                           return TypeDealNoPrice
    case isCategoryHeader:                 return TypeCategoryHeader
    case m.MediaType == "video":           return TypeVideo
    case m.MediaType == "document":        return TypeDocument
    case m.MediaType == "poll":            return TypePoll
    default:                               return TypeCommentary
    }
}
```

---

## 14. Análise do CDN do Telegram

### 14.1 Metodologia

- 4 canais testados: `gatunopromos`, `lobaopromo`, `xetdaspromocoes`, `LaPromotion`
- GET `https://t.me/{channel}/{message_id}` + extração da `<meta property="og:image">`
- HEAD na URL do CDN para headers

### 14.2 Resultados

| Aspecto | Valor |
|---------|-------|
| Funciona? | ✅ 4/4 canais |
| Resolução | ~1280x1280 (completa, não thumbnail) |
| Auth necessária? | ❌ Não |
| CORS | `Access-Control-Allow-Origin: *` |
| Cache | `max-age=10800` (**3 horas**) |
| Revalidação | `ETag` + `If-None-Match` → 304 Not Modified |

### 14.3 Implicação

- URLs do CDN **não são imutáveis** — cache de 3h
- Processor deve resolver + cachear com ETag
- Frontend deve usar ETag para revalidação
- Re-resolver a cada 3h (HEAD request barato)

### 14.4 `Photo.ID + AccessHash + FileReference`

**Insuficientes** para construir URL do CDN sem MTProto auth. A única forma
sem auth é via `t.me` preview (caminho do RSSHub).

---

## 15. Idioma

- 95% PT-BR puro (palavras e acentos)
- ~1% EN (títulos de produto importado)
- ~0% ES

Não precisa de detector de idioma na Fase 2.

---

## 16. Comparação com Relatório Anterior (v1)

| Aspecto | v1 (239 msgs) | v2 (6674 msgs) | Mudança |
|---------|---------------|----------------|---------|
| Merchant #1 | AliExpress | **Shopee** | Invertido |
| GroupedID | "sempre presente" | **sempre 0** | Artefato de amostra |
| Entities com URL | 15 (6.3%) | 30 (0.5%) | % caiu drasticamente |
| Media=null | "raro" | **362 (6%)** | Sub-representado em v1 |
| Webpage media | 0 | **20 (0.3%)** | Não detectado em v1 |
| ReplyTo | "sempre null" | **72 (1.2%)** | Sub-representado em v1 |
| Forwards > 0 | 61.5% | **41%** | Invertido |
| Video/Document | 0 | **7 cada** | Não detectado em v1 |
| Moedas virtuais | 0 | **92 AliExpress + 36 Shopee** | Não detectado em v1 |
| Cross-channel dedup | "teórico" | **21 URLs reais** | Confirmado |
| Cache CDN | "imutável" (suposição) | **3h + ETag** | Testado empiricamente |

---

## 17. Recomendações para o Processor

1. **Schema de saída:** incluir `message_type`, `urgency_signals`, `virtual_currency`, `webpage_*`
2. **Estratégia de imagens:** `t.me` preview + ETag cache (3h revalidation)
3. **Merchants priorizados por dados reais:**
   - P1: Shopee, Amazon (fáceis e volumosos)
   - P2: Kabum/AWIN (médio volume)
   - P3: Mercado Livre, AliExpress (URLs caem em landing, não produto)
   - P4: Magalu (HTTP 403, Playwright)
4. **Taxonomia de 10 clusters** com tratamento diferenciado por tipo
5. **Regex calibrado** para preço/cupom/moedas em §10.3 do ideação
6. **Decisões de frontend** baseadas em distribuições reais:
   - Truncagem: 300 chars em card
   - Aspect ratio: 1:1 default
   - Filtros: emoji, faixa de preço, "hot deals"

---

## 18. Arquivos de suporte

- `tools/payload-analyzer/payloads_export.json` — export bruto (1.3M linhas)
- `tools/payload-analyzer/main.go` — analisador Go (corrigido p/ nil panic)
- `/tmp/payload-analysis-v2.txt` — output do analisador Go
- `/tmp/deep.sh`, `/tmp/deep2.sh` — scripts jq das análises Opção A
- `/tmp/surgical.sh`, `/tmp/surgical2.sh` — scripts jq das análises Opção B
- `/tmp/batch_c.sh`, `/tmp/batch_c2.sh` — scripts jq das análises Opção C
- `/tmp/cdn_test.sh`, `/tmp/resolve_urls.sh` — testes do CDN

---

**Fim do relatório.**

Este relatório deve ser lido em conjunto com `docs/specs/PROCESSOR-IDEATION.md`
(atualizado na mesma data com os achados daqui).

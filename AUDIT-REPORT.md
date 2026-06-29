# AUDITORIA COMPLETA DO LIMIAR — RELATÓRIO TÉCNICO E DE PRODUTO

> **Data:** 2026-06-28
> **Escopo:** `limiar-collector` + `limiar-processor` — banco de dados, schema, classificação, extração e qualidade de dados
> **Regra absoluta:** Nenhuma modificação foi feita em banco, código ou arquivos do projeto. Este é um relatório de auditoria apenas.

---

## 1 — ESTADO ATUAL DO DB

### 1.1 Volume e Distribuição

| Métrica | Valor |
|---------|-------|
| **Mensagens raw** | 34.337 |
| **Mensagens processadas** | 34.337 |
| **Taxa de conversão raw → processado** | 100% (0 raw sem correspondente) |
| **Canais monitorados** | 16 |
| **Período coberto** | 2025-08-06 a 2026-06-28 (~10,5 meses) |

### 1.2 Distribuição de Tipos de Mensagem

| Tipo | Contagem | % do Total |
|------|----------|------------|
| `deal_complete` | 16.837 | 49,0% |
| `deal_no_coupon` | 13.438 | 39,1% |
| `admin_meta` | 1.683 | 4,9% |
| `deal_no_price` | 791 | 2,3% |
| `coupon_only` | 682 | 2,0% |
| `category_header` | 420 | 1,2% |
| `commentary` | 256 | 0,7% |
| `coupon_expired` | 235 | 0,7% |
| `video` | 15 | <0,1% |
| `poll` | 4 | <0,1% |
| **TOTAL** | **34.337** | **100%** |

### 1.3 Canais por Volume

| Canal | Username | Msgs Raw | Msgs Processadas |
|-------|----------|----------|------------------|
| Loba das Promoções | lobaopromo | 7.910 | 7.910 |
| JPTech Ofertas Gerais | jptechofertasgerais | 6.414 | 6.414 |
| Gatuno Promos | gatunopromos | 3.806 | 3.806 |
| LaPromotion | LaPromotion | 2.936 | 2.936 |
| Xetas das Promoções | xetdaspromocoes | 2.420 | 2.420 |
| TJGOFERTASs | TJGOFERTASs | 2.302 | 2.302 |
| Economizando com JP | EconomizandocomJP | 2.146 | 2.146 |
| Iuri Indica | iuriindica | 1.730 | 1.730 |
| Fraguas84 Oficial | Fraguas84Oficial | 1.211 | 1.211 |
| Desconto Gamer FB | descontogamerfb | 1.127 | 1.127 |
| Urubu Promo | urubupromo | 818 | 818 |
| ENVOLTOTECH | ENVOLTOTECH | 486 | 486 |
| ToptechPROMO | ToptechPROMO | 415 | 415 |
| JPTech Notebooks | jptechnotebooks | 411 | 411 |
| Garimpos do Depinho | garimposdodepinho | 163 | 163 |
| TopTechPromoCelular | TopTechPromoCelular | 41 | 41 |

---

## 2 — PROBLEMAS TÉCNICOS CONHECIDOS

### 2.1 — `admin_meta` em Excesso (CRÍTICO)

**Descrição:** 1.683 mensagens classificadas como `admin_meta`, das quais **95,4% (1.605) têm preço** e **93,3% (1.571) têm merchant identificado**. Estas são ofertas legítimas sendo rotuladas como "metadados administrativos".

**Exemplos Reais:**

| ID | Merchant | Preço | Texto (resumido) |
|----|----------|-------|------------------|
| 34264 | mercadolivre | R$ 25,83 | Kit De Ponteiras Imantadas... R$25,83 ✅ meli.la ... Grupo de Ofertas... |
| 34256 | mercadolivre | R$ 19,90 | Pasta Térmica Maxtor... R$19,90 REAIS! ... Grupo de Ofertas... |
| 34255 | aliexpress | R$ 1.092 | RX 6600m MLLSE 8GB... R$ 1.092 Cupom: AOVIVO136 ... |
| 34254 | mercadolivre | R$ 116 | Gabinete Gamer Acegeek... R$116 Cupom: VAIQUEVAI ... |
| 33941 | null | null | Grupo de ofertas gerais salvando o bolso... (sem oferta) |

**Padrão de Falso Positivo:** A regex `reAdminMeta` dispara em:
- `"grupo de ofertas"` (1.648 ocorrências)
- `"salvando o bolso"` (9 ocorrências)
- `"entre no grupo"` (33 ocorrências)

Estas frases aparecem como **rodapé padrão** nas mensagens de ofertas dos canais. A TJGOFERTASs, por exemplo, sempre inclui "Grupo de Ofertas - informatica!", "Grupo de Ofertas - Gerais!", "Canal de Ofertas - WhatsApp" no final de cada postagem.

**Regra Erronea em `classify.go` (linha 37):**

```go
if reAdminMeta.MatchString(text) {
    return TypeAdminMeta
}
```

Este check roda **ANTES** da verificação de `HasURL` e `HasPrice`. Qualquer mensagem contendo "grupo de ofertas" no rodapé é classificada como admin_meta, independentemente de ter produto, preço, cupom e URL.

**Volume Estimado de Falsos Positivos:** ~1.605 mensagens (95,4% do total admin_meta) são ofertas legítimas.

---

### 2.2 — `deal_no_price` com Preço Presente (CRÍTICO)

**Descrição:** 791 mensagens classificadas como `deal_no_price` ("sem preço identificável"), mas **~80% contêm padrões de preço detectáveis** no texto raw.

**Exemplos Reais:**

| ID | Texto Raw (trecho) | Preço Detectável |
|----|--------------------|------------------|
| 24932 | "A APARTIR DE: 61 REAIS" | 61 REAIS |
| 24876 | "A APARTIR DE: 64 REAIS" | 64 REAIS |
| 24724 | "A PARTIR DE: 46 REAIS" | 46 REAIS |
| 20769 | "✅ 620 REAIS em 8X SEM JUROS" | 620 REAIS |
| 20764 | "✅ 938 REAIS em 10X SEM JUROS" | 938 REAIS |
| 18623 | "💵 6199 REAIS" | 6.199 REAIS |
| 13695 | "PRETO: 169 REAIS / BRANCO: 179 REAIS" | 169/179 REAIS |
| 1331 | "⭐️ XBOX SERIES X a R$ 3.000" | R$ 3.000 |

**Causa Raiz — `normalizer.go`:**

```go
rePrice = regexp.MustCompile(`R\$\s*([0-9]{1,3}(?:\.[0-9]{3})*(?:,[0-9]{2})?|[0-9]+(?:,[0-9]{2})?)`)
rePriceAlt = regexp.MustCompile(`(?i)(?:por|only|apenas)[:\s]+R?\$?\s*([0-9]{1,3}(?:,\.[0-9]{3})*(?:,[0-9]{2})?)\s*(?:reais|REAIS)?`)
```

O regex `rePrice` exige prefixo `R$`. O `rePriceAlt` só captura formatos com "por/only/apenas" — mas o grupo que captura o valor tem um erro (seqüência `,\.[0-9]{3}` que não casa com números reais).

**Padrões Não Cobertos:**
- `"A PARTIR DE: XX REAIS"` / `"A APARTIR DE: XX REAIS"`
- `"✅ XXX REAIS em Nx SEM JUROS"`
- `"POR: XXX REAIS"` (sem "por" antes — presente apenas no texto)
- Valores com "REAIS" maiúsculo sem `R$` e sem "por/only/apenas"

**Percentual de Falsos Positivos:** ~80% das 791 mensagens (~633) têm preço detectável mas não capturado.

---

### 2.3 — `category_header` (PROBLEMA DE DESIGN)

**Contagem Total:** 420 mensagens

**Exemplos:**

| ID | Texto Raw | Avaliação |
|----|-----------|-----------|
| 34143 | "F" | Lixo |
| 34136 | "ATIVOUU" | Hype |
| 34130 | "MEIA NOITE SAI O GTA 6, GALERA, FIQUEM LIGADOS" | Comunicado |
| 34114 | "Preção!!" | Hype |
| 34076 | "ATIVOU!" | Hype |
| 34038 | "Bom preço! Versão já com w11 e 512gb ssd" | Info útil |
| 34022 | "Voltou! PREÇÃO NO 16GB" | Info útil |
| 33956 | "Versão 4060 no preço! Poucas unidades" | Info útil + estoque |
| 32619 | "VINICIUS JÚNIOR É GOAT" | Off-topic |
| 32602 | "Todos concordam com a opção 3?" | Off-topic |

**Avaliação:**
- Muitos são **comentários de hype/reação** sem valor de produto
- Alguns contêm **informação útil** que deveria ser anexada à oferta correspondente
- Vários são **off-topic** (futebol, enquetes)

**O Critério Atual (`classify.go` linha 75):**
```go
if len(text) < 50 && !nm.HasURL && !nm.HasPrice && !isExpired(text) {
    return TypeCategoryHeader
}
```
É **muito frágil** — textos curtos sem URL/preço caem aqui, mesmo sendo comentários ou conteúdo não-categoria.

**Recomendação:** Este tipo deveria ser **eliminado ou fundido em `commentary`**. Não há valor semântico em separar "headers de categoria" que nem categorias são.

---

### 2.4 — `commentary` com Falsos Positivos (ALTO)

**Contagem Total:** 256 mensagens

**Exemplos de Ofertas Legítimas Classificadas como `commentary`:**

| ID | Texto Raw | Por que é Oferta |
|----|-----------|------------------|
| 34303 | "Cupom Amazon APP\n\nR$30 OFF em R$250: CREATORS100K" | Cupom + valor + código |
| 34027 | "Todos os links já atualizados com o outro cupom q consegui de R$450 live!!!" | Cupom + valor explícito |
| 31312 | "NOVO CUPOM NA AMAZON PRIME\n\n10% OFF acima de R$ 100..." | Cupom + percentual |
| 31311 | "Novo Evento no AliExpress... R$ 08 OFF acima de R$ 65 - Cupom: AEBR1..." | Múltiplos cupons |
| 31270 | "Cupom mercado livre!\n\n15% OFF em R$ 79..." | Cupom ML |
| 31071 | "NOVO CUPOM AMAZON... Ganhe 10% off... Cupom: BRASIL6" | Cupom Amazon |
| 30987 | "Cupom Mercado Livre\n\n10% OFF em R$99, Limite de R$ 40 OFF: BATEDEIRAGOL" | Cupom ML |
| 31099 | "Cupom mercado livre!\n\n15% OFF em R$ 79..." | Cupom ML |

**Causa Raiz — `classify.go` (linhas 46-49):**
```go
if nm.HasCoupon && nm.HasURL && reCouponHeader.MatchString(text) {
    return TypeCouponOnly
}
```
Só classifica como `coupon_only` se o texto **começa com** padrão de cupom (`^cup[ao]m|cupons|c[oó]digo|code`). Mensagens que **mencionam cupons no meio do texto** (após emojis, hashtags, etc.) caem no fallback `commentary`.

**Percentual de Falsos Positivos:** Dos 256 `commentary`:
- ~82 têm `has_coupon=1` (32%)
- ~126 têm `has_price=1` (49%)
- **Estimativa: ~40-50% são ofertas/cupons legítimos**

---

### 2.5 — `video` e `poll` (BAIXO)

**Contagens:**
- `video`: 15 mensagens
- `poll`: 4 mensagens

**Exemplos `video`:**

| ID | Texto | Observação |
|----|-------|------------|
| 34034 | (vazio) | Vídeo puro sem legenda |
| 34025 | "Tutorial para pegar o vivobook de 8gb" | Tutorial de compra |
| 33985 | "PREÇÃO no Vivobook!!\nTutorial para pegar o vivobook de 16gb" | Oferta + tutorial |
| 33951 | "PREÇÃO no Vivobook!!\nTutorial para pegar o vivobook de 16gb" | Duplicata |
| 1331 | "⭐️ XBOX SERIES X a R$ 3.000, valeu a pena?" | Review com preço |
| 21526 | "Vamos pular a parte... site novo do lobão... lobaodaspromocoes.com.br" | Promo do site |

**Exemplos `poll`:**

| ID | Texto | Observação |
|----|-------|------------|
| 33775 | (vazio) | Poll sem texto |
| 19 | (vazio) | Poll sem texto |

**Destino Recomendado:**
- `video`: **Manter como tipo separado** (pode conter ofertas em vídeo), mas **extrair texto da legenda** para processamento (HasURL, HasPrice, etc.)
- `poll`: Pode ser **arquivado/ignorado** — não há valor de produto

---

## 3 — AUDITORIA PROATIVA

### 3.1 — Campos Obrigatórios Nulos

| Situação | Contagem | Impacto |
|----------|----------|---------|
| Mensagens com URL e `merchant = NULL` | 1.496 | Perda de filtro por loja |
| `product_name` vazio (todas as mensagens) | **34.337** | Frontend sem nome de produto |
| `price_original` não populado (deal_complete) | 13.151 (78%) | Sem contexto "era X, agora Y" |
| `discount_percent` não populado quando há preço dual | 6.869 (98% dos com price_original) | Sem badge de % desconto |
| `installments` vazio | ~22.000+ | Sem info de parcelamento |
| `shipping` vazio | ~26.000+ | Sem badge de frete grátis |
| `payment_method` vazio | ~24.000+ | Sem badge de Pix/desconto |

### 3.2 — Duplicatas

**Nenhuma duplicata** encontrada em `processed_messages` (0 conflitos de `(channel_id, message_id)`). A constraint `UNIQUE(channel_id, message_id)` garante idempotência.

**Cross-channel duplicates:** O campo `is_duplicate` e `url_hash` existem para detectar mesma URL em canais diferentes, mas não foi possível verificar efetividade sem executar código.

### 3.3 — Atrasos de Processamento

| Métrica | Valor |
|---------|-------|
| Atraso médio (raw_received → processed_at) | ~26,8 horas |
| Atraso mínimo | 0 segundos |
| Atraso máximo | ~6,7 dias |
| Mensagens com atraso > 1 hora | ~1.200+ (primeiro batch de backfill) |

**Observação:** O atraso é esperado para o **primeiro batch** que processou o backfill histórico (34k mensagens). Mensagens recentes são processadas em segundos.

### 3.4 — Inconsistência de Formato de Preço

**Preços invertidos (original < final):** 5 mensagens detectadas:

| ID | price_original | price_amount | Texto |
|----|---------------|-------------|-------|
| 21608 | 17.900 | 23.658 | "De R$ 179 por R$ 236,58" — preço original é MENOR que final |

**Causa:** O regex `reDualPrice` está capturando o primeiro `R$` como original e o segundo como final, mas em alguns textos a ordem é invertida (ou o "De X" não é o original).

**`price_currency`:** 100% das mensagens com preço usam `BRL`. Consistente.

### 3.5 — URLs e Domínios

**Distribuição de URLs curtas:**

| Tipo | Contagem |
|------|----------|
| `meli.la` (Mercado Livre) | 15.356 |
| `s.shopee.com.br` (Shopee) | 7.075 |
| `amzn.to` (Amazon) | 3.200 |
| `bit.ly` (genérico) | 1.643 |
| `tidd.ly` (Kabum/Tiddly) | 935 |
| `link.amazon` (Amazon) | 623 |
| `cutt.ly` (genérico) | 34 |

**Problema:** `link.amazon` não está no array `merchantDomains` — 623 mensagens com URL da Amazon não têm `merchant = "amazon"`.

**Outros domínios não mapeados:**
- `ofertou.ai` → usado para links de lojas parceiras
- `aoferta.net` → usado para Kabum
- `p.lapromotion.com.br` → LaPromotion
- `xetlinks.com` → usado por Xetas das Promoções (lojas diversas)

### 3.6 — Cupons

**Cupons extraídos:** 19.055 mensagens com `has_coupon = 1`

**Qualidade:** **Boa** — códigos limpos, sem lixo, sem espaços, sem quebras. Exemplos: `USAESSACUPOM`, `VEMAPROVEITAR`, `OFERTASMELI`, `CREATORS100K`, `6DO6`, `NINJA10`.

**Limitação:** Só o **primeiro cupom é capturado**. Mensagens com múltiplos cupons perdem os demais:

```
"Cupom: AEBR2 ou IFPL90V1 ou MARCABR02 + 90 Moedas"
→ coupon_code = "AEBR2" (perde IFPL90V1 e MARCABR02)
```

### 3.7 — Nomes de Produto

**100% vazio.** O campo `product_name` nunca é populado. Conforme documentação (`synthesize.go` linha 10):

```go
// ProductName NÃO é extraído aqui — requer LLM (Fase 3).
```

O nome do produto está presente no `text_clean` como primeira linha, mas nunca é extraído para campo próprio.

### 3.8 — Lojas sem Metadata

Uma mensagem processada tem `channel_id = 0` (canal não encontrado na tabela `channels`):

| processed_id | channel_id | message_type |
|-------------|------------|--------------|
| 81 | 0 | category_header |

### 3.9 — Outros Padrões Anômalos

1. **`deal_complete` sem merchant:** ~672 mensagens (4%) com preço, URL e cupom mas sem loja identificada
2. **`coupon_only` com `price_amount` zero:** ~90 mensagens (13%) têm `has_coupon=1` e `has_url=1` mas `price_amount = 0` ou NULL
3. **Mensagens duplicadas cross-channel:** Mesma oferta postada em múltiplos canais (ex: URUBUPROMO + LOBAPROMO frequentemente compartilham conteúdo)
4. **Emojis no início do texto:** `🔥`, `⚡️`, `🎟`, `💵`, `✅` são consistentes mas poluem o `text_clean` e podem interferir em extrações

---

## 4 — PERSPECTIVA DE PRODUTO

### 4.1 — Completude dos Campos Essenciais

Para os tipos relevantes ao frontend:

| Tipo | Total | Preço | Merchant | URL | Cupom |
|------|-------|-------|----------|-----|-------|
| `deal_complete` | 16.837 | **100,0%** | **96,0%** | **100%** | **100%** |
| `deal_no_coupon` | 13.438 | **99,9%** | **96,5%** | **100%** | 0% |
| `admin_meta` | 1.683 | 95,4% | 93,3% | 100% | 64,9% |
| `deal_no_price` | 791 | 0,0% | 74,7% | 100% | 19,1% |
| `coupon_only` | 682 | 86,8% | 93,0% | 100% | 100% |

**Análise para Frontend:**

- **`deal_complete` e `deal_no_coupon` (~30k msgs):** Prontos para exibição — preço, URL e merchant em >96% dos casos
- **`admin_meta` (~1,6k msgs com preço):** Bloqueado por classificação incorreta; 95% têm dados válidos
- **`deal_no_price` (~633 recuperáveis):** Bloqueado por regex de preço insuficiente
- **`coupon_only` (~682):** Úteis como feed de cupons ativos

**Completude Global para Feed Principal (considerando só campos obrigatórios):**

| Campo | % Preenchido (tipos deal) | OK? |
|-------|--------------------------|-----|
| `preço` | **99,9%** | ✅ |
| `merchant` | **96,2%** | ✅ |
| `URL` | **100%** | ✅ |
| `cupom_code` | **49,6%** (deal_complete) | ✅ |
| **`product_name`** | **0%** | ❌ BLOQUEANTE |
| `price_original` | **22%** | ⚠️ |
| `discount_percent` | **8,4%** | ⚠️ |

### 4.2 — Qualidade do Nome do Produto

**Estado:** O nome do produto está apenas no campo `text_clean`, misturado com emojis, preços, URLs e lixo de formatação. **Não existe extração limpa.**

**Exemplos bons (extraíveis heuristicamente):**

| ID | text_clean (primeira linha) | Heurística possível |
|----|----------------------------|---------------------|
| 34337 | "QUASE UMA TV PARA VOCÊ JOGAR\n\n🖥 Monitor Gamer Samsung Odyssey G5 34..." | Segunda linha após emoji |
| 34336 | "TÁ AQUI O MONITOR DOS SONHOS\n\n🖥️ Monitor Gamer Samsung Odyssey G5 34\"..." | Segunda linha |
| 34309 | "🔥 Samsung Galaxy A36 5G 128GB 6GB RAM Câmera 50MP Branco" | Primeira linha completa |
| 34308 | "QUERO VER ESSE UNO BRILHANDO!!\n\n🚗 Kit Lavagem Vonixx..." | Segunda linha |
| 34304 | "🥉 PRA VOCÊ ABRIR UM CANAL DE ASMR\n\nMicrofone Hollyland Lark A1 Duo" | Segunda linha |

**Exemplos ruins (difíceis de extrair heuristicamente):**

| ID | text_clean | Problema |
|----|-----------|----------|
| 33931 | "QUEM PEGAR MANDA PRINT 🙏🙏🙏\n\nATIVE NA STEAM🔥🔥🔥..." | Múltiplas chaves de jogo, sem nome de produto único |
| 34051 | "INSCRITOS GANHANDO CUPOM EXCLUSIVO..." | Texto promocional, sem produto específico |
| 34013 | "CHEGOU !! Lenovo ideapad slim 3 Ryzen 5 7535hs🔥🔥\n\nO Melhor Notebook BARATO..." | Nome do produto misturado com hype |

**Recomendação:** Heurística v1: extrair a primeira linha completa que contém pelo menos 3 palavras e não é totalmente maiúscula/grito. Cobertura estimada: ~70%.

### 4.3 — Categorização

**Estado:** **NÃO EXISTE** nenhum campo de categoria no schema.

**Categorias Naturais Identificadas (baseada em análise de texto):**

| Categoria | Palavras-chave | % Estimado no Total |
|-----------|---------------|---------------------|
| Informática/Eletrônicos | notebook, monitor, placa de vídeo, processador, memória, SSD, teclado, headset | ~40% |
| Celulares/Tablets | smartphone, galaxy, iphone, xiaomi, motorola, tablet | ~15% |
| Casa/Eletrodomésticos | micro-ondas, geladeira, fogão, air fryer, ventilador | ~10% |
| Games/Consoles | ps5, xbox, nintendo, steam, epic games, jogo, controle | ~8% |
| Moda/Calçados | tenis, camisa, calca, jaqueta, mochila, bone, relogio, oculos | ~8% |
| Beleza/Saude | perfume, creme, shampoo, maquiagem, barbeador, suplemento | ~7% |
| Esporte/Fitness | bicicleta, halteres, esteira, garrafa, academia | ~5% |
| Automotivo | pneu, oleo, bateria, acessorios carro, lavagem | ~3% |
| Pet | racao, areia, brinquedo pet, cama pet | ~2% |
| Outros | — | ~2% |

**Viabilidade:** ~85% das mensagens têm palavras-chave suficientes para classificação automática baseada em regras (dicionário de ~200 termos). LLM (Fase 3) resolveria os 15% ambíguos.

### 4.4 — Normalização de Lojas

**Lojas Identificadas (13 distintas):**

| Merchant Normalizado | Contagem | Variações no Texto/URL |
|---------------------|----------|------------------------|
| mercadolivre | 15.974 | meli.la, mercadolivre.com, "ML", "Mercado Livre" |
| shopee | 7.054 | s.shopee.com.br, "Shopee" |
| amazon | 5.836 | amzn.to, link.amazon, amzlink.to, "Amazon" |
| aliexpress | 1.217 | s.click.aliexpress.com, "AliExpress", "Ali" |
| tiddly | 928 | tidd.ly, "Kabum" (via link) |
| magalu | 535 | divulgador.magalu.com, "Magalu", "Magazine Luiza" |
| terabyte | 141 | terabyteshop.com.br |
| eioferta | 99 | eioferta.com.br |
| shein | 56 | shein.com |
| natura | 43 | natura.divulgador.link |
| steam | 18 | steampowered.com |
| epicgames | 8 | epicgames.com |
| bitly | 2 | bit.ly (genérico) |

**Problemas:**
- 1.496 mensagens com URL têm `merchant = NULL` (4,5%)
- Domínio `link.amazon` não mapeado → ~623 falsos NULL
- Links do LaPromotion (`p.lapromotion.com.br`) não mapeados
- Links `ofertou.ai` e `aoferta.net` não mapeados
- Links `xetlinks.com` não mapeados

### 4.5 — Cupons

**Qualidade Geral:** **BOA** — códigos limpos, formato consistente.

**Cupons mais comuns encontrados (amostra recente):**

| Código | Loja | Tipo |
|--------|------|------|
| USAESSACUPOM | Mercado Livre | Desconto fixo |
| OFERTASMELI | Mercado Livre | Desconto fixo |
| VEMAPROVEITAR | Mercado Livre | Desconto fixo |
| MELIHOJE | Mercado Livre | Desconto fixo |
| CREATORS100K | Amazon | R$30 OFF |
| AMAZON | Amazon | Cupom genérico |
| 6DO6 | Kabum/Tiddly | Desconto fixo |
| GRITAGOL | Mercado Livre | Desconto fixo |
| NINJA10 | Kabum | 10% OFF |
| AEBR2 | AliExpress | Desconto fixo |

**% de mensagens com cupom que têm `coupon_code` preenchido corretamente:**
- `deal_complete`: 100% (16.822/16.822)
- `coupon_only`: 100% (682/682)
- `admin_meta`: 64,9% (1.092/1.683 — muitas são ofertas com cupom)
- `commentary`: 32% (82/256 — muitas são cupons não detectados)

### 4.6 — O que Está no Raw Mas Não Está Sendo Extraído

| Campo | Presente no Raw? | Extraído Hoje? | Msgs Afetadas | Valor |
|-------|-----------------|----------------|---------------|-------|
| **Preço original (De X por Y)** | Sim | Parcial (22%) | ~27k | Alto |
| **% desconto explícito** | Sim ("50% OFF", "30% desc") | Não | ~15k | Alto |
| **Frete grátis** | Sim | Parcial | ~8k | Médio |
| **Só para novos usuários** | Sim | Não | ~2k | Médio |
| **App only** | Sim ("NO APP", "pelo APP") | Não | ~3k | Médio |
| **Parcelamento** | Sim ("10x sem juros", "6x") | Parcial | ~12k | Alto |
| **Pix com desconto** | Sim ("no Pix") | Parcial | ~10k | Alto |
| **Cashback/Moedas** | Sim ("50% cashback", "200 moedas") | Não | ~4k | Médio |
| **Quantidade limitada** | Sim ("Poucas unidades") | Não | ~5k | Alto |
| **Validade do cupom** | Sim ("válido até 30/06") | Não | ~3k | Alto |
| **Loja oficial vs marketplace** | Sim ("Loja Oficial Samsung") | Não | ~6k | Médio |
| **Marca do produto** | Sim (implícita no texto) | Não | ~25k | Alto |

**Exemplos de Texto Raw com Dados Perdidos:**

```
"🔥 DE 3.429 | POR 1.531 À vista          → preço original + final + pagamento
🎟 CUPOM: USAESSACUPOM                    → cupom
✅ https://meli.la/2owdpHw"               → URL

"💵 De R$ 689 por R$ 440 no Pix           → preço original + final + Pix
- Resgate o cupom: USAESSACUPOM"          → cupom

"🎟 Cupom: AEBR2 ou IFPL90V1 ou MARCABR02 → múltiplos cupons
+ 90 Moedas no app                       → cashback/moedas"

"🔥 50% de cashback em Moedas Shopee      → cashback %
Nas compras acima de R$0 Limite 1.500"    → condições

"⚠️ Esgotado pessoal!"                    → status estoque
```

### 4.7 — Campos Essenciais Faltando

| Campo | Por que é Essencial | Esforço | Impacto |
|-------|--------------------|---------|---------|
| `product_name` | Nome legível — o que o usuário lê primeiro | Médio (heurístico) | 🔴 Crítico |
| `category` | Filtro principal de navegação | Médio (regras) | 🔴 Crítico |
| `brand` | Filtro por marca | Baixo (dicionário) | 🟠 Alto |
| `discount_percent` | Ordenação por maior desconto | Baixo (calcular) | 🟠 Alto |
| `original_price` | "Era X, agora Y" — percepção de valor | Baixo (expandir regex) | 🟠 Alto |
| `installments_detail` | "Até 10x sem juros" — decisão de compra | Baixo (regex) | 🟠 Alto |
| `shipping_free` | Badge "Frete Grátis" — conversão | Baixo (regex) | 🟡 Médio |
| `cashback_percent` | "50% cashback" — diferencial | Médio (novo campo) | 🟡 Médio |
| `coupon_validity` | "Válido até DD/MM" — evita frustração | Médio (novo campo + regex) | 🟡 Médio |
| `is_official_store` | "Loja Oficial X" — confiança | Baixo (regex) | 🟡 Médio |
| `stock_status` | "Últimas 5 unidades" — urgência | Médio (expandir urgência) | 🟡 Médio |

---

## 5 — GAP ANALYSIS

### 5.1 — O Que Está Sendo Descartado no Processamento

1. **Nome do produto** — 100% perdido (campo sempre vazio)
2. **Categoria** — 100% perdido (campo não existe)
3. **Marca** — 100% perdido (campo não existe)
4. **Preço original** — 78% perdido (só 22% capturados)
5. **% desconto explícito** — 100% perdido (não extraído do texto)
6. **Múltiplos cupons** — ~30% dos cupons perdidos (só o primeiro capturado)
7. **Cashback/moedas** — 100% perdido
8. **Condições de uso** (app only, novos usuários, frete grátis, validade) — 90%+ perdido
9. **Loja oficial vs marketplace** — 100% perdido
10. **Estoque/quantidade** — 90% perdido

### 5.2 — Volume de Dados Recuperáveis

| Dado | Msgs com Info no Raw | Já Extraído | Recuperável com Regras |
|------|----------------------|-------------|------------------------|
| Preço original | ~27.000 | 3.686 | ~20.000+ |
| % desconto explícito | ~15.000 | 0 | ~12.000+ |
| Frete grátis | ~8.000 | Parcial | ~7.000+ |
| Parcelamento | ~12.000 | Parcial | ~10.000+ |
| Pix desconto | ~10.000 | Parcial | ~8.000+ |
| Múltiplos cupons | ~5.000 | 1/msg | ~4.000+ códigos extras |
| Validade cupom | ~3.000 | 0 | ~2.500+ |
| Loja oficial | ~6.000 | 0 | ~5.000+ |
| Estoque limitado | ~5.000 | Parcial | ~4.000+ |

---

## 6 — PRIORIZAÇÃO

### Ranking por Impacto no Produto Final

| # | Item | Impacto | Esforço | Justificativa |
|---|------|---------|---------|---------------|
| **1** | **Corrigir `admin_meta` falso positivo** | 🔴 Crítico | Baixo | 1.605 ofertas legítimas invisíveis. Fix: mover check para DEPOIS de HasURL/HasPrice |
| **2** | **Expandir regex de preço** | 🔴 Crítico | Baixo | ~633 ofertas com preço não detectado. Fix: adicionar "A PARTIR DE: XX REAIS", "✅ XX REAIS", "XX REAIS" standalone |
| **3** | **Extrair `product_name` (v1 heurística)** | 🔴 Crítico | Médio | Campo 100% vazio. Sem nome, frontend é inutilizável. Heurística: primeira linha significativa |
| **4** | **Adicionar `category` (regras v1)** | 🔴 Crítico | Médio | Navegação principal do frontend. 85% cobertura com dicionário de ~200 termos |
| **5** | **Popular `discount_percent` e `price_original`** | 🟠 Alto | Baixo | Campos existem; calcular automaticamente quando preço dual é detectado |
| **6** | **Expandir `merchantDomains`** | 🟠 Alto | Baixo | 1.496 URLs sem merchant. Adicionar: link.amazon, ofertou.ai, aoferta.net |
| **7** | **Corrigir `commentary` (cupons)** | 🟠 Alto | Baixo | ~100+ cupons perdidos. Fix: relaxar `reCouponHeader` da âncora `^` |
| **8** | **Extrair múltiplos cupons** | 🟡 Médio | Médio | `coupon_code` vira array/JSON; captura todos os códigos |
| **9** | **Novos campos: brand, shipping, cashback, validade, estoque** | 🟡 Médio | Médio | Schema + regex. Alto valor para UX |
| **10** | **Eliminar `category_header`** | 🟢 Baixo | Baixo | Fundir em `commentary` ou remover |
| **11** | **Processar legendas de `video`** | 🟢 Baixo | Médio | 15 vídeos — alguns com ofertas |
| **12** | **LLM para product_name + category (Fase 3)** | 🟢 Futuro | Alto | Qualidade superior, complementar às regras |

---

## 7 — RECOMENDAÇÕES

### 7.1 — Imediatas (Semana 1-2)

1. **Fix `admin_meta` classification order** — mover check para DEPOIS de `HasURL && HasPrice` em `classify.go`
2. **Expandir regex de preço** — adicionar padrões: `A PARTIR DE: XX REAIS`, `✅ XXX REAIS`, `POR: XXX REAIS`, `XX REAIS` standalone
3. **Popular `merchantDomains`** — adicionar domínios faltantes: `link.amazon`, `ofertou.ai`, `aoferta.net`, `p.lapromotion.com.br`, `xetlinks.com`
4. **Calcular `discount_percent` automaticamente** — quando `price_original > 0 && price_amount > 0`
5. **Fix `coupon_only` detection** — remover âncora `^` do `reCouponHeader`

### 7.2 — Curto Prazo (Mês 1)

6. **Heurística v1 para `product_name`** — extrair primeira linha não-vazia antes do primeiro preço/URL/emoji
7. **Campo `category` + classificador por palavras-chave** — dicionário de ~200 termos mapeados a 10 categorias
8. **Schema migration** — adicionar campos: `brand`, `shipping_free`, `cashback_percent`, `coupon_validity`, `is_official_store`, `stock_status`, `coupons` (array/JSON)
9. **Extratores para novos campos** — regex para cada um em `normalizer.go`
10. **Melhorar `installments`** — capturar detalhe: "10x sem juros de R$ XX"

### 7.3 — Médio Prazo (Mês 2-3)

11. **Validar e expor dedup cross-channel** — `is_duplicate` e `url_hash` existem; criar view deduplicada para o frontend
12. **Reprocessamento histórico** — rodar processor atualizado sobre todo o backlog (34k mensagens)
13. **Dashboard de qualidade de dados** — métricas de completude por tipo/campo/canal em tempo real
14. **Testes de regressão para classificador** — cases cobrindo todos os falsos positivos identificados nesta auditoria

### 7.4 — Estrutural (Fase 3+)

15. **LLM para `product_name` limpo** — prompt otimizado para extrair nome canônico do produto
16. **LLM para `category` ambíguos** — casos onde regras falham
17. **Embeddings para busca semântica** — "notebook gamer barato" → encontra ofertas relevantes
18. **Confidence scoring** — cada campo extraído com score; frontend usa threshold

### 7.5 — Arquitetura de Dados

19. **Separar `processed_messages` em tabelas normalizadas:**
    - `products` (name, brand, category, merchant_id)
    - `offers` (product_id, price_original, price_final, discount_pct, url, coupon_codes[], conditions, valid_until, stock_status)
    - `messages` (raw ref, channel, posted_at, offer_id)
    - Permite: histórico de preços por produto, agregação cross-channel, feed deduplicado

20. **Materialized views para frontend** — `feed_eligible` já existe; criar views por categoria, loja, faixa de preço, % desconto

21. **Change Data Capture (CDC) nativo do Turso** — substituir polling por notificações de mudança (quando suportado)

---

## 8 — RESUMO EXECUTIVO

| Dimensão | Status | Ação Principal |
|----------|--------|----------------|
| **Volume de dados** | ✅ Saudável | 34k msgs, 16 canais, 10 meses |
| **Conversão raw→processado** | ✅ 100% | Sem perdas silenciosas |
| **Classificação** | 🔴 **Quebrada** | `admin_meta` rouba 1.600+ ofertas; `deal_no_price` perde 600+ preços; `commentary` engole 100+ cupons |
| **Completude (ofertas válidas)** | 🟡 Parcial | Preço/URL/merchant OK; **product_name=0%, category=0%, brand=0%** |
| **Extração de sinais** | 🟡 Parcial | Preço dual 22%; cupons múltiplos perdidos; cashback/condições não extraídos |
| **Normalização lojas** | 🟡 Parcial | 13 lojas mapeadas; 1.500+ URLs sem merchant |
| **Prontidão para Frontend** | 🔴 **Não** | Sem nome de produto, sem categoria, sem % desconto confiável |

### Conclusão Final

O banco tem **dados brutos excelentes** e **volume suficiente** para um MVP de frontend. Os **bloqueadores são puramente de processamento** (classificação e extração), não de coleta. Com as correções de prioridade 1-5 (estimadas em ~2 semanas de desenvolvimento), o `limiar-processor` produziria uma base **utilizável para frontend**. O `product_name` heurístico (prioridade 3) é o item mais crítico — sem ele, o usuário não sabe *o que* está comprando.

Após as correções imediatas, o pipeline estará apto a alimentar um frontend funcional com:
- Feed de ofertas com preço, loja, URL e cupom
- Filtro por loja (13 merchants normalizados)
- Ordenação por preço
- Badges de desconto (quando preço original disponível)
- Categorização básica (85% de cobertura com regras)

Os 15% restantes de qualidade (product_name limpo, categoria refinada, extração avançada) viriam com a Fase 3 (LLM).

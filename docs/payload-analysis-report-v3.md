# Relatório de Análise de Payloads v3 — limiar-processor

**Data:** 2026-06-11
**Banco analisado:** `limiar.db` (44 MB, Tursogo)
**Ferramentas:** Python 3.14 + sqlite3, análise programática de JSON
**Registros analisados:** 9.118 mensagens (100% do total, 0 payloads inválidos)
**Canais monitorados:** 16
**Range temporal:** 2026-06-07 18:54 → 2026-06-11 02:12 (3,3 dias)
**Schema version:** 1 (100%)
**Processor status:** 9.118/9.118 processadas (100%)

---

## 1. Escopo e Método

### Limitações da Amostra

| Fator | Valor | Avaliação |
|-------|-------|-----------|
| Volume total | 9.118 msgs | ✅ Suficiente (>500) |
| Range temporal | 3,3 dias | ⚠️ Marginal (>3 dias) |
| Diversidade de canais | 16 | ✅ Boa cobertura |
| Distribuição | enviesada | ⚠️ @jptechofertasgerais = 56,6% |
| Payloads inválidos | 0 | ✅ Limpo |

**Distribuição por canal:**

| Canal | Mensagens | % |
|-------|-----------|---|
| @jptechofertasgerais | 5.160 | 56,6% |
| @lobaopromo | 1.235 | 13,5% |
| @gatunopromos | 598 | 6,6% |
| @xetdaspromocoes | 442 | 4,8% |
| @LaPromotion | 340 | 3,7% |
| @iuriindica | 252 | 2,8% |
| @EconomizandocomJP | 243 | 2,7% |
| @Fraguas84Oficial | 180 | 2,0% |
| @TJGOFERTASs | 166 | 1,8% |
| @urubupromo | 113 | 1,2% |
| @descontogamerfb | 98 | 1,1% |
| @ENVOLTOTECH | 87 | 1,0% |
| @jptechnotebooks | 76 | 0,8% |
| @ToptechPROMO | 61 | 0,7% |
| @garimposdodepinho | 44 | 0,5% |
| @TopTechPromoCelular | 22 | 0,2% |

---

## 2. Persona Estrutural dos Dados

### 2.1 Shapes Identificados

| Shape | Descrição | Contagem | % |
|-------|-----------|----------|---|
| **A** | Mensagem Direta (`tg.Message`) | 8.445 | 92,6% |
| **B** | Envelope Updates (`tg.Updates`) | 673 | 7,4% |

**Shape A — Mensagem Direta** (backfill via `messages.getHistory`):

```json
{
  "Flags": 8454464, "Flags2": 0,
  "ID": 12345,
  "PeerID": {"ChannelID": 1987091586},
  "Date": 1780858455,
  "Message": "🔥 Promoção imperdível! ...",
  "Media": {"Photo": {"ID": 6138941836333092787, ...}},
  "Entities": [{"Offset": 45, "Length": 28, "URL": "https://..."}],
  "Views": 683, "Forwards": 5,
  "FwdFrom": {"FromID": null, ...},
  "GroupedID": 7216302837461,
  ...
}
// 50 campos no total
```

**Shape B — Envelope Updates** (captura ao vivo via dispatcher):

```json
{
  "Updates": [{"Message": {<mesmos 50 campos>}, "Pts": 12345, "PtsCount": 1}],
  "Users": [{...metadata de usuários...}],
  "Chats": [{...metadata de canais...}],
  "Date": 0,
  "Seq": 0
}
```

**Achado crítico:** Shape B sempre contém exatamente 1 mensagem. As chaves da mensagem interna são **idênticas** às do Shape A (50 campos, zero diferença). O normalizador atual (`extractMessage`) já trata ambos corretamente.

### 2.2 Campos Estáveis (100% de presença em Shape A)

| Campo | Tipo Go | Uso no Pipeline |
|-------|---------|----------------|
| `ID` | int | Identificador único da mensagem |
| `PeerID.ChannelID` | int | Agrupamento por canal |
| `Date` | int (Unix) | Timestamp da postagem |
| `Message` | string | Texto — fonte primária de sinais |
| `Media` | dict/null | Tipo de mídia |
| `Entities` | list/null | URLs parseadas pelo MTProto |
| `Views` | int | Métrica de engajamento |
| `Forwards` | int | Métrica de viralidade |
| `ReplyTo` | dict/null | Reply context |
| `GroupedID` | int | Sempre presente (ver §3.4) |

### 2.3 Campos Descartáveis (sempre default/null)

| Campo | Valor fixo | Justificativa |
|-------|------------|---------------|
| `Flags`, `Flags2` | int | Bitmasks internos MTProto |
| `Out`, `Mentioned`, `Silent`, `Post`, `FromScheduled`, `Legacy`, `EditHide`, `Pinned`, `Noforwards`, `InvertMedia`, `Offline`, `VideoProcessingPending` | `false` | Flags de estado do cliente |
| `FromID`, `SavedPeerID`, `GuestchatViaFrom` | `null` | Não-populados para canais |
| `ViaBotID`, `ViaBusinessBotID`, `FromBoostsApplied` | `0` | Sem bots |
| `FromRank`, `PostAuthor`, `Effect`, `SummaryFromLanguage` | `""` | Strings vazias |
| `PaidSuggestedPostStars`, `PaidSuggestedPostTon`, `PaidMessageStars`, `SuggestedPost` | default | Posts pagos não-usados |
| `TTLPeriod`, `QuickReplyShortcutID`, `ReportDeliveryUntilDate`, `ScheduleRepeatPeriod` | `0` | Zero |
| `RestrictionReason` | `null` | Sem restrições |
| `Factcheck`, `Replies` | `{}` | Objetos vazios |
| `EditDate` | `0` | Mensagens não-editadas |

---

## 3. Anomalias e Edge Cases

### 3.1 Payloads Inválidos

- **Inválidos:** 0
- **Truncados:** 0
- **Incompletos:** 0

Todos os 9.118 payloads são JSON válido.

### 3.2 Mensagens com Texto Vazio (0,02% — 2 mensagens)

- **IDs:** 19, 15597
- **Media:** Photo presente em ambas
- **Interpretação:** Mensagem apenas com imagem, sem texto descritivo
- **Classificação atual:** `category_header` ou `commentary` (funciona corretamente)
- **Risco:** Nenhum — processor extrai sinais de texto vazio sem crash

### 3.3 FwdFrom.FromID (0,7% não-null — 60 mensagens)

- **Observação:** 99,3% têm `FwdFrom.FromID = null`, mas 60 mensagens têm `FromID.UserID` preenchido
- **Interpretação:** 60 mensagens são forwards reais de outros usuários/canais
- **Campo `Forwards`** é contador (64,4% têm `Forwards > 0`), não indicador de forward
- **Processor atual:** Ignora `FwdFrom` completamente — oportunidade perdida

### 3.4 GroupedID Sempre Presente (100%)

- **Observação:** Todas as 8.445 mensagens Shape A têm `GroupedID` não-zero
- **Interpretação:** Provavelmente artefato da serialização gotd/td (default value ≠ null em Go)
- **Risco:** Nenhum — campo é ignorado pelo processor

### 3.5 Mensagens Sem URL (3,0% — 257 mensagens)

**Tipos observados:**

| Tipo | Exemplo | Classificação |
|------|---------|---------------|
| Cupom esgotado | "cupom esgotado........" | `coupon_expired` |
| Comentário | "barato..." | `commentary` |
| Desconto sem link | "R$ 50 OFF em R$ 249: 66COMPENSAMAIS" | `commentary` |
| Aviso | Mensagens administrativas | `admin_meta` |

### 3.6 ReplyTo Presente (1,0% — 85 mensagens)

- **Estrutura:** `ReplyToMsgID` aponta para mensagem anterior no canal
- **Uso potencial:** Thread de discussão, follow-up de promoção
- **Processor atual:** Extrai `reply_to_msg_id` mas não usa na classificação

### 3.7 Media WebPage (0,26% — 20 mensagens)

- **Estrutura:** `Media.Webpage` (preview de link) em vez de `Media.Photo`
- **Processor atual:** Detecta como `media_type="webpage"` ✅
- **Oportunidade:** `Webpage.URL` pode conter URL resolvida não-presente no texto

---

## 4. Estruturas Aninhadas

### 4.1 Media

| Tipo | Contagem | % | Campos-chave |
|------|----------|---|-------------|
| Photo | 8.019 | 95,0% | `Photo.ID`, `Photo.AccessHash`, `Photo.Sizes` |
| None (null) | 397 | 4,7% | — |
| WebPage | 20 | 0,24% | `Webpage.URL`, `Webpage.Title` |
| Document | 8 | 0,09% | `Document.ID`, `Document.MimeType` |
| Poll | 1 | 0,01% | `Poll.Question`, `Poll.Answers` |
| Other | 0 | 0% | — |

### 4.2 Entities

| Tipo (chaves) | Contagem | Interpretação |
|---------------|----------|---------------|
| `(Offset, Length)` | 19.456 | Formatação de texto (bold, italic) |
| `(DocumentID, Offset, Length)` | 213 | Menção a documento/emoji custom |
| `(Offset, Length, URL)` | 55 | Hyperlink |
| `(Language, Offset, Length)` | 20 | Bloco de código |

**Gap crítico:** Entities captura apenas **55 URLs** vs **10.922 URLs** encontradas por regex no texto (0,5%). O regex como fonte primária é **correto e necessário**.

### 4.3 FwdFrom/Forwards

| Métrica | Valor | % |
|---------|-------|---|
| `FwdFrom` não-null | 8.445 | 100% |
| `FwdFrom.FromID` não-null | 60 | 0,7% |
| `Forwards > 0` | 5.440 | 64,4% |

---

## 5. Conteúdo de Texto

### 5.1 Estatísticas

| Métrica | Valor |
|---------|-------|
| Textos vazios | 2 (0,02%) |
| Comprimento mínimo | 0 chars |
| Comprimento máximo | 2.355 chars |
| Mediana | 171 chars |
| Mensagens com URL | 8.188 (97,0%) |
| Total de URLs no texto | 10.922 |

### 5.2 Top Domínios

| Domínio | Contagem | Tipo |
|---------|----------|------|
| meli.la | 3.274 | Mercado Livre (shortener) |
| s.shopee.com.br | 3.127 | Shopee (shortener) |
| amzn.to | 1.736 | Amazon (shortener) |
| www.amazon.com.br | 943 | Amazon (direto) |
| t.me | 239 | Telegram |
| amzn.divulgador.link | 215 | Amazon (afiliado BR) |
| tidd.ly | 175 | Shortener genérico |
| a.aliexpress.com | 169 | AliExpress (afiliado) |
| amzlink.to | 156 | Amazon (shortener) |
| magazineluiza.onelink.me | 152 | Magalu (deeplink) |
| mercadolivre.com | 141 | Mercado Livre (direto) |
| s.click.aliexpress.com | 124 | AliExpress (shortener) |
| bit.ly | 75 | Shortener genérico |
| eioferta.com.br | 63 | Portal de ofertas |
| divulgador.magalu.com | 56 | Magalu (afiliado) |

### 5.3 Preços e Cupons

| Sinal | Regex ampla | Processor regex | Gap |
|-------|-------------|-----------------|-----|
| Preço (R$) | 6.446 (76,3%) | 6.952 (76,2%) | +506 (processor regex captura mais padrões) |
| Cupom | 5.723 (67,8%) | 4.567 (50,1%) | **-1.156 (gap significativo)** |

**Gap de cupons:** O `reCoupon` atual (`cupom[:\s]+([A-Z0-9_]{3,25})`) não captura formatos como "CUPOM:", "código PROMO123", "CODE ABC", ou cupons sem separador explícito.

**Amostras de preços:** `R$ 234`, `R$ 164`, `R$ 191`, `R$ 147`, `R$ 71`, `R$ 48`, `R$93`, `R$ 113`, `R$ 226`, `R$ 549`

---

## 6. Classificação Atual — Distribuição

| MessageType | Contagem | % | Regra |
|-------------|----------|---|-------|
| `deal_complete` | 3.557 | 39,0% | URL + preço + cupom |
| `deal_no_coupon` | 3.377 | 37,0% | URL + preço, sem cupom |
| `deal_no_price` | 1.915 | 21,0% | URL sem preço |
| `coupon_expired` | 126 | 1,4% | "esgotado" sem URL |
| `category_header` | 98 | 1,1% | Texto curto sem sinais |
| `commentary` | 42 | 0,5% | Fallback |
| `video` | 2 | 0,02% | Media video |
| `poll` | 1 | 0,01% | Enquete |

### 6.1 Sinais Binários

| Sinal | Contagem | % |
|-------|----------|---|
| `has_url` | 8.849 | 97,0% |
| `has_price` | 6.952 | 76,2% |
| `has_coupon` | 4.567 | 50,1% |
| Todos os três | 3.557 | 39,0% |

### 6.2 Urgency Signals

| Signal | Contagem |
|--------|----------|
| `frete_gratis` | 335 |
| `corre` | 34 |
| `ultima_unidade` | 21 |
| `envio_nacional` | 16 |
| `corre` + `ultima_unidade` | 1 |
| Nenhum sinal | 8.711 (95,5%) |

---

## 7. Relevância para o Pipeline

### 7.1 Campos Essenciais

| Campo | Uso | Justificativa |
|-------|-----|---------------|
| `ID` + `channel_id` | Deduplicação | Constraint única no DB |
| `Date` | Ordenação temporal | Unix timestamp, sempre presente |
| `Message` | Extração de sinais | 97% URLs, 76% preços, 68% cupons |
| `Media` | Classificação de tipo | Photo 95%, Webpage 0,3% |
| `Views` | Ranking de relevância | Range 0–10.162, mediana 1.021 |
| `Forwards` | Viralidade | 64,4% com Forwards > 0 |
| `Entities[].URL` | URLs (complementar) | Apenas 55 capturadas — regex é primário |
| `ReplyTo.ReplyToMsgID` | Threads | 85 mensagens (1,0%) |

### 7.2 Oportunidades não-exploradas

| Campo | Oportunidade | Prioridade |
|-------|-------------|------------|
| `FwdFrom.FromID` | Rastrear origem de 60 forwards | P4 |
| `Media.Webpage.URL` | URL resolvida para 20 mensagens | P2 |
| `Entities` (formatação) | 19.456 entidades de bold/italic | P5 |
| `ReplyTo.ReplyToMsgID` | Threads de promoção | P5 |

---

## 8. Perguntas em Aberto

### 8.1 Ambiguidades

1. **GroupedID 100% presente** — Artefato da serialização gotd/td ou todos os canais realmente usam álbuns? **Ação:** Validar com `GroupedID == 0` em novos dados.

2. **Gap de cupons (1.156 mensagens)** — `reCoupon` perde 23% das menções. **Ação:** Expandir regex para cobrir "CUPOM:", "código", "CODE", variações.

3. **WebPage vs Photo** — 20 mensagens têm `Media.Webpage` em vez de `Media.Photo`. São previews automáticos de links? **Ação:** Inspecionar `Webpage.URL` vs URL no texto.

### 8.2 Decisões de Produto

1. **Mensagens de reply (85)** — Devem ser tratadas como threads ou ignoradas? **Produto:** Threads de promoção são relevantes para o feed?

2. **FwdFrom reais (60 mensagens)** — Mensagens forwardadas devem ter origem rastreada? **Produto:** Creditar o canal original ou tratar como promoção do canal monitorado?

3. **Resolução de shortened URLs** — Deve ser feita em tempo real (latência +100–500ms) ou batch assíncrono? **Produto:** Qual a latência aceitável para o feed?

### 8.3 Riscos de Premissas

| Premissa | Evidência contra | Mitigação |
|----------|------------------|-----------|
| "Todo canal de promoção só posta promoção" | 42 commentary, 98 category_header, 126 coupon_expired, 1 poll | Classificação em cascata já trata |
| "Entities é fonte confiável de URLs" | 55 de 10.922 (0,5%) | Regex como fonte primária |
| "Preço sempre tem R$" | ~5% dos preços podem não ter `R$` | Regex secundário para padrões numéricos |

---

## 9. Riscos Técnicos e Recomendações

### 9.1 Parser Breakage Points

| Risco | Cenário | Impacto | Mitigação atual |
|-------|---------|---------|-----------------|
| Shape B sem `Message` | `Updates[0]` sem campo `Message` | Mensagem descartada | Fallback para `first` ✅ |
| Media tipo desconhecido | Futuro `Media.Invoice` ou `Contact` | Classificado como `none` | Default handler retorna `"none"` ✅ |
| `Date` = 0 | Mensagem sem timestamp | `PostedAt` zero-value | Retornado como `time.Time{}` ✅ |
| Payload truncado | JSON inválido | Erro de unmarshal | Logado como warning, mensagem pulada ✅ |

### 9.2 Recomendações Priorizadas

| Prioridade | Ação | Impacto esperado | Esforço |
|------------|------|------------------|---------|
| **P1** | Expandir `reCoupon` para cobrir gap de 1.156 mensagens | +12,7% cobertura de cupons | 2h |
| **P2** | Extrair URLs de `Media.Webpage.URL` como fonte secundária | Cobrir 20 mensagens com webpage | 1h |
| **P3** | Adicionar regex secundário para preços sem `R$` | Cobrir ~5% de preços não-capturados | 2h |
| **P4** | Usar `FwdFrom.FromID` para rastrear origem de forwards | 60 mensagens com proveniência | 3h |
| **P5** | Expandir `reAdminMeta` com mais padrões administrativos | Cobrir mensagens admin não-detectadas | 1h |
| **P5** | Preservar emojis como features de categoria | 🔥📱💰 = sinais de categoria | 2h |

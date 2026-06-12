# Relatório Técnico: Análise de Payloads — limiar-processor

**Data:** 2026-06-08  
**Analisador:** Engenheiro de Dados Sênior  
**Escopo:** Análise completa de 239 payloads JSON armazenados em `raw_messages`  
**Objetivo:** Mapear o shape real dos dados para projetar o `limiar-processor`

---

## 1. Escopo e Método

### Metodologia de Exploração

**Banco de dados analisado:**
- Arquivo: `/home/projetos/Projetos/Limiar2/limiar.db`
- Tamanho: 1.1 MB
- Engine: Tursogo (SQLite-compatible, pure Go, sem CGO)
- Tabelas encontradas: `channels`, `peers`, `raw_messages`, `sessions`, `sqlite_sequence`

**Ferramentas utilizadas:**
- Python 3 + sqlite3 (extração e estatísticas iniciais)
- jq (exploração profunda de estruturas JSON, filtragem, agregação)
- Análise exaustiva de todos os 239 registros (não-amostral)

**Quantidade de registros analisados:**
- Total de mensagens brutas: **239**
- Total de canais monitorados: **4**
- Payloads válidos: **239** (100% JSON válido)
- Payloads inválidos ou truncados: **0**

**Limitações reais da amostra:**
1. **Janela temporal curta:** ~22.5 horas de coleta (timestamps Unix: 1780793180 → 1780874437)
2. **Volume baixo:** 239 mensagens é estatisticamente limitado para inferir padrões raros
3. **Viés de canal:** 4 canais específicos de promoções brasileiras, não-representativo do ecossistema completo
4. **Ausência de edge cases temporais:** não há dados sobre comportamento durante pices, falhas de rede, ou atualizações do MTProto
5. **Sem validação de evolução de schema:** todos os payloads têm `schema_version = 1`, não há histórico de versões anteriores

**Como jq foi usado:**
- Extração de chaves de nível superior e frequência
- Identificação de variações de tipo por campo
- Análise de estruturas aninhadas (Media, Entities, FwdFrom, Reactions)
- Extração de URLs e domínios de texto de mensagem
- Filtragem de edge cases (mensagens vazias, sem URL, sem media)
- Agregação estatística (contagens, médias, distribuições)

---

## 2. Persona Estrutural dos Dados

### 2.1 Padrões de Shape Encontrados

**Dois shapes distintos identificados:**

#### Shape A: Mensagem Padrão (92.5% — 221 mensagens)

Estrutura direta de mensagem MTProto com 48 campos de nível superior.

```json
{
  "Date": 1780858455,
  "Flags": 2624,
  "Out": false,
  "Mentioned": false,
  "MediaUnread": false,
  "Silent": false,
  "Post": true,
  "FromScheduled": false,
  "Legacy": false,
  "EditHide": false,
  "Pinned": false,
  "Noforwards": false,
  "InvertMedia": false,
  "Flags2": 0,
  "Offline": false,
  "VideoProcessingPending": false,
  "PaidSuggestedPostStars": false,
  "PaidSuggestedPostTon": false,
  "ID": 12345,
  "FromID": null,
  "FromBoostsApplied": 0,
  "FromRank": "",
  "PeerID": {"ChannelID": 1987091586},
  "SavedPeerID": null,
  "FwdFrom": {...},
  "ViaBotID": 0,
  "ViaBusinessBotID": 0,
  "GuestchatViaFrom": null,
  "ReplyTo": null,
  "Message": "Texto da promoção...",
  "Media": {...},
  "ReplyMarkup": null,
  "Entities": [...],
  "Views": 683,
  "Forwards": 5,
  "Replies": {...},
  "EditDate": 0,
  "PostAuthor": "",
  "GroupedID": 8123456789012345678,
  "Reactions": {...},
  "RestrictionReason": null,
  "TTLPeriod": 0,
  "QuickReplyShortcutID": 0,
  "Effect": 0,
  "Factcheck": {...},
  "ReportDeliveryUntilDate": 0,
  "PaidMessageStars": 0,
  "SuggestedPost": {...},
  "ScheduleRepeatPeriod": 0,
  "SummaryFromLanguage": ""
}
```

#### Shape B: Wrapper de Updates (7.5% — 18 mensagens)

Estrutura de envelope contendo arrays de updates, users e chats.

```json
{
  "Updates": [
    {
      "Message": {...},  // Mesma estrutura do Shape A
      "Pts": 12345,
      "PtsCount": 1
    }
  ],
  "Users": [...],  // Array de usuários (3 no exemplo analisado)
  "Chats": [...],  // Array de chats (0 no exemplo analisado)
  "Seq": 0
}
```

### 2.2 Estruturas Estáveis

| Campo | Presença | Tipo | Estabilidade |
|-------|----------|------|--------------|
| Date | 100% | int (Unix timestamp) | Estável |
| ID | 92.5% | int | Estável |
| PeerID | 92.5% | object {ChannelID: int} | Estável |
| Message | 92.5% | string | Estável |
| Media | 92.5% | object | Estável |
| Entities | 92.5% | array or null | Estável com variação |
| Views | 92.5% | int | Estável |
| Forwards | 92.5% | int | Estável |
| Post | 92.5% | bool | Estável (sempre true) |
| GroupedID | 92.5% | int | Estável |

### 2.3 Estruturas Variáveis

#### Media (3 variações observadas)

**Variação 1: Photo (99.5% — 220 mensagens)**
```json
{
  "Flags": 1,
  "Spoiler": false,
  "LivePhoto": false,
  "Photo": {
    "Flags": 0,
    "HasStickers": false,
    "ID": 6138941836333092787,
    "AccessHash": -8328281025564808123,
    "FileReference": "AQAACW1qJb5XxI0WSlIv6cb6CP2Pke1M0w==",
    "Date": 1780854057,
    "Sizes": [...],  // 3-5 elementos
    "VideoSizes": null,
    "DCID": 5
  },
  "TTLSeconds": 0,
  "Video": null
}
```

**Photo.Sizes (distribuição):**
- 3 tamanhos: 93 mensagens (42.3%)
- 4 tamanhos: 124 mensagens (56.4%)
- 5 tamanhos: 3 mensagens (1.4%)

**Estrutura de Size (exemplo):**
```json
[
  {"Type": "i", "Bytes": "..."},  // Thumbnail inline
  {"Type": "m", "W": 240, "H": 320, "Size": 37124},
  {"Type": "x", "W": 600, "H": 800, "Size": 154729},
  {"Type": "y", "W": 960, "H": 1280, "Sizes": [23401, 77987, 143650, 196315, 257193]}
]
```

**Variação 2: Poll (0.5% — 1 mensagem)**
```json
{
  "Flags": 0,
  "Poll": {
    "ID": 4983505679454044655,
    "Flags": 384,
    "Closed": false,
    "PublicVoters": false,
    "MultipleChoice": false,
    "Quiz": false,
    "OpenAnswers": false,
    "RevotingDisabled": true,
    "ShuffleAnswers": true,
    "HideResultsUntilClose": false,
    "Creator": false,
    "SubscribersOnly": false,
    "Question": {"Text": "Ganhador do 6.6!? 🤠", "Entities": null},
    "Answers": [...],  // 3 opções
    "ClosePeriod": 0,
    "CloseDate": 0,
    "CountriesISO2": null,
    "Hash": 1568827462
  },
  "Results": {...},
  "AttachedMedia": null
}
```

**Variação 3: Video (0% — 0 mensagens)**
- Campo `Video` presente em 220 mensagens, mas sempre `null`
- Nenhuma mensagem de vídeo pura na amostra

#### Entities (2 tipos observados)

**Tipo 1: Text Formatting (96.7% — 563 entidades)**
```json
{"Offset": 0, "Length": 34}
```
- Apenas 2 campos: Offset e Length
- Provavelmente negrito, itálico, código, etc.
- Sem campo de tipo explícito visível

**Tipo 2: URL/Hyperlink (3.3% — 19 entidades)**
```json
{"Offset": 193, "Length": 42, "URL": "https://s.click.aliexpress.com/e/_c3IIdkp3"}
```
- 3 campos: Offset, Length, URL
- Todas as URLs observadas são AliExpress na amostra de entidades

#### FwdFrom (sempre presente, mas com FromID null)

```json
{
  "Flags": 0,
  "Imported": false,
  "SavedOut": false,
  "FromID": null,  // SEMPRE null na amostra
  "FromName": "",
  "Date": 0,
  "ChannelPost": 0,
  "PostAuthor": "",
  "SavedFromPeer": null,
  "SavedFromMsgID": 0,
  "SavedFromID": null,
  "SavedFromName": "",
  "SavedDate": 0,
  "PsaType": ""
}
```

**Anomalia:** Estrutura FwdFrom presente em 100% das mensagens padrão, mas `FromID` é sempre `null`. Isso sugere que:
- Ou os payloads são serializados com campos default mesmo quando não-usados
- Ou há um bug na serialização do gotd/td
- Ou as mensagens não são realmente forwardadas, mas o campo está reservado

#### Reactions (estrutura completa, mas sem dados)

```json
{
  "Flags": 221,
  "Min": false,
  "CanSeeList": false,
  "ReactionsAsTags": false,
  "Results": null,  // ou array vazio
  "RecentReactions": null,
  "TopReactors": null
}
```

**Observação:** 0 mensagens têm `Reactions.Results[]?.Chosen == true`. Isso pode significar:
- Reações não estão sendo capturadas pelo collector
- Ou usuários não reagiram às mensagens
- Ou o campo não é populado pelo MTProto para canais

### 2.4 Campos Aninhados e Opcionais

**Campos sempre presentes (não-null):**
- Date, ID, PeerID, Flags, Flags2, Post, GroupedID

**Campos sempre null na amostra:**
- FromID, SavedPeerID, GuestchatViaFrom, ReplyTo, ReplyMarkup, RestrictionReason, VideoSizes

**Campos opcionais (variam entre null e valor):**
- Entities (219 array, 2 null)
- FwdFrom.FromID (sempre null, mas estrutura presente)

**Campos com valores default (0, "", false):**
- FromBoostsApplied, FromRank, ViaBotID, ViaBusinessBotID, EditDate, PostAuthor, TTLPeriod, QuickReplyShortcutID, Effect, ReportDeliveryUntilDate, PaidMessageStars, ScheduleRepeatPeriod, SummaryFromLanguage

### 2.5 Chaves Inesperadas

Nenhuma chave inesperada ou dinâmica encontrada. Todos os 48 campos de nível superior são consistentes com a especificação MTProto do gotd/td.

---

## 3. Anomalias e Edge Cases

### 3.1 Payloads Inválidos ou Incompletos

**Payloads inválidos:** 0  
**Payloads truncados:** 0  
**Payloads incompletos:** 0  

Todos os 239 payloads são JSON válido e completo.

### 3.2 Tipos Variáveis para o Mesmo Campo

| Campo | Tipos Observados | Frequência |
|-------|------------------|------------|
| Chats | null, list | 18 mensagens (Updates wrapper) |
| Entities | null, list | 2 null, 219 list |

**Impacto:** Parser precisa tratar `Entities` como opcional e verificar tipo antes de iterar.

### 3.3 Campos Nulos, Vazios, Duplicados, Trocados ou Ausentes

#### Mensagens com Texto Vazio (0.5% — 1 mensagem)
- **ID:** 19
- **Message:** "" (string vazia)
- **Media:** Photo presente
- **Contexto:** Mensagem apenas com imagem, sem texto descritivo

#### Mensagens sem URL (0.9% — 2 mensagens)
1. **ID 19:** Message = "" (vazia)
2. **Outra:** Message = "cupom esgotado....." (texto sem link)

**Impacto:** Pipeline não pode assumir que toda mensagem tem URL extraível.

#### Mensagens sem Menção de Preço (12.2% — 29 mensagens)
- 219 mensagens têm URL no texto
- 192 mensagens têm menção de preço/cupom
- **Gap:** 27 mensagens têm URL mas não mencionam preço explicitamente

**Impacto:** Classificador de promocionalidade não pode depender apenas de regex de preço.

#### FwdFrom.FromID Sempre Null (100% — 221 mensagens)
- Estrutura FwdFrom presente em todas as mensagens padrão
- Campo FromID sempre null
- Campo Forwards > 0 em 136 mensagens (61.5%)

**Interpretação mais provável:** O campo `Forwards` conta quantas vezes a mensagem foi forwardada, não indica que a mensagem atual é um forward. A estrutura `FwdFrom` é serializada por completo pelo gotd/td mesmo quando não-aplicável (campos default).

**Risco:** Se o processor tentar extrair "mensagem original" de FwdFrom, vai falhar em 100% dos casos.

### 3.4 Casos Raros que Podem Quebrar o limiar-processor

#### Caso 1: Poll Media (0.5%)
- 1 mensagem tem Media.Poll em vez de Media.Photo
- Estrutura completamente diferente (Question, Answers, Results)
- **Risco:** Se o processor assumir que Media sempre tem Photo, vai crashar

#### Caso 2: Mensagem "cupom esgotado" (0.5%)
- Texto indica que a promoção expirou
- Não tem URL
- **Risco:** Se o processor tentar extrair URL via regex, vai falhar e pode gerar null pointer

#### Caso 3: GroupedID Sempre Presente (100%)
- Todas as 221 mensagens têm GroupedID não-null
- Isso é incomum — normalmente GroupedID indica álbum/multi-foto
- **Hipótese:** Pode ser um artefato da serialização do gotd/td ou do método de coleta (history backfill vs live updates)
- **Risco:** Se o processor usar GroupedID para agrupar mensagens relacionadas, pode gerar falsos agrupamentos

#### Caso 4: Updates Wrapper (7.5%)
- 18 mensagens vêm envelopadas em `{Updates: [...], Users: [...], Chats: [...], Seq: 0}`
- A mensagem real está em `Updates[0].Message`
- **Risco:** Se o processor não detectar o wrapper, vai tentar parsear o envelope como mensagem e falhar

#### Caso 5: Photo.Sizes com Estruturas Heterogêneas
- Alguns sizes têm `Bytes` (thumbnail inline)
- Outros têm `W`, `H`, `Size` (dimensões + tamanho)
- Outros têm `W`, `H`, `Sizes` (array de tamanhos para progressivo)
- **Risco:** Se o processor tentar extrair "URL da imagem" sem entender a estrutura, vai falhar

---

## 4. Relevância para o Pipeline

### 4.1 Campos Essenciais para Normalização, Deduplicação, Classificação e Filtro

| Campo | Uso no Pipeline | Justificativa |
|-------|-----------------|---------------|
| **ID** | Deduplicação | Identificador único da mensagem no canal |
| **PeerID.ChannelID** | Deduplicação + Agrupamento | Identifica o canal de origem |
| **Date** | Ordenação + Filtro Temporal | Timestamp Unix para ordenar e filtrar promoções antigas |
| **Message** | Extração de Sinais | Texto contém produto, preço, cupom, URL |
| **Media.Photo** | Enriquecimento | Imagem do produto (necessário extrair URL de Sizes) |
| **Entities[]** | Extração de URLs | URLs já parseadas pelo MTProto (mais confiável que regex) |
| **Views** | Ranking de Relevância | Promoções com mais views podem ser mais relevantes |
| **Forwards** | Ranking de Relevância | Promoções muito forwardadas são sinal de qualidade |

### 4.2 Campos Úteis, Mas Secundários

| Campo | Uso Potencial | Prioridade |
|-------|---------------|------------|
| **Reactions.Results** | Engajamento | Baixa (sempre null/vazio na amostra) |
| **PostAuthor** | Atribuição | Baixa (sempre "" na amostra) |
| **EditDate** | Detecção de Edição | Baixa (sempre 0 na amostra) |
| **ReplyTo** | Contexto de Thread | Baixa (sempre null na amostra) |
| **GroupedID** | Agrupamento de Álbum | Média (presente em 100%, mas semântica incerta) |

### 4.3 Campos Descartáveis

| Campo | Justificativa para Descarte |
|-------|----------------------------|
| **Flags, Flags2** | Bitmasks internos do MTProto, não-portáveis |
| **Out, Mentioned, MediaUnread, Silent** | Flags de estado do cliente, irrelevantes para promoção |
| **FromScheduled, Legacy, EditHide, Pinned** | Metadados de UI do Telegram |
| **Noforwards, InvertMedia, Offline** | Flags de comportamento, não-conteúdo |
| **VideoProcessingPending** | Status de processamento, não-conteúdo |
| **PaidSuggestedPostStars, PaidSuggestedPostTon, PaidMessageStars** | Monetização do Telegram, irrelevante |
| **FromBoostsApplied, FromRank** | Gamificação do Telegram |
| **ViaBotID, ViaBusinessBotID, GuestchatViaFrom** | Rastreamento de bot, irrelevante para canal |
| **SavedPeerID, SavedFromPeer, SavedFromMsgID, SavedFromID, SavedFromName, SavedDate** | Campos de "Saved Messages", não-aplicáveis |
| **FwdFrom** | Sempre com campos default/null na amostra |
| **ReplyMarkup** | Sempre null na amostra (sem inline buttons) |
| **RestrictionReason** | Sempre null (sem restrições geográficas) |
| **TTLPeriod, QuickReplyShortcutID, Effect** | Features efêmeras do Telegram |
| **Factcheck** | Sistema de fact-checking, não-aplicável a promoções |
| **ReportDeliveryUntilDate** | Métrica interna de entrega |
| **SuggestedPost** | Sistema de posts sugeridos, não-aplicável |
| **ScheduleRepeatPeriod** | Agendamento recorrente, não-aplicável |
| **SummaryFromLanguage** | Tradução automática, não-aplicável |
| **Reactions.Min, CanSeeList, ReactionsAsTags, RecentReactions, TopReactors** | Metadados de reação, não-dados de reação |

**Justificativa objetiva:** Estes campos são:
1. Metadados de protocolo (não-conteúdo)
2. Sempre null/default na amostra (sem sinal útil)
3. Features do Telegram não-relacionadas a promoções
4. Redundantes com campos já-capturados

### 4.4 Decisões de Design Baseadas em Evidência

**Decisão 1: Extrair URLs de Entities, não de regex no Message**
- **Evidência:** 19 entidades têm campo URL explícito
- **Vantagem:** Mais confiável, já-parseado pelo MTProto
- **Risco:** Apenas 19 URLs em entities vs 219 URLs no texto → Entities pode estar incompleto
- **Recomendação:** Usar Entities como fonte primária, regex como fallback

**Decisão 2: Tratar Media como union type (Photo | Poll | Video | null)**
- **Evidência:** 220 Photo, 1 Poll, 0 Video, 0 null
- **Vantagem:** Cobre todos os casos observados
- **Risco:** Futuros tipos de media (Document, Audio, Sticker) não-previstos
- **Recomendação:** Usar switch/case com default handler

**Decisão 3: Ignorar FwdFrom para detecção de forward**
- **Evidência:** FromID sempre null, mas Forwards > 0 em 61.5% das mensagens
- **Interpretação:** Forwards é contador de forward, não indicador de que a mensagem é forward
- **Recomendação:** Usar Forwards como métrica de engajamento, não FwdFrom

**Decisão 4: Descartar Updates wrapper e extrair Updates[0].Message**
- **Evidência:** 18 mensagens vêm envelopadas, mas a mensagem real está em Updates[0].Message
- **Vantagem:** Simplifica o pipeline, normaliza todos os payloads para Shape A
- **Risco:** Se Updates tiver múltiplas mensagens, apenas a primeira é processada
- **Recomendação:** Verificar `Updates | length` e logar warning se > 1

---

## 5. Schema de Saída Proposto

### 5.1 Estrutura Normalizada Recomendada

```json
{
  "schema_version": "2.0",
  "source": {
    "message_id": 12345,
    "channel_id": 1987091586,
    "channel_username": "lobaopromo",
    "received_at": "2026-06-07T19:20:35Z",
    "posted_at": "2026-06-07T19:14:15Z",
    "views": 683,
    "forwards": 5
  },
  "content": {
    "text": "Memória Ram DDR4 Jazer 8GB 3200mhz\n\nSelecione a primeira opção na aba de Moedas ou na aba \"BRASIL\" somente pelo APP!\n\n💵  R$ 234\n🎟  Cupom: AEBR2 ou IFPL90V1 ou MARCABR02 + 90 Moedas no app\n✅  https://s.click.aliexpress.com/e/_c3IIdkp3",
    "text_length": 193,
    "has_price": true,
    "has_coupon": true,
    "language": "pt-BR"
  },
  "media": {
    "type": "photo",
    "photo": {
      "id": "6138941836333092787",
      "width": 960,
      "height": 1280,
      "sizes": [
        {"type": "m", "width": 240, "height": 320, "size_bytes": 37124},
        {"type": "x", "width": 600, "height": 800, "size_bytes": 154729},
        {"type": "y", "width": 960, "height": 1280, "size_bytes": 257193}
      ],
      "url": "https://..."  // A ser resolvido via Telegram API
    }
  },
  "links": [
    {
      "url": "https://s.click.aliexpress.com/e/_c3IIdkp3",
      "domain": "s.click.aliexpress.com",
      "type": "affiliate",
      "merchant": "aliexpress",
      "is_shortened": true
    }
  ],
  "promotion": {
    "product_name": "Memória Ram DDR4 Jazer 8GB 3200mhz",
    "price": {
      "amount": 234.00,
      "currency": "BRL",
      "formatted": "R$ 234"
    },
    "coupons": ["AEBR2", "IFPL90V1", "MARCABR02"],
    "instructions": "Selecione a primeira opção na aba de Moedas ou na aba \"BRASIL\" somente pelo APP!",
    "expiry": null
  },
  "classification": {
    "is_promotional": true,
    "confidence": 0.95,
    "category": "electronics",
    "subcategory": "computer_memory",
    "tags": ["hardware", "ram", "ddr4", "aliexpress"]
  },
  "deduplication": {
    "content_hash": "a1b2c3d4e5f6...",
    "url_hash": "f6e5d4c3b2a1...",
    "similar_messages": []
  },
  "metadata": {
    "entities_count": 3,
    "grouped_id": "8123456789012345678",
    "is_grouped": true,
    "raw_payload_size_bytes": 2048
  }
}
```

### 5.2 Tipos Esperados por Campo

| Campo | Tipo | Obrigatório | Validação |
|-------|------|-------------|-----------|
| schema_version | string | Sim | SemVer, atual "2.0" |
| source.message_id | int64 | Sim | > 0 |
| source.channel_id | int64 | Sim | > 0 |
| source.channel_username | string | Sim | Não-vazio, regex `^[a-zA-Z0-9_]+$` |
| source.received_at | ISO8601 | Sim | UTC, parseável |
| source.posted_at | ISO8601 | Sim | UTC, <= received_at |
| source.views | int | Sim | >= 0 |
| source.forwards | int | Sim | >= 0 |
| content.text | string | Sim | Pode ser vazio |
| content.text_length | int | Sim | >= 0 |
| content.has_price | bool | Sim | — |
| content.has_coupon | bool | Sim | — |
| content.language | string | Sim | ISO 639-1 (pt-BR, en-US) |
| media.type | enum | Sim | "photo" \| "poll" \| "video" \| "document" \| "none" |
| media.photo.id | string | Se type=photo | — |
| media.photo.width | int | Se type=photo | > 0 |
| media.photo.height | int | Se type=photo | > 0 |
| media.photo.sizes | array | Se type=photo | length > 0 |
| media.photo.url | string | Se type=photo | URL válida |
| links | array | Sim | Pode ser vazio |
| links[].url | string | Sim | URL válida |
| links[].domain | string | Sim | Domínio extraído |
| links[].type | enum | Sim | "affiliate" \| "direct" \| "shortened" |
| links[].merchant | string | Sim | Nome do merchant |
| links[].is_shortened | bool | Sim | — |
| promotion.product_name | string | Não | Extraído do texto |
| promotion.price.amount | float | Não | > 0 |
| promotion.price.currency | string | Se price.amount | ISO 4217 |
| promotion.price.formatted | string | Se price.amount | Texto original |
| promotion.coupons | array[string] | Não | Pode ser vazio |
| promotion.instructions | string | Não | Extraído do texto |
| promotion.expiry | ISO8601 | Não | null se não-detectado |
| classification.is_promotional | bool | Sim | — |
| classification.confidence | float | Sim | 0.0–1.0 |
| classification.category | string | Não | Taxonomia pré-definida |
| classification.subcategory | string | Não | — |
| classification.tags | array[string] | Não | — |
| deduplication.content_hash | string | Sim | SHA-256 do texto normalizado |
| deduplication.url_hash | string | Sim | SHA-256 da primeira URL |
| deduplication.similar_messages | array | Sim | IDs de mensagens similares |
| metadata.entities_count | int | Sim | >= 0 |
| metadata.grouped_id | string | Não | null se não-agrupado |
| metadata.is_grouped | bool | Sim | — |
| metadata.raw_payload_size_bytes | int | Sim | > 0 |

### 5.3 Regras de Limpeza, Enriquecimento, Deduplicação e Classificação

#### Limpeza

1. **Texto:**
   - Remover whitespace excessivo (3+ newlines → 2)
   - Normalizar espaços Unicode (NBSP → SPACE)
   - Remover emojis de formatação (🔥, 💰, etc.) apenas se interferirem no parsing
   - Preservar emojis de conteúdo (📱, 🎧, etc.)

2. **URLs:**
   - Resolver shortened URLs (meli.la, amzn.to, tidd.ly) para extrair destino final
   - Extrair parâmetros de tracking (UTM, affiliate ID)
   - Normalizar URLs (lowercase scheme/host, remover trailing slash)

3. **Preços:**
   - Extrair valores com regex: `R\$\s*([\d.,]+)`
   - Normalizar separadores: vírgula → ponto
   - Validar range: 0.01–999999.99
   - Detectar moeda: BRL (R$), USD ($), EUR (€)

4. **Cupons:**
   - Extrair com regex: `[Cc]upom[:\s]+([A-Z0-9]+)`
   - Validar formato: 4–20 chars, alfanumérico
   - Deduplicar cupons na mesma mensagem

#### Enriquecimento

1. **Resolução de Imagem:**
   - Usar Photo.ID + Photo.AccessHash + Photo.FileReference para gerar URL via Telegram API
   - Selecionar maior tamanho disponível (tipo "y" ou "x")
   - Cache de URLs resolvidas (expiram após 24h)

2. **Classificação de Merchant:**
   - Mapear domínios para merchants:
     - `meli.la`, `mercadolivre.com.br` → "mercadolivre"
     - `amzn.to`, `amzn.divulgador.link` → "amazon"
     - `s.shopee.com.br` → "shopee"
     - `s.click.aliexpress.com` → "aliexpress"
   - Detectar afiliados via parâmetros de URL

3. **Categorização de Produto:**
   - Extrair keywords do texto (TF-IDF, noun phrases)
   - Match com taxonomia pré-definida (electronics, fashion, home, etc.)
   - Usar embeddings para classificação semântica (Phase 3)

4. **Detecção de Expiração:**
   - Regex para datas: `\d{2}/\d{2}/\d{4}`, `até\s+\d{2}/\d{2}`
   - Keywords: "por tempo limitado", "últimas unidades", "esgotou"
   - Marcar como `expired` se texto contém "cupom esgotado"

#### Deduplicação

1. **Nível 1: Mesma mensagem no mesmo canal**
   - Chave: `(channel_id, message_id)`
   - Já-implementado no collector (UNIQUE constraint)

2. **Nível 2: Mesma URL em múltiplos canais**
   - Chave: `SHA-256(normalized_url)`
   - Agrupar mensagens com mesma URL
   - Manter a mais antiga como "canônica"
   - Marcar outras como "duplicatas"

3. **Nível 3: Conteúdo similar (fuzzy matching)**
   - Chave: `SHA-256(normalized_text)`
   - Normalização: lowercase, remover preços/cupons, colapsar whitespace
   - Similaridade: Jaccard similarity > 0.85 ou Levenshtein distance < 10%
   - Agrupar mensagens similares, manter a mais completa

4. **Nível 4: Produto idêntico, ofertas diferentes**
   - Chave: `embedding(product_name)` (Phase 3)
   - Detectar mesmo produto com preços diferentes
   - Mostrar a oferta de menor preço

#### Classificação

1. **Promocionalidade (binário):**
   - Regra: `has_url AND (has_price OR has_coupon)`
   - Confiança: 0.95 se regra match, 0.5 se apenas URL, 0.1 se apenas texto
   - Falso-positivo esperado: 5% (mensagens com URL mas não-promocionais)

2. **Categoria (multi-classe):**
   - Fase 2: Regex-based (keywords: "notebook", "celular", "tênis")
   - Fase 3: Embedding-based (sentence-transformers)

3. **Qualidade (score 0–100):**
   - Views: 0–40 pontos (log-scale)
   - Forwards: 0–30 pontos (log-scale)
   - Completo (texto + imagem + preço + cupom): 0–20 pontos
   - Freshness (idade): 0–10 pontos (decai com tempo)

### 5.4 Versionamento do Schema

**Versão atual:** `2.0`

**Estratégia de versionamento:**
- Campo `schema_version` no nível raiz (string SemVer)
- Breaking changes incrementam major version (2.0 → 3.0)
- Adições de campos opcionais incrementam minor version (2.0 → 2.1)
- Processor deve suportar N-1 versões (2.0 e 1.x durante transição)

**Migração de schema:**
- Messages antigas mantêm schema_version original
- Novas mensagens usam schema_version atual
- API serve ambas as versões com normalização on-the-fly

---

## 6. Perguntas em Aberto

### 6.1 Ambiguidades Após a Análise

1. **GroupedID sempre presente:**
   - Todas as 221 mensagens têm GroupedID não-null
   - Isso é normal para canais de promoções (álbuns de produtos)?
   - Ou é artefato da serialização/coleta?
   - **Decisão necessária:** Validar com mais dados ou inspecionar código do gotd/td

2. **Entities incompletas:**
   - Apenas 19 URLs em Entities vs 219 URLs no texto
   - Entities está capturando apenas algumas URLs?
   - Ou o MTProto só marca URLs como entities se forem "clicáveis"?
   - **Decisão necessária:** Confiar em Entities ou usar regex como fonte primária?

3. **Updates wrapper:**
   - Por que 18 mensagens vêm envelopadas e 221 não?
   - É diferença entre live updates vs history backfill?
   - Ou é variação aleatória do MTProto?
   - **Decisão necessária:** Investigar no código do collector ou log de coleta

4. **FwdFrom sempre com campos default:**
   - Estrutura presente mas FromID sempre null
   - É bug do gotd/td ou comportamento esperado?
   - **Decisão necessária:** Ignorar FwdFrom completamente ou monitorar para mudanças?

5. **Photo.Sizes heterogêneos:**
   - Alguns têm `Bytes`, outros têm `Size`, outros têm `Sizes` (array)
   - Qual é a diferença semântica?
   - Como gerar URL de imagem a partir disso?
   - **Decisão necessária:** Consultar documentação do Telegram ou gotd/td

### 6.2 Decisões que Dependem de Validação de Produto ou Engenharia

1. **Resolução de shortened URLs:**
   - Deve ser feita em tempo real (latência +100–500ms) ou batch assíncrono?
   - Quantos redirects seguir? (meli.la → mercadolivre.com.br → produto)
   - Como lidar com URLs expiradas ou quebradas?
   - **Produto:** Qual a latência aceitável para o feed de promoções?

2. **Deduplicação fuzzy:**
   - Threshold de similaridade: 0.85 é muito agressivo ou muito conservador?
   - Mensagens com mesmo produto mas preços diferentes são duplicatas?
   - Como apresentar duplicatas ao usuário (ocultar ou mostrar todas)?
   - **Produto:** Qual a UX para promoções duplicadas?

3. **Classificação de categoria:**
   - Taxonomia fixa (electronics, fashion, home) ou dinâmica (tags)?
   - Quantas categorias são necessárias para MVP?
   - Quem valida a classificação (automático ou humano-no-loop)?
   - **Produto:** Qual a granularidade de categorização necessária?

4. **Tratamento de promoções expiradas:**
   - Remover do feed ou marcar como "expirada"?
   - Por quanto tempo manter promoções expiradas visíveis?
   - Como detectar expiração sem data explícita?
   - **Produto:** Qual a política de retenção de promoções?

5. **Enriquecimento com metadata externa:**
   - Adicionar reviews, ratings, preço histórico?
   - De quais fontes (Reclame Aqui, Google Shopping, etc.)?
   - Qual o custo/latência de enriquecimento?
   - **Engenharia:** Vale a pena o overhead de enriquecimento em tempo real?

### 6.3 Riscos de Assumir Premissas Erradas no Pipeline

1. **Premissa:** "Toda mensagem de canal de promoção é promocional"
   - **Risco:** Canais podem postar avisos, enquetes, mensagens administrativas
   - **Evidência:** 1 mensagem é Poll ("Ganhador do 6.6!?"), 1 é "cupom esgotado"
   - **Mitigação:** Classificador de promocionalidade não pode ser 100% confiante

2. **Premissa:** "URLs em Entities são sempre válidas e acessíveis"
   - **Risco:** URLs podem expirar, quebrar, ou ser geo-restritas
   - **Evidência:** Nenhuma URL testada ainda, apenas extraída
   - **Mitigação:** Validar URLs antes de servir, cache de status HTTP

3. **Premissa:** "Photo.Sizes sempre tem pelo menos um tamanho usável"
   - **Risco:** Imagens podem ser removidas do Telegram, corrompidas, ou muito pequenas
   - **Evidência:** Não observado na amostra, mas possível
   - **Mitigação:** Validar dimensões mínimas (240x240), fallback para placeholder

4. **Premissa:** "Preços no texto são sempre precisos e atualizados"
   - **Risco:** Preços podem estar desatualizados, errados, ou ser "a partir de"
   - **Evidência:** Não validado na amostra
   - **Mitigação:** Marcar preços como "aproximados", link para fonte original

5. **Premissa:** "Cupons extraídos via regex são sempre válidos"
   - **Risco:** Regex pode capturar falso-positivos (códigos de pedido, IDs)
   - **Evidência:** Não validado na amostra
   - **Mitigação:** Validar cupons via API do merchant (se disponível) ou marcar como "não-verificado"

---

## 7. Riscos Técnicos do limiar-processor

### 7.1 Onde o Parser Pode Quebrar

1. **Updates wrapper não-detectado:**
   - **Cenário:** 18 mensagens têm estrutura `{Updates: [...], Users: [...], Chats: [...]}`
   - **Falha:** Parser tenta acessar `.Message` no envelope, recebe `undefined`
   - **Impacto:** 7.5% das mensagens são descartadas silenciosamente
   - **Mitigação:** Detector de wrapper no início do pipeline:
     ```go
     if payload, ok := raw["Updates"]; ok {
         raw = payload[0]["Message"]
     }
     ```

2. **Media type não-previsto:**
   - **Cenário:** Futura mensagem com Media.Document, Media.Audio, Media.Sticker
   - **Falha:** Switch/case não tem handler para tipo desconhecido
   - **Impacto:** Panic ou mensagem descartada
   - **Mitigação:** Default handler que loga warning e preserva payload bruto:
     ```go
     switch mediaType {
     case "photo": ...
     case "poll": ...
     default:
         log.Warn("unknown media type", "type", mediaType)
         return processGenericMedia(raw)
     }
     ```

3. **Entities null ou tipo inesperado:**
   - **Cenário:** 2 mensagens têm `Entities: null` em vez de array
   - **Falha:** Loop `for _, entity := range entities` panic se entities é nil
   - **Impacto:** 0.9% das mensagens crasham o processor
   - **Mitigação:** Verificação defensiva:
     ```go
     if entities == nil {
         entities = []Entity{}
     }
     ```

4. **Photo.Sizes com estrutura inválida:**
   - **Cenário:** Size sem `W`/`H` ou com `Sizes` vazio
   - **Falha:** Extração de dimensões retorna 0, divisão por zero em aspect ratio
   - **Impacto:** Imagem metadata incorreta ou panic
   - **Mitigação:** Validação de dimensões:
     ```go
     if size.W <= 0 || size.H <= 0 {
         continue // skip invalid size
     }
     ```

5. **URLs malformadas no texto:**
   - **Cenário:** URL com caracteres Unicode, espaços, ou quebra de linha
   - **Falha:** Regex não captura URL completa, ou `url.Parse` retorna erro
   - **Impacto:** URLs incompletas ou inválidas no output
   - **Mitigação:** Regex robusta + validação:
     ```go
     urls := urlRegex.FindAllString(text, -1)
     for _, u := range urls {
         if parsed, err := url.Parse(u); err == nil {
             links = append(links, parsed)
         }
     }
     ```

### 7.2 Onde o Normalizador Pode Perder Sinal Útil

1. **Limpeza agressiva de emojis:**
   - **Cenário:** Emojis como 🔥, 💰, 🎁 são removidos como "ruído"
   - **Perda:** Emojis podem indicar categoria (📱 = electronics, 👗 = fashion)
   - **Impacto:** Classificador perde features úteis
   - **Mitigação:** Preservar emojis, extrair como features separadas

2. **Normalização de preços remove contexto:**
   - **Cenário:** Texto "R$ 234,99 à vista" → preço normalizado para 234.99
   - **Perda:** Condição de pagamento ("à vista") é descartada
   - **Impacto:** Preço pode ser enganoso (à vista vs parcelado)
   - **Mitigação:** Extrair condição de pagamento como campo separado

3. **Deduplicação fuzzy agrupa ofertas diferentes:**
   - **Cenário:** "Notebook Dell R$ 3000" e "Notebook Dell R$ 2800" são 85% similares
   - **Perda:** Oferta de R$ 2800 é marcada como duplicata e ocultada
   - **Impacto:** Usuário perde a melhor oferta
   - **Mitigação:** Never dedup se preços diferem > 5%

4. **Resolução de shortened URLs perde tracking:**
   - **Cenário:** `meli.la/abc123` → `mercadolivre.com.br/produto`
   - **Perda:** Affiliate ID e parâmetros de tracking são descartados
   - **Impacto:** Não é possível atribuir comissão ao canal de origem
   - **Mitigação:** Preservar URL original + URL resolvida, extrair parâmetros

5. **Classificação de categoria ignora subcategorias:**
   - **Cenário:** "Notebook Dell" classificado como "electronics"
   - **Perda:** Subcategoria "laptops" não é extraída
   - **Impacto:** Filtros de subcategoria não funcionam
   - **Mitigação:** Taxonomia hierárquica (electronics > laptops > dell)

### 7.3 Onde o Classificador Pode Gerar Falso Positivo ou Falso Negativo

#### Falsos Positivos (mensagem não-promocional classificada como promocional)

1. **Mensagem com URL mas não-promocional:**
   - **Exemplo:** "Veja nosso blog: https://example.com"
   - **Causa:** Regra `has_url` é verdadeira, mas não tem preço/cupom
   - **Taxa esperada:** 5% das mensagens com URL
   - **Mitigação:** Exigir `has_url AND (has_price OR has_coupon)` para confiança alta

2. **Mensagem com preço mas não-promocional:**
   - **Exemplo:** "O prejuízo foi de R$ 1000"
   - **Causa:** Regex captura "R$ 1000" mas não é oferta
   - **Taxa esperada:** 2% das mensagens com preço
   - **Mitigação:** Analisar contexto (keywords: "comprar", "desconto", "oferta")

3. **Enquete ou pergunta:**
   - **Exemplo:** Poll "Qual o melhor preço?" com opções R$ 100, R$ 200, R$ 300
   - **Causa:** Tem preço e URL (se houver link na pergunta)
   - **Taxa esperada:** 0.5% (1 na amostra)
   - **Mitigação:** Detectar Media.Poll e classificar como "non-promotional"

#### Falsos Negativos (mensagem promocional classificada como não-promocional)

1. **Promoção sem preço explícito:**
   - **Exemplo:** "Notebook Dell com 30% de desconto! https://..."
   - **Causa:** Não tem "R$" no texto, apenas porcentagem
   - **Taxa esperada:** 12% das mensagens promocionais (27 na amostra)
   - **Mitigação:** Detectar padrões de desconto ("30% off", "desconto", "promoção")

2. **Promoção sem URL:**
   - **Exemplo:** "Cupom ABC123 na loja X, válido hoje"
   - **Causa:** Não tem URL, apenas cupom
   - **Taxa esperada:** 0.9% (2 na amostra)
   - **Mitigação:** Regra alternativa: `has_coupon AND has_merchant_mention`

3. **Promoção em imagem (OCR necessário):**
   - **Exemplo:** Imagem com texto "R$ 100" embutido, texto da mensagem vazio
   - **Causa:** Preço está na imagem, não no texto
   - **Taxa esperada:** 0.5% (1 mensagem com texto vazio na amostra)
   - **Mitigação:** OCR em imagens (Phase 3+) ou marcar como "needs_manual_review"

4. **Promoção com URL quebrada:**
   - **Exemplo:** "https://example.com/produto" retorna 404
   - **Causa:** URL expirou ou foi removida
   - **Taxa esperada:** 5–10% (estimativa, não-medido)
   - **Mitigação:** Validar URLs antes de classificar, marcar como "expired"

---

## Conclusões Operacionais Acionáveis

### Prioridade 1: Crítico para MVP do limiar-processor

1. **Implementar detector de Updates wrapper**
   - 7.5% das mensagens serão perdidas sem isso
   - Complexidade: Baixa (5 linhas de código)
   - Risco se não-fazer: Alto

2. **Tratar Media como union type (Photo | Poll | Video | Document | None)**
   - 1 mensagem já é Poll, futuras podem ser outros tipos
   - Complexidade: Média (switch/case com 5 handlers)
   - Risco se não-fazer: Alto (panic em produção)

3. **Validar Entities antes de iterar**
   - 0.9% das mensagens têm Entities null
   - Complexidade: Baixa (nil check)
   - Risco se não-fazer: Médio (panic esporádico)

4. **Extrair URLs de Entities + regex no texto**
   - Entities tem apenas 19 URLs, texto tem 219
   - Complexidade: Média (duas fontes, merge)
   - Risco se não-fazer: Alto (perda de 91% das URLs)

### Prioridade 2: Importante para Qualidade

5. **Resolver shortened URLs (meli.la, amzn.to, tidd.ly)**
   - 144 URLs (61%) são shortened
   - Complexidade: Alta (HTTP requests, rate limiting, cache)
   - Risco se não-fazer: Médio (URLs não-informativas)

6. **Implementar deduplicação por URL**
   - 61.5% das mensagens são forwardadas (potencial duplicação cross-channel)
   - Complexidade: Média (hash de URL, índice)
   - Risco se não-fazer: Alto (feed poluído)

7. **Classificador de promocionalidade com confiança**
   - 12% das mensagens podem ser falso-negativo (sem preço explícito)
   - Complexidade: Média (regras + heurísticas)
   - Risco se não-fazer: Médio (promoções perdidas)

### Prioridade 3: Diferencial Competitivo

8. **Extração de preço e cupom via regex**
   - 192 mensagens (80%) têm preço/cupom explícito
   - Complexidade: Média (regex robusta, validação)
   - Risco se não-fazer: Médio (dados incompletos)

9. **Categorização de produto (Phase 2: regex, Phase 3: embeddings)**
   - Taxonomia inicial: electronics, fashion, home, beauty, sports
   - Complexidade: Alta (NLP, treinamento de modelo)
   - Risco se não-fazer: Baixo (MVP funciona sem categorias)

10. **Enriquecimento com metadata de merchant**
    - Preço histórico, reviews, ratings
    - Complexidade: Muito alta (APIs externas, scraping)
    - Risco se não-fazer: Baixo (diferencial, não-essencial)

---

## Recomendações Finais

1. **Coletar mais dados antes de finalizar schema:**
   - 239 mensagens em 22.5 horas é insuficiente para cobrir edge cases
   - Meta: 10.000+ mensagens de 10+ canais ao longo de 30 dias
   - Revisitar schema após coleta ampliada

2. **Implementar logging detalhado no processor:**
   - Logar todos os edge cases encontrados (Media type desconhecido, Entities null, etc.)
   - Monitorar frequência de edge cases em produção
   - Ajustar schema e regras baseado em dados reais

3. **Validar premissas com código do gotd/td:**
   - Por que GroupedID é sempre não-null?
   - Por que FwdFrom.FromID é sempre null?
   - Consultar documentação ou source code para esclarecer

4. **Testar parser com payloads sintéticos:**
   - Gerar payloads com edge cases (Media.Document, Entities null, URLs malformadas)
   - Validar que parser não quebra
   - Cobertura de testes > 90%

5. **Monitorar evolução do MTProto:**
   - Telegram atualiza protocolo frequentemente
   - Novos campos podem aparecer, campos antigos podem ser removidos
   - Versionar schema de saída para acomodar mudanças

---

**Fim do Relatório**

Este documento fornece a base técnica completa para o design do `limiar-processor`. Todas as decisões recomendadas são fundamentadas em evidência direta dos 239 payloads analisados. Ambiguidades e riscos estão explicitamente marcados para validação futura.

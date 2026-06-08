# Guia de Análise de Payloads — limiar-processor

**Propósito:** Este documento é um guia completo e auto-contido para que qualquer agente de IA realize análise técnica profunda de payloads JSON armazenados no banco de dados do limiar-collector, produzindo um relatório estruturado para orientar o design do limiar-processor.

**Pré-requisitos:**
- Acesso ao arquivo `limiar.db` (SQLite/Tursogo)
- Python 3 com módulo `sqlite3`
- jq (utilitário de linha de comando para JSON)
- Acesso à documentação do projeto (`docs/ARCHITECTURE.md`, `docs/CONTEXT.md`, `docs/PRODUCT_BRIEF.md`)

**Resultado esperado:** Relatório técnico em Markdown cobrindo 7 seções específicas (detalhadas abaixo).

---

## 1. Contexto do Sistema

### O que é o Limiar

Limiar é um agregador de promoções em tempo real que monitora canais brasileiros do Telegram via MTProto userbot. O sistema é composto por 3 binários independentes:

1. **limiar-collector** (implementado): Captura mensagens brutas do Telegram e persiste como JSON em banco SQLite
2. **limiar-processor** (a ser projetado): Normaliza, deduplica, classifica e filtra mensagens
3. **limiar-api** (futuro): Serve feed JSON via REST + SSE para frontend

### Onde você está no pipeline

Você está analisando a **saída do collector** para projetar a **entrada do processor**. Seu trabalho é mapear o shape real dos dados brutos para que o processor possa ser construído com base em evidências, não suposições.

### Estrutura do banco de dados

O banco `limiar.db` contém 5 tabelas:

```sql
-- Mensagens brutas (alvo da análise)
CREATE TABLE raw_messages (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id     INTEGER NOT NULL REFERENCES channels(id),
    message_id     INTEGER NOT NULL,
    payload        TEXT NOT NULL,           -- JSON bruto do MTProto
    received_at    TEXT NOT NULL DEFAULT (datetime('now')),
    schema_version INTEGER NOT NULL DEFAULT 1,
    UNIQUE(channel_id, message_id)
);

-- Canais monitorados
CREATE TABLE channels (
    id                INTEGER PRIMARY KEY,
    username          TEXT NOT NULL UNIQUE,
    title             TEXT NOT NULL DEFAULT '',
    active            INTEGER NOT NULL DEFAULT 1,
    added_at          TEXT NOT NULL DEFAULT (datetime('now')),
    last_message_id   INTEGER NOT NULL DEFAULT 0,
    last_collected_at TEXT
);

-- Cache de peers (usuários/canais do Telegram)
CREATE TABLE peers (
    id          INTEGER PRIMARY KEY,
    access_hash INTEGER NOT NULL,
    type        TEXT NOT NULL CHECK (type IN ('channel', 'user', 'chat')),
    username    TEXT,
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

-- Sessão MTProto (não-analisar, contém credenciais)
CREATE TABLE sessions (
    id         INTEGER PRIMARY KEY DEFAULT 1,
    data       BLOB NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK (id = 1)
);

-- Autoincrement tracking
CREATE TABLE sqlite_sequence (
    name TEXT,
    seq  INTEGER
);
```

**Importante:** A tabela `sessions` contém dados sensíveis (credenciais MTProto). **Nunca** extraia, log ou analise o conteúdo de `sessions.data`.

---

## 2. Metodologia de Análise

### Passo 1: Estatísticas básicas

**Objetivo:** Entender o volume e distribuição dos dados.

**Script Python:**

```python
import sqlite3

conn = sqlite3.connect('limiar.db')
cursor = conn.cursor()

# Contar mensagens
cursor.execute("SELECT COUNT(*) FROM raw_messages")
total_messages = cursor.fetchone()[0]
print(f"Total de mensagens: {total_messages}")

# Contar canais
cursor.execute("SELECT COUNT(*) FROM channels")
total_channels = cursor.fetchone()[0]
print(f"Total de canais: {total_channels}")

# Distribuição por canal
cursor.execute("""
    SELECT c.username, c.title, COUNT(rm.id) as msg_count
    FROM channels c
    LEFT JOIN raw_messages rm ON c.id = rm.channel_id
    GROUP BY c.id
    ORDER BY msg_count DESC
""")
print("\nDistribuição por canal:")
for row in cursor.fetchall():
    print(f"  @{row[0]} ({row[1]}): {row[2]} mensagens")

# Range temporal
cursor.execute("""
    SELECT MIN(received_at), MAX(received_at),
           julianday(MAX(received_at)) - julianday(MIN(received_at)) as days
    FROM raw_messages
""")
min_date, max_date, days = cursor.fetchone()
print(f"\nRange temporal: {min_date} → {max_date} ({days:.1f} dias)")

conn.close()
```

**O que procurar:**
- Volume total é estatisticamente significativo? (meta: >1000 mensagens)
- Distribuição é balanceada ou há canais dominantes?
- Range temporal cobre variações sazonais? (meta: >7 dias)

**Critério de qualidade:** Se volume < 500 mensagens ou range < 3 dias, recomende coleta adicional antes de finalizar o relatório.

### Passo 2: Exportar payloads para análise com jq

**Objetivo:** Extrair todos os payloads para arquivo JSON que possa ser processado com jq.

**Script Python:**

```python
import sqlite3
import json

conn = sqlite3.connect('limiar.db')
conn.row_factory = sqlite3.Row
cursor = conn.cursor()

cursor.execute("""
    SELECT id, channel_id, message_id, payload, received_at, schema_version
    FROM raw_messages
    ORDER BY id
""")

records = []
for row in cursor.fetchall():
    record = {
        'id': row['id'],
        'channel_id': row['channel_id'],
        'message_id': row['message_id'],
        'received_at': row['received_at'],
        'schema_version': row['schema_version'],
    }
    
    payload_str = row['payload']
    if payload_str and payload_str != 'null':
        try:
            record['payload'] = json.loads(payload_str)
        except json.JSONDecodeError:
            record['payload'] = None
            record['payload_invalid'] = True
    else:
        record['payload'] = None
    
    records.append(record)

with open('payloads_export.json', 'w', encoding='utf-8') as f:
    json.dump(records, f, indent=2, ensure_ascii=False)

print(f"Exportadas {len(records)} mensagens para payloads_export.json")
conn.close()
```

**Validação:**
```bash
# Verificar que arquivo foi criado
ls -lh payloads_export.json

# Validar JSON
jq empty payloads_export.json && echo "JSON válido" || echo "JSON inválido"

# Contar registros
jq length payloads_export.json
```

### Passo 3: Identificar shapes de payload

**Objetivo:** Detectar se há múltiplas estruturas de payload (ex: mensagem direta vs envelope de updates).

**Comandos jq:**

```bash
# Contar payloads com campo "Updates" (envelope)
jq '[.[] | select(.payload | has("Updates"))] | length' payloads_export.json

# Contar payloads sem campo "Updates" (mensagem direta)
jq '[.[] | select(.payload | has("Updates") | not)] | length' payloads_export.json

# Mostrar estrutura do envelope
jq '[.[] | select(.payload | has("Updates"))] | .[0].payload | keys' payloads_export.json

# Mostrar estrutura da mensagem direta
jq '[.[] | select(.payload | has("Updates") | not)] | .[0].payload | keys' payloads_export.json
```

**O que procurar:**
- Quantos shapes distintos existem?
- Qual a proporção de cada shape?
- O envelope `Updates` contém a mensagem real em `Updates[0].Message`?

**Ação:** Se houver múltiplos shapes, documente cada um separadamente e explique como normalizá-los para o processor.

### Passo 4: Analisar campos de nível superior

**Objetivo:** Mapear todos os campos presentes nos payloads, frequência e tipos.

**Comandos jq:**

```bash
# Listar todos os campos e frequência (para mensagens diretas)
jq '[.[] | select(.payload | has("Updates") | not) | .payload | keys[]] | group_by(.) | map({field: .[0], count: length}) | sort_by(-.count)' payloads_export.json

# Identificar campos com tipos variáveis
jq '
  [.[] | select(.payload | has("Updates") | not) | .payload] |
  reduce .[] as $msg ({}; 
    reduce ($msg | keys[]) as $key (.; 
      .[$key] += [($msg[$key] | type)]
    )
  ) |
  to_entries |
  map(select(.value | unique | length > 1)) |
  map({field: .key, types: (.value | unique)})
' payloads_export.json

# Mostrar campos sempre presentes (100%)
jq '
  [.[] | select(.payload | has("Updates") | not) | .payload | keys[]] |
  group_by(.) |
  map({field: .[0], count: length}) |
  map(select(.count == 221)) |  # Ajuste para o total de mensagens diretas
  .[].field
' payloads_export.json
```

**O que procurar:**
- Quais campos estão presentes em >95% das mensagens? (essenciais)
- Quais campos têm tipos variáveis? (necessitam validação)
- Quais campos são sempre null ou default? (candidatos a descarte)

### Passo 5: Analisar estruturas aninhadas

**Objetivo:** Entender a forma de campos complexos como `Media`, `Entities`, `FwdFrom`, `Reactions`.

#### 5.1 Media

```bash
# Contar tipos de Media
jq '
  [.[] | select(.payload | has("Updates") | not) | .payload.Media] |
  map(
    if .Photo != null then "photo"
    elif .Poll != null then "poll"
    elif .Video != null then "video"
    elif .Document != null then "document"
    else "unknown"
    end
  ) |
  group_by(.) |
  map({type: .[0], count: length})
' payloads_export.json

# Mostrar estrutura de Photo
jq '[.[] | select(.payload | has("Updates") | not and .payload.Media.Photo != null)] | .[0].payload.Media' payloads_export.json

# Analisar Photo.Sizes
jq '
  [.[] | select(.payload | has("Updates") | not and .payload.Media.Photo != null)] |
  [.payload.Media.Photo.Sizes | length] |
  group_by(.) |
  map({sizes_count: .[0], frequency: length})
' payloads_export.json

# Mostrar estrutura de Poll (se existir)
jq '[.[] | select(.payload | has("Updates") | not and .payload.Media.Poll != null)] | .[0].payload.Media.Poll' payloads_export.json
```

**O que procurar:**
- Quantos tipos de Media existem? (Photo, Poll, Video, Document, etc.)
- Photo.Sizes tem estrutura consistente?
- Poll tem campos completos (Question, Answers, Results)?

#### 5.2 Entities

```bash
# Contar entidades por tipo de chave
jq '
  [.[] | select(.payload | has("Updates") | not) | .payload.Entities[]?] |
  map(keys | length) |
  group_by(.) |
  map({keys_count: .[0], frequency: length})
' payloads_export.json

# Mostrar entidades com 2 chaves (Offset + Length)
jq '[.[] | select(.payload | has("Updates") | not) | .payload.Entities[]? | select(keys | length == 2)] | .[0:3]' payloads_export.json

# Mostrar entidades com 3 chaves (Offset + Length + URL)
jq '[.[] | select(.payload | has("Updates") | not) | .payload.Entities[]? | select(keys | length == 3)] | .[0:3]' payloads_export.json

# Contar mensagens com Entities null
jq '[.[] | select(.payload | has("Updates") | not and .payload.Entities == null)] | length' payloads_export.json
```

**O que procurar:**
- Quantos tipos de entidade existem? (text formatting, URL, mention, hashtag)
- Entities está sempre presente ou às vezes é null?
- URLs em Entities correspondem a URLs no texto?

#### 5.3 FwdFrom

```bash
# Verificar se FwdFrom.FromID é sempre null
jq '
  [.[] | select(.payload | has("Updates") | not) | .payload.FwdFrom.FromID] |
  group_by(type) |
  map({type: .[0] | type, count: length})
' payloads_export.json

# Contar mensagens com Forwards > 0
jq '[.[] | select(.payload | has("Updates") | not and .payload.Forwards > 0)] | length' payloads_export.json
```

**O que procurar:**
- FwdFrom.FromID é sempre null? (sugere que mensagens não são forwards)
- Forwards > 0 indica que a mensagem foi forwardada (contador), não que é um forward

### Passo 6: Analisar conteúdo de texto

**Objetivo:** Extrair padrões de texto, URLs, preços, cupons.

#### 6.1 Estatísticas de texto

```bash
# Comprimento do texto
jq '
  [.[] | select(.payload | has("Updates") | not) | .payload.Message | length] |
  {min: min, max: max, avg: (add / length | floor), median: (sort | .[length/2 | floor])}
' payloads_export.json

# Contar mensagens vazias
jq '[.[] | select(.payload | has("Updates") | not and .payload.Message == "")] | length' payloads_export.json

# Contar mensagens com URLs
jq '[.[] | select(.payload | has("Updates") | not and (.payload.Message | test("https?://"; "i")))] | length' payloads_export.json
```

#### 6.2 Extração de URLs

```bash
# Extrair todos os domínios de URLs no texto
jq -r '
  [.[] | select(.payload | has("Updates") | not) | .payload.Message |
   match("https?://[^\\s]+"; "g") | .string] |
  map(capture("(?<domain>[^/]+://[^/]+)")) |
  group_by(.domain) |
  map({domain: .[0].domain, count: length}) |
  sort_by(-.count)
' payloads_export.json

# Comparar URLs em Entities vs URLs no texto
jq '
  {
    urls_in_entities: [.[] | select(.payload | has("Updates") | not) | .payload.Entities[]? | select(.URL != null) | .URL] | length,
    urls_in_text: [.[] | select(.payload | has("Updates") | not) | .payload.Message | match("https?://[^\\s]+"; "g")] | length
  }
' payloads_export.json
```

**O que procurar:**
- Quantas URLs estão em Entities vs texto? (se Entities < texto, usar regex como fonte primária)
- Quais domínios são mais frequentes? (meli.la, amzn.to, shopee, etc.)
- URLs são shortened ou diretas?

#### 6.3 Extração de preços e cupons

```bash
# Contar mensagens com preço (R$)
jq '[.[] | select(.payload | has("Updates") | not and (.payload.Message | test("R\\$"; "i")))] | length' payloads_export.json

# Contar mensagens com cupom
jq '[.[] | select(.payload | has("Updates") | not and (.payload.Message | test("[Cc]upom|CUPOM|cupão"; "i")))] | length' payloads_export.json

# Extrair amostras de preços
jq -r '
  [.[] | select(.payload | has("Updates") | not) | .payload.Message |
   select(test("R\\$"; "i")) |
   match("R\\$\\s*[\\d.,]+"; "g") | .string] |
  .[0:10]
' payloads_export.json

# Extrair amostras de cupons
jq -r '
  [.[] | select(.payload | has("Updates") | not) | .payload.Message |
   select(test("[Cc]upom"; "i")) |
   match("[Cc]upom[:\\s]+[A-Z0-9]+"; "g") | .string] |
  .[0:10]
' payloads_export.json
```

**O que procurar:**
- Qual % das mensagens tem preço? (esperado: >70%)
- Qual % das mensagens tem cupom? (esperado: >50%)
- Formato de preços é consistente? (R$ 123,45 vs R$123.45)
- Formato de cupons é consistente? (AEBR2, IFPL90V1, etc.)

### Passo 7: Identificar edge cases

**Objetivo:** Encontrar mensagens que fogem do padrão e podem quebrar o processor.

```bash
# Mensagens sem Media (nem Photo nem Poll)
jq '[.[] | select(.payload | has("Updates") | not and .payload.Media.Photo == null and .payload.Media.Poll == null)] | length' payloads_export.json

# Mensagens com texto vazio
jq '[.[] | select(.payload | has("Updates") | not and .payload.Message == "")] | .[].id' payloads_export.json

# Mensagens sem URL no texto
jq '[.[] | select(.payload | has("Updates") | not and (.payload.Message | test("https?://"; "i") | not))] | .[].id' payloads_export.json

# Mensagens com ReplyTo não-null (respostas)
jq '[.[] | select(.payload | has("Updates") | not and .payload.ReplyTo != null)] | length' payloads_export.json

# Mensagens com ReplyMarkup (inline buttons)
jq '[.[] | select(.payload | has("Updates") | not and .payload.ReplyMarkup != null)] | length' payloads_export.json

# Mensagens com GroupedID não-null (álbuns)
jq '[.[] | select(.payload | has("Updates") | not and .payload.GroupedID != null)] | length' payloads_export.json

# Distribuição de Views
jq '
  [.[] | select(.payload | has("Updates") | not) | .payload.Views | select(. != null)] |
  {min: min, max: max, avg: (add / length | floor), p50: (sort | .[length/2 | floor]), p95: (sort | .[length * 95 / 100 | floor])}
' payloads_export.json
```

**O que procurar:**
- Mensagens sem Media são válidas ou erro de coleta?
- Mensagens com texto vazio têm imagem com texto embutido?
- Mensagens sem URL são promocionais ou avisos administrativos?
- ReplyTo/ReplyMarkup estão presentes? (sempre null na amostra atual)

### Passo 8: Analisar envelope Updates (se presente)

**Objetivo:** Entender a estrutura do envelope e como extrair a mensagem real.

```bash
# Mostrar estrutura completa do envelope
jq '[.[] | select(.payload | has("Updates"))] | .[0].payload' payloads_export.json

# Verificar se Updates tem múltiplas mensagens
jq '[.[] | select(.payload | has("Updates"))] | map(.payload.Updates | length) | group_by(.) | map({updates_count: .[0], frequency: length})' payloads_export.json

# Extrair mensagem do envelope
jq '[.[] | select(.payload | has("Updates"))] | .[0].payload.Updates[0].Message | keys' payloads_export.json

# Verificar Users e Chats no envelope
jq '[.[] | select(.payload | has("Updates"))] | map({users: (.payload.Users | length), chats: (.payload.Chats | length)})' payloads_export.json
```

**O que procurar:**
- Updates sempre tem 1 mensagem ou pode ter múltiplas?
- Users/Chats contêm metadata útil (nomes, usernames)?
- Mensagem em Updates[0].Message tem mesma estrutura que mensagem direta?

---

## 3. Estrutura do Relatório

O relatório final deve seguir esta estrutura exata:

### Seção 1: Escopo e Método

**Conteúdo obrigatório:**
- Ferramentas usadas (Python, jq, etc.)
- Quantidade de registros analisados
- Range temporal da coleta
- Limitações da amostra (volume, diversidade, sazonalidade)
- Como cada ferramenta foi usada (queries específicas)

**Exemplo:**
```markdown
## 1. Escopo e Método

**Banco analisado:** limiar.db (1.1 MB, Tursogo)  
**Ferramentas:** Python 3 + sqlite3, jq 1.6  
**Registros analisados:** 239 mensagens (100% do total)  
**Canais monitorados:** 4 (@lobaopromo, @gatunopromos, @xetdaspromocoes, @iuriindica)  
**Range temporal:** 2026-06-07 19:20 → 2026-06-08 17:47 (22.5 horas)

**Limitações:**
1. Volume baixo (239 mensagens) — insuficiente para edge cases raros
2. Janela curta (22.5h) — não cobre variações sazonais
3. Viés de canal — 4 canais específicos, não-representativo
4. Sem validação de evolução de schema — todos schema_version=1

**Recomendação:** Coletar 10.000+ mensagens de 10+ canais ao longo de 30 dias antes de finalizar schema do processor.
```

### Seção 2: Persona Estrutural dos Dados

**Conteúdo obrigatório:**
- Todos os shapes de payload encontrados (com exemplos JSON)
- Campos estáveis (presentes em >95% das mensagens)
- Campos variáveis (tipos múltiplos, presença intermitente)
- Estruturas aninhadas (Media, Entities, FwdFrom, Reactions)
- Campos sempre null ou default

**Formato recomendado:**
```markdown
## 2. Persona Estrutural dos Dados

### 2.1 Shapes Identificados

**Shape A: Mensagem Direta (92.5% — 221 mensagens)**
```json
{
  "Date": 1780858455,
  "ID": 12345,
  "PeerID": {"ChannelID": 1987091586},
  "Message": "Texto da promoção...",
  "Media": {...},
  "Entities": [...],
  "Views": 683,
  "Forwards": 5,
  // ... 44 outros campos
}
```

**Shape B: Envelope Updates (7.5% — 18 mensagens)**
```json
{
  "Updates": [{"Message": {...}, "Pts": 12345, "PtsCount": 1}],
  "Users": [...],
  "Chats": [...],
  "Seq": 0
}
```

### 2.2 Campos Estáveis

| Campo | Presença | Tipo | Estabilidade |
|-------|----------|------|--------------|
| Date | 100% | int (Unix) | Estável |
| ID | 92.5% | int | Estável |
| PeerID | 92.5% | object | Estável |
| Message | 92.5% | string | Estável |
| Media | 92.5% | object | Estável |

### 2.3 Estruturas Variáveis

#### Media (3 variações)

**Photo (99.5% — 220 mensagens)**
```json
{
  "Photo": {
    "ID": 6138941836333092787,
    "AccessHash": -8328281025564808123,
    "Sizes": [...]
  }
}
```

**Poll (0.5% — 1 mensagem)**
```json
{
  "Poll": {
    "Question": {"Text": "Ganhador do 6.6!?"},
    "Answers": [...]
  }
}
```
```

### Seção 3: Anomalias e Edge Cases

**Conteúdo obrigatório:**
- Payloads inválidos ou truncados (com IDs)
- Campos com tipos inesperados
- Mensagens vazias, sem URL, sem preço
- Casos raros que podem quebrar o processor
- Interpretação de anomalias (bug vs feature)

**Formato recomendado:**
```markdown
## 3. Anomalias e Edge Cases

### 3.1 Payloads Inválidos
- **Inválidos:** 0
- **Truncados:** 0
- **Incompletos:** 0

Todos os 239 payloads são JSON válido.

### 3.2 Mensagens com Texto Vazio (0.5% — 1 mensagem)
- **ID:** 19
- **Message:** ""
- **Media:** Photo presente
- **Interpretação:** Mensagem apenas com imagem, sem texto descritivo
- **Risco:** Processor pode falhar ao extrair sinais de texto vazio
- **Mitigação:** Verificar `Message.length > 0` antes de processar texto

### 3.3 FwdFrom.FromID Sempre Null (100%)
- **Observação:** Estrutura FwdFrom presente em todas as mensagens, mas FromID sempre null
- **Interpretação:** Mensagens não são forwards; campo Forwards é contador de forward, não indicador
- **Risco:** Se processor tentar extrair "mensagem original" de FwdFrom, vai falhar
- **Mitigação:** Ignorar FwdFrom, usar Forwards como métrica de engajamento
```

### Seção 4: Relevância para o Pipeline

**Conteúdo obrigatório:**
- Campos essenciais (com justificativa de uso)
- Campos secundários (com prioridade)
- Campos descartáveis (com justificativa)
- Decisões de design baseadas em evidência

**Formato recomendado:**
```markdown
## 4. Relevância para o Pipeline

### 4.1 Campos Essenciais

| Campo | Uso no Pipeline | Justificativa |
|-------|-----------------|---------------|
| ID | Deduplicação | Identificador único da mensagem |
| PeerID.ChannelID | Agrupamento | Identifica canal de origem |
| Date | Ordenação | Timestamp para ordenar e filtrar |
| Message | Extração de sinais | Texto contém produto, preço, URL |
| Media.Photo | Enriquecimento | Imagem do produto |
| Entities[] | Extração de URLs | URLs já-parseadas pelo MTProto |
| Views | Ranking | Métrica de engajamento |
| Forwards | Ranking | Métrica de viralidade |

### 4.2 Campos Descartáveis

| Campo | Justificativa |
|-------|---------------|
| Flags, Flags2 | Bitmasks internos do MTProto |
| Out, Mentioned, Silent | Flags de estado do cliente |
| FwdFrom | Sempre com campos default/null |
| ReplyMarkup | Sempre null na amostra |
```

### Seção 5: Schema de Saída Proposto

**Conteúdo obrigatório:**
- Estrutura JSON normalizada (exemplo completo)
- Tipos esperados por campo (tabela)
- Campos obrigatórios vs opcionais
- Regras de limpeza, enriquecimento, deduplicação, classificação
- Estratégia de versionamento

**Formato recomendado:**
```markdown
## 5. Schema de Saída Proposto

### 5.1 Estrutura Normalizada

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
    "text": "Texto da promoção...",
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
      "url": "https://..."
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
    "product_name": "Memória Ram DDR4 Jazer 8GB",
    "price": {
      "amount": 234.00,
      "currency": "BRL",
      "formatted": "R$ 234"
    },
    "coupons": ["AEBR2", "IFPL90V1"],
    "instructions": "Selecione a primeira opção...",
    "expiry": null
  },
  "classification": {
    "is_promotional": true,
    "confidence": 0.95,
    "category": "electronics",
    "subcategory": "computer_memory",
    "tags": ["hardware", "ram", "ddr4"]
  },
  "deduplication": {
    "content_hash": "a1b2c3d4e5f6...",
    "url_hash": "f6e5d4c3b2a1...",
    "similar_messages": []
  }
}
```

### 5.2 Regras de Limpeza

1. **Texto:** Remover whitespace excessivo, normalizar Unicode
2. **URLs:** Resolver shortened URLs, extrair parâmetros de tracking
3. **Preços:** Extrair com regex `R\$\s*([\d.,]+)`, normalizar para float
4. **Cupons:** Extrair com regex `[Cc]upom[:\s]+([A-Z0-9]+)`, validar formato

### 5.3 Regras de Deduplicação

1. **Nível 1:** `(channel_id, message_id)` — já-implementado no collector
2. **Nível 2:** `SHA-256(normalized_url)` — mesma URL em múltiplos canais
3. **Nível 3:** `SHA-256(normalized_text)` — conteúdo similar (Jaccard > 0.85)
```

### Seção 6: Perguntas em Aberto

**Conteúdo obrigatório:**
- Ambiguidades não-resolvidas após análise
- Decisões que dependem de produto/engenharia
- Riscos de assumir premissas erradas

**Formato recomendado:**
```markdown
## 6. Perguntas em Aberto

### 6.1 Ambiguidades

1. **GroupedID sempre presente:** Todas as 221 mensagens têm GroupedID não-null. Isso é normal para canais de promoções ou artefato da serialização? **Decisão necessária:** Validar com mais dados ou inspecionar código do gotd/td.

2. **Entities incompletas:** Apenas 19 URLs em Entities vs 219 URLs no texto. Entities está capturando apenas algumas URLs? **Decisão necessária:** Confiar em Entities ou usar regex como fonte primária?

### 6.2 Decisões de Produto

1. **Resolução de shortened URLs:** Deve ser feita em tempo real (latência +100–500ms) ou batch assíncrono? **Produto:** Qual a latência aceitável para o feed?

2. **Deduplicação fuzzy:** Threshold de 0.85 é muito agressivo? Mensagens com mesmo produto mas preços diferentes são duplicatas? **Produto:** Qual a UX para promoções duplicadas?

### 6.3 Riscos de Premissas

1. **Premissa:** "Toda mensagem de canal de promoção é promocional"  
   **Risco:** Canais podem postar avisos, enquetes, mensagens administrativas  
   **Evidência:** 1 mensagem é Poll, 1 é "cupom esgotado"  
   **Mitigação:** Classificador não pode ser 100% confiante
```

### Seção 7: Riscos Técnicos do limiar-processor

**Conteúdo obrigatório:**
- Onde o parser pode quebrar (com cenários e mitigações)
- Onde o normalizador pode perder sinal útil
- Onde o classificador pode gerar falso positivo/negativo

**Formato recomendado:**
```markdown
## 7. Riscos Técnicos

### 7.1 Parser Breakage Points

1. **Updates wrapper não-detectado**  
   **Cenário:** 18 mensagens têm estrutura `{Updates: [...]}`  
   **Falha:** Parser tenta acessar `.Message` no envelope, recebe `undefined`  
   **Impacto:** 7.5% das mensagens descartadas  
   **Mitigação:** Detector de wrapper no início do pipeline

2. **Media type não-previsto**  
   **Cenário:** Futura mensagem com Media.Document  
   **Falha:** Switch/case não tem handler  
   **Impacto:** Panic ou mensagem descartada  
   **Mitigação:** Default handler que loga warning

### 7.2 Signal Loss

1. **Limpeza agressiva de emojis**  
   **Cenário:** Emojis 🔥, 💰 removidos como ruído  
   **Perda:** Emojis podem indicar categoria (📱 = electronics)  
   **Mitigação:** Preservar emojis, extrair como features

### 7.3 False Positives/Negatives

**Falsos Positivos:**
1. Mensagem com URL mas não-promocional: "Veja nosso blog: https://..."  
   **Taxa esperada:** 5%  
   **Mitigação:** Exigir `has_url AND (has_price OR has_coupon)`

**Falsos Negativos:**
1. Promoção sem preço explícito: "Notebook Dell com 30% de desconto!"  
   **Taxa esperada:** 12%  
   **Mitigação:** Detectar padrões de desconto ("30% off", "desconto")
```

---

## 4. Critérios de Qualidade do Relatório

O relatório deve atender a todos estes critérios:

1. **Técnico e verificável:** Todas as afirmações são suportadas por dados extraídos do banco
2. **Direto e conciso:** Sem linguagem vaga ("parece variado", "muitas mensagens")
3. **Tabelas sobre texto:** Preferir tabelas para comparar campos, tipos, frequência
4. **Exemplos com propósito:** Incluir JSON apenas quando necessário para provar padrão
5. **Não-extrapolativo:** Não afirmar o que não foi observado nos dados
6. **Acionável:** Concluir com recomendações priorizadas para o processor
7. **Honesto sobre limitações:** Explicitar o que a amostra não cobre

**Checklist final:**
- [ ] Todas as queries jq usadas estão documentadas
- [ ] Números somam corretamente (total = soma das partes)
- [ ] Edge cases são identificados por ID de mensagem
- [ ] Schema proposto cobre todos os casos observados
- [ ] Riscos têm mitigações específicas (não genéricas)
- [ ] Recomendações são priorizadas (P1/P2/P3)
- [ ] Limitações da amostra são explicitadas

---

## 5. Exemplos de Queries Úteis

### Estatísticas rápidas

```bash
# Total de mensagens
jq length payloads_export.json

# Mensagens por canal
jq '[.[] | .channel_id] | group_by(.) | map({channel: .[0], count: length})' payloads_export.json

# Mensagens por hora
jq '[.[] | .received_at | split(" ")[1] | split(":")[0]] | group_by(.) | map({hour: .[0], count: length})' payloads_export.json

# Payloads inválidos
jq '[.[] | select(.payload_invalid == true)] | length' payloads_export.json
```

### Análise de conteúdo

```bash
# Palavras mais frequentes no texto
jq -r '[.[] | .payload.Message | ascii_downcase | match("\\b[a-záéíóúâêîôûãõç]+\\b"; "g") | .string] | group_by(.) | map({word: .[0], count: length}) | sort_by(-.count) | .[0:20]' payloads_export.json

# Emojis mais frequentes
jq -r '[.[] | .payload.Message | match("[😀-🙏💰🔥📱🎧👗🏠]"; "g") | .string] | group_by(.) | map({emoji: .[0], count: length}) | sort_by(-.count)' payloads_export.json

# Comprimento médio de URLs
jq '[.[] | .payload.Message | match("https?://[^\\s]+"; "g") | .string | length] | {avg: (add / length | floor), min: min, max: max}' payloads_export.json
```

### Validação de schema

```bash
# Verificar se todos os payloads têm campos obrigatórios
jq '
  [.[] | select(.payload | has("Updates") | not)] |
  map(
    {
      has_date: (.payload | has("Date")),
      has_id: (.payload | has("ID")),
      has_peerid: (.payload | has("PeerID")),
      has_message: (.payload | has("Message")),
      has_media: (.payload | has("Media"))
    }
  ) |
  {
    total: length,
    with_date: [.[] | select(.has_date)] | length,
    with_id: [.[] | select(.has_id)] | length,
    with_peerid: [.[] | select(.has_peerid)] | length,
    with_message: [.[] | select(.has_message)] | length,
    with_media: [.[] | select(.has_media)] | length
  }
' payloads_export.json
```

---

## 6. Referências

### Documentação do projeto

- `docs/ARCHITECTURE.md` — Arquitetura do limiar-collector
- `docs/CONTEXT.md` — Visão geral do sistema Limiar
- `docs/PRODUCT_BRIEF.md` — Briefing de produto

### Código-fonte relevante

- `internal/storage/repository.go` — Schema SQL e queries
- `internal/storage/migrations/001_initial.sql` — Definição de tabelas
- `internal/telegram/client.go` — Como payloads são serializados
- `internal/collector/handler.go` — Como payloads são persistidos

### Recursos externos

- [gotd/td documentation](https://github.com/gotd/td) — Biblioteca MTProto usada
- [Telegram API documentation](https://core.telegram.org/api) — Especificação do protocolo
- [jq manual](https://stedolan.github.io/jq/manual/) — Referência de queries jq

---

## 7. Checklist de Execução

Use este checklist para garantir que todas as etapas foram completadas:

- [ ] **Passo 1:** Estatísticas básicas extraídas (total, canais, range temporal)
- [ ] **Passo 2:** Payloads exportados para `payloads_export.json`
- [ ] **Passo 3:** Shapes de payload identificados (direto vs envelope)
- [ ] **Passo 4:** Campos de nível superior analisados (frequência, tipos)
- [ ] **Passo 5:** Estruturas aninhadas analisadas (Media, Entities, FwdFrom, Reactions)
- [ ] **Passo 6:** Conteúdo de texto analisado (URLs, preços, cupons)
- [ ] **Passo 7:** Edge cases identificados (mensagens vazias, sem URL, tipos raros)
- [ ] **Passo 8:** Envelope Updates analisado (se presente)
- [ ] **Seção 1:** Escopo e método documentados
- [ ] **Seção 2:** Persona estrutural mapeada
- [ ] **Seção 3:** Anomalias e edge cases catalogados
- [ ] **Seção 4:** Relevância de campos classificada
- [ ] **Seção 5:** Schema de saída proposto
- [ ] **Seção 6:** Perguntas em aberto listadas
- [ ] **Seção 7:** Riscos técnicos identificados com mitigações
- [ ] **Qualidade:** Relatório atende a todos os critérios de qualidade
- [ ] **Validação:** Números somam corretamente, exemplos são verificáveis

---

**Fim do Guia**

Este documento é auto-contido. Um agente de IA que receba este guia + acesso ao banco `limiar.db` deve ser capaz de produzir um relatório técnico completo e acionável para o design do limiar-processor, sem necessidade de contexto adicional.

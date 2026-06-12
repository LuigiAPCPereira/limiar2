# Product Brief — Limiar

## O que é o Limiar

Limiar é um sistema de monitoramento e processamento de canais brasileiros do Telegram via MTProto userbot. Ele lê mensagens de canais de promoções, normaliza, deduplica, classifica e entrega um JSON padronizado como saída — alimentando o Limiar Frontend, um site de promoções em tempo real.

---

## Problema que resolve

Canais de promoções no Telegram são fontes ricas mas caóticas: mensagens duplicadas entre canais, formatos inconsistentes, sem categorização, sem estrutura. O Limiar transforma esse ruído em dados limpos, estruturados e prontos para consumo.

---

## Arquitetura geral (pipeline)

```
Telegram MTProto
       ↓
limiar-collector → conecta via userbot, lê histórico e eventos, persiste raw
       ↓
limiar-processor → normaliza, deduplica, classifica, sintetiza
       ↓
limiar-api → serve REST (histórico paginado) + SSE (push em tempo real)
       ↓
Limiar Frontend → site de promoções ao vivo
```

Cada componente é um binário Go independente com lifecycle próprio. Um orquestrador `limiar` futuro fará o spawn dos três.

---

## Binários

| Binário | Responsabilidade | Status |
|---------|------------------|--------|
| `limiar-collector` | MTProto userbot: autentica, monitora canais, persiste mensagens brutas | ✅ Implementado |
| `limiar-processor` | Lê do banco, normaliza, deduplica, classifica, sintetiza | ✅ Implementado |
| `limiar-api` | Expõe REST + SSE para o frontend | Futuro |
| `limiar` | Orquestrador: sobe os binários com `limiar run` | ✅ Implementado |

---

## Stack

| Camada | Tecnologia |
|--------|-----------|
| MTProto | `gotd/td` — sem wrapper, implementação própria de session/peer storage |
| Banco de dados | `turso.tech/database/tursogo` — embedded, arquivo `.db`, sem CGO, driver `"turso"` para `database/sql` |
| CLI | Cobra + Viper |
| Logging | Interface `Logger` modular com `slog` da stdlib + `PrettyHandler` custom + `Presenter` para mensagens ao operador |
| Terminal | `golang.org/x/term` — entrada mascarada no auth wizard |
| HTTP | stdlib `net/http` (dashboard) |
| Push realtime | SSE (Server-Sent Events) via dashboard Broker |
| Classificação básica | Regras e regex (cascata de 11 tipos no processor) |
| Classificação semântica | API LLM externa em batches (futuro — Fase 3) |

---

## Decisões de design relevantes

**Sem GoTGProto:** a biblioteca acopla session e peer storage ao GORM/SQLite via struct concreta não plugável. O Limiar usa Tursogo como banco único — por isso implementa a camada MTProto diretamente sobre `gotd/td`.

**Sem CGO:** `tursogo` usa `purego` para FFI. Nenhuma variável de ambiente especial necessária, binário portável.

**Tursogo como único banco:** sessão MTProto, peer cache, canais monitorados, mensagens brutas, mensagens processadas e histórico de deduplicação vivem todos no mesmo arquivo `.db`. Em produção, o arquivo é montado como volume persistente.

**Logging modular com três camadas:**
- `Logger` (interface) — log estruturado injetado via construtor em todas as camadas. Implementação usa `slog` com três handlers: JSON, Text e Pretty.
- `PrettyHandler` — handler custom para output legível com cores ANSI, formato multilinhas com tree-view de atributos.
- `Presenter` (interface) — mensagens de apresentação ao operador humano (Info/Success/Warning/Error/Step) com emojis e cor condicional, separadas do log estruturado.
- `ResolveFormat` — resolução de formato baseada em env + TTY detection.

**Redação de segredos:** chaves sensíveis (`api_hash`, `apihash`, `session`, `token`, `password`, `auth_code`) são automaticamente mascaradas em todos os formatos de log.

**Observer para updates:** updates do Telegram disparam para múltiplos handlers via channels do Go. Prepara o pipeline para receber novos consumers sem acoplamento.

**Strategy para classificação:** interface `Classifier` plugável no collector (Phase 1 usa `NoopClassifier`). O processor implementa sua própria cascata de classificação baseada em regras.

**Facade sobre gotd/td:** toda a complexidade MTProto (session, peers, reconexão, auth flow) fica atrás de uma interface `TelegramClient` limpa.

**Preços são INTEGER (centavos):** R$ 83,50 → 8350. Conversão acontece apenas nas boundaries (extração regex → ×100 → int64). Nunca float64 para preços.

**Processamento idempotente:** `SaveProcessed` usa `ON CONFLICT DO NOTHING` em `(channel_id, message_id)`. Reprocessar a mesma raw_message é seguro.

---

## Fase 1 — limiar-collector ✅

**Objetivo:** conectar ao Telegram, autenticar como userbot, monitorar canais configurados e persistir mensagens brutas em JSON no Tursogo.

**Comandos CLI:**

```
limiar-collector auth         # autentica e persiste sessão (interativo, roda uma vez)
limiar-collector channels     # list / add / remove canais monitorados
limiar-collector run          # sobe o serviço de coleta (--dashboard opcional)
limiar-collector dashboard    # dashboard HTTP standalone
```

**Done:** ✅
- `auth` persiste sessão válida no Tursogo
- `channels` gerencia a lista de canais no banco
- `run` recebe mensagens e salva o payload JSON bruto com log estruturado
- Graceful shutdown em SIGTERM/SIGINT
- Dashboard HTTP para inspeção local

---

## Fase 2 — limiar-processor ✅

**Objetivo:** ler mensagens brutas do banco, normalizá-las para estrutura canônica, classificar por tipo, deduplicar cross-channel, e persistir para consumo do futuro API.

**Execução:**

```
limiar-processor              # inicia o loop de poll
limiar-processor --dashboard  # com dashboard embutido
```

**Done:** ✅
- Normalização de payloads (Shape A e B) para `NormalizedMessage`
- Extração de preços (final + original), cupons, modifiers (frete, pagamento, parcelamento)
- Classificação em 11 `MessageType` por cascata de regras
- Deduplicação cross-channel via SHA-256 do URL
- Síntese para `SynthesizedPromotion` (merchant, preços, cupom, URL)
- Batch processing com drain mode (backlog rápido)
- Flag `--dashboard` para visualização

---

## Fases futuras (não implementar agora)

| Fase | O que entra |
|------|-------------|
| 3 | `limiar-processor`: classificação semântica via API LLM em batches, extração de ProductName |
| 4 | `limiar-api`: REST + SSE, conexão com frontend |
| 5 | `limiar-collector channels` evolui para discovery automático |
| 6 | Orquestrador `limiar` com `limiar run` |

---

## O que NÃO é o Limiar

- Não é um bot do Telegram (não responde mensagens, não interage com usuários)
- Não é um scraper via HTTP (usa MTProto, o protocolo nativo do Telegram)
- Não é um sistema de alertas (é um pipeline de dados)
- Não tem interface de admin — configuração é via CLI e arquivo de config

# CONTEXT — Visão Geral do Sistema Limiar

## O que o Limiar é

Limiar é um sistema que monitora canais promocionais brasileiros do Telegram através de um
userbot MTProto, e então normaliza, desduplica, classifica e serve um feed JSON padronizado
para um site de promoções em tempo real. Os canais promocionais do Telegram são ricos, mas caóticos:
mensagens duplicadas em vários canais, formatos inconsistentes, sem categorização.
O Limiar transforma esse ruído em dados limpos e estruturados.

O sistema completo é um pipeline de binários Go independentes:

```
Telegram MTProto
       ↓
limiar-collector → conecta via userbot, lê histórico + eventos ao vivo, persiste RAW (bruto)
       ↓
limiar-processor → normaliza, deduplica, classifica, sintetiza
       ↓
limiar-api       → serves REST (paginated history) + SSE (live push) (futuro)
       ↓
Limiar Frontend  → live promotions site                             (futuro)
```

Este repositório implementa o **`limiar-collector` (Fase 1)** e o **`limiar-processor` (Fase 2)**.

## O que este repositório faz

### limiar-collector (Fase 1) — Implementado ✅

O `limiar-collector` conecta-se ao Telegram, autentica-se como um userbot uma vez,
gerencia uma lista de canais monitorados e captura payloads **brutos (raw)** de mensagens como
JSON em um banco de dados embutido Tursogo. A Fase 1 existe para descobrir o
formato real dos dados do Telegram antes que qualquer processamento seja desenhado.

Quatro subcomandos de CLI:

- `auth` — autenticação interativa e idempotente; persiste uma sessão (logs pretty/text).
- `channels` — `list` / `add <username>` / `remove <username>`; requer uma
  sessão (logs pretty/text).
- `run` — serviço coletor não interativo; captura e persiste mensagens
  brutas, desligamento gracioso (graceful shutdown) em `SIGTERM`/`SIGINT` (logs JSON).
  Flag opcional `--dashboard` para embutir o dashboard HTTP.
- `dashboard` — servidor HTTP standalone para inspecionar mensagens capturadas (logs text).

### limiar-processor (Fase 2) — Sendo Implementado

O `limiar-processor` lê `raw_messages` (read-only) do banco compartilhado (`limiar.db`),
normaliza payloads em uma estrutura canônica (Shape A e B), classifica por tipo de
mensagem (11 tipos), detecta preços/cupons/merchants, realiza deduplicação cross-channel
via URL hash, sintetiza uma estrutura `SynthesizedPromotion`, e persiste em
`processed_messages`.

Funcionalidades implementadas:
- **Normalização:** extração de texto, preços (centavos BRL), cupons, modificadores
  (frete, pagamento, parcelamento), detecção de mídia, urgência.
- **Classificação:** cascata de 11 `MessageType` — `deal_complete`, `deal_no_coupon`,
  `deal_no_price`, `coupon_only`, `category_header`, `coupon_expired`, `commentary`,
  `video`, `document`, `poll`, `admin_meta`.
- **Deduplicação:** cross-channel via SHA-256 da primeira URL normalizada.
- **Síntese:** `SynthesizedPromotion` com merchant, preços, cupom, URL.
- **Batch processing:** poll periódico com drain mode (backlog rápido).
- Flag opcional `--dashboard` para embutir o dashboard HTTP.

## O que o Limiar NÃO é

- Não é um bot do Telegram — ele nunca responde a mensagens ou interage com usuários.
- Não é um scraper HTTP — ele fala o MTProto nativo.
- Não é um sistema de alerta — ele é um pipeline de dados.
- Não é uma interface de administração (admin UI) — a configuração é via CLI e variáveis de ambiente.

E, especificamente, **não faz (ainda)**:

- classificação semântica via LLM (Fase 3);
- extração de `ProductName` (requer LLM);
- REST API ou SSE para frontend (Fase 4);
- discovery automático de canais (Fase 5);
- orquestrador `limiar run` (Fase 6).

## Fluxo de dados (Collector — Fase 1)

```
Telegram (MTProto)
   │  atualizações (updates) / histórico
   ▼
telegram.Client (facade gotd/td)
   │  encodeUpdate → JSON []byte
   ▼
telegram.Dispatcher (Observer, fan-out, uma goroutine por handler)
   │  HandleUpdate(ctx, update []byte)
   ▼
collector.MessageHandler (Adapter: []byte → storage.RawMessage)
   │  Classifier.Classify (NoopClassifier pass-through)
   │  writeCh <- WriteJob
   ▼
collector.Collector.dbWriter (única goroutine, fan-in)
   │  Repository.SaveRawMessage + UpdateChannelLastMessage
   ▼
banco de dados Tursogo (./limiar.db): raw_messages, channels, peers, sessions
```

## Fluxo de dados (Processor — Fase 2)

```
Tursogo (./limiar.db): raw_messages (read-only)
   │  FetchUnprocessed (batch poll)
   ▼
processor.Normalize (Shape A/B → NormalizedMessage)
   │  extrai texto, preços, cupons, mídia, URLs, merchants
   ▼
processor.Classify (cascata de regras → MessageType)
   │  11 tipos exclusivos
   ▼
processor.markDuplicates (dedup cross-channel via URL hash)
   ▼
processor.Synthesize (NormalizedMessage → SynthesizedPromotion)
   ▼
storage.ProcessorRepository.SaveProcessedBatch (transação única)
   ▼
Tursogo (./limiar.db): processed_messages
```

O estado da sessão e os hashes de acesso de peers são persistidos no mesmo banco de dados, para que o
coletor (collector) se reconecte sem re-autenticar e resolva canais que já viu
anteriormente.

## Roadmap das fases

| Fase | Binário | O que adiciona | Status |
|------|---------|----------------|--------|
| **1** | `limiar-collector` | autenticação do userbot, gerenciamento de canais, captura bruta (raw), dashboard | ✅ Implementado |
| **2** | `limiar-processor` | normalização, dedup cross-channel, classificação por regras, síntese | ✅ Implementado |
| 3 | `limiar-processor` | classificação semântica via API de LLM em lote, extração de ProductName | Futuro |
| 4 | `limiar-api` | REST + SSE, integração com o frontend | Futuro |
| 5 | `limiar-collector` | descoberta automática de canais | Futuro |
| **6** | `limiar` | Orquestrador (`limiar run`) roda todos os componentes no mesmo processo | ✅ Implementado |

O design planta "costuras" (seams) para fases posteriores sem implementá-las: a
interface Strategy `Classifier` (atualmente `NoopClassifier` no collector), o Observer
`Dispatcher` (atualmente com um handler), e um esquema (schema) estável de
payload bruto (`schema_version = 1`).

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
limiar-processor → normalizes, deduplicates, classifies, filters   (futuro)
       ↓
limiar-api       → serves REST (paginated history) + SSE (live push) (futuro)
       ↓
Limiar Frontend  → live promotions site                             (futuro)
```

Este repositório implementa **apenas o `limiar-collector` (Fase 1)**.

## O que este repositório (Fase 1) faz

O `limiar-collector` conecta-se ao Telegram, autentica-se como um userbot uma vez,
gerencia uma lista de canais monitorados e captura payloads **brutos (raw)** de mensagens como
JSON em um banco de dados embutido Tursogo. A Fase 1 existe para descobrir o
formato real dos dados do Telegram antes que qualquer processamento seja desenhado.

Três subcomandos de CLI:

- `auth` — autenticação interativa e idempotente; persiste uma sessão (logs de texto).
- `channels` — `list` / `add <username>` / `remove <username>`; requer uma
  sessão (logs de texto).
- `run` — serviço coletor não interativo; captura e persiste mensagens
  brutas, desligamento gracioso (graceful shutdown) em `SIGTERM`/`SIGINT` (logs JSON).

## O que o Limiar NÃO é

- Não é um bot do Telegram — ele nunca responde a mensagens ou interage com usuários.
- Não é um scraper HTTP — ele fala o MTProto nativo.
- Não é um sistema de alerta — ele é um pipeline de dados.
- Não é uma interface de administração (admin UI) — a configuração é via CLI e variáveis de ambiente.

E, especificamente, **a Fase 1 NÃO é**:

- um processador (sem normalização, enriquecimento, classificação semântica, chamadas LLM);
- um desduplicador além da persistência segura (a restrição `UNIQUE(channel_id, message_id)`
  mais `ON CONFLICT DO NOTHING`);
- um serviço HTTP (sem REST, SSE, WebSocket, health checks ou métricas). *(Nota: na revisão final foi adicionado um dashboard HTTP apenas para visualização local, mas não é um serviço público).*

## Fluxo de dados (Fase 1)

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

O estado da sessão e os hashes de acesso de peers são persistidos no mesmo banco de dados, para que o
coletor (collector) se reconecte sem re-autenticar e resolva canais que já viu
anteriormente.

## Roadmap das fases

| Fase | Binário | O que adiciona |
|-------|--------|--------------|
| **1 (este repo)** | `limiar-collector` | autenticação do userbot, gerenciamento de canais, captura bruta (raw) |
| 2 | `limiar-processor` | normalização, dedup, classificação baseada em regras |
| 3 | `limiar-processor` | classificação semântica via API de LLM em lote |
| 4 | `limiar-api` | REST + SSE, integração com o frontend |
| 5 | `limiar-collector` | descoberta automática de canais |
| 6 | `limiar` | orquestrador que inicia todos os binários (`limiar run`) |

O design planta "costuras" (seams) para fases posteriores sem implementá-las: a
interface Strategy `Classifier` (atualmente `NoopClassifier`), o Observer
`Dispatcher` (atualmente com um handler) e um esquema (schema) estável de
payload bruto (`schema_version = 1`).

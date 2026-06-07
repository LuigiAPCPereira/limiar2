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
limiar-processor → normaliza, deduplica, classifica, filtra
       ↓
limiar-api → serve REST (histórico paginado) + SSE (push em tempo real)
       ↓
Limiar Frontend → site de promoções ao vivo
```

Cada componente é um binário Go independente com lifecycle próprio. Um orquestrador `limiar` futuro fará o spawn dos três.

---

## Binários

| Binário | Responsabilidade |
|---------|------------------|
| `limiar-collector` | MTProto userbot: autentica, monitora canais, persiste mensagens brutas |
| `limiar-processor` | Lê do banco, normaliza, deduplica, classifica, filtra |
| `limiar-api` | Expõe REST + SSE para o frontend |
| `limiar` (futuro) | Orquestrador: sobe os três binários com `limiar run` |

---

## Stack

| Camada | Tecnologia |
|--------|-----------|
| MTProto | `gotd/td` — sem wrapper, implementação própria de session/peer storage |
| Banco de dados | `turso.tech/database/tursogo` — embedded, arquivo `.db`, sem CGO, driver `"turso"` para `database/sql` |
| CLI | Cobra + Viper |
| Logging | Interface `Logger` modular, implementação padrão com `slog` da stdlib |
| HTTP | `chi` |
| Push realtime | SSE (Server-Sent Events) |
| Classificação básica | Regras e regex (Strategy pattern, interface `Classifier`) |
| Classificação semântica | API LLM externa em batches (Adapter sobre a interface `Classifier`) |

---

## Decisões de design relevantes

**Sem GoTGProto:** a biblioteca acopla session e peer storage ao GORM/SQLite via struct concreta não plugável. O Limiar usa Tursogo como banco único — por isso implementa a camada MTProto diretamente sobre `gotd/td`.

**Sem CGO:** `tursogo` usa `purego` para FFI. Nenhuma variável de ambiente especial necessária, binário portável.

**Tursogo como único banco:** sessão MTProto, peer cache, canais monitorados, mensagens brutas e histórico de deduplicação vivem todos no mesmo arquivo `.db`. Em produção, o arquivo é montado como volume persistente.

**Logging modular:** interface `Logger` injetada via construtor em todas as camadas. Implementação padrão usa `slog`. Formato JSON em serviços (`run`), texto em comandos interativos (`auth`, `channels`).

**Observer para updates:** updates do Telegram disparam para múltiplos handlers via channels do Go. Prepara o pipeline para receber novos consumers sem acoplamento.

**Strategy para classificação:** interface `Classifier` plugável desde a fase 1. `NoopClassifier` em produção agora, `RuleClassifier` e `LLMClassifier` nas próximas fases.

**Facade sobre gotd/td:** toda a complexidade MTProto (session, peers, reconexão, auth flow) fica atrás de uma interface `TelegramClient` limpa.

---

## Fase atual — limiar-collector (Fase 1)

**Objetivo:** conectar ao Telegram, autenticar como userbot, monitorar canais configurados e persistir mensagens brutas em JSON no Tursogo. Zero processamento — esta fase existe para descobrir o shape real dos dados entregues pelo Telegram.

**Comandos CLI:**

```
limiar-collector auth       # autentica e persiste sessão (interativo, roda uma vez)
limiar-collector channels   # list / add / remove canais monitorados
limiar-collector run        # sobe o serviço de coleta (não-interativo, production-ready)
```

**Done quando:**
- `auth` persiste sessão válida no Tursogo
- `channels` gerencia a lista de canais no banco
- `run` recebe mensagens e salva o payload JSON bruto com log estruturado
- Graceful shutdown em SIGTERM/SIGINT

---

## Fases futuras (não implementar agora)

| Fase | O que entra |
|------|-------------|
| 2 | `limiar-processor`: normalização, deduplicação com Tursogo, classificação por regras |
| 3 | `limiar-processor`: classificação semântica via API LLM em batches |
| 4 | `limiar-api`: REST + SSE, conexão com frontend |
| 5 | `limiar-collector channels` evolui para discovery automático |
| 6 | Orquestrador `limiar` com `limiar run` |

---

## O que NÃO é o Limiar

- Não é um bot do Telegram (não responde mensagens, não interage com usuários)
- Não é um scraper via HTTP (usa MTProto, o protocolo nativo do Telegram)
- Não é um sistema de alertas (é um pipeline de dados)
- Não tem interface de admin — configuração é via CLI e arquivo de config

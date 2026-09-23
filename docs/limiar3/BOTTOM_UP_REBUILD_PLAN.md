# Limiar 3.0 — reconstrução bottom-up com separação de authorities

**Estado:** proposta operacional derivada do rebaseline e das direções explícitas do mantenedor. Não é ADR Accepted, não fixa nomes finais de packages e não autoriza implementação automaticamente.

Fontes: [`REBASELINE_INHERITANCE.md`](REBASELINE_INHERITANCE.md), [`DETACHABLE_BOUNDARIES.md`](DETACHABLE_BOUNDARIES.md) e [`PRODUCT_AND_SCOPE.md`](PRODUCT_AND_SCOPE.md).

## 1. Objetivo

Construir o Limiar 3 por authorities explícitas, com ownership, lifecycle e failure domain claros. A separação é semântica antes de ser física: mesmo processo, binário ou SQLite não tornam duas responsabilidades a mesma authority.

**Desacoplável não significa distribuído.**

## 2. Regras estruturais

1. Sem repository global; capabilities estreitas antes de `*sql.DB` compartilhado.
2. gotd/MTProto ficam dentro do adapter Telegram; detalhes da biblioteca não vazam por conveniência para o core.
3. A credencial/autorização Telegram é authority do boundary Telegram; não confundir com MTProto `session_id`. Collector e MCP não são owners.
4. MCP é adapter removível/separável; não acessa SQLite diretamente e não contém regra comercial canônica.
5. MCP realtime consulta Telegram diretamente através da capability Telegram, não através do storage do Limiar.
6. Consulta MCP realtime não vira Source Evidence automaticamente; persistência durável passa por Source Admission.
7. Sem goroutine por camada, event bus ou RPC por ritual.
8. Sem provider framework prematuro; Telegram é a fonte concreta atual.
9. Projection persistida deve ser reconstruível/versionada.
10. Segredo fica fora do Evidence DB por default; misturá-los exige Decision própria.
11. UNKNOWN é legítimo; processing não fabrica certeza.
12. Legado entra somente por adapters de comparação/importação.

## 3. Camadas e ramos de construção

### B0 — Runtime mínimo

Composition root pequeno, config tipada/fail-closed, paths explícitos, logging sem segredos, shutdown/cancelamento, health/status e build metadata.

### B1 — Telegram Authorization Credential Boundary

**Authority:** credencial/autorização Telegram persistida e lifecycle de segurança. `session.Storage` permanece privado ao adapter; MTProto `session_id` é detalhe efêmero do gotd.

```text
LoadSession(ctx) -> bytes | not-found | error
StoreSession(ctx, bytes) -> success | error
```

Separado de peer cache e, por default, do SQLite de Evidence. ADR 023 continua Proposed.

### B2 — TelegramRuntime + restore/reuse real

Compor um owner único por `TelegramAuthorizationIdentity`, possuindo o main `gotd/telegram.Client`, storage privado, auth/readiness lifecycle e peer internals.

**Antes de estabilizar uma capability**, provar com gotd real na versão fixada: bootstrap controlado -> StoreSession -> shutdown -> restart -> LoadSession -> authorized -> query read-only sem novo OTP.

Não criar clients principais independentes por MCP/collector sobre a mesma autorização. Não criar retry genérico nem auto-login no steady state.

### B3 — Telegram Query Capability

Primeiro contract source-aware e read-only: resolver peer e consultar history bounded/paginado. `RecentMessages` pode ser apenas a primeira página de history.

Não expor `tg.*`, `InputPeer`, access hashes, bytes de sessão ou primitivas MTProto. Peer state é authority separada, porém authorization-scoped quando persistida.

### B4 — MCP Telegram realtime — exploração precoce

Permitir ao ChatGPT consultar Telegram em tempo real, sob demanda, **antes de existir modelo comercial**.

```text
ChatGPT -> MCP adapter -> autorização -> Telegram realtime capability -> Telegram
```

O MCP deve ser detachable: nenhuma dependência de schema SQLite, processing ou frontend. Resposta realtime não é automaticamente Source Evidence.

### B5 — Storage Kernel de Evidence

Reaproveitar contratos ADR 019/020 e avaliar `internal/storage/sqlite` como capability, sem carregar Tursogo. Não colocar aqui SourceSyncState, BackfillProgress, session, peer cache, projection ou índices de feed.

### B6 — Acquisition Subscription + Source Admission

```text
Telegram envelope + AcquisitionSubscription
        -> admission classification
        -> AppendEvidence
        -> sucesso
        -> forward ao recovery/manager
```

`subscription_id` nunca nasce de `channel_id` por conveniência. ADR 024 continua Proposed.

### B7 — Live Recovery / SourceSyncState

Compor `updates.Manager`, GuardedRecoveryAPI, DurabilityBarrier, Supervisor e GuardedStateStorage. ADR 021 continua Proposed.

```text
Evidence durável + state antigo = seguro/replay
Evidence ausente + state novo   = proibido
```

### B8 — Backfill

**Authority:** cobertura/progresso histórico; nunca live sync. ADR 022 continua Proposed.

```text
history page -> Evidence necessária durável -> BackfillProgress pode avançar
```

### B9 — Peer Cache e Media

Peer cache é estado operacional reconstruível; Media resolve referência e obtém asset sem fazer da imagem a fonte da mensagem. Reproduzir a falha histórica de imagens antes de alegar correção.

### B10 — Source Projection

Estado derivado reconstruível da Evidence: current message view, edit/delete/replay e provenance para a Evidence sustentadora.

### B11 — Domain Discovery Loop

Antes de congelar dados de promoção, usar o MCP realtime para investigar corpus real e registrar Findings sobre padrões, exceções e UNKNOWN.

Perguntas típicas: formatos de preço, cupom, Pix, cashback, frete, bundles, múltiplos produtos, diferenças por canal/merchant, mensagens sem preço, ambiguidades de identidade e mídia.

**Findings de exploração orientam schema/processamento, mas não se tornam Decision automaticamente.**

### B12 — Deterministic Findings

Extrair fatos observáveis e tipados: texto normalizado, URLs, merchant/source metadata, dinheiro sem `float64`, coupon candidates, códigos explícitos, disponibilidade e condições. Cada Finding mantém provenance.

### B13 — Promotion Interpretation

Interpretar facetas, não enum exclusivo: preço observado, preço anterior alegado, cupom, cashback, frete, bundle, restrições, confiança e origem. IA, se usada, permanece lateral, versionada e auditável.

### B14 — Commercial Model

Separar `Product`, `Merchant Listing`, `Offer Observation`, `Relations` e `Feed Projection`. Feed nunca define identidade.

### B15 — Limiar Query Service

Capability de leitura dos dados produzidos pelo Limiar. Não é o caminho usado pelo MCP realtime Telegram.

### B16 — MCP Limiar data + HTTP API

```text
MCP Telegram realtime -> Telegram capability
MCP Limiar data       -> Limiar Query Service
HTTP API              -> Limiar Query Service
```

### B17 — Frontend

Experiência sobre contratos reais da API: feed, busca/filtros, agrupamento, comparação, provenance e estados operacionais.

### B18 — Migração e cutover

Somente depois do novo caminho funcionar side-by-side: caracterizar legado, executar EXP-007/sucessor em cópia histórica real descartável, comparar, validar backup/restore/rollback, medir performance, migrar por fatia e desativar só com substituto comprovado e autorização.

## 4. Grafo de authorities

```text
Session Credential
       |
       v
Telegram / MTProto Adapter
   |                  \
   v                   v
MCP realtime        Collector / Source Admission
                         |
                         v
                      Evidence
                    /          \
                   v            v
            Live Recovery     Backfill
                   \            /
                    v          v
                   Source Projection
                         |
                         +-------> Media
                         |
                         v
                Deterministic Findings
                         |
                         v
               Promotion Interpretation
                         |
                         v
             Product/Listing/Offer/Relations
                         |
                         v
                  Limiar Query Service
                    /             \
                   v               v
            MCP Limiar data      HTTP API
                                      |
                                      v
                                   Frontend
```

**Loop de descoberta:** MCP realtime consulta Telegram e produz material para investigação/Findings antes e durante B11–B13. Ele não escreve Evidence por atalho.

## 5. Shape de código apenas ilustrativo

```text
cmd/limiar3/
internal/l3/config/
internal/l3/session/
internal/l3/telegram/
internal/l3/mcp/
internal/l3/evidence/
internal/l3/acquisition/
internal/l3/recovery/
internal/l3/backfill/
internal/l3/peers/
internal/l3/media/
internal/l3/projection/
internal/l3/findings/
internal/l3/promotions/
internal/l3/catalog/
internal/l3/query/
internal/l3/httpapi/
```

Isso é mapa de responsibilities, não decisão de packages.

## 6. Slices para evitar big rewrite

- **S1 — authorization storage/runtime:** config mínima + private session storage + owner único + bootstrap/steady-state separados.
- **S2 — gotd restore/reuse:** provar restart real, authorization status e query sem novo OTP.
- **S3 — TelegramQuery:** `ResolvePeer` + `History` bounded com types source-aware e erros semânticos.
- **S4 — MCP realtime:** primeira tool read-only consultando Telegram diretamente.
- **S5 — Evidence:** SQLite + append/round-trip sintético, independente do MCP.
- **S6 — admission:** update -> append -> somente então forward.
- **S7 — recovery:** manager/state/barrier + restart/replay.
- **S8 — history:** backfill independente escrevendo Evidence.
- **S9 — projection:** Evidence -> current message state.
- **S10 — mídia:** referências/assets verificáveis.
- **S11 — descoberta do domínio:** usar MCP realtime para amostrar casos e registrar Findings.
- **S12 — primeiro Finding determinístico:** regra simples com provenance.
- **S13 — primeira oferta consultável:** Findings -> Offer Observation -> Query Service.
- **S14 — MCP Limiar data:** primeira tool sobre Query Service.
- **S15 — primeira rota API + tela:** mesma capability, sem regra duplicada.

Depois de S3, MCP realtime e o ramo Evidence/admission/recovery podem evoluir em paralelo. O modelo comercial não deve ser congelado antes do loop de descoberta do domínio.

## 7. Decisões ainda necessárias

1. Sessão: revisar/aceitar/rejeitar ADR 023 ou substituto.
2. Subscription identity: resolver ADR 024 antes de Source Admission produtiva.
3. SourceSyncState: aceitar/revisar ADR 021 antes do schema produtivo.
4. BackfillProgress: aceitar/revisar ADR 022 antes do schema produtivo.
5. Boundary concreto Telegram: consolidar `TelegramAuthorizationIdentity`, owner único do `telegram.Client`, bootstrap explícito e `TelegramQuery` mínima sem abstração excessiva.
6. Auth/autorização MCP realtime: definir scopes quando a primeira tool real for implementada.
7. Media: investigar boundary e bug real antes de Decision estrutural.
8. Processing generations/projections: Decision quando storage derivado exigir.

## 8. Fundação pronta

A fundação precisa provar sessão sob contrato aceito, Telegram capability desacoplada, Source Evidence sob ADR 016–020, ordering seguro, restart/replay, separação live/backfill/session/peer e primeira projection reconstruível. O MCP realtime pode estar funcional antes disso como ramo exploratório, mas sua existência não certifica durabilidade do collector.


## 9. Findings L3-001A que condicionam etapas futuras

- `updates.Manager` não certifica sozinho Evidence-before-progress: live admission vem antes do Manager; recovery RPCs e state storage precisam guards + supervisor fail-stop.
- History/backfill e update recovery são authorities diferentes e não compartilham cursor semântico.
- Peer/access-hash state é separado da sessão, mas authorization-scoped.
- FLOOD_WAIT/reconnect/migration/downloader têm políticas diferentes; não criar retry framework genérico.
- Media mantém file reference/location internamente; futura capability deve usar referência source-aware à mensagem e re-resolver quando necessário.

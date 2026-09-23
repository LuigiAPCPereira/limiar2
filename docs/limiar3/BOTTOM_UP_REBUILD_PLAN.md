# Limiar 3.0 — reconstrução bottom-up com separação de authorities

**Estado:** proposta operacional derivada do rebaseline e das direções explícitas do mantenedor. Não é ADR Accepted, não fixa nomes finais de packages e não autoriza implementação automaticamente.

Fontes: [`REBASELINE_INHERITANCE.md`](REBASELINE_INHERITANCE.md), [`DETACHABLE_BOUNDARIES.md`](DETACHABLE_BOUNDARIES.md) e [`PRODUCT_AND_SCOPE.md`](PRODUCT_AND_SCOPE.md).

## 1. Objetivo

Construir o Limiar 3 por authorities explícitas, com ownership, lifecycle e failure domain claros. A separação é semântica antes de ser física: mesmo processo, binário ou SQLite não tornam duas responsabilidades a mesma authority.

**Desacoplável não significa distribuído.**

## 2. Regras estruturais

1. Sem repository global; capabilities estreitas antes de `*sql.DB` compartilhado.
2. gotd/MTProto ficam dentro do adapter Telegram; detalhes da biblioteca não vazam por conveniência para o core.
3. Sessão MTProto é authority de credencial do boundary Telegram, não do collector nem do MCP.
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

### B1 — Session Credential Boundary

**Authority:** bytes da sessão MTProto e lifecycle de segurança.

```text
LoadSession(ctx) -> bytes | not-found | error
StoreSession(ctx, bytes) -> success | error
```

Separado de peer cache e, por default, do SQLite de Evidence. ADR 023 continua Proposed.

### B2 — Telegram / MTProto Capability Boundary

Encapsular gotd/MTProto e oferecer capabilities estreitas: realtime/history query, update/recovery stream, peer resolution e media retrieval quando necessárias.

Collector e MCP podem consumir capacidades diferentes do mesmo boundary sem compartilhar lifecycle. Não decidir antecipadamente uma ou várias instâncias de client nem processos.

### B3 — MCP Telegram realtime — exploração precoce

Permitir ao ChatGPT consultar Telegram em tempo real, sob demanda, **antes de existir modelo comercial**.

```text
ChatGPT -> MCP adapter -> autorização -> Telegram realtime capability -> Telegram
```

O MCP deve ser detachable: nenhuma dependência de schema SQLite, processing ou frontend. Resposta realtime não é automaticamente Source Evidence.

### B4 — Storage Kernel de Evidence

Reaproveitar contratos ADR 019/020 e avaliar `internal/storage/sqlite` como capability, sem carregar Tursogo. Não colocar aqui SourceSyncState, BackfillProgress, session, peer cache, projection ou índices de feed.

### B5 — Acquisition Subscription + Source Admission

```text
Telegram envelope + AcquisitionSubscription
        -> admission classification
        -> AppendEvidence
        -> sucesso
        -> forward ao recovery/manager
```

`subscription_id` nunca nasce de `channel_id` por conveniência. ADR 024 continua Proposed.

### B6 — Live Recovery / SourceSyncState

Compor `updates.Manager`, GuardedRecoveryAPI, DurabilityBarrier, Supervisor e GuardedStateStorage. ADR 021 continua Proposed.

```text
Evidence durável + state antigo = seguro/replay
Evidence ausente + state novo   = proibido
```

### B7 — Backfill

**Authority:** cobertura/progresso histórico; nunca live sync. ADR 022 continua Proposed.

```text
history page -> Evidence necessária durável -> BackfillProgress pode avançar
```

### B8 — Peer Cache e Media

Peer cache é estado operacional reconstruível; Media resolve referência e obtém asset sem fazer da imagem a fonte da mensagem. Reproduzir a falha histórica de imagens antes de alegar correção.

### B9 — Source Projection

Estado derivado reconstruível da Evidence: current message view, edit/delete/replay e provenance para a Evidence sustentadora.

### B10 — Domain Discovery Loop

Antes de congelar dados de promoção, usar o MCP realtime para investigar corpus real e registrar Findings sobre padrões, exceções e UNKNOWN.

Perguntas típicas: formatos de preço, cupom, Pix, cashback, frete, bundles, múltiplos produtos, diferenças por canal/merchant, mensagens sem preço, ambiguidades de identidade e mídia.

**Findings de exploração orientam schema/processamento, mas não se tornam Decision automaticamente.**

### B11 — Deterministic Findings

Extrair fatos observáveis e tipados: texto normalizado, URLs, merchant/source metadata, dinheiro sem `float64`, coupon candidates, códigos explícitos, disponibilidade e condições. Cada Finding mantém provenance.

### B12 — Promotion Interpretation

Interpretar facetas, não enum exclusivo: preço observado, preço anterior alegado, cupom, cashback, frete, bundle, restrições, confiança e origem. IA, se usada, permanece lateral, versionada e auditável.

### B13 — Commercial Model

Separar `Product`, `Merchant Listing`, `Offer Observation`, `Relations` e `Feed Projection`. Feed nunca define identidade.

### B14 — Limiar Query Service

Capability de leitura dos dados produzidos pelo Limiar. Não é o caminho usado pelo MCP realtime Telegram.

### B15 — MCP Limiar data + HTTP API

```text
MCP Telegram realtime -> Telegram capability
MCP Limiar data       -> Limiar Query Service
HTTP API              -> Limiar Query Service
```

### B16 — Frontend

Experiência sobre contratos reais da API: feed, busca/filtros, agrupamento, comparação, provenance e estados operacionais.

### B17 — Migração e cutover

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

- **S1 — sessão:** config mínima + session boundary + restart.
- **S2 — Telegram capability:** encapsular gotd e provar uma operação realtime mínima.
- **S3 — MCP realtime:** primeira tool read-only consultando Telegram diretamente.
- **S4 — Evidence:** SQLite + append/round-trip sintético, independente do MCP.
- **S5 — admission:** update -> append -> somente então forward.
- **S6 — recovery:** manager/state/barrier + restart/replay.
- **S7 — history:** backfill independente escrevendo Evidence.
- **S8 — projection:** Evidence -> current message state.
- **S9 — mídia:** referências/assets verificáveis.
- **S10 — descoberta do domínio:** usar MCP realtime para amostrar casos e registrar Findings.
- **S11 — primeiro Finding determinístico:** regra simples com provenance.
- **S12 — primeira oferta consultável:** Findings -> Offer Observation -> Query Service.
- **S13 — MCP Limiar data:** primeira tool sobre Query Service.
- **S14 — primeira rota API + tela:** mesma capability, sem regra duplicada.

Os ramos S3 e S4–S9 podem evoluir em paralelo depois de S2; o modelo comercial não deve ser congelado antes do loop S10.

## 7. Decisões ainda necessárias

1. Sessão: revisar/aceitar/rejeitar ADR 023 ou substituto.
2. Subscription identity: resolver ADR 024 antes de Source Admission produtiva.
3. SourceSyncState: aceitar/revisar ADR 021 antes do schema produtivo.
4. BackfillProgress: aceitar/revisar ADR 022 antes do schema produtivo.
5. Boundary concreto Telegram capabilities: manter gotd encapsulado sem abstração excessiva.
6. Auth/autorização MCP realtime: definir scopes quando a primeira tool real for implementada.
7. Media: investigar boundary e bug real antes de Decision estrutural.
8. Processing generations/projections: Decision quando storage derivado exigir.

## 8. Fundação pronta

A fundação precisa provar sessão sob contrato aceito, Telegram capability desacoplada, Source Evidence sob ADR 016–020, ordering seguro, restart/replay, separação live/backfill/session/peer e primeira projection reconstruível. O MCP realtime pode estar funcional antes disso como ramo exploratório, mas sua existência não certifica durabilidade do collector.

# Limiar 3.0 — reconstrução bottom-up com separação de authorities

**Estado:** proposta operacional derivada do rebaseline. Não é ADR Accepted, não fixa nomes finais de packages e não autoriza implementação automaticamente.

Fonte de herança: [`REBASELINE_INHERITANCE.md`](REBASELINE_INHERITANCE.md). Produto: [`PRODUCT_AND_SCOPE.md`](PRODUCT_AND_SCOPE.md).

## 1. Objetivo

Cada camada deve ter authority, ownership, lifecycle e failure domain explícitos; entrada/saída testáveis; persistência somente quando necessária; estado derivado reconstruível; e dependências apontando para capabilities inferiores, não para UI ou topologia.

A separação é semântica antes de ser física. Mesmo processo ou mesmo SQLite não tornam duas responsabilidades a mesma authority.

## 2. Regras estruturais

1. **Sem repository global.** Não expor `*sql.DB` quando capability estreita resolve.
2. **Sem package de domínio global por conveniência.** Compartilhar tipos apenas por contrato real.
3. **Sem goroutine por camada.** Concorrência é mecanismo, não arquitetura.
4. **Sem event bus obrigatório no início.** Chamada direta é preferível enquanto suficiente.
5. **Sem provider framework prematuro.** Telegram é a fonte real atual.
6. **Sem segunda verdade derivada.** Projection persistida deve ter generation/version e ser reconstruível.
7. **Segredo fora do Evidence DB por default.** Misturar sessão e backup de Evidence exige Decision própria.
8. **MCP/API/frontend são adapters de consulta.** Não reescrevem Evidence nem regras canônicas.
9. **UNKNOWN é legítimo.** Processing não fabrica certeza.
10. **Legado só por adapter de comparação/importação.** Core novo não depende do legado por conveniência.

## 3. Camadas de construção

### B0 — Runtime mínimo

**Responsabilidade:** execução previsível antes do Telegram.

Inclui composition root pequeno, config tipada/fail-closed, paths explícitos, logging sem segredos, shutdown/cancelamento, health/status e build metadata.

**Não inclui:** Telegram, SQL de domínio, parsing comercial ou MCP.

**Gate:** startup sintético; config inválida rejeitada; shutdown testado; nenhum segredo em log.

### B1 — Session Credential Boundary

**Authority:** bytes da sessão MTProto e lifecycle de segurança.

Contrato conceitual:

```text
LoadSession(ctx) -> bytes | not-found | error
StoreSession(ctx, bytes) -> success | error
```

Separado de peer cache e, por default, do SQLite de Evidence. Writers que compartilham credencial precisam de coordenação explícita.

**Gate de decisão:** ADR 023 segue Proposed. L3-001 precisa investigar/decidir antes da produção.

**Gate de implementação:** novo/preexistente/permissivo/corrompido, cancelamento, restart, concorrência relevante, race e smoke gotd em ambiente autorizado.

### B2 — Storage Kernel de Evidence

**Authority:** Source Evidence durável.

Reaproveitar contratos ADR 019/020 e avaliar o `internal/storage/sqlite` existente como capability, sem carregar Tursogo.

Capabilities iniciais:

```text
Open / Close
AppendEvidence
ReadEvidenceByID
schema/integrity checks necessários ao boundary
```

Não colocar aqui SourceSyncState, BackfillProgress, session, peer cache, projection ou índices de feed.

**Gate:** migrations/reopen, byte-preserving, hash, append-only, schema guard, crash/restart aplicável e backup/restore revalidado quando entrar no caminho L3.

### B3 — Acquisition Subscription + Source Admission

Authorities distintas:

- configuração/identidade de aquisição;
- Source Admission;
- Evidence Store.

```text
Telegram envelope
 + AcquisitionSubscription
        ↓
admission classification
        ↓
AppendEvidence
        ↓ sucesso
forward ao recovery/manager
```

`subscription_id` nunca nasce de `channel_id` por conveniência.

**Gate:** resolver ADR 024 se materializar identity; missing/ambiguous config falha fechada; append failure => no forward.

### B4 — Live Recovery / SourceSyncState

**Authority:** continuidade live Telegram.

Compor `updates.Manager`, GuardedRecoveryAPI, DurabilityBarrier, Supervisor e GuardedStateStorage. ADR 021 continua Proposed para o storage físico.

```text
Evidence durável + state antigo = seguro/replay
Evidence ausente + state novo   = proibido
```

**Gate:** crash/restart, state read/write failure, too-long, reset explícito, replay, race e integração gotd real.

### B5 — Backfill

**Authority:** cobertura/progresso histórico; nunca live sync.

```text
history page
   ↓
Evidence necessária durável
   ↓
BackfillProgress pode avançar
```

ADR 022 continua Proposed.

**Gate:** ausência ≠ zero; regressão rejeitada; completed não significa continuidade live; cancelamento/rate limit/restart; coexistência com live.

### B6 — Peer Cache e Media

**Peer cache:** estado operacional reconstruível para access hashes/peers; não session/subscription truth.

**Media:** resolver referência e obter asset sem fazer da imagem a fonte da mensagem.

```text
Evidence/message projection
 -> MediaReference
 -> download idempotente
 -> verificação
 -> MediaAsset/cache
```

**Gate:** reproduzir a falha histórica de imagens antes de alegar correção; cobrir álbum, referência expirada, partial download, retry, duplicate e restart.

### B7 — Source Projection

Estado derivado reconstruível da Evidence. Primeira projection útil: current message view por SourceMessageKey, edit/delete/replay e referência à Evidence que sustenta o estado atual.

**Gate:** rebuild do zero equivalente; update composto; generation nova sem destruir Evidence antiga.

### B8 — Deterministic Findings

Extrair fatos antes de interpretação probabilística: texto normalizado, URLs, merchant/source metadata, dinheiro sem `float64`, coupon candidates, códigos explícitos, disponibilidade e condições observáveis.

Cada Finding mantém provenance.

**Gate:** fixtures sintéticas/anonimizadas, UNKNOWN preservado, parser versionado e reprocessável.

### B9 — Promotion Interpretation

Interpretar promoção como facetas, não enum exclusivo: preço observado, preço anterior alegado, cupom, cashback, frete, bundle, restrições, confiança e origem.

IA, se usada, é lateral e produz resultado probabilístico versionado/auditável. Falha de IA não bloqueia o núcleo determinístico.

### B10 — Commercial Model

Separar:

```text
Product
Merchant Listing
Offer Observation
Relations
Feed Projection
```

Product é identidade conceitual quando sustentada; Listing é item do merchant; Offer Observation é condição observada; Relations registram same-as/variant/possible-duplicate; Feed Projection é apresentação, nunca identidade.

**Gate:** não usar URL afiliada, preço semelhante ou posição de feed como identidade canônica.

### B11 — Query Service

Capability de leitura compartilhada por MCP, API e ferramentas internas.

Pode oferecer, conforme demanda real: buscar mensagens admitidas; provenance; pesquisar ofertas; listar ofertas por produto/listing; comparar observações; recuperar mídia.

Query não muta Source Evidence.

**Gate:** paginação/limites, ausência/stale/generation explícitos e autorização antes de retornar conteúdo privado.

### B12 — MCP

Entra em paralelo assim que Query Service tiver uma capability útil.

Primeiro slice:

```text
ChatGPT
 -> MCP adapter
 -> authorization
 -> Query Service
 -> mensagens admitidas + provenance
```

Depois ampliar para ofertas.

MCP nunca recebe sessão MTProto, não acessa SQLite diretamente, não duplica regras comerciais, trata Telegram como dado não confiável e pode falhar sem interromper pipeline.

### B13 — HTTP API

Expõe a mesma Query Service. Separar transport DTO, autorização, aplicação/query e domínio. Não duplicar SQL/regras do MCP.

### B14 — Frontend

Experiência sobre contratos reais da API: feed, busca/filtros, agrupamento, comparação, provenance, estados loading/empty/error/partial/stale e acessibilidade.

Regra de identidade ou cálculo comercial canônico não nasce no browser.

### B15 — Migração e cutover

Somente depois do novo caminho funcionar side-by-side:

1. caracterizar comportamento legado necessário;
2. executar EXP-007 ou sucessor contra cópia histórica real descartável;
3. comparar contagens/proveniência/fixtures;
4. validar backup/restore e rollback;
5. medir performance com workload comparável;
6. migrar consumidores por fatia;
7. manter legado read-only durante a janela necessária;
8. desativar/remover somente após substituto comprovado e autorização.

## 4. Grafo de authorities

```text
Session Credential ───────┐
                          v
Acquisition Config -> Telegram Adapter
                          |
                          v
                   Source Admission
                          |
                          v
                    Evidence Store
                     /          \
                    v            v
             Live Recovery     Backfill
                    \            /
                     v          v
                    Source Projection
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
                    Query Service
                   /      |       \
                  v       v        v
                MCP      API    internal tools
                          |
                          v
                       Frontend
```

Peer cache e Media são side capabilities; não viram authority entre Evidence e live state.

## 5. Shape de código apenas ilustrativo

Não criar packages antes de precisar. Uma forma possível:

```text
cmd/limiar3/
internal/l3/config/
internal/l3/session/
internal/l3/evidence/
internal/l3/acquisition/
internal/l3/telegram/
internal/l3/recovery/
internal/l3/backfill/
internal/l3/peers/
internal/l3/media/
internal/l3/projection/
internal/l3/findings/
internal/l3/promotions/
internal/l3/catalog/
internal/l3/query/
internal/l3/mcp/
internal/l3/httpapi/
```

Se menos packages preservarem ownership com clareza, usar menos.

## 6. Slices para evitar big rewrite

- **S1 — sessão:** config mínima + session boundary + restart, sem collector.
- **S2 — Evidence:** novo SQLite + append/round-trip sintético, sem Telegram.
- **S3 — admission:** adapter/update sintético -> append -> somente então forward.
- **S4 — recovery:** manager/state/barrier + restart/replay.
- **S5 — history:** backfill independente escrevendo a mesma Evidence.
- **S6 — primeira message projection:** Evidence -> current message state.
- **S7 — mídia:** projection -> MediaReference -> asset verificável.
- **S8 — primeiro Finding:** regra determinística simples com provenance.
- **S9 — primeira oferta consultável:** Finding -> Offer Observation -> Query Service.
- **S10 — primeira ferramenta MCP:** read-only sobre Query Service.
- **S11 — primeira rota API + tela:** mesma capability, sem regra duplicada.

Cada slice precisa de build/test/race pertinente e critério observável antes do próximo.

## 7. O que medir

Medir quando a fatia existir: startup/reopen; latência/throughput de append; crescimento do banco; rebuild de projection; backlog/replay; download/cache de mídia; latência de Query Service; latência MCP/API separada do core; CPU/memória sob workload definido; goroutines/conexões quando relevante.

Benchmarks antigos são referência histórica, não prova L3.

## 8. Decisões ainda necessárias

1. Sessão: revisar/aceitar/rejeitar ADR 023 ou substituto.
2. Subscription identity: resolver ADR 024 antes de Source Admission produtiva.
3. SourceSyncState: aceitar/revisar ADR 021 antes do schema produtivo.
4. BackfillProgress: aceitar/revisar ADR 022 antes do schema produtivo.
5. Media: investigar boundary e bug real antes de Decision estrutural.
6. Processing generations/projections: Decision quando o primeiro storage derivado exigir.
7. Auth de consumidor/MCP: decidir quando Query Service existir e o threat model estiver claro.

## 9. Quando a fundação estará pronta

Não quando existir skeleton, mas quando:

- sessão puder persistir/reabrir sob contrato aceito;
- Source Evidence puder ser admitida/persistida sob ADR 016–020;
- falha de Evidence não deixar progresso avançar;
- restart/replay for seguro;
- live/backfill/session/peer permanecerem separados;
- primeira projection for reconstruível;
- segredo não estiver misturado ao backup de Evidence sem decisão;
- testes e observabilidade provarem os failure modes relevantes.

A partir daí, processing e superfícies podem crescer sem reabrir a fundação a cada feature.

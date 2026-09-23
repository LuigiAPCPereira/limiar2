# Limiar 3.0 — proposta arquitetural e plano inicial

**Estado:** PROPOSTA PARA INVESTIGAÇÃO, não desenho final, ADR Accepted, autorização de código ou promessa de arquitetura pronta. Fonte de produto: [`PRODUCT_AND_SCOPE.md`](PRODUCT_AND_SCOPE.md). Regras: `AGENTS.md`, protocolo v2, `docs/adr/README.md` e Accepted. Escopo expandido em conversa pelo mantenedor: reconstrução integral do agrupador de promoções com MCP. O legado não define estrutura obrigatória.

A herança comprovada da Rebaseline 2026 está consolidada em [`REBASELINE_INHERITANCE.md`](REBASELINE_INHERITANCE.md). O princípio de desacoplamento de MTProto/MCP está em [`DETACHABLE_BOUNDARIES.md`](DETACHABLE_BOUNDARIES.md). A construção bottom-up está em [`BOTTOM_UP_REBUILD_PLAN.md`](BOTTOM_UP_REBUILD_PLAN.md). A investigação técnica MTProto/gotd v0.161.0 está em [`L3_001A_MTPROTO_GOTD_INVESTIGATION.md`](L3_001A_MTPROTO_GOTD_INVESTIGATION.md) e refina esta Proposal.

## Alternativas de isolamento — decisão ainda investigável em L3-001

| Alternativa | Benefício | Risco/trade-off | Condição de uso |
| --- | --- | --- | --- |
| Branch isolada apenas | Histórico preservado e CI/PR separados | Reescrever pacotes existentes na própria branch favorece novo entrelaçamento, divergência prolongada | Só se a inspeção provar que a estrutura existente comporta responsabilidades novas sem acoplamento. |
| Diretório novo apenas em `main` | Comparação simultânea simples | Código incompleto pode entrar nos gates/empacotamento da main | Não como isolamento de experimentação sem proteção e escopo claros. |
| **Branch dedicada + implementação nova estruturalmente isolada** | Liberdade para reorganizar e preservar legado, commits/PRs rastreáveis | Conflitos com main e duplicidade prolongada; exigir integrações por fatias e config separada | Hipótese inicial preferida, sujeita à verificação do ambiente e compatibilidade de reutilizar `internal/storage/sqlite`. |
| Outro repositório ou segundo módulo Go | Isolamento maior | Duplica dependências/governança, torna aprendizado e integração mais custosos | Só se requisito comprovado justificar. |

Esta branch `docs/limiar-3-foundation-20260922` é **somente preparação documental**; ser descendente da branch de adoção v2 evita perder entrada/protocolo, mas **não** torna o PR #214 merged nem autoriza a próxima branch de código. Confirmar base real de uma futura branch de implementação e se aplicar cherry-pick/merge documental é seguro/autorizado. Não criar code skeleton prematuro.

## Mapa inicial de ownership — hipóteses verificáveis

| Capacidade | Owner de estado/comportamento | Entrada → saída e limites | Falha que precisa ser prevista |
| --- | --- | --- | --- |
| Configuração/composição | Entrypoint/composition root pequeno, validando config | Parâmetros explícitos → dependências construídas | Config ausente/ambígua falha antes de admissão; nenhum segredo em log. |
| Autorização Telegram + credential storage | `TelegramRuntime` proposto por `TelegramAuthorizationIdentity`; storage privado, separado de peer cache | Credencial persistente ↔ gotd; collector/MCP não recebem bytes nem fazem bootstrap | `AUTH_KEY_DUPLICATED`, blob incompatível/revogado, restart/reuse, shutdown; ADR 023 continua Proposed. |
| Telegram/MTProto | Owner único do main `gotd/telegram.Client`; gotd possui mecânica MTProto/DC/reconnect | Telegram ↔ `TelegramQuery` read-only primeiro; updates/recovery/media entram em slices próprios | Vazamento de `tg.*`/access hash, retry duplicado, logging sensível, concorrência/fairness; desacoplável não implica serviço separado. |
| Peer cache | Adapter Telegram/cache com authority operacional própria, authorization-scoped quando persistida | `PeerKey` ↔ access-hash/internal resolution; access hash não cruza o contract | Cache reconstruível; hashes não universais entre autorizações; não mesclar secret e cache. |
| Admissão de fonte | Boundary de aquisição configurada | Envelope Telegram + subscription configurada → Evidence admitida durável | Append falha → não forward; identity não inferida do canal; ADR 024 Proposed. |
| Evidence | Storage capability SQLite | Payload opaco byte-preserving → Evidence append-only/proveniência | Não sobrescrever, não perder admitidos; integrity e crash; ADRs 019/020 Accepted. |
| Sync/recovery | gotd `updates.Manager` envolvido por Source Admission, GuardedRecoveryAPI, GuardedStateStorage/barrier e supervisor | pts/qts/seq/date/channel pts ↔ sync interno; Evidence durável antes de progress certification | Manager pode logar erros e avançar state em memória; `differenceTooLong`, fail-stop, replay; ADRs 016–018 Accepted; 021 Proposed. |
| Backfill | Worker com estado/progresso distinto | Intervalos históricos → Evidence, avanço seguro | Cancelamento, rate limit, retomar; ADR 022 Proposed. |
| Mídia | Downloader/armazenamento de referências e conteúdo conforme contrato futuro | Mensagem/media IDs → imagem verificável e rastreável | Imagem ausente, download parcial, referência expirada, album, retry idempotente; bug legado relatado sem causa identificada. |
| Processamento/agrupamento | Serviços de domínio/aplicação independentes de Telegram | Evidence/projeções → findings, interpretação, produtos/listings/offers/relations | Duplicidade, preço desconhecido, contradição e reprocessamento; `UNKNOWN` legítimo. |
| Consulta | Serviço de aplicação único ou capabilities estreitas conforme demanda real | Mensagens admitidas, ofertas estruturadas → resultados autorizados | Ausência, dados desatualizados, origem não acessível; não atribuir menor preço do mercado sem pesquisa externa. |
| MCP realtime | Adapter detachable sobre Telegram realtime capability | Pedido ChatGPT autorizado → consulta Telegram sob demanda; não exige Evidence existente | Não virar segunda ingestão; mensagem não confiável; sessão nunca sai; indisponibilidade não bloqueia collector. |
| MCP Limiar data | Adapter detachable sobre Query Service | Pedido ChatGPT autorizado → mensagens admitidas/ofertas/proveniência | Não acessar SQLite nem duplicar regra comercial; pode coexistir com tools realtime. |
| API e frontend | API traduz contratos de consulta, frontend apresenta agrupador | Resultado limpo → feed, filtros, comparações e estados UI | Loading/empty/error/partial, privacidade, latência, acessibilidade; não duplicar business rules. |

Esses owners são **proposta de fronteiras**, não nomes obrigatórios de packages, binários, tabelas ou número de processos. Revisar dependências reais e evitar criar uma interface para cada linha. Telegram é a fonte concreta atual; framework genérico de providers só com segunda demanda efetiva. AI probabilística permanece lateral ao núcleo determinístico enquanto contratos não determinarem o contrário.

## Fluxo e dependências propostas

```text
             Telegram Authorization Credential
                           |
                           v
                Telegram / MTProto Adapter
                   /                 \
                  v                   v
        MCP Telegram realtime     Collector
          consulta direta             |
          sob demanda                 v
                                Source Admission
                                      |
                                      v
                                   Evidence
                                      |
                           recovery/backfill/projection
                                      |
                    Findings -> modelo comercial
                                      |
                                Query Service
                               /             \
                              v               v
                    MCP Limiar data         API -> frontend
```

O MCP realtime é construído cedo para explorar o Telegram e ajudar a descobrir quais dados de promoção realmente existem e precisam ser modelados. Ele não consulta o storage do Limiar como caminho obrigatório e não transforma sua resposta em Evidence automaticamente. O MCP posterior de dados do Limiar usa Query Service. Ambos são adapters desacopláveis; gotd/MTProto também fica atrás de um boundary. Nenhuma dessas separações exige microserviços no início.

**Autenticação — conceitos separados:** (1) `TelegramAuthorizationIdentity` + credencial persistida do boundary Telegram; (2) MTProto `session_id`, efêmero e interno ao gotd; (3) `subscription_id` de escopo de aquisição; (4) autenticação/autorização eventual para usuário/MCP. O legado documenta `limiar-collector auth` para userbot, não comprova identidade de consumidor. Não assumir que um token MCP pode ser sessão Telegram; mecanismo de OAuth, planos, hospedagem e conexão ChatGPT precisam de pesquisa/documentação atuais na fatia própria.

## Reuso sem carregar legado

- `internal/storage/sqlite` é primeira candidata a capability reaproveitável, sujeita a tests na integração nova; migrar dados nunca in-place, manter Tursogo read-only/histórico enquanto autorizado.
- ADRs 016–020 Accepted orientam invariantes e storage; ADR 017 fail-stop e barriers; ADR 018 Source Admission antes de updates.Manager, ainda com integration gates. Testes/experimentos são evidência do ambiente/revisão onde executados, não implantação Limiar 3.0.
- ADRs 021–024 Proposed exigem escolha formal somente quando bloquearem fatia correspondente. ADR 023 é primeira decisão de sessão a investigar; EXP-019 só prova Unix/intra-processo experimental. PR #202 mergeada em 2026-09-14, sem runtime.
- Preservar inputs legados para caracterizar mensagens e imagens, capturar fixtures sintéticas/anonimizadas com consentimento; não copiar credentials reais ou dados privados para Git/ChatGPT sem autorização. Não se realizou nenhum acesso Telegram nesta etapa.
- Portabilidade SQLite/Linux/ARM64/Windows têm evidências delimitadas; macOS e integração completa permanecem sem comprovação aqui. Importação EXP-007 com banco real descartável pendente. Não prometer equivalência ou performance superior.

## Plano incremental, de baixo para cima (sequência revisável)

**M0 — investigação da fundação, `L3-001`:** recuperar HEAD/código/autoridades; confirmar isolamento mínimo e ownership; comparar sessão atual, ADR 004 transição e ADR 023 Proposed; separar login Telegram de consumidor MCP; definir escopo da primeira fatia, config/segredos/risco/testes e decisões indispensáveis. Produto observável: proposta de primeiro código, testes e gates, não código criado por este handoff.

**M1 — runtime + authorization lifecycle + restore/reuse (`L3-002`):** compor entrypoint isolado, storage privado do boundary Telegram, owner único do main `gotd/telegram.Client`, bootstrap explícito e steady-state fail-closed. Provar gotd real: login controlado -> persistência -> shutdown -> restart -> autorizado sem novo OTP. Só depois estabilizar `TelegramQuery` read-only (`ResolvePeer` + `History`) com types source-aware e erros semânticos.

**M2 — MCP realtime (`L3-003`):** expor a `TelegramQuery` via MCP read-only diretamente ao Telegram, sem depender do Evidence DB; usar esse ramo para explorar o corpus. Não criar segundo main client.

**M3 — admissão, Evidence e recovery (`L3-004`):** collector separado do MCP; source configured identity, append-before-forward, replay, fail-stop, state e backfill separados.

**M4 — mídia + descoberta e processamento (`L3-005`):** reproduzir imagens, usar MCP realtime para investigar casos reais, registrar Findings e então materializar deterministic findings/interpretação/modelo comercial.

**M5 — superfícies de dados (`L3-006`):** Query Service; segunda família MCP sobre Limiar data; API e frontend reutilizam as mesmas capabilities.

**M6 — comparação/substituição (`L3-007`):** performance comparável, backup/restore, migração histórica em cópia real descartável e rollback; desligar legado apenas após Evidence e autorização.

Nenhuma data, PR, schema, estrutura de diretórios ou arquitetura de contas é presumida. Black Friday 2026 é objetivo do mantenedor, não aceite de cronograma. Priorização por dependência real, não por ordem rígida de commits.

## Primeiro bloco futuro — critérios a confirmar na investigação

Resultado proposto: entrypoint isolado com `TelegramAuthorizationIdentity`, owner único do main gotd client e credential storage privado; bootstrap explícito; restore/reuse real após restart sem novo OTP; runtime unauthorized/revoked/incompatível falha fechado; primeira `TelegramQuery` bounded; access hashes e `tg.*` não vazam; config validada, segredos não logados e shutdown/cancelamento definidos. **Não considerar esses testes já executados.** Consultar a documentação versionada do gotd e fontes primárias atuais se APIs/semânticas forem determinantes. Testar preservação da sessão antiga apenas se migração for requisito concreto e houver insumo autorizado.

**Implementation Gate:** fatia identificada, dependências essenciais e ownership compreendidos, risco de segredo/platform definido, verificação disponível, ADR necessário aceito e autorização de implementação vigente. Se completo em próxima execução, implementar sem ficar em loop documental; se não, limitar investigação à lacuna impeditiva. Este documento não implementa esse Gate automaticamente.

## Refinamento técnico L3-001A

A investigação da versão real `gotd/td v0.161.0` sustenta a direção desta Proposal com ajustes. `MTProto session_id` não é a unidade de ownership; `session.ErrNotFound` upstream não pode ser interpretado automaticamente como arquivo ausente; peer cache é separado, porém authorization-scoped; history/backfill e update recovery são authorities distintas; e `updates.Manager` precisa de guards externos para satisfazer Evidence-before-progress. O relatório completo é a fonte de detalhe; nenhuma dessas conclusões promove ADR 023 ou autoriza código por si só.

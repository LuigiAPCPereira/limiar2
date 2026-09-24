# Limiar 3.0 — COMECE AQUI

**Estado:** repository rebaseline do Limiar 3 em verificação. **Repositório:** `LuigiAPCPereira/limiar2`. **Branch atual:** `refactor/limiar3-repository-rebaseline`, criada de `docs/limiar-3-foundation-20260922@00e5c422b2ed98b10ab145c53b71936352b90585`. O root já representa Limiar 3 e o legado está contido em `legacy/limiar2/`, mas a integração em `main` ainda não ocorreu. **Reconsultar HEAD, PRs, divergência com main e CI na execução seguinte**. Não pressupor acesso Telegram, deploy, credenciais, MCP conectado ou sincronização automática com arquivos do ChatGPT Project.

## Leia nesta ordem

1. `AGENTS.md` na raiz — Constituição do repositório, ordem de autoridades, limites de permissão e interpretação dos ADRs.
2. `docs/DOCUMENTATION_AND_CONTINUITY.md` — Agent Development Protocol v2.0; para Limiar 3.0 aplicar sobretudo §5 (inicialização de projeto novo), §7 (RECOVER/RECONCILE), §8 (Implementation Gate) e §9 (tarefas). A adoção v2 do repositório anterior permanece **PARCIAL** no PR #214; não declarar Adoption Gate cumprido sem conferir todas as nove funções e evidências.
3. `ENGINEERING_DNA.md` na raiz — entrada e instrução de descompactação; original **integral está preservado no GitHub em `docs/limiar3/ENGINEERING_DNA_ORIGINAL.md.gz.b64`**. O blob remoto `ceafec243be87ae28ee2d9658e3912831fc55368` confere com o blob calculado da origem compactada. Decodificar e conferir SHA-256 `c19c5d97f8e42b311d6c15440e2e9f58cec64fadf1a48b9575e1f2e4dba0e373` antes de mudanças arquiteturais relevantes, no ambiente onde haja ferramentas. **A raiz contém resumo, não Markdown integral descompactado**; não declarar disponibilidade irrestrita ou execução em outro ambiente. `docs/limiar3/ENGINEERING_GUIDE.md` é orientação derivada.
4. `docs/limiar3/PRODUCT_AND_SCOPE.md` — visão e requisitos explícitos do mantenedor, inclusive MCP.
5. `docs/limiar3/REBASELINE_INHERITANCE.md` — contratos, Evidence, limites e pendências que o Limiar 3 herda da Rebaseline 2026 sem transformar experimentos ou ADRs Proposed em produção.
6. `docs/limiar3/DETACHABLE_BOUNDARIES.md` — princípio de desacoplamento de sessão/Telegram/MTProto/MCP; desacoplável por contrato não significa distribuído.
7. `docs/limiar3/BOTTOM_UP_REBUILD_PLAN.md` — construção de baixo para cima por authorities separadas, com MCP realtime antes da modelagem comercial.
8. `docs/limiar3/adr/README.md` — registry próprio de Decisions do Limiar 3; a numeração reinicia em 001.
9. `docs/limiar3/adr/001-hardened-telegram-session-storage.md`, `002-telegram-authorization-runtime.md` e `003-repository-topology-and-legacy-containment.md` — Decisions Accepted da fundação e da topologia; cada implementação continua sujeita ao seu gate.
10. `docs/limiar3/ARCHITECTURE_PROPOSAL.md` — documentação derivada/proposta; em conflito, prevalecem os ADRs Accepted.
11. `docs/limiar3/L3_001_SESSION_BOUNDARY_INVESTIGATION.md` — Proposal inicial de L3-001; foi refinada por L3-001A.
12. `docs/limiar3/L3_001A_MTPROTO_GOTD_INVESTIGATION.md` — investigação concluída de MTProto + `gotd/td v0.161.0`; fonte técnica posterior para o boundary Telegram, ainda não Decision.
13. `docs/limiar3/L3_001B_PRODUCTION_FOUNDATION_RESEARCH.md` — terceira investigação: hardening, toolchain, bootstrap, performance, observability, supply chain e gates de produção.
14. `docs/limiar3/DEPENDENCY_TOOLCHAIN_POLICY.md` — direção `Current Stable First` do mantenedor.
15. `docs/limiar3/L3_001C_GOTD_UPSTREAM_AUDIT.md` — auditoria upstream-only concluída; recomenda gotd v0.162.0, extension map e stop condition da pesquisa ampla.
16. `docs/limiar3/GOTD_UPSTREAM_ONLY_RESEARCH_BRIEF.md` — brief histórico de L3-001C, marcado como concluído.
17. `docs/limiar3/MTPROTO_GOTD_RESEARCH_BRIEF.md` — brief histórico de L3-001A, concluído.
18. `docs/limiar3/TASKLIST.md` — inventário próprio da frente Limiar 3.0.
19. `docs/limiar3/PROJECT_STATE.md` — checkpoint e próxima ação.
20. `docs/limiar3/INIT_REPORT.md` e `docs/limiar3/SESSION_LOG.md` — matriz das nove funções, lacunas e histórico real.
21. `docs/BASELINE.md`, `docs/ARCHITECTURE.md` e `docs/limiar3/adr/README.md`; material histórico anterior está sob `legacy/limiar2/`.

## Regra de herança da rebaseline

**Limiar 3 herda conclusões, invariantes, contratos aceitos e limites de Evidence; não herda automaticamente mecanismos experimentais, schemas `Proposed`, topologia física, packages ou decisões acidentais do Limiar legado.** O mapa consolidado está em [`REBASELINE_INHERITANCE.md`](REBASELINE_INHERITANCE.md) e a sequência de construção em [`BOTTOM_UP_REBUILD_PLAN.md`](BOTTOM_UP_REBUILD_PLAN.md).

## Instrução direta para o próximo chat

**Pedido já respondido pelo mantenedor:** reconstruir integralmente o Limiar 3.0 bottom-up com boundaries corretos. Sessão e integração Telegram/MTProto devem ser desacopláveis do core por contrato; MCP também deve ser detachable. O MCP possui um ramo **Telegram realtime** que consulta a fonte diretamente, sob demanda, antes do modelo de promoções, para ajudar a descobrir quais dados devem ser coletados/modelados; depois pode ganhar tools de **Limiar data** sobre Query Service. Collector/Evidence continuam um ramo independente e durável. Desacoplamento não exige microserviços. **Não existe um segundo aplicativo Limiar.**

**Estado das investigações e Decisions de fundação:** L3-001/A/B/C estão internalizadas e a stop condition da pesquisa ampla foi atingida. L3 ADR 001/002/003 estão Accepted. `L3-BASE-001` executou a separação física do legado e está em verificação/CI. Go 1.27.1 e gotd/td v0.162.0 seguem como baseline inicial de L3-002. **Próxima ação:** validar/integrar o repository rebaseline e então retomar o Implementation Gate de L3-002.

**Distinção de autenticações:** (a) `TelegramAuthorizationIdentity` + credencial persistida do boundary Telegram; (b) MTProto `session_id`, efêmero e interno ao gotd; (c) eventual identidade/autorização para consumidores do Limiar e acesso pelo ChatGPT/MCP. Não pressupor que (c) já existe no legado, nem reutilizar diretamente arquivo de sessão Telegram como token MCP. Investigar autenticação de consumidor somente na medida em que for dependência real do primeiro slice, sem inventar sistema de contas.

**Autorização vigente:** o mantenedor autorizou `L3-BASE-001`, incluindo branch, movimentação estrutural, scaffold Go/CI e documentação. Isso não autoriza L3-002 funcional, Telegram/login/OTP, novos serviços, custos, operações com dados reais, merge ou deploy. Não exigir repetição das respostas do mantenedor contidas nestes documentos; perguntar só decisão material ainda não respondida.

## Próximas leituras técnicas focadas

- Auth/session/runtime: L3 ADR 001/002 **Accepted**. Credential storage hardened local file em Linux/single-process, owner único por `TelegramAuthorizationIdentity`, bootstrap administrativo QR-first/fallback controlado, semantic Ready/self binding, Go 1.27.1 e gotd v0.162.0 são a baseline aceita. O ADR histórico 023 continua `Proposed` apenas no registry legado; não é a authority do Limiar 3.
- Coleta/Evidence: ADRs 016–020 Accepted; ADR 018 Accepted mas Source Admission ainda não integrada em produção; ADR 024 **Proposed** para `subscription_id` (não inventar ID de produção).
- Persistência histórica: `legacy/limiar2/internal/storage/sqlite`, migrations; PR #211 integrada (guard de schema), não equivale a substituição completa Tursogo. ADRs 021/022 ainda Proposed; importação real EXP-007 não demonstrada.
- Imagens: falha funcional relatada pelo mantenedor, causa não demonstrada neste trabalho; ADR 011 `DEFER / REVALIDATE`, ADR 012 `RETIRE`.

**Critério de sucesso da preparação documental:** arquivos reabertos na branch correta, referências coerentes, escopo e tarefas recuperáveis, nenhuma pretensão de produto validado. O checkpoint informa o que foi efetivamente conferido.

## Repository rebaseline / legacy containment

Por L3 ADR 003, o root agora representa exclusivamente Limiar 3. A implementação anterior está preservada em `legacy/limiar2/`; não importar packages legacy no código L3. A fatia estrutural correspondente é `L3-BASE-001`.

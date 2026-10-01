# Limiar 3.0 — COMECE AQUI

**Estado:** L3-PLAT-001 **VALIDADA EM CI MULTIPLATAFORMA / PR DRAFT NÃO MERGEADA** no Draft PR #220; L3-002 **IMPLEMENTADA / TOOLCHAIN CI PASS / NÃO VALIDADA EM INTEGRAÇÃO TELEGRAM REAL**; L3-003 **TRACER IMPLEMENTADO / TOOLCHAIN CI PASS / INTEGRAÇÃO REAL PENDENTE**. **Repositório:** `LuigiAPCPereira/limiar2`. **Branch de trabalho:** `fix/limiar3-platform-agnostic-20260929`, baseada em `feat/limiar3-l3-003-mcp-realtime@f6ab9ced2477ef17aedc7845fe3edbf720057a99`. O root representa Limiar 3 e o legado permanece em `legacy/limiar2/`. Não houve merge/deploy. Reconsultar HEAD, PR #220 e gates na próxima execução.

## Leia nesta ordem

1. `AGENTS.md` na raiz — Constituição do repositório, ordem de autoridades, limites de permissão e interpretação dos ADRs.
2. `docs/DOCUMENTATION_AND_CONTINUITY.md` — Agent Development Protocol v2.0; para Limiar 3.0 aplicar sobretudo §5 (inicialização de projeto novo), §7 (RECOVER/RECONCILE), §8 (Implementation Gate) e §9 (tarefas). A adoção v2 do repositório anterior permanece **PARCIAL** no PR #214; não declarar Adoption Gate cumprido sem conferir todas as nove funções e evidências.
3. `ENGINEERING_DNA.md` na raiz — entrada e instrução de descompactação; original **integral está preservado no GitHub em `docs/limiar3/ENGINEERING_DNA_ORIGINAL.md.gz.b64`**. O blob remoto `ceafec243be87ae28ee2d9658e3912831fc55368` confere com o blob calculado da origem compactada. Decodificar e conferir SHA-256 `c19c5d97f8e42b311d6c15440e2e9f58cec64fadf1a48b9575e1f2e4dba0e373` antes de mudanças arquiteturais relevantes, no ambiente onde haja ferramentas. **A raiz contém resumo, não Markdown integral descompactado**; não declarar disponibilidade irrestrita ou execução em outro ambiente. `docs/limiar3/ENGINEERING_GUIDE.md` é orientação derivada.
4. `docs/limiar3/PRODUCT_AND_SCOPE.md` — visão e requisitos explícitos do mantenedor, inclusive MCP.
5. `docs/limiar3/REBASELINE_INHERITANCE.md` — contratos, Evidence, limites e pendências que o Limiar 3 herda da Rebaseline 2026 sem transformar experimentos ou ADRs Proposed em produção.
6. `docs/limiar3/DETACHABLE_BOUNDARIES.md` — princípio de desacoplamento de sessão/Telegram/MTProto/MCP; desacoplável por contrato não significa distribuído.
7. `docs/limiar3/BOTTOM_UP_REBUILD_PLAN.md` — construção de baixo para cima por authorities separadas, com MCP realtime antes da modelagem comercial.
8. `docs/limiar3/adr/README.md` — registry próprio de Decisions do Limiar 3; a numeração reinicia em 001.
9. `docs/limiar3/adr/001-hardened-telegram-session-storage.md`, `002-telegram-authorization-runtime.md`, `003-repository-topology-and-legacy-containment.md`, `004-mcp-telegram-realtime-boundary.md` e `005-platform-agnostic-runtime-and-storage.md` — Decisions Accepted; a ADR 005 corrige a restrição Linux dos ADRs 001/002.
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

**Estado das investigações e Decisions de fundação:** L3-001/A/B/C estão internalizadas e a stop condition da pesquisa ampla foi atingida. L3 ADR 001/002/003/004/005 estão Accepted. A ADR 005 estabelece que Limiar 3 é agnóstico ao sistema operacional por padrão e que restrições de plataforma só podem existir no menor adapter realmente dependente do SO. `L3-BASE-001` executou a separação física do legado. Go 1.27.1 e gotd/td v0.162.0 seguem como baseline aceita de L3-002. A revisão final evidence-first de 2026-09-29 sobre o código em `150f5105cac9ca3b96fd423220c54491ba0fa4d7` não encontrou gap concreto de implementação que justifique novo patch; as APIs usadas foram novamente confrontadas estaticamente com gotd v0.162.0. **Próxima ação L3-002:** não ampliar a arquitetura por default. Quando houver canal executável, rodar build/vet/test/race/govulncheck no Go 1.27.1 + gotd v0.162.0 e, somente em ambiente Telegram explicitamente autorizado, provar restart/reuse, same-self, first `ResolvePeer+History`, reconnect, clock-skew/lack-of-progress e cross-DC. Até esses gates, o estado permanece **IMPLEMENTADA NO CÓDIGO / NÃO VALIDADA EM INTEGRAÇÃO REAL**. O L3 ADR 004 foi **Accepted** pelo mantenedor em 2026-09-29, com acceptance reference `f24c55535560102c3184d0752bf5f78dc2104bdc`. O tracer L3-003 foi implementado em `202fe46e1c6ab59e3c0721dea71c02ef14c9a54f` com `telegram.targets` + `telegram.history`, fake-backed tests escritos e hardening HTTP do ADR. Busca/filtragem/comparação de perguntas como "ache Samsung" pertencem ao ChatGPT. `limit` é tamanho de página: default 20, máximo técnico 100; o ChatGPT pode percorrer quantidade maior seguindo `next_cursor` por quantas páginas forem necessárias. A validação executável de toolchain passou no candidato `a4f7836f0673f234b54d1a950b33f212b6106ab1`, GitHub Actions run #764 (`36846123134`): build/vet/test em Ubuntu e Windows; race e govulncheck no Ubuntu. Integração HTTP/ChatGPT/Telegram real continua UNKNOWN.

**Distinção de autenticações:** (a) `TelegramAuthorizationIdentity` + credencial persistida do boundary Telegram; (b) MTProto `session_id`, efêmero e interno ao gotd; (c) eventual identidade/autorização para consumidores do Limiar e acesso pelo ChatGPT/MCP. Não pressupor que (c) já existe no legado, nem reutilizar diretamente arquivo de sessão Telegram como token MCP. Investigar autenticação de consumidor somente na medida em que for dependência real do primeiro slice, sem inventar sistema de contas.

**Limites operacionais:** este `START_HERE.md` não concede permissões por si só. A fronteira corrente é **reconciliar o CI do checkpoint documental do Draft PR #220 e preservar a entrega validada sem merge não autorizado**; L3-002/L3-003 permanecem aguardando somente suas integrações reais específicas; cada sessão deve reconciliar a instrução vigente do mantenedor antes de ações consequenciais. Sem autorização explícita aplicável, não fazer merge/deploy, não usar conta ou credencial Telegram real, não pedir OTP/2FA, não criar/revogar autorização Telegram, não criar Secure MCP Tunnel real e não expor MCP publicamente/non-loopback.

## Próximas leituras técnicas focadas

- Auth/session/runtime: L3 ADR 001/002 **Accepted**, com a dimensão de plataforma corrigida pela L3 ADR 005. Credential storage é arquivo local endurecido e portável por padrão; single-process é limitação de lifecycle, não de sistema operacional. Owner único por `TelegramAuthorizationIdentity`, bootstrap administrativo QR-first/fallback controlado, semantic Ready/self binding, Go 1.27.1 e gotd v0.162.0 são a baseline aceita. O ADR histórico 023 continua `Proposed` apenas no registry legado; não é a authority do Limiar 3.
- Coleta/Evidence: ADRs 016–020 Accepted; ADR 018 Accepted mas Source Admission ainda não integrada em produção; ADR 024 **Proposed** para `subscription_id` (não inventar ID de produção).
- Persistência histórica: `legacy/limiar2/internal/storage/sqlite`, migrations; PR #211 integrada (guard de schema), não equivale a substituição completa Tursogo. ADRs 021/022 ainda Proposed; importação real EXP-007 não demonstrada.
- Imagens: falha funcional relatada pelo mantenedor, causa não demonstrada neste trabalho; ADR 011 `DEFER / REVALIDATE`, ADR 012 `RETIRE`.

**Critério de sucesso da preparação documental:** arquivos reabertos na branch correta, referências coerentes, escopo e tarefas recuperáveis, nenhuma pretensão de produto validado. O checkpoint informa o que foi efetivamente conferido.

## Repository rebaseline / legacy containment

Por L3 ADR 003, o root agora representa exclusivamente Limiar 3. A implementação anterior está preservada em `legacy/limiar2/`; não importar packages legacy no código L3. A fatia estrutural correspondente é `L3-BASE-001`.

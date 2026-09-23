# Limiar 3.0 — COMECE AQUI

**Estado:** planejamento/documentação da nova geração, sem implementação autorizada por esta entrega. **Repositório:** `LuigiAPCPereira/limiar2`. **Branch de planejamento:** `docs/limiar-3-foundation-20260922`, criada a partir do commit `7177d929839512b8005ae379d96e3d224feac1f8` da branch documental de adoção v2 (PR #214 draft), que por sua vez partiu de `main@796b7769449b72320b27c2557c2ccb8c8eb183e1`. Estas são referências históricas de origem; **reconsultar HEAD, PRs, divergência com main e testes na execução seguinte**. A `main` não foi modificada por esta preparação. Não pressupor worktree local, acesso Telegram, deploy, credenciais, MCP conectado ou sincronização com arquivos do ChatGPT Project.

## Leia nesta ordem

1. `AGENTS.md` na raiz — Constituição do repositório, ordem de autoridades, limites de permissão e interpretação dos ADRs.
2. `docs/DOCUMENTATION_AND_CONTINUITY.md` — Agent Development Protocol v2.0; para Limiar 3.0 aplicar sobretudo §5 (inicialização de projeto novo), §7 (RECOVER/RECONCILE), §8 (Implementation Gate) e §9 (tarefas). A adoção v2 do repositório anterior permanece **PARCIAL** no PR #214; não declarar Adoption Gate cumprido sem conferir todas as nove funções e evidências.
3. `ENGINEERING_DNA.md` na raiz — entrada e instrução de descompactação; original **integral está preservado no GitHub em `docs/limiar3/ENGINEERING_DNA_ORIGINAL.md.gz.b64`**. O blob remoto `ceafec243be87ae28ee2d9658e3912831fc55368` confere com o blob calculado da origem compactada. Decodificar e conferir SHA-256 `c19c5d97f8e42b311d6c15440e2e9f58cec64fadf1a48b9575e1f2e4dba0e373` antes de mudanças arquiteturais relevantes, no ambiente onde haja ferramentas. **A raiz contém resumo, não Markdown integral descompactado**; não declarar disponibilidade irrestrita ou execução em outro ambiente. `docs/limiar3/ENGINEERING_GUIDE.md` é orientação derivada.
4. `docs/limiar3/PRODUCT_AND_SCOPE.md` — visão e requisitos explícitos do mantenedor, inclusive MCP.
5. `docs/limiar3/REBASELINE_INHERITANCE.md` — contratos, Evidence, limites e pendências que o Limiar 3 herda da Rebaseline 2026 sem transformar experimentos ou ADRs Proposed em produção.
6. `docs/limiar3/DETACHABLE_BOUNDARIES.md` — princípio de desacoplamento de sessão/Telegram/MTProto/MCP; desacoplável por contrato não significa distribuído.
7. `docs/limiar3/BOTTOM_UP_REBUILD_PLAN.md` — construção de baixo para cima por authorities separadas, com MCP realtime antes da modelagem comercial.
8. `docs/limiar3/ARCHITECTURE_PROPOSAL.md` — alternativas, responsabilidades, riscos, reutilização e decisões pendentes; proposta, não ADR Accepted.
9. `docs/limiar3/L3_001_SESSION_BOUNDARY_INVESTIGATION.md` — Proposal inicial de L3-001; foi refinada por L3-001A.
10. `docs/limiar3/L3_001A_MTPROTO_GOTD_INVESTIGATION.md` — investigação concluída de MTProto + `gotd/td v0.161.0`; fonte técnica posterior para o boundary Telegram, ainda não Decision.
11. `docs/limiar3/MTPROTO_GOTD_RESEARCH_BRIEF.md` — brief histórico de L3-001A, marcado como concluído.
12. `docs/limiar3/TASKLIST.md` — inventário próprio da frente Limiar 3.0.
13. `docs/limiar3/PROJECT_STATE.md` — checkpoint e próxima ação.
14. `docs/limiar3/INIT_REPORT.md` e `docs/limiar3/SESSION_LOG.md` — matriz das nove funções, lacunas e histórico real.
15. `docs/BASELINE.md`, `docs/adr/README.md` e ADRs Accepted/Proposed pertinentes; verificar ref e estados vigentes antes de qualquer código.

## Regra de herança da rebaseline

**Limiar 3 herda conclusões, invariantes, contratos aceitos e limites de Evidence; não herda automaticamente mecanismos experimentais, schemas `Proposed`, topologia física, packages ou decisões acidentais do Limiar legado.** O mapa consolidado está em [`REBASELINE_INHERITANCE.md`](REBASELINE_INHERITANCE.md) e a sequência de construção em [`BOTTOM_UP_REBUILD_PLAN.md`](BOTTOM_UP_REBUILD_PLAN.md).

## Instrução direta para o próximo chat

**Pedido já respondido pelo mantenedor:** reconstruir integralmente o Limiar 3.0 bottom-up com boundaries corretos. Sessão e integração Telegram/MTProto devem ser desacopláveis do core por contrato; MCP também deve ser detachable. O MCP possui um ramo **Telegram realtime** que consulta a fonte diretamente, sob demanda, antes do modelo de promoções, para ajudar a descobrir quais dados devem ser coletados/modelados; depois pode ganhar tools de **Limiar data** sobre Query Service. Collector/Evidence continuam um ramo independente e durável. Desacoplamento não exige microserviços. **Não existe um segundo aplicativo Limiar.**

**Estado de L3-001/L3-001A:** ambas as investigações estão internalizadas. L3-001A refinou a Proposal: owner por `TelegramAuthorizationIdentity` (não MTProto `session_id`), main gotd client único por autorização, private session storage, bootstrap explícito/fail-closed, peer state authorization-scoped, restore/reuse real antes da primeira capability e guards externos ao `updates.Manager`. Fonte: `L3_001A_MTPROTO_GOTD_INVESTIGATION.md`. **Próxima ação não é implementação automática:** decidir apenas os bloqueios reais de L3-002 e passar pelo Implementation Gate.

**Distinção de autenticações:** (a) `TelegramAuthorizationIdentity` + credencial persistida do boundary Telegram; (b) MTProto `session_id`, efêmero e interno ao gotd; (c) eventual identidade/autorização para consumidores do Limiar e acesso pelo ChatGPT/MCP. Não pressupor que (b) já existe no legado, nem reutilizar diretamente arquivo de sessão Telegram como token MCP. Investigar autenticação de consumidor somente na medida em que for dependência real do primeiro slice, sem inventar sistema de contas.

**Autorização desta entrega:** criar esta branch e documentação. Nada de código, novos serviços, custos, operações com dados reais ou merge. Um futuro pedido explícito pode alterar esse limite. Não exigir repetição das respostas do mantenedor contidas nestes documentos; perguntar só decisão material ainda não respondida.

## Próximas leituras técnicas focadas

- Autorização/session storage: L3-001A concluiu a investigação upstream em `gotd/td v0.161.0`; ADR 023 continua **Proposed**; EXP-018/019 mantêm seus limites. Backend de credential storage, plataforma inicial e bootstrap operacional ainda exigem decisão explícita antes de L3-002.
- Coleta/Evidence: ADRs 016–020 Accepted; ADR 018 Accepted mas Source Admission ainda não integrada em produção; ADR 024 **Proposed** para `subscription_id` (não inventar ID de produção).
- Persistência: `internal/storage/sqlite`, migrations; PR #211 integrada (guard de schema), não equivale a substituição completa Tursogo. ADRs 021/022 ainda Proposed; importação real EXP-007 não demonstrada.
- Imagens: falha funcional relatada pelo mantenedor, causa não demonstrada neste trabalho; ADR 011 `DEFER / REVALIDATE`, ADR 012 `RETIRE`.

**Critério de sucesso da preparação documental:** arquivos reabertos na branch correta, referências coerentes, escopo e tarefas recuperáveis, nenhuma pretensão de produto validado. O checkpoint informa o que foi efetivamente conferido.
# PROJECT_STATE — Limiar 2 / checkpoint derivado

**Data da observação:** 2026-09-20. **Projeto:** `LuigiAPCPereira/limiar2`. **Resultado de adoção:** ADOÇÃO PARCIAL. Este registro é derivado de [`TASKLIST.md`](TASKLIST.md), [`ADOPTION_REPORT.md`](ADOPTION_REPORT.md), GitHub e decisões, não fonte de requisitos nem inventário concorrente.

- **Autorização:** usuário autorizou modo Aplicar e continuidade de desenvolvimento no escopo previamente aprovado. Não autorizou por esse comando merge, deploy, aceitar ADR Proposed, operação destrutiva ou modificar agendamento.
- **Fonte de protocolo:** [`DOCUMENTATION_AND_CONTINUITY.md`](DOCUMENTATION_AND_CONTINUITY.md) v2.0 agora existe na branch e `AGENTS.md` possui seção 18 de entrada. Kit externo do ChatGPT Project: SHA-256 `d078e0b3d4a8f9d4bd21cb0c7c8a3e417cba484981566d801ff8453ac7be1dab`, Git blob original calculado `78b2e86564fb287886f9df065fdb727b20c52727`; Git blob remoto da transcrição `f78d5a7e6f755955c1a78eabbd9437f299eb0430`. **Identidade byte a byte pendente; não tratar como versão canônica verificada.** Codex/Project sincronizado/agendamentos não testados.
- **Escopo e fonte de requisitos:** rebaseline 2026 de plataforma de promoções Telegram; `README.md`, `docs/PRODUCT_BRIEF.md`, `docs/CONTEXT.md`, `docs/BASELINE.md`, `docs/specs/`, ADRs Accepted. Versão/aceites e cobertura de todo escopo ativo ainda não demonstrados; não promover intenções históricas a tarefas vigentes.
- **Inventário:** [`TASKLIST.md`](TASKLIST.md) com ADOPT-001–004, DEV-001 e PROD-001–008 derivados de documentos citados. Completo para adoção delimitada e frentes de produto mapeadas, mas cobertura exaustiva do produto ainda não comprovada.
- **Marcos:** [`ROADMAP.md`](ROADMAP.md), M0–M4 sem datas inventadas; roadmap histórico em `docs/CONTEXT.md` não define automaticamente nova execução.
- **Arquitetura e decisões:** `AGENTS.md` Constitucional, `docs/adr/README.md`, ADRs Accepted e `docs/BASELINE.md` derivada. ADR 018 Accepted em seu registro por issue #212, BASELINE ainda o descreve Proposed; ADR 024 Proposed não autoriza implementação produtiva de subscription identity. ADRs 021–023 Proposed na ref-base observada. Preservar legacy side-by-side conforme ADR 019.
- **Histórico:** `docs/evolution/`, ADRs, PRs/commits e este PR; suficiência para todos os motivos/marcos ainda pendente de reconciliação.
- **Git:** ref inicial verificada `main@796b7769449b72320b27c2557c2ccb8c8eb183e1`; mudanças somente em `docs/adopt-agent-protocol-v2-20260920` e [PR #214](https://github.com/LuigiAPCPereira/limiar2/pull/214) draft. Revisão da branch muda a cada commit documental; consultar HEAD exato antes de agir. Worktree local inacessível por conector GitHub remoto. PR não integrado conforme última leitura; revalidar no handoff.
- **Implementação e validação:** nenhuma alteração de código nesta adoção; documentos criados/atualizados e algumas fontes reabertas. Testes Go/CI, runtime, merge e deploy não executados/confirmados. Resultados anteriores não validam novo HEAD.

## Seleção e próxima ação

- `ADOPT-001`: diagnóstico das nove funções validado, sem significar Gate aprovado.
- **Tarefa atual `ADOPT-002` — em andamento com integridade pendente:** obter/instalar bytes exatos do protocolo fornecido no Project, confirmar SHA-256 e Git blob contra fonte, reabrir na ref, conferir entrada AGENTS e disponibilidade no ambiente efetivo. O documento presente, sem hash coincidente, não atende ao aceite.
- `ADOPT-003` — em andamento: matriz expandida e ROADMAP presentes; mapear requisitos/aceites e todas as tarefas vigentes, corrigir BASELINE acerca do ADR 018, avaliar histórico.
- `ADOPT-004` — pendente: confrontar HEAD e PR atual, checar links, idempotência, fontes sem duplicidade e critérios do Adoption Gate v2, então relatar estado.
- `DEV-001` — bloqueada até identificar fatia segura autorizada e Implementation Gate; não interpretar ADR 024 Proposed como aceite.

**Próxima ação executável:** `ADOPT-002`, verificar integridade da fonte canônica; em paralelo `ADOPT-003`, confirmar cobertura documental e alinhar BASELINE ao ADR 018 aceito. Critério: evidência de arquivo idêntico e reaberto, matriz de nove funções sem lacunas aplicáveis, TASKLIST cobrindo escopo real e checkpoint reconciliado. Não inserir o SHA do commit que modifica este checkpoint para evitar ciclo infinito.

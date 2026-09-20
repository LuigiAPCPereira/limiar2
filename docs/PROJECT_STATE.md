# PROJECT_STATE — checkpoint de adoção, não prova de integração

Data observada: 2026-09-20. Projeto: `LuigiAPCPereira/limiar2`. Origem documental externa: Agent Development Protocol v2.0 fornecido no ChatGPT Project como `DOCUMENTATION_AND_CONTINUITY.md`, SHA-256 `d078e0b3d4a8f9d4bd21cb0c7c8a3e417cba484981566d801ff8453ac7be1dab`. **A fonte externa ainda não está integralmente implantada no repositório nem testada em Codex/Tarefas Agendadas.**

- Escopo autorizado: adaptação documental e depois continuidade de desenvolvimento apenas no escopo e nas decisões previamente aprovados. Este comando não autorizou merge, deploy ou aceitação de ADR.
- Inventário inicial de adoção: [`docs/TASKLIST.md`](TASKLIST.md). **Não cobre integralmente o produto.**
- Diagnóstico `ADOPT-001`: validado quanto à matriz de nove funções no [`docs/ADOPTION_REPORT.md`](ADOPTION_REPORT.md), reaberto na branch. Adoption Gate completo ainda não passou.
- **Tarefa corrente: `ADOPT-002`.** Aceite: cópia integral e verificada do protocolo na ref real, comparação SHA-256, mescla preservadora em `AGENTS.md`, referências e acesso operacional checados. Estado: pendente; não declarar integração por texto deste checkpoint.
- Requisitos de produto: `docs/PRODUCT_BRIEF.md`, `docs/CONTEXT.md`, `docs/BASELINE.md`, `docs/specs/` e ADRs; escopo, critérios e tarefas vigentes ainda precisam ser reconciliados em `ADOPT-003`.
- Arquitetura/decisões: `AGENTS.md` (Constitutional), `docs/adr/README.md`, ADRs Accepted, `docs/BASELINE.md` (Derived). ADR 024 `Proposed` no HEAD de referência. ADR 018 `Accepted` em seu registro, mas descrito como `Proposed` na BASELINE observada: divergência documental pendente, sem mudança arquitetural presumida.
- Marcos: `docs/CONTEXT.md` tem roadmap histórico; adequação à Rebaseline 2026 não demonstrada. Histórico: `docs/evolution/`, ADRs e PRs/commits são fontes, sem equivalência integral demonstrada.
- Git observado: base `main@796b7769449b72320b27c2557c2ccb8c8eb183e1`; branch `docs/adopt-agent-protocol-v2-20260920`. [PR #214](https://github.com/LuigiAPCPereira/limiar2/pull/214) foi criado como **draft aberto, não integrado**, com três arquivos documentais. O HEAD da PR observado ao abri-la era `32c4b0ed4ad36c042eb5038459365a9bf9f10bc1`, anterior às atualizações deste checkpoint; consultar novamente antes de agir. O conector só inspeciona GitHub remoto, não o working tree local.
- Implementação de produto: nenhum código modificado na PR documental. Testes Go/CI, runtime e deploy não executados/verificados nesta adoção. Merge não solicitado.
- Bloqueios: protocolo não integral no repositório; `AGENTS.md` não integrado ao v2; tarefas/aceites de produto, marcos e histórico não demonstrados; divergência ADR 018–BASELINE.
- **Próxima ação executável:** `ADOPT-002` — incorporar arquivo canônico integral da fonte fornecida e confirmar hash, integrar referências em AGENTS mantendo a Constituição. Depois `ADOPT-003` (cobrir escopo/tracker/roadmap/histórico), `ADOPT-004` (reabrir fontes, revisar PR/HEAD, links/idempotência e Gate v2) e apenas após elegibilidade selecionar `DEV-001`.

Snapshot derivado: não inserir SHA do commit que modifica este próprio checkpoint e evitar loop de autorreferência.

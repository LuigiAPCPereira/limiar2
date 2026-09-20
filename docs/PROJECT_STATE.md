# PROJECT_STATE — checkpoint de adoção, não prova de integração

Data observada: 2026-09-20. Projeto: `LuigiAPCPereira/limiar2`. Origem documental externa: Agent Development Protocol v2.0 fornecido no ChatGPT Project como `DOCUMENTATION_AND_CONTINUITY.md`, SHA-256 `d078e0b3d4a8f9d4bd21cb0c7c8a3e417cba484981566d801ff8453ac7be1dab`. **A origem externa não foi ainda implantada integralmente no repositório nem testada em Codex/Tarefas Agendadas.**

- Escopo autorizado: adaptação documental do protocolo, depois continuidade de desenvolvimento somente dentro de decisões e escopo previamente aprovados; nenhum merge/deploy ou aceitação de ADR foi autorizado por este comando.
- Inventário canônico inicial da adoção: [`docs/TASKLIST.md`](TASKLIST.md). Não cobre ainda o escopo inteiro do produto.
- Tarefa corrente: `ADOPT-001`; aceite: matriz documental de nove funções, com fonte/ref, suficiência e lacunas no [`docs/ADOPTION_REPORT.md`](ADOPTION_REPORT.md). Próxima tarefa depois de concluir ADOPT-001: `ADOPT-002`.
- Requisitos de produto: `docs/PRODUCT_BRIEF.md`, `docs/CONTEXT.md`, `docs/BASELINE.md`, `docs/specs/` e ADRs; autoridade concreta precisa ser reconciliada para escopo ativo, sem promover propostas.
- Arquitetura/decisões: `AGENTS.md` (Constitutional), `docs/adr/README.md`, ADRs Accepted e `docs/BASELINE.md` (Derived). ADR 024 é Proposed no HEAD observado. ADR 018 está Accepted em seu arquivo, embora a BASELINE lida no mesmo HEAD ainda o descreva como Proposed; divergência documental aberta, sem mudança semântica implícita.
- Marcos: `docs/CONTEXT.md` contém roadmap histórico de fases; suficiência para a Rebaseline 2026 ainda não verificada.
- Histórico: `docs/evolution/`, ADRs e PRs/commits oferecem fontes históricas, mas equivalência integral ainda não demonstrada.
- Referência de partida GitHub: `main@796b7769449b72320b27c2557c2ccb8c8eb183e1` (observado em 2026-09-20); branch documental `docs/adopt-agent-protocol-v2-20260920` criada a partir desse commit. Reconciliar HEAD remoto antes de cada escrita subsequente. Working tree local não acessível pelo conector.
- Implementação de produto: não modificada nesta adoção. Testes de código/CI, merge e deploy: não executados/não verificados. Documento criado em branch não significa `main` integrado.
- Bloqueios: fonte canônica ainda não inserida e validada integralmente no repositório; inventário de produto e roadmap/histórico não demonstrados como completos; AGENTS precisa de integração preservando autoridade Constitucional e regras existentes.
- Próxima ação executável: concluir `ADOPT-001` com matriz de nove linhas baseada na ref da branch; em `ADOPT-002`, instalar cópia integral bit a bit e conferir hash, mesclar entrada em `AGENTS.md` sem substituir conteúdo válido; em `ADOPT-003`, mapear requisitos/tarefas/marcos/histórico com evidências; em `ADOPT-004`, reabrir fontes, validar links, atualização da revisão e Adoption Gate. Só então selecionar `DEV-001` conforme decisão Accepted aplicável.

Este checkpoint é derivado de documentação e Git observado. Não guardar aqui o SHA do próprio commit que altera este documento para evitar autorreferência infinita.

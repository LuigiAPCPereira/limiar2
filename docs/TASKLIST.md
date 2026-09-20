# TASKLIST — adoção documental do Agent Development Protocol v2.0

Estado: inventário inicial de **adoção**, não lista exaustiva de desenvolvimento do produto. Origem: pedido explícito do mantenedor em 2026-09-20 (aplicar no repositório `LuigiAPCPereira/limiar2` e continuar desenvolvimento somente dentro do escopo previamente aprovado). Revisão de referência do GitHub: `main@796b7769449b72320b27c2557c2ccb8c8eb183e1`. Atualizar estados apenas após verificar a ref real. Este documento não concede autorização para aceitar ADR, merge ou deploy.

| ID | Marco | Resultado verificável | Estado | Dependências | Aceite / fonte | Evidência / validação | Branch / PR |
| --- | --- | --- | --- | --- | --- | --- | --- |
| ADOPT-001 | Adoção | Inventariar as nove funções e identificar fontes concorrentes ou lacunas | em andamento | nenhuma confirmada | `DOCUMENTATION_AND_CONTINUITY.md` v2.0, §§4 e 6, cópia anexada ao ChatGPT Project | `docs/ADOPTION_REPORT.md`; verificar arquivo e ref após gravação | `docs/adopt-agent-protocol-v2-20260920`; PR não criado no momento deste registro |
| ADOPT-002 | Adoção | Incorporar protocolo canônico integral e conferido à fonte acessível do repositório e integrar sem sobrescrever as regras constitucionais de `AGENTS.md` | pendente | ADOPT-001 | SHA-256 da cópia do Project: `d078e0b3d4a8f9d4bd21cb0c7c8a3e417cba484981566d801ff8453ac7be1dab`; preservar `AGENTS.md`, ADRs aceitos e fonte GitHub verificada | Ainda sem cópia integral na ref; comparar bytes/hash e reabrir arquivos antes de validar | mesma branch; PR não confirmado |
| ADOPT-003 | Adoção | Completar mapa de requisitos, arquitetura, decisões, marcos, histórico e inventário de tarefas do **escopo ativo de produto** sem duplicar autoridades | pendente | ADOPT-001 | Nove funções, cobertura dos critérios e tarefas com ID/estado/dependências/aceite/evidência | `docs/PRODUCT_BRIEF.md`, `docs/BASELINE.md`, `docs/adr/README.md` são fontes; cobertura de todo escopo ainda não demonstrada | mesma branch; PR não confirmado |
| ADOPT-004 | Adoção | Reconciliar checkpoint com ID existente, ref, validação e próxima ação; verificar links e idempotência na ref de destino | pendente | ADOPT-002, ADOPT-003 | Adoption Gate v2 integral | `docs/PROJECT_STATE.md` e relatório; confirmação final pendente | mesma branch; PR não confirmado |
| DEV-001 | Desenvolvimento — elegibilidade | Reconciliar autorização, fonte e critérios da próxima fatia de desenvolvimento antes de implementar | bloqueada | ADOPT-004; decisão aplicável | `AGENTS.md` §§2, 4 e 8; ADRs Accepted somente. ADR 024 permanece Proposed | Nenhum código alterado; não assumir que a intenção de continuar aceita ADR 024 | nenhuma branch de código selecionada |

## Regras de interpretação

- `em andamento` não é `validada`; commit, CI, merge e deploy são dimensões separadas. Ausência de evidência permanece explícita.
- Este inventário cobre inicialmente apenas a adoção e a recuperação da próxima tarefa. **Não é equivalente ao tracker completo do escopo ativo do produto.** Expandir somente a partir de requisitos e critérios reais, ou mapear um tracker comprovadamente equivalente, antes de declarar adoção concluída.
- Não derivar tarefas de produção de propostas não aceitas, nem promover `Proposed` para `Accepted` por conveniência.
- A continuidade segue `docs/PROJECT_STATE.md`; o relatório do gate segue `docs/ADOPTION_REPORT.md`.

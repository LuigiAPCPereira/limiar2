# Relatório de inicialização documental — Limiar 3.0

**Estado:** `PREPARAÇÃO DOCUMENTAL PARCIAL` até verificação final e disponibilização integral do Engineering DNA na branch. **Modo:** tratar frente Limiar 3.0 como **projeto novo dentro de repositório existente** (§5 do protocolo v2), preservando `AGENTS.md`, a fonte canônica do protocolo, dados/código/ADRs e a adoção legada em PR #214 draft. Não reclassificar o Adoption Gate v2 do repositório antigo como concluído. Escopo autorizado nesta etapa = branch e documentação somente.

## Matriz das nove funções — completude não presumida

| Função obrigatória | Fonte desta frente | Estado e evidência / próximo gate |
| --- | --- | --- |
| Identidade, visão, público e exclusões | [`PRODUCT_AND_SCOPE.md`](PRODUCT_AND_SCOPE.md) | CRIADA; apresenta agrupador de promoções, Telegram/MCP/API/frontend e exclusões, objetivos do mantenedor; validar reabertura. Público principal pessoal, perfil multiusuário não aprovado. |
| Requisitos e aceites | [`PRODUCT_AND_SCOPE.md`](PRODUCT_AND_SCOPE.md) e TASKLIST | PARCIAL: critérios por etapa/fatia e lacunas explícitos; metas numéricas, scope de chats, plataformas e aceite detalhado de auth/MCP ainda desconhecidos; não inventar. |
| Arquitetura e contratos | [`ARCHITECTURE_PROPOSAL.md`](ARCHITECTURE_PROPOSAL.md) | CRIADA como PROPOSTA: alternativas de isolamento, owners, fluxo, MCP transversal, risks e dependencies; decisão final de estrutura, sessão e identidade ainda aberta. |
| Decisões duráveis | `docs/adr/README.md`, ADRs 016–020 Accepted e 021–024 Proposed | EXISTENTE (autoridades herdadas com status explícito); nova estratégia macro é escopo de produto, não promoção de ADR; não criar ADR retrospectivo. |
| Inventário de tarefas | [`TASKLIST.md`](TASKLIST.md) | CRIADO para esta frente com IDs L3-DOC-001 e L3-001–006, estados, aceites, deps, Evidência/lacunas e branch; confirmar reabertura. `docs/TASKLIST.md` aplica-se à adoção/rebaseline legados, não tracker concorrente desta nova frente. |
| Marcos | [`ARCHITECTURE_PROPOSAL.md`](ARCHITECTURE_PROPOSAL.md) seção M0–M5 | CRIADOS com resultados e ordem dependente, sem cronograma inventado. |
| Histórico recuperável | [`SESSION_LOG.md`](SESSION_LOG.md) | CRIADO somente a partir de fatos relatados/observados, sem inventar passado; PRs/ADRs citados com limites. |
| Checkpoint e próxima ação | [`PROJECT_STATE.md`](PROJECT_STATE.md) | CRIADO, vinculado `L3-DOC-001`/`L3-001`, branch/ref de origem e incógnitas; verificar HEAD final sem ciclo SHA. |
| Instruções e protocolo versionado | Root `AGENTS.md` + `docs/DOCUMENTATION_AND_CONTINUITY.md` v2.0 + [`START_HERE.md`](START_HERE.md) + [`ENGINEERING_GUIDE.md`](ENGINEERING_GUIDE.md) | PARCIAL: root AGENTS e protocolo herdados e lidos na ref de origem; novo fluxo documentado; **Engineering DNA original integral ainda não está comprovado presente na branch** e acesso da Project a Codex/agendamentos é desconhecido. Guia derivado não substitui original. |

## Verificações factuais e limitações

- Origem da branch: PR #214 draft HEAD `7177d929839512b8005ae379d96e3d224feac1f8`, main `796b7769449b72320b27c2557c2ccb8c8eb183e1`, consultados na preparação. Branch Limiar 3.0 criada com sucesso via GitHub; não ler isso como merge ou novo runtime.
- Protocolo Project SHA-256 `d078e0b3d4a8f9d4bd21cb0c7c8a3e417cba484981566d801ff8453ac7be1dab`, Git blob na origem `78b2e86564fb287886f9df065fdb727b20c52727`. Engineering DNA original Project SHA-256 `c19c5d97f8e42b311d6c15440e2e9f58cec64fadf1a48b9575e1f2e4dba0e373`; arquivo na raiz não foi encontrado na branch de origem. Disponibilizar e validar integralmente antes de alegar que outros agentes terão a mesma fonte.
- Fontes ADR 023/024 na revisão de origem retornaram `Proposed`; PR #202 (`EXP-019`) está mergeada sem runtime; PR #211 é fatia SQLite, não conclusão de migração. Status futuro deve ser rechecado.
- Sem inspeção local do repositório/worktree; via conector GitHub remoto apenas. Sem testes Go/race/benchmark, build, MCP/Telegram, dados reais, deploy ou merge realizados nesta preparação.
- Toda fonte criada nesta branch precisa ser reaberta na ref corrente, verificar links e HEAD antes de classificar como verificada. A falta de metas e de decisões produtivas não bloqueia registrar a visão, mas bloqueia declaração de implementação autorizada.

**Resultado operacional exigido:** novo chat acessa START_HERE e retoma `L3-001` como investigação. `L3-DOC-001` só pode passar a validada após conferência de arquivos, incluindo Engenharia DNA integral. Não declarar `ADOÇÃO CONCLUÍDA` para o PR #214 por esta preparação nem iniciar código sob a justificativa de que o plano foi escrito.
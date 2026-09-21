# Relatório de adoção — Agent Development Protocol v2.0 / Limiar 2

**Resultado: ADOÇÃO PARCIAL.** Modo Aplicar autorizado em 2026-09-20 para `LuigiAPCPereira/limiar2`, com intenção de continuar desenvolvimento somente dentro do escopo já aprovado. Não houve autorização específica para merge, deploy, promoção de ADR Proposed, destruição de histórico ou alterações sensíveis. Base inicialmente observada: `main@796b7769449b72320b27c2557c2ccb8c8eb183e1`; trabalho exclusivamente em `docs/adopt-agent-protocol-v2-20260920`, PR #214 draft. Consultar HEAD novamente após cada edição. O conector GitHub oferece árvore remota, não acesso a working tree local.

**Origem e integridade do protocolo:** arquivo `DOCUMENTATION_AND_CONTINUITY.md` v2.0 anexado a este ChatGPT Project, SHA-256 **medido localmente** `d078e0b3d4a8f9d4bd21cb0c7c8a3e417cba484981566d801ff8453ac7be1dab`, Git blob original calculado `78b2e86564fb287886f9df065fdb727b20c52727`. A transcrição anterior no repositório omitia `o ` em “retomar o desenvolvimento”; retirar esses dois bytes do original reproduziu exatamente o blob antigo `f78d5a7e6f755955c1a78eabbd9437f299eb0430`. A correção foi gravada no commit `b8a820748fde73d9cdc4088c5fc51d9b99bbee2d` e o documento foi **reaberto no GitHub** com blob `78b2e86564fb287886f9df065fdb727b20c52727`, idêntico à fonte. O `AGENTS.md` com entrada atualizada foi reaberto no commit `837b255bbb5dc2ec343d7bf56e2edbd16405be8d`. Isso comprova bytes na ref consultada, não sincronização automática do Project nem execução de Codex/Tarefas Agendadas.

## Matriz de cobertura — nove funções obrigatórias

| Função | Fonte e revisão observada | Estado | Evidência e lacuna / ação |
| --- | --- | --- | --- |
| 1. Identidade, visão, público e exclusões | `README.md`, `docs/PRODUCT_BRIEF.md`, `docs/BASELINE.md` em `main@796b7769` | PENDENTE — fontes existentes, cobertura parcial | Propósito Telegram/promoções e exclusões documentados; briefing descreve stack/fases legadas e BASELINE contém a rebaseline, mas público e escopo vigente precisam de referência reconciliada. ADOPT-003. |
| 2. Requisitos e aceites | `docs/PRODUCT_BRIEF.md`, `docs/CONTEXT.md`, `docs/specs/` e ADRs na base | PENDENTE | Objetivos/fases existem, mas cobertura íntegra dos requisitos versionados e critérios verificáveis do escopo ativo não demonstrada. Mapear fonte por item, sem PRD concorrente. ADOPT-003. |
| 3. Arquitetura e contratos | `AGENTS.md`, `docs/BASELINE.md`, `docs/ARCHITECTURE.md`, ADRs Accepted na base | PENDENTE — divergência comprovada | Constituição governa precedência. ADR 018 confirmado Accepted no próprio registro (issue #212); BASELINE da branch consultada ainda o chama Proposed. Corrigir documento derivado sem alterar decisão ou contratos; revisar responsabilidades e contratos atuais. ADOPT-003. |
| 4. Decisões duráveis | `docs/adr/README.md` e ADRs 016–024 em `main@796b7769`, ADR 018 reaberto na branch | EXISTENTE E VERIFICADA quanto ao registro decisório | Registro de transição e estados presentes. ADR 018 Accepted com referência explícita; ADR 024 Proposed, assim como 021–023 na ref inspecionada. Não fabricar aceites nem promover propostas. |
| 5. Inventário de tarefas | `docs/TASKLIST.md` na branch do PR #214, reaberto antes e após correção de fonte | PENDENTE — fonte criada, abrangência não comprovada | ADOPT-001–004, DEV-001 e PROD-001–008 têm IDs, estados, dependências, aceites, fontes e evidência/ausência. Mapeiam frentes conhecidas, não prova de enumeração exaustiva do escopo de produto; issues/PRs narrativos tampouco foram demonstrados equivalentes. ADOPT-003. |
| 6. Marcos e planejamento | `docs/ROADMAP.md` criado/reaberto na branch; `docs/CONTEXT.md` e `docs/BASELINE.md` na base | CRIADA E VERIFICADA para sequência documental delimitada; cobertura global pendente | M0–M4 têm resultados verificáveis, dependências e IDs, sem datas inventadas; fases históricas separadas da Rebaseline. Reconciliar marcos com escopo e inventário completo em ADOPT-003. |
| 7. Histórico recuperável | `docs/evolution/`, ADRs, PRs/commits, docs históricos na base; PR #214 | PENDENTE — equivalência incompleta | Registro EvolutionDocs preserva motivo/experimentação; PR #214 registra eventos de adoção. Verificar se histórico de transição necessário está recuperável ou iniciar SESSION_LOG somente de agora em diante. ADOPT-003. |
| 8. Checkpoint e próxima ação | `docs/PROJECT_STATE.md` na branch | CRIADA E VERIFICADA quanto à estrutura; reconciliação final pendente | Aponta IDs reais ADOPT-002/ADOPT-003, refs, PR, bloqueios, estados separados e próxima ação; atualizar depois da validação de integridade e confrontar com HEAD final e inventário integral. ADOPT-004. |
| 9. Instruções e versão | `AGENTS.md` + `docs/DOCUMENTATION_AND_CONTINUITY.md` na branch; commits `b8a8207` e `837b255b` | EXISTENTE E VERIFICADA na ref GitHub; portabilidade não verificada | Fonte v2 idêntica ao original por Git blob e SHA-256 de origem; entrada operacional §18 reaberta, regras constitucionais anteriores preservadas pela conferência do patch do PR. Codex/agendamentos não testados; não inferir acesso. ADOPT-004 cobre revisão final. |

## Alterações e preservação

- Nesta branch/PR: `docs/TASKLIST.md` expandido e atualizado, `docs/PROJECT_STATE.md` e este relatório em atualização; criados `docs/DOCUMENTATION_AND_CONTINUITY.md` (corrigido até identidade exata) e `docs/ROADMAP.md`; `AGENTS.md` recebeu seção 18/mapa sem substituir as seções constitucionais e foi atualizado para refletir a validação da fonte. Verificar diff final do PR, pois arquivos podem receber novas revisões.
- Sem edição intencional de código, testes, ADRs, README, BASELINE, PRODUCT_BRIEF, CONTEXT ou documentação histórica nesta etapa. A divergência ADR 018/BASELINE continua explicitamente pendente.
- Sem PRD, DESIGN, PRODUCT, ADR ou registros históricos retroativos criados apenas por ritual. ROADMAP não substitui TASKLIST; PROJECT_STATE não é tracker.
- Reaberturas confirmadas da fonte v2, AGENTS, TASKLIST e deste relatório ao longo da branch. Conferência completa de todos os links, idempotência, autorizações e fontes no HEAD final ainda não concluída.

## Integridade e estados separados

- **Fonte: CONFIRMADA na branch.** Original local SHA-256 `d078e0b3d4a8f9d4bd21cb0c7c8a3e417cba484981566d801ff8453ac7be1dab` e Git blob `78b2e86564fb287886f9df065fdb727b20c52727`; arquivo reaberto no GitHub após correção apresenta exatamente esse blob. Isso comprova identidade byte a byte, não a leitura por outros hosts.
- **Git:** PR #214 é draft e não foi integrado na última consulta; reconsultar após última gravação. `main` não contém estas mudanças enquanto não houver merge confirmado.
- **Produto:** implementação preexistente de SQLite/Evidence documentada pela BASELINE e PR #211; nenhum teste Go/CI ou runtime foi executado nesta adoção; validações históricas são restritas às revisões/ambientes em que ocorreram. Nenhum deploy executado ou verificado.
- **Portabilidade:** acesso real de Codex, Project sincronizado e Tarefas Agendadas não testados; não criar agendamento por inferência.

## Pendências e próxima ação

1. **ADOPT-002:** integridade do documento e entrada AGENTS confirmadas na branch; acesso Codex/agendamentos permanece NÃO VERIFICADO e deve ser ensaiado somente em ambiente/execução efetivamente autorizado, sem simular resultados.
2. **ADOPT-003 — próxima tarefa:** reconciliar público/escopo e aceites, demonstrar cobertura de TODAS as tarefas ativas do produto, confrontar marcos e história, corrigir referência desatualizada do ADR 018 na BASELINE mediante leitura da fonte decisória.
3. **ADOPT-004:** reabrir fontes escritas na revisão final, validar caminhos e referências, evitar autoridades duplicadas, reconciliar PR/HEAD e checkpoint e aplicar todas as condições do Gate v2.
4. **DEV-001:** só selecionar bloco de código com autorização/Decision Accepted, fatia definida, dependências e verificação; ADR 024 permanece Proposed e não foi aceito pelo pedido de continuidade.

**Estado inequívoco: ADOÇÃO PARCIAL.** A presença de arquivos e PR não equivale a Adoption Gate passado nem a produto concluído.

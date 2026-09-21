# Relatório de adoção — Agent Development Protocol v2.0 / Limiar 2

**Resultado: ADOÇÃO PARCIAL.** Modo Aplicar autorizado em 2026-09-20 para `LuigiAPCPereira/limiar2`, com intenção de continuar desenvolvimento somente dentro do escopo já aprovado. Não houve autorização específica para merge, deploy, promoção de ADR Proposed, destruição de histórico ou alterações sensíveis. Base observada: `main@796b7769449b72320b27c2557c2ccb8c8eb183e1`; trabalho exclusivamente em `docs/adopt-agent-protocol-v2-20260920`, PR #214 draft. Consultar HEAD novamente após cada edição. O conector GitHub oferece árvore remota, não acesso a working tree local.

**Origem do protocolo:** kit anexo ao ChatGPT Project, `DOCUMENTATION_AND_CONTINUITY.md` v2.0, SHA-256 indicado pelo kit `d078e0b3d4a8f9d4bd21cb0c7c8a3e417cba484981566d801ff8453ac7be1dab`, Git blob calculado sobre arquivo local `78b2e86564fb287886f9df065fdb727b20c52727`. A versão transcrita e reaberta no GitHub em `docs/DOCUMENTATION_AND_CONTINUITY.md` possui Git blob `f78d5a7e6f755955c1a78eabbd9437f299eb0430`, **distinto da origem**. Identidade byte a byte NÃO verificada: comparar conteúdo e substituir pelo original exato antes de validar ADOPT-002. Não supor atualização automática da cópia do Project nem acesso em Codex/Tarefas Agendadas.

## Matriz de cobertura — nove funções obrigatórias

| Função | Fonte e revisão observada | Estado | Evidência e lacuna / ação |
| --- | --- | --- | --- |
| 1. Identidade, visão, público e exclusões | `README.md`, `docs/PRODUCT_BRIEF.md`, `docs/BASELINE.md` em `main@796b7769` | PENDENTE — fontes existentes, cobertura parcial | Propósito Telegram/promoções e exclusões documentados; briefing descreve stack/fases legadas e BASELINE contém a rebaseline, mas público e escopo vigente precisam de referência reconciliada. ADOPT-003. |
| 2. Requisitos e aceites | `docs/PRODUCT_BRIEF.md`, `docs/CONTEXT.md`, `docs/specs/` e ADRs na base | PENDENTE | Objetivos/fases existem, mas cobertura íntegra dos requisitos versionados e critérios verificáveis do escopo ativo não demonstrada. Mapear fonte por item, sem PRD concorrente. ADOPT-003. |
| 3. Arquitetura e contratos | `AGENTS.md`, `docs/BASELINE.md`, `docs/ARCHITECTURE.md`, ADRs Accepted na base | PENDENTE — divergência comprovada | Constituição governa precedência. ADR 018 está Accepted em seu registro (issue #212), mas BASELINE consultada ainda o chama Proposed. Corrigir derivado sem alterar decisão ou contratos; revisar responsabilidades e contratos atuais. ADOPT-003. |
| 4. Decisões duráveis | `docs/adr/README.md` e ADRs 016–024 em `main@796b7769` | EXISTENTE E VERIFICADA quanto ao registro decisório | Registro de transição e estados presentes. ADR 018 Accepted com referência explícita; ADR 024 Proposed, assim como 021–023 na ref inspecionada. Não fabricar aceites nem promover propostas. |
| 5. Inventário de tarefas | `docs/TASKLIST.md` na branch do PR #214, reaberto após expansão | PENDENTE — fonte criada, abrangência não comprovada | ADOPT-001–004, DEV-001 e PROD-001–008 têm IDs, estados, dependências, aceites, fontes e evidência/ausência. Mapeiam frentes conhecidas, não prova de enumeração exaustiva do escopo de produto; issues/PRs narrativos tampouco foram demonstrados equivalentes. ADOPT-003. |
| 6. Marcos e planejamento | `docs/ROADMAP.md` criado/reaberto na branch; `docs/CONTEXT.md` e `docs/BASELINE.md` na base | CRIADA E VERIFICADA para sequência documental delimitada; cobertura global pendente | M0–M4 têm resultados verificáveis, dependências e IDs, sem datas inventadas; fases históricas separadas da Rebaseline. Reconciliar marcos com escopo e inventário completo em ADOPT-003. |
| 7. Histórico recuperável | `docs/evolution/`, ADRs, PRs/commits, docs históricos na base; PR #214 | PENDENTE — equivalência incompleta | Registro EvolutionDocs preserva motivo/experimentação; PR #214 registra eventos de adoção. Verificar se histórico de transição necessário está recuperável ou iniciar SESSION_LOG somente de agora em diante. ADOPT-003. |
| 8. Checkpoint e próxima ação | `docs/PROJECT_STATE.md` na branch | CRIADA E VERIFICADA quanto à estrutura; reconciliação final pendente | Aponta IDs reais ADOPT-002/ADOPT-003, refs, PR, bloqueios, estados separados e próxima ação; reabrir depois de atualização, confrontar com HEAD final e inventário integral. ADOPT-004. |
| 9. Instruções e versão | `AGENTS.md` + `docs/DOCUMENTATION_AND_CONTINUITY.md` na branch | PENDENTE — integração realizada, integridade aberta | PR patch demonstra apenas acréscimo da seção operacional 18 ao AGENTS constitucional e mapa de nove funções, sem remoção das regras anteriores. Fonte v2 existe no GitHub, mas blob divergente do kit; acesso em Codex/agendamentos não testado. ADOPT-002/004. |

## Alterações e preservação

- Nesta branch/PR: `docs/TASKLIST.md` expandido, `docs/PROJECT_STATE.md` atualizado, este relatório atualizado; criados `docs/DOCUMENTATION_AND_CONTINUITY.md` e `docs/ROADMAP.md`; `AGENTS.md` recebeu seção 18 e mapa das nove funções, preservando as seções constitucionais anteriores. Verificar diff final do PR, pois arquivos podem receber novas revisões.
- Sem edição intencional de código, testes, ADRs, README, BASELINE, PRODUCT_BRIEF, CONTEXT ou documentação histórica. A divergência ADR 018/BASELINE continua explicitamente pendente.
- Sem PRD, DESIGN, PRODUCT, ADR ou registros históricos retroativos criados apenas por ritual. ROADMAP não substitui TASKLIST; PROJECT_STATE não é tracker.
- Reabertura confirmada para TASKLIST, ROADMAP, fonte v2 e entrada AGENTS; reabrir relatório/checkpoint depois de suas atualizações. Conferência completa de todos os links, hash de arquivo integral, idempotência, autorizações e fonte na ref final ainda não concluída.

## Integridade e estados separados

- **Fonte:** divergência do Git blob do documento do repositório em relação ao arquivo exato do Project; não declarar hashes SHA-256 idênticos com base apenas no texto ou no nome da versão.
- **Git:** PR #214 é draft e não foi integrado; consultar HEAD após a última gravação. `main` não contém estas mudanças enquanto não houver merge confirmado.
- **Produto:** implementação preexistente de SQLite/Evidence documentada pela BASELINE e PR #211; nenhum teste Go/CI ou runtime foi executado nesta sessão; validações históricas são restritas às revisões/ambientes em que ocorreram. Nenhum deploy executado ou verificado.
- **Portabilidade:** acesso real de Codex, Project sincronizado e Tarefas Agendadas não testados; não criar agendamento por inferência.

## Pendências e próxima ação

1. **ADOPT-002:** substituir ou confirmar a fonte com bytes originais e hash verificado; validar links da entrada AGENTS e capacidade de leitura efetiva de ambientes pertinentes, sem declarar acesso não testado.
2. **ADOPT-003:** reconciliar público/escopo e aceites, demonstrar cobertura de TODAS as tarefas ativas do produto, confrontar marcos e história, corrigir referência desatualizada do ADR 018 na BASELINE mediante leitura da fonte decisória.
3. **ADOPT-004:** reabrir fontes escritas na revisão final, validar caminhos e referências, evitar autoridades duplicadas, reconciliar PR/HEAD e checkpoint e aplicar todas as condições do Gate v2.
4. **DEV-001:** só selecionar bloco de código com autorização/Decision Accepted, fatia definida, dependências e verificação; ADR 024 permanece Proposed e não foi aceito pelo pedido de continuidade.

**Estado inequívoco: ADOÇÃO PARCIAL.** A presença de arquivos e PR não equivale a Adoption Gate passado nem a produto concluído.

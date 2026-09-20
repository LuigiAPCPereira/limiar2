# Relatório de adoção — Agent Development Protocol v2.0 / Limiar 2

- **Resultado: ADOÇÃO PARCIAL.** Modo Aplicar, autorizado pelo usuário em 2026-09-20 para alterações documentais e intenção de continuar desenvolvimento dentro do escopo já aprovado; não houve autorização para merge, deploy, aceitar ADRs ou criar tarefas agendadas.
- Repositório: `LuigiAPCPereira/limiar2`; base verificada `main@796b7769449b72320b27c2557c2ccb8c8eb183e1` em 2026-09-20; alterações apenas na branch `docs/adopt-agent-protocol-v2-20260920`. HEAD da branch deve ser consultado novamente após as escritas. Conector GitHub remoto não fornece inspeção do worktree local.
- Fonte do protocolo lida: `DOCUMENTATION_AND_CONTINUITY.md` v2.0 anexado ao ChatGPT Project, SHA-256 local verificado `d078e0b3d4a8f9d4bd21cb0c7c8a3e417cba484981566d801ff8453ac7be1dab`; `AGENTS_TEMPLATE.md` SHA-256 `85f73cddf49d0e7a0a890edb3441c396b17679ae3c6b1b72ad9b7065675ebd83`. A cópia do Project coincide com `SHA256SUMS.txt` do pacote fornecido, **mas não existe cópia integral verificada do protocolo no repositório**; ausência de teste de acesso em Codex/Tarefas Agendadas. Kit disponível não equivale a implantação.

## Matriz de nove funções — Gate v2

`EXISTENTE E VERIFICADA` significa apenas que a fonte e o conteúdo especificado foram lidos na ref observada, não que toda a função esteja coberta. Por essa razão funções incompletas são `PENDENTE`, ainda que exista um arquivo parcial. `CRIADA E VERIFICADA` exige reabertura na branch.

| Função | Fonte real / revisão | Estado | Evidência objetiva | Lacuna ou ação |
| --- | --- | --- | --- | --- |
| 1. Identidade, visão, público, exclusões | `README.md`, `docs/PRODUCT_BRIEF.md`, `docs/BASELINE.md` na base `796b7769` | PENDENTE — fonte existente, cobertura parcial | Propósito de pipeline Telegram/ofertas e exclusões constam; PRODUCT_BRIEF descreve stack legada e afirma Tursogo banco único enquanto BASELINE descreve novo SQLite side-by-side | Mapear público explicitamente e autoridade do escopo atual sem reescrever documentação histórica como verdade nova (`ADOPT-003`) |
| 2. Requisitos e aceites | `docs/PRODUCT_BRIEF.md`, `docs/CONTEXT.md`, `docs/specs/`, ADRs na base | PENDENTE — não comprovado | Fases e objetivos disponíveis; nenhum inventário verificado de requisitos ativos, aceites e versão/escopo completo da Rebaseline | Mapear fontes de requisitos e aceites verificáveis item a item (`ADOPT-003`); não criar PRD paralelo sem necessidade |
| 3. Arquitetura e contratos | `AGENTS.md`, `docs/BASELINE.md`, `docs/ARCHITECTURE.md`, ADRs na base | PENDENTE — fontes existentes com divergência | `AGENTS.md` fixa ordem Constitucional/ADRs Accepted/BASELINE; README distingue arquitetura legada do SQLite novo | Conferir contratos por autoridade e reconciliar BASELINE: ADR 018 já `Accepted` no próprio arquivo, mas descrito como `Proposed` na BASELINE (`ADOPT-003`) |
| 4. Decisões duráveis | `docs/adr/README.md`, ADRs 016–024 na base | EXISTENTE E VERIFICADA — registro decisório | ADR 018 declara `Accepted` com `Acceptance-reference` issue #212; ADR 024 declara `Proposed`; registry governa ADRs antigos | Preservar; nenhuma ADR retroativa, aceitação ou mudança de arquitetura inventada |
| 5. Inventário de tarefas | `docs/TASKLIST.md` na branch documental | PENDENTE — inventário de adoção criado, escopo do produto incompleto | IDs `ADOPT-001`–`ADOPT-004`, `DEV-001`, estados, dependências, aceite e lacunas de evidência registrados; arquivo reaberto na branch | Abranger **todas as tarefas ativas do produto** ou demonstrar tracker equivalente campo a campo; PR narrativo não substitui inventário (`ADOPT-003`) |
| 6. Marcos / roadmap | Seção Roadmap em `docs/CONTEXT.md` na base; `docs/BASELINE.md` §7 | PENDENTE — cobertura não demonstrada | CONTEXT lista fases 1–6; BASELINE lista questões abertas da Rebaseline e gates | Distinguir roadmap histórico de plano vigente com sequência, dependências, resultados e IDs de tarefas; criar ROADMAP somente se não houver equivalente suficiente (`ADOPT-003`) |
| 7. Histórico recuperável | `docs/evolution/README.md`, findings/experiments/proposals, ADRs e PRs/commits | PENDENTE — equivalência incompleta | EvolutionDocs define Evidence→Finding→Experiment→Proposal→ADR; commits recentes #211–#213 localizáveis | Verificar se registros preservam motivos, marcos e links da transição; se não, iniciar SESSION_LOG a partir desta adoção sem reconstituir passado (`ADOPT-003`) |
| 8. Checkpoint / próxima ação | `docs/PROJECT_STATE.md` na branch documental | CRIADA E VERIFICADA somente quanto ao checkpoint inicial | Aponta `ADOPT-001`, inventário `docs/TASKLIST.md`, base/branch, bloqueios e próxima ação; conferir reabertura | Reconciliar com inventário completo, PR e HEAD final; campo de produto permanece desconhecido (`ADOPT-004`) |
| 9. Instruções / versão do protocolo | `AGENTS.md` na base; protocolo v2.0 apenas no Project | PENDENTE — falta integração efetiva | `AGENTS.md` existente possui autoridade Constitucional e restrição explícita a mudanças arquiteturais; protocolo do Project tem hash conferido | Instalar protocolo integral na ref real, mesclar instruções sem sobrescrever constituição, verificar hashes/links e acesso de cada ambiente (`ADOPT-002`) |

## Alterações, preservação e integridade

- Criados na branch documental: `docs/TASKLIST.md`, `docs/PROJECT_STATE.md`, este `docs/ADOPTION_REPORT.md` (reabrir este último após a escrita antes de contar como validado).
- Preservados sem edição: `AGENTS.md`, `README.md`, `docs/BASELINE.md`, `docs/PRODUCT_BRIEF.md`, `docs/CONTEXT.md`, `docs/ARCHITECTURE.md`, `docs/adr/`, `docs/evolution/`, código e testes existentes.
- Não foram criados `PRODUCT.md`, `PRD.md`, `DESIGN.md`, `ADR` ou `ROADMAP.md` por ritual; equivalência e lacunas ainda precisam ser demonstradas.
- Conferência de links: links relativos do TASKLIST para o checkpoint e relatório e do checkpoint para TASKLIST e relatório estão alinhados aos caminhos planejados; validação completa de todos os links, duplicação/idempotência e revisão final pendente.
- CI do produto, testes Go, runtime, working tree local, merge e deploy **não verificados**; adoção documental não prova qualquer uma dessas dimensões.

## Lacunas impeditivas — ordem verificável

1. `ADOPT-002`: incorporar integralmente o protocolo v2.0 na fonte canônica do repositório, verificar hash e mesclar entrada em `AGENTS.md` sem alterar autoridade local; comparar cópia do Project por hash e testar acessibilidade real de cada ambiente antes de alegar portabilidade.
2. `ADOPT-003`: recuperar escopo e aceites do produto, inventariar todas as tarefas ativas ou comprovar tracker equivalente, reconciliar marcos/histórico e a divergência ADR 018–BASELINE.
3. `ADOPT-004`: verificar/refazer checkpoint no HEAD real, reabrir fontes, conferir links e duplicatas, confirmar todas as nove linhas sem pendências e só então reavaliar o Gate.
4. `DEV-001`: desenvolvimento posterior depende de decisão `Accepted`, fatia identificável e Implementation Gate; ADR 024 está `Proposed` e este pedido não o aceita.

**Estado inequívoco nesta revisão:** ADOÇÃO PARCIAL. Nenhuma declaração de adoção concluída, produto pronto, PR integrado ou deploy executado.

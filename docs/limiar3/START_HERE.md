# Limiar 3.0 — COMECE AQUI

**Estado:** planejamento/documentação da nova geração, sem implementação autorizada por esta entrega. **Repositório:** `LuigiAPCPereira/limiar2`. **Branch de planejamento:** `docs/limiar-3-foundation-20260922`, criada a partir do commit `7177d929839512b8005ae379d96e3d224feac1f8` da branch documental de adoção v2 (PR #214 draft), que por sua vez partiu de `main@796b7769449b72320b27c2557c2ccb8c8eb183e1`. Estas são referências históricas de origem; **reconsultar HEAD, PRs, divergência com main e testes na execução seguinte**. A `main` não foi modificada por esta preparação. Não pressupor worktree local, acesso Telegram, deploy, credenciais, MCP conectado ou sincronização com arquivos do ChatGPT Project.

## Leia nesta ordem

1. `AGENTS.md` na raiz — Constituição do repositório, ordem de autoridades, limites de permissão e interpretação dos ADRs.
2. `docs/DOCUMENTATION_AND_CONTINUITY.md` — Agent Development Protocol v2.0; para Limiar 3.0 aplicar sobretudo §5 (inicialização de projeto novo), §7 (RECOVER/RECONCILE), §8 (Implementation Gate) e §9 (tarefas). A adoção v2 do repositório anterior permanece **PARCIAL** no PR #214; não declarar Adoption Gate cumprido sem conferir todas as nove funções e evidências.
3. `ENGINEERING_DNA.md` na raiz desta branch **se existir e for verificado integralmente**; em caso contrário ler o arquivo canônico anexado ao ChatGPT Project, se realmente acessível, e registrar a lacuna. O documento local de princípios condensados `docs/limiar3/ENGINEERING_GUIDE.md` é orientação derivada e **não substitui o original integral**.
4. `docs/limiar3/PRODUCT_AND_SCOPE.md` — visão e requisitos explícitos do mantenedor, inclusive MCP.
5. `docs/limiar3/ARCHITECTURE_PROPOSAL.md` — alternativas, responsabilidades, riscos, reutilização e decisões pendentes; proposta, não ADR Accepted.
6. `docs/limiar3/TASKLIST.md` — inventário próprio da frente Limiar 3.0, sem alterar nem suplantar o tracker da rebaseline anterior `docs/TASKLIST.md` (que governa somente aquela frente). Não duplicar tarefas dentro da frente Limiar 3.0.
7. `docs/limiar3/PROJECT_STATE.md` — checkpoint com ID de tarefa e próximo passo; registro derivado, não nova autoridade de requisitos.
8. `docs/BASELINE.md`, `docs/adr/README.md` e ADRs Accepted/Proposed pertinentes; verificar ref e estados vigentes antes de qualquer código.

## Instrução direta para o próximo chat

**Pedido já respondido pelo mantenedor:** reconstruir a implementação integral do Limiar como Limiar 3.0, incrementalmente e de baixo para cima, com separação clara de ownership, desempenho mensurável, segurança, robustez, legibilidade e manutenção de longo prazo; incluir o MCP em paralelo como extensão que permita ao ChatGPT ler dados/mensagens autorizados e consultar promoções. Produto: Telegram → coleta/Evidence → processamento/agrupamento → dados limpos → API → frontend; MCP é superfície transversal de acesso controlado, capaz de consultar mensagens admitidas e dados processados, **não** um substituto da coleta nem dependência obrigatória da persistência/processing. **Não existe um segundo 'aplicativo Limiar' no escopo.**

**Tarefa ativa de continuidade:** `L3-001` — investigar o projeto inicial e a primeira capacidade de autenticação/sessão, recuperando a revisão real, comparando opções mínimas de isolamento e confrontando contratos já aceitos com ADR 023 Proposed. O usuário disse que abrirá outro chat para esta **investigação**. Não codificar, criar novos ambientes, aceitar ADRs, conectar Telegram/MCP, criar credenciais, migrar dados, fazer merge ou deploy automaticamente por causa deste handoff. Ao fim da investigação, entregar recomendação técnica, dependências, decisões que exigem aceite, primeiro slice verificável e atualizar tracker/checkpoint apenas se o novo pedido conceder escrita.

**Distinção de autenticações:** (a) sessão MTProto sensível usada pelo coletor para conectar-se ao Telegram; (b) eventual identidade/autorização para consumidores do Limiar e acesso pelo ChatGPT/MCP. Não pressupor que (b) já existe no legado, nem reutilizar diretamente arquivo de sessão Telegram como token MCP. Investigar autenticação de consumidor somente na medida em que for dependência real do primeiro slice, sem inventar sistema de contas.

**Autorização desta entrega:** criar esta branch e documentação. Nada de código, novos serviços, custos, operações com dados reais ou merge. Um futuro pedido explícito pode alterar esse limite. Não exigir repetição das respostas do mantenedor contidas nestes documentos; perguntar só decisão material ainda não respondida.

## Próximas leituras técnicas focadas

- Sessão: ADR 004 sob registry de transição; ADR 023 **Proposed**; F-STO-006; EXP-LIMIAR-018 Rejected para FileStorage as-is e EXP-LIMIAR-019 Supported apenas Unix/Linux intra-processo, integrado pelo PR #202, **sem runtime de produção**.
- Coleta/Evidence: ADRs 016–020 Accepted; ADR 018 Accepted mas Source Admission ainda não integrada em produção; ADR 024 **Proposed** para `subscription_id` (não inventar ID de produção).
- Persistência: `internal/storage/sqlite`, migrations; PR #211 integrada (guard de schema), não equivale a substituição completa Tursogo. ADRs 021/022 ainda Proposed; importação real EXP-007 não demonstrada.
- Imagens: falha funcional relatada pelo mantenedor, causa não demonstrada neste trabalho; ADR 011 `DEFER / REVALIDATE`, ADR 012 `RETIRE`.

**Critério de sucesso da preparação documental:** arquivos reabertos na branch correta, referências coerentes, escopo e tarefas recuperáveis, nenhuma pretensão de produto validado. O checkpoint informa o que foi efetivamente conferido.
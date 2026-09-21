# Evidências de escopo e trabalho paralelo — adoção v2 / Limiar 2

**Natureza:** diagnóstico derivado para `ADOPT-003`, não PRD, decisão, autorização, tracker nem substituto de `TASKLIST.md`. **Consulta:** repositório `LuigiAPCPereira/limiar2`, PR de adoção #214, branch `docs/adopt-agent-protocol-v2-20260920` observada inicialmente no HEAD `16ff240c479aefd1e16b80d064cb9800e644bcc1`. Alterações posteriores exigem nova consulta. Não assumir working tree local, execução Codex nem acesso em tarefas agendadas.

## 1. Escopo sustentado e fronteiras de autoridade

| Pergunta | Fonte verificada | Resultado e limite |
| --- | --- | --- |
| Para que serve? | [`README.md`](../README.md), [`BASELINE.md`](BASELINE.md) §§1–3 | Plataforma pessoal de agregação e inteligência de promoções; fonte Telegram concreta atual; Evidence preservada e processamento derivado reconstruível. Público mais específico, entrega e versão formal do escopo ativo não demonstrados. |
| Qual arquitetura manda? | [`AGENTS.md`](../AGENTS.md) §§2, 4, 8 e ADRs `Accepted`; [`BASELINE.md`](BASELINE.md) | Constituição e Decisions aceitas prevalecem sobre briefing, specs, código e EvolutionDocs. ADRs 016–020 fornecem contratos da rebaseline nos escopos explicitados; aceitação não equivale a implementação nem validação produtiva. |
| Qual decisão recente está autorizada? | [ADR 018](adr/018-source-admission-before-updates-manager.md), [issue #212](https://github.com/LuigiAPCPereira/limiar2/issues/212) | Source Admission live/recovery aceito; a issue delimita que não autoriza produção automática, exige gates e orienta PR técnica separada com código/teste/evidência. ADR 021/022/023 e ADR 024 não foram aceitos por essa issue. |
| O que é legado/histórico? | [`PRODUCT_BRIEF.md`](PRODUCT_BRIEF.md), [`CONTEXT.md`](CONTEXT.md), [`README.md`](../README.md) | Brief/Context descrevem Tursogo único, fases legadas e futuros que não substituem a rebaseline side-by-side do ADR 019. Não promover fases 3–6, frontend, IA ou discovery a tarefas atuais só pela presença nesses documentos. Preservar o histórico. |
| Onde estão as tarefas vigentes? | [`TASKLIST.md`](TASKLIST.md) | Escopo **documental de adoção** tem ADOPT-001–004 e DEV-001; PROD-001–008 são frentes conhecidas mapeadas a ADRs/BASELINE, não prova de escopo de produto aprovado e exaustivo. Não foi localizada fonte que fixe toda a versão de requisitos/aceites vigentes do produto. |
| Como recuperar o histórico? | [`evolution/README.md`](evolution/README.md), ADRs, issue #212, PRs e commits | EvolutionDocs preserva Findings/Experiments/Proposals não autoritativos; ADRs registram Decisions. Há trilha histórica observável para os itens consultados, mas equivalência de todo histórico relevante não foi provada. |

**Critério de aceite ainda ausente para fechar ADOPT-003:** uma fonte efetivamente autorizada que identifique versão e limites do escopo ativo do produto, critérios verificáveis associados, e a enumeração de todas as tarefas pertinentes; ou evidência de que a cobertura atual já é exaustiva. Não declarar equivalência ou criar PRD canônico por inferência. Se não houver, manter `ADOÇÃO PARCIAL` com ausência explícita.

## 2. Inventário de PRs abertos, separado de tarefas autorizadas

Pesquisa GitHub `repo:LuigiAPCPereira/limiar2 is:open` segmentada por criação antes/depois de 2026-09-01 encontrou **22 PRs**: o draft [#214](https://github.com/LuigiAPCPereira/limiar2/pull/214), e os **21 PRs abaixo** (criados em agosto de 2026). Busca separada `is:issue is:open` retornou zero issues não-PR. Isso descreve o estado da busca, não aprova nem cancela os PRs e não prova que toda tarefa existe em issue.

| PR | Escopo conforme título/corpo consultado | Status no inventário |
| --- | --- | --- |
| [#121](https://github.com/LuigiAPCPereira/limiar2/pull/121) | Abas WAI-ARIA | Aberto na busca; vigência/aceite atual desconhecidos |
| [#122](https://github.com/LuigiAPCPereira/limiar2/pull/122) | Abas WAI-ARIA | Idem |
| [#123](https://github.com/LuigiAPCPereira/limiar2/pull/123) | Listas SSE e status acessíveis | Idem |
| [#124](https://github.com/LuigiAPCPereira/limiar2/pull/124) | Listas SSE acessíveis | Idem |
| [#125](https://github.com/LuigiAPCPereira/limiar2/pull/125) | Listas SSE e status acessíveis | Idem |
| [#126](https://github.com/LuigiAPCPereira/limiar2/pull/126) | Listas SSE acessíveis | Idem |
| [#127](https://github.com/LuigiAPCPereira/limiar2/pull/127) | Listas SSE e título dinâmico | Idem |
| [#128](https://github.com/LuigiAPCPereira/limiar2/pull/128) | Regiões SSE e status acessíveis | Idem |
| [#129](https://github.com/LuigiAPCPereira/limiar2/pull/129) | Abas, listas SSE e status acessíveis | Idem |
| [#130](https://github.com/LuigiAPCPereira/limiar2/pull/130) | Listas SSE, status e título dinâmico | Idem |
| [#133](https://github.com/LuigiAPCPereira/limiar2/pull/133) | Listas SSE acessíveis | Idem |
| [#134](https://github.com/LuigiAPCPereira/limiar2/pull/134) | Listas SSE acessíveis | Idem |
| [#137](https://github.com/LuigiAPCPereira/limiar2/pull/137) | Listas SSE e status acessíveis | Idem |
| [#138](https://github.com/LuigiAPCPereira/limiar2/pull/138) | Abas, listas SSE e status acessíveis | Idem; inclui mudança alheia à UI em `internal/storage/db.go` |
| [#139](https://github.com/LuigiAPCPereira/limiar2/pull/139) | Abas WAI-ARIA | Idem |
| [#140](https://github.com/LuigiAPCPereira/limiar2/pull/140) | Listas SSE, status e rótulo de botão | Idem |
| [#141](https://github.com/LuigiAPCPereira/limiar2/pull/141) | Listas SSE, status e título dinâmico | Idem |
| [#142](https://github.com/LuigiAPCPereira/limiar2/pull/142) | Listas SSE e status acessíveis | Idem |
| [#144](https://github.com/LuigiAPCPereira/limiar2/pull/144) | Listas SSE, status e título dinâmico | Idem |
| [#146](https://github.com/LuigiAPCPereira/limiar2/pull/146) | Semântica ARIA para listas e status | Idem |
| [#147](https://github.com/LuigiAPCPereira/limiar2/pull/147) | Listas SSE, status e título dinâmico | Aberto/não merged verificado individualmente; vigência desconhecida |

**Colisão concreta, não só similaridade de títulos:** patches consultados dos PRs #121, #138 e #147 alteram `internal/dashboard/index.html`; #121 e #138 inserem atributos/painéis de abas em regiões coincidentes; #138 e #147 também alteram regiões de listas/status SSE. O PR #138 modifica adicionalmente `internal/storage/db.go` inserindo uma linha em branco sem relação com a11y. Esse exame de amostras demonstra sobreposição, **não** classifica qual PR deve entrar, qual pode ser fechado nem qual foi totalmente superado por `main`. Não foi executado teste de integração dessas branches.

**Conclusão operacional:** registrar esses PRs como trabalho paralelo sob triagem no tracker, sem 21 tarefas fictícias de produto. Antes de selecionar uma fatia de dashboard, conferir autorização de escopo vigente, comparar cada diff à `main` atual, deduplicar critérios e decidir tratamento de PRs individualmente com autorização apropriada. Não fechar/mergear/modificar PRs de terceiros por inferência. A tarefa ativa desta triagem é `ADOPT-003`; a aceitação final do Adoption Gate permanece `ADOPT-004`.

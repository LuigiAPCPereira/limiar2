# Evidências de escopo e trabalho paralelo — adoção v2 / Limiar 2

**Natureza:** diagnóstico derivado para `ADOPT-003`, não PRD, decisão, autorização, tracker nem substituto de `TASKLIST.md`. **Consulta:** repositório `LuigiAPCPereira/limiar2`, PR de adoção #214, branch `docs/adopt-agent-protocol-v2-20260920` observada inicialmente no HEAD `16ff240c479aefd1e16b80d064cb9800e644bcc1`. Alterações posteriores exigem nova consulta. Não assumir working tree local, execução Codex nem acesso em tarefas agendadas.

## 1. Escopo sustentado e fronteiras de autoridade

| Pergunta | Fonte verificada | Resultado e limite |
| --- | --- | --- |
| Para que serve? | [`README.md`](../README.md), [`BASELINE.md`](BASELINE.md) §§1–3 | Plataforma pessoal de agregação e inteligência de promoções; fonte Telegram concreta atual; Evidence preservada e processamento derivado reconstruível. Público mais específico, entrega e versão formal do escopo ativo não demonstrados. |
| Qual arquitetura manda? | [`AGENTS.md`](../AGENTS.md) §§2, 4, 8 e ADRs `Accepted`; [`BASELINE.md`](BASELINE.md) | Constituição e Decisions aceitas prevalecem sobre briefing, specs, código e EvolutionDocs. ADRs 016–020 fornecem contratos da rebaseline nos escopos explicitados; aceitação não equivale a implementação nem validação produtiva. Cadeia de aprovação verificada na §5. |
| Qual decisão recente está autorizada? | [ADR 018](adr/018-source-admission-before-updates-manager.md), [issue #212](https://github.com/LuigiAPCPereira/limiar2/issues/212) | Source Admission live/recovery aceito; a issue delimita que não autoriza produção automática, exige gates e orienta PR técnica separada com código/teste/evidência. ADR 021/022/023 e ADR 024 não foram aceitos por essa issue. |
| O que é legado/histórico? | [`PRODUCT_BRIEF.md`](PRODUCT_BRIEF.md), [`CONTEXT.md`](CONTEXT.md), [`README.md`](../README.md) | Brief/Context descrevem Tursogo único, fases legadas e futuros que não substituem a rebaseline side-by-side do ADR 019. Não promover fases 3–6, frontend, IA ou discovery a tarefas atuais só pela presença nesses documentos. Preservar o histórico. |
| Onde estão as tarefas vigentes? | [`TASKLIST.md`](TASKLIST.md) | Escopo **documental de adoção** tem ADOPT-001–004 e DEV-001; PROD-001–008 são frentes conhecidas mapeadas a ADRs/BASELINE, não prova de escopo de produto aprovado e exaustivo. Não foi localizada fonte que fixe toda a versão de requisitos/aceites vigentes do produto; fontes adicionais classificadas na §6. |
| Como recuperar o histórico? | [`evolution/README.md`](evolution/README.md), ADRs, issue #212, PRs e commits | EvolutionDocs preserva Findings/Experiments/Proposals não autoritativos; ADRs registram Decisions. Há trilha histórica observável para os itens consultados, mas equivalência de todo histórico relevante não foi provada. Relatórios e plano adicionais identificados na §4. |

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

## 3. Auditoria das seis specs legadas versus decisões da Rebaseline

**Consulta adicional:** diretório `docs/specs/` enumerado na mesma revisão candidata `6ccf1e88a1be7cb9a768ed10cbf6b7c8ecd86225`: seis arquivos, identificados abaixo. Cabeçalhos e contratos iniciais de cada um consultados; **não** foi feita auditoria integral linha a linha ou validação de código. O [registry de ADRs](adr/README.md) define estados, aceitação exclusiva pelo mantenedor e disposição transitória dos ADRs 001–015. `BASELINE.md` §4/§5 governa o recorte derivado dos Accepted.

| Spec existente | Evidência textual consultada | Classificação para ADOPT-003; limite |
| --- | --- | --- |
| [`DASHBOARD.md`](specs/DASHBOARD.md) | Recomenda Vite, TypeScript, Tailwind/Alpine e descreve endpoints do dashboard de inspeção legada. | Contrato histórico/implementação legada a revalidar; não prova requisitos/aceites aprovados da nova plataforma ou dos PRs de a11y. |
| [`EXTRACTION-CRE.md`](specs/EXTRACTION-CRE.md) | Cabeçalho aponta ADR 014, Fase 2 e `Status: Pronto para implementação`; propõe sequência de sprints em torno de `raw_messages`, `processed_messages` e Product. | **Divergência de autoridade:** [ADR 014](adr/014-deterministic-extraction-and-cre.md) diz `Proposto`, e registry marca 014 `SUPERSEDE / SPLIT`, sem autoridade. `Pronto` na spec não concede aprovação para execução produtiva; preservar como pesquisa e evidência histórica. |
| [`GOTD-TD.md`](specs/GOTD-TD.md) | Descreve `TursoSessionStorage`, fachada e update handler que serializa/encaminha dados; erros de serialização são registrados e engolidos no fluxo descrito. | Descrição do funcionamento anterior; [ADRs 016–018 aceitos](BASELINE.md) impõem Source Evidence e admission/recovery durável. Não reutilizar o handler descrito como contrato final. |
| [`MEDIA-TD.md`](specs/MEDIA-TD.md) | Design de imagens v3 vinculado aos ADRs 011, 009 e 012, com cache e `processed_messages.id`. | Registro histórico: registry marca 011 `DEFER / REVALIDATE`, 012 `RETIRE`; desenho não autoriza novo boundary de mídia, que BASELINE §7 mantém aberto. |
| [`PROCESSOR-IDEATION.md`](specs/PROCESSOR-IDEATION.md) | Declara ideação de junho de 2026 `pronto para Sprint 1`, processador sobre único `limiar.db` Tursogo e estágios de interpretação/deduplicação. | Ideação e evidência histórica, não aceite atual para modelo/projeções da Rebaseline; BASELINE §§3–4 separa Evidence, Findings, Promotion Interpretation e identidade de domínio. |
| [`TURSOGO.md`](specs/TURSOGO.md) | Descreve o banco único legado, sessão/peers/raw no mesmo arquivo e migrations do storage anterior. | Compatibilidade/arqueologia; ADR 019 Accepted escolheu novo SQLite side-by-side e preserva Tursogo apenas onde legado, importação e rollback exijam. Não apagar ou migrar in-place. |

**Resultado documental deste exame:** agora há **lista completa dos seis arquivos de `docs/specs/` e classificação de escopo com evidências delimitadas**, não seis novas tarefas nem prova de que a íntegra das specs foi auditada. Exigência ainda aberta: fonte autorizada de versão do escopo e requisitos/aceites verificáveis de *todas* as frentes ativas; a classificação das specs não substitui essa fonte. `ADOPT-003` permanece em andamento; `ADOPT-004` deve conferir esta evidência na ref candidata.

## 4. Fontes históricas adicionais identificadas na árvore — sem escopo novo

**Consulta:** árvore raiz e `docs/` da revisão `4b8a695f90e4dd6dbaec7579ebb87415ac3a25ec`, pasta `docs/plans/` enumerada em um arquivo; cabeçalhos/trechos iniciais consultados em `7a3c75108f6a11886e4339fb930820728feba2ef`. Isso demonstra existência e divergências específicas, **não** busca exaustiva por requisitos ou auditoria completa desses documentos e do código.

| Fonte localizada | Registro observado | Tratamento para ADOPT-003 |
| --- | --- | --- |
| [`AUDIT-REPORT.md`](../AUDIT-REPORT.md) | Auditoria datada de 2026-06-28 do collector, processor, banco/classificação/extração, declaradamente somente leitura, com métricas de mensagens e diagnósticos daquele banco. | Histórico pontual sobre legado; volume, diagnósticos e recomendações de junho não são medições atuais, aceites da Rebaseline nem autorização de migrar banco. Não foi revalidado contra base real atual. |
| [`CLI-AUDIT-REPORT.md`](../CLI-AUDIT-REPORT.md) | Auditoria somente leitura de 2026-06-28 da árvore de comandos do orquestrador, collector e processor legados. | Evidência histórica de interface e fluxo, não critério versionado para CLI futura nem teste da revisão atual. |
| [`plans/model-package-migration.md`](plans/model-package-migration.md) | Plano datado de 2026-06-21, referenciado ao ADR 013, cabeçalho `Status: Não iniciado` e waves de migração; cabeçalho do [ADR 013](adr/013-model-package.md) já declara adoção parcial, e o diretório [`internal/model/`](../internal/model/) foi enumerado com arquivos Go na revisão consultada. Registry marca ADR 013 `RETAIN-PRINCIPLE / SUPERSEDE-SHAPE`. | **Status do plano não representa o estado atual**: plano histórico em desacordo com o ADR/código quanto à existência do pacote. Não executar waves automaticamente, marcar tarefa não iniciada ou usar sua forma global de modelo como alvo da rebaseline. Adoção parcial é declaração do ADR, não validação funcional dos arquivos Go. |

**Impacto sobre o Gate:** amplia a trilha de histórico recuperável e identifica uma fonte de status obsoleto, sem completar a função 7 nem suprir as funções 1/2/5. Não se criam novas frentes produtivas a partir desses três documentos. `ADOPT-003` segue em andamento; `ADOPT-004` terá de revalidar as referências na revisão final.

## 5. Cadeia verificada de aceitação arquitetural — não prova execução

**Consulta:** ADRs na ref `df4f13656aa5fdbe92ed1592cea430d56294a7f6`, referências históricas individuais e aprovação do mantenedor quando diretamente acessível. Status de um ADR e registro da autorização não demonstram que o produto executa ou valida o contrato.

| Decision | Metadados/documento | Referência consultada e limites |
| --- | --- | --- |
| [ADR 016](adr/016-source-evidence-and-telegram-synchronization.md) | `Accepted`, 2026-08-18, referência `PR #150` | [#150](https://github.com/LuigiAPCPereira/limiar2/pull/150) foi integrado; corpo declara que materializa aprovação anterior do mantenedor. É registro da decisão, não comentário independente da aprovação original; nenhum código abrangido no PR. |
| [ADR 017](adr/017-telegram-recovery-durability-boundary.md) | `Accepted`, 2026-08-18, referência `PR #153` | [#153](https://github.com/LuigiAPCPereira/limiar2/pull/153) foi integrado e declara aprovação anterior; documenta contratos e gates, sem código de produção. |
| [ADR 018](adr/018-source-admission-before-updates-manager.md) | `Accepted`, 2026-09-19, referência issue #212 | [Issue #212](https://github.com/LuigiAPCPereira/limiar2/issues/212) foi consultada: registra autorização verbal anterior transcrita pelo agente por instrução do mantenedor e ressalva que **não é uma aprovação independente adicional na issue**. Exige PR técnica separada e gates, sem aprovar 021–023. |
| [ADR 019](adr/019-local-sqlite-storage-baseline.md) | `Accepted`, 2026-09-08, referência comentário da issue #172 | [Comentário do mantenedor](https://github.com/LuigiAPCPereira/limiar2/issues/172#issuecomment-5581795839) consultado, com aprovação explícita, metadados coincidentes e limites: gates antes de produção, sem migração in-place, EXP-LIMIAR-007 ainda necessário. |
| [ADR 020](adr/020-evidence-physical-schema.md) | `Accepted`, 2026-09-08, referência comentário do PR #174 | [Comentário explícito do mantenedor](https://github.com/LuigiAPCPereira/limiar2/pull/174#issuecomment-5584088724) consultado; [#174](https://github.com/LuigiAPCPereira/limiar2/pull/174) integrado. Apenas schema mínimo de Evidence aceito; gates e não migração in-place preservados. |

Os cabeçalhos dos [ADRs 021](adr/021-source-sync-state-physical-schema.md), [022](adr/022-backfill-progress-physical-schema.md), [023](adr/023-hardened-mtproto-session-file-boundary.md) e [024](adr/024-acquisition-subscription-identity.md) foram lidos **nessa ref** e todos permanecem `Proposed`; não é permitido transformar Evidence de harness, conclusão de auditoria ou um `<continue>` em aceite arquitetural. Essa trilha melhora a função documental 4 e parte da 7; não define um PRD abrangente nem fecha o Adoption Gate.

## 6. Requisitos e tarefas fora de `docs/specs/`: autoridade a revalidar

**Descoberta por busca na `main` e enumeração da árvore na ref `df4f13656aa5fdbe92ed1592cea430d56294a7f6`:** `.kiro/specs/` contém dois conjuntos, e `docs/superpowers/` contém um plano e uma spec de 2026-06-21. Os trechos abaixo foram lidos; nem todos os arquivos extensos foram auditados integralmente ou confrontados com o código. Portanto, a lista de **seis specs de `docs/specs/` da §3 é exaustiva somente para aquele diretório**, não para todo o repositório.

| Fonte | Conteúdo observado | Classificação/limite |
| --- | --- | --- |
| [`.kiro/specs/limiar-collector/requirements.md`](../.kiro/specs/limiar-collector/requirements.md) e [design](../.kiro/specs/limiar-collector/design.md) | Requisitos com histórias e critérios de aceite testáveis para **Fase 1 legada**; prescrevem Tursogo único, DBWriter e `LastMessageID` para retomada; design define coletor standalone com fan-in. | Fonte histórica de requisitos, não versão atual aprovada: ADRs 016–019 Accepted alteram sync/admission/storage no novo boundary; preservar critérios reaproveitáveis somente após reconciliação explícita. |
| [`.kiro/specs/logging-system-redesign/requirements.md`](../.kiro/specs/logging-system-redesign/requirements.md), [design](../.kiro/specs/logging-system-redesign/design.md), [tasks](../.kiro/specs/logging-system-redesign/tasks.md) | Especificação de logging anterior à rebaseline sobre logger legado, incluindo hipótese de lacuna de redação no PrettyHandler; plano lista itens `[ ]` e respectivos critérios; design relata aprovação do próprio redesign. | Não presumir que o alegado problema de segurança ainda exista no HEAD ou que tarefas estejam pendentes só por `[ ]`; sem revalidação de código/status e sem fonte competente de escopo atual, plano Kiro não substitui TASKLIST. Não usar a spec para sobrepor AGENTS/ADRs atuais. |
| [`docs/superpowers/specs/2026-06-21-model-package-design.md`](superpowers/specs/2026-06-21-model-package-design.md) e [plano](superpowers/plans/2026-06-21-model-package.md) | Spec de 2026-06-21 afirma `Design aprovado, aguardando implementação` para pacote `internal/model`. | Estado histórico já diverge do [ADR 013](adr/013-model-package.md) e do diretório `internal/model` existente; registry 013 `RETAIN-PRINCIPLE / SUPERSEDE-SHAPE`, sem autoridade atual para executar formato global proposto. |
| [`.kiro/raio.md`](../.kiro/raio.md) | Anotações de performance e decisões operacionais do collector legado, incluindo DBWriter/Tursogo. | Diário histórico de engenharia, não requisitos/validações atuais ou autoridade arquitetural. |

**Conclusão delimitada:** existem critérios de aceite e um tracker de tarefa em arquivos antes omitidos da arqueologia. São ligados explicitamente à Fase 1, ao antigo pipeline/logging ou ao ADR 013 em transição, sem prova de vigência para **todo o escopo da Rebaseline**. A função 2 e a equivalência de tracker da função 5 continuam pendentes; essa conclusão decorre dos trechos e autoridades consultados, não de uma auditoria integral de cada arquivo.
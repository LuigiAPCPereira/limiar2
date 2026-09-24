# PROJECT_STATE — Limiar 3.0 (checkpoint documental)

**Data de preparação:** 2026-09-22 (America/Bahia). **Última reconciliação documental:** 2026-09-24. **Repositório:** `LuigiAPCPereira/limiar2`. **Branch:** `docs/limiar-3-foundation-20260922`. **Base de criação:** `7177d929839512b8005ae379d96e3d224feac1f8` (PR #214 draft), descendente de `main@796b7769449b72320b27c2557c2ccb8c8eb183e1`. **Revalidar HEAD atual, branch e main no início do próximo chat**; este checkpoint não pode registrar o SHA do próprio commit. A branch é documentação, não PR/merge/deploy. Conector GitHub remoto não prova working tree local.

## Objetivo confirmado pelo mantenedor

Limiar 3.0: reconstrução integral do agrupador de promoções, incremental e bottom-up. Sessão e integração Telegram/MTProto ficam atrás de boundaries desacopláveis; MCP também é detachable. O MCP realtime consulta Telegram diretamente e entra cedo para investigação do domínio; collector/Evidence/recovery formam outro ramo durável; depois o MCP pode consultar dados processados via Query Service. Não existe segundo app e desacoplamento não implica microserviços. Qualidade requerida: segurança, desempenho medido, simplicidade, legibilidade e manutenção de longo prazo. Black Friday 2026 é objetivo, não prazo validado.

## Tarefas e estado

**Tarefa da entrega:** `L3-DOC-001` — branch e documentação. A branch foi criada; START_HERE, PRODUCT_AND_SCOPE, ARCHITECTURE_PROPOSAL, TASKLIST, SESSION_LOG, INIT_REPORT e ENGINEERING_GUIDE foram escritos; root `ENGINEERING_DNA.md` é entrada legível e arquivo `docs/limiar3/ENGINEERING_DNA_ORIGINAL.md.gz.b64` preserva o original completo compactado.

**Evidência do DNA:** arquivo fonte no Project 2.171 linhas / 37.531 bytes; SHA-256 `c19c5d97f8e42b311d6c15440e2e9f58cec64fadf1a48b9575e1f2e4dba0e373`. Git blob calculado do Base64/gzip derivado da fonte **é idêntico ao blob remoto reaberto** `ceafec243be87ae28ee2d9658e3912831fc55368`, contendo seis linhas e 17.953 bytes. Assim, conteúdo original foi preservado sem mudança quando descompactado, mas ainda não está como Markdown integral plaintext na raiz: o outro ambiente deve descompactar e comparar SHA antes de afirmar leitura integral. `ENGINEERING_GUIDE.md` e root `ENGINEERING_DNA.md` são resumos/entradas, não falsa versão completa. Protocol Project SHA-256 `d078e0b3d4a8f9d4bd21cb0c7c8a3e417cba484981566d801ff8453ac7be1dab`, blob Git na origem `78b2e86564fb287886f9df065fdb727b20c52727`; branch herda `AGENTS.md` e protocolo v2 do PR #214, cujo Adoption Gate permanece **PARCIAL**.

**L3-001, L3-001A, L3-001B e L3-001C estão internalizadas como investigação/Evidence/Proposal, não como Decisions arquiteturais.** Fontes: [`L3_001_SESSION_BOUNDARY_INVESTIGATION.md`](L3_001_SESSION_BOUNDARY_INVESTIGATION.md), [`L3_001A_MTPROTO_GOTD_INVESTIGATION.md`](L3_001A_MTPROTO_GOTD_INVESTIGATION.md) e [`L3_001B_PRODUCTION_FOUNDATION_RESEARCH.md`](L3_001B_PRODUCTION_FOUNDATION_RESEARCH.md). O conjunto sustenta owner por `TelegramAuthorizationIdentity`, main gotd client único, private session storage, bootstrap explícito/fail-closed, semantic Ready/self binding, peer state authorization-scoped e guards externos ao updates.Manager. L3-001B adiciona toolchain/supply-chain, threat model, performance, observability e hardening de produção.

**Direção de dependências/toolchain:** [`DEPENDENCY_TOOLCHAIN_POLICY.md`](DEPENDENCY_TOOLCHAIN_POLICY.md) registra `Current Stable First`: stable atual é o default; versão anterior exige razão técnica concreta. Para Go, a direção atual é avaliar Go 1.27.1 como baseline preferencial da nova Evidence. A auditoria L3-001C recomenda `github.com/gotd/td v0.162.0` como pin de L3-002; isso permanece Proposal até confirmação explícita do mantenedor e gate da combinação real.

**L3-001C — stop condition atingida:** [L3_001C_GOTD_UPSTREAM_AUDIT.md](L3_001C_GOTD_UPSTREAM_AUDIT.md) conclui que não há blocker upstream para iniciar L3-002 e que não é necessária nova pesquisa ampla de fundação. A extensão mínima proposta é hardened `session.Storage`, runtime ownership/readiness, TelegramQuery adapter, error translation e bounded admission; gotd permanece owner de reconnect/pools/migration/RPC/MTProto/query/media machinery.

**Próxima ação:** decisões focadas antes de L3-002: aceitar/rejeitar/substituir o pin gotd recomendado, credential storage/ADR 023 ou substituto, plataforma/deployment inicial e bootstrap surface; confirmar toolchain stable real; então aplicar o Implementation Gate e somente depois iniciar código. PFS, peer persistence, FLOOD_WAIT UX, updates/recovery e media ficam para suas fatias. Não abrir L3-001D sem dúvida material nova.

## Restrições e desconhecidos

ADRs 021–024 continuam Proposed até decisão legítima; L3-001/L3-001A/L3-001B/L3-001C não os promovem; EXP-019 só prova harness Unix/intra-processo, não runtime; `internal/storage/sqlite` + PR #211 são implementação parcial herdada, não Limiar 3.0 integrado. Imagens têm falha relatada, causa não reproduzida. Não há código Go novo, testes Go/CI/race, benchmark, Telegram/MCP conectado, acesso a dados reais, branch de implementação, PR novo, merge ou deploy executados por esta preparação. Decisão final de diretórios, processo, sessão, identidade do consumidor e integração ChatGPT fica para investigação/autoridade apropriada; sem esquema/ID fictício.

**Estado editorial:** arquivos documentais criados; conferir reflinks, presença e compare final da branch antes de declarar L3-DOC-001 validada. Engineering DNA integral compactado e blob verificados, formato Markdown integral na raiz permanece não disponibilizado. A inicialização do Limiar 3.0 não conclui adoção v2 do legado. Atualizar checkpoint apenas com fatos e task IDs, não com inferências de CI antigo.

## Atualização documental — L3-DOC-002

Foi solicitado consolidar os aprendizados caros da Rebaseline 2026 e registrar como construir o Limiar 3 de baixo para cima com separação explícita de authorities.

Foram adicionados `REBASELINE_INHERITANCE.md` e `BOTTOM_UP_REBUILD_PLAN.md`. O primeiro distingue contratos Accepted, Evidence experimental e ADRs Proposed; registra limites de portabilidade, sessão/peer, EXP-007 e Tursogo legado. O segundo ordena a construção por boundaries: runtime/config -> sessão -> Evidence -> acquisition/Source Admission -> recovery -> backfill -> peer/media -> projections -> findings/interpretação -> modelo comercial -> query -> MCP/API/frontend -> migração/cutover.

Nenhum ADR 021–024 foi promovido, nenhum package/schema novo foi autorizado e nenhum código de produto foi alterado. A próxima tarefa funcional continua `L3-001`, usando os dois documentos como base.


### Verificação de L3-DOC-002

O commit documental `7b8d6d1af12bbd289f9ca3c6e98dcb4fadb0a0cd` foi reaberto na própria branch. O compare contra `84636ae7ae9b84a8424dd2cff85a9a63454f9372` mostrou `ahead_by=1`, `behind_by=0` e exatamente oito arquivos alterados, todos sob `docs/limiar3/`. Os dois documentos novos foram reabertos e seus conteúdos/links principais conferidos. Portanto `L3-DOC-002` passa de `documentada` para `validada`. Esta validação é documental e não altera o estado de implementação do Limiar 3 nem o status dos ADRs 021–024.


## Atualização documental — L3-DOC-003

O mantenedor esclareceu três requisitos: MCP deve ser construído cedo antes de congelar o modelo de promoções porque servirá para investigar mensagens reais; o MCP realtime consulta Telegram diretamente sob demanda, sem usar o storage do Limiar como proxy obrigatório; e tanto MCP quanto integração Telegram/MTProto devem ser desacopláveis por contrato, sem transformar desacoplamento em microserviços obrigatórios.

Foi criado `DETACHABLE_BOUNDARIES.md`. O plano agora possui dois ramos após a capability Telegram: `MCP Telegram realtime` para exploração e `collector -> Source Admission -> Evidence` para durabilidade. O modelo comercial vem depois do loop de descoberta do domínio. Futuramente `MCP Limiar data` usa Query Service.

Nenhum código, ADR status, schema, serviço, credencial ou conexão real foi criado por esta atualização.


### Verificação de L3-DOC-003

A branch foi reaberta no HEAD `fdf90501e2de60c7c758ca0eae14c01d32e4618b`. O compare contra `5c3d7fe4936050387bf9b7335fd4872f2181c175` mostrou 10 commits documentais e exatamente 10 arquivos alterados, todos sob `docs/limiar3/`. Foram reabertos `DETACHABLE_BOUNDARIES.md`, `BOTTOM_UP_REBUILD_PLAN.md`, `PRODUCT_AND_SCOPE.md`, `ARCHITECTURE_PROPOSAL.md`, `START_HERE.md`, tracker, checkpoint, log, init report e herança da rebaseline. A verificação confirmou: MCP realtime consulta Telegram diretamente; não depende do Evidence DB; resposta não vira Source Evidence automaticamente; gotd/MTProto fica atrás de boundary desacoplável; e não restou a regra antiga de iniciar MCP apenas após Query Service. `L3-DOC-003` passa a `validada`.


## Internalização da investigação L3-001

Em 2026-09-23, o mantenedor forneceu o relatório completo da investigação L3-001 produzido em outro chat e autorizou sua internalização documental. O conteúdo foi condensado sem alterar seu estado epistemológico: recomenda ownership único do runtime por session identity, sessão fora do Evidence DB, hardened file como candidato inicial, capabilities read-only/bounded para MCP realtime e separação entre session, peer state, update state, Evidence e MCP auth. Os gates G-A–G-L, plano de testes e experimentos materiais ficaram registrados em `L3_001_SESSION_BOUNDARY_INVESTIGATION.md`.

A internalização **não revalidou as fontes externas citadas no relatório original**. Por decisão do mantenedor, antes de L3-002 haverá uma investigação separada de MTProto e gotd (`L3-001A`). Esta passa a ser a próxima ação executável; L3-002 permanece não autorizada.

## Internalização da investigação L3-001A

Em 2026-09-23, o mantenedor forneceu o relatório técnico MTProto/gotd produzido no gate L3-001A. A investigação usou `gotd/td v0.161.0` e fontes upstream primárias para confrontar a Proposal L3-001. O resultado foi internalizado em `L3_001A_MTPROTO_GOTD_INVESTIGATION.md` e reconciliado em `DETACHABLE_BOUNDARIES.md`, `BOTTOM_UP_REBUILD_PLAN.md` e `ARCHITECTURE_PROPOSAL.md`.

Correções centrais: `session identity` foi substituído por `TelegramAuthorizationIdentity`; `session.ErrNotFound` upstream não autoriza auto-login; peer state é separado mas authorization-scoped; `updates.Manager` não garante sozinho Evidence-before-progress; restore/reuse real gotd vem antes da primeira capability estável; o primeiro contract proposto é `TelegramQuery` coeso (`ResolvePeer` + `History`) sem `tg.*`/access hash/session bytes.

L3-002 continua sem autorização de código. Não houve login Telegram, execução dos experimentos recomendados, promoção de ADR, branch de código, PR, merge ou deploy nesta internalização.

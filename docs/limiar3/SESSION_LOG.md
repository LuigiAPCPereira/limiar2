# SESSION_LOG — Limiar 3.0

**Origem:** síntese de afirmações do mantenedor e verificações GitHub da conversa de 2026-09-22; não inventa decisões ou fatos de outros chats. Logs anteriores e PRs permanecem nas fontes originais.

## Conhecimento prévio da rebaseline, a não perder

- O Limiar 1 foi interrompido devido a forte acoplamento e problemas funcionais, incluindo imagens não coletadas corretamente segundo o mantenedor. Causa do bug não reproduzida aqui.
- Rebaseline arquitetural já foi além de documentos: primeiro SQLite/Evidence lado a lado implementado, com guards; ADRs 016–020 Accepted. PR #211 integrada para rejeitar schema incompleto/futuro. Isso não é substituição completa de Tursogo.
- Sync/backfill físicos seguem ADRs 021/022 Proposed, sem aprovação; session storage ADR 023 Proposed, identidade de aquisição ADR 024 Proposed. `EXP-LIMIAR-019` Unix/intra-processo Supported e PR #202 integrada em 2026-09-14, mas apenas teste/experimento, não runtime. EXP-007 em banco histórico real pendente. Portabilidade macOS inconclusiva na narrativa fornecida. Separação sessão vs peer cache registrada em F-STO-006.
- PR documental de adoção v2 #214 está draft e **ADOÇÃO PARCIAL**. Seu HEAD antes desta branch foi `7177d929839512b8005ae379d96e3d224feac1f8`; base main `796b7769449b72320b27c2557c2ccb8c8eb183e1`. A branch Limiar 3.0 foi criada a partir daquele HEAD, para herdar AGENTS/protocolo sem alterar a main.

## Evolução da direção solicitada pelo mantenedor

1. Inicialmente pediu reconstruir 100% a **fundação de ingestão** bottom-up começando autenticação/sessão, mensagens, coleta e imagens, com separação clara, Engineering DNA, segurança, legibilidade, robustez e performance demonstrada. Essa diretriz foi registrada em `docs/REBASELINE_SCOPE.md` no PR #214.
2. Enviou proposta mais ampla: avaliar arquitetura **nova para toda a implementação Limiar**, em ambiente isolado no mesmo repo, aproveitando conhecimento e capacidades validadas, preservando dados/comportamentos relevantes e reescrevendo incrementalmente sem transposição literal do legado. Pediu avaliar ANTES de branch/implementação, sem decisões técnicas precipitadas.
3. Propôs MCP Telegram também para a própria conversa com ChatGPT, para perguntar diretamente sobre promoções e acessar mensagens autorizadas, além de apoio ao desenvolvimento. Isso expande o produto, não cria um segundo coletor.
4. Explicitou que **Limiar e MCP devem ser construídos juntos** e cogitou reutilizar a autenticação do Limiar; ficou explicitada a necessidade de distinguir sessão MTProto do coletor e eventual autorização de consumidor MCP. Identidade de conta de usuário Limiar não comprovada no produto anterior.
5. Corrigiu o assistente: **não existe app Limiar extra**. O produto é coleta → MCP integrado em paralelo → processamento → dados limpos → API → frontend de agrupador de promoções. MCP não precisa estar no caminho crítico, mas deve constar desde a fundação e dar consulta a mensagens e ofertas.
6. Reafirmou que **não havia autorizado implementação**: antes seria feita avaliação arquitetural. Em seguida, neste pedido, autorizou especificamente **criar uma NOVA branch e colocar somente documentação**: Engineering DNA, Agent Development Protocol como projeto novo, e contexto completo Limiar 3.0; objetivo é outro chat investigar projeto inicial + autenticação/sessão. Isso NÃO autoriza código, auth real, Telegram, MCP conectado, merge ou deploy.

## Ações documentais desta execução

- Confirmado GitHub `main@796b7769449b72320b27c2557c2ccb8c8eb183e1`, PR #214 ainda draft HEAD `7177d929839512b8005ae379d96e3d224feac1f8`, repositório privado com permissão reportada de push.
- Criada branch `docs/limiar-3-foundation-20260922` diretamente do HEAD documental, sem merge.
- Criados documentos de entrada, produto, arquitetura, tracker, checkpoint, relatório de inicialização e guia derivado do Engineering DNA. Reabertura final da ref e inclusão integral do DNA devem determinar se L3-DOC-001 estará validada ou parcial.
- Nenhuma decisão de mecanismo de sessão, arquivo de configuração, estrutura de pacotes, OAuth/MCP ou schema foi aceita nesta escrita.

**Próximo capítulo:** outro chat deve recuperar START_HERE e executar somente a investigação `L3-001` (projeto inicial + autenticação/sessão) até receber autorização de engenharia específica. Registrar novas decisões e experimentos no futuro, sem retroagir status de fontes.

## Consolidação da rebaseline para Limiar 3 — L3-DOC-002

A pedido explícito do mantenedor, o conhecimento arquitetural da Rebaseline 2026 foi consolidado em contexto recuperável do Limiar 3, sem criar nova Decision. Foram criados `REBASELINE_INHERITANCE.md` e `BOTTOM_UP_REBUILD_PLAN.md`.

A regra registrada é: Limiar 3 herda invariantes, contratos aceitos, Evidence e limites; não herda automaticamente mecanismos experimentais, schemas Proposed, topologia física ou packages do legado. O plano bottom-up separa sessão, Evidence, subscription/admission, live recovery, backfill, peer cache, mídia, projections, processing, modelo comercial, query e superfícies MCP/API/frontend, deixando migração/cutover por último.

A escrita foi exclusivamente documental. ADRs 021–024 permanecem Proposed e `L3-001` continua sendo a próxima investigação funcional.


## Refinamento de boundaries e ordem do MCP — L3-DOC-003

O mantenedor definiu que o MCP deve existir antes da modelagem dos dados de promoções para permitir investigação do corpus real; esclareceu que o MCP consulta Telegram em tempo real diretamente, e não mensagens via storage do Limiar; e confirmou que MCP e MTProto devem ser desacopláveis de forma coerente com a separação do Engineering DNA.

A direção registrada distingue `Session Credential` como authority do boundary Telegram; `Telegram/MTProto Adapter` contendo gotd e expondo capabilities estreitas; `MCP Telegram realtime` como consumidor exploratório detachable; collector/Source Admission como consumidor durável separado; e, futuramente, `MCP Limiar data` sobre Query Service. Desacoplamento lógico vem antes de topologia física e não implica microserviços.

Como nenhuma das tarefas L3-002+ havia sido implementada, os escopos L3-003–L3-007 foram reorganizados no tracker antes de código, preservando esta entrada histórica.


## Investigação de sessão/Telegram boundary — L3-001

Em 2026-09-23, o mantenedor forneceu o relatório completo da investigação L3-001 produzido em outro chat. A investigação partiu do HEAD documental 8c059f10245a5ca248862becfbb00033300200a8 e analisou sessão MTProto, gotd, alternativas de session storage, separação de authorities, capabilities de MCP realtime/collector, lifecycle, segurança, testes, chaos e performance.

A recomendação foi internalizada em [L3_001_SESSION_BOUNDARY_INVESTIGATION.md](L3_001_SESSION_BOUNDARY_INVESTIGATION.md) como **Proposal**, não Decision. Síntese:

- uma autoridade TelegramRuntime por session identity;
- gotd.Client e SessionStore privados ao Telegram boundary;
- session credential separada de peer cache, update/recovery state, Evidence e MCP auth;
- hardened file local Unix/Linux como candidato inicial, sustentado apenas dentro do escopo de Evidence do EXP-LIMIAR-019;
- SQLite de Evidence não deve armazenar a sessão;
- primeira capability read-only/bounded para MCP realtime;
- live updates/recovery do collector somente quando Source Admission/Evidence puder preservar os invariantes dos ADRs 016–018;
- critérios G-A–G-L e experimentos de gotd real/restart, faults, revogação, backup isolation e concorrência.

ADRs 021–024 não foram promovidos. ADR 023 permanece Proposed. As referências externas citadas pelo relatório original não foram reconsultadas durante esta internalização.

O mantenedor decidiu que, antes de qualquer avanço para L3-002, é necessário um relatório específico sobre MTProto e gotd. Foi criado [MTPROTO_GOTD_RESEARCH_BRIEF.md](MTPROTO_GOTD_RESEARCH_BRIEF.md) e a tarefa L3-001A passou a ser o próximo gate de investigação. Nenhum código, login Telegram, MCP real, segredo, PR novo, merge ou deploy foi criado por esta atualização.


## Investigação MTProto + gotd — L3-001A

Em 2026-09-23, o mantenedor forneceu o relatório final da investigação L3-001A, ancorado no HEAD `01d2a5a67fa13bdf1c15f0dbe60424e2af6b41ff` e na versão `github.com/gotd/td v0.161.0` fixada pelo repositório. O relatório confrontou a Proposal L3-001 com MTProto oficial e código/documentação upstream da versão usada.

Resultado: a direção central foi sustentada com ajustes. O owner passa a ser uma `TelegramAuthorizationIdentity`, não MTProto `session_id`; o main `gotd.Client` deve ter owner único por autorização; `session.ErrNotFound` upstream não autoriza auto-login; peer/access-hash state é separado mas authorization-scoped; history/backfill e update recovery são authorities distintas; e `updates.Manager` precisa de Source Admission/GuardedRecoveryAPI/GuardedStateStorage + supervisor externo para satisfazer Evidence-before-progress.

A sequência bottom-up foi refinada para provar gotd real restore/reuse antes de estabilizar `TelegramQuery`. O primeiro contract proposto é coeso e read-only (`ResolvePeer` + `History`), com `PeerKey`/message identity source-aware e sem `tg.*`, `InputPeer`, access hash ou session bytes.

O relatório está em [L3_001A_MTPROTO_GOTD_INVESTIGATION.md](L3_001A_MTPROTO_GOTD_INVESTIGATION.md). `MTPROTO_GOTD_RESEARCH_BRIEF.md` foi marcado como concluído. ADR 023 e demais Proposed não foram promovidos. Nenhum experimento real, login, OTP, código de produto, branch de implementação, PR, merge ou deploy foi executado nesta internalização.


## Fundação de produção e política de versões — L3-001B

Em 2026-09-23, o mantenedor forneceu o terceiro relatório de fundação, cobrindo histórico dos Limiares, Go/toolchain, gotd ecosystem, auth/session, threat model, filesystem hardening, QR bootstrap, PFS, peer cache, concurrency/fairness, retry, observability, profiling, supply chain e gates de L3-002.

O relatório foi internalizado em [L3_001B_PRODUCTION_FOUNDATION_RESEARCH.md](L3_001B_PRODUCTION_FOUNDATION_RESEARCH.md). Ele mantém a arquitetura mínima: authorization identity explícita, private session storage, bootstrap administrativo, um main gotd client, semantic Ready/self binding, restart/reuse real e depois TelegramQuery read-only.

Na conversa posterior, o mantenedor definiu a política **Current Stable First**: para uma geração nova, stable atual é o default; permanecer em versão inferior exige motivo técnico concreto. Features atuais devem ser aproveitadas quando úteis ao escopo, sem modernidade ornamental. A política está em [DEPENDENCY_TOOLCHAIN_POLICY.md](DEPENDENCY_TOOLCHAIN_POLICY.md).

A consequência para Go é avaliar a stable atual como baseline preferencial da Evidence de L3-002, em vez de validar tudo em uma linha antiga e migrar imediatamente depois. A consequência para gotd é semelhante: v0.161.0 continua uma referência profundamente investigada, mas o stable atual precisa ser auditado/testado antes do pinning final.

### Pesquisa gotd independente

L3-001A e L3-001B usaram pesquisa externa upstream real sobre gotd, incluindo versão fixada, arquitetura, packages, releases, issues e ecossistema. Porém essa pesquisa sempre respondeu perguntas do Limiar. Ainda não existe uma auditoria documental em que o gotd seja estudado **por si só primeiro** e apenas depois confrontado com L3-002.

Foi criado [GOTD_UPSTREAM_ONLY_RESEARCH_BRIEF.md](GOTD_UPSTREAM_ONLY_RESEARCH_BRIEF.md) como tarefa recomendada L3-001C. Ela é limitada: não refaz MTProto nem produto; serve para fechar stable version, extension points, maturity, security, performance e known pitfalls do gotd.

Nenhum upgrade, código, login Telegram, ADR promotion, PR, merge ou deploy foi executado por esta atualização.


## Auditoria upstream-only do gotd — L3-001C

Em 2026-09-24, o mantenedor forneceu o relatório final da auditoria upstream-only do `gotd/td`. O relatório revalidou a branch documental em `bd9e8f0c6ba8b9e7d1c856c7c3a6fd8f1ff41a70`, auditou a stable upstream atual sem usar o Limiar como lente na fase inicial e depois confrontou os achados com L3-002.

Resultado internalizado em [L3_001C_GOTD_UPSTREAM_AUDIT.md](L3_001C_GOTD_UPSTREAM_AUDIT.md): recomenda `github.com/gotd/td v0.162.0` como pin para L3-002, sujeito à decisão do mantenedor e Implementation Gate; usa diretamente `telegram.Client`, lifecycle, reconnect/pools/migration, generated `tg.Client`, auth/query/media helpers; limita a extensão própria a hardened session storage, runtime ownership/readiness, TelegramQuery adapter, error translation e bounded admission.

Achados de cautela: não usar upstream `session.FileStorage` como credential storage production; não adotar `telegram/peers.Manager` experimental/WIP como foundation; não instalar globalmente `gotd/contrib/middleware/floodwait.Waiter` nas condições atuais devido a relato upstream reproduzível de reentrancy/deadlock em DC migration; manter `updates.Manager` fora de L3-002 e revisar issues/limites na fatia collector; adicionar experimento estreito de clock-skew/lack-of-progress antes de production-ready.

A stop condition da investigação ampla foi atingida. Não há blocker upstream encontrado para iniciar L3-002, mas L3-002 continua sem autorização automática. Restam Decisions/gates do projeto: pin final, credential storage/ADR 023 ou substituto, plataforma/deployment inicial, bootstrap surface, toolchain efetivo e autorização de implementação. Não abrir L3-001D sem nova incerteza material.

Nenhum código, upgrade, login Telegram, ADR promotion, branch de implementação, PR, merge ou deploy foi executado nesta internalização.


## Aceitação da fundação pré-L3-002 — ADRs próprios do Limiar 3

Em 2026-09-23, após L3-001/A/B/C, o mantenedor concordou com o pacote de decisões de fundação e corrigiu a governança: como Limiar 3 é uma reconstrução, ADRs exclusivos dele não continuam em `025+`; o namespace `docs/limiar3/adr/` reinicia em `001`.

A aceitação foi registrada de forma durável no commit `baa626b59ec21d36f0dbde9a8a1fb3bbb5a4a2a6`. Foram criados e marcados `Accepted`:

- L3 ADR 001 — hardened Telegram credential/session storage em arquivo local, Linux/single-host/single-process;
- L3 ADR 002 — `TelegramAuthorizationIdentity`, owner único do main gotd client, bootstrap separado, fail-closed steady-state, semantic readiness/self binding, primeira TelegramQuery e limites de retry/concurrency.

O pacote também aceita Go 1.27.1 e `github.com/gotd/td v0.162.0` como baseline inicial de L3-002, sob `Current Stable First`. O ADR histórico 023 da rebaseline permanece `Proposed`; sua Evidence foi reaproveitada, mas ele não foi promovido retroativamente.

A aceitação é arquitetural/documental. Não houve autorização implícita de código, login Telegram, OTP/2FA, credenciais, PR de implementação, merge ou deploy. O próximo estado é `L3-002 pronta para Implementation Gate`.


## Repository rebaseline / legacy containment — L3-BASE-001

Em 2026-09-23, o mantenedor autorizou executar a separação física entre a reconstrução Limiar 3 e a implementação anterior. Foi criada a branch `refactor/limiar3-repository-rebaseline` a partir de `docs/limiar-3-foundation-20260922@00e5c422b2ed98b10ab145c53b71936352b90585`.

A aceitação foi registrada no commit `70dcd4982b90a26fa6eb7f722a679537f54bb1e0` e L3 ADR 003 — Repository topology and legacy containment — foi criado como `Accepted`.

A movimentação principal foi feita por Git tree, preservando blob SHAs quando o conteúdo não precisava mudar. Foram realocados 311 blobs para `legacy/limiar2/`, incluindo implementação Go anterior, módulo/go.sum, workflows, experiments, tools, scripts e documentação histórica/rebaseline. O antigo `go.mod` mantém o blob `32b740c3d7aee68c2fb21729bf2a999b86676cf6` e `module github.com/limiar/collector`.

O root passou a ter:

- `module github.com/LuigiAPCPereira/limiar2`;
- `go 1.27.1` e `toolchain go1.27.1`;
- README/baseline/arquitetura derivados do Limiar 3;
- apenas um package sentinela `internal/foundation`, sem lógica de produto;
- CI único do root L3, pinado em Go 1.27.1;
- gate `scripts/ci/check-no-legacy-imports.sh`;
- namespace novo `docs/limiar3/evolution/`.

Os workflows anteriores estão preservados sob `legacy/limiar2/.github/workflows/` e deixam de executar como workflows do root.

A cópia integral do Engineering DNA disponível no Project foi verificada localmente nesta execução: 37.531 bytes, 2.171 LF, SHA-256 `c19c5d97f8e42b311d6c15440e2e9f58cec64fadf1a48b9575e1f2e4dba0e373`, igual ao hash canônico esperado. Isso não implica sincronização automática em outros ambientes.

O container local disponível usa Go 1.23.2, portanto não foi usado para declarar build/test/race do módulo Go 1.27.1. A validação objetiva dessa combinação deve ocorrer no CI da branch/PR.

Nenhuma capability L3-002, TelegramRuntime, login Telegram, OTP/2FA, session credential, MCP real, merge ou deploy foi criado por esta fatia.


## Verificação de L3-BASE-001

A árvore remota foi reaberta após a movimentação. O compare contra `00e5c422b2ed98b10ab145c53b71936352b90585` mostrou 300 arquivos: **269 renames, 18 modified, 13 added e zero removed**. A implementação antiga permanece preservada; não houve deleção pura no diff.

Checks estáticos:

- 158 arquivos Go preservados sob `legacy/limiar2/`;
- nenhum package de implementação L2 permaneceu em `cmd/` ou `internal/`; o único package de produto/root é o sentinela sem lógica `internal/foundation/doc.go`;
- o `legacy/limiar2/go.mod` mantém blob `32b740c3d7aee68c2fb21729bf2a999b86676cf6` e `module github.com/limiar/collector`;
- root `go.mod`: `module github.com/LuigiAPCPereira/limiar2`, Go/toolchain 1.27.1;
- workflow root único: `.github/workflows/ci.yml`; oito workflows anteriores foram preservados sob legacy;
- links relativos dos documentos canônicos/root e de todos os documentos L3 verificados não apresentaram targets quebrados;
- protocolo do Project confirmado em 33.162 bytes / 247 LF / SHA-256 `d078e0b3d4a8f9d4bd21cb0c7c8a3e417cba484981566d801ff8453ac7be1dab`; blob da branch continua `78b2e86564fb287886f9df065fdb727b20c52727`;
- Engineering DNA integral do Project confirmado em 37.531 bytes / 2.171 LF / SHA-256 `c19c5d97f8e42b311d6c15440e2e9f58cec64fadf1a48b9575e1f2e4dba0e373`.

O CI novo foi disparado no run GitHub Actions `35948099372`. Attempts 1 e 2 terminaram em failure antes de qualquer step, ambos com `runner_id=0`, `runner_name=""` e `steps=[]`. Portanto a causa exata de infraestrutura/conta do Actions permanece UNKNOWN e **nenhum build/vet/test/race/govulncheck chegou a executar**. A tarefa não deve ser marcada como plenamente validada enquanto esse gate não rodar.

O ambiente container local disponível usa Go 1.23.2 e não foi usado para fingir validação do baseline Go 1.27.1.


## Início da implementação L3-002 — hardened session storage

Em 2026-09-24 o mantenedor informou que a cota de horas do GitHub Actions está indisponível e autorizou continuar o desenvolvimento sem esperar por esse canal de CI. A limitação foi mantida como Evidence externa reportada, sem transformar jobs que não receberam runner em falha do código.

Foi criada a branch `feat/limiar3-l3-002-telegram-foundation` a partir de `refactor/limiar3-repository-rebaseline@afd81238e8f9e66a17c8bb30978978daeb6006ca`.

O Implementation Gate de L3-002 foi considerado satisfeito para a primeira fatia: L3 ADR 001/002 estão Accepted, Go 1.27.1 e gotd v0.162.0 foram revalidados como versões atuais aplicáveis, owner/riscos/limites estão documentados e existe harness local proporcional.

Foi implementado `internal/telegram/sessionstore` para o escopo Linux/single-host/single-process. O storage possui o mesmo method set de `gotd/session.Storage` sem criar interface espelho, trata ausência física separadamente de corrupção/I/O, publica por temp privado + write completo + fsync + rename + dir fsync, valida owner/perms, rejeita symlink/non-regular e coordena writers intra-processo por canonical path.

Validação observada no harness local: `go test -count=10 ./...` PASS, `go vet ./...` PASS e `go test -race -count=1 ./...` PASS. O ambiente local usa Go 1.23.2 e não possui rede para baixar Go 1.27.1/gotd; portanto integração exata com o pin real e govulncheck continuam UNKNOWN. Os blobs remotos dos dois arquivos são idênticos aos blobs dos arquivos testados localmente.


## 2026-09-24/25 — Continuação L3-002: runtime, bootstrap, query e observabilidade

A branch `feat/limiar3-l3-002-telegram-foundation` foi reconciliada no HEAD observado `65a7e42d87ec2db8b6346800a42324ffefde219e` antes desta entrada. Além do hardened session storage já registrado, a implementação agora inclui owner único por `TelegramAuthorizationIdentity`, bootstrap administrativo staged (QR-first e code/2FA hash fallback), runtime steady-state fail-closed com semantic readiness/same-self, `TelegramQuery` read-only bounded (`ResolvePeer` + `History`), cache de peers memory-only bounded, cursor history com message ID + date, error taxonomy/FLOOD_WAIT e observabilidade low-cardinality sem payload/raw error.

Foi feita verificação estática das APIs efetivamente usadas contra a tag upstream `gotd/td v0.162.0`: `session.Storage`, `auth.Flow`, `PasswordHashProvider`/`PasswordWith`, `qrlogin.OnLoginToken`/`QR.Auth`, `telegram/message/peer`, `telegram/query/messages` e `Client.API`. Nenhuma incompatibilidade material foi encontrada nessa auditoria estática.

O gate exato de toolchain continua **UNKNOWN**: o GitHub Actions permanece indisponível por horas conforme informado pelo mantenedor e a tentativa local de obter Go 1.27.1 falhou por bloqueio de rede. Não foram executados `go build/vet/test/race/govulncheck` no Go 1.27.1, nem login Telegram real, OTP/2FA, bootstrap/restart, first RPC, reconnect, clock-skew ou cross-DC. Portanto L3-002 permanece **EM ANDAMENTO**, sem merge/deploy e sem declaração de production-ready.


## 2026-09-24 — L3-002: observabilidade segura e harness real de restart/reuse

A implementação foi refinada sem ampliar o boundary arquitetural. Foram adicionados eventos low-cardinality opcionais para lifecycle do runtime, operações `ResolvePeer`/`History` e `session_load`/`session_store`. Os eventos contêm somente alias local da authorization identity, operação/state, outcome, classe semântica, duração e `retry_after`; não carregam `error` bruto, PeerRef, self user ID, payload de mensagem, session bytes, phone, OTP, 2FA ou API hash. Panic do observer é isolado para não derrubar o Telegram boundary.

Runtime e bootstrap agora podem observar o storage por wrapper interno que preserva o contract `gotd/session.Storage`; nenhuma nova authority/storage foi criada.

Foi criado `internal/telegram/runtime_real_linux_test.go` com build tag `linux && telegram_real`. Esse gate é **opt-in** e nunca roda no CI normal. Ele não faz bootstrap nem solicita credencial: exige uma sessão de teste/disposable já provisionada via environment do operador, inicia **dois subprocessos distintos** sobre o mesmo hardened session file e verifica semantic readiness/same-self; quando `LIMIAR_TELEGRAM_PEER_REF` está configurado, cada subprocesso também executa `ResolvePeer + History`. Saídas de erro são sanitizadas e nenhum segredo é colocado em fixture.

Com isso, a implementação de código prevista para a fundação L3-002 está substancialmente fechada. Permanecem como Evidence **UNKNOWN**, não PASS: compilação/testes/race/vuln no Go 1.27.1, bootstrap real, restart/reuse real, first RPC real, reconnect, clock-skew e cross-DC. Não houve login Telegram, OTP/2FA real, credencial, merge ou deploy nesta execução.

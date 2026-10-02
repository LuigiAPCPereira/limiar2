# PROJECT_STATE — Limiar 3.0 (checkpoint documental)

**Data de preparação:** 2026-09-22 (America/Bahia). **Última reconciliação:** 2026-10-01. **Repositório:** `LuigiAPCPereira/limiar2`. **Branch atual:** `fix/limiar3-platform-agnostic-20260929`. **Base:** `feat/limiar3-l3-003-mcp-realtime@f6ab9ced2477ef17aedc7845fe3edbf720057a99`. **Entrega:** Draft PR #220. Revalidar HEAD, base, main e gates no início da próxima execução; este checkpoint não registra o SHA do próprio commit. Não houve merge/deploy.

## Objetivo confirmado pelo mantenedor

Limiar 3.0: reconstrução integral do agrupador de promoções, incremental e bottom-up. Sessão e integração Telegram/MTProto ficam atrás de boundaries desacopláveis; MCP também é detachable. O MCP realtime consulta Telegram diretamente e entra cedo para investigação do domínio; collector/Evidence/recovery formam outro ramo durável; depois o MCP pode consultar dados processados via Query Service. Não existe segundo app e desacoplamento não implica microserviços. Qualidade requerida: segurança, desempenho medido, simplicidade, legibilidade e manutenção de longo prazo. Black Friday 2026 é objetivo, não prazo validado.

## Tarefas e estado

**Frentes ativas:** `L3-004` está **LIVE ADMISSION + DURABILITY BARRIER VALIDADOS / L3 ADRs 006/007 ACCEPTED / SOURCE SYNC STATE STORAGE PENDENTE**; o live ingress está no Draft PR #225, a `DurabilityBarrier` no Draft PR #226 e a Decision de SourceSyncState na branch `docs/limiar3-l3-004-source-sync-state`. `L3-PLAT-001` está **VALIDADA EM CI MULTIPLATAFORMA / PR DRAFT NÃO MERGEADA** no Draft PR #220; `L3-003` está **TRACER IMPLEMENTADO / TOOLCHAIN CI PASS / INTEGRAÇÃO REAL PENDENTE**; `L3-002` está **IMPLEMENTADA / TOOLCHAIN CI PASS / NÃO VALIDADA EM INTEGRAÇÃO TELEGRAM REAL**. Nenhuma dessas frentes autoriza Telegram real, tunnel, merge ou deploy.

**Integridade das fontes:** o Engineering DNA histórico permanece preservado nesta ref conforme os artefatos L3 já registrados. Para o Agent Development Protocol, esta branch L3 ainda contém `docs/DOCUMENTATION_AND_CONTINUITY.md` no blob histórico `78b2e86564fb287886f9df065fdb727b20c52727`. O Draft PR #214 (`docs/adopt-agent-protocol-v2-20260920@91d521a296bc11445f3ab5e7f0dad1ec819ed561`) contém uma versão posterior do protocolo, blob `7a5e4a39bb13b5e0bb750caba9acc5377ffefc62`, com a política explícita de pt-BR. Portanto, **não assumir que a cópia desta branch, a cópia do ChatGPT Project e o PR #214 estão sincronizados**; verificar a ref concreta em cada execução. O estado da adoção v2 no PR #214 permanece **ADOÇÃO PARCIAL**, sem merge.

**L3-001, L3-001A, L3-001B e L3-001C estão internalizadas como investigação/Evidence/Proposal, não como Decisions arquiteturais.** Fontes: [`L3_001_SESSION_BOUNDARY_INVESTIGATION.md`](L3_001_SESSION_BOUNDARY_INVESTIGATION.md), [`L3_001A_MTPROTO_GOTD_INVESTIGATION.md`](L3_001A_MTPROTO_GOTD_INVESTIGATION.md) e [`L3_001B_PRODUCTION_FOUNDATION_RESEARCH.md`](L3_001B_PRODUCTION_FOUNDATION_RESEARCH.md). O conjunto sustenta owner por `TelegramAuthorizationIdentity`, main gotd client único, private session storage, bootstrap explícito/fail-closed, semantic Ready/self binding, peer state authorization-scoped e guards externos ao updates.Manager. L3-001B adiciona toolchain/supply-chain, threat model, performance, observability e hardening de produção.

**Decisions de fundação aceitas:** L3 ADR 001 (session storage), L3 ADR 002 (authorization/runtime), L3 ADR 003 (repository topology/legacy containment), L3 ADR 004 (MCP Telegram realtime) e L3 ADR 005 (neutralidade de plataforma) estão `Accepted`. A ADR 005 usa acceptance reference `c940373f46d2a522e3f3c0d74ab72ecaa0f28d1f` e prevalece somente sobre as cláusulas de plataforma conflitantes dos ADRs 001/002; os demais contratos permanecem. Go 1.27.1 e `github.com/gotd/td v0.162.0` seguem como baseline inicial aceita para L3-002 sob `Current Stable First`.

**L3-001C — stop condition atingida:** [L3_001C_GOTD_UPSTREAM_AUDIT.md](L3_001C_GOTD_UPSTREAM_AUDIT.md) conclui que não há blocker upstream para iniciar L3-002 e que não é necessária nova pesquisa ampla de fundação. A extensão mínima proposta é hardened `session.Storage`, runtime ownership/readiness, TelegramQuery adapter, error translation e bounded admission; gotd permanece owner de reconnect/pools/migration/RPC/MTProto/query/media machinery.

**Validação executável reconciliada:** o candidato `a4f7836f0673f234b54d1a950b33f212b6106ab1` passou no GitHub Actions run #764 (`36846123134`). Windows: Go 1.27.1, módulo, build, vet e testes PASS. Ubuntu: Go 1.27.1, módulo, contenção do legado, build, vet, testes, race e `govulncheck v1.8.0` PASS. Isso valida a neutralidade de compilação/teste do slice e remove o UNKNOWN de toolchain de L3-002/L3-003 nessa árvore; não valida Telegram real, Secure MCP Tunnel ou integração ChatGPT real.

**Validação L3-004/S5-S6:** o kernel de Source Evidence em `internal/evidence` passou no run #768 (`36902381203`) e o checkpoint documental passou no run #769 (`36903786595`). O core genérico de Source Admission em `internal/acquisition` passou integralmente no run #770 (`36917149254`) no candidato `a268b6306a691cf0a30c2ba56e70ec8c6e3e2b8b`: Windows com módulo/build/vet/test PASS; Ubuntu com os mesmos gates + contenção do legado + `CGO_ENABLED=0` + race + govulncheck PASS. O S6 valida apenas o ordering `append Evidence -> forward`, incluindo `append failure -> no forward`; não valida wiring Telegram, barrier/supervisor, recovery, sync, backfill ou `subscription_id` produtivo.

**Decision L3-004 / subscription identity:** a **L3 ADR 006 Accepted** resolve o bloqueio do `subscription_id`: Acquisition Subscription explícita, estável, não secreta, anterior ao ingress e separada de canal, autorização Telegram e target MCP. O modelo suporta pluralidade sem exigir framework multi-subscription no MVP; mudanças materiais do scope recebem nova identidade enquanto Evidence não carregar versão de configuração; sobreposição exige Evidence por subscription antes do forward protegido.

**Decision L3-004 / SourceSyncState:** a **L3 ADR 007 Accepted** substitui, para Limiar 3, a Proposal histórica 021 como authority de schema/boundary: common state usa `(subscription_id,user_id)`, channel state usa `(subscription_id,user_id,channel_id)`, setters parciais não criam state e writes produtivas permanecem guardadas por `DurabilityBarrier`/`GuardedStateStorage`.

**Próxima ação:** validar a branch documental da ADR 007 e, em slice separado, implementar migration SQL + repository/capability estreita de `SourceSyncState`; depois integrar `GuardedStateStorage`/Supervisor. Manter `Runtime.NoUpdates=true` até o recovery path estar composto. Sem merge/deploy ou Telegram real.

## Gate paralelo L3-003 — MCP Telegram realtime

Em 2026-09-24 foi feita uma investigação estreita da superfície MCP atual, sem reabrir pesquisa ampla de fundação. A Evidence externa material foi revalidada em 2026-09-29 antes do Decision Gate:

- `modelcontextprotocol/go-sdk v1.8.0` continua a latest stable upstream observada; declara Go 1.25.0 e suporta MCP `2026-07-28`;
- MCP `2026-07-28` usa core stateless/sessionless; o SDK Go serve essa revisão sobre Streamable HTTP com `Stateless: true`;
- a stable v1.8.0 adiciona hardening de transporte/resource exhaustion e correções de lifecycle; `PropagateRequestCancellation` é opt-in, request body é bounded por default e a proteção de localhost/DNS rebinding fica habilitada salvo override;
- `ServerOptions.Capabilities == nil` anuncia `logging` por compatibilidade histórica; como logging está deprecated em `2026-07-28`, a Proposal foi refinada para capabilities explícitas mínimas;
- OpenAI Secure MCP Tunnel continua permitindo conectar MCP privado a produtos suportados sem inbound público; permissões de tunnel/workspace são externas/separadas e **não foram presumidas**.

O [L3 ADR 004](adr/004-mcp-telegram-realtime-boundary.md) está **Accepted** desde 2026-09-29. A Decision mantém SDK oficial v1.8.0, Streamable HTTP stateless, read scope por targets nomeados, listener loopback-only, trust model single trusted operator, cancelamento HTTP propagado, proteção localhost/cross-origin, request body bounded e capabilities MCP mínimas sem `logging` default. Busca/filtragem/comparação pertencem ao ChatGPT; o MCP entrega acesso read-only fiel e paginável. Secure MCP Tunnel é reachability operacional, não credential/auth Telegram. Não há OAuth próprio no primeiro slice; non-loopback/public/multi-user/write actions exigem nova Decision proporcional.

**Implementação L3-003 iniciada:** o tracer slice foi codificado em `feat/limiar3-l3-003-mcp-realtime@202fe46e1c6ab59e3c0721dea71c02ef14c9a54f`. Ele pina `modelcontextprotocol/go-sdk v1.8.0`, expõe `telegram.targets` e `telegram.history` sobre `TelegramQuery`, valida target antes de I/O, usa default 20 e teto técnico 100 por página, devolve `next_cursor` para continuidade sem teto total artificial entre páginas, marca a resposta como realtime/not-Evidence e sanitiza erros. O adapter HTTP foi codificado stateless, com propagação de cancelamento, listener apenas loopback, cross-origin protection e request body bounded. Testes fake-backed com o client MCP oficial foram executados dentro de `go test ./...` e passaram no run #764; build/vet/test em Ubuntu+Windows e race/govulncheck no Ubuntu também passaram no candidato `a4f7836f0673f234b54d1a950b33f212b6106ab1`. Não houve Telegram real, Secure MCP Tunnel real, PR, merge ou deploy. L3-002 permanece IMPLEMENTADA NO CÓDIGO / NÃO VALIDADA EM INTEGRAÇÃO REAL.

## Restrições e desconhecidos

ADRs históricos 021–024 permanecem conforme registry da rebaseline e não foram promovidos; os ADRs exclusivos do Limiar 3 têm registry próprio em `docs/limiar3/adr/`, com L3 ADR 001/002 Accepted; EXP-019 só prova harness Unix/intra-processo, não runtime; `legacy/limiar2/internal/storage/sqlite` + PR #211 são implementação parcial herdada, não Limiar 3.0 integrado. Imagens têm falha relatada, causa não reproduzida. O root já contém a fundação funcional L3-002 sob `internal/telegram/`: `sessionstore`, ownership, bootstrap, runtime, query, errors, observability e o teste opt-in `runtime_real_test.go` com build tag funcional `telegram_real`. O ambiente local disponível possui Go 1.23.2; tentativa de obter Go 1.27.1 via toolchain automático falhou por bloqueio de rede, portanto a validação exata permanece UNKNOWN. Para a fatia de storage, um módulo de harness equivalente em Go 1.23.2 executou `go test -count=10`, `go vet` e `go test -race` com PASS; os Git blob SHAs dos arquivos testados coincidem com os blobs remotos. As APIs usadas por runtime/bootstrap/query foram confrontadas com as fontes upstream de gotd v0.162.0 (`session.Storage`, `auth.Flow`, `PasswordHashProvider`/`PasswordWith`, `qrlogin.OnLoginToken`/`QR.Auth`, `telegram/message/peer`, `telegram/query/messages`, `Client.API`). Isso é verificação estática de compatibilidade, não build nem integração. Telegram/MCP não foram conectados; não houve credenciais, PR, merge ou deploy. Decisão final de diretórios, processo, sessão, identidade do consumidor e integração ChatGPT fica para investigação/autoridade apropriada; sem esquema/ID fictício.

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


## L3-BASE-001 — repository rebaseline / legacy containment

Por autorização explícita do mantenedor, foi criada a branch `refactor/limiar3-repository-rebaseline` a partir de `docs/limiar-3-foundation-20260922@00e5c422...`. L3 ADR 003 foi aceito com referência durável.

A movimentação estrutural preservou 311 blobs no primeiro commit de árvore para `legacy/limiar2/`, incluindo `cmd`, `internal`, `scripts`, `tools`, `experiments`, módulo Go legado, workflows e documentação histórica/rebaseline. O módulo legado mantém blob `go.mod` `32b740c3d7aee68c2fb21729bf2a999b86676cf6` e `module github.com/limiar/collector`.

O root L3 iniciou `module github.com/LuigiAPCPereira/limiar2`, `go 1.27.1`, `toolchain go1.27.1`, CI único L3 e package sentinela sem lógica. A verificação remota encontrou 158 arquivos Go sob `legacy/limiar2`; fora de tooling oculto `.agents`, o único Go de produto/root é `internal/foundation/doc.go`. O único workflow ativo no root é `.github/workflows/ci.yml`; oito workflows anteriores estão preservados sob legacy e não executam como workflows do repositório.

A validação de build/test/race com Go 1.27.1 ainda precisa ocorrer em CI porque o ambiente local disponível nesta execução é Go 1.23.2. Isso é condição de fechamento, não Evidence inventada.


## Início de L3-002

> **Registro histórico:** esta seção descreve a primeira implementação de 2026-09-24. A restrição Linux ali registrada foi corrigida e supersedida pela L3 ADR 005 em 2026-09-29. Não usar os nomes `_linux.go`, UID/`syscall` ou o escopo Linux como contrato corrente.


Em 2026-09-24, o mantenedor informou que o GitHub Actions está sem horas disponíveis e permanecerá bloqueado por enquanto, e autorizou continuar o desenvolvimento. Esse fato é tratado como limitação externa de validação, não como falha do código nem blocker de implementação.

Revalidação externa antes do código:
- Go 1.27.1 continua listado como stable atual pelo Go oficial;
- gotd/td v0.162.0 continua a latest release upstream;
- o contract v0.162.0 de `session.Storage` permanece `LoadSession(context.Context) ([]byte,error)` + `StoreSession(context.Context,[]byte) error`; o Loader converte resultado vazio em `session.ErrNotFound`.

Primeira fatia implementada:
- `internal/telegram/sessionstore/file_linux.go` blob `47993f6823c5d3383281185c54d748e0196682bb`;
- `internal/telegram/sessionstore/file_linux_test.go` blob `9889f56e555c8b921b0000f6adcd3b6b03c938da`;
- escopo Linux/single-host/single-process;
- parent/target ownership e private perms fail-closed;
- symlink/non-regular rejected;
- temp no mesmo diretório com 0600, full write, file sync, atomic rename, directory sync;
- coordenação intra-processo explícita e cancelável por path;
- fault injection apenas interno aos testes;
- bytes da sessão nunca entram em erros.

Validação local proporcional: `go test -count=10 ./...`, `go vet ./...` e `go test -race -count=1 ./...` PASS em harness Go 1.23.2 sem dependência externa. Isso valida a lógica stdlib e concorrência, mas **não** substitui o gate exato Go 1.27.1 + gotd real + govulncheck.


## Checkpoint L3-002 — fundação Telegram implementada

**HEAD observado antes deste checkpoint:** `399f2465af38b64ea6505f0386982c4faf19ddef`. O SHA muda com os próprios commits documentais deste checkpoint; revalidar a ref antes de usar como base de implementação.

| Gate / componente | Estado | Evidência atual |
| --- | --- | --- |
| Go 1.27.1 + gotd v0.162.0 pinados | **Implementado** | root `go.mod` / `go.sum` |
| Credential/session storage portável | **Implementado; CI multiplataforma PASS no PR #220** | stdlib portável; sem `syscall`/UID; symlink/non-regular rejeitados; escrita temporária + file sync + rename; `go test ./...` PASS em Ubuntu/Windows e race PASS no Ubuntu no run #764 |
| Owner único por authorization identity | **Implementado** | `AuthorizationCoordinator`; runtime e bootstrap compartilham lease |
| Bootstrap explícito | **Implementado, não integrado com conta real** | QR-first; code/2FA fallback via SRP hash; staging antes do commit; replacement exige self binding |
| Runtime steady-state | **Implementado, não integrado com Telegram real** | preflight; razões de rebootstrap; `Auth().Status`; same-self; capability só após readiness |
| `TelegramQuery` read-only | **Implementado, não integrado com Telegram real** | `ResolvePeer` + `History`; limits explícitos; cache de peers bounded/memory-only; cursor ID+date; sem `tg.*` no contract |
| Error taxonomy / FLOOD_WAIT | **Implementado** | cancellation preservada; unauthorized/peer/access/flood/transient/internal |
| Observabilidade segura | **Implementado** | eventos low-cardinality; sem session/OTP/password/API hash/payload/raw error; panic do observer isolado |
| API upstream v0.162.0 usada pelo código | **Verificação estática PASS** | signatures confrontadas diretamente com a tag v0.162.0 |
| Build/vet/test/race/govulncheck em Go 1.27.1 | **PASS EM CI** | run #764 (`36846123134`) no candidato `a4f7836f0673f234b54d1a950b33f212b6106ab1`: build/vet/test Ubuntu+Windows; race+govulncheck Ubuntu |
| Bootstrap → restart/reuse → same-self → first RPC | **UNKNOWN** | requer ambiente/credenciais explicitamente autorizados |
| Reconnect / clock-skew / cross-DC | **UNKNOWN** | experimentos estreitos ainda não executados |

**Leitura operacional:** a fundação está codificada, mas L3-002 não está encerrada. O bloqueio restante é de validação/integração, não uma justificativa para adicionar mais framework ao Telegram boundary sem Evidence.

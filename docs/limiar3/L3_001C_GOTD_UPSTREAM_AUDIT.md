# L3-001C — auditoria upstream-only do gotd/td

**Estado epistemológico:** investigação concluída como Evidence/Findings + Proposal técnica. Não é Decision arquitetural por si só, não promove ADR e não autoriza código.

**Ref documental observada pelo relatório:** `docs/limiar-3-foundation-20260922@bd9e8f0c6ba8b9e7d1c856c7c3a6fd8f1ff41a70`.

**Origem:** relatório aprofundado fornecido pelo mantenedor nesta conversa após execução da auditoria upstream-only. A internalização preserva suas conclusões e limitações; esta escrita documental não repete integralmente a pesquisa externa nem transforma recomendações em Decisions.

## 1. Resumo executivo

A auditoria upstream-only concluiu que a direção de L3-001/A/B continua correta, mas que o Limiar deve construir uma camada **menor** ao redor do gotd.

Stable upstream observada no relatório:

```text
github.com/gotd/td v0.162.0
Telegram API Layer 229
release: 2026-09-18
```

A versão previamente investigada pelo projeto era `v0.161.0`, Layer 228.

**Recomendação técnica do relatório:** pin de `v0.162.0` para iniciar L3-002, sujeito à aceitação do mantenedor e aos gates reais de build/test/race/vulnerability/integration.

Não foi encontrada regressão material em auth/session/runtime/query que justificasse permanecer em v0.161.0. O delta relevante é principalmente schema/layer, dependencies e override avançado de layer.

## 2. Conclusão arquitetural principal

> O TelegramRuntime não deve “encapsular gotd fazendo outro gotd”. Deve possuir gotd, fornecer lifecycle/política/segurança e expor somente a semântica necessária aos consumidores.

O gotd já deve permanecer owner de:

- MTProto framing/crypto;
- auth-key protocol;
- server salt/protocol sessions;
- msg_id/seq_no/ACK;
- request multiplexing;
- retransmission;
- reconnect;
- connection pools;
- DC selection/migration;
- authorization transfer;
- CDN/sub-DC pools;
- generated Telegram API;
- helpers de query/media;
- PFS machinery quando habilitada.

O Limiar acrescenta:

- `TelegramAuthorizationIdentity`;
- config/secrets policy;
- hardened `session.Storage`;
- exactly one owned main `telegram.Client`;
- steady-state auth policy;
- semantic readiness;
- root lifecycle/cancellation;
- bounded consumer admission;
- error translation;
- safe observability;
- uma capability pequena de consulta.

## 3. Stable atual e pin recomendado

### v0.161.0 → v0.162.0

| Área | v0.161.0 | v0.162.0 | Leitura para L3 |
| --- | --- | --- | --- |
| Telegram layer | 228 | **229** | favorece stable atual |
| auth Flow | base já investigada | sem mudança material observada | risco baixo |
| session contract | mesmo modelo | mesmo modelo | neutro |
| FileStorage | helper simples | helper simples | continua inadequado ao threat model L3 |
| QR login | disponível | disponível | neutro |
| PFS | disponível | disponível | neutro |
| query/history | disponível | disponível com schema atual | favorece stable |
| updates engine | mesma família | concerns recentes ainda aplicáveis | defer |
| peers | experimental/WIP | experimental/WIP | não usar como fundação |
| layer override | não | **sim** | escape hatch; não usar sem necessidade |
| dependencies | estado de julho | estado de setembro | stable atual |

O relatório não encontrou motivo técnico para iniciar L3-002 em v0.161.0.

A política `Current Stable First` é reforçada: stable atual é o default; permanecer atrás exige regressão/incompatibilidade concreta.

## 4. Maturity/capability map

| Componente | Função upstream | Maturidade observada | Direção L3 |
| --- | --- | --- | --- |
| `telegram.Client` | owner high-level de MTProto/DC/lifecycle | core | **USE DIRECTLY** |
| `telegram.Options` | config hooks/runtime | core | usar seletivamente |
| `tg.Client` | API Telegram gerada | core/generated | usar internamente |
| `telegram/auth` | phone/code/2FA/SRP | core | bootstrap |
| QR login | login-token + migration | helper real | candidato operacional |
| `auth/srpguard` / PasswordHashProvider | reduz handling plaintext 2FA | especializada | considerar no bootstrap |
| `session.Storage` | persistence SPI | core | **USE DIRECTLY** como interface interna |
| `session.FileStorage` | arquivo simples | helper básico | **AVOID** em produção L3 |
| `telegram/query` | pagination/iterators | helper core | **USE DIRECTLY** internamente |
| `telegram/message/peer` | peer resolver/lazy refs | helper | **USE/ADAPT** |
| `telegram/peers.Manager` | peer manager/cache | **experimental/WIP** | **AVOID em L3-002** |
| `telegram/updates` | ordering/gaps/difference | substancial com limites | **DEFER/GUARD** |
| downloader/uploader | media transfer | helpers substanciais | **USE DIRECTLY depois** |
| `tgerr` | error classification | core | usar internamente |
| MTProto/RPC/pools | protocolo/concurrency/reconnect | core | não duplicar |
| PFS | temp auth keys | core opcional | experiment first |
| middleware | invoker interception | core mechanism | parcimônia/reentrancy |
| `gotd/contrib` | addons opcionais | package-specific | avaliar individualmente |

## 5. Extension points reais

### Usar diretamente

- `telegram.Client.Run`;
- `session.Storage`;
- `tg.Client` dentro do adapter;
- `auth.Flow`/QR no bootstrap;
- `tgerr`;
- `telegram/message/peer`;
- `telegram/query/messages`;
- downloader/uploader nas fatias futuras;
- reconnect/pools/DC migration/MTProto internals.

### Adaptar/wrap mínimo

- hardened implementation de `session.Storage`;
- runtime ownership/readiness;
- DTO/capability de peer/history;
- semantic error mapping;
- logger sanitizado;
- bounded admission/concurrency.

### Evitar inicialmente

- `session.FileStorage` como credential storage production;
- `telegram/peers.Manager` como foundation;
- retry framework próprio;
- reconnect manager próprio;
- connection pool próprio;
- paginator próprio;
- global flood scheduler;
- generic TelegramProvider;
- gotd types atravessando consumer contracts.

### Experimentar posteriormente

- PFS;
- live updates/recovery;
- peer persistence;
- media/CDN;
- workload fairness;
- qualquer otimização própria de buffers/memory.

## 6. Achado crítico: middleware reentrancy / floodwait

A auditoria registrou issue upstream reproduzível envolvendo `gotd/contrib/middleware/floodwait.Waiter`.

Mecanismo relatado:

```text
user RPC
  ↓
global floodwait middleware
  ↓
gotd invoke
  ↓
DC migration
  ↓
auth.exportAuthorization
  ↓
reentra na mesma middleware chain
  ↓
middleware serializador espera worker
  ↑
worker já bloqueado na request externa
```

A implementação stable observada mantém authorization export através da chain correspondente, de forma compatível com o risco de reentrância apontado.

**Proposal para L3-002:** não instalar `floodwait.Waiter` como middleware global.

FLOOD_WAIT deve inicialmente ser:

- classificado semanticamente;
- acompanhado de wait duration quando disponível;
- preservado através do boundary;
- tratado por política do workload.

MCP realtime e collector/backfill podem exigir políticas diferentes.

Isso reforça a decisão anterior: **não criar retry/flood policy global prematuramente**.

## 7. Achado crítico: updates.Manager

O upstream Manager possui:

- common/channel state;
- pts/qts/seq;
- gap detection;
- getDifference/getChannelDifference;
- state storage;
- access-hash storage;
- callbacks;
- limit de channel-difference concurrency.

Mas o upstream também documenta limites e a auditoria encontrou issues atuais relacionados a:

- common-state gap/drop;
- channel difference timeout/recovery delay;
- custo operacional em contas com milhares de canais.

Conclusão:

- não pertence a L3-002;
- não deve ser tratado como Evidence store;
- quando collector chegar, revisar stable upstream novamente;
- manter Source Admission + recovery/state guards fora do Manager;
- não presumir escala barata.

Isso confirma e fortalece L3-001A, em vez de contradizê-la.

## 8. Achado operacional: clock skew / connected-but-not-progressing

A auditoria registrou relato upstream recente de clock skew suficientemente grande levando mensagens do servidor a serem rejeitadas continuamente, produzindo aparência de hang em vez de diagnóstico claro.

Estado epistemológico: **relato aberto com reprodução/análise upstream**, não bug reproduzido pelo Limiar.

Consequência proposta:

```text
process running
  !=
socket connected
  !=
authorization restored
  !=
Telegram operational / Ready
```

A fundação deve possuir:

- startup/first-RPC deadlines;
- semantic readiness;
- observabilidade de lack-of-progress;
- experimento estreito de clock skew antes de declarar production-ready.

Isso não bloqueia iniciar L3-002.

## 9. Auth/bootstrap

Upstream fornece:

- phone/code;
- 2FA/SRP;
- QR login;
- login token migration;
- `PasswordHashProvider`/SRP guards.

Proposal preservada:

```text
ADMIN BOOTSTRAP
  auth.Flow ou QR
        ↓
  authorization criada
        ↓
  session.Storage

STEADY STATE
  LoadSession
        ↓
  telegram.Client
        ↓
  authorization valid?
```

Nenhuma capability upstream justifica auto-login silencioso do daemon.

QR continua bom candidato de experiência operacional; phone/code/2FA pode ser fallback controlado. Essa escolha é Decision pequena do mantenedor, não nova pesquisa ampla.

## 10. Session storage

A interface upstream correta já é suficientemente estreita:

```go
LoadSession(context.Context) ([]byte, error)
StoreSession(context.Context, []byte) error
```

Não criar interface duplicada apenas para renomear o mesmo contrato.

Backend production upstream `FileStorage` continua insuficiente para os requisitos L3:

- mutex por instância;
- read/write simples;
- sem publicação temp+fsync+rename;
- sem coordination compartilhada por path;
- TODO upstream para robust write/rename.

Conclusão:

- **USE DIRECTLY:** `session.Storage`;
- **ADAPT:** hardened implementation própria;
- **AVOID:** upstream `FileStorage` como credential store production.

ADR 023 continua Proposed.

## 11. Peer resolution

O audit diferencia:

### `telegram/peers.Manager`
- grande;
- cache/state;
- experimental/WIP;
- não indicado como foundation do L3.

### `telegram/message/peer` + query resolver
- resolução menor/lazy;
- integra bem com history/query;
- suficiente para L3-002.

Portanto L3-002 pode começar com:

- on-demand peer resolution;
- access hash interno;
- zero persistent peer DB.

Persistência só entra quando restart/query volume demonstrar benefício.

## 12. Query/history

`telegram/query/messages` já cobre boa parte da mecânica:

- peer refs/resolvers;
- offsets;
- batch size;
- pagination iterator.

O Limiar acrescenta:

- limit enforcement;
- cursor contract externo;
- DTO conversion apenas na boundary;
- semantic error mapping;
- cancellation/deadlines;
- bounded admission.

A implementação não deve refazer paginator.

Capability conceitual continua:

```go
type TelegramQuery interface {
    ResolvePeer(ctx context.Context, ref PeerRef) (PeerDescriptor, error)
    History(ctx context.Context, peer PeerKey, page HistoryPage) (MessagePage, error)
}
```

Internamente, usar gotd idiomaticamente.

## 13. Media

Downloader/uploader upstream já possuem:

- chunking;
- workers;
- CDN;
- retry hooks;
- pools;
- streaming/parallel machinery.

Não criar downloader próprio agora.

Futura MediaCapability deve acrescentar apenas:

- identity/source semantics;
- re-resolution;
- budgets;
- destination/streaming contract;
- error classification.

## 14. Concurrency

Design upstream é baseado em:

- request multiplexing;
- connection pools;
- paralelismo interno;
- downloader/uploader workers.

A auditoria não encontrou contract simples dizendo que qualquer composição é magicamente thread-safe sem interferência operacional, mas não encontrou justificativa para client por capability.

Direção:

- exatamente um main client por authorization identity;
- compartilhar entre capabilities;
- fairness pertence ao boundary da aplicação;
- não usar mutex global para serializar tudo;
- não criar scheduler sofisticado antes do workload.

## 15. Performance

Claims upstream incluem baixo overhead/memory e generated code reflection-free, mas não são SLO do Limiar.

Fundamentos úteis:

- generated TL/API;
- buffer-oriented encoding;
- request multiplexing;
- connection reuse;
- helper buffer pools;
- fast paths de crypto.

Não é zero-copy ponta a ponta; wire messages são materializadas em memória.

L3 deve medir:

- startup -> Ready;
- Load/Store session;
- first RPC;
- ResolvePeer hit/miss;
- first/steady History;
- allocations;
- heap/RSS;
- goroutines;
- StoreSession frequency;
- concurrency/cancellations.

Adiar custom pools/unsafe/zero-copy/PGO até profiling.

## 16. Segurança e postura upstream

A auditoria registra que gotd:

- implementa MTProto completo e protections relevantes;
- possui security policy;
- possui histórico de correção de vulnerability pré-auth;
- não possui SLA;
- não possui auditoria formal externa declarada;
- continua em major version 0.

Isso não justifica reimplementar MTProto.

Implica:

- pin exato;
- go.sum;
- review de release notes;
- govulncheck;
- integration tests;
- SDK types contidos;
- session/security policy do Limiar independente.

## 17. Testing upstream

Upstream atual possui:

- Go stable + oldstable;
- Linux/macOS;
- Windows stable;
- race detector;
- unit tests;
- E2E;
- cron/canary;
- benchmarks;
- test server/mocks;
- fuzzing infrastructure.

Consequência:

Limiar não precisa duplicar genericamente:

- AES/TL correctness;
- ACK;
- basic reconnect;
- protocol codec.

Limiar testa sua **composição**:

- credential durability;
- auth reuse;
- semantic readiness;
- cancellation;
- boundary;
- error translation;
- concurrency;
- relevant upstream edge cases.

## 18. Known issues e impacto

| Issue/risco | Estado | Impacto | L3-002 |
| --- | --- | --- | --- |
| clock skew / hang aparente | relato upstream com reprodução/análise | core runtime diagnostics | não bloqueia início; production gate |
| migration + global Waiter deadlock | relato reproduzível | middleware/DC migration | evitar Waiter contém risco |
| common pts gap/drop | relato + PR/test | updates | collector gate |
| channel diff timeout | bug-labeled upstream | updates/channel recovery | collector gate |
| milhares de channels | measurement/report | update scale | future workload |
| peers experimental | documentação | peer helper maturity | não usar Manager |
| sem audit formal | fato upstream | security posture | gates internos |
| sem SLA | fato upstream | maintenance | pin + tests |

## 19. Uso público

A auditoria encontrou projetos públicos usando gotd em workloads reais, inclusive tooling de Telegram com mídia/export.

Esses projetos são Evidence de uso, não prova de performance/correção do Limiar.

Failure reports upstream são mais úteis para nosso threat/failure model que popularidade.

## 20. Menor extensão L3 legítima

A auditoria reduz a primeira fundação a:

```text
1. HardenedSessionStorage
   implements gotd session.Storage

2. TelegramRuntime
   owns authorization identity + Client.Run + readiness + lifecycle

3. TelegramQuery adapter
   maps peer/history semantics ↔ gotd query/tg

4. Error translation
   cancellation / unauthorized / flood / peer / transient

5. Minimal bounded admission
   prevent unbounded consumer pressure
```

Não implementar agora:

- custom MTProto;
- reconnect manager;
- connection pool;
- retry framework;
- peer DB;
- updates engine;
- paginator;
- media downloader;
- generic TelegramProvider;
- global flood scheduler;
- priority scheduler;
- OTel stack;
- custom byte pools;
- zero-copy machinery;
- PFS manager;
- process split/IPC.

## 21. Experimentos restantes

### A — gotd stable + hardened SessionStore + restart

```text
bootstrap
 -> StoreSession
 -> shutdown
 -> new process
 -> LoadSession
 -> authorized
 -> same self
 -> read-only RPC
 -> zero reauth
```

Registrar StoreSession frequency.

### B — concurrent query + cancellation/reconnect

- multiple Resolve/History;
- cancellations;
- induced reconnect;
- race/leak checks;
- no duplicate client;
- no double retry.

### C — clock-skew failure diagnostic

Condição simulada/reproduzida precisa terminar em bounded deadline/diagnóstico, não hang opaco.

### D — cross-DC migration sem global Waiter

Smoke controlado que prove que migration normal não depende do middleware problemático.

Não exigir ainda experimentos de updates, thousands-of-channels, PFS, media throughput ou encrypted remote storage.

## 22. Gates L3-002 refinados

### Correctness
- explicit authorization identity;
- exactly one main owner;
- bootstrap separado;
- absence/incompatible/revoked distintos;
- no silent reauth;
- restart/reuse real;
- semantic Ready/self binding.

### Security
- current/pinned dependencies;
- credential fora do Evidence DB;
- hardened storage;
- zero secrets em logs;
- SDK/session/access hash não cruzam consumers;
- govulncheck.

### Lifecycle
- `Client.Run` owned pelo runtime;
- shutdown root cancela children;
- caller cancellation não derruba runtime;
- reconnect pertence ao gotd;
- connection-state hooks não bloqueiam.

### Performance
- baseline registrado;
- no optimization without profile.

### Boundary
- fake query capability possível sem gotd;
- sem interface espelho do `tg.Client`;
- sem generic provider.

### Failure
- storage failure != absence;
- revoked != transient;
- FLOOD_WAIT != generic retry;
- clock/lack-of-progress bounded e diagnosticável;
- cancellation preservada.

### Observability
- runtime/auth state;
- reconnect/dead;
- startup/first-RPC latency;
- RPC error class;
- FLOOD_WAIT;
- StoreSession count/latency;
- active queries;
- cancellations.

## 23. Blockers restantes

A auditoria **não encontrou blocker upstream para iniciar L3-002**.

Blockers restantes pertencem ao projeto:

- decisão/aceitação do gotd pin recomendado;
- credential storage / ADR 023 ou substituto;
- plataforma inicial;
- deployment model inicial;
- bootstrap surface;
- Implementation Gate;
- autorização explícita de código.

Riscos upstream atuais são contidos por design/experimento estreito.

## 24. Impacto em L3-001/A/B

| Conclusão anterior | Resultado L3-001C |
| --- | --- |
| TelegramAuthorizationIdentity | confirma |
| owner único | confirma |
| main client único | confirma/reforça |
| private session storage | confirma fortemente |
| hardened file candidate | confirma como adapter necessário |
| bootstrap separado | confirma |
| no silent reauth | confirma |
| tg.* contido | confirma com ajuste: uso livre dentro do adapter |
| ResolvePeer + History | confirma e simplifica via helpers upstream |
| peer persistence inicial | torna menos necessária |
| updates.Manager guards | confirma e aumenta cautela |
| generic retry inexistente | confirma fortemente |
| global flood waiter | refina: evitar agora |
| PFS futuro | confirma |
| observability proporcional | confirma |
| Current Stable First | confirma |

## 25. Stop condition

A stop condition foi atingida.

**Versão recomendada:** `github.com/gotd/td v0.162.0`.

**Use directly:** Client/Run, internal tg.Client, reconnect/pools/migration/MTProto machinery, session.Storage interface, auth/QR bootstrap helpers, tgerr, peer/query helpers, media helpers posteriormente.

**Adapt:** hardened session store, runtime ownership/readiness, TelegramQuery DTO/boundary, error mapping, safe logging, bounded admission.

**Avoid:** production FileStorage upstream, peers.Manager na fundação, generic retry, duplicate reconnect/pools, global floodwait Waiter nas condições atuais, gotd types cruzando consumer contracts.

**Experiment:** session restore/reuse real, concurrent query/cancellation, clock-skew diagnostic, cross-DC migration; PFS/updates/media apenas nas fatias futuras.

## 26. Conclusão

Não é recomendada nova pesquisa ampla antes de L3-002.

Os unknowns restantes são experimentais e estreitos. Continuar auditoria geral produziria rendimento decrescente e contrariaria o objetivo do protocolo: reduzir incerteza até poder executar a menor mudança segura/verificável.

A recomendação final de foundation é:

```text
Validated Config
      │
TelegramAuthorizationIdentity
      │
Hardened session.Storage
      │
TelegramRuntime
  owns one telegram.Client
      │
 ┌────┴─────┐
peer       query/messages
 └────┬─────┘
      │
TelegramQuery
ResolvePeer + History
```

Por baixo, gotd permanece owner de reconnect, pools, migration, RPC/MTProto/ACK/retry/crypto.

O maior risco operacional ainda não eliminado é um runtime que pareça conectado mas não produza progresso. A fundação deve distinguir process-running, connected, authorization-restored e Telegram-operational/Ready sem construir um supervisor excessivo.

## 27. Limitação documental

O relatório original declarou que o Engineering DNA integral permaneceu preservado no repositório como gzip/base64, com identidade/hash documentados, mas aquele ambiente não realizou nova verificação byte-a-byte do plaintext descompactado. Esta internalização mantém a mesma limitação e não inventa uma nova verificação.

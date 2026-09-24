# L3-001B — pesquisa de fundação de produção Auth/Session/TelegramRuntime

**Estado epistemológico:** investigação concluída como Evidence/Findings + Proposal técnica; não é Decision arquitetural por si só e não autoriza implementação.

**Ref observada no relatório original:** `docs/limiar-3-foundation-20260922@678935b00cd7645da254d22c1d963441123eba3c`.

**Origem:** relatório aprofundado fornecido pelo mantenedor nesta conversa, combinando fontes do projeto, histórico Limiar, Telegram/MTProto, gotd, Go/toolchain, filesystem, segurança, performance, observabilidade e supply chain.

## 1. Resultado executivo

A investigação considera suficiente a base conceitual de L3-001/L3-001A e desloca o foco para uma fundação de produção mínima, moderna e verificável.

Direção proposta:

```text
Current stable Go toolchain
        ↓
typed/fail-closed config
        ↓
TelegramAuthorizationIdentity
        ↓
private hardened session.Storage
        ↓
explicit administrative bootstrap
        ↓
one TelegramRuntime
        ↓
one main gotd/telegram.Client
        ↓
authorization status + self binding
        ↓
semantic Ready
        ↓
real process restart/reuse proof
        ↓
TelegramQuery
ResolvePeer + History
```

A fundação não deve reimplementar transporte, reconnect, pools, DC migration, ACKs, MTProto protocol sessions, retry de baixo nível ou downloader. Esses concerns permanecem no gotd.

## 2. Refinamentos principais

### 2.1 Toolchain atual e corrigido

A investigação identificou que a linha operacional antiga do projeto não deve ser usada simplesmente por inércia. Para uma geração nova, o toolchain deve partir da versão stable atual e corrigida, sujeita aos gates reais do repositório.

A recomendação do relatório era não construir a Evidence nova sobre Go 1.26.2. Após discussão posterior com o mantenedor, a política foi refinada para **Current Stable First**: Go stable atual é o baseline preferencial; versão anterior exige incompatibilidade/regressão demonstrada.

### 2.2 gotd atual, mas sem upgrade cego

A versão investigada em profundidade por L3-001A foi `gotd/td v0.161.0`. O relatório L3-001B também pesquisou o estado upstream mais recente, packages relevantes, roadmap, issues e release recente.

A política posterior do mantenedor é: **a versão stable atual deve ser o default de avaliação para L3-002**. Permanecer em uma versão anterior só porque já foi investigada não é motivo suficiente. Antes de piná-la, confrontar changelog, bugs, schema/layer e testes de compatibilidade.

## 3. Current Stable First

Direção explícita do mantenedor:

> Para uma fundação nova, usar ferramentas e dependências stable atuais por default. Uma versão inferior só é escolhida quando houver uma razão técnica concreta, documentada e demonstrável.

Aplicação:

1. identificar a versão stable atual;
2. ler release notes/changelog e security notes;
3. verificar compatibilidade com contracts do Limiar;
4. executar build/test/race/vet/security gates;
5. piná-la exatamente;
6. usar features novas **seletivamente**, apenas quando melhorarem correção, segurança, performance, observabilidade ou manutenção do escopo real.

Isto não significa usar `latest` cegamente, adotar APIs experimentais nem reescrever código para demonstrar modernidade.

### Modernidade útil

Preferir ganhos entregues pelo próprio runtime/toolchain/biblioteca antes de criar otimizações próprias:

- runtime/allocator/GC atual;
- profiling e diagnostics atuais;
- context/cancellation atuais;
- reconnect/pooling/migration do gotd;
- iterators/query helpers do gotd;
- downloader/media machinery do gotd quando chegar a fatia.

Adiar até Evidence:

- PGO;
- `sync.Pool` customizado;
- unsafe;
- mmap;
- io_uring;
- custom allocator;
- zero-copy sofisticado;
- scheduler complexo;
- microservices;
- storage remoto de sessão.

## 4. Lições dos Limiares anteriores

A Evidence histórica reforça:

- não escolher storage de sessão por conveniência de um DB existente;
- não misturar auth, peer cache, history, update handlers e reconnect num client de aplicação grande;
- não duplicar mecanismos que o gotd já possui;
- separar credential authority, peer state, update/recovery state e Evidence;
- preservar o relato do bug histórico de mídia sem inventar sua causa;
- usar legado como Evidence/regressão, não como arquitetura-alvo.

## 5. TelegramRuntime — responsabilidade mínima

O runtime deve possuir:

- `TelegramAuthorizationIdentity`;
- config validada e referências de segredo;
- um main `gotd/telegram.Client`;
- private `session.Storage`;
- bootstrap/steady-state separation;
- auth status;
- self identity binding;
- semantic readiness;
- root lifecycle/cancellation;
- error translation;
- safe observability;
- futuras application-level concurrency budgets.

Não deve possuir implementações próprias de:

- MTProto framing/crypto;
- auth-key exchange;
- ACK/container machinery;
- reconnect loop;
- DC pools/migration;
- raw TL codec;
- CDN/downloader internals;
- generic retry framework.

## 6. Authorization binding

A investigação propõe detectar session file trocado/acidentalmente pertencente a outra conta.

Shape conceitual:

```text
authorization alias: primary
expected Telegram self user ID: ...
```

`Ready` não deve significar apenas “Client.Run começou”.

Semantic Ready proposto:

```text
gotd running
AND authorization == authorized
AND self identity obtida
AND self corresponde ao binding esperado
AND root lifecycle saudável
```

## 7. Bootstrap

Proposal:

```text
limiar3 telegram bootstrap
  -> QR-first
  -> 2FA quando exigido
  -> phone/code fallback explícito
```

Regras:

- operação administrativa explícita;
- não acontece silenciosamente no daemon;
- inputs humanos são efêmeros;
- nunca logs estruturados para OTP/password/QR token;
- steady-state unauthorized/revoked/incompatible => fail closed / rebootstrap required.

A implementação concreta ainda exige Decision/autorização.

## 8. Session storage e threat model

Threat model inicial proposto:

### Proteger contra

- leak acidental em config/log;
- usuário local não privilegiado;
- permissões incorretas;
- escrita parcial;
- writers intra-processo;
- backup de Evidence levando a sessão junto;
- path/symlink inesperado;
- storage unreadable virando login silencioso;
- sessão revogada tratada como healthy.

### Fora da promessa inicial

- root/kernel comprometido;
- arbitrary code com mesmo UID;
- arbitrary code dentro do processo;
- host/container escape com acesso ao processo;
- disco roubado sem FDE;
- adversário com plaintext já presente na RAM.

Backend candidato:

```text
Linux
single-host
single-process per authorization identity
service user dedicado
private parent directory
hardened atomic local file
```

ADR 023 continua Proposed. A escolha permanente requer decisão explícita.

## 9. Criptografia at-rest

Não adicionar application-level encryption apenas por aparência de segurança.

Para o threat model inicial:

- mode/owner + parent privado protegem contra outro usuário local;
- FDE é mais proporcional para roubo físico;
- envelope encryption/KMS passa a fazer sentido quando session files/backups saem do host ou houver governança central de chaves;
- root/same-process compromise não é resolvido apenas por cifrar o arquivo se o processo também possui a KEK.

## 10. gotd ecosystem — conclusões do relatório

Componentes considerados úteis:

- `telegram.Client`: core;
- `session`: contrato privado de storage;
- `telegram/auth`: bootstrap;
- QR login: opção operacional relevante;
- `tg.*`: interno ao adapter;
- `telegram/query`: pagination helpers;
- `telegram/updates`: futuro acquisition/recovery, guardado;
- `telegram/downloader`: futura mídia;
- `tgerr`: mapeamento de erros;
- `telegram/peers`: possível implementação interna, porém não contract duradouro.

Não importar indiscriminadamente packages `contrib` ou middlewares apenas porque existem.

## 11. Peer state

Primeira fatia pode ser memory-first.

Persistência só depois de Evidence de:

- custo real de resolve após restart;
- impacto sobre disponibilidade;
- corpus e quantidade de peers;
- necessidade de access hashes após restart.

Quando persistido, peer state permanece authorization-scoped e separado da credential.

## 12. Concorrência e fairness

Default:

- um main gotd client por authorization identity;
- não criar um client por capability;
- não serializar tudo com mutex global;
- ponto futuro de admission control/budget;
- sem priority scheduler antes de workload.

Fairness MCP/collector/media será resolvida quando cada workload existir e for medido.

## 13. Retry/FLOOD_WAIT

Não criar retry framework genérico.

O gotd já trata reconnect/migration/retries em diferentes camadas, e componentes como downloader possuem policy própria.

Para RPC geral:

- classificar FLOOD_WAIT;
- manter metadata de espera;
- preservar cancellation;
- policy automática só no workload que realmente precisar.

## 14. Go/runtime/performance

Prioridade:

- usar toolchain stable atual e corrigido;
- medir antes de otimizar;
- aproveitar melhorias do runtime sem complexidade de aplicação.

Baseline relevante:

- session load/store latency;
- startup -> Ready;
- auth status/self;
- first RPC;
- ResolvePeer hit/miss;
- first/steady History page;
- StoreSession frequency;
- reconnect duration;
- goroutines;
- heap/allocations;
- concurrency;
- FLOOD_WAIT;
- logging/tracing overhead.

Auth não é hot path; não otimizar login por microbenchmark.

## 15. Profiling

Ferramentas proporcionais:

- Go benchmarks;
- benchstat;
- CPU/heap/allocation profiles;
- goroutine profile;
- mutex/block profile;
- execution trace;
- race detector.

PGO só depois de workload MCP/collector representativo.

## 16. Memory/zero-copy

- session bytes: clareza/ownership e segurança > microallocation;
- nenhum `sync.Pool` de material sensível;
- converter somente DTOs necessários;
- não duplicar árvores `tg.*`;
- buffers de mídia ficam inicialmente com downloader gotd;
- unsafe/mmap/io_uring/custom allocators somente depois de Evidence/profiling.

## 17. Observabilidade

Base proposta:

- `slog`;
- allowlist/redaction;
- low-cardinality metrics;
- pprof opt-in em ambiente apropriado.

Sinais:

- authorization identity alias;
- runtime/auth state;
- reconnect;
- operation name;
- latency;
- error class;
- flood wait;
- session load/store count + duration;
- cancellations;
- concurrency.

Não logar:

- session/auth key;
- API hash;
- phone;
- OTP;
- 2FA password;
- QR token;
- message text por default;
- InputPeer/access hash;
- serialized Telegram objects.

OpenTelemetry completo pode esperar até causalidade distribuída justificar.

## 18. Supply chain

A fundação deve verificar:

- toolchain stable/corrigido;
- `go mod verify`;
- `govulncheck`;
- build/vet/tests/race;
- static/security lint vigente;
- dependency pinning;
- review explícito de upgrades estruturais.

Versão antiga não é “mais segura” apenas por ser conhecida.

## 19. Experimentos indispensáveis

### A — gotd real + session storage + restart/reuse

```text
bootstrap
 -> StoreSession
 -> shutdown
 -> new process
 -> LoadSession
 -> authorized
 -> same self binding
 -> read-only RPC
 -> zero reauth
```

Também exercer invalid/revoked/incompatible.

### B — hardened filesystem fault matrix

Falhar em:

- temp create;
- write;
- fsync;
- rename;
- directory sync;
- permission correction;
- ENOSPC;
- read-only FS;
- symlink/non-regular target.

Invariante: old snapshot intact OR new complete snapshot; nunca partial/mix.

### C — one-client concurrency + reconnect

Queries concorrentes, cancellations e interrupção controlada de rede; sem duplicate main client, race/leak ou retry duplicado.

### D — PFS

Somente quando production default precisar ser decidido.

## 20. Gates propostos para L3-002

### Correctness

- explicit authorization identity;
- one main owner;
- bootstrap separado;
- absence/incompatible/revoked distintos;
- no silent reauth;
- restart/reuse real;
- self binding antes de Ready.

### Security

- current patched toolchain;
- credential fora do Evidence DB;
- private directory/file;
- storage failure != absence;
- zero secrets em logs;
- session fora de backup geral;
- SDK/session/access hash não cruzam consumers.

### Reliability

- context propagation;
- request cancellation não derruba runtime;
- root cancellation encerra runtime;
- fail-closed storage/auth;
- clean race/leak gates.

### Performance

- baselines registrados;
- StoreSession frequency medida;
- no optimization without profile.

### Maintainability

- gotd contido;
- contracts pequenos;
- sem provider genérico;
- sem wrapper do SDK;
- sem generic retry.

## 21. Decisões ainda necessárias

Antes de código, permanecem decisões explícitas sobre:

- stable Go/toolchain baseline efetiva;
- stable gotd baseline efetiva;
- Linux como plataforma inicial;
- single-process deployment inicial;
- session backend/ADR 023 ou substituto;
- threat model aceito;
- bootstrap surface.

PFS, peer persistence, FLOOD_WAIT UX, updates/recovery e media devem ser decididos quando suas fatias bloquearem.

## 22. Conclusão

A investigação considera a fundação conceitualmente suficiente para implementação após decisões/gates. A regra é aproveitar capacidades atuais do toolchain/runtime/gotd primeiro e acrescentar complexidade própria somente quando Evidence mostrar necessidade.

O maior risco restante não é falta de abstração: é construir a Evidence de lifecycle sobre versões/dependências que não são as que pretendemos realmente usar ou assumir que restore/auth/readiness funcionam como imaginado sem executar a prova real end-to-end.

# L3 ADR 002 — Telegram authorization identity, runtime ownership e lifecycle

Authority: Decision Record — Limiar 3.0
Status: Accepted
Accepted-by: Mantenedor do Limiar
Accepted-at: 2026-09-23
Acceptance-reference: `baa626b59ec21d36f0dbde9a8a1fb3bbb5a4a2a6`

## Contexto

L3-001A distinguiu `auth_key`/authorization persistida de MTProto `session_id`. L3-001B/C investigaram production hardening e o gotd como biblioteca upstream e concluíram que o Limiar deve possuir o gotd, não reconstruí-lo.

A unidade de ownership do Limiar é `TelegramAuthorizationIdentity`, uma identidade/configuração opaca de aplicação associada à autorização Telegram que o runtime decidiu possuir.

## Decision

### Owner único

Cada `TelegramAuthorizationIdentity` possui exatamente **um main `telegram.Client`** em um único `TelegramRuntime` no deployment inicial.

O deployment inicial é Linux, single-host e single-process. Consumers como MCP realtime e futuros collector/backfill recebem capabilities; não criam main clients próprios.

Conexões auxiliares, pools, media/DC connections e authorization transfer criados internamente pelo gotd permanecem responsabilidade do SDK e não violam o owner único.

### gotd como engine

O gotd permanece owner de:

- MTProto framing/crypto;
- auth-key protocol;
- protocol sessions, salts, msg_id/seq_no e ACK;
- request multiplexing/retransmission;
- reconnect/backoff;
- connection pools;
- DC selection/migration/authorization transfer;
- generated Telegram API;
- low-level retries;
- query/media machinery quando usada.

O Limiar não criará um segundo reconnect manager, connection pool, MTProto client, paginator ou retry framework genérico.

### Baseline inicial

Para L3-002, a baseline aceita é:

- Go 1.27.1;
- `github.com/gotd/td v0.162.0`, pin exato.

Atualizações futuras seguem `Current Stable First`. Uma atualização que preserve estes contratos pode passar por gate proporcional sem novo ADR; mudança arquitetural exige Decision.

### Bootstrap versus steady-state

Bootstrap é operação administrativa explícita e separada do daemon/runtime normal.

Surface aceita:

- QR-first como caminho operacional preferido;
- phone/code/2FA como fallback controlado;
- inputs humanos efêmeros e nunca logados.

Steady-state:

- restaura credential storage;
- verifica authorization status;
- verifica identidade `self` esperada;
- incompatível/revogada/não autorizada => fail closed / rebootstrap required;
- **nunca** inicia login automaticamente.

### Semantic readiness

`Ready` não equivale a processo rodando, socket conectado ou `Client.Run` ativo.

O runtime só pode declarar semantic readiness quando, no mínimo:

- gotd lifecycle está ativo;
- autorização está válida;
- `self` foi obtido;
- `self` corresponde ao binding esperado daquela `TelegramAuthorizationIdentity`;
- root lifecycle está saudável.

Operações reais continuam com context/deadline próprios; um first-RPC smoke faz parte da validação de integração.

### Primeira capability

A primeira capability de consumidor será read-only, bounded e Telegram-aware sem expor SDK:

- `ResolvePeer`;
- `History` (recent messages pode ser apenas a primeira página bounded).

Inicialmente:

- peer resolution on-demand;
- sem persistent peer DB;
- access hashes permanecem internos;
- `tg.*`, `tg.InputPeer`, `telegram.Client`, session bytes e access hashes não cruzam consumer contracts;
- dentro do Telegram adapter, usar `tg.*`, `telegram/message/peer` e `telegram/query/messages` idiomaticamente é permitido e desejável.

### Errors, retry e FLOOD_WAIT

O boundary preserva categorias semânticas como cancellation/deadline, unauthorized/revoked, peer unavailable/access denied, FLOOD_WAIT com retry metadata, transient availability e internal/protocol failure.

Não haverá retry framework genérico do Limiar nem `gotd/contrib/middleware/floodwait.Waiter` global na primeira fundação.

### Concorrência

O main client pode servir capabilities concorrentes, com bounded admission onde necessário. Não serializar todas as RPCs com mutex global nem criar priority scheduler antes de workload/Evidence.

### Deferred

Ficam fora de L3-002:

- PFS como requisito;
- updates.Manager/live recovery;
- Source Admission/Evidence;
- persistent peer state;
- media capability;
- OpenTelemetry completo;
- PGO/custom pools/unsafe/zero-copy;
- serviço/IPC/multiprocess split.

## Gates de implementação

1. build/test/vet/race/vulnerability gate com Go/gotd pinados;
2. hardened session store conforme L3 ADR 001;
3. bootstrap explícito em ambiente autorizado;
4. process restart/reuse sem novo login;
5. same-self binding;
6. first read-only RPC;
7. concurrent Resolve/History + cancellation sem leak/race;
8. induced reconnect bounded;
9. clock-skew/lack-of-progress diagnostic antes de production-ready;
10. cross-DC migration smoke sem global floodwait Waiter;
11. zero secrets/payloads sensíveis em logs.

## Consequências

O Telegram boundary fica menor: ownership, lifecycle, segurança, policy, DTO/error translation e admission. A mecânica MTProto permanece upstream.

Isso mantém o boundary desacoplável por contrato sem exigir processo separado.
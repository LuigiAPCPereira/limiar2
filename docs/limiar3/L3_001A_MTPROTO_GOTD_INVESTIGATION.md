# L3-001A — investigação MTProto + gotd/td e boundary Telegram do Limiar 3.0

**Estado epistemológico:** investigação concluída como Evidence/Findings e recomendação técnica; não é Decision nem autoriza implementação.

**Ref investigada:** `docs/limiar-3-foundation-20260922@01d2a5a67fa13bdf1c15f0dbe60424e2af6b41ff`.

**Versão gotd investigada:** `github.com/gotd/td v0.161.0` conforme `go.mod`, com inspeção direcionada à versão fixada pelo repositório.

**Origem:** relatório produzido em outro chat e fornecido pelo mantenedor para internalização. Esta escrita preserva as conclusões e limites do relatório; não promove ADRs Proposed e não substitui as fontes upstream verificadas na investigação original.

## 1. Resultado principal

A direção central de L3-001 foi sustentada, porém com ajustes obrigatórios antes de L3-002.

O menor boundary correto é um **owner único da autorização Telegram e do `telegram.Client` gotd**, com lifecycle explícito, enquanto gotd continua owner de transporte, MTProto, conexões, DC migration, ACKs, protocol sessions, message IDs MTProto, salts, containers e mecânica de reconnect/retry interno.

Acima disso, o Limiar expõe apenas capabilities source-aware que preservem semânticas realmente necessárias: peer identity, Telegram message identity, history pagination, erros operacionais relevantes e, futuramente, observações/recovery de updates.

A recomendação anterior “um TelegramRuntime por session identity” deve ser refinada para:

> **um TelegramRuntime autoritativo por TelegramAuthorizationIdentity**, isto é, por credencial/autorização Telegram persistida que o Limiar decidiu possuir; nunca por MTProto `session_id`.

## 2. Correções materiais em relação a L3-001

### 2.1 Session identity era ambíguo

Existem conceitos distintos:

| Conceito | Significado |
| --- | --- |
| `auth_key` | material criptográfico que sustenta a autorização |
| MTProto `session_id` | identificador efêmero de uma instância do protocolo |
| Telegram authorization/login session | autorização da conta gerenciada pelo Telegram |
| gotd session blob | snapshot persistido contendo credencial + bootstrap/conectividade |

O owner arquitetural do Limiar é a **authorization identity**, não o MTProto `session_id`.

### 2.2 Owner único ganhou importância operacional

A investigação verificou que múltiplos main clients/conexões independentes usando a mesma autorização podem causar `AUTH_KEY_DUPLICATED` em condições definidas pelo Telegram. Portanto, owner único do `gotd.Client` principal por authorization identity não é apenas preferência de organização: é um controle de lifecycle e segurança.

### 2.3 `updates.Manager` não garante Evidence-before-progress sozinho

O `updates.Manager` v0.161.0 é adequado para ordering, gap detection e recovery, porém há caminhos em que falhas de persistência de pts/state ou erros downstream são logados enquanto estado em memória pode continuar avançando.

Logo, os invariantes dos ADRs 016–018 exigem composição externa:

- live Source Admission **antes** do Manager;
- GuardedRecoveryAPI envolvendo `getState/getDifference/getChannelDifference`;
- GuardedStateStorage + durability barrier;
- supervisor fail-stop, pois apenas retornar erro ao Manager não é suficiente em todos os caminhos.

### 2.4 `session.ErrNotFound` não significa sempre “arquivo ausente”

No loader de sessão da versão investigada, incompatibilidade de versão do blob pode resultar em erro que satisfaz `session.ErrNotFound`.

Portanto:

- o backend físico do Limiar deve continuar distinguindo ausência real de EACCES/I/O/corrupção;
- o runtime normal **não** deve traduzir qualquer `session.ErrNotFound` upstream em autorização automática para novo login;
- bootstrap/login precisa ser operação explícita e separada do steady-state runtime.

### 2.5 Peer cache é authority separada, porém authorization-scoped

F-STO-006 continua correto: peer state não é session storage.

Ajuste: `access_hash` e estado operacional de peer não devem ser tratados como universais. Quando persistidos, devem ser escopados à authorization identity/generation apropriada.

## 3. Boundary recomendado

```text
config / secret refs
        |
        v
+----------------------------------+
| TelegramRuntime                  |
|                                  |
| TelegramAuthorizationIdentity    |
| gotd telegram.Client             |
| private session.Storage          |
| auth/readiness lifecycle         |
| peer/access-hash internals       |
| error translation                |
| application concurrency budgets  |
| safe observability               |
+----------------+-----------------+
                 |
        capability, not SDK
                 |
+----------------v-----------------+
| TelegramQuery                    |
| ResolvePeer                      |
| History / recent first page      |
+-------------+--------------------+
              |
        +-----+------+
        |            |
        v            v
 MCP realtime   future backfill
```

Não em L3-002:

```text
raw Telegram Updates
        |
Source Admission
        |
updates.Manager
        |
collector
```

Recovery futuro:

```text
updates.Manager
        |
GuardedRecoveryAPI
        |
Source Admission
        |
response to Manager

Manager SetState/SetChannelPts
        |
GuardedStateStorage / durability barrier
        |
supervisor fail-stop on invariant failure
```

## 4. O que gotd deve continuar possuindo

O Limiar não deve modelar nem reimplementar:

- MTProto framing;
- auth-key exchange;
- MTProto `msg_id`;
- MTProto `seq_no`;
- `server_salt`;
- MTProto `session_id`;
- message containers;
- `msgs_ack`;
- bad-message protocol handling;
- time correction interna;
- raw reconnect loop;
- DC transfer algorithm;
- Telegram TL codec.

Esses concerns pertencem ao gotd/MTProto.

O Limiar deve conhecer apenas semânticas que afetam seu próprio produto/authority: peers, Telegram message IDs, history pagination, authorization health, FLOOD_WAIT metadata, update state no acquisition subsystem, discontinuities e provenance.

## 5. Session storage

O gotd já fornece o contrato mínimo necessário:

```text
LoadSession(ctx) ([]byte, error)
StoreSession(ctx, []byte) error
```

Não criar uma segunda interface pública do Limiar apenas para espelhar `session.Storage`. A implementação concreta pode satisfazer diretamente esse contrato dentro do Telegram boundary.

### Candidato de backend

O hardened local file Unix/Linux continua tecnicamente coerente com as Evidence existentes, porém:

- ADR 023 continua **Proposed**;
- EXP-LIMIAR-018 continua Evidence contra `FileStorage` upstream “as-is” no cenário testado;
- EXP-LIMIAR-019 continua Evidence limitada a Unix/Linux intra-processo;
- ainda faltam gotd real + restart/reuse, lifecycle de revogação e fault tests da implementação efetiva.

### Lifecycle recomendado

Separar:

```text
explicit bootstrap
  -> inputs efêmeros de phone/code/password
  -> auth.Flow / authorization
  -> persist session
  -> exit/hand over to normal runtime

normal runtime
  -> restore session
  -> verify authorization/readiness
  -> authorized => serve capabilities
  -> unauthorized/revoked/incompatible => fail closed
  -> nunca solicitar OTP silenciosamente
```

Apagar storage local não equivale a revogação remota.

## 6. Peers

O contract de consumidor não deve expor `tg.InputPeer` nem `access_hash`.

Shape recomendado:

```text
PeerKey
  kind
  telegram_id
```

O Telegram boundary mantém ou reobtém o access hash.

Persistência de peer state, quando necessária, deve permanecer separada da sessão e escopada à authorization identity/generation.

O package `telegram/peers` do gotd pode ser usado internamente se útil, porém não deve definir o contract duradouro do Limiar.

## 7. História e backfill

`messages.getHistory` e mecanismos equivalentes respondem ao estado histórico observável.

`updates.getDifference` / `getChannelDifference` respondem à sincronização/recovery de eventos.

Essas autoridades **não devem ser fundidas**.

### Identidade de mensagem

Não usar somente integer message ID.

A identidade source-aware mínima é:

```text
PeerKey + TelegramMessageID
```

### Pagination

Não expor a combinação inteira de parâmetros TL como API pública do Limiar.

Preferir cursor/source-aware abstraction mínima que permita ao adapter usar o iterator gotd e preservar paginação correta.

### DTO de mensagem para exploração

A primeira representação não é o modelo de promoção. Deve preservar apenas semântica Telegram necessária, por exemplo:

- PeerKey;
- TelegramMessageID;
- date;
- message kind;
- text quando presente;
- edit timestamp quando presente;
- service marker/action family;
- grouped_id quando presente;
- media presence/type;
- cursor/page metadata;
- UNKNOWN/unmapped kind quando necessário.

## 8. Updates e recovery

Estados Telegram relevantes ao acquisition subsystem:

- pts;
- pts_count;
- qts;
- seq;
- date;
- channel pts.

Eles **não** pertencem ao MCP nem ao core comercial.

### Regra durável

```text
LIVE:
raw update
 -> Source Admission
 -> durable Evidence
 -> updates.Manager
 -> ordered/recovered update
 -> collector projection

RECOVERY:
updates.Manager
 -> GuardedRecoveryAPI
 -> recovery RPC
 -> Source Admission
 -> durable recovery observation
 -> response to Manager

STATE:
Manager wants SetState/SetChannelPts
 -> durability barrier
 -> allowed only if corresponding Evidence is durable
 -> otherwise error + external supervisor fail-stop
```

Erro do handler/storage do Manager não deve ser tratado sozinho como circuit breaker suficiente.

## 9. Retry, reconnect e FLOOD_WAIT

Políticas são diferentes por mecanismo:

- reconnect seguro: gotd possui lógica própria;
- DC migration: gotd possui tratamento específico;
- downloader: possui retry/flood behavior próprio;
- `FLOOD_WAIT` em RPC geral: não deve receber um retry genérico indiscriminado do Limiar.

Não criar generic retry middleware em L3-002.

Para MCP realtime, Proposal inicial:

- traduzir FLOOD_WAIT para erro semântico;
- preservar `retry_after`;
- não prender request interativo por minutos sem política explícita.

Collector/recovery poderá ter política diferente quando seu lifecycle existir.

## 10. Concorrência

A arquitetura interna do gotd suporta trabalho concorrente, mas a investigação não encontrou contrato público suficiente para afirmar que qualquer workload combinado MCP + collector é livre de contenção operacional.

Conclusão:

- **default:** compartilhar um único main `telegram.Client` por authorization identity;
- não criar clients principais concorrentes sobre a mesma autorização;
- não serializar tudo com mutex global do Limiar;
- adicionar budgets/semaphores somente onde Evidence justificar;
- validar fairness/reconnect com workload controlado antes de live collector.

## 11. Logging e observabilidade

Não habilitar debug indiscriminado do gotd como política de produção.

Preferir observabilidade controlada:

- connection state;
- reconnect;
- RPC method name/ID;
- latency;
- flood wait;
- recovery reason;
- queue/concurrency metrics.

Evitar dumps de Telegram objects, sessão, OTP, senha, API hash e payloads de mensagem.

## 12. Mídia — implicação futura

Media permanece fora de L3-002.

A futura capability deve ser source-aware e capaz de reobter location/file reference a partir da mensagem quando necessário.

Não usar `file_reference`/location como identidade duradoura.

Shape conceitual futuro:

```text
MediaSourceRef
  peer
  message_id
  media selector / kind
```

A investigação **não** diagnosticou o bug histórico de imagens do Limiar 1.

## 13. Validação das 12 hipóteses de L3-001

| Hipótese | Resultado |
| --- | --- |
| Um TelegramRuntime por session identity | **SUSTENTADA COM AJUSTE** — usar TelegramAuthorizationIdentity |
| gotd.Client possuído exclusivamente pelo runtime | **SUSTENTADA** |
| SessionStore privado ao Telegram boundary | **SUSTENTADA** |
| MCP realtime e collector compartilham runtime/capabilities | **SUSTENTADA COM AJUSTE** — fairness precisa Evidence |
| gotd.Client/tg/session bytes não cruzam boundary | **SUSTENTADA COM AJUSTE** — preservar semântica Telegram nos DTOs |
| MCP realtime read-only direto ao Telegram | **SUSTENTADA** |
| History separado de updates/recovery | **SUSTENTADA** |
| Peer state separado de session state | **SUSTENTADA COM AJUSTE** — authorization-scoped |
| Evidence separado de session state | **SUSTENTADA** |
| Source Admission antes de durable state advancement | **SUSTENTADA COM AJUSTE** — live + guarded recovery + guarded state + supervisor |
| Runtime inicialmente single-process | **SUSTENTADA** |
| Separação física futura sem reescrever consumers | **SUSTENTADA COM AJUSTE** — live stream precisará contrato próprio |

Nenhuma hipótese central foi classificada como não sustentada.

## 14. Contracts mínimos recomendados para L3-002

Não criar `TelegramProvider`, interface por método ou espelho de `tg.Client`.

Capability coesa proposta:

```go
type TelegramQuery interface {
    ResolvePeer(ctx context.Context, ref PeerRef) (PeerDescriptor, error)
    History(ctx context.Context, peer PeerKey, page HistoryPage) (MessagePage, error)
}
```

Nomes/assinaturas finais continuam em aberto.

`RecentMessages` pode ser apenas a primeira página bounded de `History`; não merece método próprio sem semântica adicional demonstrada.

### Taxonomia mínima de erros

Preservar categorias semânticas:

- cancellation/deadline;
- unauthorized/revoked/incompatible authorization;
- peer not found/unavailable;
- access denied/private;
- flood/rate limit com retry metadata;
- temporarily unavailable/network/reconnect timeout;
- internal/protocol failure.

Não vazar centenas de `tgerr` nem reduzir tudo a um único `ErrTelegram`.

## 15. O que não criar em L3-002

- TelegramProvider genérico;
- MTProtoClient próprio do Limiar;
- wrapper por método de `tg.Client`;
- UpdateCapability produtiva;
- MediaCapability;
- generic DB abstraction para session + peer + Evidence;
- IPC/process split;
- generic retry framework;
- universal message model;
- normalização completa de mídia;
- pts/qts/seq API para MCP;
- storage persistente de peer antes de requisito real.

## 16. Sequência bottom-up revisada

A investigação move o smoke real de restore/reuse para antes da primeira capability estável:

```text
runtime/config
 -> private session storage
 -> TelegramRuntime / authorization lifecycle
 -> gotd real + restart/reuse smoke
 -> first read-only TelegramQuery
 -> MCP realtime
 -> Source Admission + Evidence
 -> updates.Manager + guarded recovery/state
 -> live updates/recovery
 -> backfill orchestration
 -> media
 -> corpus exploration
 -> Deterministic Findings
 -> promotion model
 -> Query Service
 -> MCP Limiar data / API
 -> frontend
```

Motivo: lifecycle real de `Client.Run`, loader, auth status, reconnect e session persistence precisa ser provado antes de consumidores dependerem do runtime.

## 17. Riscos que bloqueiam L3-002

- dois owners da mesma authorization identity;
- nomenclatura que confunda MTProto `session_id` com credencial/autorização;
- auto-login/rebootstrap após restore incompatível;
- uso de `FileStorage` upstream como solução final de segurança;
- access hash vazando pelo contract;
- retry genérico duplicando políticas do gotd;
- logging/debug vazando Telegram objects ou segredos.

## 18. Experimentos materiais

### Gates imediatos da fundação

1. **gotd real + SessionStorage + restart/reuse**
   - exact v0.161.0;
   - bootstrap controlado;
   - instrumentar chamadas Load/Store sem conteúdo;
   - shutdown/start;
   - authorization status;
   - query read-only;
   - sucesso = segundo start autorizado sem novo OTP.

2. **StoreSession lifecycle**
   - observar frequência/ordem/pontos de escrita;
   - nenhuma suposição de performance antes dessa Evidence.

### Antes de live collector

3. **one-client concurrency + reconnect**
   - workload read-only concorrente;
   - disconnect controlado;
   - deadlines;
   - sem race/leak e sem novo client principal.

4. **Manager + Source Admission fault matrix**
   - exact v0.161.0;
   - fake API/storage;
   - falhas em admission/recovery/state;
   - nenhum progress certification após Evidence failure;
   - supervisor encerra acquisition quando necessário.

5. **authorization lifecycle**
   - revoke/invalid authorization;
   - old local blob;
   - runtime termina como unauthorized/rebootstrap required;
   - nunca auto-login.

Não provocar FLOOD_WAIT real propositalmente; parsing/policy pode ser testado com erro simulado.

Media/file-reference refresh espera o slice de mídia.

## 19. Decisões ainda do mantenedor

A investigação não decide automaticamente:

- aceitar/rejeitar/substituir ADR 023;
- nome definitivo da authorization identity/config identity;
- habilitar PFS;
- bootstrap no mesmo binário/CLI ou operação separada;
- política MCP para FLOOD_WAIT;
- plataforma oficialmente suportada pelo primeiro backend de sessão;
- persistência de peer cache antes do live collector;
- política após `differenceTooLong`.

Todas continuam Decision separada quando necessárias.

## 20. Recomendação para L3-002

A recomendação técnica consolidada é:

> L3-002 deve construir um único owner in-process da autorização Telegram e do `gotd.Client`, com storage de credencial privado e bootstrap explícito, provar restore/reuse real e só então expor uma capability read-only `ResolvePeer + History` com tipos source-aware e erros semânticos. MTProto mecânico permanece no gotd. pts/qts/seq/date, raw update admission e recovery entram no Limiar somente na etapa do collector durável, usando os guards exigidos pelos ADRs 016–018.

Este documento atualiza/refina a Proposal de L3-001; não é um ADR Accepted.

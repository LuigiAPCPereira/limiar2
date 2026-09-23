# L3-001A — brief de investigação MTProto + gotd

**Estado:** investigação concluída; resultado em [L3_001A_MTPROTO_GOTD_INVESTIGATION.md](L3_001A_MTPROTO_GOTD_INVESTIGATION.md). Continua Evidence/Proposal, não Decision.  
**Tipo:** pesquisa técnica / Evidence usada para validar e corrigir a Proposal de L3-001.  
**Não autoriza implementação.**

## Objetivo

Entender em profundidade suficiente o MTProto e a versão efetivamente fixada de github.com/gotd/td para definir corretamente o menor boundary Telegram de L3-002, sem reimplementar MTProto, esconder semânticas essenciais ou acoplar MCP/collector ao SDK.

A investigação deve verificar, e não apenas confirmar, a recomendação registrada em L3_001_SESSION_BOUNDARY_INVESTIGATION.md.

## Fontes obrigatórias

1. mesma ref de trabalho: AGENTS.md, protocolo v2, START_HERE e docs Limiar 3;
2. go.mod/go.sum para identificar a versão real do gotd;
3. documentação oficial Telegram/MTProto;
4. código/documentação/changelog oficiais do gotd correspondente à versão usada;
5. issues/PRs upstream somente quando necessárias para esclarecer comportamento;
6. documentação MCP apenas quando afetar o boundary realtime.

Distinguir sempre fonte do projeto, fonte upstream, inferência arquitetural e hipótese ainda não verificada.

## Perguntas de MTProto

Investigar, na medida relevante ao Limiar:

- camadas: transport -> MTProto -> Telegram API -> RPC -> updates;
- auth key, auth_key_id, user authorization, 2FA, logout/revocation;
- DC topology, migration, export/import authorization quando aplicável;
- msg_id, seq_no, salts, containers, ACKs, retries, duplicate handling e time sync;
- reconnect e invalid authorization;
- updates: pts, pts_count, qts, seq, date, channel state, gaps, getState/getDifference/getChannelDifference;
- diferenças entre live update recovery e historical backfill;
- peers, InputPeer, access hashes, resolução e lifecycle de cache;
- histórico/paginação/edits/deletes/service messages/grouped media;
- mídia: file locations/references, DC, chunks, refresh/revalidation e CDN quando aplicável;
- FLOOD_WAIT/rate limits, migration errors, transient/permanent errors e cancellation;
- concorrência: chamadas simultâneas, múltiplas conexões, múltiplos clients e mesma authorization.

Para cada mecanismo, indicar o que o Limiar precisa conhecer e o que deve permanecer responsabilidade do gotd.

## Perguntas de gotd

Na versão real do projeto:

- lifecycle de telegram.Client, Client.Run, conexão, shutdown e reconnect;
- session.Storage, ErrNotFound, StorageMemory/FileStorage, quando Load/Store são chamados e como erros propagam;
- auth flow e reuse de authorization;
- tg.Client e tipos gerados: quais podem ficar internos e onde conversão seria artificial;
- updates.Manager: state storage, ordering, recovery, callbacks, persistência, restart e erros;
- responder explicitamente se é possível satisfazer Evidence antes de state advancement e em qual adaptation point;
- dispatcher/handlers: ordem, concorrência, backpressure, cancellation e erro do handler;
- peer facilities/cache/resolution;
- história paginada e resolve peer;
- downloader/media/file reference handling;
- retries, flood wait, reconnect, migration e middlewares;
- logging/observability e risco de vazamento;
- thread-safety/concurrency do client/API/downloader/updates manager/session storage.

## Mapa obrigatório

Produzir tabela:

| Conceito MTProto/Telegram | Implementação gotd | Aparece no contract do Limiar? | Owner no Limiar | Evidência/observação |
| --- | --- | --- | --- | --- |

Cobrir ao menos auth key, session blob, DC, peer, access hash, message ID, pts/qts/seq/date, updates/difference, historical query, file reference/media location, flood wait, reconnect e logout/revocation.

## Validar a Proposal de L3-001

Para cada hipótese abaixo, classificar tecnicamente como SUSTENTADA, SUSTENTADA COM AJUSTE, NÃO SUSTENTADA ou AINDA DESCONHECIDA:

1. um TelegramRuntime por session identity;
2. gotd.Client possuído exclusivamente por esse runtime;
3. SessionStore privado ao boundary Telegram;
4. MCP realtime e collector compartilhando runtime/capabilities;
5. nenhum gotd.Client/tg/session bytes cruzando o boundary;
6. MCP realtime read-only direto ao Telegram;
7. collector com history separado de updates/recovery;
8. peer state separado de session state;
9. Evidence separado de session state;
10. Source Admission antes do advancement durável do collector;
11. runtime inicialmente single-process;
12. separação física futura sem reescrever consumers.

## Contracts mínimos

Recomendar o menor conjunto de capabilities para L3-002.

Evitar interface por método, espelho de tg.Client, abstrações genéricas de provider, types comerciais antes do corpus e reimplementação da Telegram API.

Registrar também quais contracts não devem existir ainda.

## Impacto no plano bottom-up

Verificar se a sequência continua tecnicamente correta:

runtime/config -> session -> TelegramRuntime -> primeira capability read-only -> gotd real + restart -> MCP realtime -> Source Admission/Evidence -> live updates/recovery -> backfill -> media -> exploração do corpus -> Deterministic Findings -> modelo comercial -> Query Service -> MCP data/API -> frontend

Qualquer alteração deve citar a dependência concreta de MTProto/gotd que a exige.

## Riscos e experimentos

Separar riscos em:

- bloqueia L3-002;
- pode esperar;
- exige experimento real.

Propor somente experimentos que reduzam incerteza arquitetural material, informando hipótese, setup, observação, critério e o que o resultado não prova.

## Entrega

O relatório deve responder:

> Qual é o menor boundary Telegram correto para L3-002, considerando o comportamento real de MTProto e gotd, e quais partes da Proposal de L3-001 precisam ser preservadas, ajustadas ou descartadas?

Ao final, não implementar código nem promover ADRs. Atualizar tracker/checkpoint somente se a execução tiver autorização de escrita.


## Fechamento

A investigação concluiu que a direção central de L3-001 é sustentada com ajustes: owner por `TelegramAuthorizationIdentity` (não MTProto `session_id`), bootstrap explícito/fail-closed, peer state separado porém authorization-scoped, e Source Admission/recovery/state guards externos ao `updates.Manager`. O smoke real gotd restart/reuse passa a preceder a primeira capability estável. Nenhum ADR foi promovido e L3-002 continua dependente de decisões/gates próprios.

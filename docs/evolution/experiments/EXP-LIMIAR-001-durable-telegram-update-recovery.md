# EXP-LIMIAR-001 — Recuperação durável de updates Telegram

Authority: Non-authoritative
Status: Supported

## Hipótese

É possível integrar o recovery/ordering do gotd com um boundary de durabilidade do
Limiar de forma que o sync state persistido não certifique uma observação, bootstrap ou
descontinuidade cuja Evidence exigida ainda não esteja durável.

Estado proibido:

```text
Evidence exigida não durável / sync state persistido como avançado
```

A hipótese não exige exactly-once.

## Contexto

A arqueologia do ingress encontrou perda sob backpressure, avanço prematuro de cursor,
colisão entre revisões, ausência de Evidence para deletes e uso de `LastMessageID` como
uma autoridade que o protocolo Telegram não define.

Telegram usa `pts/qts/seq/date` e estado de canais, com recovery por
`updates.getDifference`/`updates.getChannelDifference`.

Versão pinada examinada: `github.com/gotd/td v0.161.0`.

## Fontes primárias

- Telegram — Working with Updates: https://core.telegram.org/api/updates
- Telegram — `updates.getDifference`: https://core.telegram.org/method/updates.getDifference
- gotd/td v0.161.0 — `telegram/updates/manager.go`
- gotd/td v0.161.0 — `telegram/updates/state.go`
- gotd/td v0.161.0 — `telegram/updates/state_apply.go`
- gotd/td v0.161.0 — `telegram/updates/state_channel.go`
- gotd/td v0.161.0 — `telegram/updates/sequence_box.go`
- gotd/td v0.161.0 — `telegram/updates/storage.go`

## Fase A — protótipo abstrato da barrier

O protótipo descartável inicial modelou:

```text
DurableEvidenceHandler
        ↓
Evidence Store

GuardedStateStorage
        ↑
DurabilityBarrier
```

A primeira versão possuía TOCTOU entre checar a barrier e escrever state. A versão
corrigida mantém verificação + state write na mesma seção crítica.

Resultado: **SUPPORTED** para a propriedade local de impedir
`Evidence não durável / state avançado`.

## Fase B — inspeção do gotd v0.161.0

A leitura do source pinado invalidou a suposição de que erro de handler/storage encerraria
necessariamente o manager.

Também confirmou que:

- `UpdatesDifferenceTooLong` avança PTS antes de `OnTooLong`;
- `UpdatesChannelDifferenceTooLong` avança channel PTS antes de `OnChannelTooLong`;
- ausência de state local leva à adoção de state remoto via `UpdatesGetState`;
- updates reconstruídos por difference podem chegar stateless ao handler.

Finding detalhado:
`docs/evolution/findings/F-ING-008-gotd-recovery-fail-stop-boundary.md`.

O candidato simples foi rejeitado. O Candidate v3 passou a ser:

```text
Telegram RPC
    ↓
GuardedRecoveryAPI
    ↓
updates.Manager
    ↓
DurableEvidenceHandler
    ↓
Evidence Store

updates.Manager
    ↓
GuardedStateStorage

DurabilityBarrier + Supervisor
```

## Fase C — contract tests de fail-stop/recovery

A PR `limiar-collector#151` preparou 12 contratos cobrindo:

1. Evidence durável -> PTS pode avançar;
2. falha de Evidence -> PTS persistido antigo;
3. falha de state write -> barrier fecha;
4. barrier -> supervisor cancela Manager;
5. common gap -> segundo `getDifference` após startup;
6. falha durante `getDifference` -> state antigo;
7. callback `DifferenceTooLong` é tardio;
8. guard bloqueia common/channel TooLong se Evidence falha;
9. bootstrap remoto não é adotado se sua Evidence falha;
10. `UpdateChannelTooLong` -> novo `getChannelDifference`;
11. falha -> restart -> replay do state persistido antigo;
12. replay de difference chega stateless (`Pts=-1`).

A PR `limiar-collector#155` acrescentou três gates:

13. `ChannelDifferenceTooLong` com Dialog/PTS válido tem Evidence antes do channel state;
14. resync explícito (`Forget=true`) tem Evidence antes da substituição do baseline;
15. erro de leitura de state não cai em bootstrap remoto.

Como o Actions do Limiar falhava antes de qualquer step, a suíte foi executada em módulos
descartáveis no `agent-runtime`, exclusivamente para usar o runner self-hosted existente.

Ambiente confirmado:

```text
Runner: LuigiCachyOS / actions-runner 2.336.0
Go: 1.26.6 linux/amd64
gotd/td: v0.161.0
```

Os 15 contratos e `go test -race` passaram.

## Fase D — preservação do source envelope

A revisão do gate de edit/delete/update composto encontrou uma premissa adicional: o
handler configurado no `updates.Manager` não recebe necessariamente o envelope bruto que
o Telegram entregou.

O source do gotd mostra que `handleUpdates` converte envelopes e que `applyCombined`:

- ordena updates por PTS;
- roteia PTS/channel/QTS por caminhos próprios;
- pode separar um único envelope em múltiplos dispatches;
- reconstrói novos `tg.Updates` antes do handler.

O contract test
`TestADR017Gate_PostManagerHandlerDoesNotPreserveSourceEnvelope` enviou um único envelope
com dois `UpdateDeleteMessages` em PTS `[2, 1]` e `Date=123456`.

No handler pós-Manager foram observados dois batches separados, reordenados para `[1]` e
`[2]`, ambos sem o `Date` original.

Execução:

```text
Workflow run: agent-runtime#32188101306
TestADR017Gate_PostManagerHandlerDoesNotPreserveSourceEnvelope PASS
go test -race ./... -count=1                                 PASS
```

Finding detalhado:
`docs/evolution/findings/F-ING-009-post-manager-handler-is-not-source-envelope.md`.

Consequência: o Candidate v3 continua **SUPPORTED** para fail-stop, ordering de state e
recovery já testados, mas é **INSUFICIENTE como boundary completo de Source Evidence** se
o `DurableEvidenceHandler` pós-Manager for a única captura do envelope bruto.

## Fase E — Source Admission v4

A hipótese seguinte moveu a admissão da Evidence live para antes do manager e ampliou o
wrapper de recovery para preservar respostas semanticamente relevantes antes de
entregá-las ao manager:

```text
Live:
Telegram UpdateHandler
    ↓
DurableSourceAdmission
    ↓
updates.Manager
    ↓
OrderedUpdateHandler / trabalho derivado

Recovery:
Telegram recovery RPC
    ↓
SourcePreservingRecoveryAPI
    ↓
updates.Manager

updates.Manager
    ↓
GuardedStateStorage

DurabilityBarrier + Supervisor atravessam admission / recovery / state.
```

O protótipo usa JSON apenas para provar preservação/ordem. **JSON não é decisão de codec
ou schema físico.**

Contratos adicionais validados:

16. envelope live composto contendo edit + delete é persistido integralmente antes dos
    PTS correspondentes;
17. falha da Source Admission impede forwarding ao manager e não produz PTS;
18. resposta comum de `getDifference` com conteúdo é persistida antes de `SetState`;
19. falha na persistência de recovery impede a resposta de chegar ao manager;
20. `getChannelDifference` é persistido antes de channel PTS;
21. o contrato permanece válido para `UpdatesChannelDifference` com conteúdo real;
22. com Source Evidence já durável, falha do handler pós-Manager não fecha a source
    durability barrier nem precisa impedir o avanço de sync state;
23. o finding da Fase D continua coberto para impedir regressão da premissa de envelope
    transparente.

Durante o desenvolvimento do harness houve duas classes de falso negativo, ambas
corrigidas antes da conclusão:

- efeitos residuais do recovery de startup foram inicialmente confundidos com writes do
  caso sob teste;
- o teste inicialmente observava o state intermediário após o primeiro de dois `SetPts`.

A versão final sincroniza explicitamente os eventos relevantes e não depende da velocidade
do scheduler.

Execução final:

```text
Workflow run: agent-runtime#32189541388
Go: 1.26.6 linux/amd64
gotd/td: v0.161.0
23 contract tests                                     PASS
go test -race ./... -count=1                         PASS
```

Resultado: **SUPPORTED** para a topologia v4 no escopo do contract harness.

A consequência semântica é importante: Source Evidence durable e saída ordenada/derivada
são responsabilidades diferentes. Uma falha reconstruível de projeção não deve ser
confundida automaticamente com falha de admissão da fonte.

Proposal derivada:
`docs/evolution/proposals/PROPOSAL-ING-003-source-admission-before-manager.md`.

## O que o experimento NÃO prova

- exactly-once;
- recuperação de eventos que o Telegram já não oferece;
- ausência de chamadas externas duplicadas após crash;
- schema físico de Evidence;
- engine/PRAGMAs de storage;
- session storage;
- policy final de retries/supervisor;
- codec final para Evidence/recovery;
- política final para respostas de Difference vazias sem observação de fonte;
- correção da integração completa antes de ela existir no Limiar;
- autorização para mudança de produção.

A garantia pretendida permanece **at-least-once observation dentro do boundary
controlado + replay + downstream idempotente**, com descontinuidades explícitas quando a
fonte não consegue mais recuperar o passado.

## Pendências antes de produção

1. decidir por nova Decision a semântica de Source Admission v4, pois ela altera
   materialmente o papel do handler pós-Manager descrito no ADR 017;
2. provar coexistência backfill/live com authorities de progresso separadas;
3. integrar o storage escolhido preservando Evidence -> state ordering;
4. definir schema/codec/versionamento de Evidence;
5. preservar os contract tests na implementação real do adapter;
6. validar crash/restart na integração real, não apenas nos fakes de contrato.

## Conclusão epistemológica

A hipótese de **fail-stop + state ordering** sobreviveu ao protótipo, à inspeção do gotd
pinado e aos contract tests com race detector.

A Fase D rejeitou a hipótese de que o Candidate v3 preservaria sozinho a **Source Evidence
bruta**. A Fase E encontrou e validou uma correção: preservar o envelope live antes do
manager e preservar respostas de recovery antes de sua entrega ao manager, mantendo o
gotd responsável pelo ordering/recovery.

O Candidate v4 está **SUPPORTED**, mas continua não autoritativo até uma Decision explícita.

# EXP-LIMIAR-001 — Recuperação durável de updates Telegram

Authority: Non-authoritative
Status: Supported with limitations; runtime contract inconclusive

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
corrigida mantém verificação + state write na mesma seção crítica:

```go
GuardStateWrite(func() error {
    return underlyingStateStorage.Write(...)
})
```

O protótipo abstrato passou testes concorrentes com race detector.

### Resultado da Fase A

**Supported** para a propriedade local:

| Evidence | state persistido | Resultado |
| --- | --- | --- |
| não | não | falha segura |
| sim | não | replay seguro |
| sim | sim | normal |
| não | sim | proibido |

Isso provou a viabilidade de uma barrier linearizável; não provou o contrato real do
`updates.Manager`.

## Fase B — inspeção do gotd v0.161.0

A leitura do source pinado invalidou uma suposição importante do protótipo:

```text
handler/state storage retorna erro
!=
manager necessariamente para
```

Erros de `dispatch` e vários erros de `StateStorage` podem ser apenas logados.

Também foi confirmado que:

- `UpdatesDifferenceTooLong` persiste/avança PTS antes de `OnTooLong`;
- `UpdatesChannelDifferenceTooLong` persiste/avança channel PTS antes de
  `OnChannelTooLong`;
- ausência de state local leva `Manager.loadState` a adotar state remoto via
  `UpdatesGetState`;
- mensagens reconstruídas por difference podem chegar ao handler com PTS/QTS negativos
  e esses valores não devem ser usados como autoridade de sync.

Finding detalhado:
`docs/evolution/findings/F-ING-008-gotd-recovery-fail-stop-boundary.md`.

### Consequência

O candidato simples foi rejeitado. O Candidate v3 experimental passa a ser:

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
atravessam os três boundaries.
```

`GuardedRecoveryAPI` existe para tornar bootstrap e `*DifferenceTooLong` observáveis como
Evidence antes que a resposta seja entregue ao manager.

## Fase C — harness real da PR #151

Foi preparado um harness contra a API pública de `telegram/updates` v0.161.0 cobrindo:

1. Evidence durável -> PTS pode avançar;
2. falha de Evidence -> PTS persistido permanece antigo;
3. falha de state write -> barrier fecha;
4. barrier -> supervisor cancela o Manager;
5. common gap -> segunda `getDifference` após drenar startup;
6. falha de Evidence durante `getDifference` -> state antigo;
7. prova de que callback `DifferenceTooLong` é tardio;
8. `GuardedRecoveryAPI` bloqueia common/channel too-long se Evidence falha;
9. bootstrap remoto não é adotado se sua Evidence falha;
10. `UpdateChannelTooLong` -> novo `getChannelDifference` após startup;
11. falha -> restart -> replay a partir do state persistido antigo;
12. replay de difference chega stateless (`Pts=-1`).

### Execução

A execução real permanece **INCONCLUSIVE**.

As GitHub Actions da PR #151 terminam antes de qualquer step/log/artifact. O mesmo
comportamento já ocorreu na PR #150, anterior ao harness, portanto não é evidência de
falha dos testes experimentais.

No ambiente local desta sessão não há toolchain/dependências adequados para executar a
versão pinada com race detector.

Estado atual:

```text
barrier abstrata: SUPPORTED
source contract v0.161.0: CONFIRMED
harness: PREPARED
runtime contract: INCONCLUSIVE
```

## O que o experimento NÃO prova

- exactly-once;
- recuperação de eventos que o Telegram já não oferece;
- ausência de chamadas externas duplicadas após crash;
- correção runtime do Candidate v3 enquanto o harness não executar;
- schema físico de Evidence;
- engine/PRAGMAs de storage;
- session storage;
- policy final de retries/supervisor;
- autorização para mudança de produção.

A garantia pretendida permanece **at-least-once observation dentro do boundary
controlado + replay + downstream idempotente**, com descontinuidades explícitas quando a
fonte não consegue mais recuperar o passado.

## Próxima validação necessária

Antes de fixar a mecânica concreta em produção:

1. executar a suíte da PR #151 com Go atual + `-race`;
2. validar runtime de `ChannelDifferenceTooLong` com diálogo/PTS válido;
3. definir schema das Evidence de bootstrap/descontinuidade junto ao storage;
4. definir política de restart/retry do supervisor após barrier fechada.

## Conclusão epistemológica

A hipótese de **barrier explícita** continua suportada, mas a composição concreta mudou
após a inspeção do gotd.

`Supported` não significa `Accepted`: o Candidate v3 continua não-autoritativo até os
contract tests reais passarem e uma Decision apropriada o autorizar.

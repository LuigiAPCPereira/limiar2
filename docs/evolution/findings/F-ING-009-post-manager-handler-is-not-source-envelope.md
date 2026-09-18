# F-ING-009 — Handler pós-Manager não preserva o envelope de fonte

Authority: Non-authoritative
Status: Confirmed

## Finding

No `github.com/gotd/td v0.161.0`, o `updates.Manager` não funciona como transporte
transparente do `tg.UpdatesClass` recebido até o `UpdateHandler` configurado.

Antes de chamar o handler, o manager pode:

- converter `tg.Updates` em `tg.UpdatesCombined`;
- ordenar membros por PTS;
- encaminhar updates de canal para workers próprios;
- separar PTS/QTS/channel updates de outros updates;
- reconstruir novos `tg.Updates` para dispatch;
- omitir metadata do envelope original, como `Date`/`Seq`, nesses batches reconstruídos.

Portanto, um `DurableEvidenceHandler` colocado somente **depois** do
`updates.Manager` não consegue, sozinho, ser o boundary de preservação da Evidence bruta
recebida da fonte.

## Evidência de source

Em `telegram/updates/state.go`, `handleUpdates` converte `tg.Updates` para
`tg.UpdatesCombined` antes do processamento.

Em `telegram/updates/state_apply.go`, `applyCombined` chama `sortUpdatesByPts`, roteia
PTS/channel/QTS separadamente e `applyPts`/`applyQts` constroem novos `tg.Updates` para o
handler.

Versão examinada: `gotd/td v0.161.0`.

## Evidência runtime

O contract test
`TestADR017Gate_PostManagerHandlerDoesNotPreserveSourceEnvelope` enviou um único envelope
live contendo dois `UpdateDeleteMessages` deliberadamente em PTS `[2, 1]`, com
`Date=123456`.

Resultado observado no handler pós-Manager:

- duas chamadas separadas;
- PTS reordenado para `[1]`, depois `[2]`;
- `Date=0` nos batches reconstruídos;
- o PTS persistido avançou normalmente para `2`.

Ambiente:

```text
Runner: LuigiCachyOS / actions-runner 2.336.0
Go: 1.26.6 linux/amd64
gotd/td: v0.161.0
Workflow run: agent-runtime#32188101306
```

Validação:

```text
TestADR017Gate_PostManagerHandlerDoesNotPreserveSourceEnvelope PASS
go test -race ./... -count=1                                 PASS
```

## Impacto

O ADR 016 exige que Evidence represente a observação da fonte e preserve adequadamente
updates compostos, edits, deletes, replay e snapshots históricos.

O ADR 017 acertou o boundary de fail-stop/state ordering, mas seu diagrama Candidate
`updates.Manager -> DurableEvidenceHandler -> Evidence Store` é insuficiente se esse
handler for interpretado como a **única** captura da Evidence de fonte.

Isso não invalida:

- `updates.Manager` como autoridade especializada de ordering/recovery;
- `GuardedStateStorage`;
- `DurabilityBarrier`;
- `Supervisor`;
- a necessidade de interceptar bootstrap/resync/TooLong antes da adoção de state.

Ele invalida a suposição de que a captura pós-Manager, sozinha, preserva o source envelope.

## Hipótese de continuação

A próxima experiência deve avaliar um boundary em que a observação bruta seja admitida
de forma durável **antes** de ser entregue ao `updates.Manager`, enquanto respostas de
recovery sejam preservadas no `GuardedRecoveryAPI` antes de retornar ao manager.

O handler pós-Manager passaria a representar saída ordenada/derivada, não a única
Evidence bruta da fonte.

Essa hipótese ainda não é Decision e pode exigir um ADR complementar/superseding se os
contract tests confirmarem que ela altera materialmente o ADR 017.

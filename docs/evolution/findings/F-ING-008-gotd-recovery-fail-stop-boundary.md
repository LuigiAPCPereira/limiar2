# F-ING-008 — O `updates.Manager` exige fail-stop externo para preservar durabilidade

Authority: Non-authoritative
Status: Confirmed by source; runtime execution blocked

## Finding

A integração direta

```text
updates.Manager
  -> DurableEvidenceHandler
  -> StateStorage
```

não é suficiente para garantir C-03 apenas por propagação de erros.

Na versão `github.com/gotd/td v0.161.0`, o manager registra em log vários erros do
handler e do `StateStorage` e pode continuar o fluxo. Além disso, alguns caminhos de
recovery avançam state antes de callbacks que informam perda de continuidade.

Logo, o candidato concreto precisa de fail-stop externo e de um adapter que observe
respostas especiais de recovery antes que elas sejam entregues ao manager.

## Evidence de código — gotd v0.161.0

Fontes primárias examinadas:

- `telegram/updates/manager.go`;
- `telegram/updates/state.go`;
- `telegram/updates/state_apply.go`;
- `telegram/updates/state_channel.go`;
- `telegram/updates/sequence_box.go`;
- `telegram/updates/storage.go`;
- `telegram/updates/utils.go`.

### Handler error não é fail-stop

Nos caminhos de PTS/QTS e updates combinados, erros de `dispatch` podem ser apenas
registrados em log. Assim:

```text
handler retorna erro
!=
manager interrompe processamento
```

A durabilidade não pode depender somente do erro retornado pelo `UpdateHandler`.

### Erro de `StateStorage` pode ser absorvido

`SetPts`, `SetQts`, `SetSeq`, `SetDateSeq`, `SetState` e `SetChannelPts` possuem caminhos
em que o erro é logado e o fluxo continua.

`sequenceBox` só evita atualizar seu state interno quando `apply` devolve erro; como os
`apply*` relevantes absorvem parte desses erros, state em memória e state persistido
podem divergir temporariamente.

Se a Evidence já está durável, state persistido antigo é aceitável porque permite replay.
O que não é aceitável é state persistido novo após falha de Evidence.

### `DifferenceTooLong` ocorre antes do callback

No common recovery, `UpdatesDifferenceTooLong` faz o manager:

1. `SetPts(remotePts)`;
2. atualiza o PTS em memória;
3. chama `OnTooLong`;
4. continua o recovery.

Portanto, `OnTooLong` sozinho é tarde demais para garantir Evidence de descontinuidade
antes do avanço do state.

### `ChannelDifferenceTooLong` possui a mesma ordem

No recovery de canal, `UpdatesChannelDifferenceTooLong` extrai o PTS remoto do diálogo,
persiste `SetChannelPts`, atualiza o state interno e só então chama `OnChannelTooLong`.

O callback também não é boundary suficiente.

### Bootstrap remoto precisa ser explícito

Quando não existe state local, `Manager.loadState` chama `UpdatesGetState` e persiste o
state remoto recebido.

Esse state é um novo baseline operacional. Ele não prova que o Limiar observou todo o
passado anterior a esse baseline; portanto a adoção precisa ser representada por Evidence
explícita antes de `SetState`.

### Recovery produz updates stateless

Mensagens reconstruídas por `getDifference`/`getChannelDifference` podem ser entregues ao
handler com PTS/QTS negativos. O próprio gotd documenta que handlers não devem usar esses
valores como posição de sincronização.

Consequência para o Limiar: Evidence de mensagem e autoridade de sync precisam continuar
separadas. O state pertence ao recovery manager/StateStorage, não ao payload derivado
entregue ao handler.

## Candidato revisado — v3

```text
Telegram RPC
    |
    v
GuardedRecoveryAPI
    |
    v
updates.Manager
    |
    v
DurableEvidenceHandler
    |
    v
Evidence Store

updates.Manager
    |
    v
GuardedStateStorage

DurabilityBarrier + Supervisor
atravessam Handler / API / StateStorage
```

### `DurableEvidenceHandler`

Persiste observações normais antes de permitir o avanço correspondente do state.

Se a Evidence falhar:

- fecha a barrier antes de retornar;
- sinaliza o supervisor;
- retorna erro apenas para observabilidade, sem depender dele para parar o gotd.

### `GuardedStateStorage`

Mantém a verificação da barrier e a escrita física do state na mesma seção crítica,
impedindo TOCTOU.

Estado permitido:

```text
Evidence durável / state antigo   -> replay aceitável
Evidence durável / state novo     -> normal
Evidence não durável / state antigo -> falha segura
Evidence não durável / state novo -> proibido
```

### `GuardedRecoveryAPI`

Intercepta respostas especiais antes do `updates.Manager`:

- `UpdatesGetState` -> Evidence de bootstrap/adopção de baseline remoto;
- `UpdatesDifferenceTooLong` -> Evidence de descontinuidade common;
- `UpdatesChannelDifferenceTooLong` -> Evidence de descontinuidade de canal.

Se a Evidence correspondente falhar, a resposta especial não é devolvida ao manager, a
barrier fecha e o supervisor encerra o lifecycle.

A Evidence real deverá carregar contexto suficiente para auditoria/reprocessamento; os
strings usados no harness são apenas marcadores de contrato, não schema proposto.

## Garantia pretendida

O desenho não promete recuperar eventos que o Telegram já declarou fora da janela de
recovery.

Ele promete algo mais preciso:

> o Limiar não avança silenciosamente sua autoridade operacional sobre uma observação,
> bootstrap ou descontinuidade que ainda não foi representada por Evidence durável no
> boundary que controla.

## Harness da PR #151

O harness de teste agora possui casos preparados para:

| Contrato | Cobertura escrita | Execução real |
| --- | --- | --- |
| Evidence durável -> PTS pode avançar | sim | pendente |
| falha de Evidence -> PTS persistido não avança | sim | pendente |
| falha do StateStorage fecha barrier | sim | pendente |
| barrier -> supervisor cancela Manager | sim | pendente |
| common gap -> segundo `getDifference` após startup | sim | pendente |
| falha durante `getDifference` deixa state antigo | sim | pendente |
| callback `DifferenceTooLong` ocorre tarde | sim | pendente |
| `GuardedRecoveryAPI` bloqueia common too-long quando Evidence falha | sim | pendente |
| `GuardedRecoveryAPI` bloqueia channel too-long quando Evidence falha | sim | pendente |
| bootstrap sem state local não é adotado se Evidence falha | sim | pendente |
| `UpdateChannelTooLong` provoca novo `getChannelDifference` após startup | sim | pendente |
| falha -> restart -> replay a partir do state persistido antigo | sim | pendente |
| replay de `getDifference` chega stateless (`Pts=-1`) | sim | pendente |

O teste inicial de common gap que podia confundir o `getDifference` de startup foi
superado por um caso mais forte que primeiro drena explicitamente a chamada de startup e
só então exige uma segunda chamada após introduzir o gap. O caso antigo permanece apenas
como cobertura fraca redundante nesta branch experimental e não deve ser usado como
Evidence isolada.

## Bloqueio de execução

As execuções da GitHub Action desta PR encerram o job antes de qualquer step, sem logs ou
artifacts de teste. O ambiente local desta sessão também não possui o toolchain/deps
necessários para executar `gotd/td v0.161.0` com race detector.

Portanto:

```text
source contract: CONFIRMED
harness design: PREPARED
runtime contract: INCONCLUSIVE
```

Nenhum teste deve ser tratado como PASS apenas porque foi escrito.

## Pendências antes de produção

1. executar o harness com Go atual e `-race` em ambiente que realmente inicie os steps;
2. remover a cobertura fraca redundante do common gap depois que a execução do caso forte
   estiver disponível;
3. validar runtime de `ChannelDifferenceTooLong` com diálogo/PTS válido, além da
   interceptação já coberta pelo adapter;
4. decidir o schema real das Evidence de bootstrap/descontinuidade no ADR de storage;
5. decidir lifecycle/retry do supervisor quando a barrier fecha.

## Conclusão epistemológica

**Finding confirmado por inspeção do source pinado.**

O Candidate v3 é mais forte que o desenho anterior e agora possui uma suíte de contratos
preparada, mas continua **não-autoritativo** até a execução real dos testes pendentes.

# F-ING-008 — O `updates.Manager` exige fail-stop externo para preservar durabilidade

Authority: Non-authoritative
Status: Confirmed by source and runtime contract

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
handler com PTS/QTS negativos. Esses valores não devem ser usados como autoridade de
sync.

Consequência para o Limiar: Evidence de mensagem e autoridade de sync permanecem
separadas. O state pertence ao recovery manager/StateStorage, não ao payload reconstruído
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
- retorna erro para observabilidade, sem depender dele para parar o gotd.

### `GuardedStateStorage`

Mantém a verificação da barrier e a escrita física do state na mesma seção crítica,
impedindo TOCTOU.

Estados admitidos:

```text
Evidence durável / state antigo       -> replay aceitável
Evidence durável / state novo         -> normal
Evidence não durável / state antigo   -> falha segura
Evidence não durável / state novo     -> proibido
```

### `GuardedRecoveryAPI`

Intercepta respostas especiais antes do `updates.Manager`:

- `UpdatesGetState` -> Evidence de bootstrap/adoção de baseline remoto;
- `UpdatesDifferenceTooLong` -> Evidence de descontinuidade common;
- `UpdatesChannelDifferenceTooLong` -> Evidence de descontinuidade de canal.

Se a Evidence correspondente falhar, a resposta especial não é devolvida ao manager, a
barrier fecha e o supervisor encerra o lifecycle.

Os strings usados no harness são apenas marcadores de contrato; não são schema de
produção.

## Garantia pretendida

O desenho não promete recuperar eventos que o Telegram já declarou fora da janela de
recovery.

Ele promete algo mais preciso:

> o Limiar não avança silenciosamente sua autoridade operacional sobre uma observação,
> bootstrap ou descontinuidade que ainda não foi representada por Evidence durável no
> boundary que controla.

## Contract tests executados

A suíte da PR experimental `limiar-collector#151` foi copiada sem dependência de código
de produção para um módulo descartável em `agent-runtime#14`, exclusivamente para usar o
runner self-hosted já configurado naquele repositório.

Ambiente observado:

```text
Runner: LuigiCachyOS / actions-runner 2.336.0
Go: 1.26.6 linux/amd64
gotd/td: v0.161.0
Workflow run: agent-runtime Actions 32171500698
```

Resultado:

| Contrato | Runtime |
| --- | --- |
| Evidence durável -> PTS pode avançar | PASS |
| falha de Evidence -> PTS persistido não avança | PASS |
| falha do StateStorage fecha barrier | PASS |
| barrier -> supervisor cancela Manager | PASS |
| common gap -> segunda `getDifference` após startup | PASS |
| falha durante `getDifference` deixa state antigo | PASS |
| callback `DifferenceTooLong` ocorre tarde | PASS |
| guard bloqueia common too-long se Evidence falha | PASS |
| guard bloqueia channel too-long se Evidence falha | PASS |
| bootstrap não é adotado se Evidence falha | PASS |
| `UpdateChannelTooLong` -> novo `getChannelDifference` | PASS |
| falha -> restart -> replay do state antigo + stateless PTS | PASS |

Comando funcional:

```text
go test ./... -count=1 -v
```

Resultado: `PASS`, 12 contratos, pacote em aproximadamente 0,53 s após compilação.

Race detector:

```text
go test -race ./... -count=1
```

Resultado: `PASS`.

O workflow do próprio Limiar continuou incapaz de alocar GitHub-hosted runner e a prova
self-hosted adicionada temporariamente ao Limiar ficou `queued`; isso é problema de
infraestrutura, não resultado do experimento.

## Estado epistemológico

```text
source contract: CONFIRMED
runtime contract: CONFIRMED para a suíte exercitada
Candidate v3: SUPPORTED
produção: NÃO AUTORIZADA por este Finding
```

`SUPPORTED` continua diferente de `Accepted`.

## Pendências antes de produção

1. definir o schema real das Evidence de bootstrap/descontinuidade no ADR de storage;
2. definir lifecycle/retry do supervisor quando a barrier fecha;
3. durante implementação, manter contract tests junto ao adapter real e validar o caminho
   de `ChannelDifferenceTooLong` com payload de diálogo/PTS realista;
4. validar a integração completa no módulo do Limiar, não apenas o boundary isolado.

## Conclusão epistemológica

O Candidate v3 sobreviveu à inspeção do source pinado e aos contract tests executados com
race detector.

Isso é Evidence suficiente para tratá-lo como **Candidate suportado**, mas não cria
Decision nem autoriza implementação de produção por si só.

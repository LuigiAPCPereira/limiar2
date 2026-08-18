# F-ING-008 — O `updates.Manager` exige fail-stop externo para preservar durabilidade

Authority: Non-authoritative
Status: Confirmed by source; runtime contract pending

## Finding

A integração direta

```text
updates.Manager
  -> DurableEvidenceHandler
  -> StateStorage
```

não é suficiente para garantir C-03 apenas por propagação de erros.

Na versão `github.com/gotd/td v0.161.0`, o manager registra em log vários erros do
handler e do `StateStorage` e continua o fluxo. Além disso, alguns caminhos de recovery
avançam o state antes de chamar callbacks de perda irrecuperável.

Logo, o candidato concreto precisa de um boundary externo de fail-stop e de um adapter
que observe respostas de recovery antes que elas sejam entregues ao manager.

## Evidence de código — gotd v0.161.0

Fontes primárias:

- `telegram/updates/state_apply.go`
- `telegram/updates/sequence_box.go`
- `telegram/updates/state.go`
- `telegram/updates/state_channel.go`
- `telegram/updates/manager.go`
- `telegram/updates/storage.go`

Tag examinada:

`v0.161.0`

### 1. Handler error não é boundary de stop

Nos caminhos de PTS/QTS e updates combinados, o retorno de `dispatch` é registrado em
log. O fluxo pode prosseguir para a escrita do state.

Portanto:

```text
handler retorna erro
!=
manager interrompe processamento
```

Uma implementação que dependa apenas do erro do `UpdateHandler` pode persistir progresso
posterior à falha.

### 2. Erro de `StateStorage` também pode ser absorvido

`SetPts`, `SetQts`, `SetSeq`, `SetDateSeq`, `SetState` e `SetChannelPts` possuem caminhos
em que o erro é apenas registrado.

`sequenceBox.Handle` só evita atualizar seu state em memória se a função `apply` retornar
erro. Como os `apply*` relevantes frequentemente absorvem os erros acima, o state em
memória pode avançar mesmo quando o state persistido permaneceu antigo.

Isso não é necessariamente perda de dados: se a Evidence já está durável, state antigo
significa replay possível. Porém, exige lifecycle explícito e invalida a hipótese de que
um erro do storage sozinho fará o manager parar.

### 3. Fail-stop precisa ser externo

Para uma falha de Evidence, a ordem necessária é:

```text
Evidence write falha
  -> fechar DurabilityBarrier
  -> impedir novos state writes
  -> cancelar/supervisionar updates.Manager
  -> reiniciar posteriormente a partir do state persistido antigo
```

O fechamento da barrier deve acontecer antes de o handler retornar.

`GuardedStateStorage` precisa manter a verificação da barrier e a escrita do state dentro
da mesma seção crítica para evitar TOCTOU.

### 4. `DifferenceTooLong` não pode ser tratado apenas por callback

No common recovery, `UpdatesDifferenceTooLong` faz o manager:

1. persistir o novo `Pts`;
2. atualizar o PTS em memória;
3. chamar `OnTooLong`;
4. continuar `getDifference`.

Logo, `OnTooLong` ocorre tarde demais para persistir uma Evidence de descontinuidade
**antes** do avanço do state.

### 5. `ChannelDifferenceTooLong` possui o mesmo problema

No recovery de canal, `UpdatesChannelDifferenceTooLong` extrai o PTS remoto do diálogo,
persiste `SetChannelPts`, atualiza o state em memória e somente depois chama
`OnChannelTooLong`.

Um callback não consegue, sozinho, garantir:

```text
SyncDiscontinuity Evidence durável
antes de
state avançado
```

### 6. Bootstrap remoto também precisa de semântica explícita

Quando não existe state local, `Manager.loadState` chama `UpdatesGetState` e persiste o
state remoto recebido.

Isso estabelece um novo baseline operacional, mas não prova que o Limiar observou todos
os eventos anteriores. Se esse caminho representar perda potencial de continuidade, ele
deve gerar uma Evidence explícita de bootstrap/resync antes de o state remoto ser
aceito.

## Candidato revisado — v3

O boundary que sobrevive à inspeção é:

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

Persiste observações normais antes de permitir que o fluxo alcance o state write.

Se a Evidence falhar:

- fecha a barrier;
- sinaliza o supervisor;
- retorna erro para observabilidade, sem depender desse erro para parar o gotd.

### `GuardedStateStorage`

Permite state write apenas enquanto a barrier permanece aberta durante toda a operação.

Se a Evidence falhou, nenhum state posterior pode ser persistido como avançado.

Uma falha do próprio state write depois de Evidence durável deixa o state persistido
antigo; replay é seguro. A política operacional pode optar por fail-stop também nesse
caso, mas isso é uma decisão de disponibilidade, não requisito para preservar Evidence.

### `GuardedRecoveryAPI`

Intercepta respostas especiais **antes** de retorná-las ao `updates.Manager`.

Antes de devolver `UpdatesDifferenceTooLong` ou `UpdatesChannelDifferenceTooLong`, deve
persistir uma Evidence explícita de descontinuidade/gap. Se essa persistência falhar, a
resposta não é entregue ao manager e o fluxo entra em fail-stop.

O mesmo princípio se aplica ao bootstrap via `UpdatesGetState` quando não há state local:
o Limiar precisa registrar que adotou um baseline remoto sem afirmar observação completa
do passado.

## Garantia resultante

O desenho não promete recuperar eventos que o Telegram já declarou fora da janela de
recovery.

Ele promete algo mais preciso:

> o Limiar não avança silenciosamente sua autoridade operacional sobre uma perda ou
> observação que ainda não foi representada por Evidence durável dentro do boundary que
> controla.

Assim, os casos irrecuperáveis tornam-se descontinuidades explícitas, não ausência
silenciosa de dados.

## Harness runtime

A PR experimental `#151` contém um harness contra a API pública de
`telegram/updates.Manager` v0.161.0.

A execução ainda está **INCONCLUSIVE**: GitHub Actions encerrou os jobs antes de qualquer
step/log, e o ambiente local da sessão possui Go 1.23.2 sem acesso de rede para baixar o
toolchain/dependências atuais.

Isso não invalida o Finding de source contract acima, mas impede afirmar que o Candidate
v3 já passou pelos contract tests reais.

## Pendências

Antes de promover a mecânica concreta para produção:

1. executar o harness real com Go atual + race detector;
2. corrigir/validar o teste de common gap sem confundir o `getDifference` de startup com
   o `getDifference` disparado pelo gap;
3. cobrir `getChannelDifference` normal e `ChannelDifferenceTooLong`;
4. cobrir restart/replay após falha de Evidence;
5. cobrir stateless updates;
6. testar `GuardedRecoveryAPI` para common/channel too-long e bootstrap sem state local.

## Conclusão epistemológica

**Finding confirmado por inspeção do source pinado.**

O candidato anterior não deve ser implementado literalmente. O Candidate v3 acima é
mais forte, mas continua não-autoritativo até os contract tests pendentes.

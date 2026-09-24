# ADR 017 — Boundary durável de recovery Telegram

Authority: Decision Record
Status: Accepted
Accepted-by: LuigiAPCPereira
Accepted-at: 2026-08-18T17:24:00-03:00
Acceptance-reference: PR #153

## Contexto

O ADR 016 aceitou os princípios de Evidence e sincronização Telegram, mas deixou a mecânica concreta de recovery fora de escopo.

O EXP-LIMIAR-001 e o F-ING-008 validaram `github.com/gotd/td v0.161.0` por inspeção e por 12 contract tests executados com Go 1.26.6 e race detector. Os testes confirmaram que erro de handler/storage não é, sozinho, uma fronteira confiável de parada; callbacks de `DifferenceTooLong` chegam depois de mudanças de state; bootstrap adota state remoto; e replay após restart é esperado.

A inspeção adicional de `Manager.loadState` confirmou duas propriedades relevantes:

- erro de leitura de `StateStorage.GetState` é propagado e não deve ser tratado como state ausente;
- `AuthOptions.Forget=true` força adoção de state remoto mesmo quando poderia existir state local.

Referências: ADR 016, PR #151, EXP-LIMIAR-001, F-ING-008 e PROPOSAL-ING-002.

## Decision

### 1. gotd continua responsável pelo ordering/recovery

O Limiar não reimplementa `pts/qts/seq`. `updates.Manager` continua responsável pelo recovery especializado do Telegram.

### 2. O recovery é envolto por quatro responsabilidades

```text
Telegram RPC -> GuardedRecoveryAPI -> updates.Manager -> DurableEvidenceHandler -> Evidence Store
                                      |
                                      -> GuardedStateStorage

DurabilityBarrier + Supervisor atravessam API / Handler / StateStorage.
```

Os nomes são ilustrativos; os contratos são obrigatórios.

### 3. Evidence precede a transição de state que ela autoriza

Para cada transição de sync state, toda Evidence exigida pelo contrato de admissão daquela transição deve estar durável antes de o state correspondente ser certificado como avançado.

Isso não exige uma Evidence por mensagem. Um update composto pode ser preservado como uma única observação de fonte e alimentar várias projeções derivadas.

Falha de Evidence fecha a barrier antes do retorno ao gotd, impede novos state writes e encerra o lifecycle do manager.

### 4. StateStorage é guardado de forma linearizável

Checagem da barrier e state write acontecem na mesma região crítica, sem TOCTOU. Falha de state write fecha a barrier e encerra o lifecycle, evitando continuar com state interno potencialmente divergente do persistido.

Erro de leitura de state não pode ser reinterpretado como ausência de state ou bootstrap. Ausência e falha são estados distintos.

### 5. Recovery API intercepta adoção de baseline e descontinuidades

Antes de entregar ao manager uma resposta que estabelece ou substitui baseline, ou representa perda de continuidade, o Limiar registra Evidence correspondente.

Casos mínimos:

- bootstrap por `UpdatesGetState` quando não existe state local;
- reset/resync explícito que substitua state local por state remoto, inclusive uso equivalente a `AuthOptions.Forget=true`;
- `UpdatesDifferenceTooLong`;
- `UpdatesChannelDifferenceTooLong`.

`Forget=true` não pode ser um default operacional casual; só pode ocorrer como ação explícita de resync/reset coberta por Evidence.

Se a Evidence exigida falhar, a resposta não chega ao manager.

### 6. TooLong é descontinuidade explícita

O Limiar pode adotar o novo baseline depois de registrar a descontinuidade, mas nunca afirma que observou os eventos que já não puderam ser recuperados.

Replay pode produzir mais de uma Evidence da mesma tentativa/descontinuidade; isso é reconciliado downstream, não eliminado por mutação do registro original.

### 7. Updates stateless não são autoridade de sync

PTS/QTS negativos ou sintéticos em envelopes reconstruídos não podem avançar `SourceSyncState`. A autoridade operacional permanece no recovery manager + StateStorage.

### 8. Barrier fechada é terminal para a instância

Uma `DurabilityBarrier` fechada nunca é reaberta. A instância atual do manager termina.

Um restart constitui nova tentativa/lifecycle, com novo contexto e nova barrier, partindo do state persistido durável. Política de backoff pode ser decidida separadamente, mas não pode existir loop apertado de restart que esconda falha persistente de storage/Evidence.

### 9. Replay é esperado

`Evidence durável + state persistido antigo` é condição segura. Restart pode reapresentar observações; downstream deve ser idempotente/reconciliável. Não há promessa de exactly-once.

### 10. A implementação pode variar sem mudar os contratos

Tipos, pacotes e nomes podem evoluir sem novo ADR se preservarem os contratos acima. Remover um boundary, alterar a semântica de interrupção, permitir state avançado sem Evidence exigida ou mudar a autoridade de sync exige nova Decision.

## Consequências

Benefícios: preserva os invariantes de durabilidade mesmo nos caminhos onde o gotd absorve erros; torna bootstrap/resync/perdas auditáveis; mantém recovery especializado; e explicita restart/replay.

Custos: ingress ganha adapters e lifecycle explícitos; falhas de persistência preferem indisponibilidade a continuidade ambígua; downstream precisa tolerar replay.

## Fora do escopo

Engine SQLite/PRAGMAs, schema físico de Evidence, payload, session/peer storage, política detalhada de backoff entre restarts, Source Message Projection, implementação de backfill e processamento comercial.

## Gates antes de produção

1. preservar os 12 contratos já executados;
2. passar `go test -race`;
3. adicionar teste end-to-end de `ChannelDifferenceTooLong` com diálogo/PTS válido;
4. testar `Forget=true`/resync explícito com Evidence antes da substituição do baseline;
5. provar que erro de leitura de state não cai em bootstrap;
6. cobrir edit/delete/update composto no boundary real;
7. provar coexistência backfill/live sem compartilhar autoridade de progresso;
8. usar storage capaz de cumprir a ordem Evidence/state.

## Relação com ADR 016

Este ADR complementa, não supersede, o ADR 016. ADR 016 define as propriedades de Evidence/sync; este ADR define o boundary de recovery do gotd que as preserva.

## Escopo da aceitação

A aceitação autoriza implementar essas responsabilidades quando os gates e as decisões de storage necessárias estiverem satisfeitos. Não aceita automaticamente engine de storage, schema físico de Evidence ou demais itens fora de escopo.

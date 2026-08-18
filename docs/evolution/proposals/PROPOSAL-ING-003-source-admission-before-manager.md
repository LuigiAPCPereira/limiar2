# PROPOSAL-ING-003 — Source Admission antes do updates.Manager

Authority: Non-authoritative
Status: Ready

## Problema

Os ADRs 016 e 017 exigem que Evidence de fonte seja preservada antes de o progresso de
sincronização certificar avanço. O Candidate v3 validou fail-stop, recovery e ordering de
state, mas o F-ING-009 demonstrou que `updates.Manager` não é um transporte transparente
do envelope recebido do Telegram: ele pode ordenar, separar e reconstruir batches antes
do handler configurado.

Consequentemente, um `DurableEvidenceHandler` colocado somente depois do manager não pode
ser a única autoridade de preservação do envelope bruto de fonte.

## Evidência

- ADR 016 — Source Evidence e sincronização Telegram;
- ADR 017 — Boundary durável de recovery Telegram;
- EXP-LIMIAR-001;
- F-ING-008;
- F-ING-009;
- PRs #151, #155 e #156;
- contract suite executada com Go 1.26.6 + gotd/td v0.161.0 no runner self-hosted.

A Fase E do EXP-LIMIAR-001 validou o Candidate v4 com 23 contract tests e race detector.

## Proposta

Separar explicitamente **Source Admission**, **ordering/recovery** e **saída derivada**.

### Live

```text
Telegram live UpdateHandler
        ↓
DurableSourceAdmission
        ↓
updates.Manager
        ↓
OrderedUpdateHandler / trigger de projeção
```

`DurableSourceAdmission` preserva a observação recebida antes de entregá-la ao manager.
Se a persistência exigida falhar, o envelope não é encaminhado e a durability barrier é
fechada.

O handler pós-Manager deixa de ser a única Source Evidence authority. Sua saída é
ordenada/reconstruída pelo gotd e deve ser tratada como entrada para trabalho derivado ou
como Evidence adicional somente quando explicitamente tipada como tal.

### Recovery

```text
Telegram recovery RPC
        ↓
SourcePreservingRecoveryAPI
        ↓
updates.Manager
```

Respostas de recovery que carregam observação recuperada, estabelecem/substituem baseline
ou representam descontinuidade são preservadas antes de serem entregues ao manager.

Isso inclui no mínimo:

- bootstrap/reset/resync que adote baseline remoto;
- `UpdatesDifference`/`UpdatesChannelDifference` com conteúdo recuperado;
- `UpdatesDifferenceTooLong`/`UpdatesChannelDifferenceTooLong`;
- demais classes que, se entregues ao manager sem Evidence, permitiriam certificar state
  que já não pode ser reconstruído a partir das observações persistidas.

O protótipo persistiu todas as respostas de Difference, inclusive vazias, como uma forma
conservadora de testar a ordem. **Isso não é uma decisão de produto.** Respostas vazias
sem observação de fonte podem ser tratadas apenas como state operacional se uma regra
`EvidenceRequired(response)` provar que nenhuma Evidence de domínio é necessária.

### State e fail-stop

Permanecem os contratos aceitos do ADR 017:

- `updates.Manager` continua responsável por ordering/recovery do protocolo;
- `GuardedStateStorage` mantém barrier-check + state-write linearizáveis;
- falha de Evidence ou state write fecha a barrier;
- `Supervisor` encerra a instância;
- replay é esperado;
- updates stateless não são autoridade de sync.

### Papel do handler pós-Manager

Depois que a Source Evidence que autoriza a transição já está durável, uma falha de
projeção/handler derivado **não precisa bloquear o sync state**. O estado derivado deve ser
reconstruível a partir da Evidence persistida.

Isso evita acoplar disponibilidade de projeções reconstruíveis à autoridade de
sincronização da fonte.

## Contratos validados

O Candidate v4 passou, entre outros, os seguintes casos:

1. envelope live composto com edit + delete é persistido integralmente antes de qualquer
   PTS correspondente avançar;
2. falha da Source Admission impede forwarding ao manager e não produz avanço de PTS;
3. `UpdatesDifference` com conteúdo é persistido antes do `SetState` correspondente;
4. falha ao persistir recovery impede a resposta de chegar ao manager;
5. `UpdatesChannelDifference` é persistido antes do channel PTS;
6. o mesmo contrato vale para `UpdatesChannelDifference` com conteúdo real;
7. falha do handler derivado pós-Manager não fecha a source durability barrier quando a
   Evidence necessária já está durável;
8. suite completa passa com race detector.

## Alternativas rejeitadas

- manter o handler pós-Manager como única captura raw;
- persistir somente a saída reconstruída do manager e chamar isso de envelope de fonte;
- bloquear sync state por toda falha de projeção derivada mesmo quando Source Evidence já
  está durável;
- reimplementar `pts/qts/seq` fora do gotd;
- tratar o codec JSON do protótipo como formato físico definitivo de Evidence.

## Trade-offs

### Benefícios

- preserva o envelope live antes de transformações internas do gotd;
- mantém o gotd como autoridade especializada de ordering/recovery;
- permite reconstruir projeções após falhas sem prender o sync a trabalho derivado;
- recovery com conteúdo deixa de avançar state sem uma observação durável correspondente;
- edits, deletes e envelopes compostos podem ser interpretados posteriormente sem perda
  causada pelo roteamento interno do manager.

### Custos

- introduz dois pontos explícitos de admissão de Evidence: live e recovery;
- precisa distinguir Evidence de fonte de saída ordenada/derivada;
- exige política clara para quais respostas de recovery vazias precisam de Evidence;
- a implementação final depende de storage capaz de sustentar a ordem Evidence -> state.

## Fora do escopo

- engine SQLite e PRAGMAs finais;
- schema físico de Evidence;
- codec JSON/BLOB/versionamento final;
- session/peer storage;
- Source Message Projection concreta;
- backfill/history progress;
- política detalhada de restart/backoff;
- processamento comercial.

## Gates restantes

Antes de produção:

1. provar backfill/live com authorities de progresso independentes;
2. integrar storage real que preserve Evidence -> state ordering;
3. definir o envelope físico/versionamento de Evidence;
4. preservar os contract tests no adapter de produção;
5. validar crash/restart na integração real, não apenas nos fakes de contrato.

## Relação com ADR 017

A maior parte do ADR 017 continua suportada. Porém, esta proposta altera materialmente a
posição e o significado da responsabilidade chamada `DurableEvidenceHandler` no diagrama
aceito.

Por isso, a recomendação é criar uma **nova Decision complementar/superseding apenas para
essa semântica de Source Admission**, sem editar retroativamente o ADR 017.

`Status: Ready` significa apenas que a proposta possui evidência suficiente para virar um
ADR `Proposed`. Não significa aceitação arquitetural.

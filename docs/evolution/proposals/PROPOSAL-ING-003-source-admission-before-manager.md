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
- EXP-LIMIAR-003;
- F-ING-008;
- F-ING-009;
- F-ING-010;
- PRs #151, #155, #156, #157 e #158;
- contract suites executadas com Go 1.26.6 no runner self-hosted.

A Fase E do EXP-LIMIAR-001 validou o Candidate v4 com 23 contract tests e race detector.

O EXP-LIMIAR-003 validou adicionalmente o gate de coexistência backfill/live com 8
contract tests e race detector, usando authorities de progresso separadas.

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

O EXP-LIMIAR-003 acrescentou a validação de que:

- live só recebe authority de `SourceSyncState`;
- backfill só recebe authority de `BackfillProgress`;
- ambos compartilham apenas admissão de Evidence;
- falha de Evidence não pode avançar o progresso correspondente;
- `BackfillProgress.Completed=true` não certifica continuidade live;
- `LastMessageID` legado não pode seedar authority atual;
- coexistência concorrente preserva as duas authorities.

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
  causada pelo roteamento interno do manager;
- backfill e live deixam de depender de um cursor genérico compartilhado.

### Custos

- introduz pontos explícitos de admissão de Evidence em live e recovery;
- precisa distinguir Evidence de fonte de saída ordenada/derivada;
- exige política clara para quais respostas de recovery vazias precisam de Evidence;
- a implementação final depende de storage capaz de sustentar a ordem Evidence -> state;
- backfill precisa de storage/capability de progresso própria na implementação concreta.

## Fora do escopo

- engine SQLite e PRAGMAs finais;
- schema físico de Evidence;
- codec JSON/BLOB/versionamento final;
- session/peer storage;
- Source Message Projection concreta;
- schema físico de `BackfillProgress`/`SourceSyncState`;
- política detalhada de restart/backoff;
- processamento comercial.

## Gates

### Suportado

1. **Backfill/live com authorities de progresso independentes** — SUPPORTED no
   EXP-LIMIAR-003, com 8 contract tests + race detector.

### Restantes antes de produção

1. integrar storage real que preserve Evidence -> state/progress ordering;
2. definir o envelope físico/versionamento de Evidence;
3. preservar os contract tests do EXP-LIMIAR-001 e EXP-LIMIAR-003 no adapter real;
4. validar crash/restart na integração real, não apenas nos fakes de contrato;
5. tornar explícita e testada a classificação de respostas de recovery sem Evidence de
   domínio.

## Relação com ADR 017

A maior parte do ADR 017 continua suportada. Porém, esta proposta altera materialmente a
posição e o significado da responsabilidade chamada `DurableEvidenceHandler` no diagrama
aceito.

Por isso, a recomendação é uma **nova Decision complementar e parcialmente superseding
apenas para essa semântica de Source Admission**, sem editar retroativamente o ADR 017.

O ADR 018 materializa essa recomendação como `Proposed`.

`Status: Ready` significa apenas que a proposta possui evidência suficiente para avaliação
de um ADR. Não significa aceitação arquitetural.
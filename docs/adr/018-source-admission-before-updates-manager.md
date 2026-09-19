# ADR 018 — Source Admission antes do updates.Manager

Authority: Decision Record
Status: Accepted
Accepted-by: LuigiAPCPereira
Accepted-at: 2026-09-19T17:08:00-03:00
Acceptance-reference: https://github.com/LuigiAPCPereira/limiar2/issues/212

> Este ADR foi aceito por autorização explícita do mantenedor, registrada na issue #212. A aceitação autoriza os contratos desta Decision, mas não dispensa os gates antes de produção.

## Contexto

O ADR 016 aceitou que Evidence represente a observação da fonte, seja preservada de forma append-only/versionada e preceda o avanço certificado de sync state.

O ADR 017 aceitou o boundary de recovery do gotd, incluindo `updates.Manager` como autoridade especializada de ordering/recovery, `GuardedStateStorage`, `DurabilityBarrier`, `Supervisor`, replay e fail-stop.

Depois dessa aceitação, o F-ING-009 encontrou uma premissa materialmente incompleta no diagrama do ADR 017: no `github.com/gotd/td v0.161.0`, o handler configurado depois do `updates.Manager` não recebe necessariamente o envelope bruto entregue pelo Telegram. O manager pode ordenar, separar, rotear e reconstruir batches antes do dispatch, inclusive perdendo metadata do envelope original.

O EXP-LIMIAR-001 Fase E validou o Candidate v4 com Go 1.26.6, gotd/td v0.161.0, 23 contract tests e race detector. A topologia suportada preserva o envelope live antes do manager e preserva respostas semanticamente relevantes de recovery antes de entregá-las ao manager.

O EXP-LIMIAR-003 validou adicionalmente que live sync e backfill podem manter authorities de progresso independentes:

```text
Live sync  -> SourceSyncState
Backfill   -> BackfillProgress
Ambos      -> Evidence append-only
```

A Proposal de origem é `PROPOSAL-ING-003-source-admission-before-manager.md`.

## Decision

Com a aceitação deste ADR, passam a valer os contratos abaixo.

### 1. Source Evidence live é admitida antes do updates.Manager

O envelope live recebido da integração Telegram deve atravessar um boundary de Source Admission antes de ser entregue ao `updates.Manager`.

```text
Telegram live UpdateHandler
        ↓
DurableSourceAdmission
        ↓
updates.Manager
        ↓
OrderedUpdateHandler / trabalho derivado
```

`DurableSourceAdmission` representa a responsabilidade de preservar a observação de fonte exigida pelo contrato. Nome de tipo, pacote e implementação concreta não são decididos por este ADR.

Se a persistência exigida falhar:

- o envelope não é encaminhado ao manager;
- a durability barrier é fechada;
- o lifecycle da instância deve terminar segundo o fail-stop aceito no ADR 017.

### 2. Recovery que carrega Evidence ou altera continuidade é preservado antes da adoção pelo manager

O wrapper de recovery deve preservar, antes de entregar ao manager, respostas que:

- contenham observações recuperadas da fonte;
- estabeleçam ou substituam baseline remoto;
- representem perda/descontinuidade de continuidade recuperável;
- de outro modo permitiriam certificar state que não pode ser reconstruído a partir da Evidence durável.

Casos mínimos incluem:

- bootstrap/reset/resync que adote baseline remoto;
- `UpdatesDifference` com conteúdo recuperado;
- `UpdatesChannelDifference` com conteúdo recuperado;
- `UpdatesDifferenceTooLong`;
- `UpdatesChannelDifferenceTooLong`.

A implementação pode possuir uma regra tipada equivalente a `EvidenceRequired(response)`. Resposta vazia sem observação de fonte pode permanecer apenas como state operacional quando essa classificação for explícita e testada. Classe desconhecida não pode ser silenciosamente tratada como "não requer Evidence".

### 3. O handler pós-Manager não é a autoridade da Source Evidence bruta

A saída do `updates.Manager` é ordenada/reconstruída e deve ser tratada como entrada para processamento derivado ou, quando necessário, como observação adicional explicitamente tipada.

Ela não pode ser chamada de envelope bruto original apenas por ter sido entregue a um handler do gotd.

Portanto, o `DurableEvidenceHandler` pós-Manager do diagrama do ADR 017 deixa de representar a única captura da Source Evidence.

### 4. Falha de trabalho derivado não bloqueia automaticamente sync state

Quando a Source Evidence exigida para uma transição já está durável, falha de projeção ou handler derivado pós-Manager não precisa fechar a source durability barrier nem impedir automaticamente o avanço de sync state.

Esse trabalho deve ser reconstruível/reexecutável a partir da Evidence preservada.

Isso não autoriza ignorar falhas da própria Source Admission, do recovery admission ou do `GuardedStateStorage`.

### 5. Os contratos de recovery/fail-stop do ADR 017 permanecem

Continuam aceitos:

- `updates.Manager` como autoridade especializada de ordering/recovery do Telegram;
- `GuardedStateStorage` com barrier-check + state-write linearizáveis;
- erro de leitura de state distinto de ausência de state;
- falha de Evidence/state write fechando a barrier;
- `Supervisor` encerrando a instância;
- barrier fechada terminal para aquela instância;
- restart como novo lifecycle a partir do state durável;
- replay esperado e ausência de promessa exactly-once;
- updates stateless sem autoridade para avançar `SourceSyncState`;
- `Forget=true`/resync apenas como ação explícita coberta pelo boundary apropriado.

### 6. Live sync e backfill não compartilham authority de progresso

Este ADR não redefine backfill, mas a Source Admission aqui proposta deve preservar a separação já exigida pelo ADR 016 e suportada pelo EXP-LIMIAR-003.

`SourceSyncState` não pode ser derivado de `BackfillProgress`, `LastMessageID` ou conclusão de history. Backfill não recebe autoridade para escrever `pts/qts/seq/channel pts`.

### 7. A forma física continua aberta

Este ADR decide responsabilidades e ordering lógico, não:

- schema SQL de Evidence;
- codec JSON/BLOB;
- engine SQLite;
- PRAGMAs;
- transação física concreta entre Evidence/state;
- package layout final;
- nome final dos tipos;
- session/peer storage.

Esses itens devem preservar os contracts deste ADR e dos ADRs 016/017 quando forem decididos.

## Consequências

### Positivas

- o envelope live é preservado antes das transformações internas do gotd;
- recovery com conteúdo não pode certificar progresso sem observação durável correspondente;
- ordering/recovery continuam delegados ao gotd;
- falhas reconstruíveis de projeção deixam de ser confundidas com falhas de admissão da fonte;
- edits, deletes e updates compostos permanecem reprocessáveis a partir da Evidence de origem;
- live e backfill permanecem semanticamente independentes.

### Negativas

- existem pontos explícitos de Source Admission em live e recovery;
- a implementação precisa distinguir Source Evidence de saída ordenada/derivada;
- a classificação de respostas de recovery precisa ser explícita e testável;
- storage real precisa sustentar a ordem Evidence -> state nos boundaries relevantes;
- crash/restart da integração final exige testes próprios.

## Alternativas rejeitadas

1. **Handler pós-Manager como única captura raw** — rejeitado pelo F-ING-009 e contract test do EXP-LIMIAR-001.
2. **Persistir somente batches reconstruídos e chamá-los de envelope original** — perde semântica/proveniência da observação da fonte.
3. **Bloquear sync por qualquer falha derivada** — acopla progresso da fonte a trabalho reconstruível e reduz disponibilidade sem aumentar durabilidade da Evidence.
4. **Reimplementar `pts/qts/seq` no domínio do Limiar** — desnecessário; o gotd continua responsável por ordering/recovery.
5. **Promover JSON do harness a schema físico** — o experimento só usou JSON para provar preservação e ordem.

## Relação com ADR 017

Este ADR **complementa o ADR 017 e prevalece sobre a cláusula conflitante referente à posição da Source Evidence bruta**.

Em particular, deixa de valer a interpretação de:

```text
updates.Manager -> DurableEvidenceHandler -> Evidence Store
```

como boundary único de preservação do envelope bruto de fonte.

Isso não muda o `Status: Accepted` do ADR 017 nem o transforma integralmente em `Superseded`: seus demais contratos continuam sendo autoridade, especialmente ordering/recovery, `GuardedStateStorage`, `DurabilityBarrier`, `Supervisor`, TooLong/resync, fail-stop e replay.

Se uma decisão futura substituir o ADR 017 como um todo, aí sim seu lifecycle deverá seguir `Accepted -> Superseded` conforme `AGENTS.md` e `docs/adr/README.md`.

O ADR 017 não deve ser editado retroativamente para esconder a evolução da decisão.

## Evidência de suporte

- ADR 016 — Accepted;
- ADR 017 — Accepted;
- F-ING-008 — comportamento/fail-stop do gotd;
- F-ING-009 — handler pós-Manager não preserva source envelope;
- F-ING-010 — `LastMessageID` legado mistura authorities;
- EXP-LIMIAR-001 — Candidate v4, 23 contracts + race detector;
- EXP-LIMIAR-003 — authorities backfill/live, 8 contracts + race detector;
- PROPOSAL-ING-003 — Source Admission antes do manager.

## Gates antes de produção

Mesmo com este ADR aceito, produção continua condicionada a:

1. storage real capaz de preservar o ordering Evidence -> state exigido pelos boundaries;
2. definição versionada do envelope físico de Evidence sem perder a observação da fonte;
3. manutenção dos contracts do EXP-LIMIAR-001 no adapter real;
4. manutenção dos contracts do EXP-LIMIAR-003 para separar live/backfill;
5. testes de crash/restart na integração real, não apenas no harness;
6. política explícita e testada para classes de recovery sem Evidence de domínio.

## Escopo da aceitação

`Status: Accepted` significa que o mantenedor autorizou expressamente os contratos desta Decision, conforme issue #212.

A aceitação não dispensa os gates de produção acima e não promove automaticamente os ADRs 021/022/023, migração do legado, política de backup/restore ou outras decisões independentes.
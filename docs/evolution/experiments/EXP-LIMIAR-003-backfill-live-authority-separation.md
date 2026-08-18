# EXP-LIMIAR-003 — Separação de authority entre backfill e live sync

Authority: Non-authoritative
Status: Supported

## Hipótese

Backfill histórico e sincronização live podem coexistir sem compartilhar autoridade de
progresso se cada lifecycle receber somente a capability de state que lhe pertence:

```text
Live sync  -> SourceSyncState
Backfill   -> BackfillProgress
Ambos      -> Evidence append-only
```

A propriedade inclui duas restrições adicionais:

1. Evidence deve estar durável antes de o progresso correspondente avançar;
2. concluir o backfill não certifica continuidade do live sync.

## Motivação

O F-ING-010 encontrou que o legado usa `channels.last_message_id` nos dois lados:

- backfill lê/escreve o campo como cursor de history;
- live escreve o mesmo campo após persistir mensagens;
- o backfill ainda pode atualizar esse cursor depois de apenas enfileirar `WriteJob`,
  antes de o `dbWriter` confirmar a durabilidade da mensagem.

Isso conflita com o ADR 016 Accepted, que já determina que:

- live usa state nativo do Telegram;
- `LastMessageID` não é authority de live sync;
- backfill possui progresso próprio;
- history não certifica ausência de gaps live.

O ADR 017 exige como gate antes de produção provar coexistência backfill/live sem
compartilhar autoridade de progresso.

## Escopo do experimento

Foi construído um contract model descartável em Go, sem dependências externas e sem
código de produção.

Os tipos do teste são semânticos, não schema aprovado:

- `sourceSyncState` representa `pts/qts/seq/date/channel pts`;
- `backfillProgress` representa apenas posição/cobertura histórica;
- `evidenceObservation` representa uma aquisição append-only;
- `sourceSyncStore` e `backfillProgressStore` são capabilities distintas;
- live e backfill compartilham somente `evidenceAppender`.

Nenhum actor recebe uma interface genérica capaz de escrever os dois progressos.

## Critérios exercitados

1. avanço de backfill não altera `SourceSyncState`;
2. avanço live não altera `BackfillProgress`;
3. falha de Evidence histórica impede avanço de progresso histórico;
4. falha de Evidence live impede avanço de sync state e não toca backfill;
5. restart carrega cada state por sua própria capability, sem usar
   `legacyLastMessageID` como seed;
6. a mesma mensagem observada por history e live preserva duas aquisições de Evidence;
7. `BackfillProgress.Completed=true` não altera nem certifica continuidade live;
8. live e backfill podem executar concorrentemente sem corromper/mesclar authorities.

## Execução

O módulo foi espelhado temporariamente em `LuigiAPCPereira/agent-runtime#20` somente
para utilizar o self-hosted runner já disponível.

Ambiente:

```text
Runner: LuigiCachyOS / actions-runner 2.336.0
Go: 1.26.6 linux/amd64
Workflow run: 32192151509
```

Resultado final:

```text
8 contract tests                     PASS
go test -race ./... -count=1        PASS
```

A execução concorrente utilizou 100 admissões live e 100 admissões históricas no mesmo
store de teste e terminou com os dois progressos independentes e 200 Evidence
preservadas.

## Interpretação

O experimento suporta o seguinte contract boundary:

```text
                    +-----------------+
Live Evidence ----> | Source admission| ----> SourceSyncState
                    +-----------------+

                    +-----------------+
History Evidence -> | Backfill        | ----> BackfillProgress
                    +-----------------+
```

`SourceSyncState` e `BackfillProgress` podem residir no mesmo banco/processo físico,
mas **não são a mesma authority** e não devem ser atualizados por um cursor genérico
compartilhado.

Para backfill, a ordem segura é:

```text
history snapshot obtido
        ↓
Evidence durável
        ↓
BackfillProgress pode avançar
```

Se Evidence falha, replay da página é preferível a pular uma faixa histórica.

`Completed=true` significa apenas que a política de varredura histórica declarou sua
janela coberta. Não significa que `pts/qts/seq/channel pts` estejam contínuos nem pode
ser usado para reparar live sync.

Overlap é esperado: uma observação histórica e uma observação live da mesma
`SourceMessageKey` são Evidence distintas. A reconciliação pertence à projeção
derivada, não ao state operacional.

## Limitações

Este experimento não prova:

- schema SQL físico de `BackfillProgress` ou `SourceSyncState`;
- fsync/crash semantics da engine escolhida;
- integração do contract com o collector legado;
- política final de paginação/janela histórica;
- performance do storage real;
- que live e backfill precisam executar simultaneamente;
- ausência de bugs numa futura implementação concreta.

O teste usa store em memória propositalmente para isolar **authority e ordering**, não
para substituir o experimento de storage.

## Relação com os gates aceitos

- ADR 016, gate 10: **SUPPORTED no contract model**;
- ADR 017, gate 7: **SUPPORTED no contract model**;
- ADR 017, gate 8 (storage capaz de cumprir Evidence/state ordering): continua sendo
  requisito de integração antes de produção.

A implementação concreta deve preservar estes contracts junto dos testes de recovery do
EXP-LIMIAR-001 e dos contracts físicos de storage.

## Conclusão epistemológica

**SUPPORTED.**

Separar live e backfill por capabilities de state próprias impede que progresso
histórico seja confundido com continuidade Telegram e impede que live altere a posição
de history. O desenho também preserva a regra Evidence-before-progress nos dois
lifecycles.

Isso confirma um gate de implementação já exigido pelos ADRs Accepted. Não cria nova
Decision, não escolhe schema e não autoriza por si só alteração de produção.

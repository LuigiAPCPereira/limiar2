# ADR 016 — Source Evidence e sincronização Telegram

Authority: Decision Record
Status: Accepted
Accepted-by: LuigiAPCPereira
Accepted-at: 2026-08-18T12:46:00-03:00
Acceptance-reference: PR #150

## Contexto

O ingress legado do Limiar foi construído em torno de `raw_messages`, cursor por
`LastMessageID` e uma pipeline de fan-in para persistência. A arqueologia da Rebaseline
2026 encontrou situações em que esse desenho pode:

- descartar live updates sob backpressure;
- avançar progresso antes da durabilidade equivalente;
- atravessar uma falha anterior com cursor posterior;
- colidir edits na mesma chave física de mensagem;
- ignorar deletes como Evidence;
- tratar message ID como mecanismo de sincronização live, embora Telegram use estado de
  updates próprio.

A documentação primária do Telegram também deixa claro que um Update pode representar
múltiplos eventos (por exemplo, deletes) e que a recuperação possui limites, incluindo
casos `differenceTooLong`/`ChannelDifferenceTooLong`. O gotd documenta limitações
adicionais para updates stateless e para recovery de `ChannelDifferenceTooLong`.

A Constituição exige especialmente:

- C-01 — Evidence admitida não desaparece silenciosamente;
- C-02 — estado derivado é reconstruível;
- C-03 — progresso não certifica durabilidade inexistente;
- C-08 — topologia não define autoridade;
- C-11 — implementação existente não cria autoridade por existência.

O registry de transição já classifica:

- ADR 002 como `RETAIN-CORE / REWRITE`;
- ADR 005 como `SUPERSEDE`;
- ADR 006 como `RETIRE`.

Base de pesquisa:

- `docs/evolution/experiments/EXP-LIMIAR-001-durable-telegram-update-recovery.md`;
- `docs/evolution/proposals/PROPOSAL-ING-001-durable-telegram-ingress.md`.

---

## Decision

O ingress do Limiar segue os princípios abaixo.

### 1. Evidence de fonte é append-only/versionada

Cada observação relevante recebida da fonte deve poder existir como Evidence própria.

Create, edit, delete, replay, update composto e snapshot histórico não são reduzidos
automaticamente a uma única linha mutável por mensagem.

A identidade física da Evidence não será `(channel_id, message_id)`.

Uma Evidence representa a observação da fonte. Ela pode não corresponder a nenhuma,
a uma ou a várias `SourceMessageKey`; projeções derivadas fazem esse mapeamento sem
falsificar a observação original.

### 2. Identidade lógica da mensagem é separada da Evidence

Uma `SourceMessageKey` identifica a mensagem lógica no escopo da fonte.

Ela não identifica a revisão, evento ou Evidence individual.

Estado corrente da mensagem será uma projeção derivada e reconstruível.

### 3. Sync live usa estado nativo do Telegram

A continuidade live será baseada no estado de update definido pelo protocolo e tratado
pela integração Telegram — `pts`, `qts`, `seq`, `date` e channel state quando aplicável.

`LastMessageID` não será autoridade de sincronização live.

Essa decisão não implica que todo gap seja sempre recuperável. Limites documentados do
protocolo e da biblioteca precisam ser tratados explicitamente, incluindo updates
stateless e situações `differenceTooLong`/`ChannelDifferenceTooLong`.

### 4. Backfill é lifecycle separado

Histórico/backfill terá progresso próprio.

Posição de history pode usar IDs de mensagem quando apropriado à API de histórico, mas
não certifica ausência de gaps no fluxo live.

### 5. Durabilidade precede avanço certificado de sync state

A implementação deve definir explicitamente o boundary de admissão entre observação da
fonte, persistência da Evidence exigida pelo contrato e avanço certificado do sync state.

Esse boundary deve impedir:

```text
Evidence exigida ainda não durável / sync state persistido como avançado
```

São aceitáveis:

```text
Evidence não durável / state antigo
Evidence durável / state antigo
Evidence durável / state avançado
```

O segundo caso implica possibilidade de replay e deve ser tratado idempotentemente.

### 6. Replay é permitido; exactly-once não é prometido

Dentro do boundary controlado pelo Limiar, duplicidade/replay explícitos são preferíveis
à perda silenciosa e devem ser reconciliáveis pelo downstream.

Este ADR não promete uma garantia end-to-end de observação de todo evento que já tenha
ultrapassado os limites de recuperação do Telegram.

### 7. History não fabrica eventos

Observação obtida por histórico é registrada como snapshot/aquisição histórica quando o
evento original não foi observado diretamente.

Metadados como data de edição podem ser preservados como fatos declarados pela fonte,
sem afirmar que o Limiar observou aquele edit em tempo real.

### 8. gotd permanece a integração Telegram; a mecânica concreta de recovery é Candidate

O princípio do ADR 002 de usar `github.com/gotd/td` para MTProto/Telegram é mantido.

O Limiar deve aproveitar o mecanismo nativo de ordering/recovery do gotd quando ele
satisfizer o contrato, em vez de reimplementar `pts/qts/seq` no domínio do produto.

Porém, a forma concreta `updates.Manager + DurableEvidenceHandler +
DurabilityBarrier/GuardedStateStorage` **não é decidida por este ADR**. Ela permanece
Candidate até os contract tests do EXP-LIMIAR-001 serem executados contra a versão
escolhida do gotd.

Se esses testes invalidarem premissas materiais, deve haver nova Proposal/ADR ou revisão
explícita antes de implementação de produção.

---

## Consequências

### Positivas

- Evidence preserva edits, deletes, updates compostos e replays;
- sincronização respeita o modelo real do protocolo;
- backfill deixa de fingir autoridade sobre live sync;
- falha de persistência não pode ser mascarada por cursor posterior;
- projeções podem ser reconstruídas a partir das observações preservadas;
- limitações de recovery tornam-se explícitas em vez de escondidas por um cursor simples.

### Negativas

- haverá mais de um tipo de state operacional;
- storage precisará modelar Evidence e sync state separadamente;
- replay/idempotência passa a ser requisito explícito;
- casos `differenceTooLong`/`ChannelDifferenceTooLong` precisam de política própria;
- testes de crash/restart e concorrência tornam-se mais importantes;
- migração do banco legado não pode ser um simples `ALTER TABLE` conceitual.

---

## Fora do escopo deste ADR

Este ADR não decide:

- engine SQLite concreta;
- PRAGMAs de durabilidade;
- schema SQL final de Evidence;
- formato JSON/BLOB do payload;
- session storage;
- peer cache;
- mecânica concreta do gotd recovery/barrier;
- política final para stateless updates e `differenceTooLong`/`ChannelDifferenceTooLong`;
- Source Message Projection concreta;
- migration tooling do banco legado;
- mídia;
- processamento comercial;
- IA.

Esses assuntos exigem decisões próprias quando estruturais.

---

## Relação com ADRs legados

Com esta aceitação:

- **ADR 002** — o princípio de gotd é mantido, com boundary de sync reescrito por este ADR;
- **ADR 005** — fica superseded quanto ao contrato de persistência raw por mensagem;
- **ADR 006** — fica superseded/retired quanto ao uso de `LastMessageID` como autoridade
  de sync live;
- **ADR 003** — este ADR não decide writer topology, mas remove o DBWriter global como
  fonte implícita de autoridade de durabilidade.

---

## Verificação necessária antes de implementação

Além de testes unitários do novo boundary, executar contract tests reais cobrindo:

1. handler failure antes da Evidence durável;
2. Evidence commitada + state write falha;
3. common gap recovery;
4. channel gap recovery;
5. cancelamento durante persistência;
6. restart após estado ambíguo;
7. replay da mesma observação;
8. edit e delete da mesma `SourceMessageKey`;
9. update composto afetando múltiplas mensagens;
10. backfill coexistindo com sync live sem compartilhar autoridade de progresso;
11. comportamento/limitação para updates stateless;
12. `differenceTooLong` e `ChannelDifferenceTooLong`.

---

## Escopo da aceitação

A aceitação deste ADR aprova somente os **princípios de Evidence e sincronização**
descritos acima.

Ela não aprova automaticamente a implementação experimental do gotd barrier, o schema
físico de Evidence nem políticas ainda abertas para limites de recovery.

A implementação concreta continua condicionada aos gates e contract tests descritos
neste documento e no EXP-LIMIAR-001.

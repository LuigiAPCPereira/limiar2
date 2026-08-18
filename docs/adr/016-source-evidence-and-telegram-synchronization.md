# ADR 016 — Source Evidence e sincronização Telegram

Authority: Decision Record
Status: Proposed

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

## Decision proposta

Se este ADR for aceito, o ingress do Limiar seguirá os princípios abaixo.

### 1. Evidence de fonte é append-only/versionada

Cada observação relevante recebida da fonte deve poder existir como Evidence própria.

Create, edit, delete, replay e snapshot histórico não são reduzidos automaticamente a
uma única linha mutável por mensagem.

A identidade física da Evidence não será `(channel_id, message_id)`.

### 2. Identidade lógica da mensagem é separada da Evidence

Uma `SourceMessageKey` identifica a mensagem lógica no escopo da fonte.

Ela não identifica a revisão, evento ou Evidence individual.

Estado corrente da mensagem será uma projeção derivada e reconstruível.

### 3. Sync live usa estado nativo do Telegram

A continuidade live será baseada no estado de update definido pelo protocolo e tratado
pela integração Telegram — `pts`, `qts`, `seq`, `date` e channel state quando aplicável.

`LastMessageID` não será autoridade de sincronização live.

### 4. Backfill é lifecycle separado

Histórico/backfill terá progresso próprio.

Posição de history pode usar IDs de mensagem quando apropriado à API de histórico, mas
não certifica ausência de gaps no fluxo live.

### 5. Durabilidade precede avanço certificado de sync state

O boundary de persistência deve impedir o estado:

```text
Evidence não durável / sync state persistido como avançado
```

São aceitáveis:

```text
Evidence não durável / state antigo
Evidence durável / state antigo
Evidence durável / state avançado
```

O segundo caso implica possibilidade de replay e deve ser tratado idempotentemente.

### 6. A garantia é at-least-once, não exactly-once

O Limiar deve preferir replay explícito a perda silenciosa.

Downstream derivado deve suportar idempotência/reconciliação adequada.

### 7. History não fabrica eventos

Observação obtida por histórico é registrada como snapshot/aquisição histórica quando o
evento original não foi observado diretamente.

Metadados como data de edição podem ser preservados como fatos declarados pela fonte,
sem afirmar que o Limiar observou aquele edit em tempo real.

### 8. gotd permanece a integração Telegram, mas a mecânica concreta ainda exige gate

O princípio do ADR 002 de usar `github.com/gotd/td` para MTProto/Telegram é mantido.

A proposta é usar o mecanismo de recovery/ordering do pacote `telegram/updates` em vez de
reimplementar `pts/qts/seq` no domínio do Limiar.

Porém, a forma concreta `recovery manager + DurableEvidenceHandler +
DurabilityBarrier/GuardedStateStorage` **não fica autorizada para produção apenas pela
aceitação deste ADR** até que os contract tests pendentes do EXP-LIMIAR-001 sejam
executados contra a versão escolhida do gotd.

Se esses testes invalidarem premissas materiais, deve haver nova Proposal/ADR ou revisão
explícita antes da implementação.

---

## Consequências

### Positivas

- Evidence preserva edits, deletes e replays;
- sincronização passa a respeitar o modelo real do protocolo;
- backfill deixa de fingir autoridade sobre live sync;
- falha de persistência não pode ser mascarada por cursor posterior;
- projeções podem ser reconstruídas a partir das observações preservadas;
- mecanismo de recovery do gotd pode ser aproveitado sem transportar sua mecânica para o
  domínio comercial.

### Negativas

- haverá mais de um tipo de state operacional;
- storage precisará modelar Evidence e sync state separadamente;
- replay/idempotência passa a ser requisito explícito;
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
- Source Message Projection concreta;
- migration tooling do banco legado;
- mídia;
- processamento comercial;
- IA.

Esses assuntos exigem decisões próprias quando estruturais.

---

## Relação com ADRs legados

Se aceito:

- **ADR 002** — princípio de gotd é mantido, com boundary de sync reescrito por este ADR;
- **ADR 005** — será superseded quanto ao contrato de persistência raw por mensagem;
- **ADR 006** — será superseded/retired quanto ao uso de `LastMessageID` como autoridade
  de sync live;
- **ADR 003** — este ADR não decide writer topology, mas remove o DBWriter global como
  fonte implícita de autoridade de durabilidade.

Enquanto `Status: Proposed`, nenhuma dessas supersessões entra em vigor por causa deste
documento.

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
9. backfill coexistindo com sync live sem compartilhar autoridade de progresso.

---

## Critério para aceitação

A aceitação deste ADR deve significar aprovação dos **princípios de Evidence e
sincronização** descritos acima.

Ela não deve ser interpretada como aprovação automática da implementação experimental
do gotd barrier enquanto os contract tests explicitamente pendentes não forem
concluídos.

Se aceito, registrar `Accepted-by`, `Accepted-at` e `Acceptance-reference` conforme
`docs/adr/README.md`.
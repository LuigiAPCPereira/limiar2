# ADR 020 — Schema físico mínimo de Evidence

Authority: Decision Record
Status: Proposed

> Enquanto este ADR estiver `Proposed`, ele não cria schema canônico de produção nem autoriza migration de dados.

## Contexto

Os ADRs 016 e 017 já tornam Evidence append-only/versionada uma authority separada de `SourceSyncState` e `BackfillProgress`. O ADR 019, agora `Accepted`, define o novo banco SQLite/ncruces, migrations SQL versionadas e payload de Evidence codec-agnostic, mas deixa explicitamente abertos o schema SQL final, o tipo físico de `EvidenceRecord.id`, a precisão dos timestamps, os índices e a identidade/deduplicação física.

Há Evidence executável suficiente para decidir um núcleo menor sem inventar um schema de produto completo:

- `F-STO-004` confirma que o RunID legado não satisfaz o contrato de identidade de Evidence;
- `EXP-LIMIAR-004` está `Supported` para UUIDv4 aleatório em `BLOB(16)`, falha fechada de entropia, payload/hash binários, timestamps Unix em milissegundos, export/import, reopen e tabela `STRICT`;
- `EXP-LIMIAR-005` está `Supported` para capability de append + triggers persistentes contra `UPDATE`/`DELETE`; no harness, a própria capability calcula `SHA-256` a partir dos bytes de payload antes do `INSERT`;
- `EXP-LIMIAR-008`, 009 e 010 suportam o ordering físico Evidence-before-state/progress nos boundaries exercitados.

O objetivo desta Decision proposta é fixar somente o envelope físico mínimo necessário para começar o novo storage sem transformar conveniências de query, projeção ou apresentação em identidade de domínio.

## Decision proposta

Se aceito, o novo banco passa a ter uma tabela canônica `evidence` com o contrato abaixo.

### 1. Identidade física

`EvidenceRecord.id` usa UUIDv4 gerado com `crypto/rand`, persistido como `BLOB(16)`.

Regras:

- geração de ID falha fechada se a fonte de entropia falhar;
- `id` é `PRIMARY KEY` e possui `CHECK(length(id) = 16)`;
- `id` não deriva de timestamp, `rowid`, hash do payload, message ID ou posição de ingestão;
- replay legítimo recebe nova identidade de Evidence quando representa nova admissão/observação;
- importação preserva IDs já atribuídos ao novo modelo quando aplicável.

UUIDv4 é escolhido por ser a menor alternativa já exercitada que satisfaz o contrato. Esta Decision não afirma superioridade geral sobre UUIDv7/ULID.

### 2. Envelope mínimo

O schema inicial contém:

```sql
CREATE TABLE evidence (
    id                 BLOB    NOT NULL PRIMARY KEY CHECK(length(id) = 16),
    subscription_id    TEXT    NOT NULL CHECK(length(subscription_id) > 0),
    acquisition        TEXT    NOT NULL CHECK(length(acquisition) > 0),
    event_kind         TEXT    NOT NULL CHECK(length(event_kind) > 0),
    source_event_type  TEXT    NOT NULL CHECK(length(source_event_type) > 0),
    source_occurred_at INTEGER,
    received_at        INTEGER NOT NULL,
    payload_format     TEXT    NOT NULL CHECK(length(payload_format) > 0),
    payload_schema     TEXT    NOT NULL CHECK(length(payload_schema) > 0),
    payload            BLOB    NOT NULL,
    payload_sha256     BLOB    NOT NULL CHECK(length(payload_sha256) = 32)
) STRICT;
```

Semântica:

- `subscription_id` identifica o escopo configurado de aquisição que admitiu a Evidence; não é identidade de mensagem;
- `acquisition` distingue formas de aquisição como live update, replay ou snapshot histórico sem fabricar evento não observado;
- `event_kind` classifica a família semântica da observação no boundary de ingress;
- `source_event_type` preserva o tipo de evento declarado pela integração/fonte;
- `source_occurred_at` é opcional porque a fonte pode não fornecer um instante confiável para todo evento;
- `received_at` registra quando o Limiar admitiu/recebeu a observação;
- `payload_format` e `payload_schema` tornam o BLOB reprocessável/versionável;
- `payload_sha256` é metadata derivada de integridade/proveniência, não identidade.

### 3. Representação temporal

`source_occurred_at` e `received_at` usam Unix time em milissegundos, UTC, persistido como `INTEGER` assinado de 64 bits no boundary Go/SQLite.

O identificador não codifica tempo. Duas Evidence podem compartilhar o mesmo timestamp.

### 4. Payload e hash não deduplicam Evidence

Não há `UNIQUE` em:

- `payload_sha256`;
- `received_at`;
- combinação de timestamp + hash;
- `SourceMessageKey` ou equivalentes;
- `(channel_id, message_id)`.

Evidence repetida pode ser semanticamente necessária para replay, auditoria ou observações distintas da mesma mensagem.

O `payload_sha256` não é fornecido como valor autoritativo pelo consumidor da capability de append. O storage calcula `SHA-256` internamente sobre os mesmos bytes que serão persistidos em `payload` e grava ambos na mesma operação. Assim, o `CHECK(length(payload_sha256) = 32)` valida somente o shape físico; a correspondência hash/payload é responsabilidade do boundary de persistência e pode ser revalidada por auditoria/reopen.

Importadores side-by-side também devem derivar o hash dos bytes efetivamente importados, em vez de confiar em hash legado inexistente ou não verificável.

### 5. `STRICT` é baseline; `WITHOUT ROWID` não

A tabela usa `STRICT` para tornar mismatch físico mais observável.

Apesar de `WITHOUT ROWID` ter passado funcionalmente no EXP-LIMIAR-004, ele não entra no baseline inicial porque o experimento não demonstrou benefício com payloads e workloads representativos. A presença de um rowid interno não lhe confere autoridade de identidade; consumidores não dependem dele.

### 6. Append-only é enforced no boundary normal e defendido no schema

Consumidores normais recebem uma capability estreita de append e não recebem API de update/delete de Evidence.

A migration inicial instala guards persistentes equivalentes a:

```sql
CREATE TRIGGER evidence_no_update
BEFORE UPDATE ON evidence
BEGIN
    SELECT RAISE(ABORT, 'evidence is append-only');
END;

CREATE TRIGGER evidence_no_delete
BEFORE DELETE ON evidence
BEGIN
    SELECT RAISE(ABORT, 'evidence is append-only');
END;
```

Esses triggers são defesa contra mutação acidental e violação interna do boundary, não mecanismo de segurança contra quem pode alterar DDL ou o arquivo SQLite offline.

Remoção/redação legal, se algum dia necessária, exige política e Decision própria; não é implementada silenciosamente por bypass dos guards.

### 7. Índices secundários permanecem mínimos

A migration inicial não cria índice de domínio ou deduplicação.

Antes de adicionar índices secundários permanentes, deve existir query pattern real ou gate que demonstre a necessidade. `PRIMARY KEY(id)` é suficiente para o primeiro slice de append/round-trip.

Se replay operacional exigir ordenação/indexação adicional, ela deve preservar que timestamp e ordem local não definem identidade de Evidence.

### 8. Migrations são a authority

A criação desta tabela e de seus guards, quando autorizada, ocorre em migration SQL versionada do novo banco definido pelo ADR 019.

Não existe repair concorrente em runtime que reconstrua ou altere esse schema ad hoc.

## Fora do escopo

Este ADR não decide:

- schema de `SourceSyncState`;
- schema de `BackfillProgress`;
- Source Message Projection;
- codec Telegram concreto além do requirement de `payload_format`/`payload_schema`;
- ledger de importação do legado;
- política de backup/restore;
- session/peer storage;
- índices de projeções/queries de produto;
- política de retenção/redação;
- power-loss durante `fsync`.

## Consequências

### Positivas

- identidade de Evidence deixa de depender de detalhes locais/legados;
- payload permanece byte-preserving e reprocessável;
- hash é derivado dentro do mesmo boundary que persiste o payload, evitando uma segunda autoridade do caller;
- tempo é explícito e independente da identidade;
- replay/duplicidade legítimos não são colapsados pelo schema;
- append-only possui uma capability clara e defesa física observável;
- o primeiro slice de storage pode ser pequeno e auditável.

### Negativas

- UUIDv4 exige fonte de entropia disponível e tratamento de erro;
- BLOB PK não possui ordenação temporal natural;
- triggers não protegem contra DDL/offline access;
- ausência inicial de índices secundários pode exigir tuning posterior;
- `subscription_id` passa a ser metadata persistida e precisa de contract estável no ingress;
- auditoria de integridade precisa recomputar hash quando quiser validar conteúdo histórico, pois SQLite não garante a correspondência payload/hash apenas pelo `CHECK` de tamanho.

## Evidência de suporte

- `docs/evolution/findings/F-STO-004-legacy-runid-is-not-evidence-identity.md`;
- `docs/evolution/experiments/EXP-LIMIAR-004-evidence-physical-envelope.md`;
- `docs/evolution/experiments/EXP-LIMIAR-005-append-only-evidence-enforcement.md`;
- `docs/evolution/experiments/EXP-LIMIAR-008-evidence-progress-physical-ordering.md`;
- EXP-LIMIAR-009 e EXP-LIMIAR-010 para integração física com `updates.Manager` e ncruces;
- RFC 9562, UUID version 4;
- documentação oficial SQLite para `STRICT` e triggers.

## Gates de implementação se aceito

1. migration SQL versionada cria `evidence` e guards em banco novo vazio;
2. UUIDv4 válido + falha fechada de entropia;
3. capability de append calcula `payload_sha256` internamente a partir dos mesmos bytes persistidos em `payload`;
4. round-trip byte a byte de ID/payload/hash e recomputação confirma a correspondência payload/hash;
5. payload/timestamp repetidos coexistem;
6. `UPDATE` e `DELETE` são rejeitados e não alteram a linha;
7. close/reopen + `PRAGMA integrity_check = ok`;
8. export/import preserva ID e bytes e produz hash correspondente ao payload importado;
9. `CGO_ENABLED=0 go test ./...` e `go test -race ./...` passam no slice relevante;
10. nenhum caminho de produção toca o banco Tursogo legado in-place;
11. a integração com state/progress mantém os gates do ADR 019.

## Escopo da proposta

`Status: Proposed` não autoriza criar o schema canônico em produção.

A aceitação exige instrução explícita do mantenedor e os metadados definidos em `AGENTS.md`/`docs/adr/README.md`.
# PROPOSAL-STO-001 — SQLite local e contrato físico mínimo de Evidence

Authority: Non-authoritative
Status: Ready

## Problema

Os ADRs 016 e 017 exigem Evidence durável antes de avanço certificado de sync state, mas
não decidem engine, PRAGMAs, schema físico de Evidence ou autoridade de migrations.

O storage legado ainda está acoplado a Tursogo, ao DBWriter global e a
`PRAGMA synchronous=NORMAL`. O ADR 001 legado está classificado como `SUPERSEDE` pela
Rebaseline 2026.

Precisamos de um storage simples e portátil que preserve a durabilidade exigida sem
transportar decisões históricas por inércia.

## Evidência

- ADR 016 — Source Evidence e sincronização Telegram;
- ADR 017 — Boundary durável de recovery Telegram;
- `EXP-LIMIAR-002-storage-durability.md`;
- `F-STO-001-tursogo-synchronous-compatibility.md`;
- código legado em `internal/storage`.

O EXP-LIMIAR-002 executou o mesmo contrato contra Tursogo v0.7.2 e
`ncruces/go-sqlite3 v0.35.3`; ambos passaram testes normais, runtime com
`CGO_ENABLED=0` e race detector em execução separada com CGO habilitado.

## Proposta

### 1. Engine preferencial: `ncruces/go-sqlite3`

Adotar `github.com/ncruces/go-sqlite3` como candidato preferencial para o **novo** banco
da rebaseline.

Razões:

- mantém a interface `database/sql` já conhecida pelo projeto;
- mantém runtime/build normal sem CGO;
- usa SQLite como engine, reduzindo uma camada de compatibilidade SQL específica;
- passou todo o contrato de durabilidade relevante junto com Tursogo;
- permite manter o novo storage em uma superfície SQL conservadora.

Tursogo não é classificado como incorreto. Ele permanece útil para leitura/migração do
banco legado e como alternativa revalidável. A preferência por ncruces não deve ser
justificada pelo microteste de desempenho isoladamente.

### 2. Um SQLite local inicialmente com uma conexão

Baseline operacional candidata:

```text
SetMaxOpenConns(1)
SetMaxIdleConns(1)
PRAGMA journal_mode=WAL
PRAGMA synchronous=FULL
PRAGMA foreign_keys=ON
PRAGMA busy_timeout=5000
```

Uma conexão reduz concorrência implícita, simplifica ownership inicial e também limita o
custo por conexão da implementação baseada em Wasm.

`cache_size` não entra como baseline sem evidência específica.

### 3. Banco novo side-by-side

Não transformar o banco legado em-place na nova arquitetura.

O novo storage nasce em arquivo próprio. O banco Tursogo antigo é preservado como artefato
imutável/read-only durante importação e rollback.

A migração detalhada fica em Proposal/ADR próprio.

### 4. Evidence é append-only e codec-agnostic no storage

Contrato lógico mínimo candidato:

```text
EvidenceRecord
- id
- subscription_id
- acquisition
- event_kind
- source_event_type
- source_occurred_at?
- received_at
- payload_format
- payload_schema
- payload
- payload_sha256
```

Regras:

- `payload` é BLOB opaco para a camada de storage;
- `payload_format` informa o codec/família;
- `payload_schema` identifica explicitamente a versão do envelope;
- `payload_sha256` ajuda integridade/proveniência, mas **não é UNIQUE** e não define
  identidade/deduplicação de Evidence;
- replay legítimo deve poder coexistir;
- não existe `UNIQUE(channel_id, message_id)` na Evidence;
- não exigir `source_message_id` singular: uma observação composta pode afetar múltiplas
  mensagens e o mapeamento pertence à projeção derivada.

### 5. O tipo físico de `id` permanece aberto

Esta Proposal não fixa `INTEGER`, UUID textual ou BLOB de 16 bytes.

O identificador precisa permanecer estável através de export/import/migração e suportar
lineage. A escolha deve ser feita antes do schema Accepted, sem acoplar Evidence à posição
local `rowid` por acidente.

### 6. Tempo persistido em forma inequívoca

Timestamps estruturais devem ser armazenados em representação sem ambiguidade e
normalizados pelo contrato, preferencialmente inteiro Unix em precisão definida pelo
schema (por exemplo, milissegundos), em vez de depender de parsing textual livre.

A precisão definitiva faz parte do schema Accepted.

### 7. Sync state é operacional e separado

O mesmo banco pode armazenar o state operacional do Telegram em estruturas separadas de
Evidence:

- state comum (`pts/qts/seq/date`);
- channel PTS;
- metadata operacional necessária ao adapter.

Esses registros não são identidade de domínio nem derivados do PTS/QTS stateless de
payloads recuperados.

O `GuardedStateStorage` do ADR 017 controla as writes.

### 8. Migrations têm uma única autoridade

Para o banco novo, migrations SQL versionadas são a autoridade de evolução de schema.

Rotinas Go que “consertam” schema dinamicamente podem permanecer apenas no caminho de
compatibilidade/importação legado; não devem formar uma segunda autoridade concorrente no
novo banco.

### 9. Storage não expõe `*sql.DB` como autoridade global

`database/sql` continua detalhe de infraestrutura.

Os consumidores usam interfaces/repositories estreitos para Evidence, sync state e outros
contratos. Não preservar o padrão legado em que `DB()` exposto + comentário de DBWriter
definem implicitamente quem pode gravar.

### 10. Sessão MTProto não é decidida aqui

Credenciais/sessão têm boundary de segredo diferente de Evidence e sync state. A decisão
de sessão permanece separada; esta Proposal não exige colocá-la no mesmo `.db`.

### 11. Backup inicial conservador

`VACUUM INTO` passou no harness para ambas as engines e é suficiente como capability
mínima demonstrada para snapshot consistente do novo DB.

A política operacional de retenção, atomicidade de publicação, restore e cópia de WAL
merece tratamento próprio antes de depender de backups em produção.

## Alternativas

### Continuar Tursogo como escolha fixa

Viável tecnicamente no contrato atual, mas não recomendado como default da rebaseline:
a escolha histórica está explicitamente em revalidação e sua superfície de
compatibilidade SQLite é distinta. Continuidade por si só não é autoridade.

### `mattn/go-sqlite3`

Não escolhido porque introduz requisito de CGO no runtime/build normal sem benefício
necessário para o escopo atual.

### ORM

Rejeitado por falta de necessidade. SQL explícito + repositories estreitos mantém o
contrato mais legível e testável.

### Múltiplos bancos por domínio desde o início

Não justificado. Um SQLite local para estado não secreto é suficiente; separar segredos
não implica fragmentar todo o domínio em múltiplos databases.

## Trade-offs e riscos

### Benefícios

- contrato mais próximo de SQLite padrão;
- portabilidade sem CGO preservada;
- durabilidade explicitamente testada;
- Evidence e state podem participar de uma arquitetura simples e auditável;
- migração side-by-side reduz risco sobre o banco histórico.

### Riscos

- `ncruces/go-sqlite3` continua uma dependência estrutural e deve ser pinada/testada;
- sua implementação VFS/Wasm possui características próprias de locking e memória;
- cada conexão carrega custo próprio, reforçando a baseline de uma conexão inicialmente;
- WAL e filesystem precisam ser revalidados nos ambientes de deployment suportados;
- não houve fault injection de power-loss real.

## Gates antes de produção

1. Decision explícita sobre engine + PRAGMAs + authority de migrations;
2. decidir o tipo físico do `EvidenceRecord.id`;
3. fechar schema físico mínimo de Evidence e índices essenciais;
4. integrar storage com os contratos do ADR 017 e preservar os tests de recovery;
5. testar migration/import side-by-side em cópia real do legado;
6. testar backup/restore do schema final;
7. `CGO_ENABLED=0 go test` e `go test -race` em gates separados;
8. revalidar WAL/locking em cada plataforma oficialmente suportada antes de declará-la
   suportada.

## Recomendação

Criar um ADR de storage que escolha **ncruces/go-sqlite3 + SQLite local conservador** como
baseline do novo banco e fixe apenas os contratos necessários agora.

O schema completo do produto, migração detalhada, session storage e tuning permanecem
separados para evitar um ADR monolítico.
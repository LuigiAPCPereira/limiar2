# EXP-LIMIAR-004 — Identidade e envelope físico mínimo de Evidence

Authority: Non-authoritative
Status: In progress

## Hipótese

Um identificador aleatório de 128 bits no formato UUIDv4, persistido como `BLOB(16)`,
pode fornecer identidade estável para Evidence sem depender de `rowid`, posição local,
timestamp ou hash do payload.

Em conjunto, um envelope SQLite `STRICT` com timestamps inteiros e payload opaco pode
preservar os facts mínimos necessários para export/import e reprocessamento sem
transformar ordem física em identidade de domínio.

## Origem

Este experimento responde diretamente ao
`F-STO-004-legacy-runid-is-not-evidence-identity.md`.

Ele também exercita um gate ainda aberto do ADR 019, que permanece `Proposed`: decidir
o tipo físico de `EvidenceRecord.id` e validar o envelope mínimo antes de qualquer
schema canônico de produção.

## Escopo

O harness avalia somente:

- UUIDv4 gerado a partir de `crypto/rand`, armazenado como 16 bytes;
- falha fechada quando a fonte de entropia falha;
- `BLOB` de payload e SHA-256 preservados byte a byte;
- timestamps Unix em milissegundos como `INTEGER`, separados da identidade;
- múltiplas Evidence com o mesmo payload e o mesmo timestamp;
- export/import entre dois bancos sem alterar a identidade;
- constraints físicas de tamanho para ID e hash;
- abertura, fechamento, reabertura e `PRAGMA integrity_check`;
- viabilidade funcional de `STRICT, WITHOUT ROWID`.

O harness não altera código, schema, dependências, defaults ou CI de produção.

## Setup

Arquivo executável:

`experiments/evidence-envelope/evidence_envelope_test.go`

Módulo isolado:

`experiments/evidence-envelope/go.mod`

Dependência pinada:

`github.com/ncruces/go-sqlite3 v0.35.3`

Baseline exercitada:

```text
SetMaxOpenConns(1)
SetMaxIdleConns(1)
PRAGMA journal_mode=WAL
PRAGMA synchronous=FULL
PRAGMA foreign_keys=ON
PRAGMA busy_timeout=5000
```

Comandos de reprodução:

```sh
cd experiments/evidence-envelope
go vet ./...
CGO_ENABLED=0 go test ./... -count=1 -v
go test -race ./... -count=1
```

## Fontes primárias

- RFC 9562, seção 5.4: UUIDv4 usa 122 bits aleatórios após fixar versão e variant:
  <https://www.rfc-editor.org/rfc/rfc9562.html#name-uuid-version-4>;
- SQLite — STRICT Tables:
  <https://www.sqlite.org/stricttables.html>;
- SQLite — WITHOUT ROWID:
  <https://www.sqlite.org/withoutrowid.html>;
- documentação do driver pinado:
  <https://pkg.go.dev/github.com/ncruces/go-sqlite3@v0.35.3>.

A documentação oficial do SQLite classifica `WITHOUT ROWID` como otimização, não como
nova capability, e recomenda medir o efeito em tabelas com linhas grandes. Como Evidence
pode carregar payloads BLOB substanciais, a simples viabilidade funcional dessa opção não
será interpretada como recomendação de produção.

## Critérios

A hipótese principal será suportada se:

1. 100.000 IDs gerados tiverem shape UUIDv4 válido e nenhuma colisão observada;
2. falha de entropia retornar erro sem produzir identidade utilizável;
3. ID, payload e hash sobreviverem byte a byte ao round-trip;
4. export/import de 1.000 registros preservar IDs e hashes;
5. 100 observações com payload e timestamp iguais coexistirem como Evidence distintas;
6. o schema recusar ID de 15 bytes e hash de 31 bytes;
7. `rowid` não estiver disponível na tabela experimental;
8. o banco reabrir com `integrity_check = ok`;
9. `go vet`, runtime sem CGO e race detector passarem.

A hipótese ficará inconclusiva se o contrato funcional passar, mas o resultado não for
suficiente para escolher entre UUIDv4 e uma identidade temporal, ou entre rowid table e
`WITHOUT ROWID`.

## Resultado

Pendente de execução reproduzível no GitHub Actions.

## Conclusão

Pendente.

## Limitações

Mesmo em caso de sucesso, este experimento não prova:

- ausência matemática de colisões; a amostra somente procura falhas observáveis;
- superioridade de UUIDv4 sobre UUIDv7, ULID ou outro ID estável;
- benefício de desempenho ou tamanho de `WITHOUT ROWID` com payloads reais;
- schema SQL final de produção;
- enforcement físico ou capability-level de append-only;
- índices finais, query patterns ou throughput;
- codec final do payload Telegram;
- ordering entre Evidence e `SourceSyncState`/`BackfillProgress`;
- migration/import do banco legado real;
- comportamento em ARM64, Android/Termux/PRoot ou power-loss durante `fsync`.

Resultado `Supported` não equivale a `Accepted`. Qualquer schema canônico continua
dependente de Proposal e ADR explícitos.

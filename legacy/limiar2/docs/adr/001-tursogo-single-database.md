# ADR 001 — Tursogo como Único Banco de Dados

## Status

Aceito

## Contexto

O `limiar-collector` deve persistir vários tipos de estado: a sessão MTProto, um
cache de peer/access-hash, a lista de canais monitorados e os payloads brutos das mensagens.
Queríamos um armazenamento de dados embutido (um único arquivo `.db` montável como um volume
persistente) em vez de lidar com múltiplos armazenamentos, e queríamos um binário portável
sem exigência de toolchain C.

O `turso.tech/database/tursogo` fornece um banco de dados embutido acessível através
da interface padrão `database/sql` sob o nome de driver `turso`, usando
`purego` para FFI — portanto, **sem CGO**.

## Decisão

Usar o Tursogo como o único datastore para tudo: `sessions`, `peers`,
`channels`, e `raw_messages` todos vivem em um arquivo `.db`
(`LIMIAR_DB_PATH`, padrão `./limiar.db`). Ele é acessado apenas através
de `database/sql` de `internal/storage`, com placeholders (marcadores) `?` e prepared statements
(instruções preparadas). O driver é registrado com um blank import
(`_ "turso.tech/database/tursogo"`) em `storage/db.go`.

## Consequências

- Um arquivo para backup, montagem e para raciocinar; sessão + cache de peers + canais +
  mensagens compartilham um armazenamento transacional.
- Nenhuma toolchain C, sem `CGO_ENABLED`; o binário é portável.
- A camada de storage (armazenamento) permanece na superfície portável do `database/sql` (`Exec`,
  `Query`, `QueryRow`, `Prepare`), o que a mantém testável.
- Aceitamos uma dependência da compatibilidade do Tursogo com a semântica padrão
  do `database/sql`; a camada de storage evita extensões específicas do driver para limitar o risco.

## Alternativas consideradas

- **SQLite via `mattn/go-sqlite3`** — requer CGO, anulando o objetivo de binário
  portável. Explicitamente proibido pelos requisitos.
- **Um ORM (GORM)** — adiciona uma abstração pesada, esconde o SQL e (no caso do
  storage do GoTGProto) acopla ao SQLite. Proibido.
- **Armazenamentos múltiplos** (ex: sessão baseada em arquivo + DB separado) — mais partes
  móveis, sem coesão transacional, operações mais difíceis.

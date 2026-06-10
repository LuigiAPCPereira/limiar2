# Spec — Uso do Tursogo

`turso.tech/database/tursogo` é o banco de dados embutido (embedded) que o
`limiar-collector` usa para qualquer tipo de estado persistido: a sessão MTProto,
o cache de peers, a lista de canais monitorados e os payloads brutos de
mensagens — tudo em um único arquivo `.db`. Ele é acessado exclusivamente através
do `database/sql`, e apenas a partir de `internal/storage`.

## Como o Tursogo é usado

### Registro do Driver (blank import)

O driver se registra sob o nome `turso` via um "blank import" (import em branco) em
`internal/storage/db.go`:

```go
import (
    "database/sql"
    _ "turso.tech/database/tursogo" // registra o driver "turso"
)

const driverName = "turso"
```

### Abrindo o banco de dados (`db.go`)

`storage.Open(ctx, dbPath)` abre a conexão usando a biblioteca padrão,
verifica-a com `PingContext`, e aplica as migrations:

```go
conn, err := sql.Open(driverName, dbPath)   // dbPath padrão: ./limiar.db
if err := conn.PingContext(ctx); err != nil { ... }
if err := migrate(ctx, conn); err != nil { ... }
```

`DB.Conn()` expõe o `*sql.DB` subjacente; apenas a goroutine DBWriter emite
gravações através dele. `DB.Close()` fecha a conexão.

### Migrations (`migrations.go`)

O schema reside em `internal/storage/migrations/*.sql`, embutido com
`//go:embed migrations/*.sql` em um `embed.FS`. `migrate` lê o diretório,
ordena os nomes dos arquivos de forma lexical e executa cada arquivo com `db.ExecContext`.
O SQL é idempotente (`CREATE TABLE IF NOT EXISTS`), de modo que reexecutar é seguro —
não há uma tabela de versão separada na Fase 1.

### Consultas (`repository.go`)

Todo o SQL é centralizado no `Repository`. Ele usa apenas a superfície padrão do `database/sql`
— `ExecContext`, `QueryContext`, `QueryRowContext`, `Prepare` — com
`?` como o único token marcador de parâmetros (placeholder). Gravações recorrentes
são instruções preparadas (prepared statements):

```go
stmtSaveMessage // INSERT INTO raw_messages (...) VALUES (?,?,?,?,?) ON CONFLICT(channel_id, message_id) DO NOTHING
stmtSavePeer    // INSERT INTO peers (...) VALUES (?,?,?,?,?) ON CONFLICT(id) DO UPDATE SET ...
```

`SaveSession` faz upsert da única linha de sessão (`ON CONFLICT(id) DO UPDATE`);
`LoadSession` mapeia `sql.ErrNoRows` para `ErrNoSession`. As operações de CRUD de
canal e peer utilizam os mesmos padrões. Datetimes são armazenadas como texto UTC
no layout `2006-01-02 15:04:05` para corresponder aos padrões `datetime('now')` do schema.

### Sem CGO

O Tursogo usa `purego` para FFI, para que o binário compile e execute sem uma toolchain
C e sem `CGO_ENABLED=1`. Nenhuma variável de ambiente especial é necessária; o binário é
portável.

## O que evitar

- **Sem driver SQLite, sem GORM, sem ORM.** O build não deve importar `sqlite`,
  `mattn` ou `gorm`. O Tursogo é o único datastore (armazenamento de dados) — veja o ADR 001.
- **Sem escritas concorrentes no `*sql.DB`.** Apenas a goroutine única `Collector.dbWriter`
  grava (fan-in). Leituras a partir de outras goroutines estão liberadas; as escritas
  são serializadas por meio do writer. Veja o ADR 003.
- **Nenhum estilo de placeholder (marcador) exceto `?`.** Não use estilos como `$1` ou `:name`.
- **Sem SQL puro fora de `repository.go`.** Outros pacotes chamam métodos de `Repository`;
  eles nunca tocam no `database/sql` diretamente.
- **Mantenha-se na superfície padrão do `database/sql`.** Use `Query`, `Exec`,
  `QueryRow`, e `Prepare`; evite extensões específicas do driver para que a camada
  permaneça portável e testável.

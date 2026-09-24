# Diretrizes — Estendendo a Camada Storage (Armazenamento)

A camada de armazenamento (`internal/storage`) é a dona de **toda** a persistência
contra o banco de dados Tursogo embutido. Toda instrução SQL reside aqui; nenhum outro pacote
toca no `database/sql`. Siga estas regras ao estender esta camada.

## Regras

1. **Todo SQL reside em `internal/storage`.** Use `repository.go` para collector/sessão/peers/dashboard raw, `processor_repository.go` para processor, mensagens processadas e `photo_cache`, e `migrations.go`/`migrations/*.sql` apenas para criação ou evolução de schema.
   Nunca escreva SQL nas camadas `telegram`, `collector`, `processor`, `dashboard` ou `cli`.
2. **Os marcadores de posição (placeholders) são apenas `?`.** Nunca use `$1` ou placeholders nomeados.
3. **Escritas recorrentes usam prepared statements** criadas nos construtores (`NewRepository`, `NewProcessorRepository`) e fechadas no `Close`.
4. **`context.Context` é o primeiro argumento** de todo método de consulta; use as
   variantes `...Context` (`ExecContext`, `QueryContext`, `QueryRowContext`).
5. **Encapsule (Wrap) os erros** com `apperrors.Wrap("storage", "<op>", err)` e mapeie
   casos de "linha faltando" para um sentinel (ex: `sql.ErrNoRows` → `ErrNoSession` ou
   `apperrors.ErrChannelNotFound`).
6. **Escrita em runtime respeita o dono da camada.** `raw_messages` é escrito apenas pelo `Collector.dbWriter`; `processed_messages` é escrito apenas pelo processor via `ProcessorRepository`; dashboard é read-only e não dispara writes ou downloads MTProto.
7. **Alterações de schema usam a estratégia consolidada + reparo (não ALTER TABLE direto).**
   O Turso/Tursogo rejeita `ALTER TABLE ADD COLUMN` em banco que já possui a coluna
   (erro `duplicate column name`). Como o `001_initial.sql` é o schema consolidado completo
   para bancos novos, qualquer `ALTER TABLE` em uma migration `NNN_*.sql` posterior causaria
   falha em banco novo. Por isso, o projeto adota três camadas:

   - **Schema consolidado** (`001_initial.sql`): toda coluna nova é adicionada aqui. Bancos
     novos nascem completos.
   - **Reparo condicional** (`currentSchemaColumns` em `migrations.go`): para bancos legados,
     o Go checa via `PRAGMA table_info` quais colunas faltam e adiciona com `ALTER TABLE ADD
     COLUMN` apenas as ausentes. É idempotente.
   - **Marcador SQL** (`NNN_*.sql`): cada alteração de schema cria um arquivo de migration que
     serve como marcador — tipicamente apenas `CREATE TABLE IF NOT EXISTS schema_migrations`.
     Isso registra a versão na tabela de controle sem tentar `ALTER TABLE` que falharia.

   Nunca escreva `ALTER TABLE ADD COLUMN` direto num arquivo `NNN_*.sql` se a coluna também
   foi adicionada ao `001_initial.sql`. Registre a coluna em `currentSchemaColumns` e no
   `001_initial.sql`, e deixe o `NNN_*.sql` como marcador. Ver exemplos em `007_legacy_schema_compat.sql`
   e `008_extraction_enhancements.sql`.
8. **As datas e horas (Datetimes) são texto em UTC** no formato `2006-01-02 15:04:05` (a constante `storage.DBTimeLayout`), correspondendo aos padrões `datetime('now')` do schema.

## Correto

```go
// Nova query: centralizada, placeholders '?', context-first, erro encapsulado (wrapped).
func (r *Repository) GetChannelByUsername(ctx context.Context, username string) (*Channel, error) {
    row := r.db.QueryRowContext(ctx, `
        SELECT id, username, title, active, added_at, last_message_id, last_collected_at
        FROM channels WHERE username = ?`, username)
    ch, err := scanChannel(row)
    if stderrors.Is(err, sql.ErrNoRows) {
        return nil, apperrors.Wrap("storage", "get_channel_by_username", apperrors.ErrChannelNotFound)
    }
    if err != nil {
        return nil, err
    }
    return ch, nil
}
```

```go
// Escrita recorrente: preparada no NewRepository, fechada no Close.
saveMsg, err := db.Prepare(`
    INSERT INTO raw_messages (channel_id, message_id, payload, received_at, schema_version)
    VALUES (?, ?, ?, ?, ?)
    ON CONFLICT(channel_id, message_id) DO NOTHING`)
```

## Incorreto

```go
// ERRADO: SQL fora do repository.go, na camada collector ou telegram.
rows, _ := someDB.Query("SELECT * FROM channels")   // nunca faça isso aqui
```

```go
// ERRADO: placeholder diferente de '?'.
r.db.ExecContext(ctx, `INSERT INTO peers (id) VALUES ($1)`, id)
```

```go
// ERRADO: importando um driver SQLite/ORM.
import _ "github.com/mattn/go-sqlite3"
import "gorm.io/gorm"
```

```go
// ERRADO: uma segunda goroutine escrevendo concorrentemente no *sql.DB junto ao DBWriter.
go func() { repo.SaveRawMessage(ctx, msg) }()   // quebra o invariante single-writer
```

## O que nunca fazer

- Adicionar dependência do SQLite, `mattn`, GORM, ou qualquer ORM (ADR 001).
- Escrever no `*sql.DB` fora de `internal/storage`.
- Fazer o dashboard escrever no banco ou disparar download MTProto sob demanda.
- Vazar tipos do `database/sql` ou SQL puro para fora de `internal/storage`.
- Introduzir funções `init()` ou estado mutável a nível de pacote.

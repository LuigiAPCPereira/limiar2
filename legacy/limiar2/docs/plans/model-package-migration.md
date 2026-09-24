# Plano de Implementação — ADR 013: `internal/model/`

**Autor:** Auditoria de código 2026-06-21
**Status:** Não iniciado
**Referência:** [ADR 013](../adr/013-model-package.md)

---

## Visão geral

```
Antes:
  storage ← processor (importa storage por RawMessage, BoolToInt, ParseDBTime)
  storage ← dashboard (importa storage por ProcessedMessage, queries)
  storage ← collector
  storage ← media (interface satisfeita por *storage.Repository)
  storage ← cli/media

Depois:
  model ← storage, processor, dashboard, collector, media, cli
  (sem ciclos — model é folha, só importa stdlib)
```

---

## Wave 1 — Criar `internal/model/` com type aliases (≈ 30 min)

### 1.1 Criar arquivos do pacote

**`internal/model/model.go`** — doc + imports:
```go
// Package model define os tipos de domínio compartilhados do pipeline Limiar.
// Não contém lógica de banco de dados, SQL, ou dependências de outros
// pacotes do projeto — apenas tipos puros e helpers de conversão.
package model
```

**`internal/model/raw.go`** — `RawMessage`, `Channel`, `Peer`, `ChannelStats`:
```go
// RawMessage é um payload de mensagem capturada do Telegram...
type RawMessage struct { ID, ChannelID, MessageID int64; Payload []byte; ... }
// Channel é um canal monitorado do Telegram...
type Channel struct { ID int64; Username, Title string; Active bool; ... }
// Peer é um peer do Telegram armazenado em cache...
type Peer struct { ID, AccessHash int64; Type, Username string; ... }
// ChannelStats mantém contagem de mensagens para um canal...
type ChannelStats struct { ChannelID int64; Username string; MessageCount int64 }
```

**`internal/model/processed.go`** — `ProcessedMessage`, `ProcessedTypeStats`:
```go
// ProcessedMessage representa uma mensagem normalizada e classificada...
type ProcessedMessage struct { ... } // 32 campos com json tags
// ProcessedTypeStats contém contagem por message_type...
type ProcessedTypeStats struct { MessageType string; Count int64 }
```

**`internal/model/media.go`** — `PhotoMetadata`, `PhotoStats`:
```go
// PhotoMetadata contém campos MTProto para download de imagem...
type PhotoMetadata struct { ID, MsgID, ChannelID, PhotoID, AccessHash int64; ... }
// PhotoStats resume cobertura de metadados MTProto...
type PhotoStats struct { TotalProcessed, WithPhoto, CompleteMTProto int64 }
```

**`internal/model/time.go`** — `DBTimeLayout`, `ParseDBTime`:
```go
const DBTimeLayout = "2006-01-02 15:04:05"
func ParseDBTime(s string) time.Time { ... }
```

**`internal/model/convert.go`** — `BoolToInt`:
```go
func BoolToInt(b bool) int { if b { return 1 }; return 0 }
```

### 1.2 Criar type aliases em `storage`

**`internal/storage/aliases.go`**:
```go
package storage
import "github.com/limiar/collector/internal/model"

// Type aliases — preservam compatibilidade durante migração (ADR 013 Wave 1).
// Serão removidos na Wave 2 após todos os callers migrarem para model.*.
type RawMessage = model.RawMessage
type Channel = model.Channel
type Peer = model.Peer
type ChannelStats = model.ChannelStats
type ProcessedMessage = model.ProcessedMessage
type ProcessedTypeStats = model.ProcessedTypeStats
type PhotoMetadata = model.PhotoMetadata
type PhotoStats = model.PhotoStats

const DBTimeLayout = model.DBTimeLayout
var ParseDBTime = model.ParseDBTime
var BoolToInt = model.BoolToInt
```

### 1.3 Remover definições originais de `storage/repository.go`

Após criar os aliases, remover as definições duplicadas originais:
- Tipos `RawMessage`, `Channel`, `Peer`, `ChannelStats`
- Tipos `ProcessedMessage`, `ProcessedTypeStats`
- Tipos `PhotoMetadata`, `PhotoStats`
- `DBTimeLayout`, `ParseDBTime`, `BoolToInt`

Manter em `storage`:
- `Repository` struct e todos os métodos (queries)
- `ErrNoSession` (pode migrar depois)
- `scanner` interface e helpers internos (`scanChannel`, `scanMessage`, `nullString`)

### 1.4 Verificação Wave 1

```bash
go build ./...      # deve passar com zero alterações nos callers
go vet ./...        # limpo
go test ./...       # todos passam
grep -r "model\." internal/model/  # model não importa nada do projeto
```

---

## Wave 2 — Migrar callers para `model.*` (≈ 45 min)

### 2.1 Atualizar imports — tabela de mudanças

| Arquivo | De | Para |
|---|---|---|
| `internal/processor/repository.go` | `"github.com/limiar/collector/internal/storage"` (para `RawMessage`, `ParseDBTime`, `BoolToInt`) | Adicionar `"github.com/limiar/collector/internal/model"` |
| `internal/processor/normalizer.go` | `storage.RawMessage` | `model.RawMessage` |
| `internal/processor/classify.go` | (referencia `NormalizedMessage`) | Sem mudança |
| `internal/processor/processor.go` | `storage.RawMessage` (via repository) | `model.RawMessage` |
| `internal/collector/collector.go` | `storage.Channel`, `storage.RawMessage` | `model.Channel`, `model.RawMessage` |
| `internal/collector/backfill.go` | `storage.Channel`, `storage.RawMessage` | `model.Channel`, `model.RawMessage` |
| `internal/collector/dbwriter.go` | `storage.RawMessage` | `model.RawMessage` |
| `internal/collector/handler.go` | `storage.RawMessage` | `model.RawMessage` |
| `internal/dashboard/server.go` | `storage.ProcessedMessage`, `storage.ChannelStats`, `storage.ProcessedTypeStats` | `model.ProcessedMessage`, etc. |
| `internal/telegram/peers.go` | `storage.Peer` | `model.Peer` |
| `internal/media/resolver.go` | (já usa `media.Repository`, não `storage`) | Sem mudança |
| `internal/cli/media.go` | `storage.PhotoMetadata`, `storage.PhotoStats` | `model.PhotoMetadata`, `model.PhotoStats` |
| `cmd/limiar-collector/main.go` | `storage.Channel` (indireto) | Verificar |
| `cmd/limiar/main.go` | (usa `storage.RawMessage` no callback) | `model.RawMessage` |
| Testes: `*_test.go` | Referências a `storage.*` | `model.*` |

### 2.2 Remover type aliases

```bash
rm internal/storage/aliases.go
```

### 2.3 Limpar imports não usados

Após migração, cada pacote pode ter imports de `storage` que não são mais
necessários. Remover.

### 2.4 Verificação Wave 2

```bash
go build ./...      # zero erros
go vet ./...        # zero warnings  
go test ./...       # todos passam
go test -race ./... # sem data races
grep -r "storage\.RawMessage\|storage\.Channel\|storage\.ProcessedMessage" internal/ cmd/  # zero resultados
```

---

## Wave 3 — Separar queries de leitura de `processed_messages` (≈ 60 min)

### 3.1 Definir interface `ProcessedReader`

**`internal/processor/reader.go`** (novo):
```go
package processor

import (
    "context"
    "github.com/limiar/collector/internal/model"
)

// ProcessedReader é a interface de leitura de mensagens processadas,
// satisfeita por *Repository. Usada pelo dashboard e CLI de mídia
// para evitar dependência em storage.Repository.
type ProcessedReader interface {
    ListProcessedMessages(ctx context.Context, channelID int64, msgType string, limit, offset int) ([]*model.ProcessedMessage, error)
    CountProcessedByType(ctx context.Context) ([]model.ProcessedTypeStats, error)
    CountProcessedMessages(ctx context.Context) (int64, error)
    GetPhotoMetadata(ctx context.Context, processedMsgID int64) (*model.PhotoMetadata, error)
    GetPhotoID(ctx context.Context, processedMsgID int64) (int64, error)
    GetInlineThumb(ctx context.Context, photoID int64) ([]byte, error)
    PhotoMetadataStats(ctx context.Context) (model.PhotoStats, error)
}
```

### 3.2 Mover queries de `storage.Repository` → `processor.Repository`

Mover implementações:
- `GetPhotoMetadata`
- `GetPhotoID`
- `GetInlineThumb`
- `PhotoMetadataStats`
- `ListProcessedMessages`
- `CountProcessedByType`
- `CountProcessedMessages`

De `internal/storage/repository.go` para `internal/processor/repository.go`.

Atualizar `processor.Repository.stmtInsertProcessed` — adicionar prepared statements
para queries frequentes se necessário.

### 3.3 Atualizar `dashboard.Server`

**Antes:**
```go
type Server struct {
    repo *storage.Repository
    // ...
}
func NewServer(repo *storage.Repository, ...) *Server
```

**Depois:**
```go
type Server struct {
    repo      dashboardRepo  // raw_messages queries
    processed processor.ProcessedReader  // processed_messages queries
    // ...
}

type dashboardRepo interface {
    ListChannels(ctx context.Context) ([]*model.Channel, error)
    ListMessages(ctx context.Context, channelID int64, limit, offset int) ([]*model.RawMessage, error)
    GetMessageByID(ctx context.Context, id int64) (*model.RawMessage, error)
    CountMessagesByChannel(ctx context.Context) ([]model.ChannelStats, error)
}
```

`storage.Repository` satisfaz `dashboardRepo` estruturalmente.

### 3.4 Atualizar orquestrador e binários standalone

**`cmd/limiar/main.go`:**
```go
// Antes
dashRepo, _ := storage.NewRepository(db.DB())
srv := dashboard.NewServer(dashRepo, log, 8080, broker)

// Depois
dashRepo, _ := storage.NewRepository(db.DB())
srv := dashboard.NewServer(dashRepo, procRepo, log, 8080, broker)
```

**`cmd/limiar-collector/main.go`** (comando `dashboard`):
Não tem acesso a `processor.Repository` — precisa criar um para queries de
processados, ou o dashboard standalone mostra só raw_messages (comportamento
atual do comando `dashboard` standalone).

**Decisão:** O dashboard standalone (`limiar-collector dashboard`) continua
mostrando apenas raw_messages. A flag `--dashboard` no `run` e no orquestrador
mostra ambas.

### 3.5 Atualizar `internal/media/resolver_test.go`

`fakeRepo` implementa `media.Repository`. Após Wave 3, `storage.Repository`
não satisfaz mais `media.Repository` diretamente — quem satisfaz é
`processor.Repository`. O teste precisa de um fake separado ou usar
`processor.Repository`.

### 3.6 Verificação Wave 3

```bash
go build ./...      # zero erros
go vet ./...        # limpo
go test ./...       # todos passam
go test -race ./... # sem data races

# Verificações estruturais:
grep -r "processed_messages" internal/storage/  # deve retornar vazio
grep "storage\." internal/dashboard/             # dashboard não importa mais storage
grep "storage\." internal/media/                 # media não importa storage
```

---

## Resumo de arquivos afetados

| Wave | Arquivos criados | Arquivos modificados | Arquivos deletados |
|---|---|---|---|
| 1 | `internal/model/*.go` (6 arquivos) | `internal/storage/repository.go` (remoção de tipos) | — |
| 1 | `internal/storage/aliases.go` | — | — |
| 2 | — | ~15 arquivos (migração de imports) | `internal/storage/aliases.go` |
| 3 | `internal/processor/reader.go` | `internal/processor/repository.go` (+7 métodos) | — |
| 3 | — | `internal/dashboard/server.go` (nova API) | — |
| 3 | — | `cmd/limiar/main.go` (wiring) | — |
| 3 | — | `internal/storage/repository.go` (-7 métodos) | — |

## Ordem de execução

```
Wave 1 ──▶ Wave 2 ──▶ Wave 3
         (cada wave é atômica: build + testes passam ao final)
```

## Rollback

Cada wave é reversível independentemente:
- Wave 1: `git revert` — type aliases são inofensivos
- Wave 2: `git revert` — callers voltam a usar `storage.*`
- Wave 3: `git revert` — queries voltam para `storage`

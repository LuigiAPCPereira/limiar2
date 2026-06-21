# Spec: Pacote `internal/model/` para Tipos de Domínio Compartilhados

**Data:** 2026-06-21
**ADR:** [013-model-package](../adr/013-model-package.md)
**Status:** Design aprovado, aguardando implementação

---

## 1. Visão geral

Criar `internal/model/` como pacote folha (leaf package) contendo exclusivamente tipos de
domínio puros e helpers de conversão — zero lógica de banco de dados, zero SQL, zero
dependências de outros pacotes do projeto.

O pacote elimina o acoplamento atual onde `storage`, `processor`, `dashboard`, `media`
e `cli` compartilham tipos definidos em `internal/storage/` — pacote cuja responsabilidade
primária é persistência do collector, não definição de modelo de domínio.

---

## 2. Arquitetura alvo

```
Antes:
  storage (define RawMessage, ProcessedMessage, PhotoMetadata, helpers...)
    ↑
    ├── processor (importa storage por tipos + ParseDBTime + BoolToInt)
    ├── dashboard (importa storage por ProcessedMessage, queries)
    ├── collector (importa storage por RawMessage, Channel, Peer)
    ├── media     (satisfaz media.Repository via *storage.Repository)
    └── cli/media (importa storage por PhotoStats, GetPhotoMetadata)

Depois:
  model (RawMessage, ProcessedMessage, PhotoMetadata, helpers)
    ↑
    ├── storage    (usa model.* nos métodos; NÃO define tipos)
    ├── processor  (usa model.*; NÃO importa storage para tipos)
    ├── dashboard  (usa model.*)
    ├── collector  (usa model.*)
    ├── media      (interface usa model.*; satisfeita por processor.Repository)
    └── cli/media  (usa model.*)
```

**Invariante:** `model` importa apenas stdlib. Nenhum pacote do projeto importa `model`
transitivamente através de outro — `model` é sempre dependência direta.

---

## 3. Componentes

### 3.1 `internal/model/` — 6 arquivos

| Arquivo | Conteúdo |
|---|---|
| `model.go` | Package doc |
| `raw.go` | `RawMessage`, `Channel`, `Peer`, `ChannelStats` |
| `processed.go` | `ProcessedMessage` (32 campos + json tags), `ProcessedTypeStats` |
| `media.go` | `PhotoMetadata`, `PhotoStats` |
| `time.go` | `DBTimeLayout`, `ParseDBTime` |
| `convert.go` | `BoolToInt` |

Nenhum destes arquivos contém:
- Métodos de banco de dados
- SQL
- Imports de outros pacotes do Limiar
- Lógica de negócio (validação, classificação, etc.)

### 3.2 `internal/storage/aliases.go` — temporário (Wave 1 apenas)

Type aliases que preservam compatibilidade durante a migração:

```go
type RawMessage = model.RawMessage
type Channel = model.Channel
// ... etc
```

Removido na Wave 2 após todos os callers migrarem.

### 3.3 `internal/processor/reader.go` — novo na Wave 3

Interface `ProcessedReader` para o dashboard consumir queries de processados sem
depender de `storage`:

```go
type ProcessedReader interface {
    ListProcessedMessages(...)
    CountProcessedByType(...)
    CountProcessedMessages(...)
    GetPhotoMetadata(...)
    GetPhotoID(...)
    GetInlineThumb(...)
    PhotoMetadataStats(...)
}
```

`processor.Repository` satisfaz esta interface.

### 3.4 `dashboard.Server` — API atualizada

**Antes:**
```go
func NewServer(repo *storage.Repository, log logger.Logger, port int, broker *Broker) *Server
```

**Depois:**
```go
func NewServer(repo dashboardRepo, processed processor.ProcessedReader, log logger.Logger, port int, broker *Broker) *Server
```

Onde `dashboardRepo` é uma interface local com os 4 métodos de leitura de raw_messages
que o dashboard precisa. `storage.Repository` a satisfaz estruturalmente.

---

## 4. Fluxo de dados

Nenhuma mudança no fluxo de dados em runtime. Apenas a organização do código-fonte muda.

```
Runtime (inalterado):
  Collector ──Write──▶ raw_messages
  Processor ──Read──▶ raw_messages ──Write──▶ processed_messages
  Dashboard ──Read──▶ raw_messages + processed_messages
  Media ──Read──▶ processed_messages (inline_thumb, photo_id)
```

---

## 5. Estratégia de migração (3 waves)

### Wave 1 — Tipos e aliases (sem quebra)

1. Criar `internal/model/*.go` com todos os tipos e helpers
2. Criar `internal/storage/aliases.go` com type aliases
3. Remover definições originais de `storage/repository.go`
4. **Verificação:** `go build ./...` passa sem alterar nenhum caller

### Wave 2 — Migrar callers (~15 arquivos)

1. Atualizar imports em `processor/`, `collector/`, `dashboard/`, `media/`, `cli/`, `cmd/`
2. Remover `internal/storage/aliases.go`
3. Remover imports não usados
4. **Verificação:** `grep -r "storage\.RawMessage\|storage\.Channel\|storage\.ProcessedMessage" internal/ cmd/` retorna vazio

### Wave 3 — Separar queries de leitura

1. Criar `internal/processor/reader.go` com interface `ProcessedReader`
2. Mover 7 métodos de `storage.Repository` → `processor.Repository`
3. Atualizar `dashboard.Server` para usar `ProcessedReader`
4. Atualizar `cmd/limiar/main.go` (orquestrador)
5. Atualizar `cmd/limiar-collector/main.go` (dashboard standalone: raw apenas)
6. **Verificação:** `grep -r "processed_messages" internal/storage/` retorna vazio

---

## 6. Edge cases

| Caso | Comportamento |
|---|---|
| Dashboard standalone (`limiar-collector dashboard`) | Mostra apenas raw_messages. Sem acesso a processed_messages (não tem processor.Repository). |
| Dashboard via orquestrador (`limiar run`) | Mostra raw_messages + processed_messages. Recebe ambos os repositórios. |
| `media.Resolver` com `processor.Repository` | `processor.Repository` satisfaz `media.Repository` (interface estrutural). Sem mudança no resolver. |
| Type alias vs IDE navegação | `gopls` segue aliases corretamente. Wave 1 é transparente para ferramentas. |
| Testes que criam `storage.RawMessage` diretamente | Atualizados na Wave 2 para usar `model.RawMessage`. |

---

## 7. O que NÃO faz parte deste spec

- Mover `ErrNoSession` para `model/` — é um erro de domínio do storage, não um tipo de dados
- Separar queries de escrita de `processed_messages` — já estão em `processor.Repository`
- Alterar o schema do banco ou migrações
- Adicionar novas funcionalidades
- Remover `storage.Repository.DB()` — fora do escopo, requer análise separada

---

## 8. Critérios de sucesso

- [ ] `internal/model/` importa apenas stdlib
- [ ] Nenhum ciclo de dependência (`go vet ./...` limpo)
- [ ] `go build ./...` passa em cada wave
- [ ] `go test ./...` passa em cada wave
- [ ] Wave 2: zero referências a `storage.RawMessage` fora de `storage/`
- [ ] Wave 3: `storage/` não contém queries em `processed_messages`
- [ ] Dashboard standalone continua funcional (raw_messages apenas)
- [ ] Orquestrador mostra raw + processed

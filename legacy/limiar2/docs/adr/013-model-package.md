# ADR 013 — Pacote `internal/model/` para Tipos de Domínio Compartilhados

## Status

Parcialmente superado pela implementação vigente. `internal/model` foi adotado,
mas o SQL do processor foi consolidado em `internal/storage/processor_repository.go`
para respeitar a regra atual de ownership físico do Turso (`internal/storage` contém
todo acesso `database/sql`). O pacote `internal/processor` expõe interfaces
(`Store`, `ProcessedReader`) e permanece sem SQL. As seções abaixo registram o
contexto histórico da decisão original.

## Contexto

O `internal/storage/repository.go` (762 linhas) centraliza todo SQL do projeto em um
único arquivo e struct. Isso funcionou bem nas Fases 1 e 2, mas criou um acoplamento
indesejado: tipos do domínio do processor (`ProcessedMessage`, `PhotoMetadata`,
`ProcessedTypeStats`, `PhotoStats`) e queries correspondentes (`ListProcessedMessages`,
`GetPhotoID`, `GetInlineThumb`, `PhotoMetadataStats`, etc.) residem em
`internal/storage` — pacote cuja responsabilidade primária é a **persistência do
collector** (raw_messages, channels, peers, sessions).

A auditoria de código (2026-06-21) identificou os seguintes problemas:

1. **Sobreposição de responsabilidade**: `storage.Repository` expõe queries de
   `processed_messages` que o dashboard e a CLI de mídia consomem. O
   `processor.Repository` (em `internal/processor/repository.go`) já existe e
   contém as queries de escrita do processor (`FetchUnprocessed`, `SaveProcessed`,
   `SaveProcessedBatch`). As queries de leitura ficaram no lugar errado.

2. **Dependência implícita**: `storage` exporta `ParseDBTime` e `BoolToInt` —
   helpers usados tanto pelo collector quanto pelo processor. O processor importa
   `storage` apenas por esses helpers, criando uma dependência mais pesada que o
   necessário.

3. **Tipos de domínio no lugar errado**: `ProcessedMessage`, `PhotoMetadata`,
   `ProcessedTypeStats`, `PhotoStats` são tipos do domínio do processor/API, mas
   vivem em `storage`. Se um dia `processor` precisar definir seus próprios tipos
   derivados, haverá confusão sobre onde declarar.

4. **Código morto**: `GetChannelUsername`, `UpdateFileReference`, `UpdatePhotoMetadata`
   foram removidos na auditoria — eram métodos definidos em `storage.Repository`
   sem callers, ilustrando que a fronteira não estava clara.

### Por que não mover diretamente para `processor/`

Mover os tipos `ProcessedMessage`, `PhotoMetadata` etc. diretamente para
`internal/processor/` criaria um ciclo de dependência:

```
storage → processor  (para usar ProcessedMessage nos retornos de query)
processor → storage   (para usar RawMessage, DBTimeLayout, BoolToInt)
                        ↑ CICLO
```

A raiz do problema é que não existe um pacote neutro para tipos de domínio que
são compartilhados entre camadas.

## Constraints

- **AGENTS.md §11** (stack fechada): não adicionar dependências externas.
- **AGENTS.md §6** (fases): cada fase tem escopo fechado. O modelo de dados é
  compartilhado, mas as queries são específicas.
- **AGENTS.md §14.3**: dashboard é read-only, nunca escreve.
- **AGENTS.md §15.2**: processor lê raw_messages (read-only) e escreve em
  processed_messages.
- **Invariante 6**: entre duas soluções corretas, menos componentes vence.
- **Go**: zero import cycles. Pacotes devem formar um DAG.

## Alternativas consideradas

### 1. Criar `internal/model/` — pacote de tipos de domínio (ESCOLHIDA)

Criar `internal/model/` contendo apenas tipos de domínio puros (structs, constantes,
helpers de conversão). Nenhum método de banco de dados, nenhuma lógica de negócio.
`storage`, `processor`, `dashboard`, `media`, e `cli` importariam de `model/`.

**Prós:**
- Elimina o ciclo de dependência
- Tipos de domínio têm um lar óbvio e único
- `storage` e `processor` podem evoluir independentemente
- Facilita a migração futura das queries de leitura de `processed_messages`

**Contras:**
- Mais um pacote no projeto (viola minimamente o invariante 6)
- Requer atualização de imports em ~15 arquivos
- Churn de API: todos os callers de `storage.ProcessedMessage` passam a usar
  `model.ProcessedMessage`

### 2. Manter tudo em `storage` (status quo)

**Rejeitada**: a auditoria mostrou que a fronteira já está borrada o suficiente
para causar código morto, dependência implícita e confusão sobre propriedade.

### 3. Mover queries de leitura para `processor/` sem pacote de modelos

**Rejeitada**: ciclo de dependência `storage ↔ processor`.

### 4. Mover helpers (`ParseDBTime`, `BoolToInt`) para um subpacote `storage/timeutil`

**Rejeitada**: resolve só parte do problema. Não endereça os tipos de domínio.

## Decisão

Criar `internal/model/` como pacote de tipos de domínio compartilhados, em 3 waves:

### Wave 1 — Tipos e helpers (sem quebra de API)

Mover para `internal/model/`:
- `RawMessage`, `Channel`, `Peer` (domínio do collector)
- `ProcessedMessage`, `ProcessedTypeStats` (domínio do processor)
- `PhotoMetadata`, `PhotoStats` (domínio de mídia)
- `ChannelStats` (domínio do dashboard)
- `DBTimeLayout`, `ParseDBTime`, `BoolToInt` (helpers)
- `ErrNoSession` (erro de domínio do storage)

Cada tipo movido ganha um type alias no pacote original por 1 release para
evitar quebra de API:

```go
// internal/storage/aliases.go
type RawMessage = model.RawMessage
type Channel = model.Channel
// ...
```

Isso permite que callers existentes compilem sem alterações enquanto a migração
acontece gradualmente.

### Wave 2 — Migração de callers

Atualizar todos os callers para usar `model.` diretamente:
- `internal/processor/` — `storage.RawMessage` → `model.RawMessage`
- `internal/collector/` — `storage.Channel` → `model.Channel`
- `internal/dashboard/` — `storage.ProcessedMessage` → `model.ProcessedMessage`
- `internal/media/` — `storage.PhotoMetadata` → `model.PhotoMetadata`
- `internal/cli/` — `storage.PhotoStats` → `model.PhotoStats`
- `internal/telegram/` — `storage.Peer` → `model.Peer`
- `cmd/` — atualizar imports

Remover os type aliases de `internal/storage/aliases.go`.

### Wave 3 — Separar queries de leitura

Mover de `storage.Repository` para `processor.Repository`:
- `ListProcessedMessages`
- `CountProcessedByType`
- `CountProcessedMessages`
- `GetPhotoMetadata`
- `GetPhotoID`
- `GetInlineThumb`
- `PhotoMetadataStats`

Atualizar `dashboard.Server` para receber interface de leitura de processados:

```go
type ProcessedReader interface {
    ListProcessedMessages(...)
    CountProcessedByType(...)
    // ...
}
```

`processor.Repository` satisfaz `ProcessedReader`. O orquestrador passa
`procRepo` para o dashboard.

## Justificativa

A alternativa 1 (model/) é a única que respeita todas as constraints:
- Zero ciclos de dependência
- Stack fechada (nenhuma dependência externa nova)
- Preserva invariantes arquiteturais (fases, read-only dashboard, etc.)
- Minimiza complexidade: um pacote de tipos é a abstração mais simples possível

O invariante 6 ("menos componentes vence") é levemente tensionado por 1 pacote
novo, mas o tradeoff é justificado: 1 pacote de tipos elimina dependências
implícitas entre 4 pacotes existentes e previne código morto futuro.

## Verificação

### Wave 1
- [ ] `go build ./...` passa
- [ ] `go vet ./...` limpo
- [ ] Type aliases em `storage` permitem compilação sem alterar callers
- [ ] `internal/model/` não importa nenhum pacote do projeto (só stdlib)

### Wave 2
- [ ] Nenhum caller usa `storage.RawMessage` (usa `model.RawMessage`)
- [ ] Type aliases removidos sem erros de compilação
- [ ] Todos os testes passam

### Wave 3
- [ ] `processor.Repository` contém todas as queries de `processed_messages`
- [ ] `dashboard.Server` usa `ProcessedReader` em vez de `*storage.Repository`
- [ ] `storage.Repository` não referencia `processed_messages`
- [ ] Orquestrador (`cmd/limiar`) passa `procRepo` para o dashboard
- [ ] Binários standalone (`cmd/limiar-processor --dashboard`) continuam funcionando

## Dependências

- Nenhuma dependência externa nova
- `internal/model/` depende apenas de stdlib (`time`, `database/sql` para `NullInt64`)
- Todos os outros pacotes ganham `internal/model/` como dependência

## Riscos

- **Churn de API**: ~15 arquivos afetados. Mitigado pelos type aliases na Wave 1.
- **Confusão temporária**: type aliases podem confundir `gopls` e ferramentas de
  navegação. Dura apenas 1 release.
- **Orquestrador**: `cmd/limiar/main.go` cria 3 repositories hoje; após Wave 3,
  o dashboard recebe `processor.Repository`. Mudança localizada.

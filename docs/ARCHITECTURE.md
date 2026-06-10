# ARCHITECTURE — limiar-collector (Phase 1)

O `limiar-collector` foi projetado com uma arquitetura em camadas focada em desacoplamento rigoroso. A base de código é dividida horizontalmente: `internal/cli` hospeda o ciclo de vida dos comandos (Cobra), `internal/collector` orquestra a captura com a concorrência fan-out/fan-in, `internal/telegram` atua como a Fachada (Facade) rígida ocultando o gotd/td e o protocolo MTProto, e `internal/storage` isola o banco de dados Tursogo embutido e todas as instruções SQL.

Os padrões de design clássicos unem as camadas: **Facade** (`telegram.TelegramClient`), **Observer** (`telegram.UpdateHandler`, `telegram.Dispatcher`), **Strategy** (`collector.Classifier`) e **Adapter** (`collector.MessageHandler`) — com a injeção manual de dependências conectada apenas em `cmd/limiar-collector/main.go`.

## Diagrama do Pipeline

```
┌──────────────────────────────────────────────────────────────────────┐
│                  cmd/limiar-collector/main.go                          │
│        raiz de composição (composition root) — ÚNICO local de injeção  │
│        (provider implementa cli.Provider)                              │
└───────────────────────────────────┬────────────────────────────────────┘
                                     │ NewRootCmd(provider)
                 ┌───────────────────┼───────────────────┐
                 ▼                   ▼                   ▼
          ┌───────────┐      ┌──────────────┐     ┌───────────┐
          │ cli/auth  │      │ cli/channels │     │  cli/run  │
          │ (log texto)│     │  (log texto) │     │(log json) │
          └─────┬─────┘      └──────┬───────┘     └─────┬─────┘
                │                   │                   │
                └───────────────────┼───────────────────┘
                                    ▼
┌──────────────────────────────────────────────────────────────────────┐
│                         camada collector                               │
│  Collector (orquestrador)   MessageHandler (Adapter)                   │
│  - dbWriter goroutine        - []byte → storage.RawMessage             │
│  - backfill history          Classifier (Strategy → NoopClassifier)    │
└───────────────────────────────────┬────────────────────────────────────┘
                                     ▼
┌──────────────────────────────────────────────────────────────────────┐
│                    camada telegram (Facade)                            │
│  TelegramClient (interface)   Dispatcher (Observer)                    │
│  Client (impl, encapsula gotd) PeerStore (RWMutex)                     │
│  TursoSessionStorage          terminalAuthenticator (auth.UserAuth.)   │
│                          │                                             │
│                   ┌──────┴───────┐                                     │
│                   │  gotd/td     │  ← ÚNICO ponto de contato MTProto   │
│                   └──────────────┘                                     │
└───────────────────────────────────┬────────────────────────────────────┘
                                     ▼
┌──────────────────────────────────────────────────────────────────────┐
│                        camada storage                                  │
│  db.go (Open/Close/Conn)   migrations.go (embed.FS)                    │
│  repository.go (TODO SQL, placeholders ?, prepared statements)         │
│                          │                                             │
│                   ┌──────┴───────┐                                     │
│                   │ Tursogo "turso"│  database/sql, sem CGO            │
│                   │  ./limiar.db │                                     │
│                   └──────────────┘                                     │
└──────────────────────────────────────────────────────────────────────┘
┌──────────────────────────────────────────────────────────────────────┐
│  transversal: config (viper/env)  logger (slog)  errors (wrap)         │
└──────────────────────────────────────────────────────────────────────┘
```

## Responsabilidades das camadas

- **`cmd/limiar-collector/main.go`** — raiz de composição (composition root). A struct `provider` implementa `cli.Provider` e é o único lugar que constrói instâncias concretas de `storage`, `telegram`, `logger` e `collector`. `run()` chama `config.Load` seguido de `config.Validate` antes de construir a árvore de comandos.
- **`internal/cli`** — Árvore de comandos do Cobra (`NewRootCmd`, `newAuthCmd`, `newChannelsCmd`, `newRunCmd`). Depende apenas da interface `Provider`; nunca constrói dependências concretas. `auth`/`channels` usam um logger de texto, `run` usa JSON.
- **`internal/collector`** — orquestração e adaptação. `Collector` executa a única goroutine `dbWriter`, realiza o backfill inicial e conduz `client.Run`. `MessageHandler` adapta bytes brutos do update para `storage.RawMessage`. `Classifier`/`NoopClassifier` é o Strategy plugável.
- **`internal/telegram`** — a Fachada sobre o gotd/td. `TelegramClient` expõe `Auth`, `IsAuthenticated`, `AddUpdateHandler`, `ResolveChannel`, `FetchHistory`, `Run`. `Client` é o único tipo que importa o gotd/td. `Dispatcher` faz fan-out das atualizações; `PeerStore` armazena peers em cache com RWMutex; `TursoSessionStorage` satisfaz `session.Storage` do gotd; `terminalAuthenticator` satisfaz `auth.UserAuthenticator` do gotd; `encodeUpdate`/`extractMessages` serializam as atualizações (updates) para JSON.
- **`internal/storage`** — toda a persistência. `DB` (open/close/conn), `migrate` (`migrations/*.sql` embutidas) e `Repository` (todas as instruções SQL).
- **`internal/config`**, **`internal/logger`**, **`internal/errors`** — infraestrutura transversal (cross-cutting).

## Modelo de Concorrência (fan-out / fan-in)

```
                 Telegram MTProto
                       │  tg.UpdatesClass
                       ▼
              Client.onUpdate → encodeUpdate → []byte
                       │
                 Dispatcher.Dispatch
                       │  fan-out: um canal bufferizado + goroutine por handler
        ┌──────────────┼──────────────┐
        ▼              ▼              ▼
   consume(h1)    consume(h2)    consume(hN)     (recover() por goroutine)
        │              │              │
        ▼              ▼              ▼
   MessageHandler.HandleUpdate → Classify → writeCh <- WriteJob
        │              │              │
        └──────────────┼──────────────┘
                       │  fan-in: único canal WriteJob
                       ▼
              Collector.dbWriter  (ÚNICO escritor no BD)
                       │  SaveRawMessage + UpdateChannelLastMessage
                       ▼
                  Banco Tursogo (escritas serializadas)
```

Principais propriedades:

- **Fan-out:** O `Dispatcher` fornece a cada `UpdateHandler` registrado seu próprio canal bufferizado (`DispatcherBufferSize`, padrão 256) drenado por uma goroutine dedicada, para que um handler lento não bloqueie os demais.
- **Isolamento de panic:** `Dispatcher.invoke` envolve cada chamada de handler com `recover()` e registra a condição recuperada no log — um update inválido nunca derruba o processo.
- **Fan-in:** todos os handlers enviam valores `WriteJob` para um único `writeCh` (`DBWriterBufferSize`, padrão 512). A goroutine única `dbWriter` é o único escritor no `*sql.DB`, eliminando concorrência de escrita.
- **Handler stateless (sem estado):** `MessageHandler` não possui estado mutável e não adquire locks; é seguro para invocação concorrente.
- **Cache de peers:** `PeerStore` protege seu `map[int64]*storage.Peer` com um `sync.RWMutex`; leituras concorrentes não bloqueiam umas às outras.
- **Graceful shutdown:** `cli/run` constrói um `signal.NotifyContext` para `SIGTERM`/`SIGINT`. Ao cancelar, `Collector.shutdown` fecha `writeCh` e faz `wg.Wait()` para o escritor drenar; `run` impõe `ShutdownTimeout` com uma corrida usando `time.After`.

## Modelo de dados

Structs internas (`internal/storage/repository.go`):

```go
type RawMessage struct {
    ID            int64
    ChannelID     int64
    MessageID     int64
    Payload       []byte   // update bruto do gotd/td serializado como JSON
    ReceivedAt    time.Time
    SchemaVersion int      // 1 na Fase 1
}

type Channel struct {
    ID              int64
    Username        string
    Title           string
    Active          bool
    AddedAt         time.Time
    LastMessageID   int64
    LastCollectedAt time.Time
}

type Peer struct {
    ID         int64
    AccessHash int64
    Type       string  // "channel", "user", "chat"
    Username   string
    UpdatedAt  time.Time
}
```

Schema SQL (`internal/storage/migrations/001_initial.sql`):

```sql
CREATE TABLE IF NOT EXISTS sessions (
    id         INTEGER PRIMARY KEY DEFAULT 1,
    data       BLOB NOT NULL,
    updated_at TEXT NOT NULL DEFAULT (datetime('now')),
    CHECK (id = 1)                          -- constraint de linha única
);

CREATE TABLE IF NOT EXISTS peers (
    id          INTEGER PRIMARY KEY,
    access_hash INTEGER NOT NULL,
    type        TEXT NOT NULL CHECK (type IN ('channel', 'user', 'chat')),
    username    TEXT,
    updated_at  TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS channels (
    id                INTEGER PRIMARY KEY,
    username          TEXT NOT NULL UNIQUE,
    title             TEXT NOT NULL DEFAULT '',
    active            INTEGER NOT NULL DEFAULT 1,
    added_at          TEXT NOT NULL DEFAULT (datetime('now')),
    last_message_id   INTEGER NOT NULL DEFAULT 0,
    last_collected_at TEXT
);

CREATE TABLE IF NOT EXISTS raw_messages (
    id             INTEGER PRIMARY KEY AUTOINCREMENT,
    channel_id     INTEGER NOT NULL REFERENCES channels(id),
    message_id     INTEGER NOT NULL,
    payload        TEXT NOT NULL,           -- JSON
    received_at    TEXT NOT NULL DEFAULT (datetime('now')),
    schema_version INTEGER NOT NULL DEFAULT 1,
    UNIQUE(channel_id, message_id)          -- desduplicação persistente segura
);

CREATE INDEX IF NOT EXISTS idx_raw_messages_channel ON raw_messages(channel_id, message_id);
CREATE INDEX IF NOT EXISTS idx_channels_username    ON channels(username);
```

Colunas datetime são armazenadas como texto no formato `2006-01-02 15:04:05` (UTC), correspondendo aos padrões `datetime('now')` do schema; `Repository.parseDBTime` faz a leitura de volta (com um fallback RFC3339).

## Ordem de build / dependências

```
errors → config → logger → storage
       → telegram/{session, peers, auth, dispatcher} → telegram/client
       → collector/{classifier, handler, collector}
       → dashboard
       → cli/{root, auth, channels, run, dashboard}
       → cmd/limiar-collector/main.go
```

O grafo é acíclico; gotd/td é confinado ao `internal/telegram` e `database/sql` ao `internal/storage`.

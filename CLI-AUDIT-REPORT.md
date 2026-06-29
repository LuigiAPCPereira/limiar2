# AUDITORIA DO CLI ATUAL — LIMIAR

> **Data:** 2026-06-28  
> **Escopo:** `limiar` (orquestrador), `limiar-collector`, `limiar-processor`  
> **Regra:** Leitura apenas — nenhuma modificação

---

## A — ÁRVORE DE COMANDOS ATUAL

### 1. `limiar` — Orquestrador Unificado (Produção)

**Arquivo:** `cmd/limiar/main.go`  
**Descrição:** Binário único que roda Collector + Processor + Dashboard no mesmo processo, compartilhando um `*sql.DB`.

```
limiar
  └── (sem subcomandos — executa tudo direto)
```

**Flags globais:** Nenhuma (toda config via `LIMIAR_*` env vars / `.env`)

**Execução:**
```bash
limiar                    # sobe collector + processor + dashboard
```

---

### 2. `limiar-collector` — Collector Standalone (Dev/Debug)

**Arquivo:** `cmd/limiar-collector/main.go` + `internal/cli/root.go`

```
limiar-collector
  ├── auth                      # Autentica userbot (interativo, one-shot)
  ├── channels                  # Gerencia canais monitorados
  │   ├── list                  # Lista canais
  │   ├── add <username>        # Adiciona canal
  │   └── remove <username>     # Remove canal
  ├── run                       # Serviço coletor (daemon, graceful shutdown)
  │   └── --dashboard           # Inicia dashboard web na porta 8080
  ├── dashboard                 # Dashboard standalone (daemon)
  │   └── --port <int>          # Porta (default: 8080 / LIMIAR_DASHBOARD_PORT)
  └── media                     # Diagnóstico de mídia
      └── resolve               # Baixa imagem via MTProto
          └── --msg-id <int64>  # ID da mensagem processada
```

**Flags globais do root:** Nenhuma

**Flags por subcomando:**
| Comando | Flags |
|---------|-------|
| `run` | `--dashboard` (bool) |
| `dashboard` | `--port` (int) |
| `media` | `--msg-id` (int64) |
| `media resolve` | `--msg-id` (int64) |

---

### 3. `limiar-processor` — Processor Standalone (Dev/Debug)

**Arquivo:** `cmd/limiar-processor/main.go`

```
limiar-processor
  └── (sem subcomandos — executa direto)
      ├── --dashboard        # Inicia dashboard web (bool)
      └── --dashboard-port   # Porta do dashboard (int, default: 8080)
```

**Execução:**
```bash
limiar-processor                    # processor only
limiar-processor --dashboard        # processor + dashboard
```

---

## B — CÓDIGO COMPARTILHADO VS DUPLICADO

### ✅ Já Compartilhado (pacotes internos usados por múltiplos binários)

| Pacote | Usado por |
|--------|-----------|
| `internal/config` | `limiar`, `limiar-collector` |
| `internal/processor/config` | `limiar`, `limiar-processor` |
| `internal/storage` | Todos os 3 |
| `internal/logger` | Todos os 3 |
| `internal/dashboard` | `limiar`, `limiar-collector` (`dashboard` cmd), `limiar-processor` (`--dashboard`) |
| `internal/media` | `limiar`, `limiar-collector` (`run`, `media`), `limiar-processor` |
| `internal/model` | Todos os 3 |
| `internal/errors` | Todos os 3 |
| `internal/id` | Todos os 3 |
| `internal/telegram` | `limiar`, `limiar-collector` |
| `internal/collector` | `limiar`, `limiar-collector` (`run`) |
| `internal/processor` | `limiar`, `limiar-processor` |

---

### ⚠️ Duplicado / Divergente Entre Binários

| Item | `limiar` | `limiar-collector` | `limiar-processor` |
|------|----------|-------------------|-------------------|
| **Entry point** | `run()` monolítica | `provider` pattern + `cli.NewRootCmd()` | `run()` simples com `flag` |
| **Config carregamento** | `config.Load()` + `processor.LoadConfig()` | Via `Provider.Config()` → `config.Load()` | `processor.LoadConfig()` próprio |
| **Logger init** | Manual inline | Via `Provider.Logger()` | Manual inline |
| **DB init** | `storage.Open()` único compartilhado | Via `Provider.OpenStore()` | `storage.Open()` próprio |
| **Repo init** | 3 repos: collector, processor, dashboard | 1 repo: collector | 1 repo: processor |
| **Signal handling** | `errgroup.WithContext` | `signal.NotifyContext` no `run` cmd | `signal.NotifyContext` em `main` |
| **Graceful shutdown** | `errgroup.Wait()` + timeout | `Collector.shutdown()` + timeout custom | Loop simples + `ctx.Done()` |
| **Dashboard** | Sempre sobe (porta fixa do config) | Opcional via `--dashboard` flag | Opcional via `--dashboard` flag |
| **Media/Cache** | Cria `media.NewCache` + `media.NewResolver` | Cria em `run` e `dashboard` cmds | Não usa |

---

### 🔴 Conflitos de Design para Unificação

1. **Dois padrões de composição:**
   - `limiar`: Composição direta no `main()` (sem Provider interface)
   - `limiar-collector`: Provider pattern (interface `cli.Provider` implementada em `main.go`)
   - `limiar-processor`: Nenhum padrão — inicialização direta com `flag` package

2. **Config duplicada com nomes diferentes:**
   - `limiar` carrega **duas** configs: `config.Config` (collector) + `processor.Config`
   - `limiar-collector` usa só `config.Config`
   - `limiar-processor` usa só `processor.Config`
   - Campos sobrepostos: `DBPath`, `LogLevel`, `LogFormat` — mas **structs diferentes**

3. **Flags vs Env vars:**
   - `limiar-processor` usa `flag` package para `--dashboard` / `--dashboard-port`
   - `limiar-collector` usa Cobra flags para `--dashboard` / `--port`
   - `limiar` não tem flags — tudo via env

4. **Repository instances:**
   - `limiar` cria **3 instâncias** de `storage.Repository` (mesmo `*sql.DB`)
   - `limiar-collector` cria 1 por comando
   - `limiar-processor` cria 1

---

## C — LIFECYCLE MAP

### Comandos One-Shot (executam e saem)

| Binário | Comando | Descrição |
|---------|---------|-----------|
| `limiar-collector` | `auth` | Autentica interativamente, persiste sessão, sai |
| `limiar-collector` | `channels list` | Lista canais, sai |
| `limiar-collector` | `channels add` | Adiciona canal, sai |
| `limiar-collector` | `channels remove` | Remove canal, sai |
| `limiar-collector` | `media` (sem subcmd) | Smoke test de metadados de foto, sai |
| `limiar-collector` | `media resolve` | Baixa uma imagem, salva em disco, sai |

### Comandos Daemon / Long-Running (loop até SIGTERM/SIGINT)

| Binário | Comando | Componentes Ativos |
|---------|---------|-------------------|
| `limiar` | (root) | Collector + Processor + Dashboard (3 goroutines via errgroup) |
| `limiar-collector` | `run` | Collector (+ Dashboard opcional) |
| `limiar-collector` | `dashboard` | Dashboard HTTP + SSE |
| `limiar-processor` | (root) | Processor (+ Dashboard opcional) |

### Graceful Shutdown — Implementação por Binário

| Binário | Mecanismo | Timeout | Detalhes |
|---------|-----------|---------|----------|
| `limiar` | `errgroup.WithContext` + `signal.NotifyContext` | `cfg.ShutdownTimeout` (default 15s) | `g.Wait()` bloqueia; se timeout, força saída com erro |
| `limiar-collector run` | `signal.NotifyContext` + `Collector.shutdown()` | `cfg.ShutdownTimeout` | Drena canal do DBWriter; aguarda goroutines |
| `limiar-collector dashboard` | `signal.NotifyContext` + `Server.ListenAndServe(ctx)` | N/A (ctx cancelado) | HTTP server para via context |
| `limiar-processor` | `signal.NotifyContext` + loop `select` | N/A (ctx cancelado) | `Processor.Run()` respeita `ctx.Done()` |

### Goroutines / Workers Ativos

| Binário | Goroutines |
|---------|------------|
| `limiar` | 3 principais (errgroup): Collector, Processor, Dashboard HTTP. Collector interno: DBWriter + statsLoop + Telegram client handlers |
| `limiar-collector run` | 2: Collector (DBWriter + statsLoop + Telegram) + Dashboard HTTP (se `--dashboard`) |
| `limiar-collector dashboard` | 1: Dashboard HTTP |
| `limiar-processor` | 1: Processor loop (poll + processBatch). Dashboard HTTP se `--dashboard` |

---

## D — DEPENDÊNCIAS DE INICIALIZAÇÃO

### Ordem de Inicialização por Binário

#### `limiar` (Orquestrador)
```
1. config.Load() → cfg (collector config)
2. processor.LoadConfig() → procCfg (processor config)
3. Validate ambos
4. Logger (slog) + Presenter
5. signal.NotifyContext (SIGTERM, SIGINT)
6. storage.Open() → *storage.DB (único *sql.DB)
7. storage.NewRepository(db) → collectorRepo
8. processor.NewRepository(db) → procRepo
9. storage.NewRepository(db) → dashRepo (para dashboard)
10. Componentes:
    - telegram.NewTursoSessionStorage(collectorRepo)
    - telegram.NewPeerStore(collectorRepo)
    - telegram.NewDispatcher(cfg.DispatcherBufferSize)
    - telegram.NewClient(cfg.AppID, cfg.APIHash, ...)
    - collector.NewCollector(client, collectorRepo, NoopClassifier, ...)
    - processor.NewProcessor(procRepo, procCfg, ...)
    - dashboard.NewBroker()
    - media.NewCache + media.NewResolver(dashRepo)
    - dashboard.NewServer(dashRepo, procRepo, mediaResolver, ..., broker)
11. Collector.SetOnMessage() → publica no broker
12. errgroup.Go() ×3: Collector.Run, Processor.Run, Server.ListenAndServe
13. g.Wait()
```

#### `limiar-collector` (via Provider)
```
Por comando (auth, channels, run, dashboard, media):
1. p.Config() → config.Load() + Validate
2. p.Logger(format) → slog logger
3. p.Presenter() → TerminalPresenter
4. p.OpenStore(ctx, log) → *storage.Repository + closer
5. p.NewClient(log, repo) → telegram.Client
6. p.NewCollector(client, repo, log) → collector (só para `run`)
7. p.NewMediaClient(log, repo) → media.Client (só para `media resolve`)
8. p.NewImageCache() → media.Cache (para dashboard/media)
9. Executa comando específico
```

#### `limiar-processor`
```
1. flag.Parse() → --dashboard, --dashboard-port
2. processor.LoadConfig() → procCfg
3. procCfg.Validate()
4. Logger (slog)
5. signal.NotifyContext
6. storage.Open(procCfg.DBPath) → *storage.DB
7. processor.NewRepository(db.DB()) → procRepo
8. processor.NewProcessor(procRepo, procCfg, log)
9. Se --dashboard: dashboard.NewServer + go ListenAndServe
10. go proc.Run(ctx)
11. select { err := <-runErr; case <-ctx.Done() }
```

---

### Dependências Compartilhadas (Interface → Implementação)

| Interface | Implementação Concreta | Onde Definida |
|-----------|------------------------|---------------|
| `cli.Provider` | `provider` struct em `cmd/limiar-collector/main.go` | `internal/cli/root.go` |
| `storage.Repository` | `storage.Repository` (concreto) | `internal/storage/repository.go` |
| `telegram.TelegramClient` | `telegram.Client` | `internal/telegram/client.go` |
| `media.Client` | `telegram.MediaClient` | `internal/telegram/media.go` |
| `collector.Repository` | `storage.Repository` (via interface) | `internal/collector/collector.go` |
| `dashboard.dashboardRepo` | `storage.Repository` | `internal/dashboard/server.go` |
| `processor.ProcessedReader` | `processor.Repository` | `internal/processor/repository.go` |

---

## E — GAPS E RISCOS PARA UNIFICAÇÃO

### 1. **Dois Sistemas de Config Diferentes**
- `config.Config` (collector) tem: `AppID`, `APIHash`, `DispatcherBufferSize`, `DBWriterBufferSize`, `HistoryMax`, `HistoryMaxDays`, `ShutdownTimeout`, `MaxRetries`, `IOTimeout`
- `processor.Config` tem: `PollInterval`, `BatchSize`
- **Overlap:** `DBPath`, `LogLevel`, `LogFormat`
- **Risco:** Unificar exige merge de structs + validar que flags/env não colidem

### 2. **Provider Pattern vs Composição Direta**
- `limiar-collector` usa `cli.Provider` interface (testável, desacoplado)
- `limiar` ignora isso e compõe direto no `main()`
- **Risco:** Unificar `limiar` exigirá adotar Provider ou refatorar `internal/cli` para não depender dele

### 3. **Flag Package vs Cobra Flags**
- `limiar-processor` usa `flag` stdlib (simples, sem subcomandos)
- `limiar-collector` usa Cobra flags (subcomandos, help automático)
- **Risco:** Unificar em um binário Cobra único exige migrar `limiar-processor` para subcomando

### 4. **Múltiplas Instâncias de Repository**
- `limiar` cria 3 `storage.Repository` no mesmo `*sql.DB` (waste de prepared statements)
- Cada `NewRepository` prepara statements próprios
- **Risco:** Unificar deve compartilhar **uma** instância de `storage.Repository` onde possível

### 5. **Dashboard Coupling**
- `dashboard.NewServer` aceita `processed processor.ProcessedReader` (opcional)
- `limiar` passa `procRepo`; `limiar-collector` passa `nil`
- **Risco:** Interface do dashboard precisa ser estável para ambos os modos

### 6. **Telegram Dependencies Only in Collector**
- `limiar-processor` **não importa** `internal/telegram` (correto — Invariant 4)
- `limiar` importa e inicializa Telegram client
- **Risco:** Unificado deve manter essa separação — processor não pode depender de Telegram

### 7. **Shutdown Timeout Diferente**
- `limiar` e `limiar-collector` usam `cfg.ShutdownTimeout` (configurável)
- `limiar-processor` não tem timeout configurável — só `ctx.Done()`
- **Risco:** Unificar exige expor timeout no processor config

### 8. **Logger/Format Resolution Divergente**
- `limiar`: `logger.ResolveFormat(cfg.LogFormat, logger.IsTerminalWriter(os.Stdout))`
- `limiar-collector`: Mesma via `Provider.Logger()`
- `limiar-processor`: Mesma inline
- **OK:** Lógica consistente, mas duplicada

### 9. **DB Path Hardcoded Defaults**
- `config.Config`: `defaultDBPath = "./limiar.db"`
- `processor.Config`: `defaultDBPath = "./limiar.db"`
- **OK:** Mesmo default, mas duplicado

### 10. **Nenhum Makefile / Build System**
- Build manual: `go build ./cmd/limiar`, `go build ./cmd/limiar-collector`, etc.
- **Risco:** Unificação precisa de build system para gerar binário único

---

## F — INSUMOS PARA MISSÃO DE UNIFICAÇÃO

### O que o agente de unificação PRECISA SABER antes de começar:

#### 1. Estrutura de Comando Alvo (Proposta)
```
limiar
  ├── auth                      # ← de limiar-collector
  ├── channels                  # ← de limiar-collector
  │   ├── list
  │   ├── add
  │   └── remove
  ├── run                       # ← daemon orquestrador (produção)
  │   ├── --no-collector        # opcional: só processor + dashboard
  │   └── --no-processor        # opcional: só collector + dashboard
  ├── collector                 # ← subcomandos do collector standalone
  │   ├── run                   # ← alias para `limiar run --no-processor`
  │   └── dashboard
  ├── processor                 # ← subcomandos do processor standalone
  │   ├── run                   # ← alias para `limiar run --no-collector`
  │   └── dashboard
  ├── dashboard                 # ← standalone dashboard
  │   └── --port
  └── media                     # ← de limiar-collector
      └── resolve
```

#### 2. Config Unificada Necessária
```go
type Config struct {
    // Shared
    DBPath          string
    LogLevel        string
    LogFormat       string
    ShutdownTimeout int
    
    // Collector (Telegram)
    AppID                int
    APIHash              string
    DispatcherBufferSize int
    DBWriterBufferSize   int
    HistoryMax           int
    HistoryMaxDays       int
    MaxRetries           int
    IOTimeout            time.Duration
    
    // Processor
    PollInterval time.Duration
    BatchSize    int
    
    // Dashboard
    DashboardPort int
}
```

#### 3. Provider Interface Unificada
```go
type Provider interface {
    Config() *Config
    Logger(format string) logger.Logger
    Presenter() logger.Presenter
    OpenStore(ctx context.Context, log logger.Logger) (*storage.Repository, func() error, error)
    NewTelegramClient(log logger.Logger, repo *storage.Repository) telegram.TelegramClient
    NewCollector(client telegram.TelegramClient, repo *storage.Repository, log logger.Logger) *collector.Collector
    NewProcessor(repo *storage.Repository, cfg *ProcessorConfig, log logger.Logger) *processor.Processor
    NewDashboardServer(repo *storage.Repository, procRepo *processor.Repository, log logger.Logger) *dashboard.Server
    NewMediaClient(log logger.Logger, repo *storage.Repository) media.Client
    NewImageCache() *media.Cache
}
```

#### 4. Arquivos a Modificar / Criar
| Ação | Arquivos |
|------|----------|
| **Criar** | `cmd/limiar/main.go` novo (substitui os 3) |
| **Criar** | `internal/config/unified.go` (merge das duas configs) |
| **Mover** | `cmd/limiar-collector/main.go` → implementa `Provider` para o novo root |
| **Remover** | `cmd/limiar-collector/main.go` antigo, `cmd/limiar-processor/main.go`, `cmd/limiar/main.go` antigo |
| **Atualizar** | `internal/cli/root.go` → adiciona subcomandos `processor`, `collector`, flags globais |
| **Atualizar** | `internal/cli/run.go` → suporta flags `--no-collector`, `--no-processor` |
| **Atualizar** | `internal/processor/config.go` → usa `Config` unificado |
| **Atualizar** | `internal/config/config.go` → adiciona campos do processor |
| **Criar** | `Makefile` / `Taskfile.yaml` para build único |

#### 5. Testes de Regressão Necessários
- `limiar auth` → funciona igual ao `limiar-collector auth`
- `limiar channels list/add/remove` → idem
- `limiar run` → sobe collector+processor+dashboard (produção)
- `limiar run --no-collector` → sobe processor+dashboard (igual `limiar-processor --dashboard`)
- `limiar run --no-processor` → sobe collector+dashboard (igual `limiar-collector run --dashboard`)
- `limiar collector run` → alias, mesmo comportamento
- `limiar processor run` → alias, mesmo comportamento
- `limiar dashboard` → idem
- `limiar media resolve` → idem
- Graceful shutdown (SIGTERM/SIGINT) em todos os modos
- Config via env vars `LIMIAR_*` funciona em todos
- `.env` carrega igual em todos
- Logger format (json/text/pretty) respeitado em todos

#### 6. Riscos Técnicos a Mitigar
| Risco | Mitigação |
|-------|-----------|
| Prepared statements duplicados | Single `storage.Repository` instance shared |
| Config validation order | Load → Merge defaults → Validate all at once |
| Flag/env collision | Prefix all with `LIMIAR_`; Cobra binds flags to same viper |
| Telegram import in processor | Keep `internal/processor` clean; no telegram import |
| DB connection pooling | Single `*sql.DB`; multiple repos share it |
| Shutdown timeout consistency | Single `ShutdownTimeout` field in unified config |

---

## RESUMO EXECUTIVO

| Aspecto | Status |
|---------|--------|
| **Binários atuais** | 3 (`limiar`, `limiar-collector`, `limiar-processor`) |
| **Framework CLI** | Cobra v1.10.2 (collector), stdlib `flag` (processor) |
| **Config** | Viper + `LIMIAR_*` env vars + `.env` opcional |
| **Logger** | `slog` via `internal/logger` (json/text/pretty) |
| **DB** | Tursogo (driver `turso`), single file `./limiar.db` |
| **Composição** | Provider pattern (collector) vs inline (processor, orchestrator) |
| **Daemon commands** | 5: `limiar` (root), `limiar-collector run`, `limiar-collector dashboard`, `limiar-processor` (root), `limiar-processor --dashboard` |
| **One-shot commands** | 6: `auth`, `channels list/add/remove`, `media`, `media resolve` |
| **Shared deps** | 11 pacotes internos compartilhados |
| **Duplicação crítica** | Config structs, DB init, logger init, repo instances, shutdown logic |
| **Esforço estimado unificação** | ~2-3 dias (config merge + root cmd + provider + build + testes) |

---

## PRÓXIMOS PASSOS RECOMENDADOS

1. **Criar `internal/config/unified.go`** mergeando `config.Config` + `processor.Config`
2. **Atualizar `internal/cli/Provider`** para incluir Processor + Dashboard factories
3. **Escrever novo `cmd/limiar/main.go`** com Cobra root + todos subcomandos
4. **Migrar `limiar-collector` provider** para nova interface unificada
5. **Remover `cmd/limiar-collector/main.go`, `cmd/limiar-processor/main.go`, `cmd/limiar/main.go` antigos**
6. **Criar `Makefile`** com targets: `build`, `test`, `run`, `run-collector`, `run-processor`
7. **Testes de integração** cobrindo todos os 11 comandos finais
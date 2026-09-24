# ADR 015 — Topologia de Deployment e Separação da API

## Status

Proposto

## Contexto

O Limiar hoje (Fase 1/2) roda como um único processo (`limiar run`) no PC
do desenvolvedor. Esse processo contém collector + processor + banco
embedded + dashboard HTTP embutido. Isso funciona para desenvolvimento,
mas não é sustentável para produção por três motivos:

1. **Disponibilidade**: o PC do desenvolvedor não é 24/7. Promoções chegam
   a qualquer hora (pico documentado às 21h), e o collector precisa estar
   sempre online para capturar.

2. **Contenção de banco**: o dashboard embutido e o `dbWriter` do collector
   competem pela única conexão (`SetMaxOpenConns(1)`). Em produção com
   tráfego real de frontend, isso degrada a captura.

3. **Separação de produto vs ferramenta**: o projeto terá DOIS frontends
   com públicos diferentes:
   - **Dashboard de Produto** (público): site de promoções com scroll
     infinito, estilo Pelando. É o produto final.
   - **Dashboard de Dev** (interno): ferramenta de inspeção, debug e
     auditoria. Não é público.

   Esses dois não podem viver no mesmo processo que o daemon de coleta.

Adicionalmente, o `CONTEXT.md` já prevê a Fase 4 (`limiar-api`) como
binário separado com "REST + SSE para o frontend". Este ADR formaliza
essa separação e define a topologia alvo.

### Por que a API não pode ficar no Vercel (serverless)

- **SSE não funciona em serverless**: functions são stateless, têm
  timeout de 10-60s, e não mantêm conexão aberta. O dashboard precisa
  de push em tempo real (SSE ou WebSocket).
- **Conexão com banco**: serverless não mantém conexões persistentes
  com o banco. Cada cold start abre nova conexão — ineficiente e lento.
- **Cold starts**: latência imprevisível que prejudica UX de um feed
  em tempo real.

A API precisa ser um servidor real (binário Go), não uma function
serverless.

## Constraints

- **AGENTS.md Invariante 5**: o banco é um detalhe físico. Não assumir
  multi-process access ao mesmo arquivo `.db` sem validar suporte do
  Turso. Hoje o daemon usa `SetMaxOpenConns(1)` — único escritor.
- **AGENTS.md Invariante 6**: simplicidade vence. Menos componentes,
  menos processos, menos dependências.
- **AGENTS.md §11**: stack fechada. Nenhuma dependência externa nova
  no daemon/processor.
- **AGENTS.md §15.1**: processor nunca importa telegram. API nunca
  importa telegram. A comunicação é exclusivamente via banco.
- **ADR 009**: o orquestrador unificado em processo único é aceito
  para a fase atual. Este ADR não reverte 009 — o registra como estado
  de transição.
- **Disponibilidade**: o daemon (collector) precisa estar online 24/7
  para capturar mensagens. Promocões não esperam.

## Alternativas consideradas

### 1. Manter tudo num processo (status quo — ADR 009)

Daemon + API + dashboard no mesmo binário, na mesma VPS.

**Rejeitada para produção.** Funciona para dev, mas em produção o
tráfego do frontend compete com a coleta pela conexão do banco. O
dashboard público (estilo Pelando) tem tráfego muito maior que o
dashboard de dev, e a contenção se torna inaceitável.

### 2. Daemon headless + API no Vercel (serverless)

Daemon na VPS. API como serverless functions no Vercel. Dashboard no
Vercel.

**Rejeitada.** Serverless não suporta SSE nem conexões persistentes
com banco. Cold starts degradam UX. A API precisa ser um servidor
real com estado e conexões longas.

### 3. Daemon headless + API na VPS + Dashboards no Vercel (ESCOLHIDA)

Daemon (headless) + API (binário Go) na mesma VPS. Dashboards (produto
e dev) no Vercel consumindo a API via HTTPS.

### 4. Daemon headless + Turso Cloud (replicação) + API no Vercel

Daemon replica para Turso Cloud. API no Vercel lê do Turso Cloud.
Dashboards no Vercel.

**Rejeitada por enquanto.** Adiciona complexidade de replicação e
sync que não se justifica no estágio atual. O free tier do Turso
(5GB, 500M rows read/mês) é suficiente em volume, mas a complexidade
de configurar e manter replicação não vale o benefício quando uma VPS
simples resolve. Pode ser revisitada se o projeto crescer.

## Decisão

Adotar uma topologia de produção com separação clara entre daemon,
API e frontends:

```
┌─────────────────────────────────────────────────────┐
│ VPS (Hostinger / Docker VPS / similar)              │
│                                                     │
│  ┌─────────────┐     ┌──────────────┐              │
│  │   Daemon    │     │  limiar-api  │              │
│  │  (headless) │     │  (REST+SSE)  │              │
│  │             │     │              │              │
│  │ collector   │     │  /api/       │              │
│  │ processor   │     │  processed   │              │
│  │ limiar.db   │◄───►│  /api/media  │              │
│  │ (embedded)  │     │  /api/events │              │
│  │             │     │  (SSE)       │              │
│  └─────────────┘     └──────┬───────┘              │
│                             │                      │
└─────────────────────────────┼──────────────────────┘
                              │ HTTPS
                    ┌─────────┴─────────┐
                    │                   │
          ┌─────────▼─────┐   ┌─────────▼─────┐
          │ Vercel        │   │ Vercel        │
          │               │   │               │
          │ Dashboard de  │   │ Dashboard de  │
          │  PRODUTO      │   │  DEV          │
          │ (público)     │   │ (interno)     │
          │               │   │               │
          │ Next.js       │   │ Next.js ou    │
          │ Scroll inf.   │   │ Vite+Alpine   │
          │ estilo Pelando│   │ (atual)       │
          └───────────────┘   └───────────────┘
```

### Componentes

**Daemon (headless) — processo de produção**
- Contém: collector + processor + banco embedded (limiar.db).
- Não contém: HTTP server, dashboard, API.
- Empacotamento: systemd unit OU Docker container (ambos válidos).
- Comunicação: exclusivamente via banco. Nenhum endpoint HTTP.
- Disponibilidade: 24/7. Restart automático (systemd Restart=on-failure
  ou Docker restart policy).
- O dashboard embutido atual (`internal/dashboard`) é removido do
  daemon quando a API separa (Fase 4).

**limiar-api — processo de produção (Fase 4)**
- Binário Go separado, mesmo repo, `cmd/limiar-api/main.go`.
- Expõe: REST (`/api/processed`, `/api/media/{id}`, `/api/stats`) + SSE
  (`/api/events`).
- Lê o banco read-only (mesmo arquivo `limiar.db` na VPS).
- Serve dois consumidores: Dashboard de Produto e Dashboard de Dev.
- Pode rodar na mesma VPS que o daemon (compartilham o arquivo `.db`).
- Autenticação: Dashboard de Dev exige token/IP allowlist; Dashboard de
  Produto pode ser público ou ter rate limiting.

**Dashboard de Produto (Vercel) — frontend público**
- Next.js. Scroll infinito de promoções. Estilo Pelando.
- Público. É o produto final que o usuário vê.
- Consome a API via HTTPS.

**Dashboard de Dev (Vercel ou local) — ferramenta interna**
- Pode ser o Vite+Alpine atual migrado para Next.js, ou mantido como
  ferramenta standalone.
- Não público. Acesso restrito (token, IP allowlist, ou Basic Auth na
  API).
- Serve para debug, auditoria, inspeção de raw/processed messages.

### Empacotamento do daemon

Duas opções válidas, decidas no momento do deploy:

**Opção A — systemd unit**
```ini
[Unit]
Description=Limiar Daemon
After=network.target

[Service]
Type=simple
User=limiar
WorkingDirectory=/opt/limiar
EnvironmentFile=/opt/limiar/.env
ExecStart=/opt/limiar/bin/limiar run
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
```

**Opção B — Docker**
```dockerfile
FROM golang:1.26 AS build
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 go build -o /limiar ./cmd/limiar

FROM gcr.io/distroless/static
COPY --from=build /limiar /limiar
COPY --from=build /src/internal/dashboard/dist /dist
ENTRYPOINT ["/limiar", "run"]
```

Ambas garantem restart automático, isolamento e gestão de ciclo de vida.
A escolha depende da preferência ops e da VPS escolhida.

### Fases de transição

**Fase atual (1/2 — desenvolvimento no PC)**
- Tudo num processo (`limiar run`) com dashboard embutido.
- Dashboard embutido é ferramenta de dev local (localhost).
- Banco é arquivo local (`limiar.db`).
- Funciona porque só o desenvolvedor acessa.

**Fase 4 (API separa)**
- `limiar-api` nasce como binário separado.
- Dashboard embutido é removido do daemon.
- Daemon fica headless.
- Dashboards migram para Vercel (ou rodam localmente apontando pra API).

**Pós-Fase 4 (produção)**
- Daemon + API na VPS 24/7.
- Dashboards na Vercel.
- Banco embedded na VPS (sem Turso Cloud, por enquanto).

### Observabilidade

Métricas expostas pela API (não pelo daemon headless):
- Endpoint `/metrics` em Prometheus text format (sem dependência nova —
  net/http da stdlib, formato text manual).
- Contadores: messages capturadas, processadas, CRE coverage, cache hit
  ratio, batch sizes, poll latency.

Logs do daemon:
- JSON para stdout/stderr.
- Se systemd: journald faz rotation nativo.
- Se Docker: log driver json-file com max-size e max-file, ou journald.

Health check:
- O daemon headless NÃO expõe HTTP. Health check é via systemd
  (`systemctl status limiar`) ou Docker healthcheck (process alive +
  banco responde).
- A API expõe `/healthz` para load balancers e uptime monitors.

### Retention policy

- `raw_messages`: manter 90 dias. Purge via job periódico
  (`DELETE WHERE received_at < datetime('now', '-90 days')`) após
  confirmar que `processed_messages` está persistido. Mantém o
  Invariante 3 (reprocessamento) dentro da janela operacional.
- `photo_cache`: já tem TTL de 30 dias (`CleanExpiredPhotoCache`).
  Garantir que roda periodicamente.
- `processed_messages`: sem purge. É a saída do pipeline.

### Backup

- `limiar.db` é a fonte da verdade. Backup periódico via
  `sqlite3 limiar.db ".backup '/backup/limiar-$(date +%F).db'"`.
- Rodar com daemon ativo é seguro (Turso/SQLite suporta backup online).
- Frequência: diária, retenção de 7-30 backups.

## Justificativa

A alternativa 3 vence porque:

1. **Resolve contenção**: daemon headless tem a conexão do banco pra si.
   A API é o único competidor, e só compete em leitura (read-only),
   nunca em escrita.

2. **Resolve SSE**: a API é um servidor real, mantém conexões SSE
   abertas sem timeout de serverless.

3. **Resolve disponibilidade**: VPS 24/7 independe do PC do
   desenvolvedor.

4. **Simplicidade (Invariante 6)**: não adiciona Turso Cloud nem
   replicação. O banco continua embedded. Apenas separa processos que
   já estavam previstos para separar (Fase 4 do `CONTEXT.md`).

5. **Separa produto de ferramenta**: Dashboard de Produto (público) e
   Dashboard de Dev (interno) são frontends distintos com públicos
   distintos. Não devem compartilhar processo nem código de frontend.

6. **Vercel faz o que faz de melhor**: servir Next.js estático com CDN
   global, sem se preocupar com banco ou SSE.

## Consequências

- **Vantagem**: daemon puro e leve. Sem goroutines de HTTP, sem
  embutir assets de frontend, sem contenção.
- **Vantagem**: frontends escalam independentemente na Vercel.
- **Vantagem**: API pode ter rate limiting, auth e cache sem afetar
  o daemon.
- **Desvantagem**: dois processos para gerenciar na VPS (daemon + API).
  Mitigado por systemd/docker-compose.
- **Desvantagem**: o `internal/dashboard` atual é removido na Fase 4.
  Código descartável por design (foi ferramenta de transição).

## Verificação

### Fase atual
- [ ] Daemon (`limiar run`) funciona no PC com dashboard embutido
- [ ] Graceful shutdown em SIGTERM/SIGINT (já implementado)

### Fase 4 (quando implementada)
- [ ] `cmd/limiar-api/main.go` existe e serve `/api/processed`,
      `/api/media/{id}`, `/api/events` (SSE)
- [ ] Daemon roda headless (sem `internal/dashboard`)
- [ ] `internal/dashboard` removido ou marcado como deprecated
- [ ] systemd unit OU Dockerfile funcionando na VPS
- [ ] Restart automático testado (kill -9, systemctl restart)
- [ ] Backup diário funcionando
- [ ] Retention de raw_messages (90 dias) funcionando
- [ ] `/metrics` exposto pela API em Prometheus text format
- [ ] Dashboard de Produto no Vercel consumindo a API
- [ ] Dashboard de Dev acessível apenas com auth

## Dependências

- Nenhuma dependência externa nova no daemon (continua stack fechada).
- limiar-api pode usar a mesma stack fechada (net/http, slog).
- Dashboards (Vercel) usam Next.js/React — fora da stack fechada do
  Go, mas são frontends independentes.

## Riscos

- **Migração do dashboard embutido**: remover `internal/dashboard` pode
  quebrar fluxos de dev que dependem dele. Mitigado: manter o dashboard
  de dev disponível (local ou Vercel) antes de remover do daemon.
- **Single-instance do banco**: daemon e API na mesma VPS compartilham
  o arquivo `.db`. O daemon mantém `SetMaxOpenConns(1)`. A API abre
  sua própria conexão read-only. Isso é dois processos no mesmo arquivo
  — precisa validar que Turso embedded suporta (Invariante 5). Se não
  suportar, a API usa HTTP pra falar com o daemon (não acessa o banco
  diretamente).
- **Custo da VPS**: ~$5/mês (Hostinger/Hetzner). Aceitável para projeto
  pessoal. Se crescer, Turso Cloud entra como opção.

## Nota sobre o ADR 009

Este ADR NÃO reverte o ADR 009 (orquestrador unificado). O ADR 009
permanece válido para a fase atual de desenvolvimento. Este ADR 015
registra a EVOLUÇÃO para produção: o que era um processo se torna dois
(daemon + API), e o que era embutido se separa (dashboards saem para
Vercel).

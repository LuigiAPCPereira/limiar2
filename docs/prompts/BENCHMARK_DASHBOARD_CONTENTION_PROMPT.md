# PROMPT — Benchmark: Dashboard Contention

## Objetivo

Você está trabalhando no repositório `github.com/limiar/collector`.

Crie um benchmark/reprodutor de contenção para medir o impacto do dashboard
embutido sobre o processamento atual. O objetivo NÃO é otimizar ainda. O objetivo
é medir com evidência se requests do dashboard/API embutidos competem com o
processor/collector pelo banco e quanto isso altera throughput, latência e uso de
recursos.

Este benchmark vai orientar decisões futuras do ADR 015: daemon headless + API
separada.

## Leitura obrigatória antes de alterar qualquer coisa

Leia primeiro:

1. `AGENTS.md`
2. `docs/adr/009-unified-orchestrator-process.md`
3. `docs/adr/015-deployment-topology.md`
4. `docs/benchmarks/2026-07-04-performance-baseline.md`
5. `internal/dashboard/server.go`
6. `internal/dashboard/broker.go`
7. `internal/storage/processor_repository.go`
8. `internal/cli/processor_cmd.go`
9. `internal/processor/processor.go`

Respeite as regras do projeto:

- Não adicionar dependências fora da stack fechada.
- Não mudar arquitetura.
- Não otimizar sem medir.
- Não criar goroutines/workers além do necessário para o benchmark.
- Não mexer em comportamento de produção fora do escopo.
- SQL continua centralizado em `internal/storage`.

## Contexto técnico

Hoje, durante desenvolvimento, `limiar run` e `limiar processor run` podem subir
processor/collector com dashboard embutido. O dashboard faz queries de leitura no
mesmo banco e pode competir pela conexão/arquivo com o processamento.

O baseline atual pós-CRE é:

- Dataset: 31.832 mensagens
- Reprocessamento completo: ~1m58s–2m26s
- Throughput: ~218–269 msg/s
- RSS durante reprocessamento: ~133–135 MB
- Banco: 420 MB
- `photo_cache`: 251.5 MB
- `raw_messages`: 124.3 MB
- `processed_messages`: 34.2 MB

As queries locais com conexão persistente são rápidas isoladamente, mas ainda não
foi medido o efeito de carga concorrente do dashboard enquanto o processor roda.

## Escopo

Implemente um benchmark de desenvolvimento, preferencialmente como ferramenta em:

```text
tools/bench-dashboard-contention/
```

Sugestão de arquivo:

```text
tools/bench-dashboard-contention/main.go
```

A ferramenta deve:

1. Rodar cenários comparáveis contra o banco atual.
2. Medir tempo total, throughput, RSS aproximado quando possível e erros.
3. Gerar saída textual clara e uma tabela final.
4. Não modificar o banco além do que o comando de reprocessamento já modifica.
5. Ter modo dry-run/help.

## Cenários mínimos

### Cenário A — Baseline sem dashboard load

Executar:

```sh
limiar processor reprocess --all
```

Medir:

- duração total;
- mensagens processadas;
- falhas;
- throughput msg/s.

### Cenário B — Dashboard read load leve

Enquanto o reprocessamento roda, disparar requests GET em loop contra endpoints
leves do dashboard/API embutido, por exemplo:

- `/healthz`
- `/api/processed/stats`
- `/api/channels`

Parâmetros configuráveis:

- duração;
- concorrência;
- intervalo entre requests;
- base URL do dashboard.

Medir:

- duração do reprocess;
- throughput;
- total de requests;
- status codes;
- p50/p95/max de latência HTTP;
- erros/timeouts.

### Cenário C — Dashboard read load pesado

Enquanto o reprocessamento roda, disparar requests paginados contra endpoints que
trazem payload maior:

- `/api/messages?limit=100&offset=N`
- `/api/processed?limit=100&offset=N`
- `/api/media/{id}` se houver IDs conhecidos e se isso não exigir rede externa.

Medir os mesmos dados do cenário B.

### Cenário D — Stats caro

Medir explicitamente endpoint equivalente a agregações caras, se existir hoje.
Se não existir endpoint, documentar como “não aplicável”.

## Como a ferramenta deve encontrar o binário

Aceitar flags:

```text
--bin ./limiar
--db ./limiar.db
--dashboard-url http://127.0.0.1:8080
--scenario baseline|light|heavy|stats|all
--concurrency 1
--duration 120s
--interval 250ms
--timeout 2s
```

Não assuma porta fixa. Leia help/comandos existentes se necessário.

## Importante sobre iniciar dashboard

Você pode escolher um de dois caminhos, documentando a escolha:

1. A ferramenta NÃO inicia o dashboard; assume que ele já está rodando e recebe
   `--dashboard-url`.
2. A ferramenta inicia `limiar processor run`/`limiar dashboard` em subprocesso,
   espera readiness em `/healthz`, e depois roda o cenário.

Recomendação: começar pelo caminho 1, porque é mais simples e evita mexer no
lifecycle de produção.

## Métricas de saída

A saída final deve ter formato legível:

```text
Scenario: heavy
Reprocess duration: 2m34.120s
Processed: 31832
Failures: 0
Throughput: 206.6 msg/s
HTTP requests: 1250
HTTP errors: 0
HTTP p50: 12.3ms
HTTP p95: 80.1ms
HTTP max: 220.4ms
Slowest endpoint: /api/messages
RSS max: 142 MB (best effort)
Conclusion: heavy dashboard load added +17.2% wall time vs baseline
```

Também deve sair com exit code != 0 se:

- reprocessamento falhar;
- houver erro de build;
- dashboard não responder quando exigido;
- mais de 5% dos requests falharem.

## Medição de RSS

RSS é best effort. Em Linux, pode ler `/proc/<pid>/status`. Se não conseguir,
registre `rss_max=unknown` e siga.

Não adicione dependência para medição de processo.

## Cuidados

- Não rode contra `.env` real com collector conectado ao Telegram.
- Este benchmark deve usar `processor reprocess --all`, não `limiar run`, para
  evitar captura real durante benchmark.
- Não exponha segredos em logs.
- Não faça commit de outputs gigantes.
- Se gerar relatório, salve em `docs/benchmarks/YYYY-MM-DD-dashboard-contention.md`.

## Verificação obrigatória

Após implementar:

```sh
go build ./...
go vet ./...
go test ./...
go test -race ./...
```

E executar pelo menos:

```sh
go run ./tools/bench-dashboard-contention --help
```

Se possível, executar um cenário curto:

```sh
go run ./tools/bench-dashboard-contention \
  --scenario light \
  --dashboard-url http://127.0.0.1:8080 \
  --duration 30s \
  --concurrency 1
```

Se o dashboard não estiver rodando, documente o blocker e não invente resultado.

## Entregável

1. Ferramenta em `tools/bench-dashboard-contention/`.
2. Relatório em `docs/benchmarks/` se o benchmark for executado.
3. Nenhuma mudança de comportamento de produção.
4. Resumo final com números reais ou blocker explícito.

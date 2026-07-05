# Dashboard Contention Benchmark — 2026-07-04

## Contexto

Benchmark executado para medir o impacto de requests do dashboard embutido durante
`processor reprocess --all`, como evidência para a discussão do ADR 015
(daemon headless + API separada).

A ferramenta usada foi `tools/bench-dashboard-contention` em modo `in-process`,
para respeitar a restrição do Tursogo: apenas um processo pode abrir o arquivo
`limiar.db` por vez.

Comando executado:

```sh
go run ./tools/bench-dashboard-contention \
  --db ./limiar.db \
  --dashboard-url http://127.0.0.1:9090 \
  --scenario all \
  --duration 120s \
  --concurrency 1 \
  --interval 250ms \
  --timeout 2s
```

Dataset observado:

- Mensagens reprocessadas por cenário: 36.672
- Falhas de reprocessamento: 0 em todos os cenários
- Carga HTTP: 1 worker, intervalo de 250ms, duração máxima de 120s por cenário
- Timeout HTTP: 2s

## Resultado resumido

| Scenario | Reprocess | Processed | Failures | Throughput | HTTP req | HTTP err | HTTP p50 | HTTP p95 | RSS max | Delta |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| baseline | 2m51.529s | 36.672 | 0 | 213.8 msg/s | 0 | 0 | 0s | 0s | 66 MB | n/a |
| light | 3m7.005s | 36.672 | 0 | 196.1 msg/s | 260 | 1 | 127ms | 710ms | 786 MB | +9.0% |
| heavy | 3m49.626s | 36.672 | 0 | 159.7 msg/s | 214 | 6 | 123ms | 1.607s | 631 MB | +33.9% |
| stats | 2m53.711s | 36.672 | 0 | 211.1 msg/s | 297 | 0 | 103ms | 676ms | 583 MB | +1.3% |

## Resultado por cenário

### Baseline — sem carga HTTP

```text
Reprocess duration: 2m51.529s
Processed: 36672
Failures: 0
Throughput: 213.8 msg/s
RSS max: 66 MB
```

Este é o ponto de comparação para os demais cenários.

### Light — endpoints leves

Endpoints:

- `/healthz`
- `/api/processed/stats`
- `/api/channels`

```text
Reprocess duration: 3m7.005s
Processed: 36672
Failures: 0
Throughput: 196.1 msg/s
HTTP requests: 260
HTTP errors: 1
HTTP p50: 127ms
HTTP p95: 710ms
HTTP max: 2.001s
Slowest endpoint: /healthz
RSS max: 786 MB
Delta: +9.0%
```

Interpretação: mesmo carga leve adicionou impacto mensurável de wall time. O erro
HTTP sem status code registrado indica timeout/erro de cliente, não resposta 5xx
observada.

### Heavy — endpoints paginados

Endpoints:

- `/api/messages?limit=100&offset=N`
- `/api/processed?limit=100&offset=N`

```text
Reprocess duration: 3m49.626s
Processed: 36672
Failures: 0
Throughput: 159.7 msg/s
HTTP requests: 214
HTTP errors: 6
HTTP p50: 123ms
HTTP p95: 1.607s
HTTP max: 2.001s
Slowest endpoint: /api/processed
RSS max: 631 MB
Delta: +33.9%
```

Interpretação: carga paginada pesada é o caso problemático. Com apenas 1 worker
HTTP e intervalo de 250ms, o reprocessamento ficou 33,9% mais lento e alguns
requests atingiram o timeout de 2s.

### Stats — agregação atual

Endpoint:

- `/api/processed/stats`

```text
Reprocess duration: 2m53.711s
Processed: 36672
Failures: 0
Throughput: 211.1 msg/s
HTTP requests: 297
HTTP errors: 0
HTTP p50: 103ms
HTTP p95: 676ms
HTTP max: 1.789s
Slowest endpoint: /api/processed/stats
RSS max: 583 MB
Delta: +1.3%
```

Interpretação: o endpoint de stats atual não alterou materialmente o wall time do
reprocessamento neste dataset e nesta carga.

## Leitura dos números

### Impacto no processamento

- `light`: +9,0% de wall time e queda de throughput de 213,8 para 196,1 msg/s.
- `heavy`: +33,9% de wall time e queda de throughput de 213,8 para 159,7 msg/s.
- `stats`: +1,3% de wall time; dentro de uma variação pequena para este teste.

O cenário `heavy` confirma contenção relevante entre leituras do dashboard e
reprocessamento quando endpoints paginados são chamados durante o processamento.

### Latência HTTP

- `heavy` teve p95 de 1.607s e máximo de 2.001s, encostando no timeout configurado.
- Os erros HTTP em `light` e `heavy` não aparecem na tabela de status codes; isso
  indica erro/timeout no cliente antes de receber resposta HTTP completa.
- `/api/processed` foi o endpoint mais lento no cenário pesado.

### RSS

Os valores de RSS dos cenários com dashboard não devem ser lidos como deltas
isolados de memória entre cenários, porque o benchmark `--scenario all` executa os
cenários em sequência dentro do mesmo processo Go. O runtime pode reter heap já
alocado entre cenários, então `RSS max` é útil como limite superior observado na
sessão, não como comparação limpa entre cenários.

Para comparar memória com mais rigor, execute cada cenário em um processo separado.

## Medição isolada posterior

Após a rodada `--scenario all`, `baseline` e `heavy` foram repetidos em
processos separados para remover retenção de heap entre cenários e aumentar o
timeout HTTP do cenário pesado para 5s.

### Baseline isolado

Comando:

```sh
go run ./tools/bench-dashboard-contention \
  --db ./limiar.db \
  --dashboard-url http://127.0.0.1:9090 \
  --scenario baseline \
  --duration 120s \
  --concurrency 1 \
  --interval 250ms \
  --timeout 2s
```

Resultado:

```text
Reprocess duration: 2m47.464s
Processed: 36672
Failures: 0
Throughput: 219.0 msg/s
RSS max: 64 MB
```

### Heavy isolado com timeout 5s

Comando:

```sh
go run ./tools/bench-dashboard-contention \
  --db ./limiar.db \
  --dashboard-url http://127.0.0.1:9090 \
  --scenario heavy \
  --duration 120s \
  --concurrency 1 \
  --interval 250ms \
  --timeout 5s
```

Resultado:

```text
Reprocess duration: 2m50.451s
Processed: 36672
Failures: 0
Throughput: 215.1 msg/s
HTTP requests: 375
HTTP errors: 0
HTTP p50: 28ms
HTTP p95: 195ms
HTTP max: 557ms
Slowest endpoint: /api/messages
RSS max: 138 MB
Status codes: 200=375
```

Comparação isolada:

| Scenario | Reprocess | Throughput | HTTP req | HTTP err | HTTP p95 | HTTP max | RSS max | Delta |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| baseline isolado | 2m47.464s | 219.0 msg/s | 0 | 0 | 0s | 0s | 64 MB | n/a |
| heavy isolado | 2m50.451s | 215.1 msg/s | 375 | 0 | 195ms | 557ms | 138 MB | +1.8% |

Interpretação: a repetição isolada não reproduziu o impacto de +33,9% observado na
rodada `--scenario all`. Com timeout 5s, concorrência 1 e processos separados por
cenário, o endpoint pesado adicionou apenas +1,8% ao wall time e não gerou erros
HTTP. A evidência mais confiável neste ponto é a medição isolada, não a rodada
sequencial `all`.

### Heavy isolado com concorrência 4

Comando:

```sh
go run ./tools/bench-dashboard-contention \
  --db ./limiar.db \
  --dashboard-url http://127.0.0.1:9090 \
  --scenario heavy \
  --duration 120s \
  --concurrency 4 \
  --interval 250ms \
  --timeout 5s
```

Resultado:

```text
Reprocess duration: 2m50.912s
Processed: 36672
Failures: 0
Throughput: 214.6 msg/s
HTTP requests: 1451
HTTP errors: 0
HTTP p50: 29ms
HTTP p95: 223ms
HTTP max: 1.033s
Slowest endpoint: /api/messages
RSS max: 143 MB
Status codes: 200=1451
```

### Light isolado com timeout 5s

Comando:

```sh
go run ./tools/bench-dashboard-contention \
  --db ./limiar.db \
  --dashboard-url http://127.0.0.1:9090 \
  --scenario light \
  --duration 120s \
  --concurrency 1 \
  --interval 250ms \
  --timeout 5s
```

Resultado:

```text
Reprocess duration: 2m50.236s
Processed: 36672
Failures: 0
Throughput: 215.4 msg/s
HTTP requests: 302
HTTP errors: 0
HTTP p50: 109ms
HTTP p95: 302ms
HTTP max: 1.385s
Slowest endpoint: /api/processed/stats
RSS max: 1002 MB
Status codes: 200=302
```

### Stats isolado com timeout 5s

Comando:

```sh
go run ./tools/bench-dashboard-contention \
  --db ./limiar.db \
  --dashboard-url http://127.0.0.1:9090 \
  --scenario stats \
  --duration 120s \
  --concurrency 1 \
  --interval 250ms \
  --timeout 5s
```

Resultado:

```text
Reprocess duration: 2m41.155s
Processed: 36672
Failures: 0
Throughput: 227.6 msg/s
HTTP requests: 316
HTTP errors: 0
HTTP p50: 112ms
HTTP p95: 302ms
HTTP max: 1.423s
Slowest endpoint: /api/processed/stats
RSS max: 646 MB
Status codes: 200=316
```

### Light isolado repetido com timeout 5s

Resultado:

```text
Reprocess duration: 2m55.213s
Processed: 36672
Failures: 0
Throughput: 209.3 msg/s
HTTP requests: 313
HTTP errors: 0
HTTP p50: 100ms
HTTP p95: 254ms
HTTP max: 1.493s
Slowest endpoint: /api/channels
RSS max: 901 MB
Status codes: 200=313
```

### Channels isolado

```text
Reprocess duration: 2m57.667s
Processed: 36672
Failures: 0
Throughput: 206.4 msg/s
HTTP requests: 289
HTTP errors: 1
HTTP p50: 81ms
HTTP p95: 523ms
HTTP max: 1.449s
Slowest endpoint: /api/channels
RSS max: 132 MB
Status codes: 200=288
```

### Healthz isolado

Primeira execução:

```text
Reprocess duration: 2m49.354s
Processed: 36672
Failures: 0
Throughput: 216.5 msg/s
HTTP requests: 323
HTTP errors: 1
HTTP p50: 85ms
HTTP p95: 245ms
HTTP max: 1.061s
Slowest endpoint: /healthz
RSS max: 889 MB
Status codes: 200=322
```

Segunda execução:

```text
Reprocess duration: 2m57.184s
Processed: 36672
Failures: 0
Throughput: 207.0 msg/s
HTTP requests: 291
HTTP errors: 0
HTTP p50: 86ms
HTTP p95: 368ms
HTTP max: 1.49s
Slowest endpoint: /healthz
RSS max: 952 MB
Status codes: 200=291
```

Baseline repetido após os testes de endpoints:

```text
Reprocess duration: 2m33.801s
Processed: 36672
Failures: 0
Throughput: 238.4 msg/s
RSS max: 64 MB
```

Comparação adicional:

| Scenario | Reprocess | Throughput | HTTP req | HTTP err | HTTP p95 | HTTP max | RSS max |
|---|---:|---:|---:|---:|---:|---:|---:|
| baseline repetido | 2m33.801s | 238.4 msg/s | 0 | 0 | 0s | 0s | 64 MB |
| channels isolado | 2m57.667s | 206.4 msg/s | 289 | 1 | 523ms | 1.449s | 132 MB |
| healthz isolado #1 | 2m49.354s | 216.5 msg/s | 323 | 1 | 245ms | 1.061s | 889 MB |
| healthz isolado #2 | 2m57.184s | 207.0 msg/s | 291 | 0 | 368ms | 1.49s | 952 MB |

Interpretação: o RSS alto reproduziu com `/healthz`, enquanto o baseline repetido
continuou em 64 MB. Isso reduz a suspeita sobre `/api/channels` e aponta para o
`CountRawMessages` executado por `/healthz` como o menor reprodutor observado do
pico de RSS.

Comparação isolada expandida:

| Scenario | Reprocess | Throughput | HTTP req | HTTP err | HTTP p95 | HTTP max | RSS max | Delta vs baseline isolado |
|---|---:|---:|---:|---:|---:|---:|---:|---:|
| baseline isolado | 2m47.464s | 219.0 msg/s | 0 | 0 | 0s | 0s | 64 MB | n/a |
| heavy c1 isolado | 2m50.451s | 215.1 msg/s | 375 | 0 | 195ms | 557ms | 138 MB | +1.8% |
| heavy c4 isolado | 2m50.912s | 214.6 msg/s | 1451 | 0 | 223ms | 1.033s | 143 MB | +2.1% |
| light c1 isolado #1 | 2m50.236s | 215.4 msg/s | 302 | 0 | 302ms | 1.385s | 1002 MB | +1.7% |
| stats c1 isolado | 2m41.155s | 227.6 msg/s | 316 | 0 | 302ms | 1.423s | 646 MB | -3.8% |
| light c1 isolado #2 | 2m55.213s | 209.3 msg/s | 313 | 0 | 254ms | 1.493s | 901 MB | +4.6% |
| channels isolado | 2m57.667s | 206.4 msg/s | 289 | 1 | 523ms | 1.449s | 132 MB | +6.1% |
| healthz isolado #1 | 2m49.354s | 216.5 msg/s | 323 | 1 | 245ms | 1.061s | 889 MB | +1.1% |
| healthz isolado #2 | 2m57.184s | 207.0 msg/s | 291 | 0 | 368ms | 1.49s | 952 MB | +5.8% |

Interpretação adicional: aumentar `heavy` de concorrência 1 para 4 não degradou
materialmente o throughput do reprocessamento e não gerou erros HTTP. A anomalia
principal é memória: `healthz` reproduziu RSS alto em duas execuções (889 MB e
952 MB), enquanto baseline repetido permaneceu em 64 MB. Como `/healthz` executa
`CountRawMessages`, este é o menor reprodutor observado do pico de RSS. `channels`
não reproduziu RSS alto (132 MB), mas teve p95 maior e 1 erro HTTP.

## Conclusão

A primeira rodada (`--scenario all`) indicou possível contenção forte no cenário
`heavy`, mas a medição isolada posterior não reproduziu esse impacto. A conclusão
atual é mais conservadora:

- com concorrência 1, `heavy` isolado adicionou apenas +1,8% de wall time;
- com concorrência 4, `heavy` isolado adicionou apenas +2,1% de wall time;
- `heavy` isolado não gerou erros HTTP com timeout de 5s;
- a latência HTTP de `heavy` ficou aceitável para uso local/dev
  (p95 223ms em concorrência 4);
- `light` isolado também não degradou throughput de forma severa (+1,7% e +4,6%
  nas duas execuções), mas atingiu RSS máximo muito alto (1002 MB e 901 MB);
- `stats` isolado atingiu RSS alto (646 MB), mesmo com throughput maior que o
  baseline observado;
- `healthz` isolado reproduziu RSS alto (889 MB e 952 MB), tornando
  `CountRawMessages` o menor reprodutor conhecido;
- `channels` isolado não reproduziu RSS alto (132 MB), mas ainda adicionou alguma
  latência/erro HTTP;
- a rodada `all` continua útil como alerta de que execução sequencial no mesmo
  processo pode distorcer RSS e latência, mas não deve ser usada sozinha para
  decidir arquitetura.

A decisão do ADR 015 ainda deve considerar tráfego real do frontend e separação
operacional, mas estes benchmarks isolados não provam contenção severa de
throughput até concorrência 4. O risco mais claro agora é memória/RSS associado a
queries de contagem/agregação repetidas durante o reprocessamento, começando por
`CountRawMessages` em `/healthz`.

## Próximas medições recomendadas

1. Investigar memória/RSS de `CountRawMessages` sob carga concorrente com
   reprocessamento; `/healthz` é o menor reprodutor observado.
2. Confirmar se `CountRawMessages` em Tursogo faz scan/aloca/cacheia páginas de
   forma proporcional ao tamanho de `raw_messages`.
3. Só depois considerar mitigação, por exemplo remover contagem de `/healthz` ou
   cachear esse valor fora do hot path.
4. Não otimizar throughput ainda: os benchmarks isolados não mostram gargalo de
   throughput até concorrência 4.

Comandos sugeridos:

```sh
go run ./tools/bench-dashboard-contention \
  --db ./limiar.db \
  --dashboard-url http://127.0.0.1:9090 \
  --scenario baseline \
  --duration 120s \
  --concurrency 1 \
  --interval 250ms \
  --timeout 2s
```

```sh
go run ./tools/bench-dashboard-contention \
  --db ./limiar.db \
  --dashboard-url http://127.0.0.1:9090 \
  --scenario heavy \
  --duration 120s \
  --concurrency 1 \
  --interval 250ms \
  --timeout 5s
```

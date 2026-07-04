# Performance Baseline — 2026-07-04

## Contexto

Baseline medido após Sprint 1+2 de extração determinística + CRE e após a
calibração do CRE contra três padrões de erro reais.

Ambiente:

- Host: Linux CachyOS (desktop do desenvolvedor)
- Banco: `limiar.db`
- Dataset: 31.832 mensagens raw/processadas
- Binário: `/tmp/limiar-bench` construído com `go build ./cmd/limiar`
- Comando principal medido: `limiar processor reprocess --all`

> Este documento mede o estado atual de desenvolvimento. Não é benchmark de
> produção final. O objetivo é criar uma linha de base antes de otimizações e
> antes da separação futura daemon headless + API (ADR 015).

## Resumo Executivo

O processo atual está leve o suficiente para rodar no PC do desenvolvedor.
O gargalo observado é CPU durante reprocessamento completo, não memória nem I/O
de dashboard/API.

Números principais:

- Reprocessamento completo: **31.832 mensagens em 2m11s–2m26s**
- Throughput efetivo: **~218–242 mensagens/s**
- Memória durante reprocessamento: **~133–135 MB RSS**
- Threads durante reprocessamento: **19**
- File descriptors durante reprocessamento: **8**
- Banco total: **420 MB**
- Maior tabela: **photo_cache (251.5 MB)**
- `raw_messages`: **124.3 MB**
- `processed_messages`: **34.2 MB**

Conclusão: o `raw_messages` não é o maior custo de disco; `photo_cache` é.
Retention policy deve atacar os dois, mas sem eliminar raw como fonte de verdade.

## Reprocessamento completo

Comando:

```sh
/tmp/limiar-bench processor reprocess --all
```

Execução observada:

```text
✅ Reprocessamento concluído: 31832 processadas, 0 falharam, 2m11.706s
```

Execuções anteriores durante a mesma sessão:

```text
31832 processadas, 0 falharam, 1m58.447s
31832 processadas, 0 falharam, 2m23.429s
31832 processadas, 0 falharam, 2m11.706s
```

Throughput aproximado:

| Tempo | Throughput |
|---:|---:|
| 1m58s | ~269 msg/s |
| 2m11s | ~242 msg/s |
| 2m23s | ~222 msg/s |

Variação esperada em desktop por carga concorrente do sistema.

## Uso de recursos durante reprocessamento

Amostras via `/proc/<pid>/status` durante `limiar processor reprocess --all`:

```text
amostra 1: RSS=133196KB HWM=133300KB threads=19 fd=8
amostra 2: RSS=133848KB HWM=133848KB threads=19 fd=8
amostra 3: RSS=134540KB HWM=134560KB threads=19 fd=8
amostra 4: RSS=133748KB HWM=133748KB threads=19 fd=8
amostra 5: RSS=134652KB HWM=134652KB threads=19 fd=8
```

Interpretação:

- Memória estável em ~135 MB RSS.
- Sem crescimento visível durante o batch observado.
- FDs baixos (8), sem indício de vazamento de descritores.
- Threads (19) aceitável para runtime Go + driver/banco.

## Cobertura do CRE após calibração

Consulta:

```sql
SELECT message_type,
       COUNT(*) AS total,
       SUM(CASE WHEN product_name != '' THEN 1 ELSE 0 END) AS with_name,
       ROUND(SUM(CASE WHEN product_name != '' THEN 1 ELSE 0 END)*100.0/COUNT(*),1) AS pct_covered,
       ROUND(AVG(product_name_confidence),3) AS avg_conf
FROM processed_messages
GROUP BY message_type
ORDER BY total DESC;
```

Resultado:

```text
message_type     total   with_name   pct_covered   avg_conf
--------------   -----   ---------   -----------   --------
deal_complete    21723   21110       97.2          0.652
deal_no_coupon   7955    7901        99.3          0.642
commentary       911     746         81.9          0.465
deal_no_price    648     612         94.4          0.639
coupon_only      323     242         74.9          0.527
coupon_expired   247     82          33.2          0.273
video            24      14          58.3          0.313
admin_meta       1       1           100.0         0.45
```

Cobertura de `deal_complete` subiu de 90.6% para 97.2% após calibração.

## Latência de queries de leitura

Medição com conexão SQLite persistente em Python (`sqlite3`), 100 execuções por
query, contra o mesmo arquivo `limiar.db`. A medição evita o custo de spawn do
CLI `sqlite3`.

```text
count_raw:         rows=1  p50=0.021ms  avg=0.022ms  p95=0.027ms  max=0.033ms
count_processed:   rows=1  p50=0.011ms  avg=0.011ms  p95=0.012ms  max=0.022ms
processed_page:    rows=50 p50=0.090ms  avg=0.092ms  p95=0.101ms  max=0.114ms
raw_page_payload:  rows=50 p50=0.170ms  avg=0.175ms  p95=0.187ms  max=0.407ms
processed_stats:   rows=8  p50=2.679ms  avg=2.679ms  p95=2.703ms  max=2.813ms
cre_coverage:      rows=8  p50=40.063ms avg=40.214ms p95=41.458ms max=43.496ms
```

Interpretação:

- Queries paginadas do dashboard/API são muito baratas no dataset atual.
- `raw_page_payload` é mais caro que `processed_page`, mas ainda sub-millisecond
  em conexão local persistente.
- `cre_coverage` faz agregação por `message_type` com expressão sobre
  `product_name`; é o mais caro (~40ms), mas aceitável para endpoint de stats
  eventual. Não deve ser chamado em loop apertado.

## Tamanho do banco

```text
Banco total: 420.0 MB
Freelist:    0.0 MB
```

Top tabelas/índices via `dbstat`:

```text
photo_cache:                         251.5 MB
raw_messages:                        124.3 MB
processed_messages:                   34.2 MB
idx_processed_url_hash:                2.4 MB
idx_raw_messages_received_at:          1.7 MB
idx_raw_messages_channel_received_at:  1.4 MB
idx_processed_messages_posted:         0.9 MB
sqlite_autoindex_raw_messages_1:       0.9 MB
sqlite_autoindex_processed_messages_1: 0.8 MB
idx_processed_messages_type:           0.8 MB
idx_processed_merchant:                0.6 MB
idx_processed_raw_message_id:          0.4 MB
```

Interpretação:

- `photo_cache` é o maior consumidor de disco, não `raw_messages`.
- `raw_messages` continua relevante como fonte de verdade e representa ~30% do
  banco atual.
- Retention policy deve priorizar:
  1. garantir limpeza periódica de `photo_cache` (TTL 30 dias);
  2. manter `raw_messages` por uma janela operacional (proposta: 90 dias);
  3. nunca purgar `processed_messages` por padrão.

## Riscos e próximos benchmarks

Este baseline ainda NÃO mede:

1. Contenção real entre dashboard embutido e `dbWriter` sob carga.
2. Daemon idle rodando por horas/dias com collector conectado ao Telegram.
3. Uso de CPU/memória do dashboard com browser aberto e SSE ativo.
4. Latência da futura `limiar-api` em VPS.

Próximos benchmarks recomendados:

- `bench/dashboard-contention`: reprocessamento + carga concorrente nos endpoints
  atuais do dashboard para medir impacto no tempo total.
- `bench/daemon-idle-24h`: daemon rodando 24h com amostragem de RSS, goroutines,
  FDs e tamanho do banco.
- `bench/api-readonly`: quando a Fase 4 existir, medir `limiar-api` separada
  lendo o banco em read-only.

## Decisões de arquitetura reforçadas

- Não eliminar `raw_messages`: o custo atual não justifica quebrar os invariantes
  de fonte da verdade e reprocessamento.
- Daemon headless continua sendo o alvo correto (ADR 015), mas a pressão de
  performance do dashboard embutido não é urgente enquanto o uso é local/dev.
- Métricas Prometheus text format manual na futura API são suficientes para o
  próximo estágio; não há necessidade imediata de adicionar dependência nova.

## 2024-05-20 - sync.Pool não compensa em serialização com json.Encoder
**Aprendizado:** A serialização de cada mensagem capturada para JSON em `encodeUpdate` e `extractMessages` foi testada trocando `json.Marshal` por um `sync.Pool` com `bytes.Buffer` e `json.Encoder`. Ao medir via benchmark (`go test -bench`), os resultados mostraram que o tempo da operação na verdade aumentou. O `json.Marshal` internamente já usa um pool otimizado (`encodeStatePool`) sem o overhead do `io.Writer`. O uso do `bytes.Buffer` e posterior alocação de `[]byte` aumentou a CPU e alocações na prática, especialmente porque `Encoder.Encode` adiciona um `\n` não intencional ao final.

**Ação:** Reverti essa alteração porque piorou a performance e a corretude (o \n extra). Buscarei gargalos em canais subdimensionados, contenção de locks ou cópias desnecessárias no próprio codebase do projeto ao invés de tentar melhorar `json.Marshal` sem uma biblioteca externa.

## 2024-05-20 - Early return no MessageHandler para canais não monitorados
**Aprendizado:** No hot path de `MessageHandler.HandleUpdate`, updates de canais não monitorados não devem ser persistidos. Antes de criar o struct `storage.RawMessage` e passar para classificação e DBWriter, devemos validar se o canal é monitorado.
**Ação:** Adicionar early return se o canal não for monitorado antes de alocar `RawMessage`. No código original, isso já estava implementado parcialmente no topo do método com `if _, ok := h.monitoredChannels[update.ChannelID]; !ok { return nil }` então mantivemos isso.

## 2024-05-20 - Otimizando o tamanho dos slices em extractMessages
**Aprendizado:** Em `extractMessages` o limit dos slices criados por `messages.getHistory` geralmente vem do tamanho do array `Messages` da resposta. Porém, no codebase estava hardcoded um parse manual que podia sofrer `append` num loop sem `make` com capacidade pré-alocada em outros cantos. Em `extractMessages`, pre-alocar o `make([]HistoryMessage, 0, len(raw))` já é a melhor prática para evitar alocações de array extra à medida que os items são convertidos.

## 2024-05-20 - Index para ListMessages sem channelID
**Aprendizado:** A view de "All channels" do dashboard invoca `Repository.ListMessages(ctx, 0, 50, 0)`, que ordena por `received_at DESC`. Antes, isso ativava um TEMP B-TREE na engine do SQLite para ordenação porque o único índice relacionado era `(channel_id, received_at DESC)` da migration 002. O benchmark passava de ~5.8ms/op para ~0.7ms/op criando um index em `received_at DESC` diretamente, garantindo que `ORDER BY` não necessite carregar e sortear a tabela `raw_messages` inteira na memória.
**Ação:** Incluir migration `004_dashboard_index.sql` criando o índice sobre `received_at DESC`.

## 2026-06-12 - Colapsar N fsyncs em 1 via batch transacional no dbWriter
**Aprendizado:** O gargalo dominante do pipeline de escrita não era CPU, JSON marshaling nem lock contention — era o custo de `fsync` por transação. Cada `SaveRawMessage` no `dbWriter` fazia `BEGIN → Exec → COMMIT`, e o COMMIT dispara fsync no WAL do Tursogo (~100–170μs/op em SSD). Em backfill com milhares de mensagens, isso somava centenas de milissegundos por página de 100 mensagens. Consolidando N inserts em uma única transação (`BEGIN → N × Exec → COMMIT`), o fsync acontece apenas uma vez por lote. Benchmark no mesmo hardware:
- `SaveRawMessage` single: **169.7μs/msg**, 2250 B/op, 80 allocs/op
- `SaveRawMessageBatch` (lote=100): 13.08ms/100msgs = **130.8μs/msg**, 247.5 kB/100 = 2475 B/op, 83 allocs/msg
- **Ganho: ~1.30× throughput (+30%) com alocações equivalentes por mensagem.** Em cenários de alta contenção de I/O (backfill com páginas cheias), o ganho real é ainda maior porque o scheduler do Tursogo não precisa interleavar fsyncs de múltiplas transações.
**Ação:** Substituí o loop do `dbWriter` por uma versão com *timer-based batching* (janela de 10ms) em `internal/collector/collector.go`. Adicionei `SaveRawMessageBatch` em `internal/storage/repository.go` (mesma prepared statement, nova transação). A invariante "uma única goroutine escreve no DB" (§14.1 AGENTS.md) é preservada — o dbWriter continua único. O fallback em caso de falha do lote é a escrita individual com retry (`writeWithRetry`), mantendo a semântica original de "mensagem perdida é logada, nunca descartada silenciosamente".

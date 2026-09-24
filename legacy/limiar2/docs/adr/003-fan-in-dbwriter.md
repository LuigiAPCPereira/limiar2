# ADR 003 — Goroutine Única de DBWriter via Fan-In

## Status

Aceito

## Contexto

Canais são monitorados concorrentemente e as atualizações (updates) passam por um fan-out para handlers em
suas próprias goroutines (padrão Observer, `Dispatcher`). Se cada handler gravasse no
banco de dados de forma independente, múltiplas goroutines iriam gravar no mesmo
`*sql.DB` concorrentemente, correndo o risco de contenção de escrita e erros de bloqueio (lock) em um banco de
dados embutido, além de tornar a ordenação de escritas e o tratamento de erros difíceis de raciocinar.

## Decisão

Serializar todas as escritas de banco de dados através de uma única goroutine DBWriter
(`Collector.dbWriter`). Os produtores (o `MessageHandler` stateless e o
backfill inicial) enviam valores `WriteJob` para um único canal com buffer
(`writeCh`, dimensionado por `DBWriterBufferSize`, padrão 512). O DBWriter drena o
canal e é o **único** componente que escreve no `*sql.DB`. Ao desligar,
`Collector.shutdown` fecha `writeCh` e aguarda (`wg.Wait()`) que o escritor termine
o dreno.

`storage.DB.Conn()` documenta este invariante: apenas o DBWriter pode emitir escritas
através da conexão retornada.

## Consequências

- Sem escritores concorrentes; a contenção de escrita é eliminada por construção.
- Escritas são naturalmente ordenadas e possuem um único local para política de repetição/erros
  (`writeWithRetry`, limitada por `maxWriteRetry`, que registra no log as mensagens perdidas).
- O caminho crítico (hot path) não usa locks: os handlers não possuem estado e apenas enfileiram.
- O throughput (vazão) é limitado a um único escritor; aceitável para os volumes da Fase 1 e
  ajustável via tamanho do buffer. Um banco de dados persistentemente lento acabará exercendo
  pressão contrária (backpressure) através do canal bufferizado.

## Alternativas consideradas

- **Gravações por handler (Per-handler writes)** — escritores concorrentes em um `*sql.DB`; rejeitado devido à
  contenção e complexidade de ordenação.
- **Um mutex de gravação no `Repository`** — serializa as gravações mas espalha os pontos de chamada
  de escrita por várias goroutines, complicando repetições/desligamento; o fan-in com canal
  é mais limpo e oferece um único ponto de drenagem.
- **Um pool de escritores** — desnecessário para um banco de dados embutido de arquivo único e
  reintroduz a concorrência na conexão.

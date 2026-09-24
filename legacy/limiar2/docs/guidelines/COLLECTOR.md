# Diretrizes — Estendendo a Camada Collector

A camada collector (`internal/collector`) adapta os updates brutos (raw) do Telegram
em registros `storage.RawMessage` e os persiste através de uma única goroutine DBWriter.
A Fase 1 realiza apenas captura — sem enriquecimento. Siga estas regras.

## Regras

1. **`MessageHandler` permanece sem estado (stateless).** Ele contém apenas o seu `classifier`,
   `writeCh`, e `log`, não adquire nenhum lock, e é seguro para invocação
   concorrente. Não adicione campos mutáveis ou estado por canal.
2. **Apenas o DBWriter escreve.** `Collector.dbWriter` é a única goroutine
   que chama `Repository.SaveRawMessage` / `UpdateChannelLastMessage` (fan-in
   via `writeCh chan WriteJob`). Os handlers e o backfill **enfileiram** (enqueue) jobs; eles
   nunca escrevem diretamente no banco de dados.
3. **O classifier é plugável.** Utilize a interface Strategy `Classifier`.
   A Fase 1 entrega apenas o `NoopClassifier` (pass-through, identidade). Fases
   posteriores trocam por classificadores baseados em regras/LLMs sem mexer no pipeline.
4. **Sem enriquecimento na Fase 1.** O handler preserva os bytes originais
   do payload literalmente (`Payload: update`) e estampa `SchemaVersion = 1`. Nenhuma
   normalização, desduplicação além da persistência segura, ou processamento semântico.
5. **Gravações perdidas são logadas, nunca descartadas silenciosamente.** `writeWithRetry` tenta novamente
   até `maxWriteRetry`, depois registra um erro explícito (e notifica `ErrCh` se
   definido) — veja o Requisito 3.10.
6. **Prioridade ao Contexto e consciente de cancelamento.** Envios para `writeCh` fazem select no
   `ctx.Done()`; `shutdown` fecha o `writeCh` e faz `wg.Wait()` no escritor.
7. **Backfill é apenas na primeira execução.** `backfill` pula os canais com
   `LastMessageID > 0` (modo de retomada) e busca `historyBatchSize` (20) para
   os canais na primeira execução — veja o ADR 006.
8. **Dependa da interface `Repository`**, não da struct concreta, para que o
   collector permaneça testável usando um fake.

## Correto

```go
// Handler stateless: adapta, classifica via Strategy, enfileira — nunca escreve.
func (h *MessageHandler) HandleUpdate(ctx context.Context, update []byte) error {
    msg := &storage.RawMessage{
        Payload:       update,        // preservado literalmente (verbatim)
        ReceivedAt:    time.Now().UTC(),
        SchemaVersion: schemaVersion, // 1
        // ChannelID / MessageID extraídos do envelope
    }
    classified, err := h.classifier.Classify(ctx, msg)
    if err != nil {
        return apperrors.Wrap("collector", "classify", err)
    }
    select {
    case h.writeCh <- WriteJob{Message: classified}:
        return nil
    case <-ctx.Done():
        return apperrors.Wrap("collector", "handle_update", ctx.Err())
    }
}
```

```go
// Um classificador futuro se acopla sem mudar o handler ou o DBWriter.
type RuleClassifier struct{ /* regras */ }
func (c RuleClassifier) Classify(ctx context.Context, raw *storage.RawMessage) (*storage.RawMessage, error) {
    // Fase 2+: retorna uma cópia transformada. A Fase 1 permanece como Noop.
    return raw, nil
}
```

## Incorreto

```go
// ERRADO: o handler escreve no banco de dados diretamente, quebrando o invariante do single-writer.
func (h *MessageHandler) HandleUpdate(ctx context.Context, update []byte) error {
    return h.repo.SaveRawMessage(ctx, adapt(update)) // deveria enfileirar no writeCh em vez disso
}
```

```go
// ERRADO: enriquecimento na Fase 1 (modificar/normalizar o payload bruto).
msg.Payload = normalize(update)   // A Fase 1 deve armazenar os bytes brutos inalterados
```

```go
// ERRADO: criar goroutines de escrita extra no BD (fan-out para múltiplos writers).
for _, job := range jobs {
    go c.repo.SaveRawMessage(ctx, job.Message)  // apenas o dbWriter pode escrever
}
```

```go
// ERRADO: descartar silenciosamente uma mensagem que falhou ao persistir.
if err := c.repo.SaveRawMessage(ctx, msg); err != nil {
    return // perdida sem log — viola o Requisito 3.10
}
```

## O que nunca fazer

- Escrever no banco de dados de qualquer outro lugar que não seja o `Collector.dbWriter` (ADR 003).
- Adicionar normalização, desduplicação, classificação ou chamadas a LLMs na Fase 1.
- Tornar o `MessageHandler` dependente de estado (stateful) ou de locks.
- Descartar uma gravação falha sem registrar a mensagem perdida no log.

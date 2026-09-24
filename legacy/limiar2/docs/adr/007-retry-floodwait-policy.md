# ADR 007 — Política de Retry, Backoff e Flood-Wait

## Status

Aceito

## Contexto

Um collector de longa duração deve sobreviver a falhas transitórias sem intervenção
manual: conexões com o Telegram caem, e o Telegram aplica rate-limits (limite de taxa) aos clientes com
sinais de `FLOOD_WAIT` que especificam uma duração de espera exata. Escritas em banco de
dados também podem falhar temporariamente. Precisamos de uma política de resiliência que se recupere
automaticamente, limite o seu esforço, e respeite as esperas exigidas pelo servidor, enquanto
ainda termina de forma limpa no cancelamento de contexto.

## Decisão

**Backoff de Conexão.** Na perda de conexão, `Client.Run` se reconecta usando
backoff exponencial via `CalculateBackoff`:

- atraso (delay) base **1s**, multiplicador **2x**, **10%** de jitter, teto **5m**;
- delimitado por `MaxRetries` — uma vez que as tentativas se esgotam, `CalculateBackoff`
  retorna `ErrMaxRetriesExceeded` e o `Run` para de se reconectar;
- o contador de tentativas é zerado a cada conexão bem-sucedida;
- a espera do backoff (sleep) faz select em `ctx.Done()`, então o cancelamento vence imediatamente.

`DefaultBackoff(maxRetries)` constrói essa política; ela é injetada no `NewClient`
em `main.go`.

**Flood-wait.** Quando o Telegram sinaliza `FLOOD_WAIT`, o cliente aguarda exatamente a
duração que o servidor especificar antes de tentar novamente (nenhum jitter é aplicado a uma
espera exigida pelo servidor).

**Gravações no Banco (DB writes).** O DBWriter repete (retries) cada `WriteJob` até `maxWriteRetry` vezes
(`writeWithRetry`); em caso de falha persistente ele loga a mensagem perdida explicitamente
e, se um `ErrCh` estiver presente, relata um erro que satisfaz a condição
`errors.Is(err, ErrDBWriteFailed)`.

## Consequências

- O collector se recupera sozinho de conexões perdidas e rate limits sem
  ação do operador.
- O Jitter evita tempestades de reconexão sincronizadas (thundering-herd); o teto de 5m limita o backoff.
- Tentativas limitadas (Bounded retries) previnem giros infinitos; o esgotamento é um erro explícito
  e inspecionável.
- O respeito à duração exata do flood-wait mantém o userbot em conformidade e
  evita banimentos progressivos.
- Todas as esperas respeitam o contexto, logo o desligamento (shutdown) fica dentro de `ShutdownTimeout`.

## Alternativas consideradas

- **Retry com intervalo fixo** — mais simples, mas propenso a tempestades sincronizadas de reconexão
  e ignora as regras semânticas do flood-wait. Rejeitado.
- **Tentativas ilimitadas (Unbounded retries)** — poderia ficar rodando para sempre numa falha permanente; rejeitado em
  favor de `MaxRetries`.
- **Ignorar a duração do flood-wait do servidor** (usando nosso próprio backoff) —
  arrisca banimentos; rejeitado.

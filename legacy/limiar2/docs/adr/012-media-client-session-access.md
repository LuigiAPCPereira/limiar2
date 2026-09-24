# ADR 012 — Acesso à Sessão MTProto Persistente pelo MediaClient (Wave 4)

## Status

Aceito. Implementação operacional atual usa `runOnce` serializado para CLI e backfill; o dashboard interno passa `client=nil` ao resolver e permanece read-only. Accessor de sessão persistente permanece decisão para a API pública concorrente.

## Contexto

O ADR 011 define o subsistema de resolução de imagens: um `Resolver` (pacote
`internal/media`) que tenta cache RAM, `photo_cache`, download MTProto sob demanda
quando recebe um `media.Client`, e `inline_thumb`. A implementação concreta do
client faz `upload.GetFile` e, quando possível, renova `file_reference` via
`messages.getMessages`/histórico.

Dois invariantes do projeto (AGENTS.md) criam uma tensão que precisa ser resolvida:

- **§14.2**: "Tipos do gotd/td não vazam para fora de `internal/telegram`." Logo, a
  implementação concreta do `MediaClient` sobre gotd/td **deve** morar em
  `internal/telegram`, não em `internal/media`.
- **ADR 011**: pressupõe que o resolver "tem acesso direto à conexão MTProto do
  collector sem necessidade de IPC, gRPC, ou segunda sessão."

O `telegram.Client` (facade) atual, porém, **não expõe** a conexão persistente
mantida por `Run()`. O único mecanismo disponível é `runOnce`, que abre uma nova
conexão MTProto por chamada. Não há, hoje, forma de chamar `upload.GetFile` na
sessão *já autenticada e ativa* do collector.

Este ADR registra **como** essa lacuna será fechada e **quando**.

## Decisão

### Abordagem escolhida: `runOnce` serializado para CLI/backfill; dashboard read-only

Usar o `MediaClient` concreto em `internal/telegram` com `runOnce` serializado para
os caminhos operacionais autenticados (`limiar media resolve`, `limiar media backfill`
e download proativo do Collector). O dashboard interno não recebe `MediaClient`;
serve apenas cache RAM, `photo_cache` e `inline_thumb`.

O accessor persistente, se necessário na API pública, provavelmente tomará a forma
de um callback/channel registrado antes de `Run()`, ou de um campo `api` preenchido
dentro do closure de `tg.Run` e invalidado na saída, com tratamento explícito das
janelas de reconexão.

### Diferimento parcial para a API pública

A implementação concreta do `MediaClient` já existe em `internal/telegram/media.go`.
Ela usa `runOnce` com mutex interno, suficiente para comandos CLI e backfill em lote.
O dashboard interno evita `runOnce` em request HTTP: recebe `client=nil` e consulta
somente caches já preenchidos. Um accessor para a sessão persistente continua
reservado para uma API pública concorrente, se a latência/carga tornar `runOnce` inadequado.

## Alternativas consideradas

1. **Opção B — `runOnce` para comandos CLI one-shot**: o comando abre uma conexão
   MTProto temporária, executa uma operação finita e encerra. **Aceita para
   `media resolve` e `media backfill`** porque são comandos operacionais isolados,
   não endpoints HTTP concorrentes. O backfill usa uma única conexão `runOnce` para
   o lote inteiro e limita o download a poucas goroutines de I/O, mantendo o handler
   serial para não paralelizar escrita no banco.
2. **Opção C — expor `*tg.Client` diretamente**: rejeitada: viola explicitamente o
   invariante §14.2 (tipos do gotd não vazam da facade).

## Consequências

### Positivas
- Fiel ao ADR 011: uma única sessão MTProto, compartilhada entre collector e resolver.
- `internal/media` permanece livre de gotd/td (testável com mocks, como hoje).
- A mudança fica isolada em `internal/telegram` (a única camada que pode importar gotd).

### Negativas
- Exige uma mudança no modelo de ciclo de vida do `telegram.Client` (AGENTS.md §19:
  mudança no modelo de concorrência/comunicação — justificada por este ADR).
- Janelas de reconexão precisam de tratamento explícito (o accessor pode estar
  indisponível brevemente; o resolver deve reagir com retry/backoff, não pânico).

## Verificação (Wave 4)

- O `MediaClient` concreto implementa `media.Client` e vive em `internal/telegram`.
- Nenhum tipo do gotd aparece em `internal/media` (grep de `github.com/gotd` em `internal/media` retorna vazio).
- Dashboard interno usa resolver sem `MediaClient`; API pública concorrente deve reavaliar accessor de sessão persistente antes de habilitar download em request HTTP.
- Testes do resolver seguem passando com mock (a adição do cliente real não quebra os testes existentes).

## Estado atual

- `internal/media`: `Cache` LRU+TTL, `Resolver` com fallback L1/L2/L3 opcional/L4 e testes `-race`.
- `internal/telegram/media.go`: `MediaClient` concreto sobre gotd/td, exposto via
  `limiar media resolve`, `limiar media backfill` e download proativo do Collector.
- `limiar run`: Collector e Dashboard recebem o mesmo `media.Cache`; downloads
  proativos populam o cache que o endpoint `/api/media/{processed_messages.id}` consulta sem abrir MTProto.
- **Futuro (API pública)**: se o endpoint público tiver concorrência/carga alta,
  implementar accessor da sessão persistente descrito neste ADR em vez de múltiplos
  `runOnce` concorrentes.

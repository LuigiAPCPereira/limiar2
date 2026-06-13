# ADR 012 — Acesso à Sessão MTProto Persistente pelo MediaClient (Wave 4)

## Status

Aceito (decisão registrada; implementação adiada para a Wave 4 — limiar-api).

## Contexto

O ADR 011 definiu o subsistema de resolução de imagens: um `MediaResolver` (pacote
`internal/media`) que baixa fotos sob demanda via MTProto, com cache LRU+TTL e
singleflight. O resolver depende de uma implementação concreta de `MediaClient`
que faça as chamadas MTProto (`upload.GetFile`, `messages.getMessages`,
`FetchHistory`).

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

### Abordagem escolhida: Opção A — accessor à sessão persistente

Adicionar ao `telegram.Client` um mecanismo que expõe a `*tg.Client` viva durante
o ciclo de `Run()` — renovado a cada reconexão — de forma que o `MediaClient`
concreto (em `internal/telegram`) possa disparar `upload.GetFile` reutilizando a
sessão autenticada do collector, sem segunda conexão.

Detalhes de implementação (a definir na Wave 4): o accessor provavelmente tomará a
forma de um callback/channel registrado antes de `Run()`, ou de um campo `api`
preenchido dentro do closure de `tg.Run` e invalidado na saída, com tratamento
explícito das janelas de reconexão (a conexão pode estar temporariamente
indisponível entre tentativas).

### Diferimento para a Wave 4

A implementação do `MediaClient` concreto e do endpoint `GET /api/media/:id/photo`
pertence à **Wave 4 (limiar-api)**, que ainda não começou (AGENTS.md §6: Fase 4 é a
API pública de consulta). A Wave 2 (processor) está completa; o próximo passo do
projeto é **validar o processor em produção** antes de avançar para a API.

## Alternativas consideradas

1. **Opção B — `runOnce` por download**: cada download abre uma conexão MTProto nova.
   **Rejeitada para o resolver em produção (HTTP concorrente)** — adiciona overhead de
   (re)conexão/re-autenticação por imagem e desvia da premissa do ADR 011 ("sessão
   compartilhada"). **Mas usada legitimamente no comando CLI one-shot `media resolve`**
   (item 6): sendo um processo isolado, `runOnce` é apropriado para validar o download
   real sem exigir o accessor persistente. A Opção A só fará falta na Wave 4 (HTTP).
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

- O `MediaClient` concreto implementa `media.MediaClient` e vive em `internal/media`? **Não** — em `internal/telegram` (§14.2).
- Nenhum tipo do gotd aparece em `internal/media` (grep de `github.com/gotd` em `internal/media` retorna vazio).
- `upload.GetFile` é disparado na mesma sessão do collector (não há segunda `telegram.NewClient`).
- Testes do resolver seguem passando com mock (a adição do cliente real não quebra os testes existentes).

## Estado atual

- **Itens 5 e 7** (pacote `internal/media`: `ImageCache` LRU+TTL, `MediaResolver` com
  singleflight + renovação L1/L2/L3): completos, 15 testes `-race`.
- **Item 6** (`MediaClient` concreto sobre gotd/td): **implementado** em
  `internal/telegram/media.go`, exposto via `limiar-collector media resolve` (Opção B /
  `runOnce`, apropriada para CLI one-shot). Valida o download real (`upload.GetFile`
  via downloader com transferência de DC e renovação L3) sem depender da Wave 4.
- **Wave 4 (restante)**: endpoint `GET /api/media/:id/photo` envolvendo o MESMO
  resolver, mas wired com a **Opção A** (sessão persistente do collector) para servir
  HTTP concorrente — `runOnce` quebraria sob concorrência (muta o campo `c.tg`).

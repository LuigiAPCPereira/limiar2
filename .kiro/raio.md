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

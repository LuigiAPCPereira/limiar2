# Diretrizes — Estendendo a Camada Telegram

A camada telegram (`internal/telegram`) é a **Fachada (Facade)** sobre o gotd/td. É
o único pacote com permissão para importar `github.com/gotd/td/...`. Tudo o que
ela expõe para o restante da base de código é um tipo de domínio (`storage.Peer`,
`HistoryMessage`) ou `[]byte` simples. Siga estas regras ao estendê-la.

## Regras

1. **Mantenha o gotd/td escondido pela fachada.** Novas capacidades do MTProto são adicionadas como
   métodos na interface `TelegramClient` e implementadas no `Client`. A
   interface não deve expor nenhum tipo `tg.*`, `auth.*` ou `session.*`.
2. **O `Client` é o único tipo que importa o gotd.** Helpers (`encodeUpdate`,
   `extractMessages`, `firstChannel`) podem usar tipos `tg` internamente, mas devem
   retornar tipos de domínio ou `[]byte`.
3. **Nunca importe gotd/td em `cli` ou `collector`.** Essas camadas dependem apenas dos modelos
   `TelegramClient` e `storage`.
4. **Chamadas de API "one-shot" rodam dentro de `runOnce`** (`c.tg.Run(ctx, f)`); o loop
   ao vivo roda no `Run`. Ambos respeitam o cancelamento de contexto.
5. **Atualizações saem do pacote como JSON.** `onUpdate` serializa via
   `encodeUpdate` e entrega os bytes ao `Dispatcher`; ele nunca deve bloquear ou
   causar crash no loop de recebimento (erros de encode são registrados no log e engolidos).
6. **Reconexão usa `CalculateBackoff`** (base de 1s, 2x, 10% de jitter, teto de 5m,
   limitado por `MaxRetries`) — veja o ADR 007.
7. **O armazenamento de sessão mapeia "ausente" para `session.ErrNotFound` do gotd** para que o fluxo
   de autenticação (auth) comece do zero; a corrupção de dados é sinalizada como `ErrSessionCorrupted`.
8. **Mutações no PeerStore passam por seus métodos RWMutex** (`Get`/`Set`/
   `LoadFromDB`/`FlushToDB`); nunca toque no map diretamente.

## Correto

```go
// Novo método da facade: apenas tipos de domínio, context-first, erros encapsulados (wrapped),
// chamada de rede dentro de runOnce.
func (c *Client) GetChannelFull(ctx context.Context, channelID, accessHash int64) ([]byte, error) {
    var payload []byte
    err := c.runOnce(ctx, func(ctx context.Context) error {
        res, err := c.tg.API().ChannelsGetFullChannel(ctx, &tg.InputChannel{
            ChannelID: channelID, AccessHash: accessHash,
        })
        if err != nil {
            return apperrors.Wrap("telegram", "get_full_channel", err)
        }
        payload, err = json.Marshal(res)        // o tipo do gotd se mantém interno
        return err
    })
    return payload, err
}
```

## Incorreto

```go
// ERRADO: vazar um tipo do gotd através da interface.
type TelegramClient interface {
    Resolve(ctx context.Context, username string) (*tg.Channel, error) // expõe tg.*
}
```

```go
// ERRADO: importar o gotd na camada collector.
package collector
import "github.com/gotd/td/tg"   // proibido fora de internal/telegram
```

```go
// ERRADO: tratar uma atualização sincronicamente no onUpdate, ignorando o dispatcher.
func (c *Client) onUpdate(ctx context.Context, u tg.UpdatesClass) error {
    return c.repo.SaveRawMessage(ctx, adapt(u))  // sem fan-out, sem recover()
}
```

## O que nunca fazer

- Adicionar um wrapper MTProto de terceiros, como o GoTGProto (ADR 002).
- Retornar ou aceitar tipos gotd/td cruzando a fronteira do `TelegramClient`.
- Gravar no banco de dados a partir dessa camada — a persistência é tarefa do storage/collector;
  o pacote telegram apenas lê/escreve as linhas de session e peer via `Repository`.
- Chamar `panic()`; o único `recover()` do sistema está no Dispatcher.

# Spec — Uso do gotd/td

`github.com/gotd/td` é a implementação MTProto sobre a qual o `limiar-collector`
é construído. Ela é usada **diretamente**, sem nenhum wrapper de terceiros. Tudo
que é específico do gotd fica confinado em `internal/telegram`; a fachada (facade)
`TelegramClient` expõe apenas tipos do domínio, de modo que o restante do código
nunca importe o gotd/td.

## Como o gotd/td é usado

### Construção do Client (`telegram/client.go`)

`Client` é o único tipo que importa o gotd/td. O client gotd é construído em
`NewClient` com nosso armazenamento de sessão e nosso dispatcher configurado como
o tratador de atualizações (update handler); nenhuma operação de rede ocorre até que
`Run` ou um método de ação seja chamado:

```go
c.tg = telegram.NewClient(appID, appHash, telegram.Options{
    SessionStorage: session,                                  // *TursoSessionStorage
    UpdateHandler:  telegram.UpdateHandlerFunc(c.onUpdate),   // encaminha para o Dispatcher
})
```

### Armazenamento de sessão (`session.Storage`)

`TursoSessionStorage` satisfaz a interface `Storage` de `github.com/gotd/td/session`
(asserção em tempo de compilação via `var _ gotdsession.Storage =
(*TursoSessionStorage)(nil)`):

- `LoadSession(ctx) ([]byte, error)` — retorna bytes persistidos, mapeando nosso
  `storage.ErrNoSession` para `session.ErrNotFound` do gotd, para que o fluxo de autenticação trate
  "nenhuma sessão" como "iniciar do zero" em vez de uma falha crítica.
- `StoreSession(ctx, data) error` — persiste os bytes da sessão via repository.

### Fluxo de Autenticação (`auth.Flow`, `auth.UserAuthenticator`)

`Client.Auth` executa o fluxo do gotd apenas se ainda não estiver autorizado:

```go
authn := newTerminalAuthenticator(os.Stdin, os.Stdout, c.log)
flow := auth.NewFlow(authn, auth.SendCodeOptions{})
err := c.tg.Auth().IfNecessary(ctx, flow)
```

`terminalAuthenticator` satisfaz `UserAuthenticator` de `github.com/gotd/td/telegram/auth`:
`Phone`, `Code`, `Password` (a etapa 2FA), mais `AcceptTermsOfService`/`SignUp`, que
rejeitam novos cadastros — a conta userbot já deve existir. `IsAuthenticated` chama
`c.tg.Auth().Status(ctx)` e relata `st.Authorized`.

### Tratamento de atualizações (`UpdateHandler`)

`Client.onUpdate(ctx, u tg.UpdatesClass)` é o ponto de entrada (entrypoint) do gotd. Ele serializa
a atualização com `encodeUpdate` e entrega os bytes JSON para `Dispatcher.Dispatch`.
Um erro de serialização é registrado no log e engolido (`return nil`), para que uma única atualização ruim
nunca derrube o loop de recebimento. A serialização para `RawMessage` ocorre
posteriormente, no adapter do collector.

### Resolução de canais (`ContactsResolveUsername`)

`Client.ResolveChannel` normaliza o username (remove `@`) e chama
`c.tg.API().ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{...})`,
extrai o primeiro `*tg.Channel` dos chats resolvidos, lê seu hash de acesso
via `GetAccessHash()`, e armazena em cache um `storage.Peer{Type: "channel", ...}`.

### Backfill de histórico (`MessagesGetHistory`)

`Client.FetchHistory` chama
`c.tg.API().MessagesGetHistory(ctx, &tg.MessagesGetHistoryRequest{Peer:
&tg.InputPeerChannel{ChannelID, AccessHash}, Limit: limit})`. `extractMessages`
lida com as variantes de resposta `*tg.MessagesChannelMessages`, `*tg.MessagesMessages`, e
`*tg.MessagesMessagesSlice`, serializando cada `*tg.Message` para JSON e pulando
mensagens de serviço. Cada resultado é uma `HistoryMessage{MessageID, Payload}`.

### Serialização (`encode.go`)

Os tipos `tg` do gotd são structs Go simples com campos exportados, então
`encoding/json.Marshal` captura o formato bruto completo. Isso é exatamente o que a Fase 1
precisa — veja o ADR 005.

## Padrões adotados

- **Facade:** toda a complexidade do gotd (sessão, peers, reconexão, auth) fica
  escondida atrás do `TelegramClient`.
- **`runOnce` para ações de "tiro único" (one-shot):** `IsAuthenticated`, `Auth`,
  `ResolveChannel` e `FetchHistory` rodam dentro de `c.tg.Run(ctx, f)` porque o status
  de auth e as chamadas de API requerem que o ciclo de vida do client gotd esteja ativo.
- **Loop de reconexão em `Run`:** na perda de conexão, `Run` aplica backoff
  exponencial (`CalculateBackoff`) e reinicia o contador de tentativas em uma conexão
  bem-sucedida; um contexto cancelado resulta em um desligamento limpo.

## O que NÃO usar

- **Sem wrapper MTProto de terceiros** (ex: GoTGProto). Seu peer/session storage
  é uma struct concreta acoplada ao GORM/SQLite e não pode ser plugada no Tursogo — veja o ADR 002.
- **Não vazar tipos do gotd/td fora de `internal/telegram`.** A interface `TelegramClient`
  retorna apenas `storage.Peer`, `HistoryMessage`, e `[]byte`. As camadas CLI e
  collector não devem importar `github.com/gotd/td/...`.
- **Não trate atualizações de forma síncrona em `onUpdate`.** Sempre despache pelo
  `Dispatcher` para aplicar fan-out e recuperação de panic.
- **Não suporte criação de conta (sign-up).** `terminalAuthenticator.SignUp` retorna
  um erro por design.

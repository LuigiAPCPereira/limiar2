# ADR 002 — Usar gotd/td Diretamente, Sem Wrapper MTProto

## Status

Aceito

## Contexto

Precisamos de um cliente MTProto para operar como um userbot do Telegram. Uma escolha comum é
um wrapper de alto nível como o GoTGProto, que empacota o armazenamento de sessão e peers.
No entanto, o `PeerStorage` (e o armazenamento de sessão) do GoTGProto é uma struct concreta
acoplada ao GORM/SQLite — não é uma interface plugável. Como o `limiar-collector`
usa Tursogo como seu único datastore (ADR 001), um wrapper que exija
GORM/SQLite forçaria um segundo caminho de persistência incompatível.

`github.com/gotd/td` expõe as "costuras" (seams) corretas: `session.Storage` para persistência
de sessão e um fluxo de autenticação flexível, nos permitindo apoiar ambos com o Tursogo.

## Decisão

Construir diretamente sobre `gotd/td`. Implementar nosso próprio armazenamento de sessão
(`TursoSessionStorage` que satisfaz `session.Storage`), cache de peers (`PeerStore`),
autenticador (`terminalAuthenticator` que satisfaz `auth.UserAuthenticator`), e
dispatcher. Todo o uso do gotd/td fica confinado ao pacote `internal/telegram`, atrás da
fachada (facade) `TelegramClient`.

## Consequências

- A sessão, peers, canais e mensagens são todos persistidos no mesmo arquivo Tursogo —
  sem que uma dependência de GORM/SQLite se infiltre.
- Somos donos do código de sessão/peer/auth, o que significa mais código, mas controle
  total sobre o armazenamento, reconexão e serialização.
- Tipos do gotd/td ficam isolados em um único pacote; o restante do código permanece
  desacoplado (veja o ADR sobre o limite da facade e `docs/specs/GOTD-TD.md`).
- Nós fixamos a versão do gotd/td e adaptamos apenas `session.go`/`client.go` caso
  suas interfaces mudem.

## Alternativas consideradas

- **GoTGProto** — seu armazenamento concreto acoplado a GORM/SQLite não pode ser plugado
  no Tursogo; rejeitado.
- **Outros wrappers de alto nível** — mesmo risco de armazenamento opinativo e dependências
  ocultas; rejeitados em favor de um controle explícito.
- **Uma linguagem/biblioteca diferente** — fora de escopo; o pipeline é em Go.

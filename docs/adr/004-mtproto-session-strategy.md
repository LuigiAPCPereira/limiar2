# ADR 004 — Sessão MTProto Persistida no Tursogo

## Status

Aceito

## Contexto

Um userbot deve se autenticar uma vez e se reconectar posteriormente sem inserir as
credenciais novamente. O gotd/td abstrai a persistência de sessão atrás da interface
`Storage` em `github.com/gotd/td/session` (`LoadSession`/`StoreSession`).
Precisamos de um lugar para manter o blob dessa sessão. Conforme o ADR 001, o Tursogo é o
único datastore (armazenamento de dados), então a sessão deve residir lá em vez de num
arquivo separado.

## Decisão

Implementar o `TursoSessionStorage` para satisfazer a interface `session.Storage` do gotd,
apoiando-o com o `storage.Repository` (que usa o Tursogo). A sessão é armazenada como uma única
linha (row) na tabela `sessions`, imposta por `CHECK (id = 1)` para que nunca haja mais
de uma sessão. O `SaveSession` faz upsert (`ON CONFLICT(id) DO UPDATE`), tornando a
re-autenticação idempotente.

O `LoadSession` mapeia o `ErrNoSession` do nível de armazenamento para o `session.ErrNotFound`
do gotd, o qual o fluxo de autenticação trata como "iniciar do zero". Uma sessão
que não puder ser decodificada resultará em `ErrSessionCorrupted`.

## Consequências

- Sessão, peers, canais e mensagens compartilham um único arquivo transacional; fazer
  o backup do `.db` também faz o backup do estado da autenticação.
- O comando `auth` é idempotente: executá-lo com uma sessão válida não tem efeito, e
  `SELECT COUNT(*) FROM sessions` permanece 1 (Requisito 1.4/1.5).
- O formato trafegado na rede é o que o gotd serializar; nós armazenamos bytes opacos e
  nunca os inspecionamos. Os bytes da sessão são tratados como dados sensíveis (omitidos nos
  logs).

## Alternativas consideradas

- **Armazenamento de sessão em arquivo** (`session.FileStorage` do gotd) — um segundo caminho
  de persistência fora do Tursogo; rejeitado em prol de coesão e simplicidade de operações.
- **Tabela de sessões de múltiplas linhas** — desnecessário; um userbot possui exatamente uma
  sessão, e o `CHECK` de linha única torna a idempotência trivial.
- **Apenas em memória** — exigiria re-autenticação a cada inicialização;
  inaceitável para um serviço de longa duração.

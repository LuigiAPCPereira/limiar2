# L3 ADR 001 — Credential/session storage hardened em arquivo local

Authority: Decision Record — Limiar 3.0
Status: Accepted
Accepted-by: Mantenedor do Limiar
Accepted-at: 2026-09-23
Acceptance-reference: `baa626b59ec21d36f0dbde9a8a1fb3bbb5a4a2a6`

## Contexto

A autorização Telegram persistida é uma credencial de alto impacto. L3-001/A/B/C separaram essa autoridade de Evidence, peer state, update state e MCP auth.

O contract upstream correto já existe em `gotd/session.Storage`. A auditoria L3-001C confirmou que o `session.FileStorage` upstream é deliberadamente simples e não oferece as garantias de publicação/durabilidade/permissions exigidas pelo threat model inicial do Limiar.

A Evidence histórica relevante inclui F-STO-006, EXP-LIMIAR-018, EXP-LIMIAR-019 e o antigo ADR 023 `Proposed`. Esses artefatos informam esta Decision, mas não são promovidos por ela.

## Decision

Para a primeira fundação do Limiar 3, a credencial Telegram será persistida em **arquivo local hardened**, privado ao Telegram boundary, satisfazendo diretamente `github.com/gotd/td/session.Storage`.

Escopo inicial suportado:

- Linux;
- single-host;
- single-process por `TelegramAuthorizationIdentity`;
- filesystem local com semântica de replace/rename compatível com o contract validado;
- usuário de serviço dedicado e diretório pai privado.

### Propriedades obrigatórias

1. um destino por `TelegramAuthorizationIdentity`/configuração autorizada;
2. destino separado do SQLite/Evidence e do peer cache;
3. diretório pai privado e ownership esperado;
4. destino deve ser arquivo regular; symlink/non-regular é erro fail-closed;
5. temporário exclusivo criado no mesmo diretório/filesystem;
6. proteção privada estabelecida antes da publicação (`0600` ou proteção equivalente no escopo Linux);
7. escrita completa e verificada;
8. sync do arquivo antes da publicação;
9. replace/rename atômico conforme contrato da plataforma;
10. sync do diretório quando aplicável para a garantia assumida;
11. coordenação intra-processo compartilhada por path;
12. cleanup de temporários em falhas;
13. falha de permission/I/O/corrupção não pode ser convertida em 'sessão ausente';
14. bytes de sessão nunca entram em logs, erros, fixtures ou métricas.

## Não decidido / fora do contrato

Esta Decision não exige nem promete:

- Windows;
- multi-process sharing ou failover;
- network filesystem;
- lock cross-process;
- KMS/Vault;
- application-level encryption;
- backup automático da credencial;
- remote secret store;
- peer cache no mesmo storage;
- parsing do blob gotd pelo Limiar.

Se threat model/deployment mudar, ampliar o contrato requer Evidence e nova Decision proporcional.

## Relação com bootstrap/runtime

Este ADR decide persistência, não login. Bootstrap administrativo, fail-closed steady-state, ownership do client e semantic readiness pertencem ao L3 ADR 002.

Delete do arquivo local não equivale a revogação remota da autorização Telegram.

## Gates de implementação

Antes de considerar a implementação validada:

- arquivo novo e overwrite;
- arquivo preexistente permissivo;
- symlink/non-regular destination;
- temp/write/fsync/rename/dir-sync failure injection;
- ENOSPC/read-only filesystem;
- writers concorrentes intra-processo;
- `go test -race`;
- nenhum secret em logs/errors;
- gotd real: bootstrap → persist → shutdown → processo novo → restore → same self → read-only RPC → zero reauth;
- registrar frequência/latência real de `StoreSession`.

## Consequências

O Limiar mantém uma authority adicional pequena de persistência, mas evita acoplar credencial a Evidence ou inventar um banco apenas para um blob mutável.

A implementação pode usar APIs modernas do Go apropriadas ao toolchain aceito, mas o contract acima — e não uma API específica — é a autoridade.
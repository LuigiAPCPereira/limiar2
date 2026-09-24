# EXP-LIMIAR-018 — Boundary de sessão MTProto em arquivo

Authority: Non-authoritative
Status: Rejected

## Hipótese

`github.com/gotd/td/session.FileStorage` v0.161.0, usado **as-is**, é suficiente como
candidato de persistência da sessão MTProto do Limiar sem transportar o mecanismo de
persistência legado.

O experimento não decide a estratégia final. Ele tenta falsificar o candidato mais
simples antes de justificar uma implementação própria ou um storage dedicado.

## Origem

O registry da rebaseline classifica o ADR 004 como `RETAIN-PRINCIPLE / REWRITE`: a
necessidade de sessão MTProto durável permanece, mas o mecanismo de persistência histórico
não é automaticamente preservado.

F-STO-006, integrado pela PR #200, separa sessão e peer cache como autoridades distintas.
Sessão é material de credencial sensível; peer cache é estado operacional reconstruível e
possui outro ciclo de vida.

`PROPOSAL-STO-001` já mantém sessão fora do escopo do banco SQLite de Evidence: o
boundary de segredo precisa ser decidido separadamente.

## Evidence primária antes da execução

Versões do repositório:

- Go 1.26.2;
- `github.com/gotd/td v0.161.0`.

No commit correspondente ao tag `v0.161.0`, o upstream do gotd:

- define `session.Storage` como `LoadSession`/`StoreSession` sobre bytes;
- alerta que comprometimento da sessão pode permitir autenticação como usuário/bot e, em
  determinadas situações, decriptação de mensagens anteriores;
- implementa `FileStorage` com mutex por **instância**;
- implementa `StoreSession` por `os.WriteFile(path, data, 0600)`;
- mantém TODO explícito para substituir essa escrita por mecanismo robusto/rename.

No Go 1.26.2, a documentação/implementação de `os.WriteFile` é explícita:

- `perm` é usado na criação do arquivo;
- se o arquivo já existe, ele é truncado sem alteração de permissões;
- uma falha no meio da operação pode deixar o arquivo parcialmente escrito.

O gotd testa `FileStorage` apenas através do contrato básico de ausência + save/load. O
upstream não fornece Evidence, nesse teste, sobre permissões preexistentes, escrita
atômica diante de falha ou coordenação entre instâncias independentes.

## Escopo executável

Arquivo de harness:

`internal/telegram/session_file_storage_exp_test.go`

O harness é somente `_test.go`; não altera runtime, defaults, schema, configuração nem
API de produção.

Ele verifica no ambiente em execução:

1. ausência retorna `session.ErrNotFound`;
2. bytes opacos sobrevivem a store/load e a uma nova instância apontando para o mesmo
   path;
3. overwrite substitui o payload anterior;
4. em Unix, arquivo novo é criado com permissões `0600`;
5. em Unix, arquivo preexistente explicitamente `0644` permanece `0644` após
   `StoreSession` — comportamento esperado da implementação upstream atual e Evidence de
   que `0600` no call site não funciona como permission hardening de arquivo existente.

Os testes de modo POSIX fazem `Skip` no Windows porque bits `0600` não equivalem a uma
garantia de ACL Windows. EXP-LIMIAR-017 validou execução de `internal/storage/sqlite` em
worker Travis Windows AMD64 com Go 1.26.2 e `CGO_ENABLED=0`, mas não testou ACLs de arquivo
nem estabeleceu equivalência com permissões POSIX.

## O que este experimento não tenta provar

Este EXP não tenta fabricar Evidence de power-loss ou atomicidade de `os.WriteFile` por
um teste probabilístico. A própria API do Go documenta que falha intermediária pode deixar
escrita parcial; uma execução verde normal não pode contradizer esse contrato.

Também não conclui que duas instâncias de `FileStorage` necessariamente corromperão uma
sessão. O que é conhecido é mais estreito:

- cada instância possui mutex próprio;
- o Limiar atual pode manter clients Telegram independentes para collector e mídia;
- `gotd.Client.saveSession` executa um ciclo Load → modificar Config/AuthKey/DC/Salt →
  Save quando recebe sessão primária.

Portanto coordenação entre clients é requisito a ser tratado pela estratégia futura, mas
corrupção concreta não é declarada sem Evidence executável.

## Critério

A hipótese de suficiência **as-is** só pode ser `Supported` se o candidato satisfizer
simultaneamente o contrato funcional e o boundary de segurança/failure semantics exigido
para credencial sensível.

Em particular, um candidato que:

- não consiga garantir/reparar permissões privadas de um arquivo preexistente no ambiente
  Unix observado; ou
- possua contrato explícito permitindo arquivo parcialmente escrito em falha intermediária
  sem uma camada adicional de publicação atômica/recovery

não é suficiente **as-is**, mesmo que round-trip e overwrite normais passem.

Isso não rejeita uma estratégia **baseada em arquivo** com camada própria de hardening.
Rejeita somente equiparar `session.FileStorage` upstream, sem adaptação, ao boundary final
do Limiar.

## Execução

A PR #201 foi revalidada sobre `main` já contendo F-STO-006. O HEAD executado foi
`50dbe308e8b943bc5308d7f25480eed16435b46f`.

Travis build `278809467`, concluído em 2026-09-13, executou a matriz de 13 jobs com sucesso.
O job `Linux X64 / race` executou a suíte normal com Go 1.26.2, Linux Noble e
`CGO_ENABLED=1`, incluindo o harness deste experimento.

No ambiente Unix observado, os asserts do harness confirmaram simultaneamente:

- ausência como `session.ErrNotFound`;
- round-trip, reopen e overwrite funcionais;
- arquivo novo criado com modo `0600`;
- arquivo preexistente explicitamente ajustado para `0644` permaneceu `0644` após
  `StoreSession`.

Os jobs Windows permaneceram verdes porque os asserts POSIX foram corretamente pulados;
isso não produz Evidence sobre ACLs Windows.

## Resultado

`Rejected`.

A Evidence executável confirma o comportamento que falsifica a hipótese: apesar do
contrato funcional básico funcionar, `session.FileStorage` v0.161.0 **as-is** não corrige
permissões de um arquivo preexistente no ambiente Unix observado. Além disso, seu
`StoreSession` continua baseado em `os.WriteFile`, cujo contrato permite escrita parcial
em falha intermediária.

Para uma credencial MTProto, round-trip funcional não é suficiente para considerar esse
candidato o boundary final sem adaptação.

## Limitações e consequência

A rejeição é estritamente do `session.FileStorage` upstream usado **as-is**. Ela não:

- rejeita uma estratégia baseada em arquivo com camada própria de hardening;
- escolhe SQLite para sessão;
- demonstra corrupção entre duas instâncias concorrentes;
- estabelece ACL adequada para Windows;
- autoriza mudança de produção.

A próxima hipótese útil, se mantida a estratégia de arquivo, é falsificar a menor camada
hardened capaz de garantir permissões privadas quando aplicável, publicação/recovery segura
e coordenação coerente entre clients. Essa hipótese deve permanecer experimental até haver
Evidence suficiente para uma Proposal/Decision de session storage.

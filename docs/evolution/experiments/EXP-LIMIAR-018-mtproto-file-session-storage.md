# EXP-LIMIAR-018 — Boundary de sessão MTProto em arquivo

Authority: Non-authoritative
Status: In Progress

## Hipótese

`github.com/gotd/td/session.FileStorage` v0.161.0, usado **as-is**, é suficiente como
candidato de persistência da sessão MTProto do Limiar sem transportar o mecanismo
Tursogo legado.

O experimento não decide a estratégia final. Ele tenta falsificar o candidato mais
simples antes de justificar uma implementação própria ou um storage dedicado.

## Origem

O registry da rebaseline classifica o ADR 004 como `RETAIN-PRINCIPLE / REWRITE`: a
necessidade de sessão MTProto durável permanece, mas o mecanismo Tursogo histórico não é
automaticamente preservado.

A investigação registrada por F-STO-006 (em revisão na PR #200 no início deste
experimento) separa sessão e peer cache como autoridades distintas. Sessão é material de
credencial sensível; peer cache é estado operacional reconstruível e possui outro
lifecycle.

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
garantia de ACL Windows; EXP-LIMIAR-017 já mostrou que essa equivalência não pode ser
presumida.

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

## Resultado atual

`In Progress`.

O harness foi materializado, mas os gates do HEAD deste experimento ainda precisam ser
executados. A Evidence de source acima já limita o que um resultado verde normal pode
provar; a conclusão só será atualizada após o harness executar no ambiente registrado.

## Próximo gate

Executar os gates normais do repositório, incluindo `go test -race ./...`, e registrar o
ambiente real do job que exercitou o harness.

Se o comportamento observado confirmar que arquivo preexistente `0644` não é restringido,
a hipótese de suficiência **as-is** deve ser marcada `Rejected`. O próximo experimento
deverá então comparar a menor camada de arquivo hardened (permissões explícitas +
publicação segura) com alternativas somente se a complexidade adicional for justificada;
não adotar SQLite de sessão por default apenas porque SQLite já existe para Evidence.

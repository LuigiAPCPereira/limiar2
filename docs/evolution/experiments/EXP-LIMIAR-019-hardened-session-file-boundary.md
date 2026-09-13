# EXP-LIMIAR-019 — Boundary hardened de sessão MTProto em arquivo

Authority: Non-authoritative
Status: In Progress

## Hipótese

Uma camada de arquivo mínima, ainda experimental, consegue corrigir no ambiente Unix observado as duas insuficiências diretamente demonstradas pelo EXP-LIMIAR-018 sem introduzir um storage de produção novo:

1. reparar permissões privadas ao substituir um arquivo de sessão preexistente;
2. publicar o novo conteúdo por arquivo temporário no mesmo diretório + `fsync` + `rename`, serializando writers independentes do mesmo processo por path.

A hipótese é deliberadamente mais estreita do que “arquivo é a estratégia final”. Ela não decide storage, não escolhe API pública e não autoriza substituir `TursoSessionStorage`.

## Origem

F-STO-006 separou sessão MTProto de peer cache e registrou que clients Telegram independentes podem compartilhar a mesma persistência de sessão.

EXP-LIMIAR-018 rejeitou `github.com/gotd/td/session.FileStorage` v0.161.0 **as-is** como boundary final porque, no Linux observado:

- um arquivo preexistente `0644` permaneceu `0644` após `StoreSession`;
- a implementação upstream usa `os.WriteFile`, cujo contrato permite conteúdo parcialmente escrito quando ocorre falha intermediária.

O resultado não rejeitou uma estratégia baseada em arquivo com camada hardened e não escolheu SQLite de sessão.

`PROPOSAL-STO-001` também mantém sessão MTProto fora do escopo decidido para o SQLite de Evidence; credencial continua sendo um boundary separado.

## Escopo executável

Harness:

`internal/telegram/session_hardened_file_exp_test.go`

O tipo `hardenedFileStorageExp` existe somente em `_test.go`. Ele não satisfaz nem é conectado ao runtime de produção.

No ambiente Unix, o harness experimental:

1. cria o temporário no mesmo diretório do destino;
2. força `0600` no temporário;
3. grava o payload e executa `Sync` antes da publicação;
4. publica com `os.Rename`;
5. reafirma `0600` no target e sincroniza o diretório;
6. usa um mutex compartilhado por path no processo, de modo que duas instâncias independentes do harness não publiquem simultaneamente no mesmo arquivo.

## Testes

### Permissão preexistente

Cria um arquivo de sessão `0644`, executa `store` e exige:

- payload novo íntegro;
- modo final `0600` no Unix observado.

### Writers independentes no mesmo processo

Duas instâncias distintas apontam para o mesmo path e escrevem payloads grandes concorrentes. O resultado final precisa ser exatamente um dos payloads completos, nunca conteúdo parcial/misto.

Esse teste demonstra somente serialização intra-processo proporcionada pelo lock compartilhado do harness. Não demonstra coordenação entre processos.

### Publicação limpa

Depois de uma publicação bem-sucedida, nenhum temporário `.limiar-session-*` deve permanecer no diretório.

## Critério

A hipótese será `Supported` somente se os gates executáveis no ambiente Unix observado confirmarem simultaneamente:

- hardening de arquivo preexistente para `0600`;
- payload final completo sob writers independentes no mesmo processo;
- ausência de temporários após publicação bem-sucedida;
- race detector verde para o harness.

Será `Rejected` se qualquer uma dessas propriedades falhar no ambiente testado.

Será `Inconclusive` se o ambiente não conseguir executar os testes relevantes.

## Limitações explícitas

Mesmo um resultado `Supported` **não** prova:

- ACL Windows equivalente a `0600`;
- replace/rename seguro no Windows;
- coordenação entre processos diferentes;
- sobrevivência a power-loss durante ou depois do `fsync`;
- proteção criptográfica do arquivo em repouso;
- integração correta com `gotd.Client` real;
- política de backup/restore de segredo;
- que uma implementação baseada em arquivo é superior a outra alternativa.

Os testes POSIX/rename de overwrite fazem `Skip` no Windows. Isso é uma limitação registrada, não Evidence positiva sobre Windows.

## Relação com produção

Nenhum arquivo de produção é alterado. O experimento não muda defaults, schema, configuração, source of truth, API pública ou dependências.

Uma eventual implementação de produção continua exigindo Proposal/Decision proporcional ao boundary de segurança e à estratégia de sessão escolhida.

## Resultado atual

`In Progress`.

O harness foi materializado, mas ainda não há resultado de CI deste HEAD. Um build verde anterior do EXP-LIMIAR-018 não conta como Evidence deste experimento.

## Próximo gate

Executar os gates normais do repositório, em especial `go test -race ./...`, e registrar exatamente o ambiente que exercitou os asserts Unix.

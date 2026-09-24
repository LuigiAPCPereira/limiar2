# F-STO-003 — Gate de dependências do CI afirma authority arquitetural obsoleta

Authority: Non-authoritative
Type: Finding
Status: Confirmed

## Evidência

O workflow `.github/workflows/ci.yml` contém o job GitHub-hosted `build-and-test` e um step chamado `Ensure Strict Dependencies (No ORMs/SQLite drivers)`.

O step declara em texto que drivers SQLite não-Turso são "estritamente proibidos pela arquitetura" e associa essa regra ao `AGENTS.md`. A lista mecânica atual bloqueia imports diretos de `github.com/mattn/go-sqlite3`, `modernc.org/sqlite` e `gorm.io/gorm`; ela não contém `github.com/ncruces/go-sqlite3`.

No momento da constatação, o ADR 001 legado estava em transição `SUPERSEDE` e o ADR 019 ainda estava `Proposed`, propondo `ncruces/go-sqlite3` para o novo storage. Na Rebaseline 2026, implementação, testes, defaults e CI são Evidence, não Decision.

## Finding

O texto do gate mistura enforcement de uma baseline histórica com uma afirmação de authority que ele não possui.

Isso cria dois riscos diferentes:

1. um agente pode interpretar o CI como prova de que Tursogo continua arquiteturalmente obrigatório;
2. uma futura Decision aceita pode ser contradita por mensagem/gate legado sem que o conflito seja percebido como drift de implementação.

A ausência de `ncruces/go-sqlite3` na lista proibida não resolvia o problema epistemológico: o texto continuava atribuindo authority arquitetural ao workflow.

## Consequência

Este Finding, sozinho, não autorizava mudança no CI.

Enquanto o ADR 019 permanecesse `Proposed`, o workflow deveria continuar intacto. Se uma Decision posterior aceitasse um novo baseline de storage, o CI deveria ser reconciliado com essa authority e o enforcement deveria validar somente constraints realmente aceitas.

## Resolução

O mantenedor aceitou o ADR 019 em 2026-09-08; a aceitação foi integrada ao `main` pela PR #173 (`62ea1ab13074c9e4329027a1178c8b703ea16243`). A Decision passou a autorizar `github.com/ncruces/go-sqlite3` para o novo storage e a permitir Tursogo somente onde ainda necessário ao legado/importação.

A PR #175 reconcilia o gate derivado com essa nova authority: remove a afirmação de exclusividade do Tursogo, deixa explícito o baseline aceito e mantém apenas a rejeição de dependências estruturais alternativas não autorizadas. O Finding permanece `Confirmed` porque registra uma inconsistência histórica real; esta seção registra sua resolução sem apagar a Evidence original.

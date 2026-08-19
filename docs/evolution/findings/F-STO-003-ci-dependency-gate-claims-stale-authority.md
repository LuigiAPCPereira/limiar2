# F-STO-003 — Gate de dependências do CI afirma authority arquitetural obsoleta

Authority: Non-authoritative
Type: Finding
Status: Confirmed

## Evidência

O workflow `.github/workflows/ci.yml` contém o job GitHub-hosted `build-and-test` e um step chamado `Ensure Strict Dependencies (No ORMs/SQLite drivers)`.

O step declara em texto que drivers SQLite não-Turso são "estritamente proibidos pela arquitetura" e associa essa regra ao `AGENTS.md`. A lista mecânica atual bloqueia imports diretos de `github.com/mattn/go-sqlite3`, `modernc.org/sqlite` e `gorm.io/gorm`; ela não contém `github.com/ncruces/go-sqlite3`.

Na Rebaseline 2026, implementação, testes, defaults e CI são Evidence, não Decision. O ADR 001 legado está em transição `SUPERSEDE`, enquanto o ADR 019 propõe — ainda sem aceitar — `ncruces/go-sqlite3` para o novo storage.

## Finding

O texto do gate mistura enforcement de uma baseline histórica com uma afirmação de authority que ele não possui.

Isso cria dois riscos diferentes:

1. um agente pode interpretar o CI como prova de que Tursogo continua arquiteturalmente obrigatório;
2. uma futura Decision aceita pode ser contradita por mensagem/gate legado sem que o conflito seja percebido como drift de implementação.

A ausência atual de `ncruces/go-sqlite3` na lista proibida não resolve o problema epistemológico: o texto continua atribuindo authority arquitetural ao workflow.

## Consequência

Nenhuma mudança no CI é autorizada por este Finding.

Enquanto o ADR 019 permanecer `Proposed`, o workflow atual deve continuar intacto. Se uma Decision posterior aceitar um novo baseline de storage, o CI deverá ser reconciliado com essa authority e o enforcement deve validar somente constraints realmente aceitas.

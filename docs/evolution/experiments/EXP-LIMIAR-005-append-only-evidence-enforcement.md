# EXP-LIMIAR-005 — Enforcement append-only de Evidence

Authority: Non-authoritative
Status: In Progress

## Hipótese

O envelope físico de Evidence validado pelo EXP-LIMIAR-004 pode ser protegido contra mutação e remoção acidentais usando duas camadas complementares no boundary experimental:

1. capability estreita que oferece somente append ao consumidor normal;
2. triggers SQLite persistentes que rejeitam `UPDATE` e `DELETE` mesmo quando código interno obtém acesso SQL bruto.

Essa combinação pode tornar a invariante append-only observável e fail-closed sem alterar o storage de produção nem promover o ADR 019.

## Origem

O EXP-LIMIAR-004 terminou como `Supported`, mas deixou explicitamente aberto o gate de enforcement físico ou capability-level de append-only.

A hipótese também exercita a invariante constitucional C-01: Evidence admitida não deve desaparecer silenciosamente.

## Escopo

Este experimento é restrito ao módulo isolado `experiments/evidence-envelope`.

Ele testa:

- append inicial via capability estreita;
- rejeição física de `UPDATE`;
- rejeição física de `DELETE`;
- preservação byte a byte do payload/hash após tentativas rejeitadas;
- possibilidade de novos appends depois de mutações rejeitadas;
- persistência dos guards após close/reopen;
- `PRAGMA integrity_check` após reopen.

Fora de escopo:

- mudança no schema ou writer de produção;
- aceitar ADR 019;
- autorização/ACL contra atores externos com permissão de alterar o arquivo SQLite;
- proteção contra `DROP TRIGGER`, `DROP TABLE`, corrupção do arquivo ou acesso offline;
- política de retenção/redação legal;
- importação do banco legado;
- migrations finais do novo banco.

## Harness

Arquivo:

`experiments/evidence-envelope/append_only_test.go`

Os guards avaliados são triggers `BEFORE UPDATE` e `BEFORE DELETE` com `RAISE(ABORT, ...)`. A capability experimental `evidenceAppender` só oferece a operação `Append`.

## Critérios

A hipótese é suportada se:

1. Evidence puder ser anexada normalmente;
2. `UPDATE` direto retornar erro e não modificar payload/hash;
3. `DELETE` direto retornar erro e não remover a Evidence;
4. um segundo append funcionar depois dos rejects;
5. após reopen, `UPDATE` e `DELETE` continuarem bloqueados;
6. `PRAGMA integrity_check` retornar `ok`;
7. `go vet`, runtime sem CGO e race detector passarem no módulo experimental.

## Interpretação antecipada

Mesmo que todos os critérios passem, o resultado demonstrará somente enforcement defensivo no boundary testado. Triggers não constituem um boundary de segurança contra um ator que controla o arquivo ou possui capacidade de executar DDL arbitrário.

Um resultado `Supported` também não decide que triggers são obrigatórias no schema final; ele apenas prova que a combinação capability estreita + guard físico é viável e detecta mutações acidentais de forma fail-closed.

## Resultado

Pendente de execução dos gates do branch experimental.

# EXP-LIMIAR-005 — Enforcement append-only de Evidence

Authority: Non-authoritative
Status: Supported

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

## Resultado

Supported.

No head `0a567c6e8df8656f11a739b541df9cfffa6b8b2e`, o workflow dedicado `EXP-LIMIAR-005` executou com sucesso todos os gates do módulo experimental:

- `go vet` — PASS;
- `CGO_ENABLED=0 go test` — PASS;
- `go test -race` — PASS.

O harness confirmou que:

- append via capability estreita funciona;
- `UPDATE` direto é rejeitado pelo guard físico;
- `DELETE` direto é rejeitado pelo guard físico;
- payload e hash permanecem preservados após as tentativas rejeitadas;
- novos appends continuam possíveis após os rejects;
- os guards sobrevivem a close/reopen;
- `PRAGMA integrity_check` retorna `ok` após reopen.

A CI geral do repositório também concluiu com sucesso no mesmo head, incluindo lint, gosec, `go vet`, build e testes com race detector/coverage.

## Interpretação

A hipótese foi suportada no boundary experimental: capability estreita + triggers persistentes é uma defesa viável para tornar mutações acidentais de Evidence observáveis e fail-closed.

Isso não transforma os triggers em boundary de segurança contra um ator com controle do arquivo SQLite ou capacidade DDL, e não decide que esse mecanismo deva integrar o schema final. O resultado permanece Evidence não autoritativa e não promove nem aceita o ADR 019.

## Próximo gate

Com o enforcement append-only demonstrado, o próximo gap conhecido de maior valor antes de qualquer migração de produção é validar importação side-by-side de uma cópia legado para o envelope experimental preservando identidade, bytes/hash e possibilidade de auditoria/reconciliação, sem alterar o banco de origem.

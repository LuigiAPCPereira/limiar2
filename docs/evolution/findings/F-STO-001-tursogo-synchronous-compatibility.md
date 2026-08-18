# F-STO-001 — `synchronous=NORMAL` não é contrato documentado do Tursogo v0.7.2

Authority: Non-authoritative
Status: Confirmed

## Finding

O storage atual do Limiar executa `PRAGMA synchronous=NORMAL` em `internal/storage/db.go`.

A matriz oficial de compatibilidade do Turso declara `PRAGMA synchronous` como suporte
parcial, com apenas `OFF` e `FULL` documentados.

No contract harness executado em 2026-08-18 contra `tursogo v0.7.2`, o comando
`synchronous=NORMAL` foi aceito e o readback retornou `1` (NORMAL). Isso demonstra
comportamento runtime, mas não transforma uma semântica não documentada em contrato de
produção.

## Evidência

- código atual: `internal/storage/db.go`;
- Turso `COMPAT.md`: https://github.com/tursodatabase/turso/blob/main/COMPAT.md;
- `EXP-LIMIAR-002`;
- execução self-hosted `agent-runtime` workflow run `32183259646`.

## Impacto

A configuração atual depende de um comportamento que o upstream não promete. Para o
novo storage, o baseline de durabilidade deve usar somente comportamento documentado e
testado.

`PRAGMA synchronous=FULL` passou no mesmo harness para Tursogo v0.7.2 e
`ncruces/go-sqlite3 v0.35.3`.

## Conclusão

`NORMAL` não deve ser carregado para a nova arquitetura apenas por compatibilidade com o
código legado. O candidato atual de PRAGMAs usa `WAL + FULL + foreign_keys=ON +
busy_timeout`, sujeito à Decision de storage.

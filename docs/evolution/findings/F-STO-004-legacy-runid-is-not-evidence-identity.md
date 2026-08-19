# F-STO-004 — RunID legado não satisfaz o contrato de identidade de Evidence

Authority: Non-authoritative
Type: Finding
Status: Confirmed

## Evidência

`internal/id/runid.go` gera `RunID` com 8 bytes aleatórios (`crypto/rand`) codificados em 16 caracteres hexadecimais.

Se a leitura de `crypto/rand` falha, o helper retorna silenciosamente `0000000000000000`.

## Finding

Esse helper não deve ser reutilizado por conveniência para `EvidenceRecord.id`.

Para Evidence, a identidade precisa sobreviver export/import/migração, não depender de `rowid` local e falhar fechado se a fonte de entropia necessária à geração do ID falhar. Um valor fixo de fallback criaria colisão deliberada e poderia confundir registros distintos.

Além disso, a Evidence não precisa derivar ordenação temporal do identificador: tempo de aquisição/observação é um fato próprio do envelope e deve permanecer explícito.

## Consequência

Este Finding não escolhe UUID, ULID, BLOB ou TEXT.

O próximo experimento deve comparar uma identidade aleatória de 128 bits com alternativas temporais/locais e verificar round-trip, export/import, constraints físicas e a relação com `ROWID`/`WITHOUT ROWID` antes de uma Proposal de schema.

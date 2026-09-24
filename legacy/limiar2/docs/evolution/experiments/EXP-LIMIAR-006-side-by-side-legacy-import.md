# EXP-LIMIAR-006 — Importação side-by-side do legado

Authority: Non-authoritative
Status: Supported

## Hipótese

Uma cópia do storage legado pode ser lida em modo read-only e importada para o envelope experimental de Evidence sem alterar o arquivo de origem, preservando bytes/hash e mantendo uma trilha explícita de reconciliação entre a linha histórica e a nova Evidence.

O retry da mesma origem pode ser idempotente por meio de um ledger experimental de importação, sem promover `(channel_id, message_id)` ou o `raw_messages.id` a identidade canônica da nova Evidence.

## Origem

O EXP-LIMIAR-005 suportou enforcement append-only no boundary experimental. O ADR 019, ainda `Proposed`, estabelece como direção proposta que um eventual banco novo nasça side-by-side, que o legado permaneça preservado/read-only durante importação/rollback e que a política/ferramenta/ledger de importação sejam tratadas separadamente.

O schema legado real do repositório persiste `raw_messages` com `id`, `channel_id`, `message_id`, `payload`, `received_at` e `schema_version`, com unicidade em `(channel_id, message_id)`.

## Escopo

Este experimento permanece no módulo isolado `experiments/evidence-envelope`.

Ele avalia:

- leitura do arquivo legado via conexão `mode=ro`;
- preservação do SHA-256 do arquivo de origem antes/depois da importação;
- preservação byte a byte de `raw_messages.payload`;
- hash SHA-256 do payload no envelope e no ledger;
- geração de uma nova identidade UUIDv4 para Evidence;
- ledger que preserva `source_sha256`, `raw_message_id`, `channel_id` e `message_id` como proveniência, não como identidade canônica;
- importação de Evidence + entrada do ledger na mesma transação do banco novo;
- retry da mesma origem sem duplicar Evidence;
- `PRAGMA integrity_check` no destino.

Fora de escopo:

- alterar o storage de produção;
- aceitar/promover ADR 019;
- migrar o banco legado in-place;
- importar `processed_messages`, sessão, peers, channels ou caches;
- decidir o schema final do ledger;
- decidir estratégia final para timestamps ou payload codec;
- remover Tursogo;
- afirmar compatibilidade com uma cópia real de produção antes de executar esse gate.

## Harness

Arquivo:

`experiments/evidence-envelope/legacy_import_test.go`

O harness cria uma fixture usando o shape atual de `channels` + `raw_messages` da migration legada, fecha o arquivo, calcula seu SHA-256 e reabre a origem em modo read-only. O destino usa o envelope experimental já validado, os guards append-only do EXP-LIMIAR-005 e um ledger experimental.

A fixture cobre payload JSON textual e bytes UTF-8 não-ASCII para detectar alteração incidental de conteúdo.

## Critérios

A hipótese fica suportada neste primeiro boundary se:

1. a origem abrir em modo read-only;
2. o hash do arquivo legado permanecer idêntico antes/depois;
3. cada payload importado for byte-identical ao payload histórico;
4. hashes do envelope e do ledger coincidirem com o payload histórico;
5. a proveniência legado -> Evidence puder ser reconciliada pelo ledger;
6. Evidence e ledger forem escritos atomicamente no destino;
7. retry da mesma origem importar zero novas Evidence;
8. `integrity_check` retornar `ok`;
9. `go vet`, runtime sem CGO e race detector passarem no módulo experimental.

## Limitação material

O repositório não contém um arquivo `.db` legado real versionado. Portanto, este harness valida o contrato contra uma fixture construída a partir do schema legado real do repositório, mas **não fecha ainda o gate de compatibilidade contra uma cópia real de produção**.

Esse segundo gate deve usar uma cópia descartável/read-only do arquivo histórico, nunca o original operacional. Resultado positivo da fixture não deve ser interpretado como autorização para migração de produção.

## Resultado

Supported no boundary de fixture.

No head `7a7413ab160c056ba1856023c32e706c96b7f8e1`, o workflow `EXP-LIMIAR-006` (run `33964927070`) concluiu com sucesso. O job `side-by-side-legacy-import` executou e passou:

- verificação do módulo;
- `gofmt`;
- `go vet`;
- testes com `CGO_ENABLED=0`;
- race detector.

O harness confirmou, para a fixture baseada no schema legado versionado no repositório, o contrato definido nos critérios: origem read-only, SHA-256 da origem preservado, payloads byte-identical, hashes reconciliáveis, nova identidade de Evidence independente da identidade legada, persistência atômica de Evidence + ledger, retry idempotente e `integrity_check` do destino.

Esse resultado suporta a hipótese apenas para o boundary exercitado. Ele não prova compatibilidade com um arquivo operacional real, não define o schema final do ledger, não transforma ids legados em identidade canônica e não autoriza migração ou alteração de produção.

## Próximo gate

Executar o mesmo reconciliador contra uma cópia real do banco legado e registrar apenas métricas/provas não sensíveis: contagem de linhas, contagem importada, mismatches, hashes agregados apropriados e `integrity_check`. Nenhum payload real ou segredo deve entrar no repositório.

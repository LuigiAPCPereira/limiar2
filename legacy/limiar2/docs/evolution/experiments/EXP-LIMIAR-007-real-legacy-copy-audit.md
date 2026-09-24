# EXP-LIMIAR-007 — Auditoria contra cópia real do legado

Authority: Non-authoritative
Status: In Progress

## Hipótese

Uma cópia real e descartável do banco legado pode passar o mesmo contrato de importação side-by-side demonstrado no EXP-LIMIAR-006 sem alteração do arquivo de origem e sem expor payloads ou outros dados sensíveis na evidência registrada.

## Origem

O EXP-LIMIAR-006 ficou `Supported` somente no boundary de fixture. Ele provou, usando o schema legado versionado no repositório, leitura read-only, preservação byte a byte do payload, nova identidade de Evidence, ledger de proveniência, persistência atômica, retry idempotente e `integrity_check`.

A limitação material restante é explícita: uma fixture não prova compatibilidade com um arquivo histórico operacional real.

Desde a criação deste experimento, os ADRs 019 e 020 foram aceitos e o primeiro slice real do novo storage SQLite foi materializado em `internal/storage/sqlite`. Portanto o gate de cópia real deve validar a importação contra esse storage atual, e não contra uma réplica experimental anterior do schema de Evidence.

Este experimento não altera o storage legado nem promove ADRs Proposed.

## Finding de continuidade do harness

A revisão de continuidade encontrou que o harness original ainda criava o destino com a constante experimental `evidenceSchema` do EXP-LIMIAR-006. Essa definição divergiu do schema que posteriormente se tornou autoritativo:

- a constante experimental usava `STRICT, WITHOUT ROWID`;
- criava o índice `evidence_received_idx`;
- a migration atual `001_evidence.sql` cria `evidence` como `STRICT`, sem `WITHOUT ROWID` e sem índice secundário inicial;
- os guards append-only atuais vêm da migration real.

Executar o EXP-LIMIAR-007 naquele estado provaria compatibilidade com um schema experimental stale, não com o novo storage materializado.

O harness foi então alinhado para criar o destino por `internal/storage/sqlite.Open`, fechá-lo e reabri-lo apenas para o reconciliador experimental. O único schema adicional criado pelo experimento é `legacy_import_ledger`, no mesmo arquivo temporário, preservando a transação atômica Evidence + ledger já exercitada pelo EXP-LIMIAR-006.

Nenhuma migration, schema ou API de produção foi alterada para acomodar o experimento.

## Boundary que permanece não provado

O alinhamento acima valida compatibilidade física com o schema e as migrations atuais, mas **não** prova que a capability pública `EvidenceAppender` seja suficiente para uma ferramenta futura de importação.

O reconciliador experimental continua usando SQL direto dentro de uma única transação porque precisa gravar Evidence e `legacy_import_ledger` atomicamente. Expor uma capability transacional nova, incorporar o ledger ao storage de produção ou escolher outra semântica de idempotência seria uma decisão separada e não é autorizada por este experimento.

Portanto um resultado `Supported` do EXP-LIMIAR-007 significará somente que a cópia histórica é fisicamente reconciliável side-by-side com o storage atual sob este harness; não autoriza transformar o reconciliador experimental em API/default de produção.

## Boundary de segurança

O gate é opt-in e só executa quando `LIMIAR_LEGACY_DB_COPY` aponta para uma cópia externa explicitamente preparada para auditoria.

Regras:

- nunca apontar a variável para o arquivo operacional original;
- a origem é aberta por SQLite com `mode=ro`;
- o SHA-256 do arquivo é comparado antes/depois para detectar alteração incidental;
- o destino é criado em diretório temporário do teste;
- o destino é descartado ao fim da execução;
- nenhum payload, caminho do arquivo, channel/message id ou hash por linha é emitido no log;
- a única saída positiva persistível é composta por contagens e resultado de integridade.

## Harness

Arquivo:

`experiments/evidence-envelope/real_legacy_audit_test.go`

O destino temporário é inicializado pelo storage SQLite atual (`internal/storage/sqlite.Open`) antes da criação do ledger experimental.

Execução deliberadamente manual contra uma cópia local:

```sh
cd experiments/evidence-envelope
LIMIAR_LEGACY_DB_COPY=/caminho/para/legacy-copy.db \
  go test -run '^TestRealLegacyCopyAudit$' -count=1 -v .
```

Sem a variável, o teste faz `Skip`. Isso é intencional: CI não possui uma cópia real e um resultado verde com o teste pulado **não** suporta a hipótese deste EXP.

## Critérios de suporte

A hipótese só pode mudar para `Supported` quando uma execução contra cópia real demonstrar simultaneamente:

1. a origem é arquivo regular e abre read-only;
2. `PRAGMA integrity_check` da origem retorna `ok`;
3. o shape necessário de `raw_messages` é legível pelo reconciliador;
4. o destino é criado com as migrations e baseline do novo storage SQLite atual;
5. a contagem importada coincide com a contagem histórica;
6. Evidence e ledger possuem a mesma contagem da origem;
7. retry importa zero novas Evidence;
8. `PRAGMA integrity_check` do destino retorna `ok`;
9. o SHA-256 do arquivo de origem é idêntico antes/depois;
10. nenhuma informação sensível entra em commit, fixture ou log persistido.

## Resultado atual

`In Progress`.

O harness para executar o gate foi materializado e agora usa o storage SQLite atual como destino, mas nenhuma cópia real está presente no repositório ou disponível neste boundary de execução. Portanto não existe Evidence válida para declarar compatibilidade operacional real.

A revalidação da fixture contra o storage atual já foi concluída na PR #196: o Travis `278808848` ficou verde nos quatro jobs registrados pela PR, incluindo Linux X64/race, `CGO_ENABLED=0`, cross-build SQLite e runtime nativo Linux ARM64. Isso sustenta que o reconciliador experimental continua funcionando contra a fixture e o schema/migrations atuais; o gate `TestRealLegacyCopyAudit` continua fazendo `Skip` sem `LIMIAR_LEGACY_DB_COPY`, portanto esse CI não suporta a hipótese principal deste EXP.

O formato de `received_at` usado pelo reconciliador (`2006-01-02 15:04:05`, UTC) foi confrontado com o writer legado atual: `SaveRawMessage` persiste `ReceivedAt.UTC().Format(model.DBTimeLayout)` e `model.DBTimeLayout` possui exatamente esse layout. Isso sustenta a premissa para linhas produzidas por esse writer, sem substituir a auditoria da cópia histórica real.

## Próximo gate

Executar o comando acima contra uma **cópia descartável** do banco histórico e registrar somente a linha de métricas não sensíveis produzida pelo teste, junto com ambiente/toolchain e resultado dos gates do módulo.

Se o gate falhar por incompatibilidade de schema ou dado histórico, registrar a divergência como Finding antes de adaptar o importador. Não modificar o legado para fazer o experimento passar.

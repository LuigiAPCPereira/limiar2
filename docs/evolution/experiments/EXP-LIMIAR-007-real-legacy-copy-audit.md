# EXP-LIMIAR-007 — Auditoria contra cópia real do legado

Authority: Non-authoritative
Status: In Progress

## Hipótese

Uma cópia real e descartável do banco legado pode passar o mesmo contrato de importação side-by-side demonstrado no EXP-LIMIAR-006 sem alteração do arquivo de origem e sem expor payloads ou outros dados sensíveis na evidência registrada.

## Origem

O EXP-LIMIAR-006 ficou `Supported` somente no boundary de fixture. Ele provou, usando o schema legado versionado no repositório, leitura read-only, preservação byte a byte do payload, nova identidade de Evidence, ledger de proveniência, persistência atômica, retry idempotente e `integrity_check`.

A limitação material restante é explícita: uma fixture não prova compatibilidade com um arquivo histórico operacional real.

O ADR 019 continua `Proposed` e lista a importação side-by-side em cópia real do banco legado como gate antes de produção. Este experimento não promove o ADR nem altera o storage de produção.

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
4. a contagem importada coincide com a contagem histórica;
5. Evidence e ledger possuem a mesma contagem da origem;
6. retry importa zero novas Evidence;
7. `PRAGMA integrity_check` do destino retorna `ok`;
8. o SHA-256 do arquivo de origem é idêntico antes/depois;
9. nenhuma informação sensível entra em commit, fixture ou log persistido.

## Resultado atual

`In Progress`.

O harness para executar o gate foi materializado, mas nenhuma cópia real está presente no repositório ou disponível neste boundary de execução. Portanto não existe Evidence válida para declarar compatibilidade operacional real.

CI pode validar apenas que o harness compila e que os experimentos anteriores continuam verdes; um `Skip` por ausência de `LIMIAR_LEGACY_DB_COPY` não fecha este experimento.

## Próximo gate

Executar o comando acima contra uma **cópia descartável** do banco histórico e registrar somente a linha de métricas não sensíveis produzida pelo teste, junto com ambiente/toolchain e resultado dos gates do módulo.

Se o gate falhar por incompatibilidade de schema ou dado histórico, registrar a divergência como Finding antes de adaptar o importador. Não modificar o legado para fazer o experimento passar.

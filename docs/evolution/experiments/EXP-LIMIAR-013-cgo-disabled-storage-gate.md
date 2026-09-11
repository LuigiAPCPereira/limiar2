# EXP-LIMIAR-013 — Compatibilidade do storage com CGO desabilitado

Authority: Experiment
Status: In Progress

## Pergunta

O estado atual do repositório continua compilável e testável em Linux X64 com `CGO_ENABLED=0`, incluindo o novo storage SQLite baseado em `github.com/ncruces/go-sqlite3` e os boundaries legados que ainda coexistem durante a rebaseline?

## Por que este experimento existe

A baseline mantém `CGO_ENABLED=0 go test` como gate explícito antes de colocar o novo ingress/storage em produção. Os gates recentes exercitam Linux X64 com race detector, o que implica CGO habilitado e portanto não responde essa pergunta.

O ADR 019 escolheu `github.com/ncruces/go-sqlite3` para o novo storage, mas não autoriza inferir compatibilidade do repositório inteiro com CGO desabilitado apenas pela natureza do driver. O legado Tursogo, integrações auxiliares ou dependências transitivas também podem limitar esse modo.

## Hipótese

Em Linux X64 com Go 1.26.2 e `CGO_ENABLED=0`, o repositório deve:

1. verificar os módulos sem alteração;
2. passar `go vet ./...`;
3. preservar o gate de dependências de storage;
4. compilar com `go build ./...`;
5. passar `go test -count=1 ./...`.

O race detector não faz parte deste job porque o gate experimental desabilita CGO deliberadamente. O job normal com `CGO_ENABLED=1` continua exercitando `go test -race` e os demais gates completos.

## Harness

A branch experimental adiciona temporariamente ao Travis uma matriz de dois jobs:

- `Linux X64 / race`: preserva o pipeline atual com CGO habilitado;
- `Linux X64 / CGO disabled experiment`: executa vet, gate de dependências, build e testes com `CGO_ENABLED=0`.

Nenhum default de produção, schema, migration, pool, dependência ou comportamento de runtime é alterado por este experimento.

## Critérios para `Supported`

O experimento só pode mudar para `Supported` quando o job `Linux X64 / CGO disabled experiment` executar em CI real no HEAD correspondente e concluir com sucesso todas as etapas listadas na hipótese.

Falha deve ser tratada como Evidence. Não corrigir dependência, build tag ou boundary estrutural sem primeiro identificar a causa e verificar se a correção já é autorizada por ADR vigente.

## Limites

Mesmo se suportado, o resultado será limitado ao ambiente efetivamente exercitado: Linux X64, Go 1.26.2 e conjunto de dependências do HEAD testado. Não prova Android/Termux, ARM64, Windows, macOS ou PRoot.

## Fora de escopo

- declarar suporte permanente a uma plataforma;
- substituir Tursogo legado;
- mudar o driver SQLite;
- alterar o contrato dos ADRs 019/020;
- implementar `SourceSyncState` ou `BackfillProgress`;
- generalizar resultado para plataformas não executadas.

# EXP-LIMIAR-013 — Compatibilidade do storage com CGO desabilitado

Authority: Experiment
Status: Supported

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

A branch experimental adiciona ao Travis uma matriz de dois jobs:

- `Linux X64 / race`: preserva o pipeline atual com CGO habilitado;
- `Linux X64 / CGO disabled experiment`: executa vet, gate de dependências, build e testes com `CGO_ENABLED=0`.

Nenhum default de produção, schema, migration, pool, dependência ou comportamento de runtime é alterado por este experimento.

## Evidence

No HEAD `0e17613d287dd2cb6feebeac71128f733f38a2c3`, o build Travis `278801913` executou em Linux Noble com Go 1.26.2 e concluiu com sucesso os dois jobs da matriz.

O job `Linux X64 / CGO disabled experiment` executou com `LIMIAR_CI_MODE=cgo-disabled CGO_ENABLED=0` e passou `go mod verify`, `go vet ./...`, `scripts/ci/check-storage-dependencies.sh`, `go build -v ./...` e `go test -v -count=1 ./...`.

O job `Linux X64 / race` também passou no mesmo build, preservando o gate normal com CGO habilitado e race detector. Portanto, o experimento suporta a hipótese somente no ambiente e no conjunto de dependências exercitados por esse HEAD.

## Resultado

A hipótese é suportada para Linux X64/Noble, Go 1.26.2 e o conjunto de dependências do HEAD validado: o repositório, incluindo o novo storage SQLite e os boundaries legados ainda presentes, compila e passa os testes com `CGO_ENABLED=0`.

Esse resultado fecha o gate experimental de compatibilidade CGO-disabled previsto na baseline para esse ambiente. Não transforma `CGO_ENABLED=0` em default de produção nem declara portabilidade para plataformas não exercitadas.

## Limites

O resultado é limitado ao ambiente efetivamente exercitado: Linux X64, Go 1.26.2 e conjunto de dependências do HEAD testado. Não prova Android/Termux, ARM64, Windows, macOS ou PRoot.

Mudanças futuras em dependências, build tags, toolchain ou boundaries de storage podem invalidar a Evidence e devem reexecutar o gate quando materialmente relevantes.

## Fora de escopo

- declarar suporte permanente a uma plataforma;
- substituir Tursogo legado;
- mudar o driver SQLite;
- alterar o contrato dos ADRs 019/020;
- implementar `SourceSyncState` ou `BackfillProgress`;
- generalizar resultado para plataformas não executadas.

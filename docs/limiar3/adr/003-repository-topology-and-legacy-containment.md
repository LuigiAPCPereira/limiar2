# L3 ADR 003 — Repository topology and legacy containment

Authority: Decision Record — Limiar 3.0
Status: Accepted
Accepted-by: Mantenedor do Limiar
Accepted-at: 2026-09-23
Acceptance-reference: `70dcd4982b90a26fa6eb7f722a679537f54bb1e0`

## Contexto

O repositório ainda expõe no root a implementação sobrevivente do Limiar anterior: módulo `github.com/limiar/collector`, `cmd/limiar`, `internal/*`, storage/Telegram/processor/dashboard, workflows e documentação histórica/rebaseline. L3-001/A/B/C e L3 ADR 001/002 decidiram uma reconstrução que não deve herdar essa estrutura por acidente.

Manter L2 no mesmo namespace de código faria caminhos como `internal/telegram` parecerem autoridade atual e incentivaria evolução incremental do legado, contrariando a reconstrução.

## Decision

### Root = Limiar 3 atual

O root do repositório passa a representar exclusivamente a implementação corrente do Limiar 3 e suas fontes constitucionais/canônicas.

Permanecem no root:

- `AGENTS.md` e aliases/instruções de agentes que apontem para ele;
- `ENGINEERING_DNA.md`;
- `docs/DOCUMENTATION_AND_CONTINUITY.md`;
- `docs/limiar3/**`;
- tooling/agentes genéricos não pertencentes à implementação L2;
- novo `README.md`, `go.mod`, `.gitignore` e CI do Limiar 3.

### Legacy containment

A implementação e artefatos específicos do Limiar anterior são movidos para `legacy/limiar2/`, preservando os mesmos blobs sempre que possível.

Inclui, quando presentes:

- `cmd/`, `internal/`, `scripts/`, `tools/`, `experiments/`;
- `go.mod`, `go.sum`, Makefile, env/config/release/CI legados;
- documentação histórica/rebaseline anterior a `docs/limiar3`, inclusive ADR registry histórico, EvolutionDocs, specs, benchmarks, prompts e relatórios;
- workflows experimentais/legados;
- specs/tool configs específicos de `limiar-collector`.

O código independente do Limiar 1 que não exista no repositório não será inventado. `legacy/limiar2` representa a implementação sobrevivente e o material histórico disponível.

### Módulos Go

`legacy/limiar2` preserva o módulo legado `github.com/limiar/collector` e seu `go.sum`, para manter imports e capacidade de reprodução histórica.

O root Limiar 3 inicia um módulo novo e mínimo com path correspondente ao repositório atual: `github.com/LuigiAPCPereira/limiar2`, Go 1.27.1. Dependências de produto entram somente nas fatias que as utilizam; gotd v0.162.0 permanece a baseline aceita para L3-002 e será adicionado pelo primeiro código que o consumir.

### Regra de dependência

Dependência de código **L3 -> `legacy/limiar2` é proibida**.

Legacy pode ser consultado como Evidence, baseline de regressão ou comportamento a reproduzir conscientemente, mas packages atuais não importam packages legacy nem usam legacy como source of truth.

### Git/history

A migração usa movimentação por árvore Git preservando blob SHAs quando o conteúdo não precisa mudar. O fato de um arquivo estar sob `legacy/` não apaga sua história.

### CI

Workflows antigos da implementação anterior deixam de executar no root e são preservados sob legacy. O root recebe CI mínimo/reprodutível do Limiar 3 com toolchain pinado. Jobs legacy, se voltarem a ser necessários, devem ser explícitos e separados do gate do produto atual.

## Gates

Antes de considerar L3-BASE-001 concluída:

1. root não contém packages de implementação L2;
2. `legacy/limiar2/go.mod` preserva o módulo legado;
3. root `go.mod` usa Go 1.27.1 e módulo L3 corrente;
4. documentação L3 não possui links críticos quebrados para os artefatos históricos movidos;
5. AGENTS/protocolo/handoff apontam para as authorities corretas;
6. CI root não executa workflows experimentais L2;
7. barreira automatizada impede imports para `/legacy/limiar2` no código root;
8. nenhum código funcional de L3-002 é introduzido nesta fatia;
9. diff deve ser predominantemente rename/move + scaffolding/documentação necessária.

## Consequências

O root ganha significado inequívoco: código em `internal/telegram` futuro será Limiar 3, não uma facade L2.

A mudança aumenta a quantidade de renames num PR específico, mas evita misturá-los com implementação funcional e mantém o legado reproduzível/consultável.
# Limiar 3

Este root representa a **implementação corrente do Limiar 3.0**.

O Limiar 3 é uma reconstrução do agregador de promoções: aquisição de fontes Telegram, Evidence, processamento determinístico, modelo comercial, Query Service, MCP, API e frontend serão reconstruídos por fatias verificáveis.

## Estado atual

A fundação documental e arquitetural foi concluída em L3-001/A/B/C. Os Decisions exclusivos do Limiar 3 vivem em [docs/limiar3/adr/](docs/limiar3/adr/).

A próxima capacidade de produto é **L3-002 — Telegram Auth/Session/Runtime**, mas ela não é implementada nesta fatia de repository rebaseline.

Baseline aceita para L3-002:

- Go 1.27.1;
- github.com/gotd/td v0.162.0 quando a primeira fatia que o utiliza for implementada;
- Linux / single-host / single-process por TelegramAuthorizationIdentity;
- hardened session.Storage;
- um main telegram.Client por authorization identity.

## Legado

A implementação anterior foi preservada sob [legacy/limiar2/](legacy/limiar2/).

Ela é **Evidence e referência histórica**, não package source do Limiar 3. Código L3 não pode importar packages de legacy.

O código independente do Limiar 1 não está presente como uma implementação separada neste repositório; não foi inventado durante a reorganização.

## Fontes canônicas

Antes de desenvolvimento:

1. [AGENTS.md](AGENTS.md)
2. [docs/DOCUMENTATION_AND_CONTINUITY.md](docs/DOCUMENTATION_AND_CONTINUITY.md)
3. [ENGINEERING_DNA.md](ENGINEERING_DNA.md)
4. [docs/limiar3/START_HERE.md](docs/limiar3/START_HERE.md)
5. [docs/limiar3/TASKLIST.md](docs/limiar3/TASKLIST.md)
6. [docs/limiar3/PROJECT_STATE.md](docs/limiar3/PROJECT_STATE.md)

Não use o legado como autoridade arquitetural por conveniência.

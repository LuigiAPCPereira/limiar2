# Baseline — Limiar 3.0

Authority: Derived Baseline

Este documento resume o estado corrente do Limiar 3. Ele **não cria Decisions**; em conflito, prevalecem `AGENTS.md` e ADRs Accepted em [limiar3/adr/](limiar3/adr/).

## Produto

O Limiar 3 é o próprio agregador de promoções. A direção de produto é: aquisição de fontes Telegram -> Evidence -> processamento/agrupamento -> dados estruturados -> Query Service -> MCP/API -> frontend.

Fonte detalhada: [limiar3/PRODUCT_AND_SCOPE.md](limiar3/PRODUCT_AND_SCOPE.md).

## Topologia do repositório

Por L3 ADR 003:

- o root representa exclusivamente a geração atual Limiar 3;
- a implementação anterior está preservada em [../legacy/limiar2/](../legacy/limiar2/);
- código L3 não pode importar packages legacy;
- material legacy permanece Evidence/histórico, não arquitetura-alvo.

## Fundação Telegram aceita

L3 ADR 001 e L3 ADR 002 aceitam:

- `TelegramAuthorizationIdentity` como unidade de ownership;
- Linux / single-host / single-process inicial;
- hardened local file implementando diretamente `gotd/session.Storage`;
- exatamente um main `telegram.Client` por authorization identity;
- bootstrap administrativo explícito; QR-first com fallback controlado;
- steady-state fail-closed e sem auto-login;
- semantic Ready + self binding;
- primeira capability read-only: `ResolvePeer + History`;
- Go 1.27.1;
- gotd/td v0.162.0 como pin inicial da fatia L3-002.

## Estado de implementação

A repository rebaseline L3-BASE-001 é estrutural. Nenhum TelegramRuntime, bootstrap real, login Telegram ou credential de produção é criado por ela.

A próxima fatia funcional é L3-002 após o Implementation Gate e autorização explícita.

## Storage/Evidence futuro

As conclusões/regras históricas de Evidence/recovery da rebaseline anterior continuam disponíveis em `legacy/limiar2/docs/` e podem ser herdadas somente quando a frente L3 correspondente as confirmar explicitamente.

## Fontes de continuidade

- [limiar3/START_HERE.md](limiar3/START_HERE.md)
- [limiar3/TASKLIST.md](limiar3/TASKLIST.md)
- [limiar3/PROJECT_STATE.md](limiar3/PROJECT_STATE.md)
- [limiar3/adr/README.md](limiar3/adr/README.md)

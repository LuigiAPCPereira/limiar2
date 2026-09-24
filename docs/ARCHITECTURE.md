# Arquitetura — Limiar 3.0

Authority: Derived Architecture

Este documento é uma visão derivada. Decisions pertencem aos ADRs Accepted em [limiar3/adr/](limiar3/adr/).

## Princípio

> Desacoplar não significa distribuir.

O Limiar separa ownership, authority, lifecycle, failure domain e contracts antes de decidir processos/serviços.

## Estado atual

Apenas a fundação de repositório e as Decisions Telegram/Auth/Session foram aceitas. A implementação funcional ainda será construída por slices.

```text
Telegram
   ↓
TelegramRuntime / gotd adapter
   ↓
TelegramQuery
   ↓
[futuro] Source Admission + Evidence
   ↓
[futuro] processamento / agrupamento
   ↓
[futuro] Query Service
   ↓
MCP + API + frontend
```

## Boundaries aceitos da primeira fatia

- gotd fica contido no boundary Telegram;
- session credential é authority separada de peer/update/Evidence;
- root L3 não depende de `legacy/limiar2`;
- reconnect, pools, DC migration, MTProto e low-level retry pertencem ao gotd;
- o Limiar acrescenta storage hardened, ownership/readiness, capability e semantic error mapping.

Detalhes: [limiar3/ARCHITECTURE_PROPOSAL.md](limiar3/ARCHITECTURE_PROPOSAL.md), [limiar3/DETACHABLE_BOUNDARIES.md](limiar3/DETACHABLE_BOUNDARIES.md) e ADRs Accepted.

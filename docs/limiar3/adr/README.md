# ADRs — Limiar 3.0

Authority: Decision Registry — Limiar 3.0

Este diretório contém as Decisions duráveis exclusivas da reconstrução Limiar 3.0.

## Numeração

O Limiar 3 é uma reconstrução com baseline arquitetural próprio. Por direção explícita do mantenedor, a numeração reinicia em `001` neste namespace e **não continua** a sequência histórica `legacy/limiar2/docs/adr/001–024` da rebaseline/legado.

Os ADRs históricos continuam disponíveis como Evidence, decisões herdadas quando explicitamente aplicáveis ou propostas históricas conforme `legacy/limiar2/docs/adr/README.md`; eles não são renumerados nem promovidos silenciosamente.

## Governança

Aplicam-se integralmente `AGENTS.md` e `docs/DOCUMENTATION_AND_CONTINUITY.md`:

- `Proposed` não autoriza produção;
- somente o mantenedor promove uma Decision para `Accepted`;
- Acceptance deve registrar `Accepted-by`, `Accepted-at` e `Acceptance-reference` durável;
- uma Decision Accepted permanece autoridade do Limiar 3 até ser `Superseded` por outra Decision aceita.

## Registry

| ADR | Título | Status | Acceptance reference |
| --- | --- | --- | --- |
| [001](001-hardened-telegram-session-storage.md) | Credential/session storage hardened em arquivo local | **Accepted** | `baa626b59ec21d36f0dbde9a8a1fb3bbb5a4a2a6` |
| [002](002-telegram-authorization-runtime.md) | Telegram authorization identity, runtime ownership e lifecycle | **Accepted** | `baa626b59ec21d36f0dbde9a8a1fb3bbb5a4a2a6` |\n| [003](003-repository-topology-and-legacy-containment.md) | Repository topology and legacy containment | **Accepted** | `70dcd4982b90a26fa6eb7f722a679537f54bb1e0` |

## Relação com ADRs históricos

- `legacy/limiar2/docs/adr/023-hardened-mtproto-session-file-boundary.md` permanece `Proposed` no registry histórico. L3 ADR 001 reaproveita Evidence e refinamentos dele, mas sua aceitação **não promove** o ADR histórico.
- ADR histórico 004 continua sob o registry transitório da rebaseline; ele não é a autoridade do mecanismo concreto de sessão do Limiar 3.
- ADRs 016–020 Accepted continuam relevantes para a futura fatia de Evidence/acquisition quando explicitamente herdados pelos documentos L3.

## Baseline de dependências

`DEPENDENCY_TOOLCHAIN_POLICY.md` governa versões atuais. Para o início de L3-002, o mantenedor aceitou Go 1.27.1 e `github.com/gotd/td v0.162.0` como baseline inicial. Pins posteriores podem avançar sob `Current Stable First` quando não alterarem os contratos destas Decisions; mudança arquitetural exige ADR proporcional.
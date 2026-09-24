# ADRs — Autoridade e transição da Rebaseline 2026

Authority: Decision Registry

Este diretório contém os Architecture Decision Records do Limiar.

Um ADR registra uma decisão durável. Pesquisa, experimento e proposta pertencem a
`docs/evolution/` até que exista uma decisão explícita.

---

## Estados permitidos

- `Proposed`
- `Accepted`
- `Rejected`
- `Superseded`

Transições normais:

`Proposed -> Accepted`

`Proposed -> Rejected`

`Accepted -> Superseded`

Um ADR `Proposed` não autoriza mudança arquitetural de produção.

---

## Aceitação

Somente o mantenedor pode autorizar `Proposed -> Accepted`.

Quando um ADR for aceito, registre no documento:

```text
Status: Accepted
Accepted-by: <mantenedor>
Accepted-at: <data ISO-8601>
Acceptance-reference: <PR, issue, commit ou referência durável>
```

A referência de aceitação existe para que um agente futuro consiga verificar que o
status não foi promovido unilateralmente por outro agente.

---

## Regra de supersessão

Um ADR Accepted continua sendo autoridade até que outro ADR Accepted o substitua.

Quando isso ocorrer, o ADR anterior deve ser marcado como `Superseded` e apontar para a
nova decisão.

Não apague ADRs antigos apenas porque foram substituídos. Eles preservam a história e o
porquê das mudanças.

---

## Registry de transição — Rebaseline 2026

Os ADRs 001–015 foram produzidos antes da rebaseline de autoridade iniciada em 2026.
Durante a arqueologia, foi constatado que documentos históricos, propostas e código
chegaram a divergir; portanto, esses ADRs não devem ser interpretados isoladamente como
baseline atual enquanto sua disposição não for concluída.

A tabela abaixo é um **registry transitório**, não um novo conjunto de decisões sobre a
arquitetura futura. Ela informa como tratar cada registro legado durante a rebaseline.

| ADR | Disposição transitória | Interpretação durante a rebaseline |
| --- | --- | --- |
| 001 — Tursogo / banco único | `SUPERSEDE` | A escolha concreta de engine será redecidida; preservar evidência histórica. |
| 002 — gotd/td direto | `RETAIN-CORE / REWRITE` | Manter o princípio de gotd como integração Telegram enquanto o contrato de sync é revalidado. |
| 003 — fan-in DBWriter | `SUPERSEDE` | O DBWriter global não é autoridade para o novo ingress. |
| 004 — sessão MTProto | `RETAIN-PRINCIPLE / REWRITE` | Manter necessidade de sessão durável; redecidir mecanismo de armazenamento. |
| 005 — serialização raw JSON | `SUPERSEDE` | Preservação de Evidence permanece; o contrato físico será redesenhado/versionado. |
| 006 — retomada por LastMessageID | `RETIRE` | Não usar como autoridade de sincronização live. |
| 007 — retry/FloodWait | `RETAIN-PRINCIPLES / REWRITE` | Manter tratamento explícito de erros transitórios; revalidar política concreta. |
| 008 — validação de config | `RETAIN-PRINCIPLE / SIMPLIFY` | Config deve ser validada; forma concreta pode mudar. |
| 009 — processo unificado | `RETAIN-CORE / CLARIFY` | Single-process permanece baseline candidata sem misturar ownership e responsabilidades. |
| 010 — migrations atômicas | `RETAIN-CORE / REWRITE` | Migrations continuam necessárias; autoridade e migração do legado serão reorganizadas. |
| 011 — image resolver | `DEFER / REVALIDATE` | Aguardar auditoria específica de mídia. |
| 012 — MediaClient/session access | `RETIRE` | Implementação histórica não deve orientar o novo boundary de mídia. |
| 013 — package `internal/model` | `RETAIN-PRINCIPLE / SUPERSEDE-SHAPE` | Tipos de domínio continuam necessários; pacote global atual não é baseline. |
| 014 — extração determinística/CRE | `SUPERSEDE / SPLIT` | Algoritmos e testes são valiosos; o ADR permaneceu Proposed e não é autoridade. |
| 015 — deployment topology | `HISTORICAL` | Preservar como proposta histórica; não tratar como topologia atual. |

Enquanto este registry existir, um agente não pode citar um ADR legado isoladamente para
bloquear ou autorizar a rebaseline sem considerar sua disposição transitória.

---

## Relação com EvolutionDocs

Fluxo normal:

```text
Finding / Experiment
        ↓
Proposal
        ↓
ADR Proposed
        ↓
aceitação explícita do mantenedor
        ↓
ADR Accepted
        ↓
implementação
```

Consulte `docs/evolution/README.md` e `AGENTS.md`.

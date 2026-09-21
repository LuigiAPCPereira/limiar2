# ROADMAP — Rebaseline 2026 (marcos verificáveis)

**Estado:** planejamento de reconciliação documental da rebaseline, não autorização para implementar propostas nem previsão de datas. Base observada: `main@796b7769449b72320b27c2557c2ccb8c8eb183e1`; adoção na branch `docs/adopt-agent-protocol-v2-20260920`, PR #214 draft. As fontes de autoridade continuam `AGENTS.md` e ADRs `Accepted`; este roadmap é derivado. As fases históricas de `docs/CONTEXT.md` não são cronograma do novo storage.

| Marco | Resultado verificável | Dependência / autoridade | Tarefas | Estado / evidência |
| --- | --- | --- | --- | --- |
| M0 — adoção documental | Protocolo e constituição legíveis na mesma ref; nove funções, inventário integral, checkpoint, links e relatório verificados | Protocolo v2.0 e autorização de aplicar documentação | ADOPT-001–ADOPT-004 | Parcial: PR #214 draft, inventário de adoção e matriz existentes; hash integral do protocolo e cobertura de produto ainda pendentes |
| M1 — fundação de Evidence | Novo SQLite side-by-side com migrations e append de Evidence, sem alteração in-place do legado; verificar gates de integridade e plataforma no ambiente exato | ADR 019 e ADR 020 `Accepted`, ADRs 016/017 | PROD-001, PROD-002 | Implementação inicial descrita em `docs/BASELINE.md` §4 e commit/PR #211; não inferir prontidão de produção nem validação da revisão atual |
| M2 — admissão live/recovery | Boundary pré-`updates.Manager`, Evidence-before-state, fail-stop e contratos de recovery exercitados no adapter real | ADRs 016–018 `Accepted`; identidade de acquisition subscription ainda `Proposed` no ADR 024 | PROD-003, PROD-004 | Contrato aceito no ADR 018; wiring produtivo bloqueado por identidade/configuração não decidida e gates reais pendentes |
| M3 — progresso independente | Capabilities físicas de `SourceSyncState` e `BackfillProgress` validadas e integradas sem misturar authorities | ADRs 016/017/019; ADR 021/022 `Proposed` — requerem aceitação para mudanças estruturais correspondentes | PROD-005, PROD-006 | Experimentos e propostas não equivalem a implementação autorizada |
| M4 — migração e prontidão operacional | Importação side-by-side com cópia histórica real, rollback, crash/restart e matriz de plataformas verificadas; efeito de deploy documentado antes de qualquer integração | Gates do ADR 019, `docs/BASELINE.md` §7; dados históricos descartáveis e autorização específica para ações com efeitos reais | PROD-007, PROD-008 | Gates abertos; resultados de harness e cross-build não comprovam produto completo |

## Sequência, decisões e limites

- Caminho crítico documental: ADOPT-002 (integridade da fonte) → ADOPT-003 (escopo, requisitos, inventário de produto, reconciliação) → ADOPT-004 (Gate). Não confundir com caminho crítico de engenharia já aprovado.
- Dependência para M2: registrar decisão formal sobre identidade antes de alterar o caminho produtivo; o ADR 024 continua `Proposed`. A aceitação do ADR 018 não a substitui.
- M3 não pode ser declarado liberado pelo estado de experimentos dos ADRs 021/022; M4 preserva dados legados e requer evidência de ambiente real.
- As propostas de IA, feed, descoberta de canais e outras fases futuras em `docs/CONTEXT.md` e `docs/PRODUCT_BRIEF.md` **não** viram tarefas ativas automaticamente.
- **Datas:** não definidas. **Status de CI/merge/deploy do produto nesta adoção:** não verificado / não executado. O roadmap não substitui [`TASKLIST.md`](TASKLIST.md), que guarda IDs, estados e aceites.

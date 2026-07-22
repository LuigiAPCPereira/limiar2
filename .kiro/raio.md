## 2025-07-22 - Reduzir interface boxing em chamadas de Logger no hot path
**Aprendizado:** Chamadas de interface disparam alocações dinâmicas e adicionam overhead em hot paths (fan-out de updates do MTProto) mesmo quando o log está desabilitado.
**Ação:** Usar uma fast type assertion para pular chamadas em interface (`logger.Logger`) quando a instância concreta for `NopLogger`.

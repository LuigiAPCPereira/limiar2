## 2024-05-18 - Index SQL para paginação no Dashboard
**Aprendizado:** As queries do dashboard usam `ORDER BY received_at DESC`. O SQLite realiza um *TEMP B-TREE* para a ordenação devido a ausência de um índice em `received_at`, isso tem um alto custo para grandes volumes de dados.
**Ação:** Criar os índices `idx_raw_messages_received_at` para listagens gerais, e `idx_raw_messages_channel_received_at` para listagens otimizadas por canal otimizando a latência do dashboard.

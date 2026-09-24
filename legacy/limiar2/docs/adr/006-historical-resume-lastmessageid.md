# ADR 006 — Backfill Inicial + Retomada a partir do LastMessageID

## Status

Aceito

## Contexto

Quando o collector começa a monitorar um canal, existem duas situações: o
canal nunca foi coletado (sem cursor), ou já foi coletado antes e
queremos evitar o reprocessamento do que já temos. Precisamos de um cursor determinístico
de onde retomar, e em um canal totalmente novo queremos algum histórico imediato para que o
pipeline subsequente (downstream) tenha dados para trabalhar, em vez de esperar pela próxima mensagem
ao vivo.

A tabela `channels` carrega `last_message_id` (padrão 0) e
`last_collected_at` como o cursor por canal.

## Decisão

No `run`, o `Collector` distingue a primeira execução de uma retomada (resume) usando
`Channel.LastMessageID`:

- **Primeira execução (`LastMessageID == 0`)** — `backfill` chama
  `client.FetchHistory(ctx, channelID, accessHash, historyBatchSize)` com
  `historyBatchSize = 20`, enfileira cada payload como um `WriteJob`, e avança
  o cursor do canal para a ID de mensagem mais nova recuperada antes do início da captura
  ao vivo.
- **Retomada (`LastMessageID > 0`)** — `backfill` pula o canal; a captura ao vivo
  continua do cursor existente, e o DBWriter avança o cursor conforme novas
  mensagens são persistidas (`UpdateChannelLastMessage`). A constraint
  `UNIQUE(channel_id, message_id)` juntamente com `ON CONFLICT DO NOTHING`
  protege contra duplicatas.

A falha no backfill é logada mas não é fatal — a captura ao vivo inicia mesmo assim.

## Consequências

- Canais novos rendem dados imediatos (20 mensagens recentes) sem a necessidade de esperar
  pelo tráfego ao vivo.
- Reinicializações não reprocessam o histórico; o cursor + a constraint única tornam
  a persistência idempotente.
- O lote (batch) de 20 mensagens é uma constante fixa da Fase 1; um histórico mais profundo precisaria
  de paginação (trabalho futuro). *(Nota: implementado paginação posteriormente na refatoração)*
- Canais inativos (`Active == false`) são pulados durante o backfill.

## Alternativas consideradas

- **Sem backfill** — canais novos ficariam vazios até a próxima mensagem ao vivo;
  ruim para testar o pipeline downstream. Rejeitado.
- **Backfill histórico completo** — caro, suscetível a rate-limits (limite de taxa) e desnecessário para
  descobrir o formato (shape) dos dados. Rejeitado para a Fase 1.
- **Cursor baseado em Timestamp** — o id da mensagem é o cursor natural e monotônico
  do Telegram e evita ambiguidades de defasagem de relógio (clock-skew).

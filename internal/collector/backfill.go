package collector

import (
	"context"
	"time"

	"github.com/limiar/collector/internal/model"
	"github.com/limiar/collector/internal/telegram"
)

// backfill busca o histórico para cada canal ativo, paginando até o limite
// configurado (c.historyMax) ou até que o cursor armazenado (LastMessageID) seja
// atingido. SaveRawMessage é idempotente (ON CONFLICT DO NOTHING na constraint
// única (channel_id, message_id)), portanto buscar novamente mensagens que
// já foram persistidas é seguro — elas são silenciosamente descartadas pelo DB.
func (c *Collector) backfill(ctx context.Context, channels []*model.Channel) error {
	for _, ch := range channels {
		if !ch.Active {
			continue
		}
		c.log.Info("📜 Buscando histórico",
			"canal", ch.Username,
			"desde_msg_id", ch.LastMessageID,
			"teto", c.historyMax,
			"desde_data", time.Now().AddDate(0, 0, -c.historyMaxDays).Format("2006-01-02"))

		fetched, maxID, err := c.backfillChannel(ctx, ch)
		if err != nil {
			c.log.Error("📜 Falha ao buscar histórico do canal",
				"canal", ch.Username, "erro", err)
			continue // não aborta todos os canais — tenta o próximo
		}

		if maxID > 0 {
			if err := c.repo.UpdateChannelLastMessage(ctx, ch.ID, maxID, time.Now().UTC()); err != nil {
				c.log.Error("📜 Falha ao avançar cursor do canal",
					"canal", ch.Username, "erro", err)
				continue
			}
		}

		c.log.Info("📜 Histórico carregado",
			"canal", ch.Username,
			"total", fetched,
			"max_msg_id", maxID)
	}
	return nil
}

// backfillChannel pagina através do histórico de um canal começando pelas mensagens
// mais recentes e caminhando para trás. Duas condições de parada se aplicam:
//   - limite numérico (c.historyMax): rede de segurança contra canais hiperativos
//   - limite temporal (agora - c.historyMaxDays): regra de negócio que diz que promoções
//     antigas perdem valor e não devem ser coletadas
//
// O messages.getHistory do Telegram suporta MinID (id > MinID, no servidor) e
// OffsetID (id < OffsetID). Nós usamos MinID=cursor na primeira requisição para descartar
// tudo igual-ou-abaixo do cursor armazenado no lado do servidor, depois usamos OffsetID nas
// requisições subsequentes para paginar retroativamente através da janela retornada. O loop
// termina em caso de: página vazia, qualquer das condições de parada, ou cancelamento de contexto.
// SaveRawMessage é idempotente (ON CONFLICT DO NOTHING no (channel_id, message_id)
// único), então buscar uma mensagem já persistida é um no-op silencioso na camada do DB.
func (c *Collector) backfillChannel(ctx context.Context, ch *model.Channel) (fetched, maxID int64, err error) {
	cursor := ch.LastMessageID
	ceiling := int64(c.historyMax)
	cutoff := time.Now().AddDate(0, 0, -c.historyMaxDays)
	var totalFetched int64
	// scannedMaxID é o maior MessageID entre mensagens varridas que NÃO são
	// duplicatas abaixo do cursor (ou seja, mensagens acima do cursor ou
	// descartadas pelo cutoff temporal). É o valor seguro para avançar o
	// cursor:
	//  - mensagens acima do cursor: varridas e enfileiradas → cursor avança
	//  - mensagens filtradas pelo cutoff: vistas mas descartadas → cursor avança
	//    (não serão úteis na próxima execução, pois continuarão antigas)
	//  - mensagens abaixo do cursor (duplicatas reais): NÃO fazem scannedMaxID
	//    crescer, então o cursor não avança em cenários de reprocessamento
	//    onde todo o lote é duplicata.
	// scannedMaxID == 0 significa "nenhuma mensagem acima do cursor foi vista"
	// (tudo duplicata), e o orquestrador preserva o cursor armazenado.
	var scannedMaxID int64
	offsetID := int64(0)
	firstPage := true

	for {
		if ctx.Err() != nil {
			return totalFetched, scannedMaxID, ctx.Err()
		}

		remaining := ceiling - totalFetched
		if remaining <= 0 {
			break
		}
		pageSize := historyPageSize
		if remaining < int64(pageSize) {
			pageSize = int(remaining)
		}

		var msgs []telegram.HistoryMessage
		var fetchErr error
		if firstPage && cursor > 0 {
			// MinID no servidor descarta tudo com id <= cursor na primeira
			// requisição, então recebemos apenas mensagens mais novas que o cursor.
			msgs, fetchErr = c.client.FetchHistory(ctx, ch.ID, cursor, pageSize)
		} else {
			msgs, fetchErr = c.client.FetchHistoryWithOffset(ctx, ch.ID, offsetID, pageSize)
		}
		if fetchErr != nil {
			return totalFetched, scannedMaxID, fetchErr
		}
		firstPage = false
		if len(msgs) == 0 {
			break
		}

		pageMinID := msgs[0].MessageID
		pageMinDate := msgs[0].Date
		for _, m := range msgs {
			if m.MessageID < pageMinID {
				pageMinID = m.MessageID
			}
			if m.Date.Before(pageMinDate) {
				pageMinDate = m.Date
			}
		}

		for _, m := range msgs {
			// Pula mensagens iguais ou abaixo do cursor armazenado (já persistidas
			// em uma execução anterior). Estas são duplicatas REAIS — não entram
			// no cálculo de scannedMaxID, evitando regressão do cursor em
			// cenários de reprocessamento.
			if cursor > 0 && m.MessageID <= cursor {
				continue
			}
			// Mensagem acima do cursor: entra em scannedMaxID (foi varrida).
			if m.MessageID > scannedMaxID {
				scannedMaxID = m.MessageID
			}
			// Pula mensagens mais antigas que o limite temporal (cutoff).
			if !m.Date.IsZero() && m.Date.Before(cutoff) {
				continue
			}
			job := WriteJob{
				Message: &model.RawMessage{
					ChannelID:     ch.ID,
					MessageID:     m.MessageID,
					Payload:       m.Payload,
					ReceivedAt:    time.Now().UTC(),
					SchemaVersion: schemaVersion,
				},
				Backfill: true,
			}
			select {
			case c.writeCh <- job:
			case <-ctx.Done():
				return totalFetched, scannedMaxID, ctx.Err()
			}
			totalFetched++
		}

		// Se a mensagem mais antiga nesta página estiver no cursor armazenado ou abaixo dele,
		// todas as mensagens mais recentes que o cursor foram cobertas — pare.
		if cursor > 0 && pageMinID <= cursor {
			break
		}
		// Se a mensagem mais antiga nesta página for mais antiga que o cutoff (limite temporal),
		// todas as mensagens dentro da janela temporal foram cobertas — pare (todas
		// as páginas subsequentes seriam ainda mais antigas).
		if !pageMinDate.IsZero() && pageMinDate.Before(cutoff) {
			break
		}

		// Caminhe para trás: a próxima página buscará mensagens mais velhas que pageMinID.
		offsetID = pageMinID
	}

	return totalFetched, scannedMaxID, nil
}

package telegram

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gotd/td/tg"
)

// encodeUpdate serializa um contêiner de atualização do gotd para JSON. Os tipos tg do gotd
// são structs Go simples com campos exportados, então encoding/json captura todo o formato
// bruto — o que é exatamente o que a Fase 1 precisa para descobrir os dados reais.
func encodeUpdate(u tg.UpdatesClass) ([]byte, error) {
	payload, err := json.Marshal(u)
	if err != nil {
		return nil, fmt.Errorf("marshal update: %w", err)
	}
	return payload, nil
}

// extractUpdateMeta extrai channelID e messageID de um contêiner de atualização do gotd.
// Retorna (0, 0, false) se a atualização não contiver uma mensagem de canal.
func extractUpdateMeta(u tg.UpdatesClass) (channelID, messageID int64, ok bool) {
	switch upd := u.(type) {
	case *tg.Updates:
		for _, item := range upd.Updates {
			if ch, msg, found := extractFromUpdate(item); found {
				return ch, msg, true
			}
		}
	case *tg.UpdateShort:
		if ch, msg, found := extractFromUpdate(upd.Update); found {
			return ch, msg, true
		}
	}
	return 0, 0, false
}

// extractFromUpdate extrai channelID e messageID de um único UpdateClass.
func extractFromUpdate(u tg.UpdateClass) (channelID, messageID int64, ok bool) {
	switch upd := u.(type) {
	case *tg.UpdateNewChannelMessage:
		return extractFromMessage(upd.Message)
	case *tg.UpdateNewMessage:
		return extractFromMessage(upd.Message)
	case *tg.UpdateEditChannelMessage:
		return extractFromMessage(upd.Message)
	case *tg.UpdateEditMessage:
		return extractFromMessage(upd.Message)
	}
	return 0, 0, false
}

// extractFromMessage extrai channelID e messageID de um MessageClass.
func extractFromMessage(mc tg.MessageClass) (channelID, messageID int64, ok bool) {
	msg, ok := mc.(*tg.Message)
	if !ok {
		return 0, 0, false
	}
	peer, ok := msg.PeerID.(*tg.PeerChannel)
	if !ok {
		return 0, 0, false
	}
	return peer.ChannelID, int64(msg.ID), true
}

// extractMessages extrai mensagens individuais de uma resposta de messages.getHistory,
// serializando cada uma para JSON. Ele lida com as variantes de resposta de canal (channel)
// e de fatia (slice) que o gotd pode retornar.
func extractMessages(res tg.MessagesMessagesClass) ([]HistoryMessage, error) {
	var raw []tg.MessageClass
	switch m := res.(type) {
	case *tg.MessagesChannelMessages:
		raw = m.Messages
	case *tg.MessagesMessages:
		raw = m.Messages
	case *tg.MessagesMessagesSlice:
		raw = m.Messages
	default:
		return nil, fmt.Errorf("tipo de resposta de mensagens não suportado: %T", res)
	}

	out := make([]HistoryMessage, 0, len(raw))
	for _, mc := range raw {
		msg, ok := mc.(*tg.Message)
		if !ok {
			// Ignora mensagens de serviço e outras variantes que não são mensagens comuns.
			continue
		}
		payload, err := json.Marshal(msg)
		if err != nil {
			return nil, fmt.Errorf("marshal history message %d: %w", msg.ID, err)
		}
		out = append(out, HistoryMessage{
			MessageID: int64(msg.ID),
			Date:      time.Unix(int64(msg.Date), 0),
			Payload:   payload,
		})
	}
	return out, nil
}

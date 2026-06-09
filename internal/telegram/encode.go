package telegram

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/gotd/td/tg"
)

// encodeUpdate serializes a gotd update container to JSON. gotd's tg types are
// plain Go structs with exported fields, so encoding/json captures the full
// raw shape — which is exactly what Phase 1 needs to discover the real data.
func encodeUpdate(u tg.UpdatesClass) ([]byte, error) {
	payload, err := json.Marshal(u)
	if err != nil {
		return nil, fmt.Errorf("marshal update: %w", err)
	}
	return payload, nil
}

// extractUpdateMeta extracts channelID and messageID from a gotd update
// container. Returns (0, 0, false) if the update does not contain a channel
// message.
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

// extractFromUpdate pulls channelID and messageID from a single UpdateClass.
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

// extractFromMessage extracts channelID and messageID from a MessageClass.
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

// extractMessages pulls individual messages out of a messages.getHistory
// response, serializing each to JSON. It handles both the channel and slice
// response variants gotd may return.
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
		return nil, fmt.Errorf("unsupported messages response type %T", res)
	}

	out := make([]HistoryMessage, 0, len(raw))
	for _, mc := range raw {
		msg, ok := mc.(*tg.Message)
		if !ok {
			// Skip service messages and other non-message variants.
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

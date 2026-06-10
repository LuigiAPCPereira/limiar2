package telegram

import (
	"encoding/json"
	"testing"

	"github.com/gotd/td/tg"
)

func TestEncodeUpdate(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		u := &tg.Updates{
			Updates: []tg.UpdateClass{
				&tg.UpdateNewMessage{
					Message: &tg.Message{
						ID:      123,
						Message: "hello test",
					},
				},
			},
		}

		payload, err := encodeUpdate(u)
		if err != nil {
			t.Fatalf("encodeUpdate failed: %v", err)
		}

		// Verify it's valid JSON and matches expectations.
		var raw map[string]any
		if err := json.Unmarshal(payload, &raw); err != nil {
			t.Fatalf("output is not valid JSON: %v", err)
		}

		if len(raw) == 0 {
			t.Fatalf("expected non-empty JSON object")
		}
	})
}

func TestExtractUpdateMeta(t *testing.T) {
	t.Run("updates with channel message", func(t *testing.T) {
		u := &tg.Updates{
			Updates: []tg.UpdateClass{
				&tg.UpdateNewChannelMessage{
					Message: &tg.Message{
						ID: 456,
						PeerID: &tg.PeerChannel{
							ChannelID: 123,
						},
					},
				},
			},
		}

		ch, msg, ok := extractUpdateMeta(u)
		if !ok {
			t.Fatal("expected to find message meta, but got ok=false")
		}
		if ch != 123 || msg != 456 {
			t.Fatalf("expected channelID 123 and messageID 456, got %d and %d", ch, msg)
		}
	})

	t.Run("updateshort with message", func(t *testing.T) {
		u := &tg.UpdateShort{
			Update: &tg.UpdateNewMessage{
				Message: &tg.Message{
					ID: 789,
					PeerID: &tg.PeerChannel{
						ChannelID: 321,
					},
				},
			},
		}

		ch, msg, ok := extractUpdateMeta(u)
		if !ok {
			t.Fatal("expected to find message meta, but got ok=false")
		}
		if ch != 321 || msg != 789 {
			t.Fatalf("expected channelID 321 and messageID 789, got %d and %d", ch, msg)
		}
	})

	t.Run("updates without matching message", func(t *testing.T) {
		u := &tg.Updates{
			Updates: []tg.UpdateClass{
				&tg.UpdateChatParticipantAdd{}, // some other update
			},
		}
		_, _, ok := extractUpdateMeta(u)
		if ok {
			t.Fatal("expected ok=false for update without message")
		}
	})

	t.Run("updateshort without matching message", func(t *testing.T) {
		u := &tg.UpdateShort{
			Update: &tg.UpdateChatParticipantAdd{},
		}
		_, _, ok := extractUpdateMeta(u)
		if ok {
			t.Fatal("expected ok=false for update without message")
		}
	})

	t.Run("other updatesclass type", func(t *testing.T) {
		u := &tg.UpdateShortMessage{} // doesn't hold standard update types as fields
		_, _, ok := extractUpdateMeta(u)
		if ok {
			t.Fatal("expected ok=false for unsupported UpdatesClass type")
		}
	})
}

func TestExtractFromMessage(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		mc := &tg.Message{
			ID: 42,
			PeerID: &tg.PeerChannel{
				ChannelID: 99,
			},
		}
		ch, msg, ok := extractFromMessage(mc)
		if !ok || ch != 99 || msg != 42 {
			t.Fatalf("failed to extract properly, got: ch=%d, msg=%d, ok=%t", ch, msg, ok)
		}
	})

	t.Run("not a message type", func(t *testing.T) {
		mc := &tg.MessageService{
			ID: 42,
		}
		_, _, ok := extractFromMessage(mc)
		if ok {
			t.Fatal("expected ok=false for MessageService")
		}
	})

	t.Run("no peer channel", func(t *testing.T) {
		mc := &tg.Message{
			ID: 42,
			PeerID: &tg.PeerUser{
				UserID: 99,
			},
		}
		_, _, ok := extractFromMessage(mc)
		if ok {
			t.Fatal("expected ok=false for peer that is not a channel")
		}
	})

	t.Run("nil message class", func(t *testing.T) {
		_, _, ok := extractFromMessage(nil)
		if ok {
			t.Fatal("expected ok=false for nil message class")
		}
	})

	t.Run("nil peer ID", func(t *testing.T) {
		mc := &tg.Message{
			ID:     42,
			PeerID: nil,
		}
		_, _, ok := extractFromMessage(mc)
		if ok {
			t.Fatal("expected ok=false for nil peer ID")
		}
	})
}

func TestExtractFromUpdate(t *testing.T) {
	tests := []struct {
		name      string
		update    tg.UpdateClass
		expectOk  bool
		expectCh  int64
		expectMsg int64
	}{
		{
			name: "UpdateNewChannelMessage",
			update: &tg.UpdateNewChannelMessage{
				Message: &tg.Message{ID: 1, PeerID: &tg.PeerChannel{ChannelID: 10}},
			},
			expectOk: true, expectCh: 10, expectMsg: 1,
		},
		{
			name: "UpdateNewMessage",
			update: &tg.UpdateNewMessage{
				Message: &tg.Message{ID: 2, PeerID: &tg.PeerChannel{ChannelID: 20}},
			},
			expectOk: true, expectCh: 20, expectMsg: 2,
		},
		{
			name: "UpdateEditChannelMessage",
			update: &tg.UpdateEditChannelMessage{
				Message: &tg.Message{ID: 3, PeerID: &tg.PeerChannel{ChannelID: 30}},
			},
			expectOk: true, expectCh: 30, expectMsg: 3,
		},
		{
			name: "UpdateEditMessage",
			update: &tg.UpdateEditMessage{
				Message: &tg.Message{ID: 4, PeerID: &tg.PeerChannel{ChannelID: 40}},
			},
			expectOk: true, expectCh: 40, expectMsg: 4,
		},
		{
			name:     "Unsupported update type",
			update:   &tg.UpdateChatParticipantAdd{},
			expectOk: false,
		},
		{
			name:     "Nil update",
			update:   nil,
			expectOk: false,
		},
		{
			name:     "UpdateNewChannelMessage with nil message",
			update:   &tg.UpdateNewChannelMessage{Message: nil},
			expectOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ch, msg, ok := extractFromUpdate(tt.update)
			if ok != tt.expectOk {
				t.Fatalf("expected ok=%t, got %t", tt.expectOk, ok)
			}
			if ok && (ch != tt.expectCh || msg != tt.expectMsg) {
				t.Fatalf("expected ch=%d, msg=%d, got ch=%d, msg=%d", tt.expectCh, tt.expectMsg, ch, msg)
			}
		})
	}
}

func TestExtractMessages(t *testing.T) {
	t.Run("MessagesChannelMessages", func(t *testing.T) {
		res := &tg.MessagesChannelMessages{
			Messages: []tg.MessageClass{
				&tg.Message{ID: 1, Message: "hello"},
				&tg.MessageService{ID: 2}, // Should be skipped
				&tg.Message{ID: 3, Message: "world"},
			},
		}

		msgs, err := extractMessages(res)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(msgs) != 2 {
			t.Fatalf("expected 2 messages, got %d", len(msgs))
		}
		if msgs[0].MessageID != 1 || msgs[1].MessageID != 3 {
			t.Fatalf("unexpected message IDs extracted")
		}
		if len(msgs[0].Payload) == 0 || len(msgs[1].Payload) == 0 {
			t.Fatalf("expected non-empty JSON payload for extracted messages")
		}
	})

	t.Run("MessagesMessages", func(t *testing.T) {
		res := &tg.MessagesMessages{
			Messages: []tg.MessageClass{
				&tg.Message{ID: 4},
			},
		}

		msgs, err := extractMessages(res)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		if len(msgs[0].Payload) == 0 {
			t.Fatalf("expected non-empty JSON payload for extracted messages")
		}
	})

	t.Run("MessagesMessagesSlice", func(t *testing.T) {
		res := &tg.MessagesMessagesSlice{
			Messages: []tg.MessageClass{
				&tg.Message{ID: 5},
			},
		}

		msgs, err := extractMessages(res)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(msgs) != 1 {
			t.Fatalf("expected 1 message, got %d", len(msgs))
		}
		if len(msgs[0].Payload) == 0 {
			t.Fatalf("expected non-empty JSON payload for extracted messages")
		}
	})

	t.Run("Unsupported response type", func(t *testing.T) {
		res := &tg.MessagesMessagesNotModified{}
		_, err := extractMessages(res)
		if err == nil {
			t.Fatal("expected error for unsupported type")
		}
	})
}

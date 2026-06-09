package collector_test

import (
	"context"
	"encoding/json"
	"testing"

	"pgregory.net/rapid"

	"github.com/limiar/collector/internal/collector"
	"github.com/limiar/collector/internal/storage"
	"github.com/limiar/collector/internal/telegram"
)

// Feature: limiar-collector, Property 6: NoopClassifier Identity.
// For any RawMessage, NoopClassifier.Classify returns it unchanged.
func TestProperty6NoopClassifierIdentity(t *testing.T) {
	c := collector.NoopClassifier{}
	ctx := context.Background()
	rapid.Check(t, func(t *rapid.T) {
		in := &storage.RawMessage{
			ChannelID:     rapid.Int64().Draw(t, "channel"),
			MessageID:     rapid.Int64().Draw(t, "message"),
			Payload:       rapid.SliceOfN(rapid.Byte(), 0, 128).Draw(t, "payload"),
			SchemaVersion: 1,
		}
		out, err := c.Classify(ctx, in)
		if err != nil {
			t.Fatalf("Classify error: %v", err)
		}
		if out != in {
			t.Fatalf("NoopClassifier must return the same pointer")
		}
	})
}

// Feature: limiar-collector, Property 5: RawMessage Payload Preservation.
// The handler adapts an update into a RawMessage whose Payload is the exact
// bytes it was given; persisting and reading back is byte-identical.
func TestProperty5PayloadPreservation(t *testing.T) {
	repo := newRepo(t)
	ctx := context.Background()
	ch := &storage.Channel{ID: 1, Username: "c", Title: "c", Active: true}
	if err := repo.AddChannel(ctx, ch); err != nil {
		t.Fatalf("AddChannel: %v", err)
	}

	writeCh := make(chan collector.WriteJob, 16)
	monitored := map[int64]struct{}{1: {}}
	h := collector.NewMessageHandler(collector.NoopClassifier{}, writeCh, monitored, nil)

	rapid.Check(t, func(t *rapid.T) {
		// Build a minimal update envelope the adapter can read.
		payload := map[string]any{
			"channel_id": 1,
			"message_id": rapid.IntRange(1, 1_000_000).Draw(t, "mid"),
			"text":       rapid.StringMatching(`[a-zA-Z0-9 ]{0,40}`).Draw(t, "text"),
		}
		raw, _ := json.Marshal(payload)

		update := telegram.Update{
			ChannelID: 1,
			MessageID: int64(payload["message_id"].(int)),
			Payload:   raw,
		}
		if err := h.HandleUpdate(ctx, update); err != nil {
			t.Fatalf("HandleUpdate: %v", err)
		}
		job := <-writeCh
		if string(job.Message.Payload) != string(raw) {
			t.Fatalf("payload mutated: got %q want %q", job.Message.Payload, raw)
		}
	})
}

func TestMessageHandlerExtractsChannelAndMessageID(t *testing.T) {
	ctx := context.Background()
	writeCh := make(chan collector.WriteJob, 1)
	monitored := map[int64]struct{}{555: {}}
	h := collector.NewMessageHandler(collector.NoopClassifier{}, writeCh, monitored, nil)

	raw := []byte(`{"channel_id":555,"message_id":42,"text":"hi"}`)
	update := telegram.Update{
		ChannelID: 555,
		MessageID: 42,
		Payload:   raw,
	}
	if err := h.HandleUpdate(ctx, update); err != nil {
		t.Fatalf("HandleUpdate: %v", err)
	}
	job := <-writeCh
	if job.Message.ChannelID != 555 || job.Message.MessageID != 42 {
		t.Fatalf("ids not extracted: %+v", job.Message)
	}
	if job.Message.SchemaVersion != 1 {
		t.Errorf("schema version = %d, want 1", job.Message.SchemaVersion)
	}
}

// Regression: updates with channel_id=0 (DMs, system events, non-channel
// updates) must be silently discarded. Previously the filter skipped the
// monitored-channels check when ChannelID==0, letting these through.
func TestMessageHandlerDiscardsZeroChannelID(t *testing.T) {
	ctx := context.Background()
	writeCh := make(chan collector.WriteJob, 4)
	monitored := map[int64]struct{}{100: {}}
	h := collector.NewMessageHandler(collector.NoopClassifier{}, writeCh, monitored, nil)

	update := telegram.Update{ChannelID: 0, MessageID: 0, Payload: []byte(`{}`)}
	if err := h.HandleUpdate(ctx, update); err != nil {
		t.Fatalf("HandleUpdate: %v", err)
	}
	select {
	case job := <-writeCh:
		t.Fatalf("channel_id=0 update should be discarded, but got: %+v", job)
	default:
	}
}

// Regression: updates from channels not in the monitored set must be silently
// discarded, regardless of ChannelID value.
func TestMessageHandlerDiscardsNonMonitoredChannel(t *testing.T) {
	ctx := context.Background()
	writeCh := make(chan collector.WriteJob, 4)
	monitored := map[int64]struct{}{100: {}}
	h := collector.NewMessageHandler(collector.NoopClassifier{}, writeCh, monitored, nil)

	update := telegram.Update{ChannelID: 999, MessageID: 7, Payload: []byte(`{"text":"x"}`)}
	if err := h.HandleUpdate(ctx, update); err != nil {
		t.Fatalf("HandleUpdate: %v", err)
	}
	select {
	case job := <-writeCh:
		t.Fatalf("non-monitored channel update should be discarded, but got: %+v", job)
	default:
	}
}

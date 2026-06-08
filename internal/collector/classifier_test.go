package collector_test

import (
	"context"
	"testing"

	"github.com/limiar/collector/internal/collector"
	"github.com/limiar/collector/internal/storage"
)

func TestNoopClassifier_Classify(t *testing.T) {
	classifier := collector.NoopClassifier{}
	ctx := context.Background()

	t.Run("returns original pointer", func(t *testing.T) {
		input := &storage.RawMessage{
			ChannelID: 1,
			MessageID: 2,
			Payload:   []byte(`{"test":true}`),
		}

		output, err := classifier.Classify(ctx, input)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if output != input {
			t.Errorf("expected same pointer %p, got %p", input, output)
		}
	})

	t.Run("handles nil input gracefully", func(t *testing.T) {
		output, err := classifier.Classify(ctx, nil)
		if err != nil {
			t.Fatalf("expected no error, got %v", err)
		}

		if output != nil {
			t.Errorf("expected nil output, got %v", output)
		}
	})
}

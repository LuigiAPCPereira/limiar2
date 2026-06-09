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

	t.Run("retorna o ponteiro original", func(t *testing.T) {
		input := &storage.RawMessage{
			ChannelID: 1,
			MessageID: 2,
			Payload:   []byte(`{"test":true}`),
		}

		output, err := classifier.Classify(ctx, input)
		if err != nil {
			t.Fatalf("esperava não obter erro, obteve %v", err)
		}

		if output != input {
			t.Errorf("esperava o mesmo ponteiro %p, obteve %p", input, output)
		}
	})

	t.Run("lida com entrada nil graciosamente", func(t *testing.T) {
		output, err := classifier.Classify(ctx, nil)
		if err != nil {
			t.Fatalf("esperava não obter erro, obteve %v", err)
		}

		if output != nil {
			t.Errorf("esperava saída nil, obteve %v", output)
		}
	})
}

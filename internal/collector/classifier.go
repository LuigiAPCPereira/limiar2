// Package collector owns Phase 1 message capture: it adapts raw gotd/td
// updates into RawMessage records and persists them through a single DBWriter
// goroutine (fan-in). It performs no enrichment, classification, or
// deduplication beyond safe persistence.
package collector

import (
	"context"

	"github.com/limiar/collector/internal/storage"
)

// Classifier is the Strategy interface for message classification, pluggable
// from the start. Phase 1 ships only NoopClassifier; later phases swap in
// rule- and LLM-based classifiers without touching the pipeline.
type Classifier interface {
	// Classify returns a (possibly transformed) message. Phase 1's
	// implementation returns its input unchanged.
	Classify(ctx context.Context, raw *storage.RawMessage) (*storage.RawMessage, error)
}

// NoopClassifier is the Phase 1 pass-through Classifier: it returns its input
// unchanged, preserving identity.
type NoopClassifier struct{}

var _ Classifier = NoopClassifier{}

// Classify returns raw unchanged.
func (NoopClassifier) Classify(_ context.Context, raw *storage.RawMessage) (*storage.RawMessage, error) {
	return raw, nil
}

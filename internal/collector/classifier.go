// Package collector é o responsável pela captura de mensagens na Fase 1: ele adapta updates
// brutos (raw) do gotd/td em registros RawMessage e os persiste através de uma única
// goroutine DBWriter (fan-in). Ele não realiza enriquecimento, classificação ou
// desduplicação além da persistência segura.
package collector

import (
	"context"

	"github.com/limiar/collector/internal/storage"
)

// Classifier é a interface Strategy para classificação de mensagens, plugável
// desde o início. A Fase 1 entrega apenas o NoopClassifier; fases posteriores podem substituir
// por classificadores baseados em regras e em LLMs sem precisar alterar o pipeline.
type Classifier interface {
	// Classify retorna uma mensagem (possivelmente transformada). A implementação
	// da Fase 1 retorna a sua entrada inalterada.
	Classify(ctx context.Context, raw *storage.RawMessage) (*storage.RawMessage, error)
}

// NoopClassifier é o Classifier "pass-through" (de passagem) da Fase 1: ele retorna a
// sua entrada inalterada, preservando a identidade.
type NoopClassifier struct{}

var _ Classifier = NoopClassifier{}

// Classify retorna raw inalterado.
func (NoopClassifier) Classify(_ context.Context, raw *storage.RawMessage) (*storage.RawMessage, error) {
	return raw, nil
}

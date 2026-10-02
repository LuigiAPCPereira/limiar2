package telegram

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/LuigiAPCPereira/limiar2/internal/acquisition"
	"github.com/LuigiAPCPereira/limiar2/internal/evidence"
	"github.com/gotd/td/bin"
	gotdtelegram "github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

const (
	liveAcquisition     = "telegram_live_update"
	liveEventKind       = "source_update"
	livePayloadFormat   = "telegram-tl"
	livePayloadSchema   = "gotd-td-v0.162.0:UpdatesClass"
)

var ErrInvalidLiveIngress = errors.New("ingress live do Telegram: configuração inválida")

// LiveSubscriptionClassifier decide quais Acquisition Subscriptions configuradas se
// aplicam a um envelope live recebido do Telegram.
//
// O classificador não cria identidade: retorna apenas IDs já definidos pela configuração.
// Erro ou ausência de subscriptions impede a admissão e, portanto, qualquer forward.
type LiveSubscriptionClassifier interface {
	SubscriptionIDs(context.Context, tg.UpdatesClass) ([]string, error)
}

// LiveSubscriptionClassifierFunc adapta uma função ao boundary de classificação live.
type LiveSubscriptionClassifierFunc func(context.Context, tg.UpdatesClass) ([]string, error)

func (f LiveSubscriptionClassifierFunc) SubscriptionIDs(ctx context.Context, updates tg.UpdatesClass) ([]string, error) {
	return f(ctx, updates)
}

// LiveIngress preserva o envelope bruto live antes de encaminhá-lo ao próximo handler.
//
// Ele implementa o UpdateHandler do gotd, mas não é instalado automaticamente pelo
// Runtime. A composição produtiva deve fazê-lo explicitamente quando recovery/lifecycle
// estiverem prontos para esse caminho.
type LiveIngress struct {
	admission  *acquisition.ConfiguredAdmission
	classifier LiveSubscriptionClassifier
	forward    gotdtelegram.UpdateHandler
	now        func() time.Time
}

var _ gotdtelegram.UpdateHandler = (*LiveIngress)(nil)

// NewLiveIngress constrói o adapter de envelope live do gotd.
//
// A política de quais envelopes são relevantes fica no classifier. O adapter serializa
// o envelope TL exato recebido, contextualiza Evidence através de ConfiguredAdmission e
// somente então chama forward.
func NewLiveIngress(
	admission *acquisition.ConfiguredAdmission,
	classifier LiveSubscriptionClassifier,
	forward gotdtelegram.UpdateHandler,
) (*LiveIngress, error) {
	return newLiveIngressWithClock(admission, classifier, forward, time.Now)
}

func newLiveIngressWithClock(
	admission *acquisition.ConfiguredAdmission,
	classifier LiveSubscriptionClassifier,
	forward gotdtelegram.UpdateHandler,
	now func() time.Time,
) (*LiveIngress, error) {
	switch {
	case admission == nil:
		return nil, fmt.Errorf("%w: Source Admission configurada ausente", ErrInvalidLiveIngress)
	case classifier == nil:
		return nil, fmt.Errorf("%w: classificador de subscription ausente", ErrInvalidLiveIngress)
	case forward == nil:
		return nil, fmt.Errorf("%w: handler downstream ausente", ErrInvalidLiveIngress)
	case now == nil:
		return nil, fmt.Errorf("%w: relógio ausente", ErrInvalidLiveIngress)
	default:
		return &LiveIngress{
			admission:  admission,
			classifier: classifier,
			forward:    forward,
			now:        now,
		}, nil
	}
}

// Handle captura e persiste o envelope recebido antes do downstream.
//
// SourceOccurredAt permanece ausente neste boundary porque tg.UpdatesClass possui formas
// distintas e nem todo envelope oferece um timestamp de fonte semanticamente equivalente.
// ReceivedAt registra o instante local de admissão. Inferir data de mensagem aqui
// misturaria envelope de fonte com projeção derivada.
func (h *LiveIngress) Handle(ctx context.Context, updates tg.UpdatesClass) error {
	if h == nil || h.admission == nil || h.classifier == nil || h.forward == nil || h.now == nil {
		return fmt.Errorf("%w: adapter não inicializado", ErrInvalidLiveIngress)
	}
	if updates == nil {
		return fmt.Errorf("%w: envelope Telegram ausente", ErrInvalidLiveIngress)
	}

	subscriptionIDs, err := h.classifier.SubscriptionIDs(ctx, updates)
	if err != nil {
		return fmt.Errorf("ingress live do Telegram: classificar subscriptions: %w", err)
	}

	payload, sourceEventType, err := encodeLiveUpdates(updates)
	if err != nil {
		return err
	}

	item := evidence.Evidence{
		Acquisition:     liveAcquisition,
		EventKind:       liveEventKind,
		SourceEventType: sourceEventType,
		ReceivedAt:      h.now().UTC().UnixMilli(),
		PayloadFormat:   livePayloadFormat,
		PayloadSchema:   livePayloadSchema,
		Payload:         payload,
	}

	return h.admission.Admit(ctx, subscriptionIDs, item, func(ctx context.Context) error {
		return h.forward.Handle(ctx, updates)
	})
}

func encodeLiveUpdates(updates tg.UpdatesClass) ([]byte, string, error) {
	encoder, ok := any(updates).(bin.Encoder)
	if !ok {
		return nil, "", fmt.Errorf("%w: envelope %T não implementa serialização TL", ErrInvalidLiveIngress, updates)
	}

	var buffer bin.Buffer
	if err := buffer.Encode(encoder); err != nil {
		return nil, "", fmt.Errorf("ingress live do Telegram: serializar envelope %T: %w", updates, err)
	}

	eventType := fmt.Sprintf("%T", updates)
	if named, ok := any(updates).(interface{ TypeName() string }); ok {
		if name := named.TypeName(); name != "" {
			eventType = name
		}
	}

	return buffer.Copy(), eventType, nil
}

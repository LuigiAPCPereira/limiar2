package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/LuigiAPCPereira/limiar2/internal/acquisition"
	"github.com/LuigiAPCPereira/limiar2/internal/evidence"
	"github.com/LuigiAPCPereira/limiar2/internal/recovery"
	"github.com/gotd/td/bin"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
)

const (
	recoveryAcquisition   = "telegram_recovery"
	recoveryPayloadFormat = "telegram-tl"
	recoveryPayloadSchema = "gotd-td-v0.162.0:updates.API"

	recoveryEventBaseline      = "sync_baseline"
	recoveryEventObservation   = "recovery_observation"
	recoveryEventDiscontinuity = "sync_discontinuity"
)

var (
	ErrInvalidRecoveryAPI      = errors.New("recovery Telegram: configuração inválida")
	ErrUnknownRecoveryResponse = errors.New("recovery Telegram: classe de resposta desconhecida")
)

// GuardedRecoveryAPI preserva respostas de recovery semanticamente relevantes antes de
// entregá-las ao updates.Manager.
//
// A instância representa exatamente uma Acquisition Subscription. O upstream continua
// sendo a interface updates.API oficial do gotd.
type GuardedRecoveryAPI struct {
	upstream       updates.API
	admission      *acquisition.ConfiguredAdmission
	subscriptionID string
	barrier        *recovery.DurabilityBarrier
	now            func() time.Time
}

var _ updates.API = (*GuardedRecoveryAPI)(nil)

// NewGuardedRecoveryAPI constrói o boundary de recovery para uma subscription.
func NewGuardedRecoveryAPI(
	upstream updates.API,
	admission *acquisition.ConfiguredAdmission,
	subscriptionID string,
	barrier *recovery.DurabilityBarrier,
) (*GuardedRecoveryAPI, error) {
	return newGuardedRecoveryAPIWithClock(upstream, admission, subscriptionID, barrier, time.Now)
}

func newGuardedRecoveryAPIWithClock(
	upstream updates.API,
	admission *acquisition.ConfiguredAdmission,
	subscriptionID string,
	barrier *recovery.DurabilityBarrier,
	now func() time.Time,
) (*GuardedRecoveryAPI, error) {
	switch {
	case upstream == nil:
		return nil, fmt.Errorf("%w: updates.API upstream ausente", ErrInvalidRecoveryAPI)
	case admission == nil:
		return nil, fmt.Errorf("%w: Source Admission configurada ausente", ErrInvalidRecoveryAPI)
	case strings.TrimSpace(subscriptionID) == "":
		return nil, fmt.Errorf("%w: subscription_id ausente", ErrInvalidRecoveryAPI)
	case strings.TrimSpace(subscriptionID) != subscriptionID:
		return nil, fmt.Errorf("%w: subscription_id contém whitespace nas bordas", ErrInvalidRecoveryAPI)
	case barrier == nil:
		return nil, fmt.Errorf("%w: DurabilityBarrier ausente", ErrInvalidRecoveryAPI)
	case now == nil:
		return nil, fmt.Errorf("%w: relógio ausente", ErrInvalidRecoveryAPI)
	default:
		return &GuardedRecoveryAPI{
			upstream:       upstream,
			admission:      admission,
			subscriptionID: subscriptionID,
			barrier:        barrier,
			now:            now,
		}, nil
	}
}

// UpdatesGetState sempre representa adoção potencial de baseline remoto no lifecycle do
// manager: bootstrap sem state local ou resync/Forget explícito.
func (a *GuardedRecoveryAPI) UpdatesGetState(ctx context.Context) (*tg.UpdatesState, error) {
	if err := a.ensureUsable(); err != nil {
		return nil, err
	}

	state, err := a.upstream.UpdatesGetState(ctx)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, a.failClosed(fmt.Errorf("%w: UpdatesGetState retornou nil", ErrUnknownRecoveryResponse))
	}

	if err := a.preserve(ctx, state, recoveryEventBaseline); err != nil {
		return nil, err
	}
	return state, nil
}

func (a *GuardedRecoveryAPI) UpdatesGetDifference(
	ctx context.Context,
	request *tg.UpdatesGetDifferenceRequest,
) (tg.UpdatesDifferenceClass, error) {
	if err := a.ensureUsable(); err != nil {
		return nil, err
	}

	diff, err := a.upstream.UpdatesGetDifference(ctx, request)
	if err != nil {
		return nil, err
	}

	required, eventKind, err := classifyCommonDifference(diff)
	if err != nil {
		return nil, a.failClosed(err)
	}
	if required {
		if err := a.preserve(ctx, diff, eventKind); err != nil {
			return nil, err
		}
	} else if err := a.ensureUsable(); err != nil {
		return nil, err
	}

	return diff, nil
}

func (a *GuardedRecoveryAPI) UpdatesGetChannelDifference(
	ctx context.Context,
	request *tg.UpdatesGetChannelDifferenceRequest,
) (tg.UpdatesChannelDifferenceClass, error) {
	if err := a.ensureUsable(); err != nil {
		return nil, err
	}

	diff, err := a.upstream.UpdatesGetChannelDifference(ctx, request)
	if err != nil {
		return nil, err
	}

	required, eventKind, err := classifyChannelDifference(diff)
	if err != nil {
		return nil, a.failClosed(err)
	}
	if required {
		if err := a.preserve(ctx, diff, eventKind); err != nil {
			return nil, err
		}
	} else if err := a.ensureUsable(); err != nil {
		return nil, err
	}

	return diff, nil
}

func classifyCommonDifference(diff tg.UpdatesDifferenceClass) (bool, string, error) {
	switch value := diff.(type) {
	case *tg.UpdatesDifferenceEmpty:
		return false, "", nil
	case *tg.UpdatesDifferenceTooLong:
		return true, recoveryEventDiscontinuity, nil
	case *tg.UpdatesDifference:
		return commonDifferenceHasObservations(value.NewMessages, value.NewEncryptedMessages, value.OtherUpdates),
			recoveryEventObservation, nil
	case *tg.UpdatesDifferenceSlice:
		return commonDifferenceHasObservations(value.NewMessages, value.NewEncryptedMessages, value.OtherUpdates),
			recoveryEventObservation, nil
	case nil:
		return false, "", fmt.Errorf("%w: UpdatesGetDifference retornou nil", ErrUnknownRecoveryResponse)
	default:
		return false, "", fmt.Errorf("%w: UpdatesGetDifference retornou %T", ErrUnknownRecoveryResponse, diff)
	}
}

func commonDifferenceHasObservations(
	messages []tg.MessageClass,
	encrypted []tg.EncryptedMessageClass,
	other []tg.UpdateClass,
) bool {
	return len(messages) > 0 || len(encrypted) > 0 || len(other) > 0
}

func classifyChannelDifference(diff tg.UpdatesChannelDifferenceClass) (bool, string, error) {
	switch value := diff.(type) {
	case *tg.UpdatesChannelDifferenceEmpty:
		return false, "", nil
	case *tg.UpdatesChannelDifferenceTooLong:
		return true, recoveryEventDiscontinuity, nil
	case *tg.UpdatesChannelDifference:
		return len(value.NewMessages) > 0 || len(value.OtherUpdates) > 0,
			recoveryEventObservation, nil
	case nil:
		return false, "", fmt.Errorf("%w: UpdatesGetChannelDifference retornou nil", ErrUnknownRecoveryResponse)
	default:
		return false, "", fmt.Errorf("%w: UpdatesGetChannelDifference retornou %T", ErrUnknownRecoveryResponse, diff)
	}
}

func (a *GuardedRecoveryAPI) preserve(ctx context.Context, response any, eventKind string) error {
	return a.barrier.Guard(func() error {
		payload, sourceEventType, err := encodeRecoveryResponse(response)
		if err != nil {
			return err
		}

		item := evidence.Evidence{
			Acquisition:     recoveryAcquisition,
			EventKind:       eventKind,
			SourceEventType: sourceEventType,
			ReceivedAt:      a.now().UTC().UnixMilli(),
			PayloadFormat:   recoveryPayloadFormat,
			PayloadSchema:   recoveryPayloadSchema,
			Payload:         payload,
		}

		return a.admission.Admit(
			ctx,
			[]string{a.subscriptionID},
			item,
			func(context.Context) error { return nil },
		)
	})
}

func encodeRecoveryResponse(response any) ([]byte, string, error) {
	encoder, ok := response.(bin.Encoder)
	if !ok {
		return nil, "", fmt.Errorf("%w: resposta %T não implementa serialização TL", ErrUnknownRecoveryResponse, response)
	}

	var buffer bin.Buffer
	if err := buffer.Encode(encoder); err != nil {
		return nil, "", fmt.Errorf("recovery Telegram: serializar resposta %T: %w", response, err)
	}

	eventType := fmt.Sprintf("%T", response)
	if named, ok := response.(interface{ TypeName() string }); ok {
		if name := named.TypeName(); name != "" {
			eventType = name
		}
	}

	return buffer.Copy(), eventType, nil
}

func (a *GuardedRecoveryAPI) ensureUsable() error {
	if a == nil || a.upstream == nil || a.admission == nil || a.barrier == nil || a.now == nil {
		return fmt.Errorf("%w: adapter não inicializado", ErrInvalidRecoveryAPI)
	}
	if err := a.barrier.Err(); err != nil {
		return fmt.Errorf("recovery Telegram: lifecycle encerrado: %w", err)
	}
	return nil
}

func (a *GuardedRecoveryAPI) failClosed(cause error) error {
	if cause == nil {
		cause = ErrUnknownRecoveryResponse
	}
	a.barrier.Close(cause)
	if err := a.barrier.Err(); err != nil {
		return fmt.Errorf("recovery Telegram: resposta bloqueada: %w", err)
	}
	return cause
}

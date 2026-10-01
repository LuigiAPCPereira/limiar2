package acquisition

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/LuigiAPCPereira/limiar2/internal/evidence"
)

var (
	ErrInvalidSubscriptionConfig    = errors.New("acquisition subscription: configuração inválida")
	ErrInvalidSubscriptionSelection = errors.New("acquisition subscription: seleção inválida")
)

// Subscription representa a identidade configurada de uma Acquisition Subscription.
//
// O ID é uma chave opaca e estável. A política que decide quais subscriptions se aplicam
// a um envelope pertence ao boundary de classificação, não a este tipo.
type Subscription struct {
	ID string
}

// ConfiguredAdmission liga uma snapshot validada de Acquisition Subscriptions ao core
// de Source Admission. Ele não deriva identidade de mensagem, canal, sessão ou MCP.
type ConfiguredAdmission struct {
	core       *Admission
	configured map[string]struct{}
}

// NewConfiguredAdmission cria o boundary configurado de admissão.
//
// A snapshot precisa conter ao menos uma subscription, com IDs exatos, não vazios e
// únicos. Whitespace nas bordas é rejeitado em vez de normalizado silenciosamente.
func NewConfiguredAdmission(core *Admission, subscriptions []Subscription) (*ConfiguredAdmission, error) {
	if core == nil || core.appender == nil {
		return nil, fmt.Errorf("%w: Source Admission core ausente", ErrInvalidSubscriptionConfig)
	}
	if len(subscriptions) == 0 {
		return nil, fmt.Errorf("%w: nenhuma subscription configurada", ErrInvalidSubscriptionConfig)
	}

	configured := make(map[string]struct{}, len(subscriptions))
	for _, subscription := range subscriptions {
		if err := validateSubscriptionID(subscription.ID); err != nil {
			return nil, err
		}
		if _, exists := configured[subscription.ID]; exists {
			return nil, fmt.Errorf("%w: subscription duplicada %q", ErrInvalidSubscriptionConfig, subscription.ID)
		}
		configured[subscription.ID] = struct{}{}
	}

	return &ConfiguredAdmission{
		core:       core,
		configured: configured,
	}, nil
}

func validateSubscriptionID(id string) error {
	trimmed := strings.TrimSpace(id)
	switch {
	case trimmed == "":
		return fmt.Errorf("%w: subscription_id vazio", ErrInvalidSubscriptionConfig)
	case id != trimmed:
		return fmt.Errorf("%w: subscription_id %q contém whitespace nas bordas", ErrInvalidSubscriptionConfig, id)
	default:
		return nil
	}
}

// Admit persiste uma Evidence para cada subscription aplicável e somente então executa
// forward uma única vez.
//
// subscriptionIDs deve ser a classificação explícita do envelope contra a snapshot
// configurada. Todos os IDs são validados antes do primeiro append. Se um append falhar
// depois de outro ter sido persistido, forward não é executado e a Evidence já durável
// permanece disponível para replay.
func (a *ConfiguredAdmission) Admit(
	ctx context.Context,
	subscriptionIDs []string,
	item evidence.Evidence,
	forward func(context.Context) error,
) error {
	if a == nil || a.core == nil || a.core.appender == nil || a.configured == nil {
		return fmt.Errorf("%w: boundary não inicializado", ErrInvalidSubscriptionConfig)
	}
	if forward == nil {
		return fmt.Errorf("%w: forward obrigatório", ErrInvalidSubscriptionSelection)
	}
	if item.SubscriptionID != "" {
		return fmt.Errorf("%w: Evidence já contém subscription_id; a identidade deve vir da configuração", ErrInvalidSubscriptionSelection)
	}

	selected, err := a.validateSelection(subscriptionIDs)
	if err != nil {
		return err
	}

	for _, subscriptionID := range selected {
		contextualized := item
		contextualized.SubscriptionID = subscriptionID
		if _, err := a.core.appender.Append(ctx, contextualized); err != nil {
			return fmt.Errorf("source admission configurada: persistir Evidence da subscription %q: %w", subscriptionID, err)
		}
	}

	if err := forward(ctx); err != nil {
		return fmt.Errorf("source admission configurada: encaminhar após Evidence durável: %w", err)
	}
	return nil
}

func (a *ConfiguredAdmission) validateSelection(subscriptionIDs []string) ([]string, error) {
	if len(subscriptionIDs) == 0 {
		return nil, fmt.Errorf("%w: nenhuma subscription aplicável", ErrInvalidSubscriptionSelection)
	}

	selected := append([]string(nil), subscriptionIDs...)
	seen := make(map[string]struct{}, len(selected))

	for _, id := range selected {
		if err := validateSubscriptionID(id); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrInvalidSubscriptionSelection, err)
		}
		if _, duplicate := seen[id]; duplicate {
			return nil, fmt.Errorf("%w: subscription repetida %q", ErrInvalidSubscriptionSelection, id)
		}
		if _, configured := a.configured[id]; !configured {
			return nil, fmt.Errorf("%w: subscription %q não está configurada", ErrInvalidSubscriptionSelection, id)
		}
		seen[id] = struct{}{}
	}

	return selected, nil
}

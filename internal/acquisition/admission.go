// Package acquisition implementa boundaries de admissão da fonte sem acoplar
// configuração de aquisição, Telegram ou recovery ao storage de Evidence.
package acquisition

import (
	"context"
	"errors"
	"fmt"

	"github.com/LuigiAPCPereira/limiar2/internal/evidence"
)

// Admission garante o ordering mínimo aceito pelo ADR 018:
// Source Evidence durável antes de qualquer encaminhamento downstream.
//
// A origem de SubscriptionID não pertence a este tipo. O caller fornece a Evidence
// já contextualizada; ligar esse contexto à configuração produtiva continua sujeito
// à Decision específica de identidade de subscription.
type Admission struct {
	appender evidence.EvidenceAppender
}

// NewAdmission cria o boundary genérico de Source Admission.
func NewAdmission(appender evidence.EvidenceAppender) (*Admission, error) {
	if appender == nil {
		return nil, errors.New("source admission: EvidenceAppender obrigatório")
	}
	return &Admission{appender: appender}, nil
}

// Admit persiste a Source Evidence antes de executar forward.
//
// Se o append falhar, forward não é chamado. Se forward falhar depois do append,
// a Evidence permanece durável e o erro é propagado; replay é esperado pelo contrato.
func (a *Admission) Admit(ctx context.Context, item evidence.Evidence, forward func(context.Context) error) error {
	if a == nil || a.appender == nil {
		return errors.New("source admission: não inicializada")
	}
	if forward == nil {
		return errors.New("source admission: forward obrigatório")
	}

	if _, err := a.appender.Append(ctx, item); err != nil {
		return fmt.Errorf("source admission: persistir Evidence: %w", err)
	}
	if err := forward(ctx); err != nil {
		return fmt.Errorf("source admission: encaminhar após Evidence durável: %w", err)
	}
	return nil
}

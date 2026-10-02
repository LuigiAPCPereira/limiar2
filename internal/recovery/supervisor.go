package recovery

import (
	"context"
	"errors"
	"fmt"
)

var ErrInvalidSupervisor = errors.New("recovery: supervisor inválido")

// Supervisor possui o lifecycle fail-stop de uma instância de recovery.
//
// A barrier é a autoridade terminal: quando fecha, o contexto do lifecycle é cancelado e
// Run só retorna depois que o lifecycle observado encerra. O lifecycle deve respeitar
// cancelamento de context; updates.Manager.Run satisfaz esse formato.
type Supervisor struct {
	barrier *DurabilityBarrier
}

// NewSupervisor cria um supervisor para exatamente uma barrier/lifecycle.
func NewSupervisor(barrier *DurabilityBarrier) (*Supervisor, error) {
	if barrier == nil {
		return nil, fmt.Errorf("%w: DurabilityBarrier ausente", ErrInvalidSupervisor)
	}
	return &Supervisor{barrier: barrier}, nil
}

// Run executa lifecycle até conclusão normal, cancelamento do parent context ou
// fechamento terminal da DurabilityBarrier.
//
// Se a barrier fechar, sua primeira causa é preservada no erro retornado mesmo que o
// lifecycle encerre com context cancellation.
func (s *Supervisor) Run(
	ctx context.Context,
	lifecycle func(context.Context) error,
) error {
	if s == nil || s.barrier == nil {
		return fmt.Errorf("%w: supervisor não inicializado", ErrInvalidSupervisor)
	}
	if ctx == nil {
		return fmt.Errorf("%w: context ausente", ErrInvalidSupervisor)
	}
	if lifecycle == nil {
		return fmt.Errorf("%w: lifecycle ausente", ErrInvalidSupervisor)
	}
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("recovery supervisor: contexto já encerrado: %w", err)
	}
	if err := s.barrier.Err(); err != nil {
		return fmt.Errorf("recovery supervisor: barrier já fechada: %w", err)
	}

	lifecycleCtx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)

	done := make(chan error, 1)
	go func() {
		done <- lifecycle(lifecycleCtx)
	}()

	select {
	case err := <-done:
		if barrierErr := s.barrier.Err(); barrierErr != nil {
			return fmt.Errorf("recovery supervisor: lifecycle encerrou com barrier fechada: %w", barrierErr)
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return fmt.Errorf("recovery supervisor: contexto encerrado: %w", ctxErr)
		}
		if err != nil {
			return fmt.Errorf("recovery supervisor: lifecycle falhou: %w", err)
		}
		return nil

	case <-s.barrier.Done():
		barrierErr := s.barrier.Err()
		if barrierErr == nil {
			barrierErr = ErrDurabilityBarrierClosed
		}
		cancel(barrierErr)
		<-done
		return fmt.Errorf("recovery supervisor: lifecycle interrompido pela durability barrier: %w", barrierErr)

	case <-ctx.Done():
		cancel(ctx.Err())
		<-done
		return fmt.Errorf("recovery supervisor: contexto encerrado: %w", ctx.Err())
	}
}

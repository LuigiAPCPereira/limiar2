// Package recovery implementa os boundaries de durabilidade e fail-stop do
// recovery live do Telegram sem possuir o storage físico de SourceSyncState.
package recovery

import (
	"errors"
	"fmt"
	"sync"
)

var (
	// ErrDurabilityBarrierClosed sinaliza que a instância de recovery não pode mais
	// certificar progresso. Uma barrier fechada é terminal para aquele lifecycle.
	ErrDurabilityBarrierClosed = errors.New("recovery: durability barrier fechada")

	// ErrInvalidBarrierOperation representa uso inválido do boundary protegido.
	ErrInvalidBarrierOperation = errors.New("recovery: operação protegida inválida")
)

// DurabilityBarrier serializa a verificação de durabilidade com as transições que
// dependem dela. A primeira causa de fechamento é preservada para diagnóstico.
//
// Uma instância fechada nunca reabre. Restart cria uma nova barrier.
type DurabilityBarrier struct {
	mu     sync.Mutex
	closed bool
	cause  error
	done   chan struct{}
}

// NewDurabilityBarrier cria uma barrier inicialmente aberta.
func NewDurabilityBarrier() *DurabilityBarrier {
	return &DurabilityBarrier{done: make(chan struct{})}
}

// Close fecha a barrier de forma terminal.
//
// Retorna true somente para o caller que realizou a primeira transição aberta->fechada.
// Chamadas posteriores não substituem a causa original.
func (b *DurabilityBarrier) Close(cause error) bool {
	if b == nil {
		return false
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return false
	}
	b.closeLocked(cause)
	return true
}

// Done é fechado exatamente quando a barrier entra no estado terminal.
//
// A channel existe também para o zero value de DurabilityBarrier. Receiver nil retorna
// uma channel já fechada, preservando fail-closed para supervisors mal configurados.
func (b *DurabilityBarrier) Done() <-chan struct{} {
	if b == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.done == nil {
		b.done = make(chan struct{})
		if b.closed {
			close(b.done)
		}
	}
	return b.done
}

// Err retorna nil enquanto a barrier está aberta. Depois do fechamento, retorna um erro
// que satisfaz errors.Is(err, ErrDurabilityBarrierClosed) e preserva a primeira causa.
func (b *DurabilityBarrier) Err() error {
	if b == nil {
		return &barrierClosedError{
			cause: fmt.Errorf("%w: barrier ausente", ErrInvalidBarrierOperation),
		}
	}

	b.mu.Lock()
	defer b.mu.Unlock()
	return b.errLocked()
}

// Guard executa operation somente enquanto a barrier está aberta, mantendo a checagem e
// a operação na mesma região crítica.
//
// Esse contrato permite que GuardedStateStorage faça barrier-check + state-write sem
// TOCTOU. Se operation falhar, a barrier é fechada com esse erro antes de Guard retornar.
// O callback não deve reentrar na mesma barrier.
func (b *DurabilityBarrier) Guard(operation func() error) error {
	if b == nil {
		return &barrierClosedError{
			cause: fmt.Errorf("%w: barrier ausente", ErrInvalidBarrierOperation),
		}
	}
	if operation == nil {
		cause := fmt.Errorf("%w: callback ausente", ErrInvalidBarrierOperation)
		b.Close(cause)
		return b.Err()
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	if b.closed {
		return b.errLocked()
	}

	if err := operation(); err != nil {
		b.closeLocked(err)
		return b.errLocked()
	}

	return nil
}

func (b *DurabilityBarrier) closeLocked(cause error) {
	b.closed = true
	b.cause = cause
	if b.done != nil {
		close(b.done)
	}
}

func (b *DurabilityBarrier) errLocked() error {
	if !b.closed {
		return nil
	}
	return &barrierClosedError{cause: b.cause}
}

type barrierClosedError struct {
	cause error
}

func (e *barrierClosedError) Error() string {
	if e == nil || e.cause == nil {
		return ErrDurabilityBarrierClosed.Error()
	}
	return fmt.Sprintf("%s: %v", ErrDurabilityBarrierClosed, e.cause)
}

func (e *barrierClosedError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

func (e *barrierClosedError) Is(target error) bool {
	return target == ErrDurabilityBarrierClosed
}

package recovery

import (
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
)

func TestDurabilityBarrierStartsOpen(t *testing.T) {
	barrier := NewDurabilityBarrier()
	if err := barrier.Err(); err != nil {
		t.Fatalf("barrier nova deveria estar aberta: %v", err)
	}
}

func TestDurabilityBarrierCloseIsTerminalAndPreservesFirstCause(t *testing.T) {
	first := errors.New("primeira falha de Evidence")
	second := errors.New("falha posterior")

	barrier := NewDurabilityBarrier()
	if !barrier.Close(first) {
		t.Fatal("primeiro Close deveria fechar a barrier")
	}
	if barrier.Close(second) {
		t.Fatal("segundo Close não deveria alterar uma barrier já fechada")
	}

	err := barrier.Err()
	if !errors.Is(err, ErrDurabilityBarrierClosed) {
		t.Fatalf("erro=%v, esperado ErrDurabilityBarrierClosed", err)
	}
	if !errors.Is(err, first) {
		t.Fatalf("erro=%v não preserva a primeira causa", err)
	}
	if errors.Is(err, second) {
		t.Fatalf("erro=%v substituiu a primeira causa pela segunda", err)
	}
}

func TestDurabilityBarrierGuardRunsWhileOpen(t *testing.T) {
	barrier := NewDurabilityBarrier()
	calls := 0

	if err := barrier.Guard(func() error {
		calls++
		return nil
	}); err != nil {
		t.Fatalf("Guard: %v", err)
	}

	if calls != 1 {
		t.Fatalf("operation chamada %d vezes, esperado 1", calls)
	}
	if err := barrier.Err(); err != nil {
		t.Fatalf("operation bem-sucedida fechou a barrier: %v", err)
	}
}

func TestDurabilityBarrierGuardRejectsClosedWithoutRunningOperation(t *testing.T) {
	cause := errors.New("storage indisponível")
	barrier := NewDurabilityBarrier()
	barrier.Close(cause)

	called := false
	err := barrier.Guard(func() error {
		called = true
		return nil
	})

	if called {
		t.Fatal("operation executada após fechamento da barrier")
	}
	if !errors.Is(err, ErrDurabilityBarrierClosed) || !errors.Is(err, cause) {
		t.Fatalf("erro=%v não preserva barrier fechada e causa", err)
	}
}

func TestDurabilityBarrierGuardFailureClosesBeforeReturn(t *testing.T) {
	writeErr := errors.New("falha ao persistir state")
	barrier := NewDurabilityBarrier()

	err := barrier.Guard(func() error {
		return writeErr
	})
	if !errors.Is(err, ErrDurabilityBarrierClosed) {
		t.Fatalf("erro=%v, esperado barrier fechada", err)
	}
	if !errors.Is(err, writeErr) {
		t.Fatalf("erro=%v não preserva falha da operation", err)
	}

	after := barrier.Err()
	if !errors.Is(after, ErrDurabilityBarrierClosed) || !errors.Is(after, writeErr) {
		t.Fatalf("barrier não estava fechada ao retornar: %v", after)
	}
}

func TestDurabilityBarrierNilOperationFailsClosed(t *testing.T) {
	barrier := NewDurabilityBarrier()

	err := barrier.Guard(nil)
	if !errors.Is(err, ErrDurabilityBarrierClosed) {
		t.Fatalf("erro=%v, esperado barrier fechada", err)
	}
	if !errors.Is(err, ErrInvalidBarrierOperation) {
		t.Fatalf("erro=%v, esperado operação inválida como causa", err)
	}
	if !errors.Is(barrier.Err(), ErrDurabilityBarrierClosed) {
		t.Fatal("callback nil não deixou a barrier fechada")
	}
}

func TestDurabilityBarrierNilReceiverFailsClosed(t *testing.T) {
	var barrier *DurabilityBarrier

	err := barrier.Err()
	if !errors.Is(err, ErrDurabilityBarrierClosed) {
		t.Fatalf("Err nil receiver=%v, esperado barrier fechada", err)
	}
	if !errors.Is(err, ErrInvalidBarrierOperation) {
		t.Fatalf("Err nil receiver=%v, esperado configuração inválida", err)
	}

	called := false
	err = barrier.Guard(func() error {
		called = true
		return nil
	})
	if called {
		t.Fatal("nil receiver executou operation")
	}
	if !errors.Is(err, ErrDurabilityBarrierClosed) {
		t.Fatalf("Guard nil receiver=%v, esperado barrier fechada", err)
	}
}

func TestDurabilityBarrierConcurrentCloseHasSingleWinner(t *testing.T) {
	barrier := NewDurabilityBarrier()
	const workers = 32

	start := make(chan struct{})
	results := make(chan bool, workers)
	var wg sync.WaitGroup

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			results <- barrier.Close(fmt.Errorf("falha %d", i))
		}(i)
	}

	close(start)
	wg.Wait()
	close(results)

	var winners int32
	for won := range results {
		if won {
			atomic.AddInt32(&winners, 1)
		}
	}

	if winners != 1 {
		t.Fatalf("vencedores do Close=%d, esperado 1", winners)
	}
	if err := barrier.Err(); !errors.Is(err, ErrDurabilityBarrierClosed) {
		t.Fatalf("barrier não fechou após corrida de Close: %v", err)
	}
}


func TestDurabilityBarrierDoneSignalsCloseAndGuardFailure(t *testing.T) {
	t.Run("Close", func(t *testing.T) {
		barrier := NewDurabilityBarrier()
		done := barrier.Done()
		select {
		case <-done:
			t.Fatal("Done fechada antes da barrier")
		default:
		}

		barrier.Close(errors.New("falha"))
		select {
		case <-done:
		default:
			t.Fatal("Done não fechou com Close")
		}
	})

	t.Run("Guard failure", func(t *testing.T) {
		barrier := NewDurabilityBarrier()
		done := barrier.Done()
		if err := barrier.Guard(func() error { return errors.New("write falhou") }); err == nil {
			t.Fatal("Guard deveria falhar")
		}
		select {
		case <-done:
		default:
			t.Fatal("Done não fechou com falha de Guard")
		}
	})
}

func TestDurabilityBarrierDoneSupportsZeroValueAndNilReceiver(t *testing.T) {
	var zero DurabilityBarrier
	done := zero.Done()
	select {
	case <-done:
		t.Fatal("zero value aberto sinalizou fechamento")
	default:
	}
	if !zero.Close(errors.New("falha")) {
		t.Fatal("zero value não fechou")
	}
	select {
	case <-done:
	default:
		t.Fatal("Done do zero value não fechou")
	}

	var nilBarrier *DurabilityBarrier
	select {
	case <-nilBarrier.Done():
	default:
		t.Fatal("nil receiver deveria sinalizar fail-closed")
	}
}

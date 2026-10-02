package recovery

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestSupervisorRejectsInvalidConfiguration(t *testing.T) {
	if got, err := NewSupervisor(nil); !errors.Is(err, ErrInvalidSupervisor) || got != nil {
		t.Fatalf("NewSupervisor(nil) got=%v err=%v", got, err)
	}

	supervisor, err := NewSupervisor(NewDurabilityBarrier())
	if err != nil {
		t.Fatal(err)
	}

	if err := supervisor.Run(nil, func(context.Context) error { return nil }); !errors.Is(err, ErrInvalidSupervisor) {
		t.Fatalf("Run nil context=%v", err)
	}
	if err := supervisor.Run(context.Background(), nil); !errors.Is(err, ErrInvalidSupervisor) {
		t.Fatalf("Run nil lifecycle=%v", err)
	}
}

func TestSupervisorAllowsNormalLifecycleCompletion(t *testing.T) {
	barrier := NewDurabilityBarrier()
	supervisor, err := NewSupervisor(barrier)
	if err != nil {
		t.Fatal(err)
	}

	called := false
	err = supervisor.Run(context.Background(), func(context.Context) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if !called {
		t.Fatal("lifecycle não foi executado")
	}
	if err := barrier.Err(); err != nil {
		t.Fatalf("lifecycle normal fechou barrier: %v", err)
	}
}

func TestSupervisorPropagatesLifecycleFailureWithoutClosingBarrier(t *testing.T) {
	barrier := NewDurabilityBarrier()
	supervisor, err := NewSupervisor(barrier)
	if err != nil {
		t.Fatal(err)
	}
	sentinel := errors.New("manager falhou")

	err = supervisor.Run(context.Background(), func(context.Context) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("erro=%v, esperado lifecycle failure", err)
	}
	if barrierErr := barrier.Err(); barrierErr != nil {
		t.Fatalf("lifecycle failure fechou barrier sem authority: %v", barrierErr)
	}
}

func TestSupervisorRefusesLifecycleWhenBarrierAlreadyClosed(t *testing.T) {
	barrier := NewDurabilityBarrier()
	cause := errors.New("Evidence falhou")
	barrier.Close(cause)

	supervisor, err := NewSupervisor(barrier)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	err = supervisor.Run(context.Background(), func(context.Context) error {
		called = true
		return nil
	})

	if called {
		t.Fatal("lifecycle iniciou com barrier já fechada")
	}
	if !errors.Is(err, ErrDurabilityBarrierClosed) || !errors.Is(err, cause) {
		t.Fatalf("erro=%v não preserva barrier/cause", err)
	}
}

func TestSupervisorCancelsLifecycleWhenBarrierCloses(t *testing.T) {
	barrier := NewDurabilityBarrier()
	supervisor, err := NewSupervisor(barrier)
	if err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	exited := make(chan error, 1)
	result := make(chan error, 1)
	go func() {
		result <- supervisor.Run(context.Background(), func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			err := context.Cause(ctx)
			exited <- err
			return err
		})
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("lifecycle não iniciou")
	}

	cause := errors.New("storage indisponível")
	if !barrier.Close(cause) {
		t.Fatal("Close deveria fechar a barrier")
	}

	select {
	case lifecycleErr := <-exited:
		if !errors.Is(lifecycleErr, ErrDurabilityBarrierClosed) || !errors.Is(lifecycleErr, cause) {
			t.Fatalf("context cause=%v não preserva barrier/cause", lifecycleErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("lifecycle não recebeu cancelamento da barrier")
	}

	select {
	case err := <-result:
		if !errors.Is(err, ErrDurabilityBarrierClosed) || !errors.Is(err, cause) {
			t.Fatalf("Run=%v não preserva barrier/cause", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Supervisor não encerrou após barrier")
	}
}

func TestSupervisorParentCancellationStopsLifecycleWithoutClosingBarrier(t *testing.T) {
	barrier := NewDurabilityBarrier()
	supervisor, err := NewSupervisor(barrier)
	if err != nil {
		t.Fatal(err)
	}

	parent, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- supervisor.Run(parent, func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return ctx.Err()
		})
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("lifecycle não iniciou")
	}
	cancel()

	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run=%v, esperado context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Supervisor não encerrou após cancelamento do parent")
	}
	if err := barrier.Err(); err != nil {
		t.Fatalf("cancelamento do parent fechou barrier: %v", err)
	}
}

func TestSupervisorObservesGuardFailureAsTerminalCause(t *testing.T) {
	barrier := NewDurabilityBarrier()
	supervisor, err := NewSupervisor(barrier)
	if err != nil {
		t.Fatal(err)
	}

	started := make(chan struct{})
	result := make(chan error, 1)
	go func() {
		result <- supervisor.Run(context.Background(), func(ctx context.Context) error {
			close(started)
			<-ctx.Done()
			return context.Cause(ctx)
		})
	}()

	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("lifecycle não iniciou")
	}

	writeErr := errors.New("state write falhou")
	guardErr := barrier.Guard(func() error { return writeErr })
	if !errors.Is(guardErr, ErrDurabilityBarrierClosed) || !errors.Is(guardErr, writeErr) {
		t.Fatalf("Guard=%v não preserva fechamento/cause", guardErr)
	}

	select {
	case err := <-result:
		if !errors.Is(err, ErrDurabilityBarrierClosed) || !errors.Is(err, writeErr) {
			t.Fatalf("Supervisor=%v não preserva state write failure", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Supervisor não interrompeu lifecycle após Guard failure")
	}
}

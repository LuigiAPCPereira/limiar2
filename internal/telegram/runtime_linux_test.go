//go:build linux

package telegram

import (
	"context"
	"errors"
	"testing"
	"time"
)

type memorySessionStorage struct{}

func (memorySessionStorage) LoadSession(context.Context) ([]byte, error) { return nil, nil }
func (memorySessionStorage) StoreSession(context.Context, []byte) error  { return nil }

func TestNewRuntimeValidatesConfig(t *testing.T) {
	good := RuntimeConfig{Identity: AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, AppID: 1, AppHash: "secret", SessionStorage: memorySessionStorage{}, ReadinessTimeout: time.Second}
	tests := []struct {
		name   string
		mutate func(*RuntimeConfig)
	}{
		{"identity key", func(c *RuntimeConfig) { c.Identity.Key = " " }},
		{"self id", func(c *RuntimeConfig) { c.Identity.ExpectedSelfUserID = 0 }},
		{"app id", func(c *RuntimeConfig) { c.AppID = 0 }},
		{"app hash", func(c *RuntimeConfig) { c.AppHash = "" }},
		{"storage", func(c *RuntimeConfig) { c.SessionStorage = nil }},
		{"timeout", func(c *RuntimeConfig) { c.ReadinessTimeout = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := good
			tt.mutate(&cfg)
			if _, err := NewRuntime(cfg); !errors.Is(err, ErrInvalidRuntimeConfig) {
				t.Fatalf("error=%v, want ErrInvalidRuntimeConfig", err)
			}
		})
	}
	if _, err := NewRuntime(good); err != nil {
		t.Fatalf("NewRuntime(good) error=%v", err)
	}
}

func TestRunServesOnlyAfterAuthorizedMatchingSelf(t *testing.T) {
	var served bool
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, time.Second,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{Authorized: true, SelfUserID: 42}, nil
		})
	if err := r.Run(context.Background(), func(context.Context) error { served = true; return nil }); err != nil {
		t.Fatalf("Run() error=%v", err)
	}
	if !served {
		t.Fatal("serve callback not called")
	}
}

func TestRunUnauthorizedRequiresExplicitRebootstrap(t *testing.T) {
	var served bool
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, time.Second,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(context.Context) (authorizationStatus, error) { return authorizationStatus{}, nil })
	err := r.Run(context.Background(), func(context.Context) error { served = true; return nil })
	if !errors.Is(err, ErrRebootstrapRequired) {
		t.Fatalf("Run() error=%v, want ErrRebootstrapRequired", err)
	}
	if served {
		t.Fatal("serve called for unauthorized runtime")
	}
}

func TestRunRejectsSelfMismatch(t *testing.T) {
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, time.Second,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{Authorized: true, SelfUserID: 99}, nil
		})
	if err := r.Run(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, ErrSelfMismatch) {
		t.Fatalf("Run() error=%v, want ErrSelfMismatch", err)
	}
}

func TestRunPreservesStatusError(t *testing.T) {
	sentinel := errors.New("status failed")
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, time.Second,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(context.Context) (authorizationStatus, error) { return authorizationStatus{}, sentinel })
	if err := r.Run(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, sentinel) {
		t.Fatalf("Run() error=%v, want sentinel", err)
	}
}

func TestRunReadinessTimeoutIsBounded(t *testing.T) {
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, 10*time.Millisecond,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(ctx context.Context) (authorizationStatus, error) {
			<-ctx.Done()
			return authorizationStatus{}, ctx.Err()
		})
	if err := r.Run(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error=%v, want deadline", err)
	}
}

func TestRuntimeIsOneShot(t *testing.T) {
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, time.Second,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{Authorized: true, SelfUserID: 42}, nil
		})
	if err := r.Run(context.Background(), func(context.Context) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("second Run error=%v, want ErrAlreadyStarted", err)
	}
}

func TestRunPreservesServeCancellationEvenIfEngineSwallowsIt(t *testing.T) {
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, time.Second,
		func(ctx context.Context, f func(context.Context) error) error { _ = f(ctx); return nil },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{Authorized: true, SelfUserID: 42}, nil
		})
	if err := r.Run(context.Background(), func(context.Context) error { return context.Canceled }); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error=%v, want context.Canceled", err)
	}
}

func TestRunPreservesCallerCancellationWhenEngineReturnsNil(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, time.Second,
		func(runCtx context.Context, f func(context.Context) error) error { cancel(); return nil },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{Authorized: true, SelfUserID: 42}, nil
		})
	if err := r.Run(ctx, func(context.Context) error { return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run() error=%v, want context.Canceled", err)
	}
}

func TestRunPreservesEngineFailureWhenCallbackNeverRuns(t *testing.T) {
	sentinel := errors.New("transport failed")
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, time.Second,
		func(context.Context, func(context.Context) error) error { return sentinel },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{Authorized: true, SelfUserID: 42}, nil
		})
	if err := r.Run(context.Background(), func(context.Context) error { return nil }); !errors.Is(err, sentinel) {
		t.Fatalf("Run() error=%v, want sentinel", err)
	}
}

func TestIdentityDoesNotExposeCredentials(t *testing.T) {
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, time.Second, nil, nil)
	got := r.Identity()
	if got.Key != "primary" || got.ExpectedSelfUserID != 42 {
		t.Fatalf("Identity()=%+v", got)
	}
}

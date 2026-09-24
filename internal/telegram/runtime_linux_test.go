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
	good := RuntimeConfig{Identity: AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, AppID: 1, AppHash: "secret", SessionStorage: memorySessionStorage{}, Coordinator: NewAuthorizationCoordinator(), ReadinessTimeout: time.Second, MaxConcurrentQueries: 2, MaxHistoryPageSize: 100}
	tests := []struct {
		name   string
		mutate func(*RuntimeConfig)
	}{
		{"identity key", func(c *RuntimeConfig) { c.Identity.Key = " " }},
		{"self id", func(c *RuntimeConfig) { c.Identity.ExpectedSelfUserID = 0 }},
		{"app id", func(c *RuntimeConfig) { c.AppID = 0 }},
		{"app hash", func(c *RuntimeConfig) { c.AppHash = "" }},
		{"storage", func(c *RuntimeConfig) { c.SessionStorage = nil }},
		{"coordinator", func(c *RuntimeConfig) { c.Coordinator = nil }},
		{"timeout", func(c *RuntimeConfig) { c.ReadinessTimeout = 0 }},
		{"query concurrency", func(c *RuntimeConfig) { c.MaxConcurrentQueries = 0 }},
		{"history page size", func(c *RuntimeConfig) { c.MaxHistoryPageSize = 0 }},
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
	if err := r.Run(context.Background(), func(context.Context, Capabilities) error { served = true; return nil }); err != nil {
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
	err := r.Run(context.Background(), func(context.Context, Capabilities) error { served = true; return nil })
	if !errors.Is(err, ErrRebootstrapRequired) || !errors.Is(err, ErrAuthorizationRejected) {
		t.Fatalf("Run() error=%v, want ErrRebootstrapRequired + ErrAuthorizationRejected", err)
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
	if err := r.Run(context.Background(), func(context.Context, Capabilities) error { return nil }); !errors.Is(err, ErrSelfMismatch) {
		t.Fatalf("Run() error=%v, want ErrSelfMismatch", err)
	}
}

func TestRunPreservesStatusError(t *testing.T) {
	sentinel := errors.New("status failed")
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, time.Second,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(context.Context) (authorizationStatus, error) { return authorizationStatus{}, sentinel })
	if err := r.Run(context.Background(), func(context.Context, Capabilities) error { return nil }); !errors.Is(err, sentinel) {
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
	if err := r.Run(context.Background(), func(context.Context, Capabilities) error { return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error=%v, want deadline", err)
	}
}

func TestRuntimeIsOneShot(t *testing.T) {
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, time.Second,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{Authorized: true, SelfUserID: 42}, nil
		})
	if err := r.Run(context.Background(), func(context.Context, Capabilities) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if err := r.Run(context.Background(), func(context.Context, Capabilities) error { return nil }); !errors.Is(err, ErrAlreadyStarted) {
		t.Fatalf("second Run error=%v, want ErrAlreadyStarted", err)
	}
}

func TestRunPreservesServeCancellationEvenIfEngineSwallowsIt(t *testing.T) {
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, time.Second,
		func(ctx context.Context, f func(context.Context) error) error { _ = f(ctx); return nil },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{Authorized: true, SelfUserID: 42}, nil
		})
	if err := r.Run(context.Background(), func(context.Context, Capabilities) error { return context.Canceled }); !errors.Is(err, context.Canceled) {
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
	if err := r.Run(ctx, func(context.Context, Capabilities) error { return nil }); !errors.Is(err, context.Canceled) {
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
	if err := r.Run(context.Background(), func(context.Context, Capabilities) error { return nil }); !errors.Is(err, sentinel) {
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

func TestRunPreflightStopsBeforeClientOnMissingOrIncompatibleCredential(t *testing.T) {
	var engineStarted bool
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, time.Second,
		func(ctx context.Context, f func(context.Context) error) error { engineStarted = true; return f(ctx) },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{Authorized: true, SelfUserID: 42}, nil
		})
	r.preflight = func(context.Context) error { return ErrRebootstrapRequired }
	if err := r.Run(context.Background(), func(context.Context, Capabilities) error { return nil }); !errors.Is(err, ErrRebootstrapRequired) {
		t.Fatalf("Run() error=%v, want ErrRebootstrapRequired", err)
	}
	if engineStarted {
		t.Fatal("gotd engine started before credential preflight passed")
	}
}

func TestRunPreflightPreservesStorageFailure(t *testing.T) {
	sentinel := errors.New("read session: permission denied")
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, time.Second,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{Authorized: true, SelfUserID: 42}, nil
		})
	r.preflight = func(context.Context) error { return sentinel }
	if err := r.Run(context.Background(), func(context.Context, Capabilities) error { return nil }); !errors.Is(err, sentinel) {
		t.Fatalf("Run() error=%v, want storage failure", err)
	}
}

func TestRunPreflightTimeoutIsBounded(t *testing.T) {
	r := newRuntimeForTest(AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}, 10*time.Millisecond,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{Authorized: true, SelfUserID: 42}, nil
		})
	r.preflight = func(ctx context.Context) error { <-ctx.Done(); return ctx.Err() }
	if err := r.Run(context.Background(), func(context.Context, Capabilities) error { return nil }); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Run() error=%v, want preflight deadline", err)
	}
}


type fixedSessionStorage struct {
	data []byte
	err  error
}

func (s fixedSessionStorage) LoadSession(context.Context) ([]byte, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.data, nil
}

func (fixedSessionStorage) StoreSession(context.Context, []byte) error { return nil }

func TestPreflightDistinguishesPhysicalAbsenceFromIncompatibleBlob(t *testing.T) {
	t.Parallel()

	if err := preflightPersistedSession(context.Background(), fixedSessionStorage{}); !errors.Is(err, ErrRebootstrapRequired) || !errors.Is(err, ErrSessionAbsent) {
		t.Fatalf("absent session error=%v, want ErrRebootstrapRequired + ErrSessionAbsent", err)
	}

	incompatible := []byte(`{"Version":2,"Data":{}}`)
	err := preflightPersistedSession(context.Background(), fixedSessionStorage{data: incompatible})
	if !errors.Is(err, ErrIncompatibleSession) || !errors.Is(err, ErrRebootstrapRequired) {
		t.Fatalf("incompatible session error=%v, want ErrIncompatibleSession + ErrRebootstrapRequired", err)
	}
	if errors.Is(err, ErrSessionAbsent) {
		t.Fatalf("incompatible session misclassified as physical absence: %v", err)
	}
}

func TestPreflightPreservesMalformedAndStorageFailures(t *testing.T) {
	t.Parallel()

	malformed := []byte("{not-json")
	err := preflightPersistedSession(context.Background(), fixedSessionStorage{data: malformed})
	if err == nil {
		t.Fatal("malformed session error=nil, want decode failure")
	}
	if errors.Is(err, ErrRebootstrapRequired) || errors.Is(err, ErrIncompatibleSession) {
		t.Fatalf("malformed session misclassified: %v", err)
	}

	sentinel := errors.New("permission denied")
	err = preflightPersistedSession(context.Background(), fixedSessionStorage{err: sentinel})
	if !errors.Is(err, sentinel) {
		t.Fatalf("storage failure error=%v, want sentinel", err)
	}
}

func TestPreflightAcceptsCurrentGotdSessionEncoding(t *testing.T) {
	t.Parallel()

	current := []byte(`{"Version":1,"Data":{}}`)
	if err := preflightPersistedSession(context.Background(), fixedSessionStorage{data: current}); err != nil {
		t.Fatalf("current session preflight error=%v", err)
	}
}


type testQuery struct{}

func (testQuery) ResolvePeer(context.Context, PeerRef) (PeerDescriptor, error) {
	return PeerDescriptor{}, nil
}

func (testQuery) History(context.Context, PeerKey, HistoryRequest) (MessagePage, error) {
	return MessagePage{}, nil
}

func newRuntimeForTest(identity AuthorizationIdentity, readinessTimeout time.Duration, run runFunc, status statusFunc) *Runtime {
	return &Runtime{
		identity:         identity,
		coordinator:      NewAuthorizationCoordinator(),
		readinessTimeout: readinessTimeout,
		run:              run,
		status:           status,
		preflight:        func(context.Context) error { return nil },
		query:            testQuery{},
	}
}

func TestRunExposesQueryOnlyAfterSemanticReadiness(t *testing.T) {
	t.Parallel()

	var got TelegramQuery
	r := newRuntimeForTest(
		AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42},
		time.Second,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{Authorized: true, SelfUserID: 42}, nil
		},
	)
	if err := r.Run(context.Background(), func(_ context.Context, caps Capabilities) error {
		got = caps.Query
		return nil
	}); err != nil {
		t.Fatalf("Run() error=%v", err)
	}
	if got == nil {
		t.Fatal("ready capabilities did not expose TelegramQuery")
	}
}

func TestRunDoesNotExposeCapabilitiesBeforeReadiness(t *testing.T) {
	t.Parallel()

	var served bool
	r := newRuntimeForTest(
		AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42},
		time.Second,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{}, nil
		},
	)
	err := r.Run(context.Background(), func(context.Context, Capabilities) error {
		served = true
		return nil
	})
	if !errors.Is(err, ErrRebootstrapRequired) {
		t.Fatalf("Run() error=%v, want ErrRebootstrapRequired", err)
	}
	if served {
		t.Fatal("capabilities callback ran before semantic readiness")
	}
}


func TestRuntimeOwnershipRejectsConcurrentSameIdentity(t *testing.T) {
	t.Parallel()

	coordinator := NewAuthorizationCoordinator()
	identity := AuthorizationIdentity{Key: "primary", ExpectedSelfUserID: 42}
	ready := make(chan struct{})
	releaseServe := make(chan struct{})

	first := newRuntimeForTest(identity, time.Second,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{Authorized: true, SelfUserID: 42}, nil
		})
	first.coordinator = coordinator

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- first.Run(context.Background(), func(context.Context, Capabilities) error {
			close(ready)
			<-releaseServe
			return nil
		})
	}()
	<-ready

	second := newRuntimeForTest(identity, time.Second,
		func(ctx context.Context, f func(context.Context) error) error { return f(ctx) },
		func(context.Context) (authorizationStatus, error) {
			return authorizationStatus{Authorized: true, SelfUserID: 42}, nil
		})
	second.coordinator = coordinator
	if err := second.Run(context.Background(), func(context.Context, Capabilities) error { return nil }); !errors.Is(err, ErrAuthorizationInUse) {
		t.Fatalf("second runtime error=%v, want ErrAuthorizationInUse", err)
	}

	close(releaseServe)
	if err := <-firstDone; err != nil {
		t.Fatalf("first runtime error=%v", err)
	}
}

func TestRuntimeOwnershipAllowsDifferentIdentities(t *testing.T) {
	t.Parallel()

	coordinator := NewAuthorizationCoordinator()
	releasePrimary, err := coordinator.acquire("primary")
	if err != nil {
		t.Fatal(err)
	}
	defer releasePrimary()

	releaseResearch, err := coordinator.acquire("research")
	if err != nil {
		t.Fatalf("different identity acquire error=%v", err)
	}
	releaseResearch()
}

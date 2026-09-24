//go:build linux

// Package telegram owns the Limiar Telegram authorization runtime boundary.
package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	gotdsession "github.com/gotd/td/session"
	gotdtelegram "github.com/gotd/td/telegram"
)

var (
	ErrInvalidRuntimeConfig = errors.New("telegram runtime: invalid config")
	ErrAlreadyStarted       = errors.New("telegram runtime: already started")
	ErrRebootstrapRequired  = errors.New("telegram runtime: rebootstrap required")
	ErrIncompatibleSession = errors.New("telegram runtime: incompatible persisted session")
	ErrSelfMismatch         = errors.New("telegram runtime: authorization identity mismatch")
	ErrInvalidAuthStatus    = errors.New("telegram runtime: invalid authorization status")
)

// AuthorizationIdentity is the Limiar-local owner key for one persisted
// Telegram authorization. It is deliberately distinct from MTProto session_id.
type AuthorizationIdentity struct {
	Key                string
	ExpectedSelfUserID int64
}

func (i AuthorizationIdentity) validate() error {
	if strings.TrimSpace(i.Key) == "" {
		return fmt.Errorf("%w: empty authorization identity key", ErrInvalidRuntimeConfig)
	}
	if i.ExpectedSelfUserID <= 0 {
		return fmt.Errorf("%w: expected Telegram self user id must be positive", ErrInvalidRuntimeConfig)
	}
	return nil
}

// RuntimeConfig contains only the configuration needed to own a gotd client.
// AppHash and session bytes are credentials and must never be logged.
type RuntimeConfig struct {
	Identity         AuthorizationIdentity
	AppID            int
	AppHash          string
	SessionStorage       gotdtelegram.SessionStorage
	ReadinessTimeout     time.Duration
	MaxConcurrentQueries int
}

func (c RuntimeConfig) validate() error {
	if err := c.Identity.validate(); err != nil {
		return err
	}
	if c.AppID <= 0 {
		return fmt.Errorf("%w: app id must be positive", ErrInvalidRuntimeConfig)
	}
	if strings.TrimSpace(c.AppHash) == "" {
		return fmt.Errorf("%w: app hash is required", ErrInvalidRuntimeConfig)
	}
	if c.SessionStorage == nil {
		return fmt.Errorf("%w: session storage is required", ErrInvalidRuntimeConfig)
	}
	if c.ReadinessTimeout <= 0 {
		return fmt.Errorf("%w: readiness timeout must be positive", ErrInvalidRuntimeConfig)
	}
	if c.MaxConcurrentQueries <= 0 {
		return fmt.Errorf("%w: max concurrent queries must be positive", ErrInvalidRuntimeConfig)
	}
	return nil
}

type authorizationStatus struct {
	Authorized bool
	SelfUserID int64
}

type runFunc func(context.Context, func(context.Context) error) error
type statusFunc func(context.Context) (authorizationStatus, error)
type preflightFunc func(context.Context) error

// Capabilities are exposed only after semantic readiness succeeds.
type Capabilities struct {
	Query TelegramQuery
}

// Runtime owns exactly one main gotd telegram.Client for one authorization
// identity. The client is intentionally not exposed to consumers.
type Runtime struct {
	identity         AuthorizationIdentity
	client           *gotdtelegram.Client
	readinessTimeout time.Duration
	run              runFunc
	status           statusFunc
	preflight        preflightFunc
	query            TelegramQuery
	started          atomic.Bool
}

// NewRuntime constructs an unstarted Telegram runtime. It never performs login.
func NewRuntime(cfg RuntimeConfig) (*Runtime, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	client := gotdtelegram.NewClient(cfg.AppID, cfg.AppHash, gotdtelegram.Options{
		SessionStorage: cfg.SessionStorage,
		NoUpdates:      true,
	})
	queryClient, err := newQueryClient(client.API(), cfg.MaxConcurrentQueries)
	if err != nil {
		return nil, fmt.Errorf("telegram runtime: construct query capability: %w", err)
	}

	return &Runtime{
		identity:         cfg.Identity,
		client:           client,
		readinessTimeout: cfg.ReadinessTimeout,
		run:              client.Run,
		preflight: func(ctx context.Context) error {
			return preflightPersistedSession(ctx, cfg.SessionStorage)
		},
		query: queryClient,
		status: func(ctx context.Context) (authorizationStatus, error) {
			status, err := client.Auth().Status(ctx)
			if err != nil {
				return authorizationStatus{}, classifyTelegramError("auth_status", err)
			}
			if status == nil {
				return authorizationStatus{}, ErrInvalidAuthStatus
			}
			if !status.Authorized {
				return authorizationStatus{}, nil
			}
			if status.User == nil || status.User.ID <= 0 {
				return authorizationStatus{}, ErrInvalidAuthStatus
			}
			return authorizationStatus{Authorized: true, SelfUserID: status.User.ID}, nil
		},
	}, nil
}

// Identity returns the non-secret Limiar authorization identity binding.

func preflightPersistedSession(ctx context.Context, storage gotdtelegram.SessionStorage) error {
	raw, err := storage.LoadSession(ctx)
	if err != nil {
		return fmt.Errorf("telegram runtime: load persisted authorization: %w", err)
	}
	if len(raw) == 0 {
		return ErrRebootstrapRequired
	}

	loader := gotdsession.Loader{Storage: staticSessionStorage{data: raw}}
	if _, err := loader.Load(ctx); err != nil {
		if errors.Is(err, gotdsession.ErrNotFound) {
			// At this point physical storage was present and non-empty, so gotd's
			// ErrNotFound can only represent an incompatible serialized version.
			return fmt.Errorf("%w: %v", ErrIncompatibleSession, err)
		}
		return fmt.Errorf("telegram runtime: decode persisted authorization: %w", err)
	}
	return nil
}

type staticSessionStorage struct {
	data []byte
}

func (s staticSessionStorage) LoadSession(context.Context) ([]byte, error) {
	return s.data, nil
}

func (staticSessionStorage) StoreSession(context.Context, []byte) error {
	return errors.New("telegram runtime: read-only session validation storage")
}

func (r *Runtime) Identity() AuthorizationIdentity {
	if r == nil {
		return AuthorizationIdentity{}
	}
	return r.identity
}

// Run starts the owned gotd client, verifies semantic readiness, then runs
// serve while the client lifecycle is active. Returning from serve stops the
// gotd client. A Runtime is intentionally one-shot; construct a new one after
// shutdown/restart.
func (r *Runtime) Run(ctx context.Context, serve func(context.Context, Capabilities) error) error {
	if r == nil || r.run == nil || r.status == nil || r.preflight == nil || r.query == nil || r.readinessTimeout <= 0 {
		return fmt.Errorf("%w: invalid runtime", ErrInvalidRuntimeConfig)
	}
	if serve == nil {
		return fmt.Errorf("%w: nil serve callback", ErrInvalidRuntimeConfig)
	}
	if !r.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}

	preflightCtx, cancel := context.WithTimeout(ctx, r.readinessTimeout)
	preflightErr := r.preflight(preflightCtx)
	cancel()
	if preflightErr != nil {
		return preflightErr
	}

	callbackResult := make(chan error, 1)
	runErr := r.run(ctx, func(runCtx context.Context) error {
		err := r.runReady(runCtx, serve)
		callbackResult <- err
		return err
	})

	select {
	case callbackErr := <-callbackResult:
		if callbackErr != nil {
			return callbackErr
		}
	default:
	}
	if runErr != nil {
		return fmt.Errorf("telegram runtime: gotd client run: %w", runErr)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) runReady(ctx context.Context, serve func(context.Context, Capabilities) error) error {
	verifyCtx, cancel := context.WithTimeout(ctx, r.readinessTimeout)
	defer cancel()

	status, err := r.status(verifyCtx)
	if err != nil {
		return err
	}
	if !status.Authorized {
		return ErrRebootstrapRequired
	}
	if status.SelfUserID <= 0 {
		return ErrInvalidAuthStatus
	}
	if status.SelfUserID != r.identity.ExpectedSelfUserID {
		return fmt.Errorf("%w: expected user id %d, got %d", ErrSelfMismatch, r.identity.ExpectedSelfUserID, status.SelfUserID)
	}
	if err := serve(ctx, Capabilities{Query: r.query}); err != nil {
		return fmt.Errorf("telegram runtime: serve ready runtime: %w", err)
	}
	return nil
}

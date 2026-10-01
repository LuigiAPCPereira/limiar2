// Package telegram contém o boundary de runtime da autorização Telegram do Limiar.
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
	ErrInvalidRuntimeConfig = errors.New("runtime do Telegram: configuração inválida")
	ErrAlreadyStarted       = errors.New("runtime do Telegram: já iniciado")
	ErrRebootstrapRequired  = errors.New("runtime do Telegram: novo bootstrap obrigatório")
	ErrSessionAbsent        = errors.New("runtime do Telegram: sessão persistida ausente")
	ErrAuthorizationRejected = errors.New("runtime do Telegram: autorização persistida rejeitada")
	ErrIncompatibleSession  = errors.New("runtime do Telegram: sessão persistida incompatível")
	ErrSelfMismatch         = errors.New("runtime do Telegram: identidade de autorização divergente")
	ErrInvalidAuthStatus    = errors.New("runtime do Telegram: status de autorização inválido")
)

// AuthorizationIdentity é a chave local do Limiar que identifica o owner de uma autorização
// Telegram persistida. Ela é deliberadamente distinta do session_id do MTProto.
type AuthorizationIdentity struct {
	Key                string
	ExpectedSelfUserID int64
}

func (i AuthorizationIdentity) validate() error {
	if strings.TrimSpace(i.Key) == "" {
		return fmt.Errorf("%w: chave da identidade de autorização vazia", ErrInvalidRuntimeConfig)
	}
	if i.ExpectedSelfUserID <= 0 {
		return fmt.Errorf("%w: ID esperado do próprio usuário Telegram deve ser positivo", ErrInvalidRuntimeConfig)
	}
	return nil
}

// RuntimeConfig contém somente a configuração necessária para possuir um client gotd.
// AppHash e bytes da sessão são credenciais e nunca devem ser registrados em log.
type RuntimeConfig struct {
	Identity         AuthorizationIdentity
	AppID            int
	AppHash          string
	SessionStorage        gotdtelegram.SessionStorage
	Coordinator           *AuthorizationCoordinator
	ReadinessTimeout      time.Duration
	MaxConcurrentQueries int
	MaxHistoryPageSize   int
	MaxResolvedPeers     int
	Observer             Observer
}

func (c RuntimeConfig) validate() error {
	if err := c.Identity.validate(); err != nil {
		return err
	}
	if c.AppID <= 0 {
		return fmt.Errorf("%w: app ID deve ser positivo", ErrInvalidRuntimeConfig)
	}
	if strings.TrimSpace(c.AppHash) == "" {
		return fmt.Errorf("%w: app hash é obrigatório", ErrInvalidRuntimeConfig)
	}
	if c.SessionStorage == nil {
		return fmt.Errorf("%w: armazenamento de sessão é obrigatório", ErrInvalidRuntimeConfig)
	}
	if c.Coordinator == nil {
		return fmt.Errorf("%w: coordenador de autorização é obrigatório", ErrInvalidRuntimeConfig)
	}
	if c.ReadinessTimeout <= 0 {
		return fmt.Errorf("%w: timeout de prontidão deve ser positivo", ErrInvalidRuntimeConfig)
	}
	if c.MaxConcurrentQueries <= 0 {
		return fmt.Errorf("%w: máximo de consultas concorrentes deve ser positivo", ErrInvalidRuntimeConfig)
	}
	if c.MaxHistoryPageSize <= 0 || c.MaxHistoryPageSize > MaxHistoryPageSize {
		return fmt.Errorf("%w: tamanho máximo da página de histórico deve ficar entre 1 e %d", ErrInvalidRuntimeConfig, MaxHistoryPageSize)
	}
	if c.MaxResolvedPeers <= 0 {
		return fmt.Errorf("%w: máximo de peers resolvidos deve ser positivo", ErrInvalidRuntimeConfig)
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

// Capabilities são expostas somente após a prontidão semântica ser confirmada.
type Capabilities struct {
	Query TelegramQuery
}

// Runtime possui exatamente um telegram.Client principal do gotd para uma identidade de autorização.
// O client não é exposto aos consumidores intencionalmente.
type Runtime struct {
	identity         AuthorizationIdentity
	coordinator      *AuthorizationCoordinator
	client           *gotdtelegram.Client
	readinessTimeout time.Duration
	run              runFunc
	status           statusFunc
	preflight        preflightFunc
	query            TelegramQuery
	observer         Observer
	started          atomic.Bool
}

// NewRuntime constrói um runtime Telegram ainda não iniciado. Ele nunca executa login.
func NewRuntime(cfg RuntimeConfig) (*Runtime, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	sessionStorage := observeSessionStorage(cfg.SessionStorage, cfg.Identity.Key, cfg.Observer)
	client := gotdtelegram.NewClient(cfg.AppID, cfg.AppHash, gotdtelegram.Options{
		SessionStorage: sessionStorage,
		NoUpdates:      true,
	})
	queryClient, err := newQueryClient(client.API(), cfg.MaxConcurrentQueries, cfg.MaxHistoryPageSize, cfg.MaxResolvedPeers)
	if err != nil {
		return nil, fmt.Errorf("runtime do Telegram: construir capability de consulta: %w", err)
	}
	queryClient.observer = cfg.Observer
	queryClient.identityKey = cfg.Identity.Key

	return &Runtime{
		identity:         cfg.Identity,
		coordinator:      cfg.Coordinator,
		client:           client,
		readinessTimeout: cfg.ReadinessTimeout,
		run:              client.Run,
		preflight: func(ctx context.Context) error {
			return preflightPersistedSession(ctx, sessionStorage)
		},
		query:    queryClient,
		observer: cfg.Observer,
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

// Identity retorna o vínculo não secreto da identidade de autorização do Limiar.

func preflightPersistedSession(ctx context.Context, storage gotdtelegram.SessionStorage) error {
	raw, err := storage.LoadSession(ctx)
	if err != nil {
		return fmt.Errorf("runtime do Telegram: carregar autorização persistida: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(raw) == 0 {
		return newRebootstrapError(ErrSessionAbsent)
	}

	loader := gotdsession.Loader{Storage: staticSessionStorage{data: raw}}
	if _, err := loader.Load(ctx); err != nil {
		if errors.Is(err, gotdsession.ErrNotFound) {
			// Neste ponto o armazenamento físico estava presente e não vazio; portanto,
			// ErrNotFound do gotd só pode representar uma versão serializada incompatível.
			return newRebootstrapError(fmt.Errorf("%w: %v", ErrIncompatibleSession, err))
		}
		return fmt.Errorf("runtime do Telegram: decodificar autorização persistida: %w", err)
	}
	return nil
}

type rebootstrapError struct {
	reason error
}

func (e *rebootstrapError) Error() string {
	if e == nil || e.reason == nil {
		return ErrRebootstrapRequired.Error()
	}
	return fmt.Sprintf("%s: %s", ErrRebootstrapRequired, e.reason)
}

func (e *rebootstrapError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.reason
}

func (e *rebootstrapError) Is(target error) bool {
	if target == ErrRebootstrapRequired {
		return true
	}
	return e != nil && errors.Is(e.reason, target)
}

func newRebootstrapError(reason error) error {
	return &rebootstrapError{reason: reason}
}

type staticSessionStorage struct {
	data []byte
}

func (s staticSessionStorage) LoadSession(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return append([]byte(nil), s.data...), nil
}

func (staticSessionStorage) StoreSession(context.Context, []byte) error {
	return errors.New("runtime do Telegram: armazenamento de validação de sessão somente leitura")
}

func (r *Runtime) Identity() AuthorizationIdentity {
	if r == nil {
		return AuthorizationIdentity{}
	}
	return r.identity
}

// Run inicia o client gotd de propriedade do runtime, verifica a prontidão semântica e executa
// serve enquanto o lifecycle do client estiver ativo. O retorno de serve encerra o client gotd.
// Runtime é intencionalmente de uso único; construa outro após shutdown/restart.
func (r *Runtime) Run(ctx context.Context, serve func(context.Context, Capabilities) error) (retErr error) {
	if r == nil || r.run == nil || r.status == nil || r.preflight == nil || r.query == nil || r.coordinator == nil || r.readinessTimeout <= 0 {
		return fmt.Errorf("%w: runtime inválido", ErrInvalidRuntimeConfig)
	}
	if serve == nil {
		return fmt.Errorf("%w: callback serve ausente", ErrInvalidRuntimeConfig)
	}
	release, err := r.coordinator.acquire(r.identity.Key)
	if err != nil {
		return err
	}
	defer release()
	if !r.started.CompareAndSwap(false, true) {
		return ErrAlreadyStarted
	}

	startedAt := time.Now()
	observe(r.observer, Event{
		Type:        EventTypeRuntimeState,
		IdentityKey: r.identity.Key,
		State:       RuntimeStateStarting,
	})
	defer func() {
		state := RuntimeStateStopped
		if retErr != nil {
			state = RuntimeStateFailed
		}
		outcome, kind, retryAfter := eventOutcome(retErr)
		observe(r.observer, Event{
			Type:        EventTypeRuntimeState,
			IdentityKey: r.identity.Key,
			State:       state,
			Outcome:     outcome,
			ErrorKind:   kind,
			Duration:    time.Since(startedAt),
			RetryAfter:  retryAfter,
		})
	}()

	preflightCtx, cancel := context.WithTimeout(ctx, r.readinessTimeout)
	preflightErr := r.preflight(preflightCtx)
	cancel()
	if preflightErr != nil {
		return preflightErr
	}

	callbackResult := make(chan error, 1)
	runErr := r.run(ctx, func(runCtx context.Context) error {
		err := r.runReady(runCtx, startedAt, serve)
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
		return fmt.Errorf("runtime do Telegram: execução do client gotd: %w", runErr)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (r *Runtime) runReady(ctx context.Context, startedAt time.Time, serve func(context.Context, Capabilities) error) error {
	verifyCtx, cancel := context.WithTimeout(ctx, r.readinessTimeout)
	defer cancel()

	status, err := r.status(verifyCtx)
	if err != nil {
		return err
	}
	if !status.Authorized {
		return newRebootstrapError(ErrAuthorizationRejected)
	}
	if status.SelfUserID <= 0 {
		return ErrInvalidAuthStatus
	}
	if status.SelfUserID != r.identity.ExpectedSelfUserID {
		return fmt.Errorf("%w: ID de usuário esperado %d; obtido %d", ErrSelfMismatch, r.identity.ExpectedSelfUserID, status.SelfUserID)
	}
	observe(r.observer, Event{
		Type:        EventTypeRuntimeState,
		IdentityKey: r.identity.Key,
		State:       RuntimeStateReady,
		Outcome:     EventOutcomeOK,
		Duration:    time.Since(startedAt),
	})
	if err := serve(ctx, Capabilities{Query: r.query}); err != nil {
		return fmt.Errorf("runtime do Telegram: executar runtime pronto: %w", err)
	}
	return nil
}

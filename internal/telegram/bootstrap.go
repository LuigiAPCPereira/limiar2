package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	gotdtelegram "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/telegram/auth/qrlogin"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"
)

var (
	ErrInvalidBootstrapConfig      = errors.New("bootstrap do Telegram: configuração inválida")
	ErrBootstrapAlreadyProvisioned = errors.New("bootstrap do Telegram: credencial já provisionada")
	ErrBootstrapTwoFARequired      = errors.New("bootstrap do Telegram: provider do hash da senha 2FA é obrigatório")
	ErrBootstrapSignUpRequired     = errors.New("bootstrap do Telegram: cadastro de nova conta não é permitido")
	ErrBootstrapUnauthorized       = errors.New("bootstrap do Telegram: autorização não foi concluída")
	ErrBootstrapSessionMissing     = errors.New("bootstrap do Telegram: snapshot da sessão autenticada ausente")
	ErrBootstrapAlreadyStarted     = errors.New("bootstrap do Telegram: já iniciado")
)

// BootstrapConfig configura uma operação administrativa explícita de autorização.
// ExpectedSelfUserID pode ser zero no primeiro bootstrap; o SelfUserID retornado
// deve então ser persistido como vínculo futuro do runtime.
type BootstrapConfig struct {
	IdentityKey        string
	ExpectedSelfUserID int64
	AppID              int
	AppHash            string
	SessionStorage     gotdtelegram.SessionStorage
	Coordinator        *AuthorizationCoordinator
	ReplaceExisting    bool
	CommitTimeout      time.Duration
	Observer           Observer
}

func (c BootstrapConfig) validate() error {
	if strings.TrimSpace(c.IdentityKey) == "" {
		return fmt.Errorf("%w: chave da identidade de autorização vazia", ErrInvalidBootstrapConfig)
	}
	if c.ExpectedSelfUserID < 0 {
		return fmt.Errorf("%w: ID esperado do próprio usuário não pode ser negativo", ErrInvalidBootstrapConfig)
	}
	if c.ReplaceExisting && c.ExpectedSelfUserID <= 0 {
		return fmt.Errorf("%w: substituir uma credencial existente exige o ID esperado do próprio usuário", ErrInvalidBootstrapConfig)
	}
	if c.AppID <= 0 {
		return fmt.Errorf("%w: app ID deve ser positivo", ErrInvalidBootstrapConfig)
	}
	if strings.TrimSpace(c.AppHash) == "" {
		return fmt.Errorf("%w: app hash é obrigatório", ErrInvalidBootstrapConfig)
	}
	if c.SessionStorage == nil {
		return fmt.Errorf("%w: armazenamento de sessão é obrigatório", ErrInvalidBootstrapConfig)
	}
	if c.Coordinator == nil {
		return fmt.Errorf("%w: coordenador de autorização é obrigatório", ErrInvalidBootstrapConfig)
	}
	if c.CommitTimeout <= 0 {
		return fmt.Errorf("%w: timeout de commit deve ser positivo", ErrInvalidBootstrapConfig)
	}
	return nil
}

// QRChallenge contém material sensível e transitório do bootstrap. Chamadores podem exibi-lo
// ao operador, mas não devem enviar a URL para logs, métricas ou armazenamento durável.
type QRChallenge struct {
	URL       string
	ExpiresAt time.Time
}

type QRPresenter interface {
	PresentQR(ctx context.Context, challenge QRChallenge) error
}

type QRPresenterFunc func(context.Context, QRChallenge) error

func (f QRPresenterFunc) PresentQR(ctx context.Context, challenge QRChallenge) error {
	return f(ctx, challenge)
}

// ExistingAccountAuthenticator fornece somente credenciais de uma conta Telegram existente.
// PasswordHash é chamado apenas quando o Telegram exige 2FA e pode usar auth/srpguard,
// evitando que senha em texto puro precise entrar neste package.
type ExistingAccountAuthenticator interface {
	Phone(ctx context.Context) (string, error)
	Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error)
	PasswordHash(ctx context.Context, p *tg.AccountPassword) (*tg.InputCheckPasswordSRP, error)
}

type BootstrapResult struct {
	IdentityKey      string
	SelfUserID       int64
	ReplacedExisting bool
}

// Bootstrapper possui um client administrativo de uso único do gotd. Ele é deliberadamente
// separado de Runtime e nunca deve executar concorrentemente com o Runtime da mesma
// TelegramAuthorizationIdentity.
type Bootstrapper struct {
	cfg     BootstrapConfig
	started atomic.Bool
}

func NewBootstrapper(cfg BootstrapConfig) (*Bootstrapper, error) {
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Bootstrapper{cfg: cfg}, nil
}

func (b *Bootstrapper) QR(
	ctx context.Context,
	presenter QRPresenter,
	passwordHash auth.PasswordHashFunc,
) (BootstrapResult, error) {
	if b == nil {
		return BootstrapResult{}, fmt.Errorf("%w: bootstrapper ausente", ErrInvalidBootstrapConfig)
	}
	if presenter == nil {
		return BootstrapResult{}, fmt.Errorf("%w: apresentador de QR é obrigatório", ErrInvalidBootstrapConfig)
	}
	release, err := b.cfg.Coordinator.acquire(b.cfg.IdentityKey)
	if err != nil {
		return BootstrapResult{}, err
	}
	defer release()
	if !b.started.CompareAndSwap(false, true) {
		return BootstrapResult{}, ErrBootstrapAlreadyStarted
	}

	staging, replaced, err := b.prepare(ctx)
	if err != nil {
		return BootstrapResult{}, err
	}

	dispatcher := tg.NewUpdateDispatcher()
	loggedIn := qrlogin.OnLoginToken(&dispatcher)
	client := gotdtelegram.NewClient(b.cfg.AppID, b.cfg.AppHash, gotdtelegram.Options{
		SessionStorage: staging,
		UpdateHandler:  dispatcher,
	})

	var result BootstrapResult
	runErr := client.Run(ctx, func(runCtx context.Context) error {
		status, err := client.Auth().Status(runCtx)
		if err != nil {
			return classifyTelegramError("bootstrap_auth_status", err)
		}
		if status.Authorized {
			return ErrBootstrapAlreadyProvisioned
		}

		_, err = client.QR().Auth(runCtx, loggedIn, func(showCtx context.Context, token qrlogin.Token) error {
			return presenter.PresentQR(showCtx, QRChallenge{
				URL:       token.URL(),
				ExpiresAt: token.Expires(),
			})
		})
		if err != nil {
			if tgerr.Is(err, "SESSION_PASSWORD_NEEDED") {
				if passwordHash == nil {
					return ErrBootstrapTwoFARequired
				}
				if _, passwordErr := client.Auth().PasswordWith(runCtx, passwordHash); passwordErr != nil {
					return classifyTelegramError("bootstrap_2fa", passwordErr)
				}
			} else {
				return classifyTelegramError("bootstrap_qr", err)
			}
		}

		selfID, err := b.verifySelf(runCtx, client)
		if err != nil {
			return err
		}
		result = BootstrapResult{
			IdentityKey:      b.cfg.IdentityKey,
			SelfUserID:       selfID,
			ReplacedExisting: replaced,
		}
		return nil
	})
	if runErr != nil {
		return BootstrapResult{}, runErr
	}
	if err := b.finalize(ctx, result, staging); err != nil {
		return BootstrapResult{}, err
	}
	return result, nil
}

func (b *Bootstrapper) Code(
	ctx context.Context,
	input ExistingAccountAuthenticator,
) (BootstrapResult, error) {
	if b == nil {
		return BootstrapResult{}, fmt.Errorf("%w: bootstrapper ausente", ErrInvalidBootstrapConfig)
	}
	if input == nil {
		return BootstrapResult{}, fmt.Errorf("%w: autenticador por código é obrigatório", ErrInvalidBootstrapConfig)
	}
	release, err := b.cfg.Coordinator.acquire(b.cfg.IdentityKey)
	if err != nil {
		return BootstrapResult{}, err
	}
	defer release()
	if !b.started.CompareAndSwap(false, true) {
		return BootstrapResult{}, ErrBootstrapAlreadyStarted
	}

	staging, replaced, err := b.prepare(ctx)
	if err != nil {
		return BootstrapResult{}, err
	}

	client := gotdtelegram.NewClient(b.cfg.AppID, b.cfg.AppHash, gotdtelegram.Options{
		SessionStorage: staging,
		NoUpdates:      true,
	})
	authenticator := existingAccountAuthAdapter{input: input}

	var result BootstrapResult
	runErr := client.Run(ctx, func(runCtx context.Context) error {
		status, err := client.Auth().Status(runCtx)
		if err != nil {
			return classifyTelegramError("bootstrap_auth_status", err)
		}
		if status.Authorized {
			return ErrBootstrapAlreadyProvisioned
		}

		flow := auth.NewFlow(authenticator, auth.SendCodeOptions{})
		if err := flow.Run(runCtx, client.Auth()); err != nil {
			if errors.Is(err, ErrBootstrapSignUpRequired) {
				return err
			}
			return classifyTelegramError("bootstrap_code", err)
		}

		selfID, err := b.verifySelf(runCtx, client)
		if err != nil {
			return err
		}
		result = BootstrapResult{
			IdentityKey:      b.cfg.IdentityKey,
			SelfUserID:       selfID,
			ReplacedExisting: replaced,
		}
		return nil
	})
	if runErr != nil {
		return BootstrapResult{}, runErr
	}
	if err := b.finalize(ctx, result, staging); err != nil {
		return BootstrapResult{}, err
	}
	return result, nil
}

func (b *Bootstrapper) prepare(ctx context.Context) (*stagingSessionStorage, bool, error) {
	raw, err := b.storage().LoadSession(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("bootstrap do Telegram: inspecionar credencial existente: %w", err)
	}
	exists := len(raw) > 0
	if exists && !b.cfg.ReplaceExisting {
		return nil, false, ErrBootstrapAlreadyProvisioned
	}
	return &stagingSessionStorage{}, exists, nil
}

func (b *Bootstrapper) verifySelf(ctx context.Context, client *gotdtelegram.Client) (int64, error) {
	status, err := client.Auth().Status(ctx)
	if err != nil {
		return 0, classifyTelegramError("bootstrap_verify_self", err)
	}
	if status == nil || !status.Authorized || status.User == nil || status.User.ID <= 0 {
		return 0, ErrBootstrapUnauthorized
	}
	if b.cfg.ExpectedSelfUserID > 0 && status.User.ID != b.cfg.ExpectedSelfUserID {
		return 0, fmt.Errorf("%w: ID de usuário esperado %d; obtido %d", ErrSelfMismatch, b.cfg.ExpectedSelfUserID, status.User.ID)
	}
	return status.User.ID, nil
}

func (b *Bootstrapper) finalize(ctx context.Context, result BootstrapResult, staging *stagingSessionStorage) error {
	if strings.TrimSpace(result.IdentityKey) == "" || result.SelfUserID <= 0 {
		return ErrBootstrapUnauthorized
	}
	return b.commit(ctx, staging)
}

func (b *Bootstrapper) commit(ctx context.Context, staging *stagingSessionStorage) error {
	data := staging.snapshot()
	if len(data) == 0 {
		return ErrBootstrapSessionMissing
	}

	commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), b.cfg.CommitTimeout)
	defer cancel()
	if err := b.storage().StoreSession(commitCtx, data); err != nil {
		return fmt.Errorf("bootstrap do Telegram: persistir sessão autenticada: %w", err)
	}
	return nil
}

func (b *Bootstrapper) storage() gotdtelegram.SessionStorage {
	return observeSessionStorage(b.cfg.SessionStorage, b.cfg.IdentityKey, b.cfg.Observer)
}

type existingAccountAuthAdapter struct {
	input ExistingAccountAuthenticator
}

func (a existingAccountAuthAdapter) Phone(ctx context.Context) (string, error) {
	return a.input.Phone(ctx)
}

func (a existingAccountAuthAdapter) Code(ctx context.Context, sentCode *tg.AuthSentCode) (string, error) {
	return a.input.Code(ctx, sentCode)
}

func (a existingAccountAuthAdapter) Password(context.Context) (string, error) {
	return "", errors.New("bootstrap do Telegram: caminho de senha 2FA em texto puro desabilitado")
}

func (a existingAccountAuthAdapter) PasswordHash(ctx context.Context, p *tg.AccountPassword) (*tg.InputCheckPasswordSRP, error) {
	return a.input.PasswordHash(ctx, p)
}

func (existingAccountAuthAdapter) AcceptTermsOfService(context.Context, tg.HelpTermsOfService) error {
	return ErrBootstrapSignUpRequired
}

func (existingAccountAuthAdapter) SignUp(context.Context) (auth.UserInfo, error) {
	return auth.UserInfo{}, ErrBootstrapSignUpRequired
}

var (
	_ auth.UserAuthenticator   = existingAccountAuthAdapter{}
	_ auth.PasswordHashProvider = existingAccountAuthAdapter{}
)

type stagingSessionStorage struct {
	mu   sync.RWMutex
	data []byte
}

func (s *stagingSessionStorage) LoadSession(context.Context) ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]byte(nil), s.data...), nil
}

func (s *stagingSessionStorage) StoreSession(_ context.Context, data []byte) error {
	if len(data) == 0 {
		return ErrBootstrapSessionMissing
	}
	s.mu.Lock()
	s.data = append(s.data[:0], data...)
	s.mu.Unlock()
	return nil
}

func (s *stagingSessionStorage) snapshot() []byte {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]byte(nil), s.data...)
}

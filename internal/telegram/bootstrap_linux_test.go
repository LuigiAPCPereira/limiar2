//go:build linux

package telegram

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

type memoryBootstrapStorage struct {
	mu       sync.Mutex
	data     []byte
	loadErr  error
	storeErr error
	stores   int
}

func (s *memoryBootstrapStorage) LoadSession(context.Context) ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return nil, s.loadErr
	}
	return append([]byte(nil), s.data...), nil
}

func (s *memoryBootstrapStorage) StoreSession(context.Context, []byte) error {
	panic("StoreSession test implementation must use store")
}

func (s *memoryBootstrapStorage) store(_ context.Context, data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.storeErr != nil {
		return s.storeErr
	}
	s.stores++
	s.data = append(s.data[:0], data...)
	return nil
}

type bootstrapStorageAdapter struct{ *memoryBootstrapStorage }

func (s bootstrapStorageAdapter) StoreSession(ctx context.Context, data []byte) error {
	return s.store(ctx, data)
}

func goodBootstrapConfig(storage gotdSessionStorageForTest) BootstrapConfig {
	return BootstrapConfig{
		IdentityKey:    "primary",
		AppID:          1,
		AppHash:        "secret",
		SessionStorage: storage,
		Coordinator:    NewAuthorizationCoordinator(),
		CommitTimeout:  time.Second,
	}
}

// gotdSessionStorageForTest mirrors the gotd session.Storage shape only to make
// test helpers accept either memory adapter or other in-package implementations.
type gotdSessionStorageForTest interface {
	LoadSession(context.Context) ([]byte, error)
	StoreSession(context.Context, []byte) error
}

func TestNewBootstrapperValidatesConfig(t *testing.T) {
	t.Parallel()

	base := bootstrapStorageAdapter{&memoryBootstrapStorage{}}
	good := goodBootstrapConfig(base)
	tests := []struct {
		name   string
		mutate func(*BootstrapConfig)
	}{
		{"identity", func(c *BootstrapConfig) { c.IdentityKey = " " }},
		{"negative self", func(c *BootstrapConfig) { c.ExpectedSelfUserID = -1 }},
		{"replacement without binding", func(c *BootstrapConfig) { c.ReplaceExisting = true; c.ExpectedSelfUserID = 0 }},
		{"app id", func(c *BootstrapConfig) { c.AppID = 0 }},
		{"app hash", func(c *BootstrapConfig) { c.AppHash = "" }},
		{"storage", func(c *BootstrapConfig) { c.SessionStorage = nil }},
		{"coordinator", func(c *BootstrapConfig) { c.Coordinator = nil }},
		{"commit timeout", func(c *BootstrapConfig) { c.CommitTimeout = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := good
			tt.mutate(&cfg)
			if _, err := NewBootstrapper(cfg); !errors.Is(err, ErrInvalidBootstrapConfig) {
				t.Fatalf("NewBootstrapper() error=%v, want ErrInvalidBootstrapConfig", err)
			}
		})
	}
	if _, err := NewBootstrapper(good); err != nil {
		t.Fatalf("NewBootstrapper(good) error=%v", err)
	}
}

func TestBootstrapperIsOneShotAcrossBootstrapModes(t *testing.T) {
	t.Parallel()

	b, err := NewBootstrapper(goodBootstrapConfig(bootstrapStorageAdapter{&memoryBootstrapStorage{}}))
	if err != nil {
		t.Fatal(err)
	}
	b.started.Store(true)

	presenter := QRPresenterFunc(func(context.Context, QRChallenge) error { return nil })
	if _, err := b.QR(context.Background(), presenter, nil); !errors.Is(err, ErrBootstrapAlreadyStarted) {
		t.Fatalf("QR() error=%v, want ErrBootstrapAlreadyStarted", err)
	}
	if _, err := b.Code(context.Background(), existingInputStub{}); !errors.Is(err, ErrBootstrapAlreadyStarted) {
		t.Fatalf("Code() error=%v, want ErrBootstrapAlreadyStarted", err)
	}
}

func TestBootstrapPrepareFreshDoesNotPersistAnything(t *testing.T) {
	t.Parallel()

	base := &memoryBootstrapStorage{}
	storage := bootstrapStorageAdapter{base}
	b, err := NewBootstrapper(goodBootstrapConfig(storage))
	if err != nil {
		t.Fatal(err)
	}

	staging, replaced, err := b.prepare(context.Background())
	if err != nil {
		t.Fatalf("prepare() error=%v", err)
	}
	if replaced {
		t.Fatal("fresh bootstrap unexpectedly marked as replacement")
	}
	if got := staging.snapshot(); len(got) != 0 {
		t.Fatalf("staging snapshot=%q, want empty", got)
	}
	if base.stores != 0 || len(base.data) != 0 {
		t.Fatalf("underlying storage changed during prepare: stores=%d data=%q", base.stores, base.data)
	}
}

func TestBootstrapExistingCredentialRequiresExplicitReplacement(t *testing.T) {
	t.Parallel()

	base := &memoryBootstrapStorage{data: []byte("old-session")}
	storage := bootstrapStorageAdapter{base}
	cfg := goodBootstrapConfig(storage)
	b, err := NewBootstrapper(cfg)
	if err != nil {
		t.Fatal(err)
	}

	if _, _, err := b.prepare(context.Background()); !errors.Is(err, ErrBootstrapAlreadyProvisioned) {
		t.Fatalf("prepare() error=%v, want ErrBootstrapAlreadyProvisioned", err)
	}
	if got := string(base.data); got != "old-session" {
		t.Fatalf("existing credential changed: %q", got)
	}
}

func TestBootstrapReplacementStagesWithoutTouchingOldCredential(t *testing.T) {
	t.Parallel()

	base := &memoryBootstrapStorage{data: []byte("old-session")}
	storage := bootstrapStorageAdapter{base}
	cfg := goodBootstrapConfig(storage)
	cfg.ReplaceExisting = true
	cfg.ExpectedSelfUserID = 42
	b, err := NewBootstrapper(cfg)
	if err != nil {
		t.Fatal(err)
	}

	staging, replaced, err := b.prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !replaced {
		t.Fatal("replacement was not recorded")
	}
	if len(staging.snapshot()) != 0 {
		t.Fatal("staging unexpectedly loaded old credential")
	}
	if got := string(base.data); got != "old-session" {
		t.Fatalf("old credential changed before authentication: %q", got)
	}
}

func TestBootstrapCommitPublishesOnlyFinalStagedSnapshot(t *testing.T) {
	t.Parallel()

	base := &memoryBootstrapStorage{data: []byte("old-session")}
	storage := bootstrapStorageAdapter{base}
	cfg := goodBootstrapConfig(storage)
	cfg.ReplaceExisting = true
	cfg.ExpectedSelfUserID = 42
	b, err := NewBootstrapper(cfg)
	if err != nil {
		t.Fatal(err)
	}
	staging, _, err := b.prepare(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	first := []byte("intermediate-auth-key")
	if err := staging.StoreSession(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	first[0] = 'X'
	if got := string(base.data); got != "old-session" {
		t.Fatalf("intermediate stage leaked to durable storage: %q", got)
	}

	final := []byte("final-session")
	if err := staging.StoreSession(context.Background(), final); err != nil {
		t.Fatal(err)
	}
	final[0] = 'X'

	if err := b.commit(context.Background(), staging); err != nil {
		t.Fatalf("commit() error=%v", err)
	}
	if got := string(base.data); got != "final-session" {
		t.Fatalf("durable data=%q, want final-session", got)
	}
	if base.stores != 1 {
		t.Fatalf("durable StoreSession calls=%d, want 1", base.stores)
	}
}

func TestBootstrapCommitUsesIndependentBoundedContext(t *testing.T) {
	t.Parallel()

	base := &memoryBootstrapStorage{}
	storage := bootstrapStorageAdapter{base}
	b, err := NewBootstrapper(goodBootstrapConfig(storage))
	if err != nil {
		t.Fatal(err)
	}
	staging := &stagingSessionStorage{}
	if err := staging.StoreSession(context.Background(), []byte("final-session")); err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := b.commit(ctx, staging); err != nil {
		t.Fatalf("commit() inherited caller cancellation: %v", err)
	}
	if got := string(base.data); got != "final-session" {
		t.Fatalf("durable data=%q", got)
	}
}

func TestBootstrapCommitFailureDoesNotMutateMemoryStorage(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("disk full")
	base := &memoryBootstrapStorage{data: []byte("old-session"), storeErr: sentinel}
	storage := bootstrapStorageAdapter{base}
	cfg := goodBootstrapConfig(storage)
	cfg.ReplaceExisting = true
	cfg.ExpectedSelfUserID = 42
	b, err := NewBootstrapper(cfg)
	if err != nil {
		t.Fatal(err)
	}
	staging := &stagingSessionStorage{}
	if err := staging.StoreSession(context.Background(), []byte("new-session")); err != nil {
		t.Fatal(err)
	}

	err = b.commit(context.Background(), staging)
	if !errors.Is(err, sentinel) {
		t.Fatalf("commit() error=%v, want sentinel", err)
	}
	if got := string(base.data); got != "old-session" {
		t.Fatalf("old credential changed after failed durable commit: %q", got)
	}
}

func TestStagingStorageCopiesInputAndOutput(t *testing.T) {
	t.Parallel()

	s := &stagingSessionStorage{}
	input := []byte("secret-session")
	if err := s.StoreSession(context.Background(), input); err != nil {
		t.Fatal(err)
	}
	input[0] = 'X'

	loaded, err := s.LoadSession(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if string(loaded) != "secret-session" {
		t.Fatalf("stored bytes aliased caller buffer: %q", loaded)
	}
	loaded[0] = 'Y'
	if got := string(s.snapshot()); got != "secret-session" {
		t.Fatalf("loaded bytes aliased internal buffer: %q", got)
	}
}

type existingInputStub struct{}

func (existingInputStub) Phone(context.Context) (string, error) { return "+10000000000", nil }
func (existingInputStub) Code(context.Context, *tg.AuthSentCode) (string, error) {
	return "12345", nil
}
func (existingInputStub) PasswordHash(context.Context, *tg.AccountPassword) (*tg.InputCheckPasswordSRP, error) {
	return &tg.InputCheckPasswordSRP{}, nil
}

func TestExistingAccountAdapterBlocksSignupAndPlaintextPassword(t *testing.T) {
	t.Parallel()

	a := existingAccountAuthAdapter{input: existingInputStub{}}
	if _, err := a.Password(context.Background()); err == nil {
		t.Fatal("Password() error=nil, plaintext path must be disabled")
	}
	if err := a.AcceptTermsOfService(context.Background(), tg.HelpTermsOfService{}); !errors.Is(err, ErrBootstrapSignUpRequired) {
		t.Fatalf("AcceptTermsOfService() error=%v", err)
	}
	if _, err := a.SignUp(context.Background()); !errors.Is(err, ErrBootstrapSignUpRequired) {
		t.Fatalf("SignUp() error=%v", err)
	}

	var _ auth.UserAuthenticator = a
	var _ auth.PasswordHashProvider = a
}

func TestBootstrapPreparePreservesStorageFailure(t *testing.T) {
	t.Parallel()

	sentinel := errors.New("permission denied")
	base := &memoryBootstrapStorage{loadErr: sentinel}
	b, err := NewBootstrapper(goodBootstrapConfig(bootstrapStorageAdapter{base}))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = b.prepare(context.Background())
	if !errors.Is(err, sentinel) {
		t.Fatalf("prepare() error=%v, want sentinel", err)
	}
}

func TestStagingSnapshotIsLatestCompleteWrite(t *testing.T) {
	t.Parallel()

	s := &stagingSessionStorage{}
	for _, value := range [][]byte{[]byte("a"), []byte("bb"), []byte("ccc")} {
		if err := s.StoreSession(context.Background(), value); err != nil {
			t.Fatal(err)
		}
	}
	if got := s.snapshot(); !bytes.Equal(got, []byte("ccc")) {
		t.Fatalf("snapshot=%q", got)
	}
}


func TestBootstrapFinalizeRequiresVerifiedSelfBeforeDurableCommit(t *testing.T) {
	t.Parallel()

	base := &memoryBootstrapStorage{data: []byte("old-session")}
	storage := bootstrapStorageAdapter{base}
	cfg := goodBootstrapConfig(storage)
	cfg.ReplaceExisting = true
	cfg.ExpectedSelfUserID = 42
	b, err := NewBootstrapper(cfg)
	if err != nil {
		t.Fatal(err)
	}
	staging := &stagingSessionStorage{}
	if err := staging.StoreSession(context.Background(), []byte("new-session")); err != nil {
		t.Fatal(err)
	}

	err = b.finalize(context.Background(), BootstrapResult{}, staging)
	if !errors.Is(err, ErrBootstrapUnauthorized) {
		t.Fatalf("finalize() error=%v, want ErrBootstrapUnauthorized", err)
	}
	if got := string(base.data); got != "old-session" {
		t.Fatalf("durable credential changed without verified self: %q", got)
	}
	if base.stores != 0 {
		t.Fatalf("StoreSession calls=%d, want 0", base.stores)
	}

	result := BootstrapResult{IdentityKey: "primary", SelfUserID: 42, ReplacedExisting: true}
	if err := b.finalize(context.Background(), result, staging); err != nil {
		t.Fatalf("finalize(valid) error=%v", err)
	}
	if got := string(base.data); got != "new-session" {
		t.Fatalf("durable credential=%q, want new-session", got)
	}
}


func TestBootstrapAndRuntimeShareExclusiveAuthorizationOwnership(t *testing.T) {
	t.Parallel()

	coordinator := NewAuthorizationCoordinator()
	release, err := coordinator.acquire("primary")
	if err != nil {
		t.Fatal(err)
	}
	defer release()

	cfg := goodBootstrapConfig(bootstrapStorageAdapter{&memoryBootstrapStorage{}})
	cfg.Coordinator = coordinator
	b, err := NewBootstrapper(cfg)
	if err != nil {
		t.Fatal(err)
	}

	presenter := QRPresenterFunc(func(context.Context, QRChallenge) error { return nil })
	if _, err := b.QR(context.Background(), presenter, nil); !errors.Is(err, ErrAuthorizationInUse) {
		t.Fatalf("QR() error=%v, want ErrAuthorizationInUse", err)
	}

	codeBootstrapper, err := NewBootstrapper(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := codeBootstrapper.Code(context.Background(), existingInputStub{}); !errors.Is(err, ErrAuthorizationInUse) {
		t.Fatalf("Code() error=%v, want ErrAuthorizationInUse", err)
	}
}

func TestBootstrapOwnershipLeaseIsReleasedOnPrepareFailure(t *testing.T) {
	t.Parallel()

	coordinator := NewAuthorizationCoordinator()
	sentinel := errors.New("read session failed")
	cfg := goodBootstrapConfig(bootstrapStorageAdapter{&memoryBootstrapStorage{loadErr: sentinel}})
	cfg.Coordinator = coordinator
	b, err := NewBootstrapper(cfg)
	if err != nil {
		t.Fatal(err)
	}
	presenter := QRPresenterFunc(func(context.Context, QRChallenge) error { return nil })

	if _, err := b.QR(context.Background(), presenter, nil); !errors.Is(err, sentinel) {
		t.Fatalf("QR() error=%v, want sentinel", err)
	}

	release, err := coordinator.acquire("primary")
	if err != nil {
		t.Fatalf("ownership remained stuck after bootstrap failure: %v", err)
	}
	release()
}

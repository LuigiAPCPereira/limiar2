//go:build linux && telegram_real

package telegram

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/limiar2/internal/telegram/sessionstore"
)

const realChildEnv = "LIMIAR_TG_REAL_CHILD"

// TestRealRuntimeRestartReuse is an opt-in integration gate.
//
// It never bootstraps or asks for credentials. The operator must provision a
// disposable/test Telegram authorization beforehand and point the environment
// at its hardened session file. The test starts two distinct subprocesses to
// prove that the same persisted authorization can be restored by a new process
// without any login flow.
//
// Required environment:
//   LIMIAR_TELEGRAM_APP_ID
//   LIMIAR_TELEGRAM_APP_HASH
//   LIMIAR_TELEGRAM_SESSION_PATH
//   LIMIAR_TELEGRAM_SELF_USER_ID
//
// Optional:
//   LIMIAR_TELEGRAM_PEER_REF  - if set, each child performs ResolvePeer+History.
//
// Run manually:
//   go test -tags=telegram_real ./internal/telegram -run '^TestRealRuntimeRestartReuse$' -count=1
func TestRealRuntimeRestartReuse(t *testing.T) {
	if os.Getenv(realChildEnv) == "1" {
		t.Skip("parent-only integration test")
	}
	cfg := loadRealIntegrationConfig(t)

	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRealRuntimeChild$", "-test.count=1")
		cmd.Env = append(os.Environ(), realChildEnv+"=1")
		output, err := cmd.CombinedOutput()
		cancel()
		if err != nil {
			t.Fatalf("real runtime subprocess %d failed: %s; child output=%s", i+1, safeIntegrationError(err), sanitizeChildOutput(output))
		}
	}

	_ = cfg // parent validates environment without logging it.
}

// TestRealRuntimeChild is invoked only by TestRealRuntimeRestartReuse.
func TestRealRuntimeChild(t *testing.T) {
	if os.Getenv(realChildEnv) != "1" {
		t.Skip("integration child is invoked by TestRealRuntimeRestartReuse")
	}
	cfg := loadRealIntegrationConfig(t)

	storeCoordinator := sessionstore.NewCoordinator()
	storage, err := storeCoordinator.File(cfg.sessionPath)
	if err != nil {
		t.Fatal(safeIntegrationError(err))
	}

	runtime, err := NewRuntime(RuntimeConfig{
		Identity: AuthorizationIdentity{
			Key:                cfg.identityKey,
			ExpectedSelfUserID: cfg.selfUserID,
		},
		AppID:                cfg.appID,
		AppHash:              cfg.appHash,
		SessionStorage:       storage,
		Coordinator:          NewAuthorizationCoordinator(),
		ReadinessTimeout:     20 * time.Second,
		MaxConcurrentQueries: 2,
		MaxHistoryPageSize:   20,
		MaxResolvedPeers:     64,
	})
	if err != nil {
		t.Fatal(safeIntegrationError(err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), 35*time.Second)
	defer cancel()

	err = runtime.Run(ctx, func(runCtx context.Context, caps Capabilities) error {
		if cfg.peerRef == "" {
			return nil
		}
		peer, err := caps.Query.ResolvePeer(runCtx, PeerRef{Value: cfg.peerRef})
		if err != nil {
			return err
		}
		_, err = caps.Query.History(runCtx, peer.Key, HistoryRequest{Limit: 1})
		return err
	})
	if err != nil {
		t.Fatal(safeIntegrationError(err))
	}
}

type realIntegrationConfig struct {
	identityKey string
	appID       int
	appHash     string
	sessionPath string
	selfUserID  int64
	peerRef     string
}

func loadRealIntegrationConfig(t *testing.T) realIntegrationConfig {
	t.Helper()

	appID := parseRequiredEnvInt(t, "LIMIAR_TELEGRAM_APP_ID")
	selfUserID := parseRequiredEnvInt64(t, "LIMIAR_TELEGRAM_SELF_USER_ID")
	appHash := requireEnv(t, "LIMIAR_TELEGRAM_APP_HASH")
	sessionPath := requireEnv(t, "LIMIAR_TELEGRAM_SESSION_PATH")

	identity := os.Getenv("LIMIAR_TELEGRAM_IDENTITY")
	if identity == "" {
		identity = "integration"
	}

	return realIntegrationConfig{
		identityKey: identity,
		appID:       appID,
		appHash:     appHash,
		sessionPath: sessionPath,
		selfUserID:  selfUserID,
		peerRef:     os.Getenv("LIMIAR_TELEGRAM_PEER_REF"),
	}
}

func requireEnv(t *testing.T, key string) string {
	t.Helper()
	value := os.Getenv(key)
	if value == "" {
		t.Skipf("%s is not configured for opt-in Telegram integration", key)
	}
	return value
}

func parseRequiredEnvInt(t *testing.T, key string) int {
	t.Helper()
	value := requireEnv(t, key)
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		t.Fatalf("%s must be a positive integer", key)
	}
	return parsed
}

func parseRequiredEnvInt64(t *testing.T, key string) int64 {
	t.Helper()
	value := requireEnv(t, key)
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		t.Fatalf("%s must be a positive integer", key)
	}
	return parsed
}

func safeIntegrationError(err error) string {
	if err == nil {
		return "ok"
	}
	switch {
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline exceeded"
	case errors.Is(err, ErrRebootstrapRequired):
		return "rebootstrap required"
	case errors.Is(err, ErrSelfMismatch):
		return "authorization identity mismatch"
	case errors.Is(err, ErrAuthorizationInUse):
		return "authorization already in use"
	}
	var opErr *OperationError
	if errors.As(err, &opErr) {
		if opErr.RetryAfter > 0 {
			return fmt.Sprintf("%s (retry after %s)", opErr.Kind, opErr.RetryAfter)
		}
		return string(opErr.Kind)
	}
	return "integration operation failed; details suppressed"
}

func sanitizeChildOutput(output []byte) string {
	if len(output) == 0 {
		return "(none)"
	}
	// The child tests intentionally never log Telegram payloads or credentials.
	// Keep output bounded anyway so an unexpected dependency message cannot flood
	// CI/operator logs.
	const max = 2048
	if len(output) > max {
		output = output[:max]
	}
	return string(output)
}

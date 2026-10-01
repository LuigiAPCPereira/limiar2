package sessionstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestFileStorageRoundTripAndRegularFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "telegram.session")
	storage := mustFileStorage(t, NewCoordinator(), path)
	want := []byte(`{"session":"opaque-secret"}`)

	if err := storage.StoreSession(context.Background(), want); err != nil {
		t.Fatalf("StoreSession() erro = %v", err)
	}

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat() erro = %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("credencial armazenada não é arquivo regular: %v", info.Mode())
	}

	got, err := storage.LoadSession(context.Background())
	if err != nil {
		t.Fatalf("LoadSession() erro = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("LoadSession() = %q; esperado %q", got, want)
	}
}

func TestLoadMissingReturnsEmptyWithoutMaskingParentFailure(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := mustFileStorage(t, NewCoordinator(), filepath.Join(dir, "missing.session"))

	got, err := storage.LoadSession(context.Background())
	if err != nil {
		t.Fatalf("LoadSession() erro = %v", err)
	}
	if got != nil {
		t.Fatalf("LoadSession() = %q; esperado nil para ausência física", got)
	}

	parentAsFile := filepath.Join(dir, "parent-file")
	if err := os.WriteFile(parentAsFile, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	bad := mustFileStorage(t, NewCoordinator(), filepath.Join(parentAsFile, "session"))
	if _, err := bad.LoadSession(context.Background()); !errors.Is(err, ErrUnsafeParent) {
		t.Fatalf("LoadSession() erro = %v; esperado ErrUnsafeParent", err)
	}
}

func TestPresentEmptyFileFailsClosed(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "empty.session")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("WriteFile() erro = %v", err)
	}
	storage := mustFileStorage(t, NewCoordinator(), path)

	_, err := storage.LoadSession(context.Background())
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("LoadSession() erro = %v; esperado ErrInvalidSession", err)
	}
}

func TestStoreRejectsEmptyPayload(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := mustFileStorage(t, NewCoordinator(), filepath.Join(dir, "session"))

	if err := storage.StoreSession(context.Background(), nil); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("StoreSession() erro = %v; esperado ErrInvalidSession", err)
	}
}

func TestUnsafeExistingTargetsFailClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, path string)
	}{
		{
			name: "symlink",
			setup: func(t *testing.T, path string) {
				t.Helper()
				target := path + ".target"
				if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Skipf("ambiente não permite criar symlink: %v", err)
				}
			},
		},
		{
			name: "diretório",
			setup: func(t *testing.T, path string) {
				t.Helper()
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "session")
			tt.setup(t, path)
			storage := mustFileStorage(t, NewCoordinator(), path)

			if _, err := storage.LoadSession(context.Background()); !errors.Is(err, ErrUnsafeTarget) {
				t.Fatalf("LoadSession() erro = %v; esperado ErrUnsafeTarget", err)
			}
			if err := storage.StoreSession(context.Background(), []byte("new")); !errors.Is(err, ErrUnsafeTarget) {
				t.Fatalf("StoreSession() erro = %v; esperado ErrUnsafeTarget", err)
			}
		})
	}
}

func TestStoreNormalizesExistingRegularFileProtectionWhereSupported(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "session")
	if err := os.WriteFile(path, []byte("old"), 0o666); err != nil {
		t.Fatal(err)
	}
	storage := mustFileStorage(t, NewCoordinator(), path)
	if err := storage.StoreSession(context.Background(), []byte("new")); err != nil {
		t.Fatalf("StoreSession() erro = %v", err)
	}
	got, err := storage.LoadSession(context.Background())
	if err != nil {
		t.Fatalf("LoadSession() erro = %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("sessão = %q; esperado new", got)
	}
}

func TestStoreFailureBeforePublishPreservesOldCredentialAndCleansTemp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		point faultPoint
		err   error
	}{
		{name: "falha ao criar", point: faultBeforeCreate, err: errors.New("falha simulada ao criar")},
		{name: "falha ao escrever", point: faultBeforeWrite, err: errors.New("falha simulada ao escrever")},
		{name: "falha no sync", point: faultBeforeFileSync, err: errors.New("falha simulada no sync")},
		{name: "falha no rename", point: faultBeforeRename, err: errors.New("falha simulada no rename")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "session")
			coordinator := NewCoordinator()
			base := mustFileStorage(t, coordinator, path)
			old := []byte("old-session")
			if err := base.StoreSession(context.Background(), old); err != nil {
				t.Fatalf("StoreSession() inicial erro = %v", err)
			}

			faulty, err := newFileStorageWithFaults(coordinator, path, func(point faultPoint) error {
				if point == tt.point {
					return tt.err
				}
				return nil
			})
			if err != nil {
				t.Fatalf("newFileStorageWithFaults() erro = %v", err)
			}
			if err := faulty.StoreSession(context.Background(), []byte("new-session")); err == nil {
				t.Fatal("StoreSession() erro = nil; esperada falha injetada")
			}

			got, err := base.LoadSession(context.Background())
			if err != nil {
				t.Fatalf("LoadSession() erro = %v", err)
			}
			if !bytes.Equal(got, old) {
				t.Fatalf("credencial mudou após falha anterior à publicação: obtido %q; esperado %q", got, old)
			}
			assertNoTempFiles(t, dir, path)
		})
	}
}

func TestCoordinatorSerializesWritersByCanonicalPath(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "session")
	coordinator := NewCoordinator()

	const writers = 24
	payloads := make([][]byte, writers)
	stores := make([]*FileStorage, writers)
	for i := 0; i < writers; i++ {
		payloads[i] = bytes.Repeat([]byte{byte('A' + i)}, 32*1024)
		stores[i] = mustFileStorage(t, coordinator, path)
	}

	var wg sync.WaitGroup
	errs := make(chan error, writers)
	for i := 0; i < writers; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- stores[i].StoreSession(context.Background(), payloads[i])
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("StoreSession() concorrente erro = %v", err)
		}
	}

	got, err := stores[0].LoadSession(context.Background())
	if err != nil {
		t.Fatalf("LoadSession() erro = %v", err)
	}
	matched := false
	for _, want := range payloads {
		if bytes.Equal(got, want) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatal("credencial final está truncada ou não corresponde a nenhum payload completo")
	}
	assertNoTempFiles(t, dir, path)
}

func TestPathLockRespectsContextCancellation(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	storage := mustFileStorage(t, NewCoordinator(), filepath.Join(dir, "session"))

	if err := storage.lock.lock(context.Background()); err != nil {
		t.Fatalf("lock() erro = %v", err)
	}
	defer storage.lock.unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := storage.LoadSession(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("LoadSession() erro = %v; esperado cancelamento do contexto", err)
	}
}

func TestStoreErrorsDoNotContainSessionBytes(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "session")
	secret := "never-log-this-session-secret"
	storage, err := newFileStorageWithFaults(NewCoordinator(), path, func(point faultPoint) error {
		if point == faultBeforeWrite {
			return errors.New("falha simulada de escrita")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = storage.StoreSession(context.Background(), []byte(secret))
	if err == nil {
		t.Fatal("StoreSession() erro = nil; esperada falha injetada")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("erro vaza bytes da sessão: %v", err)
	}
}

func TestCoordinatorCanonicalizesPathForSharedLock(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	coordinator := NewCoordinator()
	first := mustFileStorage(t, coordinator, filepath.Join(dir, "session"))
	second := mustFileStorage(t, coordinator, filepath.Join(dir, ".", "session"))

	if first.lock != second.lock {
		t.Fatal("aliases canônicos não compartilham o mesmo lock intraprocesso")
	}
}

func TestMissingParentIsNotReportedAsMissingSession(t *testing.T) {
	t.Parallel()

	root := t.TempDir()
	storage := mustFileStorage(t, NewCoordinator(), filepath.Join(root, "missing-parent", "session"))

	_, err := storage.LoadSession(context.Background())
	if !errors.Is(err, ErrUnsafeParent) {
		t.Fatalf("LoadSession() erro = %v; esperado ErrUnsafeParent", err)
	}
}

func TestSeparateCoordinatorsDoNotPretendToProvideCrossProcessCoordination(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	path := filepath.Join(dir, "session")
	first := mustFileStorage(t, NewCoordinator(), path)
	second := mustFileStorage(t, NewCoordinator(), path)

	if first.lock == second.lock {
		t.Fatal("coordenadores independentes compartilham lock inesperadamente")
	}
}

func mustFileStorage(t *testing.T, c *Coordinator, path string) *FileStorage {
	t.Helper()
	storage, err := c.File(path)
	if err != nil {
		t.Fatalf("Coordinator.File() erro = %v", err)
	}
	return storage
}

func assertNoTempFiles(t *testing.T, dir, path string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "."+filepath.Base(path)+".tmp-*"))
	if err != nil {
		t.Fatalf("Glob() erro = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("arquivos temporários permaneceram após a operação: %v", matches)
	}
}

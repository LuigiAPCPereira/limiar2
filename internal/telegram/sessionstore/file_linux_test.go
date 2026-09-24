//go:build linux

package sessionstore

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
)

func TestFileStorageRoundTripAndPermissions(t *testing.T) {
	t.Parallel()

	dir := privateTempDir(t)
	path := filepath.Join(dir, "telegram.session")
	storage := mustFileStorage(t, NewCoordinator(), path)
	want := []byte(`{"session":"opaque-secret"}`)

	if err := storage.StoreSession(context.Background(), want); err != nil {
		t.Fatalf("StoreSession() error = %v", err)
	}

	info, err := os.Lstat(path)
	if err != nil {
		t.Fatalf("Lstat() error = %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("mode = %04o, want 0600", got)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("stored credential is not regular: %v", info.Mode())
	}

	got, err := storage.LoadSession(context.Background())
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("LoadSession() = %q, want %q", got, want)
	}
}

func TestLoadMissingReturnsEmptyWithoutMaskingOtherFailures(t *testing.T) {
	t.Parallel()

	dir := privateTempDir(t)
	storage := mustFileStorage(t, NewCoordinator(), filepath.Join(dir, "missing.session"))

	got, err := storage.LoadSession(context.Background())
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if got != nil {
		t.Fatalf("LoadSession() = %q, want nil for physical absence", got)
	}

	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	_, err = storage.LoadSession(context.Background())
	if !errors.Is(err, ErrUnsafeParent) {
		t.Fatalf("LoadSession() error = %v, want ErrUnsafeParent", err)
	}
}

func TestPresentEmptyFileFailsClosed(t *testing.T) {
	t.Parallel()

	dir := privateTempDir(t)
	path := filepath.Join(dir, "empty.session")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	storage := mustFileStorage(t, NewCoordinator(), path)

	_, err := storage.LoadSession(context.Background())
	if !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("LoadSession() error = %v, want ErrInvalidSession", err)
	}
}

func TestStoreRejectsEmptyPayload(t *testing.T) {
	t.Parallel()

	dir := privateTempDir(t)
	storage := mustFileStorage(t, NewCoordinator(), filepath.Join(dir, "session"))

	if err := storage.StoreSession(context.Background(), nil); !errors.Is(err, ErrInvalidSession) {
		t.Fatalf("StoreSession() error = %v, want ErrInvalidSession", err)
	}
}

func TestUnsafeExistingTargetsFailClosed(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		setup func(t *testing.T, path string)
	}{
		{
			name: "permissive regular file",
			setup: func(t *testing.T, path string) {
				t.Helper()
				if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "symlink",
			setup: func(t *testing.T, path string) {
				t.Helper()
				target := path + ".target"
				if err := os.WriteFile(target, []byte("target"), 0o600); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(target, path); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "directory",
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
			dir := privateTempDir(t)
			path := filepath.Join(dir, "session")
			tt.setup(t, path)
			storage := mustFileStorage(t, NewCoordinator(), path)

			if _, err := storage.LoadSession(context.Background()); !errors.Is(err, ErrUnsafeTarget) {
				t.Fatalf("LoadSession() error = %v, want ErrUnsafeTarget", err)
			}
			if err := storage.StoreSession(context.Background(), []byte("new")); !errors.Is(err, ErrUnsafeTarget) {
				t.Fatalf("StoreSession() error = %v, want ErrUnsafeTarget", err)
			}
		})
	}
}

func TestUnsafeParentFailsClosed(t *testing.T) {
	t.Parallel()

	dir := privateTempDir(t)
	if err := os.Chmod(dir, 0o755); err != nil {
		t.Fatalf("Chmod() error = %v", err)
	}
	storage := mustFileStorage(t, NewCoordinator(), filepath.Join(dir, "session"))

	if err := storage.StoreSession(context.Background(), []byte("data")); !errors.Is(err, ErrUnsafeParent) {
		t.Fatalf("StoreSession() error = %v, want ErrUnsafeParent", err)
	}
}

func TestStoreFailureBeforePublishPreservesOldCredentialAndCleansTemp(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		point faultPoint
		err   error
	}{
		{name: "read-only style create failure", point: faultBeforeCreate, err: syscall.EROFS},
		{name: "enospc write failure", point: faultBeforeWrite, err: syscall.ENOSPC},
		{name: "file sync failure", point: faultBeforeFileSync, err: errors.New("fsync failed")},
		{name: "rename failure", point: faultBeforeRename, err: errors.New("rename failed")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := privateTempDir(t)
			path := filepath.Join(dir, "session")
			coordinator := NewCoordinator()
			base := mustFileStorage(t, coordinator, path)
			old := []byte("old-session")
			if err := base.StoreSession(context.Background(), old); err != nil {
				t.Fatalf("initial StoreSession() error = %v", err)
			}

			faulty, err := newFileStorageWithFaults(coordinator, path, func(point faultPoint) error {
				if point == tt.point {
					return tt.err
				}
				return nil
			})
			if err != nil {
				t.Fatalf("newFileStorageWithFaults() error = %v", err)
			}
			if err := faulty.StoreSession(context.Background(), []byte("new-session")); err == nil {
				t.Fatal("StoreSession() error = nil, want injected failure")
			}

			got, err := base.LoadSession(context.Background())
			if err != nil {
				t.Fatalf("LoadSession() error = %v", err)
			}
			if !bytes.Equal(got, old) {
				t.Fatalf("credential changed after pre-publish failure: got %q want %q", got, old)
			}
			assertNoTempFiles(t, dir, path)
		})
	}
}

func TestDirectorySyncFailureReportsUncertainDurabilityAfterPublish(t *testing.T) {
	t.Parallel()

	dir := privateTempDir(t)
	path := filepath.Join(dir, "session")
	coordinator := NewCoordinator()
	base := mustFileStorage(t, coordinator, path)
	if err := base.StoreSession(context.Background(), []byte("old")); err != nil {
		t.Fatalf("initial StoreSession() error = %v", err)
	}

	faulty, err := newFileStorageWithFaults(coordinator, path, func(point faultPoint) error {
		if point == faultBeforeDirSync {
			return errors.New("dir sync failed")
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := faulty.StoreSession(context.Background(), []byte("new")); err == nil {
		t.Fatal("StoreSession() error = nil, want directory sync failure")
	}

	got, err := base.LoadSession(context.Background())
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	if string(got) != "new" {
		t.Fatalf("visible credential = %q, want newly published bytes", got)
	}
	assertNoTempFiles(t, dir, path)
}

func TestCoordinatorSerializesWritersByCanonicalPath(t *testing.T) {
	t.Parallel()

	dir := privateTempDir(t)
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
			t.Fatalf("concurrent StoreSession() error = %v", err)
		}
	}

	got, err := stores[0].LoadSession(context.Background())
	if err != nil {
		t.Fatalf("LoadSession() error = %v", err)
	}
	matched := false
	for _, want := range payloads {
		if bytes.Equal(got, want) {
			matched = true
			break
		}
	}
	if !matched {
		t.Fatal("final credential is torn or does not match any complete writer payload")
	}
	assertNoTempFiles(t, dir, path)
}

func TestPathLockRespectsContextCancellation(t *testing.T) {
	t.Parallel()

	dir := privateTempDir(t)
	storage := mustFileStorage(t, NewCoordinator(), filepath.Join(dir, "session"))

	if err := storage.lock.lock(context.Background()); err != nil {
		t.Fatalf("lock() error = %v", err)
	}
	defer storage.lock.unlock()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := storage.LoadSession(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("LoadSession() error = %v, want context cancellation", err)
	}
}

func TestStoreErrorsDoNotContainSessionBytes(t *testing.T) {
	t.Parallel()

	dir := privateTempDir(t)
	path := filepath.Join(dir, "session")
	secret := "never-log-this-session-secret"
	storage, err := newFileStorageWithFaults(NewCoordinator(), path, func(point faultPoint) error {
		if point == faultBeforeWrite {
			return syscall.ENOSPC
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	err = storage.StoreSession(context.Background(), []byte(secret))
	if err == nil {
		t.Fatal("StoreSession() error = nil, want injected failure")
	}
	if strings.Contains(err.Error(), secret) {
		t.Fatalf("error leaks session bytes: %v", err)
	}
}

func TestCoordinatorCanonicalizesPathForSharedLock(t *testing.T) {
	t.Parallel()

	dir := privateTempDir(t)
	coordinator := NewCoordinator()
	first := mustFileStorage(t, coordinator, filepath.Join(dir, "session"))
	second := mustFileStorage(t, coordinator, filepath.Join(dir, ".", "session"))

	if first.lock != second.lock {
		t.Fatal("canonical aliases do not share the same intra-process lock")
	}
}

func TestMissingParentIsNotReportedAsMissingSession(t *testing.T) {
	t.Parallel()

	root := privateTempDir(t)
	storage := mustFileStorage(t, NewCoordinator(), filepath.Join(root, "missing-parent", "session"))

	_, err := storage.LoadSession(context.Background())
	if !errors.Is(err, ErrUnsafeParent) {
		t.Fatalf("LoadSession() error = %v, want ErrUnsafeParent", err)
	}
}

func TestSeparateCoordinatorsDoNotPretendToProvideCrossProcessCoordination(t *testing.T) {
	t.Parallel()

	dir := privateTempDir(t)
	path := filepath.Join(dir, "session")
	first := mustFileStorage(t, NewCoordinator(), path)
	second := mustFileStorage(t, NewCoordinator(), path)

	if first.lock == second.lock {
		t.Fatal("independent coordinators unexpectedly share a lock")
	}
}

func privateTempDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatalf("Chmod(%q) error = %v", dir, err)
	}
	return dir
}

func mustFileStorage(t *testing.T, c *Coordinator, path string) *FileStorage {
	t.Helper()
	storage, err := c.File(path)
	if err != nil {
		t.Fatalf("Coordinator.File() error = %v", err)
	}
	return storage
}

func assertNoTempFiles(t *testing.T, dir, path string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, "."+filepath.Base(path)+".tmp-*"))
	if err != nil {
		t.Fatalf("Glob() error = %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain after operation: %v", matches)
	}
}

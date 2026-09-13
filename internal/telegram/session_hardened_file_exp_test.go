package telegram_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// hardenedFileStorageExp é um harness experimental. Ele não é código de produção.
// O objetivo é testar a menor camada de arquivo que endereça as falhas observadas no
// EXP-LIMIAR-018 sem escolher ainda um storage definitivo.
type hardenedFileStorageExp struct {
	path  string
	locks *pathLockRegistry
}

type pathLockRegistry struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

func newPathLockRegistry() *pathLockRegistry {
	return &pathLockRegistry{locks: make(map[string]*sync.Mutex)}
}

func (r *pathLockRegistry) lockFor(path string) *sync.Mutex {
	r.mu.Lock()
	defer r.mu.Unlock()
	if mu, ok := r.locks[path]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	r.locks[path] = mu
	return mu
}

func (s hardenedFileStorageExp) load(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, os.ErrNotExist
	}
	return data, err
}

func (s hardenedFileStorageExp) store(ctx context.Context, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	mu := s.locks.lockFor(s.path)
	mu.Lock()
	defer mu.Unlock()

	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmpName, err := writeSyncedTemp(dir, data)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = os.Remove(tmpName)
		}
	}()

	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, s.path); err != nil {
		return err
	}
	committed = true
	return hardenAndSyncPublishedFile(s.path, dir)
}

func writeSyncedTemp(dir string, data []byte) (string, error) {
	tmp, err := os.CreateTemp(dir, ".limiar-session-*")
	if err != nil {
		return "", err
	}
	name := tmp.Name()
	cleanup := true
	defer func() {
		_ = tmp.Close()
		if cleanup {
			_ = os.Remove(name)
		}
	}()

	if err := tmp.Chmod(0o600); err != nil {
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	cleanup = false
	return name, nil
}

func hardenAndSyncPublishedFile(path, dir string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return err
	}
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}

func newExperimentalStorage(path string, locks *pathLockRegistry) hardenedFileStorageExp {
	return hardenedFileStorageExp{path: path, locks: locks}
}

func TestHardenedSessionFileRepairsPreexistingUnixPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not an equivalent Windows ACL guarantee")
	}

	path := filepath.Join(t.TempDir(), "session.bin")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed session file: %v", err)
	}
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod seeded session file: %v", err)
	}

	storage := newExperimentalStorage(path, newPathLockRegistry())
	if err := storage.store(context.Background(), []byte("replacement")); err != nil {
		t.Fatalf("store: %v", err)
	}
	assertPrivateMode(t, path)
	assertPayload(t, storage, []byte("replacement"))
}

func assertPrivateMode(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("permissions=%#o, want 0600", got)
	}
}

func assertPayload(t *testing.T, storage hardenedFileStorageExp, want []byte) {
	t.Helper()
	got, err := storage.load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("payload=%q, want %q", got, want)
	}
}

func TestHardenedSessionFileSerializesIndependentInstances(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("overwrite-by-rename semantics require a separate Windows experiment")
	}

	path := filepath.Join(t.TempDir(), "session.bin")
	locks := newPathLockRegistry()
	first := newExperimentalStorage(path, locks)
	second := newExperimentalStorage(path, locks)

	payloadA := bytes.Repeat([]byte("A"), 64*1024)
	payloadB := bytes.Repeat([]byte("B"), 64*1024)

	start := make(chan struct{})
	errs := make(chan error, 2)
	go func() {
		<-start
		errs <- first.store(context.Background(), payloadA)
	}()
	go func() {
		<-start
		errs <- second.store(context.Background(), payloadB)
	}()
	close(start)

	for range 2 {
		if err := <-errs; err != nil {
			t.Fatalf("concurrent store: %v", err)
		}
	}

	got, err := first.load(context.Background())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !bytes.Equal(got, payloadA) && !bytes.Equal(got, payloadB) {
		t.Fatalf("final payload is partial/mixed: len=%d", len(got))
	}
}

func TestHardenedSessionFileLeavesNoTemporaryFileAfterSuccessfulPublish(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("overwrite-by-rename semantics require a separate Windows experiment")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, "session.bin")
	storage := newExperimentalStorage(path, newPathLockRegistry())
	if err := storage.store(context.Background(), []byte("secret")); err != nil {
		t.Fatalf("store: %v", err)
	}

	matches, err := filepath.Glob(filepath.Join(dir, ".limiar-session-*"))
	if err != nil {
		t.Fatalf("glob temp files: %v", err)
	}
	if len(matches) != 0 {
		t.Fatalf("temporary files remain after publish: %v", matches)
	}
}

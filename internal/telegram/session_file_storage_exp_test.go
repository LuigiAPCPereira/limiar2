package telegram_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	gotdsession "github.com/gotd/td/session"
)

func TestGotdFileStorageBasicContractAndReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "session.json")

	first := &gotdsession.FileStorage{Path: path}
	if _, err := first.LoadSession(ctx); !errors.Is(err, gotdsession.ErrNotFound) {
		t.Fatalf("empty LoadSession error=%v, want session.ErrNotFound", err)
	}

	payloadA := []byte{0x00, 0x01, 0x02, 0xff, 'a', 'b', 'c'}
	if err := first.StoreSession(ctx, payloadA); err != nil {
		t.Fatalf("StoreSession first payload: %v", err)
	}

	second := &gotdsession.FileStorage{Path: path}
	got, err := second.LoadSession(ctx)
	if err != nil {
		t.Fatalf("LoadSession after reopen: %v", err)
	}
	if !bytes.Equal(got, payloadA) {
		t.Fatalf("reopen payload changed: got=%x want=%x", got, payloadA)
	}

	payloadB := []byte("replacement-session-payload")
	if err := second.StoreSession(ctx, payloadB); err != nil {
		t.Fatalf("StoreSession replacement payload: %v", err)
	}
	got, err = first.LoadSession(ctx)
	if err != nil {
		t.Fatalf("LoadSession after overwrite: %v", err)
	}
	if !bytes.Equal(got, payloadB) {
		t.Fatalf("overwrite payload changed: got=%x want=%x", got, payloadB)
	}
}

func TestGotdFileStorageCreatesPrivateFileOnUnix(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not an equivalent Windows ACL guarantee")
	}

	path := filepath.Join(t.TempDir(), "session.json")
	storage := &gotdsession.FileStorage{Path: path}
	if err := storage.StoreSession(context.Background(), []byte("secret")); err != nil {
		t.Fatalf("StoreSession: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat session file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("new session file permissions=%#o, want 0600", got)
	}
}

func TestGotdFileStorageDoesNotTightenPreexistingUnixPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not an equivalent Windows ACL guarantee")
	}

	path := filepath.Join(t.TempDir(), "session.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatalf("seed session file: %v", err)
	}
	// Neutraliza umask para tornar a precondição explícita e reproduzível.
	if err := os.Chmod(path, 0o644); err != nil {
		t.Fatalf("chmod seeded session file: %v", err)
	}

	storage := &gotdsession.FileStorage{Path: path}
	if err := storage.StoreSession(context.Background(), []byte("replacement")); err != nil {
		t.Fatalf("StoreSession: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat session file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o644 {
		t.Fatalf("preexisting session file permissions=%#o, want observed 0644", got)
	}
}

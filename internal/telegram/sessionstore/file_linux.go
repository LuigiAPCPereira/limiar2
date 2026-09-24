//go:build linux

// Package sessionstore persists Telegram authorization session bytes behind a
// Linux-only hardened local-file boundary.
package sessionstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"syscall"

	gotdsession "github.com/gotd/td/session"
)

var (
	// ErrUnsafeParent reports that the credential directory does not satisfy
	// the initial Linux single-user boundary.
	ErrUnsafeParent = errors.New("telegram session storage: unsafe parent directory")
	// ErrUnsafeTarget reports that an existing credential path is not a private,
	// regular file owned by the current effective user.
	ErrUnsafeTarget = errors.New("telegram session storage: unsafe target")
	// ErrInvalidSession reports a present credential file whose bytes cannot be
	// treated as a valid opaque session payload by this boundary.
	ErrInvalidSession = errors.New("telegram session storage: invalid session payload")
)

// Coordinator owns intra-process coordination for all file-backed Telegram
// credential stores created through it. Stores for the same canonical path
// share one cancelable lock.
type Coordinator struct {
	mu    sync.Mutex
	locks map[string]*pathLock
}

// NewCoordinator creates an explicit owner for intra-process file-store locks.
func NewCoordinator() *Coordinator {
	return &Coordinator{locks: make(map[string]*pathLock)}
}

// File returns a hardened storage bound to path.
//
// The method set intentionally matches github.com/gotd/td/session.Storage.
// Go interface satisfaction is structural; no wrapper interface is introduced.
func (c *Coordinator) File(path string) (*FileStorage, error) {
	if c == nil {
		return nil, errors.New("telegram session storage: nil coordinator")
	}
	if path == "" {
		return nil, errors.New("telegram session storage: empty path")
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("telegram session storage: canonicalize path: %w", err)
	}
	abs = filepath.Clean(abs)

	c.mu.Lock()
	lock := c.locks[abs]
	if lock == nil {
		lock = newPathLock()
		c.locks[abs] = lock
	}
	c.mu.Unlock()

	return &FileStorage{path: abs, lock: lock}, nil
}

// FileStorage stores one opaque Telegram authorization/session blob.
//
// It is intentionally scoped to Linux, local filesystems, single host and a
// single process. Cross-process locking, network filesystems and encryption are
// outside this contract.
type FileStorage struct {
	path   string
	lock   *pathLock
	faults faultInjector
}

var _ gotdsession.Storage = (*FileStorage)(nil)

// Path returns the canonical credential path. It never exposes session bytes.
func (s *FileStorage) Path() string { return s.path }

// LoadSession returns the persisted opaque session bytes.
//
// Physical absence returns (nil, nil). gotd v0.162.0 session.Loader treats a
// zero-length result as session.ErrNotFound. A present but empty file is not
// treated as absence: it fails closed as invalid/corrupt state.
func (s *FileStorage) LoadSession(ctx context.Context) ([]byte, error) {
	if err := s.validate(); err != nil {
		return nil, err
	}
	if err := s.lock.lock(ctx); err != nil {
		return nil, err
	}
	defer s.lock.unlock()

	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validateParent(filepath.Dir(s.path)); err != nil {
		return nil, err
	}

	info, err := os.Lstat(s.path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("telegram session storage: inspect target: %w", err)
	}
	if err := validatePrivateRegular(info); err != nil {
		return nil, err
	}

	buf, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("telegram session storage: read target: %w", err)
	}
	if len(buf) == 0 {
		return nil, ErrInvalidSession
	}
	return buf, nil
}

// StoreSession durably publishes opaque session bytes using a private temporary
// file in the same directory, file sync, atomic rename and directory sync.
func (s *FileStorage) StoreSession(ctx context.Context, data []byte) error {
	if err := s.validate(); err != nil {
		return err
	}
	if len(data) == 0 {
		return ErrInvalidSession
	}
	if err := s.lock.lock(ctx); err != nil {
		return err
	}
	defer s.lock.unlock()

	if err := ctx.Err(); err != nil {
		return err
	}
	parent := filepath.Dir(s.path)
	if err := validateParent(parent); err != nil {
		return err
	}
	if err := validateExistingTarget(s.path); err != nil {
		return err
	}
	if err := s.fail(faultBeforeCreate); err != nil {
		return fmt.Errorf("telegram session storage: create temporary: %w", err)
	}

	tmp, err := os.CreateTemp(parent, "."+filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("telegram session storage: create temporary: %w", err)
	}
	tmpPath := tmp.Name()
	published := false
	defer func() {
		_ = tmp.Close()
		if !published {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(0o600); err != nil {
		return fmt.Errorf("telegram session storage: protect temporary: %w", err)
	}
	if err := s.fail(faultBeforeWrite); err != nil {
		return fmt.Errorf("telegram session storage: write temporary: %w", err)
	}
	if err := writeFull(tmp, data); err != nil {
		return fmt.Errorf("telegram session storage: write temporary: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.fail(faultBeforeFileSync); err != nil {
		return fmt.Errorf("telegram session storage: sync temporary: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("telegram session storage: sync temporary: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("telegram session storage: close temporary: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.fail(faultBeforeRename); err != nil {
		return fmt.Errorf("telegram session storage: publish target: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("telegram session storage: publish target: %w", err)
	}
	published = true

	// After rename the new credential is visible. Finish the durability barrier
	// even if the caller cancels at this point; returning early would report an
	// ambiguous commit state without improving safety.
	if err := s.fail(faultBeforeDirSync); err != nil {
		return fmt.Errorf("telegram session storage: sync parent directory: %w", err)
	}
	dir, err := os.Open(parent)
	if err != nil {
		return fmt.Errorf("telegram session storage: open parent directory: %w", err)
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return fmt.Errorf("telegram session storage: sync parent directory: %w", err)
	}
	if err := dir.Close(); err != nil {
		return fmt.Errorf("telegram session storage: close parent directory: %w", err)
	}
	return nil
}

func (s *FileStorage) validate() error {
	if s == nil || s.lock == nil || s.path == "" {
		return errors.New("telegram session storage: invalid storage")
	}
	return nil
}

func validateParent(parent string) error {
	info, err := os.Lstat(parent)
	if err != nil {
		return fmt.Errorf("%w: inspect parent: %w", ErrUnsafeParent, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: parent is not a directory", ErrUnsafeParent)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: parent permissions %04o expose group/other bits", ErrUnsafeParent, info.Mode().Perm())
	}
	if err := validateOwner(info, ErrUnsafeParent); err != nil {
		return err
	}
	return nil
}

func validateExistingTarget(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("telegram session storage: inspect existing target: %w", err)
	}
	return validatePrivateRegular(info)
}

func validatePrivateRegular(info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: target is not a regular file", ErrUnsafeTarget)
	}
	if info.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("%w: target permissions %04o expose group/other bits", ErrUnsafeTarget, info.Mode().Perm())
	}
	if err := validateOwner(info, ErrUnsafeTarget); err != nil {
		return err
	}
	return nil
}

func validateOwner(info os.FileInfo, category error) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: ownership metadata unavailable", category)
	}
	if int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("%w: owner uid %d does not match effective uid", category, stat.Uid)
	}
	return nil
}

func writeFull(w io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := w.Write(data)
		if err != nil {
			return err
		}
		if n <= 0 || n > len(data) {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

type pathLock struct {
	token chan struct{}
}

func newPathLock() *pathLock {
	l := &pathLock{token: make(chan struct{}, 1)}
	l.token <- struct{}{}
	return l
}

func (l *pathLock) lock(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-l.token:
		return nil
	}
}

func (l *pathLock) unlock() { l.token <- struct{}{} }

type faultPoint uint8

const (
	faultBeforeCreate faultPoint = iota + 1
	faultBeforeWrite
	faultBeforeFileSync
	faultBeforeRename
	faultBeforeDirSync
)

type faultInjector func(faultPoint) error

func (s *FileStorage) fail(point faultPoint) error {
	if s.faults == nil {
		return nil
	}
	return s.faults(point)
}

func newFileStorageWithFaults(c *Coordinator, path string, faults faultInjector) (*FileStorage, error) {
	s, err := c.File(path)
	if err != nil {
		return nil, err
	}
	s.faults = faults
	return s, nil
}

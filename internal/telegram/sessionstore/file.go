// Package sessionstore persiste bytes opacos da sessão de autorização do Telegram
// atrás de um boundary de arquivo local portável e endurecido.
package sessionstore

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	gotdsession "github.com/gotd/td/session"
)

var (
	// ErrUnsafeParent indica que o diretório pai não satisfaz o contrato local do storage.
	ErrUnsafeParent = errors.New("armazenamento de sessão do Telegram: diretório pai inseguro")
	// ErrUnsafeTarget indica que o caminho existente não é um arquivo regular seguro.
	ErrUnsafeTarget = errors.New("armazenamento de sessão do Telegram: alvo inseguro")
	// ErrInvalidSession indica que um arquivo presente não contém um payload opaco válido.
	ErrInvalidSession = errors.New("armazenamento de sessão do Telegram: payload de sessão inválido")
)

// Coordinator possui a coordenação intraprocesso de todos os storages de credencial
// baseados em arquivo criados por ele. Storages do mesmo caminho canônico compartilham
// um único lock cancelável.
type Coordinator struct {
	mu    sync.Mutex
	locks map[string]*pathLock
}

// NewCoordinator cria o owner explícito dos locks intraprocesso do storage.
func NewCoordinator() *Coordinator {
	return &Coordinator{locks: make(map[string]*pathLock)}
}

// File retorna um storage endurecido associado ao caminho informado.
//
// O conjunto de métodos satisfaz diretamente github.com/gotd/td/session.Storage.
// A implementação usa apenas primitivas portáveis da biblioteca padrão.
func (c *Coordinator) File(path string) (*FileStorage, error) {
	if c == nil {
		return nil, errors.New("armazenamento de sessão do Telegram: coordenador ausente")
	}
	if path == "" {
		return nil, errors.New("armazenamento de sessão do Telegram: caminho vazio")
	}

	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("armazenamento de sessão do Telegram: canonicalizar caminho: %w", err)
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

// FileStorage armazena um único blob opaco de autorização/sessão do Telegram.
//
// O contrato inicial é local, single-host e coordenado dentro de um processo.
// Coordenação entre processos, filesystem de rede e criptografia ficam fora deste contrato.
// Isso é uma limitação de lifecycle, não uma restrição de sistema operacional.
type FileStorage struct {
	path   string
	lock   *pathLock
	faults faultInjector
}

var _ gotdsession.Storage = (*FileStorage)(nil)

// Path retorna o caminho canônico da credencial. Ele nunca expõe bytes da sessão.
func (s *FileStorage) Path() string { return s.path }

// LoadSession retorna os bytes opacos persistidos da sessão.
//
// Ausência física retorna (nil, nil). O Loader do gotd v0.162.0 interpreta resultado
// de tamanho zero como session.ErrNotFound. Arquivo presente e vazio não é tratado
// como ausência: falha fechado como estado inválido/corrompido.
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
		return nil, fmt.Errorf("armazenamento de sessão do Telegram: inspecionar alvo: %w", err)
	}
	if err := protectRegular(s.path, info); err != nil {
		return nil, err
	}

	buf, err := os.ReadFile(s.path)
	if err != nil {
		return nil, fmt.Errorf("armazenamento de sessão do Telegram: ler alvo: %w", err)
	}
	if len(buf) == 0 {
		return nil, ErrInvalidSession
	}
	return buf, nil
}

// StoreSession publica de forma durável os bytes opacos da sessão usando arquivo temporário
// privado no mesmo diretório, Sync do arquivo e Rename para publicação atômica quando
// suportada pelo filesystem local.
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
		return fmt.Errorf("armazenamento de sessão do Telegram: criar temporário: %w", err)
	}

	tmp, err := os.CreateTemp(parent, "."+filepath.Base(s.path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("armazenamento de sessão do Telegram: criar temporário: %w", err)
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
		return fmt.Errorf("armazenamento de sessão do Telegram: proteger temporário: %w", err)
	}
	if err := s.fail(faultBeforeWrite); err != nil {
		return fmt.Errorf("armazenamento de sessão do Telegram: escrever temporário: %w", err)
	}
	if err := writeFull(tmp, data); err != nil {
		return fmt.Errorf("armazenamento de sessão do Telegram: escrever temporário: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.fail(faultBeforeFileSync); err != nil {
		return fmt.Errorf("armazenamento de sessão do Telegram: sincronizar temporário: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("armazenamento de sessão do Telegram: sincronizar temporário: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("armazenamento de sessão do Telegram: fechar temporário: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := s.fail(faultBeforeRename); err != nil {
		return fmt.Errorf("armazenamento de sessão do Telegram: publicar alvo: %w", err)
	}
	if err := os.Rename(tmpPath, s.path); err != nil {
		return fmt.Errorf("armazenamento de sessão do Telegram: publicar alvo: %w", err)
	}
	published = true
	return nil
}

func (s *FileStorage) validate() error {
	if s == nil || s.lock == nil || s.path == "" {
		return errors.New("armazenamento de sessão do Telegram: storage inválido")
	}
	return nil
}

func validateParent(parent string) error {
	info, err := os.Lstat(parent)
	if err != nil {
		return fmt.Errorf("%w: inspecionar diretório pai: %w", ErrUnsafeParent, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("%w: caminho pai não é um diretório", ErrUnsafeParent)
	}
	return nil
}

func validateExistingTarget(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("armazenamento de sessão do Telegram: inspecionar alvo existente: %w", err)
	}
	return protectRegular(path, info)
}

func protectRegular(path string, info os.FileInfo) error {
	if !info.Mode().IsRegular() {
		return fmt.Errorf("%w: alvo não é um arquivo regular", ErrUnsafeTarget)
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return fmt.Errorf("%w: aplicar proteção privada ao alvo: %v", ErrUnsafeTarget, err)
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

package telegram

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

var ErrAuthorizationInUse = errors.New("autorização do Telegram: identidade já está ativa")

// AuthorizationCoordinator é o registro explícito de ownership do processo para identidades
// de autorização do Telegram. Operações de runtime e bootstrap da mesma identidade devem
// compartilhar um único coordenador.
//
// Ele falha imediatamente em vez de enfileirar ownership: um segundo client principal para
// a mesma autorização é erro de configuração/lifecycle, não trabalho que deva aguardar silenciosamente.
type AuthorizationCoordinator struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func NewAuthorizationCoordinator() *AuthorizationCoordinator {
	return &AuthorizationCoordinator{active: make(map[string]struct{})}
}

func (c *AuthorizationCoordinator) acquire(identityKey string) (func(), error) {
	if c == nil {
		return nil, fmt.Errorf("%w: coordenador de autorização ausente", ErrInvalidRuntimeConfig)
	}
	key := strings.TrimSpace(identityKey)
	if key == "" {
		return nil, fmt.Errorf("%w: chave da identidade de autorização vazia", ErrInvalidRuntimeConfig)
	}

	c.mu.Lock()
	if _, ok := c.active[key]; ok {
		c.mu.Unlock()
		return nil, fmt.Errorf("%w: %s", ErrAuthorizationInUse, key)
	}
	c.active[key] = struct{}{}
	c.mu.Unlock()

	var once sync.Once
	release := func() {
		once.Do(func() {
			c.mu.Lock()
			delete(c.active, key)
			c.mu.Unlock()
		})
	}
	return release, nil
}

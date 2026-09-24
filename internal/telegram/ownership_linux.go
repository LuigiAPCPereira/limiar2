//go:build linux

package telegram

import (
	"errors"
	"fmt"
	"strings"
	"sync"
)

var ErrAuthorizationInUse = errors.New("telegram authorization: identity already active")

// AuthorizationCoordinator is the explicit process owner registry for Telegram
// authorization identities. Runtime and bootstrap operations for the same
// identity must share one coordinator.
//
// It deliberately fails fast instead of queueing ownership: a second main
// client for the same authorization is a configuration/lifecycle error, not
// work that should silently wait.
type AuthorizationCoordinator struct {
	mu     sync.Mutex
	active map[string]struct{}
}

func NewAuthorizationCoordinator() *AuthorizationCoordinator {
	return &AuthorizationCoordinator{active: make(map[string]struct{})}
}

func (c *AuthorizationCoordinator) acquire(identityKey string) (func(), error) {
	if c == nil {
		return nil, fmt.Errorf("%w: nil authorization coordinator", ErrInvalidRuntimeConfig)
	}
	key := strings.TrimSpace(identityKey)
	if key == "" {
		return nil, fmt.Errorf("%w: empty authorization identity key", ErrInvalidRuntimeConfig)
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

package media

import (
	"time"

	"github.com/hashicorp/golang-lru/v2/expirable"
)

// ImageCache é um cache in-memory de bytes de imagem com eviction LRU e TTL.
// Usa github.com/hashicorp/golang-lru/v2/expirable (bounded LRU + TTL).
//
// É a camada L1 do subsistema de media (ADR 011): absorve requests repetidos
// sem chamadas MTProto ou queries ao banco. Bounded por size (default 500
// entradas, ~25MB a 50KB/imagem) e TTL (default 30min).
//
// Thread-safe: o expirable.LRU usa sync.Mutex internamente.
type ImageCache struct {
	lru *expirable.LRU[int64, []byte]
}

// NewImageCache cria um cache com limites dados.
// size: número máximo de entradas (evicção LRU quando excede).
// ttl: tempo de vida de cada entrada (0 = sem TTL, apenas LRU).
func NewImageCache(size int, ttl time.Duration) *ImageCache {
	if size <= 0 {
		size = 1
	}
	return &ImageCache{
		lru: expirable.NewLRU[int64, []byte](size, nil, ttl),
	}
}

// Get retorna os bytes da foto se presente e dentro do TTL; caso contrário
// (nil, false).
func (c *ImageCache) Get(photoID int64) ([]byte, bool) {
	return c.lru.Get(photoID)
}

// Put armazena os bytes da foto, evictando a entrada LRU se o cache exceder
// o tamanho máximo.
func (c *ImageCache) Put(photoID int64, data []byte) {
	c.lru.Add(photoID, data)
}

// Len retorna o número de entradas corrente (para asserção em testes).
func (c *ImageCache) Len() int {
	return c.lru.Len()
}

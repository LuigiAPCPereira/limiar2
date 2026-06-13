package media

import (
	"container/list"
	"sync"
	"time"
)

// ImageCache é um cache in-memory de bytes de imagem com eviction LRU e TTL.
// É a camada L1 do MediaResolver (ADR 011): absorve requests repetidos sem
// chamadas MTProto. Bounded por maxLen (default 200, ~10MB a 50KB/imagem) e TTL
// (default 30min, alinhado ao Cache-Control do endpoint).
//
// LRU real: Get promove a entrada para o topo (mais recente); Put evicta a
// entrada menos recentemente usada (fundo) quando excede maxLen. TTL é checado
// lazy em Get: entradas expiradas são removidas e tratadas como miss.
//
// Como Get promove (muta a ordem), ele toma Lock de escrita em vez de RLock — a
// seção crítica é microssegundos (troca de ponteiros de list), então a contenção
// é desprezível para o working set de ~200 entradas.
type ImageCache struct {
	mu      sync.Mutex
	maxLen  int
	ttl     time.Duration
	now     func() time.Time // injetável para testes de TTL determinísticos
	entries map[int64]*list.Element
	order   *list.List // Front = mais recente; Back = candidato a eviction
}

type cacheEntry struct {
	photoID   int64
	data      []byte
	fetchedAt time.Time
}

// NewImageCache cria um cache com limites dados. now usa time.Now em produção.
func NewImageCache(maxLen int, ttl time.Duration) *ImageCache {
	if maxLen <= 0 {
		maxLen = 1
	}
	return &ImageCache{
		maxLen:  maxLen,
		ttl:     ttl,
		now:     time.Now,
		entries: make(map[int64]*list.Element),
		order:   list.New(),
	}
}

// Get retorna os bytes da foto se presente e dentro do TTL; caso contrário
// (nil, false). Acertos promovem a entrada para o topo (semântica LRU).
func (c *ImageCache) Get(photoID int64) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.entries[photoID]
	if !ok {
		return nil, false
	}
	e := el.Value.(*cacheEntry)
	if c.ttl > 0 && c.now().Sub(e.fetchedAt) > c.ttl {
		c.removeElement(el)
		return nil, false
	}
	c.order.MoveToFront(el)
	return e.data, true
}

// Put armazena os bytes da foto, sobrescrevendo uma entrada existente, e evicta
// a entrada LRU (fundo) se o cache exceder maxLen.
func (c *ImageCache) Put(photoID int64, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.entries[photoID]; ok {
		e := el.Value.(*cacheEntry)
		e.data = data
		e.fetchedAt = c.now()
		c.order.MoveToFront(el)
		return
	}
	c.entries[photoID] = c.order.PushFront(&cacheEntry{
		photoID:   photoID,
		data:      data,
		fetchedAt: c.now(),
	})
	for c.order.Len() > c.maxLen {
		if oldest := c.order.Back(); oldest != nil {
			c.removeElement(oldest)
		}
	}
}

// removeElement remove o elemento de ambas as estruturas. O chamador segura c.mu.
func (c *ImageCache) removeElement(el *list.Element) {
	e := c.order.Remove(el).(*cacheEntry)
	delete(c.entries, e.photoID)
}

// Len retorna o número de entradas corrente (para asserção em testes).
func (c *ImageCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

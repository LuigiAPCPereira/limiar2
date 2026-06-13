package media

import (
	"sync"
	"testing"
	"time"
)

func TestImageCache_PutGet(t *testing.T) {
	c := NewImageCache(10, time.Minute)
	c.Put(1, []byte("img1"))

	got, ok := c.Get(1)
	if !ok {
		t.Fatal("Get(1) miss, esperava hit")
	}
	if string(got) != "img1" {
		t.Errorf("Get(1) = %q, quer %q", got, "img1")
	}
}

func TestImageCache_MissUnknown(t *testing.T) {
	c := NewImageCache(10, time.Minute)
	if _, ok := c.Get(999); ok {
		t.Fatal("Get(999) hit em cache vazio, esperava miss")
	}
}

func TestImageCache_TTLExpiry(t *testing.T) {
	clock := &fakeClock{}
	c := NewImageCache(10, 30*time.Minute)
	c.now = clock.now

	c.Put(1, []byte("img1"))
	if _, ok := c.Get(1); !ok {
		t.Fatal("hit imediatamente após Put")
	}

	clock.advance(31 * time.Minute) // além do TTL
	if _, ok := c.Get(1); ok {
		t.Fatal("Get(1) hit após TTL, esperava miss (expirado)")
	}
	if c.Len() != 0 {
		t.Errorf("Len = %d após expiração, quer 0 (entrada removida)", c.Len())
	}
}

func TestImageCache_LRUEviction(t *testing.T) {
	c := NewImageCache(3, time.Minute) // capacidade 3
	c.Put(1, []byte("a"))
	c.Put(2, []byte("b"))
	c.Put(3, []byte("c"))
	// capacity cheio; próximo Put evicta o LRU (1).
	c.Put(4, []byte("d"))

	if _, ok := c.Get(1); ok {
		t.Error("Get(1) hit, esperava miss (deveria ter sido evicto)")
	}
	for _, id := range []int64{2, 3, 4} {
		if _, ok := c.Get(id); !ok {
			t.Errorf("Get(%d) miss, esperava hit", id)
		}
	}
}

func TestImageCache_GetPromotesLRU(t *testing.T) {
	c := NewImageCache(3, time.Minute)
	c.Put(1, []byte("a"))
	c.Put(2, []byte("b"))
	c.Put(3, []byte("c"))

	// Acessa 1 → promove para mais recente.
	if _, ok := c.Get(1); !ok {
		t.Fatal("Get(1) miss")
	}
	// Agora 2 é o LRU. Put(4) evicta 2, não 1.
	c.Put(4, []byte("d"))

	if _, ok := c.Get(1); !ok {
		t.Error("Get(1) miss, esperava hit (foi promovido, não evicto)")
	}
	if _, ok := c.Get(2); ok {
		t.Error("Get(2) hit, esperava miss (LRU verdadeiro evicto o 2)")
	}
}

func TestImageCache_UpdateExisting(t *testing.T) {
	c := NewImageCache(2, time.Minute)
	c.Put(1, []byte("old"))
	c.Put(1, []byte("new"))

	got, ok := c.Get(1)
	if !ok || string(got) != "new" {
		t.Errorf("Get(1) = %q ok=%v, quer %q", got, ok, "new")
	}
	if c.Len() != 1 {
		t.Errorf("Len = %d após update, quer 1 (sem duplicata)", c.Len())
	}
}

func TestImageCache_Concurrent(t *testing.T) {
	c := NewImageCache(100, time.Minute)
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			c.Put(id, []byte("x"))
			c.Get(id)
			c.Get(id)
		}(int64(i))
	}
	wg.Wait()
	if c.Len() != 50 {
		t.Errorf("Len = %d, quer 50 após 50 Puts distintos", c.Len())
	}
}

// fakeClock permite testes de TTL determinísticos sem sleeps.
type fakeClock struct {
	mu   sync.Mutex
	base time.Time
}

func (f *fakeClock) now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.base
}

func (f *fakeClock) advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.base = f.base.Add(d)
}

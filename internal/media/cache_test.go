package media

import (
	"sync"
	"testing"
	"time"
)

func TestImageCache_PutAndGet(t *testing.T) {
	c := NewCache(3, time.Minute)

	c.Put(1, []byte("img1"))
	c.Put(2, []byte("img2"))

	data, ok := c.Get(1)
	if !ok || string(data) != "img1" {
		t.Errorf("Get(1) = %q, %v; want img1, true", data, ok)
	}

	data, ok = c.Get(2)
	if !ok || string(data) != "img2" {
		t.Errorf("Get(2) = %q, %v; want img2, true", data, ok)
	}

	_, ok = c.Get(99)
	if ok {
		t.Error("Get(99) = true; want false")
	}
}

func TestImageCache_LRUEviction(t *testing.T) {
	c := NewCache(2, time.Minute)

	c.Put(1, []byte("img1"))
	c.Put(2, []byte("img2"))
	c.Put(3, []byte("img3")) // evicts img1

	_, ok := c.Get(1)
	if ok {
		t.Error("Get(1) should be evicted")
	}

	data, ok := c.Get(2)
	if !ok || string(data) != "img2" {
		t.Errorf("Get(2) = %q, %v; want img2, true", data, ok)
	}

	data, ok = c.Get(3)
	if !ok || string(data) != "img3" {
		t.Errorf("Get(3) = %q, %v; want img3, true", data, ok)
	}
}

func TestImageCache_TTLExpiry(t *testing.T) {
	c := NewCache(10, 50*time.Millisecond)

	c.Put(1, []byte("img1"))

	data, ok := c.Get(1)
	if !ok || string(data) != "img1" {
		t.Errorf("Get(1) = %q, %v; want img1, true", data, ok)
	}

	time.Sleep(100 * time.Millisecond)

	_, ok = c.Get(1)
	if ok {
		t.Error("Get(1) should be expired after TTL")
	}
}

func TestImageCache_Len(t *testing.T) {
	c := NewCache(5, time.Minute)

	if c.Len() != 0 {
		t.Errorf("Len() = %d; want 0", c.Len())
	}

	c.Put(1, []byte("img1"))
	c.Put(2, []byte("img2"))

	if c.Len() != 2 {
		t.Errorf("Len() = %d; want 2", c.Len())
	}
}

// TestCache_Concurrent verifica que o cache é thread-safe para uso compartilhado
// entre Collector (Put proativo) e API (Get). Execute com -race.
func TestCache_Concurrent(t *testing.T) {
	c := NewCache(100, 30*time.Minute)

	const goroutines = 10
	const opsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(base int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				id := int64(base*opsPerGoroutine + i)
				c.Put(id, []byte{byte(id % 256)})
				// Get no mesmo ID ou em outro aleatório.
				if i%2 == 0 {
					c.Get(id)
				} else {
					c.Get(int64((base+1)%goroutines*opsPerGoroutine + i))
				}
			}
		}(g)
	}

	wg.Wait()

	// Após toda concorrência, Len deve refletir estado estável.
	n := c.Len()
	if n <= 0 || n > 100 {
		t.Errorf("Len() = %d; want 1..100", n)
	}
}

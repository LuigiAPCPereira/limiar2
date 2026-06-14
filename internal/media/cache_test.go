package media

import (
	"testing"
	"time"
)

func TestImageCache_PutAndGet(t *testing.T) {
	c := NewImageCache(3, time.Minute)

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
	c := NewImageCache(2, time.Minute)

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
	c := NewImageCache(10, 50*time.Millisecond)

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
	c := NewImageCache(5, time.Minute)

	if c.Len() != 0 {
		t.Errorf("Len() = %d; want 0", c.Len())
	}

	c.Put(1, []byte("img1"))
	c.Put(2, []byte("img2"))

	if c.Len() != 2 {
		t.Errorf("Len() = %d; want 2", c.Len())
	}
}

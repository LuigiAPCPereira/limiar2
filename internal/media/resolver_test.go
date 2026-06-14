package media

import (
	"context"
	"errors"
	"testing"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
)

// --- fakes ---

type fakeRepo struct {
	photoID int64
	thumb   []byte
	thumbErr error
}

func (r *fakeRepo) GetPhotoID(_ context.Context, _ int64) (int64, error) {
	return r.photoID, nil
}

func (r *fakeRepo) GetInlineThumb(_ context.Context, _ int64) ([]byte, error) {
	return r.thumb, r.thumbErr
}

func newResolver(repo MediaRepository, cache *ImageCache) *MediaResolver {
	return NewMediaResolver(nil, repo, cache, logger.NopLogger{})
}

// --- tests ---

func TestResolveImage_CacheHit(t *testing.T) {
	cache := NewImageCache(10, 0)
	cache.Put(42, []byte("cached-img"))

	repo := &fakeRepo{photoID: 42}
	r := newResolver(repo, cache)

	data, source, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if source != "collector-cache" {
		t.Errorf("source = %q; want collector-cache", source)
	}
	if string(data) != "cached-img" {
		t.Errorf("data = %q; want cached-img", data)
	}
}

func TestResolveImage_InlineThumbFallback(t *testing.T) {
	cache := NewImageCache(10, 0)

	repo := &fakeRepo{photoID: 42, thumb: []byte("inline-thumb-data")}
	r := newResolver(repo, cache)

	data, source, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if source != "inline-thumb" {
		t.Errorf("source = %q; want inline-thumb", source)
	}
	if string(data) != "inline-thumb-data" {
		t.Errorf("data = %q; want inline-thumb-data", data)
	}
}

func TestResolveImage_NoPhoto(t *testing.T) {
	cache := NewImageCache(10, 0)

	repo := &fakeRepo{photoID: 0} // no photo
	r := newResolver(repo, cache)

	_, _, err := r.ResolveImage(context.Background(), 100)
	if !errors.Is(err, apperrors.ErrNoPhoto) {
		t.Errorf("err = %v; want ErrNoPhoto", err)
	}
}

func TestResolveImage_NoThumbNoCache(t *testing.T) {
	cache := NewImageCache(10, 0)

	repo := &fakeRepo{photoID: 42, thumb: nil} // photo exists but no inline thumb
	r := newResolver(repo, cache)

	_, _, err := r.ResolveImage(context.Background(), 100)
	if !errors.Is(err, apperrors.ErrNoPhoto) {
		t.Errorf("err = %v; want ErrNoPhoto", err)
	}
}

func TestResolveImage_DBError(t *testing.T) {
	cache := NewImageCache(10, 0)

	repo := &fakeRepo{photoID: 42, thumbErr: errors.New("db offline")}
	r := newResolver(repo, cache)

	_, _, err := r.ResolveImage(context.Background(), 100)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestPutCache(t *testing.T) {
	cache := NewImageCache(10, 0)
	r := newResolver(&fakeRepo{}, cache)

	r.PutCache(42, []byte("proactive-img"))

	data, ok := cache.Get(42)
	if !ok || string(data) != "proactive-img" {
		t.Errorf("cache.Get(42) = %q, %v; want proactive-img, true", data, ok)
	}
}

func TestPutCache_NilCache(t *testing.T) {
	r := newResolver(&fakeRepo{}, nil)

	// Should not panic
	r.PutCache(42, []byte("img"))
}

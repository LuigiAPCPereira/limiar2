package media

import (
	"context"
	"errors"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/storage"
)

// --- fakes ---

type fakeClient struct {
	downloadFn          func(ctx context.Context, req PhotoDownloadRequest) ([]byte, error)
	renewFn             func(ctx context.Context, channelID, msgID int64) (*storage.PhotoMetadata, error)
	refetchFn           func(ctx context.Context, channelID, msgID int64) (*storage.PhotoMetadata, error)
	refetchAndDownloadFn func(ctx context.Context, channelID, msgID int64) ([]byte, *storage.PhotoMetadata, error)
}

func (f *fakeClient) DownloadPhoto(ctx context.Context, req PhotoDownloadRequest) ([]byte, error) {
	return f.downloadFn(ctx, req)
}
func (f *fakeClient) RenewFileReference(ctx context.Context, ch, msg int64) (*storage.PhotoMetadata, error) {
	return f.renewFn(ctx, ch, msg)
}
func (f *fakeClient) RefetchFromChannel(ctx context.Context, ch, msg int64) (*storage.PhotoMetadata, error) {
	return f.refetchFn(ctx, ch, msg)
}
func (f *fakeClient) RefetchAndDownload(ctx context.Context, ch, msg int64) ([]byte, *storage.PhotoMetadata, error) {
	if f.refetchAndDownloadFn != nil {
		return f.refetchAndDownloadFn(ctx, ch, msg)
	}
	return nil, nil, errors.New("RefetchAndDownload not configured")
}
func (f *fakeClient) ScrapePhotoURL(ctx context.Context, username string, msgID int64) (string, error) {
	return "", errors.New("ScrapePhotoURL not configured")
}

type fakeRepo struct {
	mu           sync.Mutex
	meta         *storage.PhotoMetadata
	getErr       error
	upRefCalled  bool
	upMetaCalled bool
	updatedRef   string
	updatedMeta  *storage.PhotoMetadata
}

func (r *fakeRepo) GetPhotoMetadata(context.Context, int64) (*storage.PhotoMetadata, error) {
	return r.meta, r.getErr
}
func (r *fakeRepo) UpdateFileReference(_ context.Context, _ int64, ref string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upRefCalled = true
	r.updatedRef = ref
	return nil
}
func (r *fakeRepo) UpdatePhotoMetadata(_ context.Context, _ int64, meta *storage.PhotoMetadata) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.upMetaCalled = true
	r.updatedMeta = meta
	return nil
}

func newResolver(c MediaClient, repo MediaRepository) *MediaResolver {
	return NewMediaResolver(c, repo, NewImageCache(8, time.Minute), logger.NopLogger{})
}

func photoMeta(photoID int64) *storage.PhotoMetadata {
	return &storage.PhotoMetadata{
		MsgID:         100,
		ChannelID:     7,
		PhotoID:       photoID,
		AccessHash:    999,
		FileReference: "", // decode → nil sem erro
		DCID:          2,
	}
}

// --- tests ---

func TestResolveImage_CacheMiss_DownloadsAndCaches(t *testing.T) {
	repo := &fakeRepo{meta: photoMeta(50)}
	var calls int
	cli := &fakeClient{
		downloadFn: func(context.Context, PhotoDownloadRequest) ([]byte, error) {
			calls++
			return []byte("jpeg"), nil
		},
	}
	r := newResolver(cli, repo)

	got, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if string(got) != "jpeg" {
		t.Errorf("got %q, quer jpeg", got)
	}
	if calls != 1 {
		t.Errorf("download calls = %d, quer 1", calls)
	}

	// Segunda chamada: cache hit, sem novo download.
	if _, err := r.ResolveImage(context.Background(), 100); err != nil {
		t.Fatalf("segunda chamada: %v", err)
	}
	if calls != 1 {
		t.Errorf("download calls = %d após cache hit, quer 1", calls)
	}
}

func TestResolveImage_NoPhoto(t *testing.T) {
	// PhotoID=0 → mensagem sem foto.
	repo := &fakeRepo{meta: &storage.PhotoMetadata{MsgID: 100, ChannelID: 7}}
	r := newResolver(&fakeClient{}, repo)

	_, err := r.ResolveImage(context.Background(), 100)
	if !errors.Is(err, apperrors.ErrNoPhoto) {
		t.Errorf("err = %v, quer ErrNoPhoto", err)
	}
}

func TestResolveImage_GetMetadataError(t *testing.T) {
	repo := &fakeRepo{meta: nil, getErr: errors.New("db offline")}
	r := newResolver(&fakeClient{}, repo)

	_, err := r.ResolveImage(context.Background(), 100)
	if err == nil {
		t.Fatal("esperava erro do repositório")
	}
	if !strings.Contains(err.Error(), "media: get_metadata") {
		t.Errorf("err = %q, quer prefixo 'media: get_metadata'", err.Error())
	}
}

func TestResolveImage_L3SoftRenew(t *testing.T) {
	repo := &fakeRepo{meta: photoMeta(50)}
	var downloads int
	renewed := photoMeta(50)
	cli := &fakeClient{
		downloadFn: func(context.Context, PhotoDownloadRequest) ([]byte, error) {
			downloads++
			if downloads == 1 {
				return nil, apperrors.ErrFileReferenceExpired // gatilho L3
			}
			return []byte("jpeg-renovado"), nil
		},
		renewFn: func(context.Context, int64, int64) (*storage.PhotoMetadata, error) {
			return renewed, nil
		},
	}
	r := newResolver(cli, repo)

	got, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if string(got) != "jpeg-renovado" {
		t.Errorf("got %q, quer jpeg-renovado", got)
	}
	if downloads != 2 {
		t.Errorf("downloads = %d, quer 2 (expirado + soft-ok)", downloads)
	}
	if !repo.upRefCalled {
		t.Error("UpdateFileReference não chamado na renovação soft")
	}
}

func TestResolveImage_L3HardRenew(t *testing.T) {
	repo := &fakeRepo{meta: photoMeta(50)}
	var downloads int
	renewed := photoMeta(50)
	cli := &fakeClient{
		downloadFn: func(context.Context, PhotoDownloadRequest) ([]byte, error) {
			downloads++
			switch downloads {
			case 1:
				return nil, apperrors.ErrFileReferenceExpired // gatilho L3
			case 2:
				return nil, errors.New("download falhou após soft renew") // soft retry falha
			default:
				return []byte("unexpected"), nil
			}
		},
		renewFn: func(context.Context, int64, int64) (*storage.PhotoMetadata, error) {
			return renewed, nil
		},
		refetchFn: func(context.Context, int64, int64) (*storage.PhotoMetadata, error) {
			t.Fatal("RefetchFromChannel não deveria ser chamado — L3 hard usa RefetchAndDownload")
			return nil, nil
		},
		refetchAndDownloadFn: func(context.Context, int64, int64) ([]byte, *storage.PhotoMetadata, error) {
			return []byte("jpeg-hard"), renewed, nil // hard: fetch+download na mesma sessão
		},
	}
	r := newResolver(cli, repo)

	got, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if string(got) != "jpeg-hard" {
		t.Errorf("got %q, quer jpeg-hard", got)
	}
	if downloads != 2 {
		t.Errorf("downloads = %d, quer 2 (expirado + soft-falha; hard usa RefetchAndDownload)", downloads)
	}
	if !repo.upMetaCalled {
		t.Error("UpdatePhotoMetadata não chamado na renovação hard")
	}
}

func TestResolveImage_AllRenewFails(t *testing.T) {
	repo := &fakeRepo{meta: photoMeta(50)}
	cli := &fakeClient{
		downloadFn: func(context.Context, PhotoDownloadRequest) ([]byte, error) {
			return nil, apperrors.ErrFileReferenceExpired
		},
		renewFn: func(context.Context, int64, int64) (*storage.PhotoMetadata, error) {
			return nil, errors.New("soft fail")
		},
		refetchAndDownloadFn: func(context.Context, int64, int64) ([]byte, *storage.PhotoMetadata, error) {
			return nil, nil, errors.New("hard fail")
		},
	}
	r := newResolver(cli, repo)

	_, err := r.ResolveImage(context.Background(), 100)
	if err == nil {
		t.Fatal("esperava erro quando toda renovação falha")
	}
	if !strings.Contains(err.Error(), "media: hard_renew") {
		t.Errorf("err = %q, quer prefixo 'media: hard_renew'", err.Error())
	}
}

// TestResolveImage_SingleflightDedup valida que N requests concorrentes para a
// mesma foto disparam APENAS UM download MTProto (corretude, ADR 011 §4).
func TestResolveImage_SingleflightDedup(t *testing.T) {
	repo := &fakeRepo{meta: photoMeta(50)}
	gate := make(chan struct{})
	var entered int32
	var downloads int32
	cli := &fakeClient{
		downloadFn: func(context.Context, PhotoDownloadRequest) ([]byte, error) {
			atomic.AddInt32(&entered, 1)
			<-gate // bloqueia até o teste liberar — amplia a janela de coalescing
			atomic.AddInt32(&downloads, 1)
			return []byte("jpeg"), nil
		},
	}
	r := newResolver(cli, repo)

	const N = 10
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < N; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _ = r.ResolveImage(context.Background(), 100)
		}()
	}
	close(start) // dispara todas concorrentemente

	// Espera o líder entrar no download (singleflight garante só 1).
	for atomic.LoadInt32(&entered) == 0 {
		runtime.Gosched()
	}
	time.Sleep(20 * time.Millisecond) // dá tempo aos seguidores alcançarem group.Do
	close(gate)                       // libera o líder
	wg.Wait()

	if got := atomic.LoadInt32(&downloads); got != 1 {
		t.Errorf("downloads = %d, quer 1 (singleflight deve coalescer %d calls)", got, N)
	}
}

// TestResolveImage_SingleflightByKeyPhotoID valida que a chave do singleflight é
// photo_id (não msg_id): dois msgIDs diferentes com a mesma foto geram 1 download.
func TestResolveImage_SingleflightByKeyPhotoID(t *testing.T) {
	// Dois msgIDs apontam para a MESMA photo_id (forward/cross-channel).
	repo := &fakeRepo{meta: photoMeta(50)}
	gate := make(chan struct{})
	entered := make(chan struct{}, 1)
	var downloads int32
	cli := &fakeClient{
		downloadFn: func(context.Context, PhotoDownloadRequest) ([]byte, error) {
			select {
			case entered <- struct{}{}:
			default:
			}
			<-gate
			atomic.AddInt32(&downloads, 1)
			return []byte("jpeg"), nil
		},
	}
	r := newResolver(cli, repo)

	var wg sync.WaitGroup
	for _, msgID := range []int64{100, 200} { // msgIDs diferentes, mesma photo_id
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			_, _ = r.ResolveImage(context.Background(), id)
		}(msgID)
	}
	<-entered                         // líder entrou
	time.Sleep(20 * time.Millisecond) // segundo request alcança group.Do
	close(gate)
	wg.Wait()

	if got := atomic.LoadInt32(&downloads); got != 1 {
		t.Errorf("downloads = %d, quer 1 (dedup por photo_id, não msg_id)", got)
	}
}

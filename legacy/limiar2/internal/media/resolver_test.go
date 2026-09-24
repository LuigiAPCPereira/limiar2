package media

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"testing"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/model"
)

// --- fakes ---

// fakeRepo satisfaz media.Repository para testes. Cada campo controla uma
// camada da cadeia de fallback; valores zero representam "ausente".
type fakeRepo struct {
	photoID      int64
	photoData    []byte
	photoDataErr error
	// photoDataByID, quando não-nil, faz GetPhotoData responder por photo_id
	// exato (retorna nil para IDs ausentes). Permite testar que o resolver
	// consulta o photo_id exato dos metadados, e não um valor arredondado.
	photoDataByID map[int64][]byte
	// photoDataCalls registra cada photo_id consultado em GetPhotoData.
	photoDataCalls []int64
	thumb          []byte
	thumbs         map[int64][]byte
	thumbErr       error
	meta           *model.PhotoMetadata
	metaErr        error

	// captura da última escrita via SavePhotoData.
	savedID   int64
	savedData []byte
	// saveCalls conta quantas vezes SavePhotoData foi invocado. Usado para
	// defender o invariante read-only do dashboard: o caminho HTTP de mídia
	// nunca deve persistir em photo_cache.
	saveCalls int
}

func (r *fakeRepo) GetPhotoID(_ context.Context, _ int64) (int64, error) {
	return r.photoID, nil
}

func (r *fakeRepo) GetPhotoData(_ context.Context, photoID int64) ([]byte, error) {
	r.photoDataCalls = append(r.photoDataCalls, photoID)
	if r.photoDataByID != nil {
		// Modo seletivo: responde apenas para o photo_id exato presente no mapa.
		// IDs ausentes (incluindo um photo_id arredondado por float64) devolvem
		// nil — expondo colisões caso o resolver use o ID errado.
		return r.photoDataByID[photoID], r.photoDataErr
	}
	return r.photoData, r.photoDataErr
}

func (r *fakeRepo) SavePhotoData(_ context.Context, photoID int64, data []byte) error {
	r.saveCalls++
	r.savedID = photoID
	r.savedData = data
	return nil
}

func (r *fakeRepo) GetPhotoMetadata(_ context.Context, _ int64) (*model.PhotoMetadata, error) {
	return r.meta, r.metaErr
}

func (r *fakeRepo) GetInlineThumb(_ context.Context, key int64) ([]byte, error) {
	if r.thumbs != nil {
		return r.thumbs[key], r.thumbErr
	}
	return r.thumb, r.thumbErr
}

// fakeClient satisfaz media.Client.
type fakeClient struct {
	data    []byte
	err     error
	called  bool
	lastReq PhotoDownloadRequest
}

func (c *fakeClient) DownloadPhoto(_ context.Context, req PhotoDownloadRequest) ([]byte, error) {
	c.called = true
	c.lastReq = req
	return c.data, c.err
}

func (c *fakeClient) DownloadPhotoBatch(_ context.Context, reqs []PhotoDownloadRequest, handler func(int64, []byte, error)) error {
	for _, req := range reqs {
		handler(req.PhotoID, c.data, c.err)
	}
	return nil
}

// compile-time checks.
var (
	_ Repository = (*fakeRepo)(nil)
	_ Client     = (*fakeClient)(nil)
)

func newResolver(repo Repository, cache *Cache, client Client) *Resolver {
	return NewResolver(repo, cache, client, logger.NopLogger{})
}

// --- L1: cache in-memory ---

func TestResolveImage_CacheHit(t *testing.T) {
	cache := NewCache(10, 0)
	cache.Put(42, []byte("cached-img"))

	r := newResolver(&fakeRepo{photoID: 42}, cache, nil)

	data, source, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if source != "cache" {
		t.Errorf("source = %q; want cache", source)
	}
	if string(data) != "cached-img" {
		t.Errorf("data = %q; want cached-img", data)
	}
}

// --- L2: photo_cache no DB ---

func TestResolveImage_PhotoCacheHit(t *testing.T) {
	cache := NewCache(10, 0)
	repo := &fakeRepo{photoID: 42, photoData: []byte("photo-cache-img")}
	r := newResolver(repo, cache, nil)

	data, source, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if source != "photo-cache" {
		t.Errorf("source = %q; want photo-cache", source)
	}
	if string(data) != "photo-cache-img" {
		t.Errorf("data = %q; want photo-cache-img", data)
	}
	// L2 hit promove ao cache L1 para a próxima chamada.
	if cached, ok := cache.Get(42); !ok || string(cached) != "photo-cache-img" {
		t.Errorf("cache.Get(42) = %q, %v; esperado promoção para L1", cached, ok)
	}
}

// L2 hit não dispara download mesmo com client configurado.
func TestResolveImage_PhotoCacheSkipsDownload(t *testing.T) {
	cache := NewCache(10, 0)
	client := &fakeClient{data: []byte("should-not-download")}
	repo := &fakeRepo{photoID: 42, photoData: []byte("photo-cache-img")}
	r := newResolver(repo, cache, client)

	_, source, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if source != "photo-cache" {
		t.Errorf("source = %q; want photo-cache", source)
	}
	if client.called {
		t.Error("client.DownloadPhoto foi chamado; L2 deveria ter evitado o download")
	}
}

// --- L3: download MTProto on-demand ---

func TestResolveImage_Downloaded(t *testing.T) {
	cache := NewCache(10, 0)
	fileRef := base64.StdEncoding.EncodeToString([]byte("secret-ref"))
	client := &fakeClient{data: []byte("downloaded-img")}
	repo := &fakeRepo{
		photoID: 42,
		meta: &model.PhotoMetadata{
			PhotoID:       42,
			AccessHash:    123,
			DCID:          4,
			FileReference: fileRef,
			ChannelID:     1352231186,
			MsgID:         95232,
		},
	}
	r := newResolver(repo, cache, client)

	data, source, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if source != "downloaded" {
		t.Errorf("source = %q; want downloaded", source)
	}
	if string(data) != "downloaded-img" {
		t.Errorf("data = %q; want downloaded-img", data)
	}
	if !client.called {
		t.Error("client.DownloadPhoto não foi chamado")
	}
	// FileReference base64 decodificado corretamente para o request MTProto.
	if string(client.lastReq.FileReference) != "secret-ref" {
		t.Errorf("lastReq.FileReference = %q; want secret-ref", client.lastReq.FileReference)
	}
	if client.lastReq.AccessHash != 123 || client.lastReq.DCID != 4 ||
		client.lastReq.ChannelID != 1352231186 || client.lastReq.MessageID != 95232 {
		t.Errorf("lastReq = %+v; want AccessHash=123 DCID=4 ChannelID=1352231186 MessageID=95232", client.lastReq)
	}
	// Download bem-sucedido é persistido em photo_cache.
	if repo.savedID != 42 || string(repo.savedData) != "downloaded-img" {
		t.Errorf("SavePhotoData = (id=%d, data=%q); want (42, downloaded-img)", repo.savedID, repo.savedData)
	}
	// ...e promovido ao cache L1.
	if cached, ok := cache.Get(42); !ok || string(cached) != "downloaded-img" {
		t.Errorf("cache.Get(42) = %q, %v; esperado promoção para L1", cached, ok)
	}
}

// L3 é pulado quando client == nil (modo processor run sem Telegram).
func TestResolveImage_DownloadSkippedWhenClientNil(t *testing.T) {
	cache := NewCache(10, 0)
	repo := &fakeRepo{
		photoID: 42,
		thumb:   []byte("inline-thumb-data"),
		meta:    &model.PhotoMetadata{PhotoID: 42, AccessHash: 123, DCID: 4},
	}
	r := newResolver(repo, cache, nil)

	data, source, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if source != "inline-thumb" {
		t.Errorf("source = %q; want inline-thumb (client nil → pula L3)", source)
	}
	if string(data) != "inline-thumb-data" {
		t.Errorf("data = %q; want inline-thumb-data", data)
	}
}

// L3 aborta quando metadados MTProto estão incompletos (AccessHash/DCID == 0).
func TestResolveImage_DownloadSkippedIncompleteMetadata(t *testing.T) {
	cache := NewCache(10, 0)
	client := &fakeClient{data: []byte("downloaded-img")}
	repo := &fakeRepo{
		photoID: 42,
		thumb:   []byte("inline-thumb-data"),
		meta:    &model.PhotoMetadata{PhotoID: 42, AccessHash: 0, DCID: 0}, // incompleto
	}
	r := newResolver(repo, cache, client)

	_, source, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if source != "inline-thumb" {
		t.Errorf("source = %q; want inline-thumb (metadados incompletos → pula L3)", source)
	}
	if client.called {
		t.Error("client.DownloadPhoto foi chamado com metadados incompletos")
	}
}

// Falha de download cai para L4 (inline_thumb).
func TestResolveImage_DownloadFailsFallsToInlineThumb(t *testing.T) {
	cache := NewCache(10, 0)
	client := &fakeClient{err: errors.New("FILE_REFERENCE_EXPIRED")}
	repo := &fakeRepo{
		photoID: 42,
		thumb:   []byte("inline-thumb-data"),
		meta:    &model.PhotoMetadata{PhotoID: 42, AccessHash: 123, DCID: 4},
	}
	r := newResolver(repo, cache, client)

	data, source, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if source != "inline-thumb" {
		t.Errorf("source = %q; want inline-thumb (fallback após falha de download)", source)
	}
	if string(data) != "inline-thumb-data" {
		t.Errorf("data = %q; want inline-thumb-data", data)
	}
}

// --- L4: inline_thumb ---

func TestResolveImage_InlineThumbFallback(t *testing.T) {
	cache := NewCache(10, 0)
	repo := &fakeRepo{photoID: 42, thumb: []byte("inline-thumb-data")}
	r := newResolver(repo, cache, nil)

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

func TestResolveImage_InlineThumbUsesProcessedMessageID(t *testing.T) {
	repo := &fakeRepo{
		photoID: 999,
		thumbs: map[int64][]byte{
			100: []byte("thumb-da-mensagem-certa"),
			999: []byte("thumb-de-outro-produto"),
		},
	}
	r := newResolver(repo, NewCache(10, 0), nil)

	data, source, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if source != "inline-thumb" {
		t.Errorf("source = %q; want inline-thumb", source)
	}
	if string(data) != "thumb-da-mensagem-certa" {
		t.Errorf("data = %q; want thumb-da-mensagem-certa", data)
	}
}

func TestResolveImage_InlineThumbDoesNotPopulateFullResCache(t *testing.T) {
	cache := NewCache(10, 0)
	repo := &fakeRepo{photoID: 42, thumb: []byte("inline-thumb-data")}
	r := newResolver(repo, cache, nil)

	_, source, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if source != "inline-thumb" {
		t.Fatalf("source = %q; want inline-thumb", source)
	}
	if data, ok := cache.Get(42); ok {
		t.Fatalf("inline thumb foi promovido para cache full-res: %q", data)
	}
}

// --- erros e ausência ---

func TestResolveImage_NoPhoto(t *testing.T) {
	r := newResolver(&fakeRepo{photoID: 0}, NewCache(10, 0), nil)

	_, _, err := r.ResolveImage(context.Background(), 100)
	if !errors.Is(err, apperrors.ErrNoPhoto) {
		t.Errorf("err = %v; want ErrNoPhoto", err)
	}
}

func TestResolveImage_NoThumbNoCache(t *testing.T) {
	// foto existe, mas sem photo_cache, sem client e sem inline_thumb.
	r := newResolver(&fakeRepo{photoID: 42}, NewCache(10, 0), nil)

	_, _, err := r.ResolveImage(context.Background(), 100)
	if !errors.Is(err, apperrors.ErrNoPhoto) {
		t.Errorf("err = %v; want ErrNoPhoto", err)
	}
}

func TestResolveImage_PhotoDataError(t *testing.T) {
	r := newResolver(&fakeRepo{photoID: 42, photoDataErr: errors.New("db offline")}, NewCache(10, 0), nil)

	_, _, err := r.ResolveImage(context.Background(), 100)
	if err == nil {
		t.Fatal("esperado erro de GetPhotoData")
	}
}

func TestResolveImage_DBError(t *testing.T) {
	r := newResolver(&fakeRepo{photoID: 42, thumbErr: errors.New("db offline")}, NewCache(10, 0), nil)

	_, _, err := r.ResolveImage(context.Background(), 100)
	if err == nil {
		t.Fatal("esperado erro de GetInlineThumb")
	}
}

// --- PutCache ---

func TestPutCache(t *testing.T) {
	cache := NewCache(10, 0)
	r := newResolver(&fakeRepo{}, cache, nil)

	r.PutCache(42, []byte("proactive-img"))

	data, ok := cache.Get(42)
	if !ok || string(data) != "proactive-img" {
		t.Errorf("cache.Get(42) = %q, %v; want proactive-img, true", data, ok)
	}
}

func TestPutCache_NilCache(t *testing.T) {
	r := newResolver(&fakeRepo{}, nil, nil)

	// Não deve panicar.
	r.PutCache(42, []byte("img"))
}

// --- Regressão: photo_id MTProto de 64 bits (metadados exatos vs. arredondado) ---

// Regressão do sistema de imagens: quando GetPhotoMetadata devolve um photo_id
// exato (derivado do raw payload, sem perda de float64), o resolver deve usar
// ESSE photo_id para as consultas ao cache/photo_cache — nunca o photo_id
// arredondado persistido em colunas antigas. Caso contrário, dois IDs MTProto de
// 64 bits distintos colidiriam e o resolver retornaria os bytes de outra foto.
//
// Cenário: metadados exatos apontam exactID; GetPhotoID devolve roundedID
// (diferente). Apenas roundedID possui bytes em photo_cache (armadilha de
// colisão). O resolver deve consultar exactID, achar nada e cair em ErrNoPhoto —
// nunca devolver os bytes do photo_id arredondado.
func TestResolveImage_UsesExactMetadataPhotoID_NotRounded(t *testing.T) {
	const (
		// IDs MTProto de 64 bits: valor exato vs. valor que sobraria de um
		// arredondamento por float64. Intencionalmente distintos para expor colisão.
		exactPhotoID   int64 = 7189234567890123456
		roundedPhotoID int64 = 7189234567890123264
	)

	repo := &fakeRepo{
		photoID: roundedPhotoID, // coluna antiga processada antes da correção
		meta: &model.PhotoMetadata{
			PhotoID: exactPhotoID, // metadados exatos derivados do raw payload
		},
		photoDataByID: map[int64][]byte{roundedPhotoID: []byte("bytes-de-outra-foto")},
	}
	r := newResolver(repo, NewCache(10, 0), nil)

	data, _, err := r.ResolveImage(context.Background(), 100)

	// O resolver consultou APENAS o photo_id exato dos metadados.
	if len(repo.photoDataCalls) != 1 || repo.photoDataCalls[0] != exactPhotoID {
		t.Fatalf("GetPhotoData chamado com %v; quer [%d] (photo_id exato)",
			repo.photoDataCalls, exactPhotoID)
	}
	// Sem dados para o photo_id exato → ErrNoPhoto; nunca os bytes da colisão.
	if !errors.Is(err, apperrors.ErrNoPhoto) {
		t.Fatalf("err = %v; quer ErrNoPhoto (photo_id exato sem cache)", err)
	}
	if data != nil {
		t.Fatalf("retornou %q; esperado nil — não devolver bytes do photo_id arredondado", data)
	}
}

// Complemento: quando há bytes para o photo_id exato, o resolver os retorna
// (source "photo-cache") mesmo que GetPhotoID aponte outro ID. Prova que a
// recuperação — não só a ausência — usa o photo_id dos metadados.
func TestResolveImage_ReturnsDataForExactMetadataPhotoID(t *testing.T) {
	const (
		exactPhotoID int64 = 4985843018296396847
		stalePhotoID int64 = 4985843018296396800
	)
	want := []byte("bytes-da-foto-exata")

	repo := &fakeRepo{
		photoID: stalePhotoID,
		meta:    &model.PhotoMetadata{PhotoID: exactPhotoID},
		photoDataByID: map[int64][]byte{
			exactPhotoID: want,
			stalePhotoID: []byte("bytes-errados"),
		},
	}
	r := newResolver(repo, NewCache(10, 0), nil)

	data, source, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if source != "photo-cache" {
		t.Errorf("source = %q; want photo-cache", source)
	}
	if !bytes.Equal(data, want) {
		t.Errorf("data = %q; want %q (bytes do photo_id exato)", data, want)
	}
}

// --- Regressão arquitetural: modo read-only (client == nil) nunca persiste ---

// O dashboard e o `processor run` operam o resolver SEM client MTProto. Nesse
// modo o resolver deve servir mídia pela cadeia cache → photo_cache →
// inline_thumb SEM nunca acionar L3 (download) e, consequentemente, SEM nunca
// chamar SavePhotoData. Qualquer escrita em photo_cache originada de um
// request HTTP viola o invariante "Dashboard → escrita em banco" (AGENTS.md §8).
//
// Cenário com garras: photo_cache ausente (L2 miss) força a cadeia além de L2.
// Com client nil, o resolver cai em inline_thumb (L4) em vez de tentar L3.
func TestResolveImage_NilClientInlineThumb_NeverPersists(t *testing.T) {
	cache := NewCache(10, 0)
	repo := &fakeRepo{
		photoID: 42,
		// photoData zero → L2 miss: se o resolver tentasse L3, persistiria.
		meta:  &model.PhotoMetadata{PhotoID: 42, AccessHash: 123, DCID: 4},
		thumb: []byte("inline-thumb-data"),
	}
	r := newResolver(repo, cache, nil) // client nil → modo read-only

	data, source, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if source != "inline-thumb" {
		t.Errorf("source = %q; want inline-thumb (client nil → pula L3)", source)
	}
	if string(data) != "inline-thumb-data" {
		t.Errorf("data = %q; want inline-thumb-data", data)
	}
	if repo.saveCalls != 0 {
		t.Errorf("SavePhotoData chamado %dx; modo read-only (client nil) nunca deve persistir em photo_cache", repo.saveCalls)
	}
}

// Um hit em photo_cache (L2) serve os bytes e promove ao cache L1 em memória,
// mas NÃO deve re-persistir em photo_cache: o byte já está lá. Uma regressão
// que adicionasse SavePhotoData no caminho de hit faria escrita redundante a
// cada GET do dashboard — este teste pinça exatamente isso.
func TestResolveImage_PhotoCacheHit_NeverPersists(t *testing.T) {
	cache := NewCache(10, 0)
	repo := &fakeRepo{photoID: 42, photoData: []byte("photo-cache-img")}
	// client presente provaria que mesmo com capacidade de download, o hit em
	// L2 curto-circuita a cadeia antes de qualquer escrita.
	client := &fakeClient{data: []byte("should-not-download")}
	r := newResolver(repo, cache, client)

	data, source, err := r.ResolveImage(context.Background(), 100)
	if err != nil {
		t.Fatalf("ResolveImage: %v", err)
	}
	if source != "photo-cache" {
		t.Errorf("source = %q; want photo-cache", source)
	}
	if string(data) != "photo-cache-img" {
		t.Errorf("data = %q; want photo-cache-img", data)
	}
	if client.called {
		t.Error("client.DownloadPhoto foi chamado; hit em L2 deveria curto-circuitar L3")
	}
	if repo.saveCalls != 0 {
		t.Errorf("SavePhotoData chamado %dx; hit em photo_cache não deve re-persistir", repo.saveCalls)
	}
}

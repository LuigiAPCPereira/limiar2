package collector

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/media"
	"github.com/limiar/collector/internal/model"
)

// --- Regressão: download proativo de imagens no flush em batch (#2) ---
//
// Contrato: quando SetMediaDownload está configurado, o flush — após commitar o
// lote — baixa proativamente a imagem full-res de cada mensagem INSERIDA que
// contém foto (metadados MTProto no raw payload), na janela em que o
// file_reference ainda é válido, e entrega os bytes ao cache compartilhado
// (imageCache). O download não dispara para mensagens sem foto nem para
// duplicatas (não inseridas). O mecanismo esperado é por-mensagem via
// mediaClient.DownloadPhoto (único método declarado na interface mediaClient).

// flushRepo satisfaz collector.Repository para testes de flush. SaveRawMessageBatch
// retorna `inserted` configurável (default: todas inseridas) para controlar o
// caminho de download. batchCalls conta quantas vezes o batch foi cometido.
type flushRepo struct {
	inserted   []bool // nil => todas true
	batchCalls int
}

func (r *flushRepo) ListChannels(context.Context) ([]*model.Channel, error) { return nil, nil }
func (r *flushRepo) SaveRawMessage(context.Context, *model.RawMessage) (bool, error) {
	return true, nil
}
func (r *flushRepo) SaveRawMessageBatch(_ context.Context, msgs []*model.RawMessage) ([]bool, int, error) {
	r.batchCalls++
	if r.inserted != nil {
		return r.inserted, countTrue(r.inserted), nil
	}
	ins := make([]bool, len(msgs))
	for i := range msgs {
		ins[i] = true
	}
	return ins, len(msgs), nil
}
func (r *flushRepo) UpdateChannelLastMessage(context.Context, int64, int64, time.Time) error {
	return nil
}

func countTrue(b []bool) int {
	n := 0
	for _, v := range b {
		if v {
			n++
		}
	}
	return n
}

// captureMediaClient satisfaz mediaClient e registra cada DownloadPhoto.
func waitUntil(t *testing.T, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	if cond() {
		return
	}
	t.Fatal("condição assíncrona não satisfeita antes do timeout")
}

type captureMediaClient struct {
	mu    sync.Mutex
	calls []media.PhotoDownloadRequest
	data  []byte
	err   error
}

func (c *captureMediaClient) DownloadPhoto(_ context.Context, req media.PhotoDownloadRequest) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls = append(c.calls, req)
	return c.data, c.err
}

func (c *captureMediaClient) callCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.calls)
}

func (c *captureMediaClient) firstCall() media.PhotoDownloadRequest {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.calls) == 0 {
		return media.PhotoDownloadRequest{}
	}
	return c.calls[0]
}

// captureImageCache satisfaz imageCache e registra cada Put por photo_id.
type captureImageCache struct {
	mu    sync.Mutex
	puts  map[int64][]byte
	pkeys []int64 // ordem de inserção
}

func (c *captureImageCache) Put(photoID int64, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.puts == nil {
		c.puts = map[int64][]byte{}
	}
	if _, ok := c.puts[photoID]; !ok {
		c.pkeys = append(c.pkeys, photoID)
	}
	c.puts[photoID] = data
}

func (c *captureImageCache) get(photoID int64) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	data, ok := c.puts[photoID]
	return data, ok
}

func (c *captureImageCache) putCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.pkeys)
}

// photoPayload monta um raw payload Shape A com uma foto MTProto canônica.
func photoPayload(t *testing.T, msgID, photoID int64) []byte {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"ID":      msgID,
		"Message": "produto com foto",
		"Media": map[string]any{
			"Photo": map[string]any{
				"ID":            photoID,
				"AccessHash":    int64(-7524180780117537531),
				"FileReference": "AlCZaRIAAXQAaiW+TljSnSegJJ4ybQVsJymwX1s=",
				"DCID":          int64(2),
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal photo payload: %v", err)
	}
	return b
}

// Regressão: com SetMediaDownload configurado, o flush dispara download proativo
// apenas para a mensagem com foto (não para a sem foto) e entrega os bytes ao
// cache compartilhado. Antes do contrato, o flush não baixava nada.
func TestFlush_ProactiveDownloadWhenMediaDownloadConfigured(t *testing.T) {
	const photoID int64 = 4985843018296396847

	photoMsg := photoPayload(t, 500, photoID)
	noPhotoMsg, _ := json.Marshal(map[string]any{"ID": int64(501), "Message": "texto sem foto"})

	mc := &captureMediaClient{data: []byte("full-res-img")}
	ic := &captureImageCache{}
	c := NewCollector(nil, &flushRepo{}, nil, logger.NopLogger{}, 10, 0, 100, 30)
	c.SetMediaDownload(mc, ic)

	now := time.Now().UTC()
	batch := []WriteJob{
		{Message: &model.RawMessage{ChannelID: 1, MessageID: 500, Payload: photoMsg, ReceivedAt: now}},
		{Message: &model.RawMessage{ChannelID: 1, MessageID: 501, Payload: noPhotoMsg, ReceivedAt: now}},
	}
	c.flush(context.Background(), batch)
	waitUntil(t, func() bool {
		got, ok := ic.get(photoID)
		return mc.callCount() == 1 && ok && string(got) == "full-res-img"
	})

	// Download disparado exatamente uma vez — apenas para a mensagem com foto.
	if mc.callCount() != 1 {
		t.Fatalf("DownloadPhoto chamado %d vezes; quer 1 (apenas a msg com foto)", mc.callCount())
	}
	if gotReq := mc.firstCall(); gotReq.PhotoID != photoID {
		t.Errorf("req.PhotoID = %d; quer %d", gotReq.PhotoID, photoID)
	}
	// Bytes baixados entregues ao cache compartilhado pela chave photo_id exata.
	got, ok := ic.get(photoID)
	if !ok || string(got) != "full-res-img" {
		t.Errorf("imageCache.Put(%d) = %q, %v; quer \"full-res-img\", true", photoID, got, ok)
	}
}

// Regressão: duplicatas não inseridas (ON CONFLICT) não disparam download
// proativo — não há razão para re-baixar uma imagem já capturada.
func TestFlush_ProactiveDownloadSkipsDuplicates(t *testing.T) {
	const photoID int64 = 4985843018296396847
	photoMsg := photoPayload(t, 500, photoID)

	mc := &captureMediaClient{data: []byte("img")}
	ic := &captureImageCache{}
	repo := &flushRepo{inserted: []bool{false}} // mensagem já existia (duplicata)
	c := NewCollector(nil, repo, nil, logger.NopLogger{}, 10, 0, 100, 30)
	c.SetMediaDownload(mc, ic)

	batch := []WriteJob{
		{Message: &model.RawMessage{ChannelID: 1, MessageID: 500, Payload: photoMsg, ReceivedAt: time.Now().UTC()}},
	}
	c.flush(context.Background(), batch)

	if mc.callCount() != 0 {
		t.Errorf("DownloadPhoto chamado %d vezes para duplicata; quer 0", mc.callCount())
	}
	if ic.putCount() != 0 {
		t.Errorf("imageCache recebeu %d puts para duplicata; quer 0", ic.putCount())
	}
}

// Complemento: sem SetMediaDownload (mediaClient/imageCache nil), o flush não
// panicar e ainda commita o lote. Defende o nil-guard no caminho de download.
func TestFlush_NoDownloadWhenMediaDownloadNotConfigured(t *testing.T) {
	const photoID int64 = 4985843018296396847
	photoMsg := photoPayload(t, 500, photoID)

	repo := &flushRepo{}
	c := NewCollector(nil, repo, nil, logger.NopLogger{}, 10, 0, 100, 30)
	// SetMediaDownload intencionalmente NÃO chamado: mediaClient/imageCache nil.

	batch := []WriteJob{
		{Message: &model.RawMessage{ChannelID: 1, MessageID: 500, Payload: photoMsg, ReceivedAt: time.Now().UTC()}},
	}
	c.flush(context.Background(), batch) // não deve panicar

	if repo.batchCalls != 1 {
		t.Errorf("SaveRawMessageBatch chamado %d vezes; quer 1 (lote commitado)", repo.batchCalls)
	}
}

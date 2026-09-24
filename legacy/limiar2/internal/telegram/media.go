package telegram

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"sync"

	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/media"
	"github.com/limiar/collector/internal/model"
)

// defaultThumbSize é a variante de tamanho da foto baixada: "x" = 800px (~50KB).
// Escolhida no MEDIA-TD — suficiente para o frontend sem full resolution.
const defaultThumbSize = "x"

const photoBatchWorkers = 4

// MediaClient implementa media.MediaClient sobre gotd/td. Vive em internal/telegram
// porque tipos do gotd não vazam daqui (AGENTS.md §14.2).
//
// Usado pelo Collector para download proativo de imagens no momento da chegada
// da mensagem (janela onde file_reference MTProto é válido).
type MediaClient struct {
	c   *Client
	log logger.Logger
	mu  sync.Mutex // serializa runOnce
}

// NewMediaClient envolve a facade telegram.Client para implementar media.MediaClient.
func NewMediaClient(c *Client, log logger.Logger) *MediaClient {
	if log == nil {
		log = logger.NopLogger{}
	}
	return &MediaClient{c: c, log: log}
}

// Compile-time: MediaClient satisfaz media.MediaClient.
var _ media.Client = (*MediaClient)(nil)

// DownloadPhoto baixa a variante defaultThumbSize da foto via Client.Download().
// Abre uma conexão Telegram (runOnce) por chamada. Para múltiplos downloads, use
// DownloadPhotoBatch (uma única conexão para todas).
func (m *MediaClient) DownloadPhoto(ctx context.Context, req media.PhotoDownloadRequest) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var data []byte
	err := m.c.runOnce(ctx, func(ctx context.Context) error {
		if err := m.c.peers.LoadFromDB(ctx); err != nil {
			m.log.Warn("media: falha ao carregar peers do DB", "erro", err)
		}
		var derr error
		data, derr = m.downloadOnce(ctx, req)
		return derr
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

// DownloadPhotoBatch baixa múltiplas fotos numa ÚNICA conexão Telegram.
// Usa poucas goroutines de I/O dentro dessa conexão para reduzir latência sem
// abrir várias sessões MTProto nem executar handlers em paralelo.
func (m *MediaClient) DownloadPhotoBatch(ctx context.Context, reqs []media.PhotoDownloadRequest, handler func(photoID int64, data []byte, err error)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.c.runOnce(ctx, func(ctx context.Context) error {
		// Carregar peers do DB para permitir renovação de file_reference
		// (re-busca de mensagens do canal via ChannelsGetMessages).
		if err := m.c.peers.LoadFromDB(ctx); err != nil {
			m.log.Warn("media: falha ao carregar peers do DB", "erro", err)
		}
		return runPhotoDownloadBatch(ctx, reqs, photoBatchWorkers, m.downloadOnce, handler)
	})
}

type photoBatchResult struct {
	photoID int64
	data    []byte
	err     error
}

func runPhotoDownloadBatch(
	ctx context.Context,
	reqs []media.PhotoDownloadRequest,
	workers int,
	download func(context.Context, media.PhotoDownloadRequest) ([]byte, error),
	handler func(photoID int64, data []byte, err error),
) error {
	if len(reqs) == 0 {
		return nil
	}
	if workers < 1 {
		workers = 1
	}
	if workers > len(reqs) {
		workers = len(reqs)
	}

	jobs := make(chan media.PhotoDownloadRequest)
	results := make(chan photoBatchResult, workers)
	var wg sync.WaitGroup
	wg.Add(workers)
	for range workers {
		go func() {
			defer wg.Done()
			for req := range jobs {
				data, err := download(ctx, req)
				result := photoBatchResult{photoID: req.PhotoID, data: data, err: err}
				select {
				case results <- result:
				case <-ctx.Done():
					return
				}
			}
		}()
	}

	go func() {
		defer close(jobs)
		for _, req := range reqs {
			select {
			case jobs <- req:
			case <-ctx.Done():
				return
			}
		}
	}()

	go func() {
		wg.Wait()
		close(results)
	}()

	for result := range results {
		handler(result.photoID, result.data, result.err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

// downloadOnce baixa uma foto dentro de uma conexão já estabelecida (runOnce loop).
// Lida com FILE_REFERENCE_EXPIRED: re-busca a mensagem do canal para renovar.
func (m *MediaClient) downloadOnce(ctx context.Context, req media.PhotoDownloadRequest) ([]byte, error) {
	fileRef := req.FileReference
	for attempt := range 2 {
		var buf bytes.Buffer
		loc := &tg.InputPhotoFileLocation{
			ID:            req.PhotoID,
			AccessHash:    req.AccessHash,
			FileReference: fileRef,
			ThumbSize:     defaultThumbSize,
		}
		_, derr := m.c.tg.Download(loc).Stream(ctx, &buf)
		if derr == nil {
			return buf.Bytes(), nil
		}
		if !tgerr.Is(derr, "FILE_REFERENCE_EXPIRED") || attempt > 0 {
			return nil, wrapFileErr(derr)
		}
		// FILE_REFERENCE_EXPIRED: renovar re-buscando a mensagem do canal.
		freshRef, rerr := m.renewFileReference(ctx, req.ChannelID, req.MessageID, req.PhotoID)
		if rerr != nil {
			m.log.Debug("media: renovação de file_reference falhou",
				"photo_id", req.PhotoID, "channel_id", req.ChannelID, "erro", rerr)
			return nil, wrapFileErr(derr)
		}
		fileRef = freshRef
		m.log.Debug("media: file_reference renovado", "photo_id", req.PhotoID)
	}
	return nil, nil
}

// renewFileReference re-busca uma mensagem do canal para obter um file_reference
// fresco para a foto. Usa ChannelsGetMessages via tg.Client.
func (m *MediaClient) renewFileReference(ctx context.Context, channelID, messageID, photoID int64) ([]byte, error) {
	if channelID == 0 || messageID == 0 {
		return nil, fmt.Errorf("channel_id ou message_id não fornecido")
	}
	peer, ok := m.c.peers.Get(channelID)
	if !ok {
		return nil, fmt.Errorf("peer do canal %d não encontrado no PeerStore", channelID)
	}
	api := m.c.tg.API()
	resp, err := api.ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
		Channel: &tg.InputChannel{
			ChannelID:  channelID,
			AccessHash: peer.AccessHash,
		},
		ID: []tg.InputMessageClass{&tg.InputMessageID{ID: int(messageID)}},
	})
	if err != nil {
		return nil, fmt.Errorf("channels.GetMessages: %w", err)
	}
	msgs, ok := resp.(*tg.MessagesChannelMessages)
	if !ok || len(msgs.Messages) == 0 {
		return nil, fmt.Errorf("resposta sem mensagens")
	}
	for _, msg := range msgs.Messages {
		mm, ok := msg.(*tg.Message)
		if !ok {
			continue
		}
		media, ok := mm.Media.(*tg.MessageMediaPhoto)
		if !ok {
			continue
		}
		photo, ok := media.Photo.(*tg.Photo)
		if !ok || photo.ID != photoID {
			continue
		}
		return photo.FileReference, nil
	}
	return nil, fmt.Errorf("foto %d não encontrada na mensagem %d", photoID, messageID)
}

// ExtractPhotoRequest extrai os campos MTProto de download do payload JSON de uma
// mensagem. Usado pelo Collector para download proativo no momento da chegada.
// Retorna (nil, nil) se a mensagem não tem foto. Delega a navegação do payload
// para model.ExtractPhotoFields (função compartilhada com processor/normalizer.go).
func ExtractPhotoRequest(payload []byte) (*media.PhotoDownloadRequest, error) {
	msg, err := model.DecodePayloadMap(payload)
	if err != nil {
		return nil, apperrors.Wrap("telegram", "parse_payload", err)
	}
	pf := model.ExtractPhotoFields(msg)
	if pf.PhotoID == 0 {
		return nil, nil
	}

	var refBytes []byte
	if pf.FileRef != "" {
		decoded, err := base64.StdEncoding.DecodeString(pf.FileRef)
		if err == nil {
			refBytes = decoded
		}
	}

	return &media.PhotoDownloadRequest{
		PhotoID:       pf.PhotoID,
		AccessHash:    pf.AccessHash,
		FileReference: refBytes,
		DCID:          pf.DCID,
	}, nil
}

// wrapFileErr traduz erros gotd de download. FILE_REFERENCE_EXPIRED vira o sentinel
// apperrors.ErrFileReferenceExpired (envolvido) para o resolver detectar.
func wrapFileErr(err error) error {
	if err == nil {
		return nil
	}
	if tgerr.Is(err, "FILE_REFERENCE_EXPIRED") {
		return apperrors.Wrap("telegram", "get_file", apperrors.ErrFileReferenceExpired)
	}
	return apperrors.Wrap("telegram", "get_file", err)
}

package telegram

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
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
// Usa a infraestrutura completa de download do gotd (DC transfers, CDN).
// Se o file_reference estiver expirado, retenta sem ele.
func (m *MediaClient) DownloadPhoto(ctx context.Context, req media.PhotoDownloadRequest) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var data []byte
	err := m.c.runOnce(ctx, func(ctx context.Context) error {
		var buf bytes.Buffer
		loc := &tg.InputPhotoFileLocation{
			ID:            req.PhotoID,
			AccessHash:    req.AccessHash,
			FileReference: req.FileReference,
			ThumbSize:     defaultThumbSize,
		}
		_, derr := m.c.tg.Download(loc).Stream(ctx, &buf)
		if derr != nil && tgerr.Is(derr, "FILE_REFERENCE_EXPIRED") {
			buf.Reset()
			loc.FileReference = nil
			_, derr = m.c.tg.Download(loc).Stream(ctx, &buf)
		}
		if derr != nil {
			return wrapFileErr(derr)
		}
		data = buf.Bytes()
		return nil
	})
	if err != nil {
		return nil, err
	}
	return data, nil
}

// ExtractPhotoRequest extrai os campos MTProto de download do payload JSON de uma
// mensagem. Usado pelo Collector para download proativo no momento da chegada.
// Retorna (nil, nil) se a mensagem não tem foto. Delega a navegação do payload
// para model.ExtractPhotoFields (função compartilhada com processor/normalizer.go).
func ExtractPhotoRequest(payload []byte) (*media.PhotoDownloadRequest, error) {
	var msg map[string]any
	if err := json.Unmarshal(payload, &msg); err != nil {
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

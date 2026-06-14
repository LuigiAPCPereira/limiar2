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
var _ media.MediaClient = (*MediaClient)(nil)

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
// Retorna (nil, nil) se a mensagem não tem foto.
func ExtractPhotoRequest(payload []byte) (*media.PhotoDownloadRequest, error) {
	var msg map[string]any
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil, apperrors.Wrap("telegram", "parse_payload", err)
	}
	m, ok := msg["Media"].(map[string]any)
	if !ok {
		return nil, nil
	}
	photo, ok := m["Photo"].(map[string]any)
	if !ok {
		return nil, nil
	}
	photoID := payloadToInt64(photo["ID"])
	accessHash := payloadToInt64(photo["AccessHash"])
	fileRef, _ := photo["FileReference"].(string)
	dcid := payloadToInt(photo["DCID"])

	var refBytes []byte
	if fileRef != "" {
		decoded, err := base64.StdEncoding.DecodeString(fileRef)
		if err == nil {
			refBytes = decoded
		}
	}

	return &media.PhotoDownloadRequest{
		PhotoID:       photoID,
		AccessHash:    accessHash,
		FileReference: refBytes,
		DCID:          dcid,
	}, nil
}

func payloadToInt64(v any) int64 {
	if n, ok := v.(float64); ok {
		return int64(n)
	}
	return 0
}

func payloadToInt(v any) int {
	if n, ok := v.(float64); ok {
		return int(n)
	}
	return 0
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

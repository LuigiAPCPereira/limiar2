package telegram

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gotd/td/telegram/downloader"
	"github.com/gotd/td/tg"
	"github.com/gotd/td/tgerr"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/media"
	"github.com/limiar/collector/internal/storage"
)

// defaultThumbSize é a variante de tamanho da foto baixada: "x" = 800px (~50KB).
// Escolhida no MEDIA-TD §8 — suficiente para o scroll infinito do frontend, sem
// full resolution. Quase toda foto de mensagem possui a variante "x".
const defaultThumbSize = "x"

// MediaClient implementa media.MediaClient sobre gotd/td. Vive em internal/telegram
// porque tipos do gotd não vazam daqui (AGENTS.md §14.2).
//
// Cada chamada abre uma conexão MTProto nova via runOnce — apropriado para o
// comando CLI one-shot `media resolve`. Para a Wave 4 (HTTP concorrente dentro do
// `limiar run`), o resolver deverá usar a sessão persistente do collector
// (ADR 012, Opção A): runOnce muta o campo c.tg da facade, o que quebra sob
// chamadas concorrentes. O mutex serializa as chamadas como proteção até lá.
type MediaClient struct {
	c   *Client
	log logger.Logger
	mu  sync.Mutex // serializa runOnce; remove na Wave 4 com sessão persistente
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

// DownloadPhoto baixa a variante defaultThumbSize da foto via upload.GetFile.
// O downloader do gotd cuida da transferência de DC automaticamente.
func (m *MediaClient) DownloadPhoto(ctx context.Context, req media.PhotoDownloadRequest) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var data []byte
	err := m.c.runOnce(ctx, func(ctx context.Context) error {
		loc := &tg.InputPhotoFileLocation{
			ID:            req.PhotoID,
			AccessHash:    req.AccessHash,
			FileReference: req.FileReference,
			ThumbSize:     defaultThumbSize,
		}
		var buf bytes.Buffer
		if _, derr := downloader.NewDownloader().Download(m.c.tg.API(), loc).Stream(ctx, &buf); derr != nil {
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

// RenewFileReference re-busca a mensagem via messages.getMessages (L3 soft) e extrai
// os metadados MTProto atualizados. Para mensagens de canal o getMessages pode não
// retornar; o resolver então cai para RefetchFromChannel (L3 hard), que usa o
// histórico do canal — caminho confiável para canais.
func (m *MediaClient) RenewFileReference(ctx context.Context, channelID, msgID int64) (*storage.PhotoMetadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var meta *storage.PhotoMetadata
	err := m.c.runOnce(ctx, func(ctx context.Context) error {
		resp, err := m.c.tg.API().MessagesGetMessages(ctx,
			[]tg.InputMessageClass{&tg.InputMessageID{ID: int(msgID)}})
		if err != nil {
			return wrapFileErr(err)
		}
		msgs, err := extractMessages(resp)
		if err != nil {
			return apperrors.Wrap("telegram", "extract_renew", err)
		}
		if len(msgs) == 0 {
			return apperrors.Wrap("telegram", "renew", fmt.Errorf("mensagem %d não encontrada via getMessages", msgID))
		}
		meta, err = photoMetaFromPayload(msgs[0].Payload, channelID, msgID)
		return err
	})
	if err != nil {
		return nil, err
	}
	return meta, nil
}

// RefetchFromChannel re-coleta a mensagem do canal via messages.getHistory
// (L3 hard) e extrai os metadados atualizados. FetchHistoryWithOffset já abre sua
// própria conexão (runOnce interno) — não a envolvemos em outra.
// Carrega os peers do banco primeiro — necessário para o caso CLI one-shot onde
// o Client é criado sem LoadPeers (o collector faz isso em Run, mas aqui não).
func (m *MediaClient) RefetchFromChannel(ctx context.Context, channelID, msgID int64) (*storage.PhotoMetadata, error) {
	if err := m.c.LoadPeers(ctx); err != nil {
		return nil, apperrors.Wrap("telegram", "load_peers", err)
	}
	msgs, err := m.c.FetchHistoryWithOffset(ctx, channelID, msgID, 1)
	if err != nil {
		return nil, apperrors.Wrap("telegram", "refetch", err)
	}
	if len(msgs) == 0 {
		return nil, apperrors.Wrap("telegram", "refetch",
			fmt.Errorf("mensagem %d não encontrada no canal %d", msgID, channelID))
	}
	return photoMetaFromPayload(msgs[0].Payload, channelID, msgID)
}

// photoMetaFromPayload extrai os campos MTProto da foto do payload JSON de uma
// mensagem. Espelha internal/processor.extractPhotoMetadata — não podemos importar
// o processor (telegram é Fase 1; a dep. seria reversa). FileReference já vem
// base64 no JSON gerado pelo gotd, alinhado ao que o processor persiste.
func photoMetaFromPayload(payload []byte, channelID, msgID int64) (*storage.PhotoMetadata, error) {
	var msg map[string]any
	if err := json.Unmarshal(payload, &msg); err != nil {
		return nil, apperrors.Wrap("telegram", "parse_payload", err)
	}
	m, ok := msg["Media"].(map[string]any)
	if !ok {
		return nil, apperrors.Wrap("telegram", "photo_meta", fmt.Errorf("mensagem %d sem Media", msgID))
	}
	photo, ok := m["Photo"].(map[string]any)
	if !ok {
		return nil, apperrors.Wrap("telegram", "photo_meta", fmt.Errorf("mensagem %d sem Photo", msgID))
	}
	return &storage.PhotoMetadata{
		MsgID:         msgID,
		ChannelID:     channelID,
		PhotoID:       payloadToInt64(photo["ID"]),
		AccessHash:    payloadToInt64(photo["AccessHash"]),
		FileReference: payloadString(photo["FileReference"]),
		DCID:          payloadToInt(photo["DCID"]),
	}, nil
}

// wrapFileErr traduz erros gotd de download. FILE_REFERENCE_EXPIRED vira o sentinel
// apperrors.ErrFileReferenceExpired (envolvido) para o resolver detectar L3.
func wrapFileErr(err error) error {
	if err == nil {
		return nil
	}
	if tgerr.Is(err, "FILE_REFERENCE_EXPIRED") {
		return apperrors.Wrap("telegram", "get_file", apperrors.ErrFileReferenceExpired)
	}
	return apperrors.Wrap("telegram", "get_file", err)
}

// payloadToInt64 converte um valor JSON (float64) para int64. Duplica o padrão do
// processor localmente (telegram não importa processor).
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

func payloadString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

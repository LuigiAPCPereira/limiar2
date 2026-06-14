package telegram

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"sync"

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

// RenewFileReference re-busca a mensagem via messages.getMessages (L3 soft) e extrai
// os metadados MTProto atualizados. Usa InputMessageChannelMessageID (com channel_id
// + access_hash) porque InputMessageID não funciona para mensagens de canal.
func (m *MediaClient) RenewFileReference(ctx context.Context, channelID, msgID int64) (*storage.PhotoMetadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.c.LoadPeers(ctx); err != nil {
		return nil, apperrors.Wrap("telegram", "load_peers", err)
	}
	peer, ok := m.c.peers.Get(channelID)
	if !ok {
		return nil, apperrors.Wrap("telegram", "renew",
			fmt.Errorf("peer not found for channel %d", channelID))
	}

	var meta *storage.PhotoMetadata
	err := m.c.runOnce(ctx, func(ctx context.Context) error {
		resp, err := m.c.tg.API().ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: channelID, AccessHash: peer.AccessHash},
			ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: int(msgID)}},
		})
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

// RefetchAndDownload re-coleta a mensagem e baixa a imagem na MESMA sessão
// MTProto (runOnce). Usa MessagesGetMessages com InputMessageChannelMessageID
// para buscar a mensagem exata (não MessagesGetHistory que retorna mensagens
// ANTERIORES ao OffsetID). O file_reference fresco é usado imediatamente para
// download na mesma sessão, evitando FILE_REFERENCE_EXPIRED.
func (m *MediaClient) RefetchAndDownload(ctx context.Context, channelID, msgID int64) ([]byte, *storage.PhotoMetadata, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.c.LoadPeers(ctx); err != nil {
		return nil, nil, apperrors.Wrap("telegram", "load_peers", err)
	}
	peer, ok := m.c.peers.Get(channelID)
	if !ok {
		return nil, nil, apperrors.Wrap("telegram", "refetch_download",
			fmt.Errorf("peer not found for channel %d", channelID))
	}

	var data []byte
	var meta *storage.PhotoMetadata

	err := m.c.runOnce(ctx, func(ctx context.Context) error {
		// Step 1: channels.getMessages com InputChannel + InputMessageID para
		// buscar a mensagem EXATA com file_reference fresco.
		resp, err := m.c.tg.API().ChannelsGetMessages(ctx, &tg.ChannelsGetMessagesRequest{
			Channel: &tg.InputChannel{ChannelID: channelID, AccessHash: peer.AccessHash},
			ID:      []tg.InputMessageClass{&tg.InputMessageID{ID: int(msgID)}},
		})
		if err != nil {
			return apperrors.Wrap("telegram", "get_messages", err)
		}
		msgs, err := extractMessages(resp)
		if err != nil {
			return apperrors.Wrap("telegram", "extract_messages", err)
		}
		if len(msgs) == 0 {
			return apperrors.Wrap("telegram", "refetch_download",
				fmt.Errorf("mensagem %d não encontrada no canal %d", msgID, channelID))
		}

		// Step 2: extrair metadados da foto do payload.
		meta, err = photoMetaFromPayload(msgs[0].Payload, channelID, msgID)
		if err != nil {
			return err
		}
		m.log.Debug("RefetchAndDownload: metadados obtidos",
			"photo_id", meta.PhotoID, "access_hash", meta.AccessHash,
			"file_ref_len", len(meta.FileReference), "dcid", meta.DCID)

		// Step 3: download via gotd Client.Download() que usa a infraestrutura
		// completa de download (DC transfers, CDN). O método anterior usava
		// API().UploadGetFile() diretamente, que NÃO lida com FILE_MIGRATE
		// e resulta em FILE_REFERENCE_EXPIRED.
		ref, derr := decodeFileRef(meta.FileReference)
		if derr != nil {
			return apperrors.Wrap("telegram", "decode_fileref", derr)
		}

		var buf bytes.Buffer
		for _, thumbSize := range []string{"x", "m", "y"} {
			loc := &tg.InputPhotoFileLocation{
				ID:            meta.PhotoID,
				AccessHash:    meta.AccessHash,
				FileReference: ref,
				ThumbSize:     thumbSize,
			}
			buf.Reset()
			m.log.Debug("RefetchAndDownload: tentando download",
				"thumb_size", thumbSize, "photo_id", meta.PhotoID)

			_, ferr := m.c.tg.Download(loc).Stream(ctx, &buf)
			if ferr != nil {
				m.log.Debug("RefetchAndDownload: falhou",
					"thumb_size", thumbSize, "err", fmt.Sprintf("%v", ferr))
				continue
			}
			if buf.Len() > 0 {
				m.log.Debug("RefetchAndDownload: sucesso",
					"thumb_size", thumbSize, "bytes", buf.Len())
				data = buf.Bytes()
				return nil
			}
		}
		return apperrors.Wrap("telegram", "download_all_sizes",
			fmt.Errorf("todos os tamanhos falharam para photo_id %d", meta.PhotoID))
	})
	if err != nil {
		return nil, nil, err
	}
	return data, meta, nil
}

// decodeFileRef decodifica o file_reference base64 para bytes.
func decodeFileRef(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(s)
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

// rePhotoMsg faz match em widgets de mensagem do Telegram web com fotos.
// Captura data-post="channel/MSGID" e depois procura por background-image ou img src
// com URL do CDN telesco.pe dentro dos próximos 3000 caracteres.
var rePhotoMsg = regexp.MustCompile(
	`data-post="[^"]+/(\d+)"`,
)

// reTelescopeURL extrai URLs do CDN do Telegram (telesco.pe) de um trecho de HTML.
var reTelescopeURL = regexp.MustCompile(
	`(?:background-image:\s*url\('|<img[^>]+src=")(https://cdn\d*\.telesco\.pe/file/[^"')]+)`,
)

// ScrapePhotoURL faz scraping da página web pública do canal (t.me/s/username)
// para encontrar a URL CDN da foto da mensagem com o msgID informado.
// Abordagem alternativa ao MTProto upload.GetFile que evita FILE_REFERENCE_EXPIRED.
func (m *MediaClient) ScrapePhotoURL(ctx context.Context, username string, msgID int64) (string, error) {
	url := fmt.Sprintf("https://t.me/s/%s", username)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", apperrors.Wrap("telegram", "scrape_request", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Limiar/1.0)")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", apperrors.Wrap("telegram", "scrape_fetch", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", apperrors.Wrap("telegram", "scrape_fetch",
			fmt.Errorf("HTTP %d para @%s", resp.StatusCode, username))
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 5*1024*1024)) // max 5MB
	if err != nil {
		return "", apperrors.Wrap("telegram", "scrape_read", err)
	}

	targetID := strconv.FormatInt(msgID, 10)
	matches := rePhotoMsg.FindAllStringSubmatchIndex(string(body), -1)
	for _, match := range matches {
		if len(match) < 4 {
			continue
		}
		// match[2]:match[3] é a posição do grupo capturado (msg ID)
		foundID := string(body[match[2]:match[3]])
		if foundID != targetID {
			continue
		}
		// Procura URL do CDN nos próximos 3000 chars após o data-post
		end := match[1] + 3000
		if end > len(body) {
			end = len(body)
		}
		chunk := string(body[match[1]:end])
		urlMatch := reTelescopeURL.FindStringSubmatch(chunk)
		if len(urlMatch) >= 2 {
			photoURL := urlMatch[1]
			m.log.Debug("ScrapePhotoURL: foto encontrada",
				"username", username, "msg_id", msgID, "url_len", len(photoURL))
			return photoURL, nil
		}
	}
	return "", apperrors.Wrap("telegram", "scrape_photo",
		fmt.Errorf("msg %d não encontrada na página de @%s", msgID, username))
}

// DownloadHTTP baixa bytes de uma URL via HTTP GET. Usado pelo resolver para
// baixar imagens do CDN do Telegram após scraping (sem MTProto, sem file_reference).
func (m *MediaClient) DownloadHTTP(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, apperrors.Wrap("telegram", "http_request", err)
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Limiar/1.0)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, apperrors.Wrap("telegram", "http_fetch", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, apperrors.Wrap("telegram", "http_fetch",
			fmt.Errorf("HTTP %d", resp.StatusCode))
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		return nil, apperrors.Wrap("telegram", "http_read", err)
	}
	return data, nil
}

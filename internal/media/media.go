// Package media implementa o subsistema de resolução de imagens via proxy MTProto
// (ADR 011). O MediaResolver baixa fotos sob demanda usando os campos MTProto
// persistidos pelo processor, com cache in-memory (LRU + TTL) e dedup de requests
// concorrentes via singleflight.
//
// Este pacote NÃO importa gotd/td: a implementação concreta de MediaClient sobre
// gotd vive em internal/telegram (AGENTS.md §14.2 — tipos do gotd não vazam para
// fora de internal/telegram). Aqui ficam apenas as interfaces e a lógica de
// orquestração (cache, singleflight, renovação L1/L2/L3), totalmente testável
// com mocks.
package media

import (
	"context"
	"encoding/base64"

	"github.com/limiar/collector/internal/storage"
)

// PhotoDownloadRequest carrega os campos MTProto necessários para uma chamada
// upload.GetFile. FileReference é o binário decodificado do base64 persistido.
type PhotoDownloadRequest struct {
	PhotoID       int64
	AccessHash    int64
	FileReference []byte
	DCID          int
}

// MediaClient abstrai as chamadas MTProto necessárias à resolução de imagens.
// A implementação concreta sobre gotd/td vive em internal/telegram.
//
// Contrato de erro: DownloadPhoto deve envolver apperrors.ErrFileReferenceExpired
// (via apperrors.Wrap) quando o Telegram retornar FILE_REFERENCE_EXPIRED, para
// que o resolver detecte a condição L3 com errors.Is.
type MediaClient interface {
	// DownloadPhoto baixa os bytes da imagem via upload.GetFile.
	DownloadPhoto(ctx context.Context, req PhotoDownloadRequest) ([]byte, error)
	// RenewFileReference renova o file_reference via messages.getMessages (L3 soft).
	RenewFileReference(ctx context.Context, channelID, msgID int64) (*storage.PhotoMetadata, error)
	// RefetchFromChannel re-coleta a mensagem do canal para renovar todos os
	// campos MTProto (L3 hard).
	RefetchFromChannel(ctx context.Context, channelID, msgID int64) (*storage.PhotoMetadata, error)
	// RefetchAndDownload re-coleta a mensagem e baixa a imagem na MESMA sessão
	// MTProto. O file_reference do MTProto é vinculado à sessão — fetch e download
	// em sessões diferentes resulta em FILE_REFERENCE_EXPIRED. Este método resolve
	// isso fazendo ambas as operações num único runOnce (L3 hard com download).
	RefetchAndDownload(ctx context.Context, channelID, msgID int64) ([]byte, *storage.PhotoMetadata, error)
	// ScrapePhotoURL faz scraping da página web pública do canal (t.me/s/username)
	// para encontrar a URL CDN da foto de uma mensagem específica. Retorna a URL
	// HTTP que pode ser baixada sem file_reference MTProto. Retorna erro se o
	// canal não for público ou a mensagem não for encontrada na página.
	ScrapePhotoURL(ctx context.Context, username string, msgID int64) (string, error)
	// DownloadHTTP baixa bytes de uma URL via HTTP GET. Usado para baixar
	// imagens do CDN do Telegram após scraping (sem MTProto).
	DownloadHTTP(ctx context.Context, url string) ([]byte, error)
}

// MediaRepository abstrai o acesso ao banco para metadados de foto. É satisfeita
// por *storage.Repository (satisfação implícita — mesmas assinaturas), permitindo
// testes do resolver com um mock sem acionar o banco.
type MediaRepository interface {
	GetPhotoMetadata(ctx context.Context, processedMsgID int64) (*storage.PhotoMetadata, error)
	UpdateFileReference(ctx context.Context, processedMsgID int64, fileRef string) error
	UpdatePhotoMetadata(ctx context.Context, processedMsgID int64, meta *storage.PhotoMetadata) error
	GetChannelUsername(ctx context.Context, channelID int64) (string, error)
}

// downloadRequestFromMeta monta a requisição de download a partir dos metadados
// persistidos, decodificando o file_reference base64.
func downloadRequestFromMeta(m *storage.PhotoMetadata) (PhotoDownloadRequest, error) {
	ref, err := decodeBase64(m.FileReference)
	if err != nil {
		return PhotoDownloadRequest{}, err
	}
	return PhotoDownloadRequest{
		PhotoID:       m.PhotoID,
		AccessHash:    m.AccessHash,
		FileReference: ref,
		DCID:          m.DCID,
	}, nil
}

// decodeBase64 decodifica o file_reference persistido. Retorna slice vazio (sem
// erro) para string vazia — foto sem file_reference ainda é baixável uma vez.
func decodeBase64(s string) ([]byte, error) {
	if s == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(s)
}

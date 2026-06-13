package media

import (
	"context"
	"errors"
	"strconv"

	"golang.org/x/sync/singleflight"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/storage"
)

// MediaResolver baixa imagens via MTProto com cache in-memory (L1) e dedup de
// requests concorrentes via singleflight. É a peça central do subsistema de
// imagens (ADR 011), vivendo no orquestrador limiar run onde compartilha a
// sessão MTProto do collector.
type MediaResolver struct {
	client MediaClient
	repo   MediaRepository
	cache  *ImageCache
	log    logger.Logger
	group  singleflight.Group
}

// NewMediaResolver constrói o resolver com suas dependências. O cache é criado
// pelo chamador (permite configurar maxLen/TTL). log nil vira NopLogger.
func NewMediaResolver(client MediaClient, repo MediaRepository, cache *ImageCache, log logger.Logger) *MediaResolver {
	if log == nil {
		log = logger.NopLogger{}
	}
	return &MediaResolver{client: client, repo: repo, cache: cache, log: log}
}

// ResolveImage retorna os bytes da foto da mensagem processada informada.
// Fluxo (ADR 011): L1 cache → L2 download MTProto → L3 renovação de file_reference.
//
// Singleflight: requests concorrentes para a MESMA foto (chave = photo_id, não
// msg_id, pois a mesma foto pode aparecer em múltiplas mensagens por forward)
// compartilham um único download. Sem isso, dois usuários pedindo a mesma imagem
// disparam dois upload.GetFile — flood MTProto e desperdício de banda. É
// corretude em produção, não apenas otimização.
//
// Nota sobre contexto: o closure executado pelo singleflight captura o ctx do
// chamador líder (o que de fato dispara o download). O cancelamento do líder
// aborta o download compartilhado e os seguidores recebem o mesmo erro —
// limitação aceitável para o MVP.
func (r *MediaResolver) ResolveImage(ctx context.Context, processedMsgID int64) ([]byte, error) {
	// A leitura de metadados fica FORA do singleflight: é uma leitura indexada
	// barata no banco. Só a parte cara (download MTProto) é deduplicada.
	meta, err := r.repo.GetPhotoMetadata(ctx, processedMsgID)
	if err != nil {
		return nil, apperrors.Wrap("media", "get_metadata", err)
	}
	if meta.PhotoID == 0 {
		return nil, apperrors.ErrNoPhoto
	}

	key := strconv.FormatInt(meta.PhotoID, 10)
	v, err, _ := r.group.Do(key, func() (any, error) {
		return r.resolve(ctx, meta)
	})
	if err != nil {
		return nil, err
	}
	return v.([]byte), nil
}

// resolve executa o fluxo L1→L2→L3 para uma foto cujos metadados já foram
// lidos. Roda dentro do singleflight (uma execução por photo_id concorrente).
func (r *MediaResolver) resolve(ctx context.Context, meta *storage.PhotoMetadata) ([]byte, error) {
	// L1: cache.
	if data, ok := r.cache.Get(meta.PhotoID); ok {
		return data, nil
	}

	// L2: download MTProto.
	req, err := downloadRequestFromMeta(meta)
	if err != nil {
		return nil, apperrors.Wrap("media", "decode_fileref", err)
	}
	data, err := r.client.DownloadPhoto(ctx, req)
	if err == nil {
		r.cache.Put(meta.PhotoID, data)
		return data, nil
	}

	// L3: file_reference expirado? renova e retenta.
	if errors.Is(err, apperrors.ErrFileReferenceExpired) {
		return r.renewAndRetry(ctx, meta)
	}
	return nil, apperrors.Wrap("media", "download", err)
}

// renewAndRetry implementa a renovação L3 em dois níveis (ADR 011):
//
//	L3 soft: messages.getMessages → atualiza file_reference no DB → retry.
//	L3 hard: FetchHistory(1) → atualiza TODOS os campos no DB → retry final.
//
// A persistência da renovação é best-effort: uma falha ao gravar o novo
// file_reference não impede o retry imediato (a memória do processo já tem o
// valor renovado), apenas faria a próxima invocação pós-restart repetir a
// renovação. Por isso o erro é logado, não propagado.
func (r *MediaResolver) renewAndRetry(ctx context.Context, meta *storage.PhotoMetadata) ([]byte, error) {
	// L3 soft.
	renewed, softErr := r.client.RenewFileReference(ctx, meta.ChannelID, meta.MsgID)
	if softErr == nil {
		if perr := r.repo.UpdateFileReference(ctx, meta.ID, renewed.FileReference); perr != nil {
			r.log.Warn("falha ao persistir file_reference renovado (não fatal)",
				"msg_id", meta.MsgID, "erro", perr)
		}
		if data, derr := r.tryDownload(ctx, renewed); derr == nil {
			return data, nil
		}
	}

	// L3 hard — log do motivo do soft ter falhado para debugging em produção.
	r.log.Debug("L3 soft falhou, escalando para hard",
		"msg_id", meta.MsgID, "canal_id", meta.ChannelID, "erro_soft", softErr)
	refetched, err := r.client.RefetchFromChannel(ctx, meta.ChannelID, meta.MsgID)
	if err != nil {
		return nil, apperrors.Wrap("media", "hard_renew", err)
	}
	if perr := r.repo.UpdatePhotoMetadata(ctx, meta.ID, refetched); perr != nil {
		r.log.Warn("falha ao persistir metadados renovados (não fatal)",
			"msg_id", meta.MsgID, "erro", perr)
	}
	data, err := r.tryDownload(ctx, refetched)
	if err != nil {
		return nil, apperrors.Wrap("media", "retry_download", err)
	}
	return data, nil
}

// tryDownload monta a requisição a partir dos metadados (possivelmente renovados),
// baixa e popula o cache. Retorna (data, nil) em sucesso; caso contrário o erro
// sinaliza falha intermediária — o chamador decide entre escalar (L3 hard) ou
// envolver.
func (r *MediaResolver) tryDownload(ctx context.Context, meta *storage.PhotoMetadata) ([]byte, error) {
	req, err := downloadRequestFromMeta(meta)
	if err != nil {
		return nil, apperrors.Wrap("media", "decode_fileref", err)
	}
	data, err := r.client.DownloadPhoto(ctx, req)
	if err != nil {
		return nil, err
	}
	r.cache.Put(meta.PhotoID, data)
	return data, nil
}

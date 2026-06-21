package telegram

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"

	apperrors "github.com/limiar/collector/internal/errors"
	"github.com/limiar/collector/internal/logger"
	"github.com/limiar/collector/internal/model"
)

// TelegramClient é a fachada (Facade) que esconde toda a complexidade do gotd/td (sessão, peers,
// reconexão, auth) por trás de uma interface pequena. Nenhum tipo do gotd/td aparece aqui,
// para que as camadas CLI e collector nunca importem o gotd/td.
type TelegramClient interface {
	// Auth executa o fluxo interativo de autenticação, se ainda não estiver autorizado.
	Auth(ctx context.Context) error
	// IsAuthenticated relata se uma sessão válida está presente.
	IsAuthenticated(ctx context.Context) (bool, error)
	// LoadPeers carrega peers persistidos do storage para o cache em memória.
	LoadPeers(ctx context.Context) error
	// AddUpdateHandler registra um observador para atualizações recebidas.
	AddUpdateHandler(h UpdateHandler)
	// ResolveChannel resolve um @username para um Peer com o seu hash de acesso e
	// o persiste no armazenamento de peers.
	ResolveChannel(ctx context.Context, username string) (*model.Peer, error)
	// ResolveChannelChecked verifica a autenticação e resolve um canal em
	// um único ciclo de vida de conexão. Retorna ErrNotAuthenticated se a
	// sessão não estiver autorizada.
	ResolveChannelChecked(ctx context.Context, username string) (*model.Peer, error)
	// FetchHistory retorna até `limit` payloads brutos de mensagens recentes (as mais novas
	// primeiro) para o canal, cada uma já serializada para JSON. O hash de acesso
	// é buscado no armazenamento de peers; se o peer for desconhecido, ele retorna um
	// erro. minID filtra mensagens com id <= minID (0 significa sem filtro).
	FetchHistory(ctx context.Context, channelID int64, minID int64, limit int) ([]HistoryMessage, error)
	// FetchHistoryWithOffset retorna até `limit` payloads brutos de mensagens com
	// id < offsetID para o canal (as mais novas primeiro quando offsetID == 0). É a
	// primitiva de paginação usada pelo backfill retroativo do collector:
	// cada chamada subsequente usa offsetID = min(id) da página anterior.
	FetchHistoryWithOffset(ctx context.Context, channelID int64, offsetID int64, limit int) ([]HistoryMessage, error)
	// Run conecta e bloqueia, despachando atualizações até que ctx seja cancelado.
	Run(ctx context.Context) error
}

// HistoryMessage é uma única mensagem histórica capturada durante o backfill: seu
// id de mensagem do Telegram, o timestamp de envio (Date), e o payload JSON bruto.
// Date é usado pelo collector para parar de paginar quando as mensagens são mais
// velhas que o limite temporal configurado (LIMIAR_HISTORY_MAX_DAYS).
type HistoryMessage struct {
	MessageID int64
	Date      time.Time
	Payload   []byte
}

// BackoffConfig parametriza o backoff exponencial de reconexão.
type BackoffConfig struct {
	BaseDelay     time.Duration
	Multiplier    float64
	JitterPercent float64
	Ceiling       time.Duration
	MaxRetries    int
}

// DefaultBackoff retorna a política de reconexão da especificação: 1s base, 2x, 10%
// de jitter, teto de 5m.
func DefaultBackoff(maxRetries int) BackoffConfig {
	return BackoffConfig{
		BaseDelay:     time.Second,
		Multiplier:    2.0,
		JitterPercent: 0.10,
		Ceiling:       5 * time.Minute,
		MaxRetries:    maxRetries,
	}
}

// CalculateBackoff retorna o atraso (delay) para uma dada tentativa, ou
// ErrMaxRetriesExceeded quando as tentativas se esgotam.
func CalculateBackoff(attempt int, cfg BackoffConfig, rng *rand.Rand) (time.Duration, error) {
	if attempt >= cfg.MaxRetries {
		return 0, apperrors.ErrMaxRetriesExceeded
	}
	delay := float64(cfg.BaseDelay) * math.Pow(cfg.Multiplier, float64(attempt))
	if delay > float64(cfg.Ceiling) {
		delay = float64(cfg.Ceiling)
	}
	jitter := delay * cfg.JitterPercent
	delay += (rng.Float64()*2 - 1) * jitter
	if delay < 0 {
		delay = 0
	}
	return time.Duration(delay), nil
}

// Client é a única implementação de TelegramClient e o único tipo que
// importa gotd/td. Ele possui o client gotd, o armazenamento de sessão, o cache de peers,
// e o dispatcher.
type Client struct {
	appID      int
	appHash    string
	ioTimeout  time.Duration
	backoff    BackoffConfig
	session    *TursoSessionStorage
	peers      *PeerStore
	dispatcher *Dispatcher
	log        logger.Logger
	rng        *rand.Rand

	tg *telegram.Client
}

var _ TelegramClient = (*Client)(nil)

// NewClient constrói a fachada (facade). O client gotd é criado de forma preguiçosa (lazily)
// a cada conexão para que a mesma instância possa ser reutilizada após a desconexão.
func NewClient(
	appID int,
	appHash string,
	ioTimeout time.Duration,
	backoff BackoffConfig,
	session *TursoSessionStorage,
	peers *PeerStore,
	dispatcher *Dispatcher,
	log logger.Logger,
) *Client {
	if log == nil {
		log = logger.NopLogger{}
	}
	c := &Client{
		appID:      appID,
		appHash:    appHash,
		ioTimeout:  ioTimeout,
		backoff:    backoff,
		session:    session,
		peers:      peers,
		dispatcher: dispatcher,
		log:        log,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())), // #nosec G404 — jitter, not crypto
	}
	return c
}

// newGotdClient cria um novo client gotd usando a sessão e o dispatcher atuais.
// Cada chamada retorna uma nova instância porque clients gotd não podem ser
// reutilizados após o retorno de Run.
func (c *Client) newGotdClient() *telegram.Client {
	return telegram.NewClient(c.appID, c.appHash, telegram.Options{
		SessionStorage: c.session,
		UpdateHandler:  telegram.UpdateHandlerFunc(c.onUpdate),
	})
}

// AddUpdateHandler registra um observador no dispatcher.
func (c *Client) AddUpdateHandler(h UpdateHandler) {
	c.dispatcher.Register(h)
}

// onUpdate é o ponto de entrada (entrypoint) do gotd UpdateHandler: ele extrai metadados
// de roteamento e encaminha a atualização (com metadados) ao dispatcher para fan-out.
func (c *Client) onUpdate(ctx context.Context, u tg.UpdatesClass) error {
	// Early return no hot path. Extrai o metadata ANTES de
	// codificar o payload. Evita alocações pesadas do json.Marshal
	// para updates que não contém mensagens (ex: status de usuário).
	channelID, messageID, ok := extractUpdateMeta(u)
	if !ok {
		return nil
	}

	payload, err := encodeUpdate(u)
	if err != nil {
		c.log.Error("🔧 Falha ao codificar update", "erro", err)
		return nil // nunca encerre o loop de recebimento devido a uma única atualização ruim
	}

	c.dispatcher.Dispatch(ctx, Update{
		ChannelID: channelID,
		MessageID: messageID,
		Payload:   payload,
	})
	return nil
}

// IsAuthenticated relata se a sessão atual está autorizada. Ele roda
// dentro do ciclo de vida do client gotd porque o status de autenticação é uma chamada de rede.
func (c *Client) IsAuthenticated(ctx context.Context) (bool, error) {
	var authorized bool
	err := c.runOnce(ctx, func(ctx context.Context) error {
		st, err := c.tg.Auth().Status(ctx)
		if err != nil {
			return apperrors.Wrap("telegram", "auth_status", err)
		}
		authorized = st.Authorized
		return nil
	})
	if err != nil {
		return false, err
	}
	return authorized, nil
}

// LoadPeers carrega os peers persistidos do storage para o cache em memória.
func (c *Client) LoadPeers(ctx context.Context) error {
	return c.peers.LoadFromDB(ctx)
}

// Auth executa o fluxo interativo se a sessão ainda não estiver autorizada.
func (c *Client) Auth(ctx context.Context) error {
	_, _ = fmt.Fprintln(os.Stdout, "\n  📡 Conectando ao Telegram...")
	return c.runOnce(ctx, func(ctx context.Context) error {
		authn := newTerminalAuthenticator(os.Stdin, os.Stdout, c.log)
		flow := auth.NewFlow(authn, auth.SendCodeOptions{})
		if err := c.tg.Auth().IfNecessary(ctx, flow); err != nil {
			return apperrors.Wrap("telegram", "auth_flow", err)
		}
		return nil
	})
}

func (c *Client) doResolveChannel(ctx context.Context, username string) (*model.Peer, error) {
	resolved, err := c.tg.API().ContactsResolveUsername(ctx, &tg.ContactsResolveUsernameRequest{
		Username: username,
	})
	if err != nil {
		return nil, apperrors.Wrap("telegram", "resolve_username", err)
	}
	if resolved == nil {
		return nil, apperrors.Wrap("telegram", "resolve_username", fmt.Errorf("resolved response is nil"))
	}
	ch, err := firstChannel(resolved.Chats)
	if err != nil {
		return nil, apperrors.Wrap("telegram", "resolve_channel", err)
	}
	accessHash, _ := ch.GetAccessHash()
	peer := &model.Peer{
		ID:         ch.GetID(),
		AccessHash: accessHash,
		Type:       "channel",
		Username:   username,
	}
	c.peers.Set(peer)
	return peer, nil
}

// ResolveChannel resolve um username para um Peer e o armazena em cache.
func (c *Client) ResolveChannel(ctx context.Context, username string) (*model.Peer, error) {
	username = NormalizeUsername(username)
	var peer *model.Peer
	err := c.runOnce(ctx, func(ctx context.Context) error {
		p, err := c.doResolveChannel(ctx, username)
		if err != nil {
			return err
		}
		peer = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return peer, nil
}

// ResolveChannelChecked verifica a autenticação e resolve um canal em um
// único ciclo de vida de conexão, evitando o problema de double-Run.
func (c *Client) ResolveChannelChecked(ctx context.Context, username string) (*model.Peer, error) {
	username = NormalizeUsername(username)
	var peer *model.Peer
	err := c.runOnce(ctx, func(ctx context.Context) error {
		st, err := c.tg.Auth().Status(ctx)
		if err != nil {
			return apperrors.Wrap("telegram", "auth_status", err)
		}
		if !st.Authorized {
			return apperrors.ErrNotAuthenticated
		}
		p, err := c.doResolveChannel(ctx, username)
		if err != nil {
			return err
		}
		peer = p
		return nil
	})
	if err != nil {
		return nil, err
	}
	return peer, nil
}

// FetchHistory faz o backfill de mensagens recentes de um canal. minID exclui
// mensagens com id <= minID, para que execuções repetidas busquem apenas o que é novo.
func (c *Client) FetchHistory(ctx context.Context, channelID int64, minID int64, limit int) ([]HistoryMessage, error) {
	return c.fetchHistory(ctx, channelID, minID, 0, limit)
}

// FetchHistoryWithOffset busca até 'limit' mensagens com id < offsetID
// (as mais novas primeiro quando offsetID == 0). É a primitiva de paginação usada
// pelo backfill retroativo (walk-backward) do collector.
func (c *Client) FetchHistoryWithOffset(ctx context.Context, channelID int64, offsetID int64, limit int) ([]HistoryMessage, error) {
	return c.fetchHistory(ctx, channelID, 0, offsetID, limit)
}

// fetchHistory é a implementação compartilhada para FetchHistory e
// FetchHistoryWithOffset. No máximo um de minID/offsetID deve ser não-zero;
// se ambos forem zero, as `limit` mensagens mais novas serão retornadas.
func (c *Client) fetchHistory(ctx context.Context, channelID int64, minID int64, offsetID int64, limit int) ([]HistoryMessage, error) {
	peer, ok := c.peers.Get(channelID)
	if !ok {
		return nil, apperrors.Wrap("telegram", "fetch_history", fmt.Errorf("peer not found for channel %d", channelID))
	}
	var out []HistoryMessage
	err := c.runOnce(ctx, func(ctx context.Context) error {
		req := &tg.MessagesGetHistoryRequest{
			Peer:  &tg.InputPeerChannel{ChannelID: channelID, AccessHash: peer.AccessHash},
			Limit: limit,
		}
		if minID > 0 {
			req.MinID = int(minID)
		}
		if offsetID > 0 {
			req.OffsetID = int(offsetID)
		}
		res, err := c.tg.API().MessagesGetHistory(ctx, req)
		if err != nil {
			return apperrors.Wrap("telegram", "get_history", err)
		}
		msgs, err := extractMessages(res)
		if err != nil {
			return apperrors.Wrap("telegram", "extract_history", err)
		}
		out = msgs
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Run conecta e despacha atualizações até que o contexto (ctx) seja cancelado, reconectando
// com backoff exponencial em caso de perda de conexão.
func (c *Client) Run(ctx context.Context) error {
	c.dispatcher.Start(ctx)
	defer func() { _ = c.dispatcher.Shutdown(ctx) }()

	attempt := 0
	for {
		c.tg = c.newGotdClient()
		err := c.tg.Run(ctx, func(ctx context.Context) error {
			attempt = 0 // reseta em caso de conexão bem-sucedida
			<-ctx.Done()
			return ctx.Err()
		})
		if ctx.Err() != nil {
			return nil // desligamento limpo
		}
		if err != nil {
			delay, berr := CalculateBackoff(attempt, c.backoff, c.rng)
			if berr != nil {
				return apperrors.Wrap("telegram", "run", berr)
			}
			c.log.Warn("🔄 Reconectando ao Telegram", "tentativa", attempt, "delay", delay.String(), "erro", err)
			if !sleep(ctx, delay) {
				return nil
			}
			attempt++
			continue
		}
		return nil
	}
}

// runOnce conecta, executa f uma vez e desconecta. Usado pelos métodos
// de ação "one-shot" (auth, status, resolve, history).
func (c *Client) runOnce(ctx context.Context, f func(ctx context.Context) error) error {
	c.tg = c.newGotdClient()
	return c.tg.Run(ctx, f)
}

// sleep aguarda por d ou pelo cancelamento de ctx. Retorna false se ctx for cancelado.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func NormalizeUsername(u string) string {
	// Aceitar links t.me: https://t.me/foo ou t.me/foo
	for _, prefix := range []string{"https://t.me/", "http://t.me/", "t.me/"} {
		if len(u) > len(prefix) && u[:len(prefix)] == prefix {
			u = u[len(prefix):]
			break
		}
	}
	// Remove a barra final deixada por alguns formatos de link
	for len(u) > 0 && u[len(u)-1] == '/' {
		u = u[:len(u)-1]
	}
	// Remove o @ inicial
	for len(u) > 0 && u[0] == '@' {
		u = u[1:]
	}

	// Removemos qualquer caractere inválido (sanitização de segurança)
	hasInvalid := false
	for i := 0; i < len(u); i++ {
		c := u[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') && c != '_' {
			hasInvalid = true
			break
		}
	}

	if hasInvalid {
		var b strings.Builder
		b.Grow(len(u))
		for i := 0; i < len(u); i++ {
			c := u[i]
			if (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_' {
				b.WriteByte(c)
			}
		}
		u = b.String()
	}

	return u
}

func firstChannel(chats []tg.ChatClass) (*tg.Channel, error) {
	for _, c := range chats {
		if ch, ok := c.(*tg.Channel); ok {
			return ch, nil
		}
	}
	return nil, fmt.Errorf("no channel in resolved peer")
}

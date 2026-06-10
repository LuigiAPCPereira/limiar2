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
	"github.com/limiar/collector/internal/storage"
)

// TelegramClient is the Facade hiding all gotd/td complexity (session, peers,
// reconnection, auth) behind a small interface. No gotd/td type appears here,
// so the CLI and collector layers never import gotd/td.
type TelegramClient interface {
	// Auth runs the interactive authentication flow if not already authorized.
	Auth(ctx context.Context) error
	// IsAuthenticated reports whether a valid session is present.
	IsAuthenticated(ctx context.Context) (bool, error)
	// LoadPeers loads persisted peers from storage into the in-memory cache.
	LoadPeers(ctx context.Context) error
	// AddUpdateHandler registers an observer for incoming updates.
	AddUpdateHandler(h UpdateHandler)
	// ResolveChannel resolves a @username to a Peer with its access hash and
	// persists it to the peer store.
	ResolveChannel(ctx context.Context, username string) (*storage.Peer, error)
	// ResolveChannelChecked verifies authentication and resolves a channel in
	// a single connection lifecycle. Returns ErrNotAuthenticated if the
	// session is not authorized.
	ResolveChannelChecked(ctx context.Context, username string) (*storage.Peer, error)
	// FetchHistory returns up to limit recent raw message payloads (newest
	// first) for the channel, each already serialized to JSON. The access hash
	// is looked up from the peer store; if the peer is unknown, it returns an
	// error. minID filters out messages with id <= minID (0 means no filter).
	FetchHistory(ctx context.Context, channelID int64, minID int64, limit int) ([]HistoryMessage, error)
	// FetchHistoryWithOffset returns up to limit raw message payloads with
	// id < offsetID for the channel (newest-first when offsetID == 0). It is
	// the pagination primitive used by the collector's walk-backward backfill:
	// each subsequent call uses offsetID = min(id) of the previous page.
	FetchHistoryWithOffset(ctx context.Context, channelID int64, offsetID int64, limit int) ([]HistoryMessage, error)
	// Run connects and blocks, dispatching updates until ctx is cancelled.
	Run(ctx context.Context) error
}

// HistoryMessage is a single historical message captured during backfill: its
// Telegram message id, the send timestamp (Date), and the raw JSON payload.
// Date is used by the collector to stop paginating when messages are older
// than the configured temporal cutoff (LIMIAR_HISTORY_MAX_DAYS).
type HistoryMessage struct {
	MessageID int64
	Date      time.Time
	Payload   []byte
}

// BackoffConfig parameterizes exponential reconnection backoff.
type BackoffConfig struct {
	BaseDelay     time.Duration
	Multiplier    float64
	JitterPercent float64
	Ceiling       time.Duration
	MaxRetries    int
}

// DefaultBackoff returns the spec's reconnection policy: 1s base, 2x, 10%
// jitter, 5m ceiling.
func DefaultBackoff(maxRetries int) BackoffConfig {
	return BackoffConfig{
		BaseDelay:     time.Second,
		Multiplier:    2.0,
		JitterPercent: 0.10,
		Ceiling:       5 * time.Minute,
		MaxRetries:    maxRetries,
	}
}

// CalculateBackoff returns the delay for a given attempt, or
// ErrMaxRetriesExceeded once attempts are exhausted.
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

// Client is the sole implementation of TelegramClient and the only type that
// imports gotd/td. It owns the gotd client, the session store, the peer cache,
// and the dispatcher.
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

// NewClient constructs the facade. The gotd client is lazily created on each
// connection so that the same instance can be reused after disconnection.
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
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	return c
}

// newGotdClient creates a fresh gotd client using the current session and
// dispatcher. Each call returns a new instance because gotd clients cannot be
// reused after Run returns.
func (c *Client) newGotdClient() *telegram.Client {
	return telegram.NewClient(c.appID, c.appHash, telegram.Options{
		SessionStorage: c.session,
		UpdateHandler:  telegram.UpdateHandlerFunc(c.onUpdate),
	})
}

// AddUpdateHandler registers an observer with the dispatcher.
func (c *Client) AddUpdateHandler(h UpdateHandler) {
	c.dispatcher.Register(h)
}

// onUpdate is the gotd UpdateHandler entrypoint: it extracts routing metadata
// and forwards the update (with metadata) to the dispatcher for fan-out.
func (c *Client) onUpdate(ctx context.Context, u tg.UpdatesClass) error {
	payload, err := encodeUpdate(u)
	if err != nil {
		c.log.Error("🔧 Falha ao codificar update", "erro", err)
		return nil // never crash the recv loop on a single bad update
	}
	channelID, messageID, _ := extractUpdateMeta(u)
	c.dispatcher.Dispatch(ctx, Update{
		ChannelID: channelID,
		MessageID: messageID,
		Payload:   payload,
	})
	return nil
}

// IsAuthenticated reports whether the current session is authorized. It runs
// inside the gotd client lifecycle because auth status is a network call.
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

// LoadPeers loads persisted peers from storage into the in-memory cache.
func (c *Client) LoadPeers(ctx context.Context) error {
	return c.peers.LoadFromDB(ctx)
}

// Auth runs the interactive flow if the session is not already authorized.
func (c *Client) Auth(ctx context.Context) error {
	fmt.Fprintln(os.Stdout, "\n  📡 Conectando ao Telegram...")
	return c.runOnce(ctx, func(ctx context.Context) error {
		authn := newTerminalAuthenticator(os.Stdin, os.Stdout, c.log)
		flow := auth.NewFlow(authn, auth.SendCodeOptions{})
		if err := c.tg.Auth().IfNecessary(ctx, flow); err != nil {
			return apperrors.Wrap("telegram", "auth_flow", err)
		}
		return nil
	})
}

func (c *Client) doResolveChannel(ctx context.Context, username string) (*storage.Peer, error) {
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
	peer := &storage.Peer{
		ID:         ch.GetID(),
		AccessHash: accessHash,
		Type:       "channel",
		Username:   username,
	}
	c.peers.Set(peer)
	return peer, nil
}

// ResolveChannel resolves a username to a Peer and caches it.
func (c *Client) ResolveChannel(ctx context.Context, username string) (*storage.Peer, error) {
	username = NormalizeUsername(username)
	var peer *storage.Peer
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

// ResolveChannelChecked verifies authentication and resolves a channel in a
// single connection lifecycle, avoiding the double-Run problem.
func (c *Client) ResolveChannelChecked(ctx context.Context, username string) (*storage.Peer, error) {
	username = NormalizeUsername(username)
	var peer *storage.Peer
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

// FetchHistory backfills recent messages for a channel. minID excludes
// messages with id <= minID, so repeated runs only fetch what is new.
func (c *Client) FetchHistory(ctx context.Context, channelID int64, minID int64, limit int) ([]HistoryMessage, error) {
	return c.fetchHistory(ctx, channelID, minID, 0, limit)
}

// FetchHistoryWithOffset fetches up to limit messages with id < offsetID
// (newest-first when offsetID == 0). It is the pagination primitive used by
// the collector's walk-backward backfill.
func (c *Client) FetchHistoryWithOffset(ctx context.Context, channelID int64, offsetID int64, limit int) ([]HistoryMessage, error) {
	return c.fetchHistory(ctx, channelID, 0, offsetID, limit)
}

// fetchHistory is the shared implementation for FetchHistory and
// FetchHistoryWithOffset. At most one of minID/offsetID should be non-zero;
// if both are zero, the newest `limit` messages are returned.
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

// Run connects and dispatches updates until ctx is cancelled, reconnecting
// with exponential backoff on connection loss.
func (c *Client) Run(ctx context.Context) error {
	c.dispatcher.Start(ctx)
	defer func() { _ = c.dispatcher.Shutdown(ctx) }()

	attempt := 0
	for {
		c.tg = c.newGotdClient()
		err := c.tg.Run(ctx, func(ctx context.Context) error {
			attempt = 0 // reset on successful connect
			<-ctx.Done()
			return ctx.Err()
		})
		if ctx.Err() != nil {
			return nil // clean shutdown
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

// runOnce connects, runs f once, and disconnects. Used by the one-shot action
// methods (auth, status, resolve, history).
func (c *Client) runOnce(ctx context.Context, f func(ctx context.Context) error) error {
	c.tg = c.newGotdClient()
	return c.tg.Run(ctx, f)
}

// sleep waits for d or ctx cancellation. It returns false if ctx was cancelled.
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
	// Trim espaços e caracteres de controle invisíveis
	u = strings.TrimSpace(u)

	// Accept t.me links: https://t.me/foo or t.me/foo
	for _, prefix := range []string{"https://t.me/", "http://t.me/", "t.me/"} {
		if len(u) > len(prefix) && u[:len(prefix)] == prefix {
			u = u[len(prefix):]
			break
		}
	}

	// Strip query parameters if any (e.g., ?start=xxx)
	if idx := strings.Index(u, "?"); idx != -1 {
		u = u[:idx]
	}

	// Strip trailing slash left by some link formats
	for len(u) > 0 && u[len(u)-1] == '/' {
		u = u[:len(u)-1]
	}
	// Strip leading @
	for len(u) > 0 && u[0] == '@' {
		u = u[1:]
	}

	// Removemos qualquer caractere inválido (sanitização de segurança)
	hasInvalid := false
	for i := 0; i < len(u); i++ {
		c := u[i]
		if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
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

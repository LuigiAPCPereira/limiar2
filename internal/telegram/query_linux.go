//go:build linux

package telegram

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gotd/td/telegram/message/peer"
	querymessages "github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
)

var (
	ErrInvalidQuery      = errors.New("telegram query: invalid request")
	ErrPeerNotResolved   = errors.New("telegram query: peer not resolved in this runtime")
	ErrUnsupportedPeer   = errors.New("telegram query: unsupported peer")
	ErrInvalidCursor     = errors.New("telegram query: invalid cursor")
)

type PeerKind string

const (
	PeerKindUser    PeerKind = "user"
	PeerKindChat    PeerKind = "chat"
	PeerKindChannel PeerKind = "channel"
)

type PeerRef struct {
	Value string
}

type PeerKey struct {
	Kind PeerKind
	ID   int64
}

func (k PeerKey) validate() error {
	switch k.Kind {
	case PeerKindUser, PeerKindChat, PeerKindChannel:
	default:
		return fmt.Errorf("%w: unknown peer kind %q", ErrInvalidQuery, k.Kind)
	}
	if k.ID <= 0 {
		return fmt.Errorf("%w: peer id must be positive", ErrInvalidQuery)
	}
	return nil
}

type PeerDescriptor struct {
	Key PeerKey
}

type HistoryRequest struct {
	Limit  int
	Cursor string
}

type MessageKind string

const (
	MessageKindRegular MessageKind = "message"
	MessageKindService MessageKind = "service"
	MessageKindUnknown MessageKind = "unknown"
)

type Message struct {
	ID            int64
	Peer          PeerKey
	Date          time.Time
	Kind          MessageKind
	Text          string
	EditedAt      time.Time
	GroupedID     int64
	MediaKind     string
	ServiceAction string
}

type MessagePage struct {
	Messages   []Message
	NextCursor string
}

type TelegramQuery interface {
	ResolvePeer(ctx context.Context, ref PeerRef) (PeerDescriptor, error)
	History(ctx context.Context, key PeerKey, req HistoryRequest) (MessagePage, error)
}

type resolvePeerFunc func(context.Context, string) (tg.InputPeerClass, error)
type historyFunc func(context.Context, tg.InputPeerClass, int, int) ([]querymessages.Elem, error)

type QueryClient struct {
	resolve resolvePeerFunc
	history historyFunc

	admission chan struct{}

	mu    sync.RWMutex
	peers map[PeerKey]tg.InputPeerClass
}

var _ TelegramQuery = (*QueryClient)(nil)

func newQueryClient(raw *tg.Client, maxConcurrent int) (*QueryClient, error) {
	if raw == nil {
		return nil, fmt.Errorf("%w: nil gotd API client", ErrInvalidQuery)
	}
	if maxConcurrent <= 0 {
		return nil, fmt.Errorf("%w: max concurrent queries must be positive", ErrInvalidQuery)
	}

	resolver := peer.DefaultResolver(raw)
	return newQueryClientWithFuncs(maxConcurrent,
		func(ctx context.Context, value string) (tg.InputPeerClass, error) {
			return peer.ResolveInputPeer(ctx, resolver, peer.Resolve(value))
		},
		func(ctx context.Context, input tg.InputPeerClass, limit, offsetID int) ([]querymessages.Elem, error) {
			iter := querymessages.NewQueryBuilder(raw).
				GetHistory(input).
				BatchSize(limit).
				OffsetID(offsetID).
				Iter()

			result := make([]querymessages.Elem, 0, limit)
			for len(result) < limit && iter.Next(ctx) {
				result = append(result, iter.Value())
			}
			if err := iter.Err(); err != nil {
				return nil, err
			}
			return result, nil
		},
	)
}

func newQueryClientWithFuncs(maxConcurrent int, resolve resolvePeerFunc, history historyFunc) (*QueryClient, error) {
	if maxConcurrent <= 0 || resolve == nil || history == nil {
		return nil, fmt.Errorf("%w: invalid query adapter configuration", ErrInvalidQuery)
	}
	return &QueryClient{
		resolve:    resolve,
		history:    history,
		admission:  make(chan struct{}, maxConcurrent),
		peers:      make(map[PeerKey]tg.InputPeerClass),
	}, nil
}

func (q *QueryClient) ResolvePeer(ctx context.Context, ref PeerRef) (PeerDescriptor, error) {
	if q == nil || q.resolve == nil {
		return PeerDescriptor{}, fmt.Errorf("%w: invalid query adapter", ErrInvalidQuery)
	}
	value := strings.TrimSpace(ref.Value)
	if value == "" {
		return PeerDescriptor{}, fmt.Errorf("%w: empty peer reference", ErrInvalidQuery)
	}
	if err := q.acquire(ctx); err != nil {
		return PeerDescriptor{}, err
	}
	defer q.release()

	input, err := q.resolve(ctx, value)
	if err != nil {
		return PeerDescriptor{}, classifyTelegramError("resolve_peer", err)
	}
	key, err := peerKey(input)
	if err != nil {
		return PeerDescriptor{}, err
	}

	q.mu.Lock()
	q.peers[key] = input
	q.mu.Unlock()

	return PeerDescriptor{Key: key}, nil
}

func (q *QueryClient) History(ctx context.Context, key PeerKey, req HistoryRequest) (MessagePage, error) {
	if q == nil || q.history == nil {
		return MessagePage{}, fmt.Errorf("%w: invalid query adapter", ErrInvalidQuery)
	}
	if err := key.validate(); err != nil {
		return MessagePage{}, err
	}
	if req.Limit < 1 || req.Limit > 100 {
		return MessagePage{}, fmt.Errorf("%w: history limit must be between 1 and 100", ErrInvalidQuery)
	}
	offsetID, err := decodeHistoryCursor(req.Cursor)
	if err != nil {
		return MessagePage{}, err
	}

	q.mu.RLock()
	input, ok := q.peers[key]
	q.mu.RUnlock()
	if !ok {
		return MessagePage{}, fmt.Errorf("%w: %s:%d", ErrPeerNotResolved, key.Kind, key.ID)
	}

	if err := q.acquire(ctx); err != nil {
		return MessagePage{}, err
	}
	defer q.release()

	elems, err := q.history(ctx, input, req.Limit, offsetID)
	if err != nil {
		return MessagePage{}, classifyTelegramError("history", err)
	}

	out := make([]Message, 0, len(elems))
	for _, elem := range elems {
		msg := messageFromElem(elem, key)
		out = append(out, msg)
	}

	page := MessagePage{Messages: out}
	if len(out) == req.Limit {
		lastID := out[len(out)-1].ID
		if lastID > 0 {
			page.NextCursor = encodeHistoryCursor(lastID)
		}
	}
	return page, nil
}

func (q *QueryClient) acquire(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case q.admission <- struct{}{}:
		return nil
	}
}

func (q *QueryClient) release() {
	<-q.admission
}

func peerKey(input tg.InputPeerClass) (PeerKey, error) {
	switch p := input.(type) {
	case *tg.InputPeerUser:
		return PeerKey{Kind: PeerKindUser, ID: p.UserID}, nil
	case *tg.InputPeerUserFromMessage:
		return PeerKey{Kind: PeerKindUser, ID: p.UserID}, nil
	case *tg.InputPeerChat:
		return PeerKey{Kind: PeerKindChat, ID: p.ChatID}, nil
	case *tg.InputPeerChannel:
		return PeerKey{Kind: PeerKindChannel, ID: p.ChannelID}, nil
	case *tg.InputPeerChannelFromMessage:
		return PeerKey{Kind: PeerKindChannel, ID: p.ChannelID}, nil
	default:
		return PeerKey{}, fmt.Errorf("%w: %T", ErrUnsupportedPeer, input)
	}
}

const historyCursorPrefix = "tg-history-v1:"

func encodeHistoryCursor(messageID int64) string {
	if messageID <= 0 {
		return ""
	}
	return historyCursorPrefix + strconv.FormatInt(messageID, 10)
}

func decodeHistoryCursor(cursor string) (int, error) {
	if cursor == "" {
		return 0, nil
	}
	if !strings.HasPrefix(cursor, historyCursorPrefix) {
		return 0, ErrInvalidCursor
	}
	raw := strings.TrimPrefix(cursor, historyCursorPrefix)
	id, err := strconv.ParseInt(raw, 10, 32)
	if err != nil || id <= 0 {
		return 0, ErrInvalidCursor
	}
	return int(id), nil
}

func messageFromElem(elem querymessages.Elem, fallback PeerKey) Message {
	result := Message{
		ID:   int64(elem.Msg.GetID()),
		Peer: fallback,
		Date: time.Unix(int64(elem.Msg.GetDate()), 0).UTC(),
		Kind: MessageKindUnknown,
	}
	if key, err := peerKey(elem.Peer); err == nil {
		result.Peer = key
	}

	switch m := elem.Msg.(type) {
	case *tg.Message:
		result.Kind = MessageKindRegular
		result.Text = m.Message
		if m.EditDate > 0 {
			result.EditedAt = time.Unix(int64(m.EditDate), 0).UTC()
		}
		result.GroupedID = m.GroupedID
		if m.Media != nil && !m.Media.Zero() && m.Media.TypeName() != "messageMediaEmpty" {
			result.MediaKind = m.Media.TypeName()
		}
	case *tg.MessageService:
		result.Kind = MessageKindService
		if m.Action != nil {
			result.ServiceAction = m.Action.TypeName()
		}
	}
	return result
}

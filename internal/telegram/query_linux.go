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
	ErrUnsupportedPeer       = errors.New("telegram query: unsupported peer")
	ErrInvalidCursor         = errors.New("telegram query: invalid cursor")
	ErrInvalidUpstreamMessage = errors.New("telegram query: invalid upstream message")
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
	UpstreamType  string
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

type historyOffset struct {
	ID   int
	Date int
}

type historyFunc func(context.Context, tg.InputPeerClass, int, historyOffset) ([]querymessages.Elem, error)

type QueryClient struct {
	resolve resolvePeerFunc
	history historyFunc

	admission        chan struct{}
	maxPageSize      int
	maxResolvedPeers int

	mu        sync.RWMutex
	peers     map[PeerKey]tg.InputPeerClass
	peerOrder []PeerKey

	observer    Observer
	identityKey string
}

var _ TelegramQuery = (*QueryClient)(nil)

func newQueryClient(raw *tg.Client, maxConcurrent, maxPageSize, maxResolvedPeers int) (*QueryClient, error) {
	if raw == nil {
		return nil, fmt.Errorf("%w: nil gotd API client", ErrInvalidQuery)
	}
	if maxConcurrent <= 0 {
		return nil, fmt.Errorf("%w: max concurrent queries must be positive", ErrInvalidQuery)
	}
	if maxPageSize <= 0 {
		return nil, fmt.Errorf("%w: max history page size must be positive", ErrInvalidQuery)
	}
	if maxResolvedPeers <= 0 {
		return nil, fmt.Errorf("%w: max resolved peers must be positive", ErrInvalidQuery)
	}

	resolver := peer.DefaultResolver(raw)
	return newQueryClientWithFuncs(maxConcurrent, maxPageSize, maxResolvedPeers,
		func(ctx context.Context, value string) (tg.InputPeerClass, error) {
			return peer.ResolveInputPeer(ctx, resolver, peer.Resolve(value))
		},
		func(ctx context.Context, input tg.InputPeerClass, limit int, offset historyOffset) ([]querymessages.Elem, error) {
			iter := querymessages.NewQueryBuilder(raw).
				GetHistory(input).
				BatchSize(limit).
				OffsetID(offset.ID).
				OffsetDate(offset.Date).
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

func newQueryClientWithFuncs(maxConcurrent, maxPageSize, maxResolvedPeers int, resolve resolvePeerFunc, history historyFunc) (*QueryClient, error) {
	if maxConcurrent <= 0 || maxPageSize <= 0 || maxResolvedPeers <= 0 || resolve == nil || history == nil {
		return nil, fmt.Errorf("%w: invalid query adapter configuration", ErrInvalidQuery)
	}
	return &QueryClient{
		resolve:     resolve,
		history:     history,
		admission:        make(chan struct{}, maxConcurrent),
		maxPageSize:      maxPageSize,
		maxResolvedPeers: maxResolvedPeers,
		peers:            make(map[PeerKey]tg.InputPeerClass, maxResolvedPeers),
		peerOrder:        make([]PeerKey, 0, maxResolvedPeers),
	}, nil
}

func (q *QueryClient) ResolvePeer(ctx context.Context, ref PeerRef) (result PeerDescriptor, retErr error) {
	startedAt := time.Now()
	defer func() {
		q.observeOperation("resolve_peer", startedAt, retErr)
	}()
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

	q.rememberPeer(key, input)

	return PeerDescriptor{Key: key}, nil
}

func (q *QueryClient) History(ctx context.Context, key PeerKey, req HistoryRequest) (result MessagePage, retErr error) {
	startedAt := time.Now()
	defer func() {
		q.observeOperation("history", startedAt, retErr)
	}()
	if q == nil || q.history == nil {
		return MessagePage{}, fmt.Errorf("%w: invalid query adapter", ErrInvalidQuery)
	}
	if err := key.validate(); err != nil {
		return MessagePage{}, err
	}
	if req.Limit < 1 || req.Limit > q.maxPageSize {
		return MessagePage{}, fmt.Errorf("%w: history limit must be between 1 and %d", ErrInvalidQuery, q.maxPageSize)
	}
	offset, err := decodeHistoryCursor(req.Cursor)
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

	elems, err := q.history(ctx, input, req.Limit, offset)
	if err != nil {
		return MessagePage{}, classifyTelegramError("history", err)
	}

	out := make([]Message, 0, len(elems))
	for _, elem := range elems {
		msg, err := messageFromElem(elem, key)
		if err != nil {
			return MessagePage{}, err
		}
		out = append(out, msg)
	}

	page := MessagePage{Messages: out}
	if len(out) == req.Limit {
		lastID := out[len(out)-1].ID
		if lastID > 0 {
			page.NextCursor = encodeHistoryCursor(lastID, out[len(out)-1].Date)
		}
	}
	return page, nil
}

func (q *QueryClient) observeOperation(operation string, startedAt time.Time, err error) {
	if q == nil {
		return
	}
	outcome, kind, retryAfter := eventOutcome(err)
	observe(q.observer, Event{
		Type:        EventTypeOperation,
		IdentityKey: q.identityKey,
		Operation:   operation,
		Outcome:     outcome,
		ErrorKind:   kind,
		Duration:    time.Since(startedAt),
		RetryAfter:  retryAfter,
	})
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

func (q *QueryClient) rememberPeer(key PeerKey, input tg.InputPeerClass) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if _, exists := q.peers[key]; exists {
		q.peers[key] = input
		return
	}
	if len(q.peerOrder) >= q.maxResolvedPeers {
		evict := q.peerOrder[0]
		delete(q.peers, evict)
		copy(q.peerOrder, q.peerOrder[1:])
		q.peerOrder = q.peerOrder[:len(q.peerOrder)-1]
	}
	q.peers[key] = input
	q.peerOrder = append(q.peerOrder, key)
}

const historyCursorPrefix = "tg-history-v1:"

func encodeHistoryCursor(messageID int64, date time.Time) string {
	if messageID <= 0 || date.IsZero() {
		return ""
	}
	return historyCursorPrefix +
		strconv.FormatInt(messageID, 10) + ":" +
		strconv.FormatInt(date.Unix(), 10)
}

func decodeHistoryCursor(cursor string) (historyOffset, error) {
	if cursor == "" {
		return historyOffset{}, nil
	}
	if !strings.HasPrefix(cursor, historyCursorPrefix) {
		return historyOffset{}, ErrInvalidCursor
	}
	parts := strings.Split(strings.TrimPrefix(cursor, historyCursorPrefix), ":")
	if len(parts) != 2 {
		return historyOffset{}, ErrInvalidCursor
	}
	id, err := strconv.ParseInt(parts[0], 10, 32)
	if err != nil || id <= 0 {
		return historyOffset{}, ErrInvalidCursor
	}
	date, err := strconv.ParseInt(parts[1], 10, 32)
	if err != nil || date <= 0 {
		return historyOffset{}, ErrInvalidCursor
	}
	return historyOffset{ID: int(id), Date: int(date)}, nil
}

func messageFromElem(elem querymessages.Elem, fallback PeerKey) (Message, error) {
	if elem.Msg == nil {
		return Message{}, fmt.Errorf("%w: nil message", ErrInvalidUpstreamMessage)
	}
	id := int64(elem.Msg.GetID())
	if id <= 0 {
		return Message{}, fmt.Errorf("%w: non-positive message id", ErrInvalidUpstreamMessage)
	}

	result := Message{
		ID:           id,
		Peer:         fallback,
		Date:         telegramTimestamp(elem.Msg.GetDate()),
		Kind:         MessageKindUnknown,
		UpstreamType: elem.Msg.TypeName(),
	}
	if key, err := peerKey(elem.Peer); err == nil {
		result.Peer = key
	}

	switch m := elem.Msg.(type) {
	case *tg.Message:
		result.Kind = MessageKindRegular
		result.Text = m.Message
		if m.EditDate > 0 {
			result.EditedAt = telegramTimestamp(m.EditDate)
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
	return result, nil
}

func telegramTimestamp(value int) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	return time.Unix(int64(value), 0).UTC()
}

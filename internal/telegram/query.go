package telegram

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	gotdpeer "github.com/gotd/td/telegram/message/peer"
	gotdmessages "github.com/gotd/td/telegram/query/messages"
	gotdtelegram "github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

var (
	ErrInvalidQueryConfig = errors.New("telegram query: invalid config")
	ErrInvalidPeerRef     = errors.New("telegram query: invalid peer reference")
	ErrUnsupportedPeer    = errors.New("telegram query: unsupported peer")
	ErrPeerNotResolved    = errors.New("telegram query: peer not resolved in this runtime")
	ErrInvalidHistoryPage = errors.New("telegram query: invalid history page")
)

// PeerKind identifies the Telegram peer namespace without exposing access hashes.
type PeerKind string

const (
	PeerKindUser    PeerKind = "user"
	PeerKindChat    PeerKind = "chat"
	PeerKindChannel PeerKind = "channel"
)

// PeerRef is a Telegram-native reference accepted by gotd's resolver.
// Examples include @username, t.me links and phone references.
//
// The raw reference is input to resolution only. It is not persisted as peer
// authority by this first memory-only capability.
type PeerRef struct {
	Value string
}

func (r PeerRef) validate() error {
	if strings.TrimSpace(r.Value) == "" {
		return fmt.Errorf("%w: empty reference", ErrInvalidPeerRef)
	}
	return nil
}

// PeerKey is the source-aware public identity of a resolved Telegram peer.
// Access hashes remain private to the adapter.
type PeerKey struct {
	Kind PeerKind
	ID   int64
}

func (k PeerKey) validate() error {
	if k.ID <= 0 {
		return fmt.Errorf("%w: peer id must be positive", ErrUnsupportedPeer)
	}
	switch k.Kind {
	case PeerKindUser, PeerKindChat, PeerKindChannel:
		return nil
	default:
		return fmt.Errorf("%w: peer kind %q", ErrUnsupportedPeer, k.Kind)
	}
}

// PeerDescriptor is safe to cross the Telegram boundary.
type PeerDescriptor struct {
	Key PeerKey
}

// MessageID is a Telegram message identifier scoped by PeerKey.
type MessageID int64

// MessageKind preserves whether a Telegram history item is a regular or service
// message. Unknown is retained explicitly for forward compatibility.
type MessageKind string

const (
	MessageKindRegular MessageKind = "regular"
	MessageKindService MessageKind = "service"
	MessageKindUnknown MessageKind = "unknown"
)

// Message is the minimal source DTO for realtime/domain exploration.
// It intentionally is not a promotion/product model.
type Message struct {
	Peer          PeerKey
	ID            MessageID
	Date          time.Time
	Kind          MessageKind
	UpstreamType  string
	Text          string
	EditedAt      time.Time
	GroupedID     int64
	MediaType     string
	ServiceAction string
}

// HistoryCursor is an opaque-enough Telegram pagination position for the first
// query boundary. It contains no access hash or SDK type.
type HistoryCursor struct {
	BeforeMessageID MessageID
	BeforeDate      time.Time
}

// HistoryPage requests one bounded page.
type HistoryPage struct {
	Limit  int
	Cursor *HistoryCursor
}

// MessagePage is one bounded history result.
type MessagePage struct {
	Messages []Message
	Next     *HistoryCursor
}

// TelegramQuery is the read-only Telegram capability exposed to consumers.
type TelegramQuery interface {
	ResolvePeer(ctx context.Context, ref PeerRef) (PeerDescriptor, error)
	History(ctx context.Context, peer PeerKey, page HistoryPage) (MessagePage, error)
}

// QueryConfig makes concurrency and page limits explicit at composition time.
type QueryConfig struct {
	MaxConcurrent int
	MaxPageSize    int
}

func (c QueryConfig) validate() error {
	if c.MaxConcurrent <= 0 {
		return fmt.Errorf("%w: max concurrent must be positive", ErrInvalidQueryConfig)
	}
	if c.MaxPageSize <= 0 {
		return fmt.Errorf("%w: max page size must be positive", ErrInvalidQueryConfig)
	}
	return nil
}

type queryBackend interface {
	Resolve(ctx context.Context, ref string) (tg.InputPeerClass, error)
	History(ctx context.Context, peer tg.InputPeerClass, limit, offsetID, offsetDate int) ([]tg.NotEmptyMessage, error)
}

// Query is the gotd-backed TelegramQuery implementation.
//
// Resolved InputPeer values, including access hashes, are memory-only and scoped
// to this runtime. After restart callers must resolve peers again.
type Query struct {
	backend     queryBackend
	maxPageSize int
	slots       chan struct{}

	mu    sync.RWMutex
	peers map[PeerKey]tg.InputPeerClass
}

var _ TelegramQuery = (*Query)(nil)

// NewQuery builds a read-only capability over the owned gotd client.
// The client itself never crosses this boundary.
func NewQuery(client *gotdtelegram.Client, cfg QueryConfig) (*Query, error) {
	if client == nil {
		return nil, fmt.Errorf("%w: nil gotd client", ErrInvalidQueryConfig)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}

	raw := client.API()
	resolver := gotdpeer.DefaultResolver(raw)
	builder := gotdmessages.NewQueryBuilder(raw).WithResolver(resolver)
	return newQueryWithBackend(&gotdQueryBackend{
		resolver: resolver,
		builder:  builder,
	}, cfg)
}

func newQueryWithBackend(backend queryBackend, cfg QueryConfig) (*Query, error) {
	if backend == nil {
		return nil, fmt.Errorf("%w: nil backend", ErrInvalidQueryConfig)
	}
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return &Query{
		backend:     backend,
		maxPageSize: cfg.MaxPageSize,
		slots:       make(chan struct{}, cfg.MaxConcurrent),
		peers:       make(map[PeerKey]tg.InputPeerClass),
	}, nil
}

// ResolvePeer resolves one Telegram reference and retains its operational
// InputPeer only inside this runtime.
func (q *Query) ResolvePeer(ctx context.Context, ref PeerRef) (PeerDescriptor, error) {
	if q == nil || q.backend == nil {
		return PeerDescriptor{}, fmt.Errorf("%w: invalid query", ErrInvalidQueryConfig)
	}
	if err := ref.validate(); err != nil {
		return PeerDescriptor{}, err
	}
	if err := q.acquire(ctx); err != nil {
		return PeerDescriptor{}, err
	}
	defer q.release()

	input, err := q.backend.Resolve(ctx, ref.Value)
	if err != nil {
		return PeerDescriptor{}, fmt.Errorf("telegram query: resolve peer: %w", err)
	}
	key, err := peerKeyFromInput(input)
	if err != nil {
		return PeerDescriptor{}, err
	}

	q.mu.Lock()
	q.peers[key] = input
	q.mu.Unlock()

	return PeerDescriptor{Key: key}, nil
}

// History returns one bounded page for a peer resolved in this runtime.
func (q *Query) History(ctx context.Context, key PeerKey, page HistoryPage) (MessagePage, error) {
	if q == nil || q.backend == nil {
		return MessagePage{}, fmt.Errorf("%w: invalid query", ErrInvalidQueryConfig)
	}
	if err := key.validate(); err != nil {
		return MessagePage{}, err
	}
	if err := q.validatePage(page); err != nil {
		return MessagePage{}, err
	}

	q.mu.RLock()
	input := q.peers[key]
	q.mu.RUnlock()
	if input == nil {
		return MessagePage{}, ErrPeerNotResolved
	}

	offsetID, offsetDate, err := historyOffsets(page.Cursor)
	if err != nil {
		return MessagePage{}, err
	}

	if err := q.acquire(ctx); err != nil {
		return MessagePage{}, err
	}
	defer q.release()

	raw, err := q.backend.History(ctx, input, page.Limit, offsetID, offsetDate)
	if err != nil {
		return MessagePage{}, fmt.Errorf("telegram query: history: %w", err)
	}

	result := MessagePage{Messages: make([]Message, 0, len(raw))}
	for _, item := range raw {
		mapped, err := mapHistoryMessage(key, item)
		if err != nil {
			return MessagePage{}, err
		}
		result.Messages = append(result.Messages, mapped)
	}

	if len(result.Messages) == page.Limit && len(result.Messages) > 0 {
		last := result.Messages[len(result.Messages)-1]
		result.Next = &HistoryCursor{
			BeforeMessageID: last.ID,
			BeforeDate:      last.Date,
		}
	}
	return result, nil
}

func (q *Query) validatePage(page HistoryPage) error {
	if page.Limit <= 0 || page.Limit > q.maxPageSize {
		return fmt.Errorf("%w: limit %d outside 1..%d", ErrInvalidHistoryPage, page.Limit, q.maxPageSize)
	}
	if page.Cursor != nil {
		if page.Cursor.BeforeMessageID < 0 || page.Cursor.BeforeMessageID > MessageID(math.MaxInt32) {
			return fmt.Errorf("%w: message offset out of Telegram int range", ErrInvalidHistoryPage)
		}
	}
	return nil
}

func historyOffsets(cursor *HistoryCursor) (offsetID, offsetDate int, err error) {
	if cursor == nil {
		return 0, 0, nil
	}
	if cursor.BeforeMessageID < 0 || cursor.BeforeMessageID > MessageID(math.MaxInt32) {
		return 0, 0, fmt.Errorf("%w: message offset out of Telegram int range", ErrInvalidHistoryPage)
	}
	offsetID = int(cursor.BeforeMessageID)
	if !cursor.BeforeDate.IsZero() {
		unix := cursor.BeforeDate.Unix()
		if unix < 0 || unix > math.MaxInt32 {
			return 0, 0, fmt.Errorf("%w: date offset out of Telegram int range", ErrInvalidHistoryPage)
		}
		offsetDate = int(unix)
	}
	return offsetID, offsetDate, nil
}

func (q *Query) acquire(ctx context.Context) error {
	select {
	case q.slots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (q *Query) release() { <-q.slots }

func peerKeyFromInput(input tg.InputPeerClass) (PeerKey, error) {
	switch p := input.(type) {
	case *tg.InputPeerUser:
		return PeerKey{Kind: PeerKindUser, ID: p.UserID}, nil
	case *tg.InputPeerChat:
		return PeerKey{Kind: PeerKindChat, ID: p.ChatID}, nil
	case *tg.InputPeerChannel:
		return PeerKey{Kind: PeerKindChannel, ID: p.ChannelID}, nil
	default:
		return PeerKey{}, fmt.Errorf("%w: input type %T", ErrUnsupportedPeer, input)
	}
}

func mapHistoryMessage(peer PeerKey, item tg.NotEmptyMessage) (Message, error) {
	if item == nil {
		return Message{}, errors.New("telegram query: nil history message")
	}
	id := item.GetID()
	if id <= 0 {
		return Message{}, fmt.Errorf("telegram query: invalid message id %d", id)
	}

	out := Message{
		Peer:         peer,
		ID:           MessageID(id),
		Date:         telegramTime(item.GetDate()),
		Kind:         MessageKindUnknown,
		UpstreamType: item.TypeName(),
	}

	switch m := item.(type) {
	case *tg.Message:
		out.Kind = MessageKindRegular
		out.Text = m.Message
		if edit, ok := m.GetEditDate(); ok {
			out.EditedAt = telegramTime(edit)
		}
		if grouped, ok := m.GetGroupedID(); ok {
			out.GroupedID = grouped
		}
		if media, ok := m.GetMedia(); ok && media != nil {
			out.MediaType = media.TypeName()
		}
	case *tg.MessageService:
		out.Kind = MessageKindService
		if m.Action != nil {
			out.ServiceAction = m.Action.TypeName()
		}
	}

	return out, nil
}

func telegramTime(v int) time.Time {
	if v <= 0 {
		return time.Time{}
	}
	return time.Unix(int64(v), 0).UTC()
}

type gotdQueryBackend struct {
	resolver gotdpeer.Resolver
	builder  *gotdmessages.QueryBuilder
}

func (b *gotdQueryBackend) Resolve(ctx context.Context, ref string) (tg.InputPeerClass, error) {
	return gotdpeer.Resolve(ref).Bind(b.resolver)(ctx)
}

func (b *gotdQueryBackend) History(ctx context.Context, input tg.InputPeerClass, limit, offsetID, offsetDate int) ([]tg.NotEmptyMessage, error) {
	iter := b.builder.
		GetHistory(input).
		BatchSize(limit).
		OffsetID(offsetID).
		OffsetDate(offsetDate).
		Iter()

	out := make([]tg.NotEmptyMessage, 0, limit)
	for len(out) < limit && iter.Next(ctx) {
		out = append(out, iter.Value().Msg)
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

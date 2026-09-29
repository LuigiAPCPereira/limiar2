package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	querymessages "github.com/gotd/td/telegram/query/messages"
	"github.com/gotd/td/tg"
)

func TestQueryResolveCachesOperationalPeerWithoutLeakingAccessHash(t *testing.T) {
	t.Parallel()

	q, err := newQueryClientWithFuncs(2, 100, 8,
		func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerChannel{ChannelID: 42, AccessHash: 999999}, nil
		},
		func(context.Context, tg.InputPeerClass, int, historyOffset) ([]querymessages.Elem, error) {
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	got, err := q.ResolvePeer(context.Background(), PeerRef{Value: "@offers"})
	if err != nil {
		t.Fatalf("ResolvePeer() error=%v", err)
	}
	if got.Key != (PeerKey{Kind: PeerKindChannel, ID: 42}) {
		t.Fatalf("ResolvePeer()=%+v", got)
	}

	q.mu.RLock()
	internal := q.peers[got.Key]
	q.mu.RUnlock()
	p, ok := internal.(*tg.InputPeerChannel)
	if !ok || p.AccessHash != 999999 {
		t.Fatalf("internal operational peer=%T %+v", internal, internal)
	}
}

func TestQueryHistoryRequiresPeerResolvedInThisRuntime(t *testing.T) {
	t.Parallel()

	q, err := newQueryClientWithFuncs(1, 100, 8,
		func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerChannel{ChannelID: 42, AccessHash: 7}, nil
		},
		func(context.Context, tg.InputPeerClass, int, historyOffset) ([]querymessages.Elem, error) {
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = q.History(context.Background(), PeerKey{Kind: PeerKindChannel, ID: 42}, HistoryRequest{Limit: 10})
	if !errors.Is(err, ErrPeerNotResolved) {
		t.Fatalf("History() error=%v, esperado ErrPeerNotResolved", err)
	}
}

func TestQueryHistoryMapsBoundedPageAndOpaqueCursor(t *testing.T) {
	t.Parallel()

	var gotLimit int
	var gotOffset historyOffset
	q, err := newQueryClientWithFuncs(2, 100, 8,
		func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerChannel{ChannelID: 42, AccessHash: 12345}, nil
		},
		func(_ context.Context, input tg.InputPeerClass, limit int, offset historyOffset) ([]querymessages.Elem, error) {
			gotLimit, gotOffset = limit, offset
			channel := input.(*tg.InputPeerChannel)
			return []querymessages.Elem{
				{
					Msg: &tg.Message{
						ID:        11,
						Date:      1_700_000_000,
						Message:   "promo one",
						EditDate:  1_700_000_100,
						GroupedID: 77,
					},
					Peer: &tg.InputPeerChannel{ChannelID: channel.ChannelID, AccessHash: channel.AccessHash},
				},
				{
					Msg: &tg.Message{ID: 10, Date: 1_699_999_900, Message: "promo two"},
					Peer: &tg.InputPeerChannel{ChannelID: channel.ChannelID, AccessHash: channel.AccessHash},
				},
			}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	desc, err := q.ResolvePeer(context.Background(), PeerRef{Value: "offers"})
	if err != nil {
		t.Fatal(err)
	}
	page, err := q.History(context.Background(), desc.Key, HistoryRequest{Limit: 2, Cursor: encodeHistoryCursor(12, time.Unix(1_700_000_200, 0).UTC())})
	if err != nil {
		t.Fatalf("History() error=%v", err)
	}
	if gotLimit != 2 || gotOffset.ID != 12 || gotOffset.Date != 1_700_000_200 {
		t.Fatalf("history args limit=%d offset=%+v", gotLimit, gotOffset)
	}
	if len(page.Messages) != 2 {
		t.Fatalf("messages=%d, esperado 2", len(page.Messages))
	}
	first := page.Messages[0]
	if first.ID != 11 || first.Text != "promo one" || first.Peer != desc.Key || first.Kind != MessageKindRegular {
		t.Fatalf("first message=%+v", first)
	}
	if first.UpstreamType != "message" {
		t.Fatalf("first upstream type=%q, esperado message", first.UpstreamType)
	}
	if first.GroupedID != 77 || first.EditedAt.IsZero() {
		t.Fatalf("message optional metadata=%+v", first)
	}
	if page.NextCursor != "tg-history-v1:10:1699999900" {
		t.Fatalf("NextCursor=%q", page.NextCursor)
	}
}

func TestQueryHistoryMapsServiceAndUnknownWithoutInventingText(t *testing.T) {
	t.Parallel()

	key := PeerKey{Kind: PeerKindChat, ID: 5}
	q, err := newQueryClientWithFuncs(1, 100, 8,
		func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerChat{ChatID: 5}, nil
		},
		func(context.Context, tg.InputPeerClass, int, historyOffset) ([]querymessages.Elem, error) {
			return []querymessages.Elem{{
				Msg:  &tg.MessageService{ID: 3, Date: 100},
				Peer: &tg.InputPeerChat{ChatID: 5},
			}}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.ResolvePeer(context.Background(), PeerRef{Value: "group"}); err != nil {
		t.Fatal(err)
	}
	page, err := q.History(context.Background(), key, HistoryRequest{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 || page.Messages[0].Kind != MessageKindService || page.Messages[0].Text != "" {
		t.Fatalf("service mapping=%+v", page.Messages)
	}
	if page.Messages[0].UpstreamType != "messageService" {
		t.Fatalf("service upstream type=%q", page.Messages[0].UpstreamType)
	}
}

func TestQueryValidatesLimitCursorAndPeer(t *testing.T) {
	t.Parallel()

	q, err := newQueryClientWithFuncs(1, 100, 8,
		func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerUser{UserID: 7, AccessHash: 1}, nil
		},
		func(context.Context, tg.InputPeerClass, int, historyOffset) ([]querymessages.Elem, error) { return nil, nil },
	)
	if err != nil {
		t.Fatal(err)
	}
	desc, err := q.ResolvePeer(context.Background(), PeerRef{Value: "user"})
	if err != nil {
		t.Fatal(err)
	}

	for _, req := range []HistoryRequest{
		{Limit: 0},
		{Limit: 101},
		{Limit: 1, Cursor: "not-a-cursor"},
		{Limit: 1, Cursor: "tg-history-v1:0:1700000000"},
		{Limit: 1, Cursor: "tg-history-v1:10:0"},
		{Limit: 1, Cursor: "tg-history-v1:10"},
	} {
		if _, err := q.History(context.Background(), desc.Key, req); err == nil {
			t.Fatalf("History(%+v) error=nil, esperado validation error", req)
		}
	}
	if _, err := q.ResolvePeer(context.Background(), PeerRef{}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("empty ResolvePeer error=%v", err)
	}
	if _, err := q.History(context.Background(), PeerKey{}, HistoryRequest{Limit: 1}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("invalid key error=%v", err)
	}
}

func TestQueryHistoryRespectsConfiguredPageLimit(t *testing.T) {
	t.Parallel()

	called := false
	q, err := newQueryClientWithFuncs(1, 2, 8,
		func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerChannel{ChannelID: 42, AccessHash: 7}, nil
		},
		func(context.Context, tg.InputPeerClass, int, historyOffset) ([]querymessages.Elem, error) {
			called = true
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	desc, err := q.ResolvePeer(context.Background(), PeerRef{Value: "offers"})
	if err != nil {
		t.Fatal(err)
	}

	if _, err := q.History(context.Background(), desc.Key, HistoryRequest{Limit: 3}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("History() error=%v, esperado ErrInvalidQuery", err)
	}
	if called {
		t.Fatal("backend de histórico foi chamado for request above configured page limit")
	}

	if _, err := q.History(context.Background(), desc.Key, HistoryRequest{Limit: 2}); err != nil {
		t.Fatalf("History() at configured limit error=%v", err)
	}
	if !called {
		t.Fatal("backend de histórico não foi chamado at configured page limit")
	}
}

func TestQueryAdmissionIsBoundedAndCancelable(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	q, err := newQueryClientWithFuncs(1, 100, 8,
		func(ctx context.Context, _ string) (tg.InputPeerClass, error) {
			close(entered)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-release:
				return &tg.InputPeerUser{UserID: 1, AccessHash: 2}, nil
			}
		},
		func(context.Context, tg.InputPeerClass, int, historyOffset) ([]querymessages.Elem, error) { return nil, nil },
	)
	if err != nil {
		t.Fatal(err)
	}

	firstDone := make(chan error, 1)
	go func() {
		_, err := q.ResolvePeer(context.Background(), PeerRef{Value: "first"})
		firstDone <- err
	}()
	<-entered

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := q.ResolvePeer(ctx, PeerRef{Value: "second"}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("erro no segundo ResolvePeer=%v, esperado deadline", err)
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("erro no primeiro ResolvePeer=%v", err)
	}
}

func TestTelegramTimestampPreservesUnknownZero(t *testing.T) {
	t.Parallel()

	if got := telegramTimestamp(0); !got.IsZero() {
		t.Fatalf("telegramTimestamp(0)=%v, esperado zero time", got)
	}
	got := telegramTimestamp(1_700_000_000)
	if got.IsZero() || got.Location() != time.UTC || got.Unix() != 1_700_000_000 {
		t.Fatalf("telegramTimestamp(valid)=%v", got)
	}
}

func TestPeerKeySupportsSourceKindsWithoutAccessHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input tg.InputPeerClass
		esperado  PeerKey
	}{
		{&tg.InputPeerUser{UserID: 1, AccessHash: 99}, PeerKey{Kind: PeerKindUser, ID: 1}},
		{&tg.InputPeerChat{ChatID: 2}, PeerKey{Kind: PeerKindChat, ID: 2}},
		{&tg.InputPeerChannel{ChannelID: 3, AccessHash: 88}, PeerKey{Kind: PeerKindChannel, ID: 3}},
	}
	for _, tt := range tests {
		got, err := peerKey(tt.input)
		if err != nil {
			t.Fatalf("peerKey(%T) error=%v", tt.input, err)
		}
		if got != tt.esperado {
			t.Fatalf("peerKey(%T)=%+v esperado %+v", tt.input, got, tt.want)
		}
	}
}


func TestQueryResolvedPeerCacheIsBounded(t *testing.T) {
	t.Parallel()

	nextID := int64(0)
	q, err := newQueryClientWithFuncs(1, 10, 2,
		func(context.Context, string) (tg.InputPeerClass, error) {
			nextID++
			return &tg.InputPeerChannel{ChannelID: nextID, AccessHash: nextID * 10}, nil
		},
		func(context.Context, tg.InputPeerClass, int, historyOffset) ([]querymessages.Elem, error) {
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	first, err := q.ResolvePeer(context.Background(), PeerRef{Value: "one"})
	if err != nil {
		t.Fatal(err)
	}
	second, err := q.ResolvePeer(context.Background(), PeerRef{Value: "two"})
	if err != nil {
		t.Fatal(err)
	}
	third, err := q.ResolvePeer(context.Background(), PeerRef{Value: "three"})
	if err != nil {
		t.Fatal(err)
	}

	q.mu.RLock()
	defer q.mu.RUnlock()
	if len(q.peers) != 2 || len(q.peerOrder) != 2 {
		t.Fatalf("cache sizes peers=%d order=%d, esperado 2", len(q.peers), len(q.peerOrder))
	}
	if _, ok := q.peers[first.Key]; ok {
		t.Fatalf("peer mais antigo %+v não foi removido", first.Key)
	}
	if _, ok := q.peers[second.Key]; !ok {
		t.Fatalf("second peer %+v ausente", second.Key)
	}
	if _, ok := q.peers[third.Key]; !ok {
		t.Fatalf("third peer %+v ausente", third.Key)
	}
}

func TestQueryReResolveUpdatesPeerWithoutGrowingCache(t *testing.T) {
	t.Parallel()

	accessHash := int64(1)
	q, err := newQueryClientWithFuncs(1, 10, 1,
		func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerChannel{ChannelID: 42, AccessHash: accessHash}, nil
		},
		func(context.Context, tg.InputPeerClass, int, historyOffset) ([]querymessages.Elem, error) {
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	desc, err := q.ResolvePeer(context.Background(), PeerRef{Value: "offers"})
	if err != nil {
		t.Fatal(err)
	}
	accessHash = 2
	if _, err := q.ResolvePeer(context.Background(), PeerRef{Value: "offers"}); err != nil {
		t.Fatal(err)
	}

	q.mu.RLock()
	defer q.mu.RUnlock()
	if len(q.peers) != 1 || len(q.peerOrder) != 1 {
		t.Fatalf("cache cresceu após nova resolução: peers=%d order=%d", len(q.peers), len(q.peerOrder))
	}
	got := q.peers[desc.Key].(*tg.InputPeerChannel)
	if got.AccessHash != 2 {
		t.Fatalf("access hash=%d, esperado refreshed value 2", got.AccessHash)
	}
}


func TestQueryHistoryRejectsInvalidUpstreamMessages(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		msg  tg.NotEmptyMessage
	}{
		{name: "nil message", msg: nil},
		{name: "non-positive id", msg: &tg.Message{ID: 0, Date: 1_700_000_000}},
		{name: "non-positive date", msg: &tg.Message{ID: 1, Date: 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			q, err := newQueryClientWithFuncs(
				1,
				10,
				8,
				func(context.Context, string) (tg.InputPeerClass, error) {
					return &tg.InputPeerChannel{ChannelID: 42, AccessHash: 77}, nil
				},
				func(context.Context, tg.InputPeerClass, int, historyOffset) ([]querymessages.Elem, error) {
					return []querymessages.Elem{{
						Msg:  tt.msg,
						Peer: &tg.InputPeerChannel{ChannelID: 42, AccessHash: 77},
					}}, nil
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			desc, err := q.ResolvePeer(context.Background(), PeerRef{Value: "offers"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := q.History(context.Background(), desc.Key, HistoryRequest{Limit: 1}); !errors.Is(err, ErrInvalidUpstreamMessage) {
				t.Fatalf("History() error=%v, esperado ErrInvalidUpstreamMessage", err)
			}
		})
	}
}


func TestQueryConstructorRejectsPageSizeAboveTelegramLimit(t *testing.T) {
	t.Parallel()

	_, err := newQueryClientWithFuncs(
		1,
		MaxHistoryPageSize+1,
		8,
		func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerChannel{ChannelID: 42, AccessHash: 7}, nil
		},
		func(context.Context, tg.InputPeerClass, int, historyOffset) ([]querymessages.Elem, error) {
			return nil, nil
		},
	)
	if !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("newQueryClientWithFuncs() error=%v, esperado ErrInvalidQuery", err)
	}
}

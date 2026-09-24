//go:build linux

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

	q, err := newQueryClientWithFuncs(2,
		func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerChannel{ChannelID: 42, AccessHash: 999999}, nil
		},
		func(context.Context, tg.InputPeerClass, int, int) ([]querymessages.Elem, error) {
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

	q, err := newQueryClientWithFuncs(1,
		func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerChannel{ChannelID: 42, AccessHash: 7}, nil
		},
		func(context.Context, tg.InputPeerClass, int, int) ([]querymessages.Elem, error) {
			return nil, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = q.History(context.Background(), PeerKey{Kind: PeerKindChannel, ID: 42}, HistoryRequest{Limit: 10})
	if !errors.Is(err, ErrPeerNotResolved) {
		t.Fatalf("History() error=%v, want ErrPeerNotResolved", err)
	}
}

func TestQueryHistoryMapsBoundedPageAndOpaqueCursor(t *testing.T) {
	t.Parallel()

	var gotLimit, gotOffset int
	q, err := newQueryClientWithFuncs(2,
		func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerChannel{ChannelID: 42, AccessHash: 12345}, nil
		},
		func(_ context.Context, input tg.InputPeerClass, limit, offsetID int) ([]querymessages.Elem, error) {
			gotLimit, gotOffset = limit, offsetID
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
	page, err := q.History(context.Background(), desc.Key, HistoryRequest{Limit: 2, Cursor: encodeHistoryCursor(12)})
	if err != nil {
		t.Fatalf("History() error=%v", err)
	}
	if gotLimit != 2 || gotOffset != 12 {
		t.Fatalf("history args limit=%d offset=%d", gotLimit, gotOffset)
	}
	if len(page.Messages) != 2 {
		t.Fatalf("messages=%d, want 2", len(page.Messages))
	}
	first := page.Messages[0]
	if first.ID != 11 || first.Text != "promo one" || first.Peer != desc.Key || first.Kind != MessageKindRegular {
		t.Fatalf("first message=%+v", first)
	}
	if first.GroupedID != 77 || first.EditedAt.IsZero() {
		t.Fatalf("message optional metadata=%+v", first)
	}
	if page.NextCursor != "tg-history-v1:10" {
		t.Fatalf("NextCursor=%q", page.NextCursor)
	}
}

func TestQueryHistoryMapsServiceAndUnknownWithoutInventingText(t *testing.T) {
	t.Parallel()

	key := PeerKey{Kind: PeerKindChat, ID: 5}
	q, err := newQueryClientWithFuncs(1,
		func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerChat{ChatID: 5}, nil
		},
		func(context.Context, tg.InputPeerClass, int, int) ([]querymessages.Elem, error) {
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
}

func TestQueryValidatesLimitCursorAndPeer(t *testing.T) {
	t.Parallel()

	q, err := newQueryClientWithFuncs(1,
		func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerUser{UserID: 7, AccessHash: 1}, nil
		},
		func(context.Context, tg.InputPeerClass, int, int) ([]querymessages.Elem, error) { return nil, nil },
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
		{Limit: 1, Cursor: "tg-history-v1:0"},
	} {
		if _, err := q.History(context.Background(), desc.Key, req); err == nil {
			t.Fatalf("History(%+v) error=nil, want validation error", req)
		}
	}
	if _, err := q.ResolvePeer(context.Background(), PeerRef{}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("empty ResolvePeer error=%v", err)
	}
	if _, err := q.History(context.Background(), PeerKey{}, HistoryRequest{Limit: 1}); !errors.Is(err, ErrInvalidQuery) {
		t.Fatalf("invalid key error=%v", err)
	}
}

func TestQueryAdmissionIsBoundedAndCancelable(t *testing.T) {
	t.Parallel()

	entered := make(chan struct{})
	release := make(chan struct{})
	q, err := newQueryClientWithFuncs(1,
		func(ctx context.Context, _ string) (tg.InputPeerClass, error) {
			close(entered)
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-release:
				return &tg.InputPeerUser{UserID: 1, AccessHash: 2}, nil
			}
		},
		func(context.Context, tg.InputPeerClass, int, int) ([]querymessages.Elem, error) { return nil, nil },
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
		t.Fatalf("second ResolvePeer error=%v, want deadline", err)
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first ResolvePeer error=%v", err)
	}
}

func TestPeerKeySupportsSourceKindsWithoutAccessHash(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input tg.InputPeerClass
		want  PeerKey
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
		if got != tt.want {
			t.Fatalf("peerKey(%T)=%+v want %+v", tt.input, got, tt.want)
		}
	}
}

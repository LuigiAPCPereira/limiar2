package telegram

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/gotd/td/tg"
)

type fakeQueryBackend struct {
	resolve func(context.Context, string) (tg.InputPeerClass, error)
	history func(context.Context, tg.InputPeerClass, int, int, int) ([]tg.NotEmptyMessage, error)
}

func (f fakeQueryBackend) Resolve(ctx context.Context, ref string) (tg.InputPeerClass, error) {
	if f.resolve == nil {
		return nil, errors.New("unexpected Resolve")
	}
	return f.resolve(ctx, ref)
}

func (f fakeQueryBackend) History(ctx context.Context, peer tg.InputPeerClass, limit, offsetID, offsetDate int) ([]tg.NotEmptyMessage, error) {
	if f.history == nil {
		return nil, errors.New("unexpected History")
	}
	return f.history(ctx, peer, limit, offsetID, offsetDate)
}

func TestNewQueryWithBackendValidatesConfig(t *testing.T) {
	backend := fakeQueryBackend{}
	tests := []QueryConfig{
		{},
		{MaxConcurrent: 1},
		{MaxPageSize: 10},
		{MaxConcurrent: -1, MaxPageSize: 10},
		{MaxConcurrent: 1, MaxPageSize: -1},
	}
	for _, cfg := range tests {
		if _, err := newQueryWithBackend(backend, cfg); !errors.Is(err, ErrInvalidQueryConfig) {
			t.Fatalf("newQueryWithBackend(%+v) error=%v, want ErrInvalidQueryConfig", cfg, err)
		}
	}
	if _, err := newQueryWithBackend(nil, QueryConfig{MaxConcurrent: 1, MaxPageSize: 10}); !errors.Is(err, ErrInvalidQueryConfig) {
		t.Fatalf("nil backend error=%v, want ErrInvalidQueryConfig", err)
	}
}

func TestResolvePeerReturnsSafeKeyAndCachesOperationalPeer(t *testing.T) {
	const accessHash = int64(987654321)
	q := mustQuery(t, fakeQueryBackend{
		resolve: func(_ context.Context, ref string) (tg.InputPeerClass, error) {
			if ref != "@offers" {
				t.Fatalf("ref=%q, want @offers", ref)
			}
			return &tg.InputPeerChannel{ChannelID: 42, AccessHash: accessHash}, nil
		},
		history: func(_ context.Context, peer tg.InputPeerClass, limit, _, _ int) ([]tg.NotEmptyMessage, error) {
			got, ok := peer.(*tg.InputPeerChannel)
			if !ok {
				t.Fatalf("peer type=%T, want *tg.InputPeerChannel", peer)
			}
			if got.ChannelID != 42 || got.AccessHash != accessHash {
				t.Fatalf("operational peer=%+v", got)
			}
			if limit != 1 {
				t.Fatalf("limit=%d, want 1", limit)
			}
			return nil, nil
		},
	}, QueryConfig{MaxConcurrent: 1, MaxPageSize: 10})

	desc, err := q.ResolvePeer(context.Background(), PeerRef{Value: "@offers"})
	if err != nil {
		t.Fatalf("ResolvePeer() error=%v", err)
	}
	if desc.Key != (PeerKey{Kind: PeerKindChannel, ID: 42}) {
		t.Fatalf("key=%+v", desc.Key)
	}

	if _, err := q.History(context.Background(), desc.Key, HistoryPage{Limit: 1}); err != nil {
		t.Fatalf("History() error=%v", err)
	}
}

func TestResolvePeerRejectsUnsupportedInputPeer(t *testing.T) {
	q := mustQuery(t, fakeQueryBackend{
		resolve: func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerSelf{}, nil
		},
	}, QueryConfig{MaxConcurrent: 1, MaxPageSize: 10})

	_, err := q.ResolvePeer(context.Background(), PeerRef{Value: "self"})
	if !errors.Is(err, ErrUnsupportedPeer) {
		t.Fatalf("ResolvePeer() error=%v, want ErrUnsupportedPeer", err)
	}
}

func TestHistoryRequiresResolutionInCurrentRuntime(t *testing.T) {
	q := mustQuery(t, fakeQueryBackend{}, QueryConfig{MaxConcurrent: 1, MaxPageSize: 10})

	_, err := q.History(context.Background(), PeerKey{Kind: PeerKindChannel, ID: 42}, HistoryPage{Limit: 1})
	if !errors.Is(err, ErrPeerNotResolved) {
		t.Fatalf("History() error=%v, want ErrPeerNotResolved", err)
	}
}

func TestHistoryMapsMessagesAndCursorWithoutSDKTypes(t *testing.T) {
	regular := &tg.Message{
		ID:      100,
		Date:    1_700_000_100,
		Message: "promo text",
	}
	regular.SetEditDate(1_700_000_200)
	regular.SetGroupedID(777)
	regular.SetMedia(&tg.MessageMediaPhoto{})

	service := &tg.MessageService{
		ID:   99,
		Date: 1_700_000_000,
	}

	var gotLimit, gotOffsetID, gotOffsetDate int
	q := mustQuery(t, fakeQueryBackend{
		resolve: func(context.Context, string) (tg.InputPeerClass, error) {
			return &tg.InputPeerChannel{ChannelID: 42, AccessHash: 88}, nil
		},
		history: func(_ context.Context, _ tg.InputPeerClass, limit, offsetID, offsetDate int) ([]tg.NotEmptyMessage, error) {
			gotLimit, gotOffsetID, gotOffsetDate = limit, offsetID, offsetDate
			return []tg.NotEmptyMessage{regular, service}, nil
		},
	}, QueryConfig{MaxConcurrent: 2, MaxPageSize: 20})

	desc, err := q.ResolvePeer(context.Background(), PeerRef{Value: "offers"})
	if err != nil {
		t.Fatal(err)
	}

	before := time.Unix(1_699_999_900, 0).UTC()
	page, err := q.History(context.Background(), desc.Key, HistoryPage{
		Limit: 2,
		Cursor: &HistoryCursor{
			BeforeMessageID: 101,
			BeforeDate:      before,
		},
	})
	if err != nil {
		t.Fatalf("History() error=%v", err)
	}
	if gotLimit != 2 || gotOffsetID != 101 || gotOffsetDate != int(before.Unix()) {
		t.Fatalf("backend page=(limit=%d id=%d date=%d)", gotLimit, gotOffsetID, gotOffsetDate)
	}
	if len(page.Messages) != 2 {
		t.Fatalf("messages=%d, want 2", len(page.Messages))
	}

	first := page.Messages[0]
	if first.Peer != desc.Key || first.ID != 100 || first.Kind != MessageKindRegular || first.Text != "promo text" {
		t.Fatalf("first message=%+v", first)
	}
	if first.UpstreamType != "message" {
		t.Fatalf("upstream type=%q, want message", first.UpstreamType)
	}
	if first.EditedAt.Unix() != 1_700_000_200 || first.GroupedID != 777 {
		t.Fatalf("edit/group mapping=%+v", first)
	}
	if first.MediaType != "messageMediaPhoto" {
		t.Fatalf("media type=%q, want messageMediaPhoto", first.MediaType)
	}

	second := page.Messages[1]
	if second.Kind != MessageKindService || second.UpstreamType != "messageService" || second.ID != 99 {
		t.Fatalf("second message=%+v", second)
	}
	if page.Next == nil || page.Next.BeforeMessageID != 99 || page.Next.BeforeDate.Unix() != 1_700_000_000 {
		t.Fatalf("next=%+v", page.Next)
	}
}

func TestHistoryOmitsNextWhenPageIsShort(t *testing.T) {
	q := resolvedQuery(t, fakeQueryBackend{
		history: func(context.Context, tg.InputPeerClass, int, int, int) ([]tg.NotEmptyMessage, error) {
			return []tg.NotEmptyMessage{&tg.Message{ID: 10, Date: 100}}, nil
		},
	}, QueryConfig{MaxConcurrent: 1, MaxPageSize: 10})

	page, err := q.History(context.Background(), PeerKey{Kind: PeerKindChannel, ID: 42}, HistoryPage{Limit: 2})
	if err != nil {
		t.Fatal(err)
	}
	if page.Next != nil {
		t.Fatalf("Next=%+v, want nil", page.Next)
	}
}

func TestHistoryValidatesBoundsBeforeBackend(t *testing.T) {
	q := resolvedQuery(t, fakeQueryBackend{
		history: func(context.Context, tg.InputPeerClass, int, int, int) ([]tg.NotEmptyMessage, error) {
			t.Fatal("backend must not be called for invalid page")
			return nil, nil
		},
	}, QueryConfig{MaxConcurrent: 1, MaxPageSize: 5})

	key := PeerKey{Kind: PeerKindChannel, ID: 42}
	for _, page := range []HistoryPage{
		{Limit: 0},
		{Limit: 6},
		{Limit: 1, Cursor: &HistoryCursor{BeforeMessageID: -1}},
		{Limit: 1, Cursor: &HistoryCursor{BeforeMessageID: MessageID(1 << 40)}},
	} {
		if _, err := q.History(context.Background(), key, page); !errors.Is(err, ErrInvalidHistoryPage) {
			t.Fatalf("History(%+v) error=%v, want ErrInvalidHistoryPage", page, err)
		}
	}
}

func TestAdmissionWaitRespectsCancellation(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	q := resolvedQuery(t, fakeQueryBackend{
		history: func(ctx context.Context, _ tg.InputPeerClass, _, _, _ int) ([]tg.NotEmptyMessage, error) {
			close(started)
			select {
			case <-release:
				return nil, nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}, QueryConfig{MaxConcurrent: 1, MaxPageSize: 10})

	key := PeerKey{Kind: PeerKindChannel, ID: 42}
	firstDone := make(chan error, 1)
	go func() {
		_, err := q.History(context.Background(), key, HistoryPage{Limit: 1})
		firstDone <- err
	}()

	<-started
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := q.History(ctx, key, HistoryPage{Limit: 1}); !errors.Is(err, context.Canceled) {
		t.Fatalf("second History() error=%v, want context.Canceled", err)
	}

	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("first History() error=%v", err)
	}
}

func TestResolvePeerValidatesReference(t *testing.T) {
	q := mustQuery(t, fakeQueryBackend{}, QueryConfig{MaxConcurrent: 1, MaxPageSize: 10})
	if _, err := q.ResolvePeer(context.Background(), PeerRef{}); !errors.Is(err, ErrInvalidPeerRef) {
		t.Fatalf("ResolvePeer() error=%v, want ErrInvalidPeerRef", err)
	}
}

func mustQuery(t *testing.T, backend queryBackend, cfg QueryConfig) *Query {
	t.Helper()
	q, err := newQueryWithBackend(backend, cfg)
	if err != nil {
		t.Fatalf("newQueryWithBackend() error=%v", err)
	}
	return q
}

func resolvedQuery(t *testing.T, backend fakeQueryBackend, cfg QueryConfig) *Query {
	t.Helper()
	backend.resolve = func(context.Context, string) (tg.InputPeerClass, error) {
		return &tg.InputPeerChannel{ChannelID: 42, AccessHash: 88}, nil
	}
	q := mustQuery(t, backend, cfg)
	if _, err := q.ResolvePeer(context.Background(), PeerRef{Value: "offers"}); err != nil {
		t.Fatalf("ResolvePeer() error=%v", err)
	}
	return q
}

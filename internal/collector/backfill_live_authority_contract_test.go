package collector_test

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
)

// Estes tipos existem somente no contract test. Eles nomeiam responsabilidades
// sem decidir o schema físico que uma futura implementação deve usar.
type sourceSyncState struct {
	Pts        int
	Qts        int
	Seq        int
	Date       int
	ChannelPts map[int64]int
}

type backfillProgress struct {
	ChannelID           int64
	NextOffsetMessageID int64
	NewestObservedID    int64
	Completed           bool
}

type evidenceObservation struct {
	Acquisition string
	ChannelID   int64
	MessageID   int64
	Payload     string
}

type sourceSyncStore interface {
	LoadSourceSyncState(context.Context) (sourceSyncState, error)
	SaveSourceSyncState(context.Context, sourceSyncState) error
}

type backfillProgressStore interface {
	LoadBackfillProgress(context.Context, int64) (backfillProgress, error)
	SaveBackfillProgress(context.Context, backfillProgress) error
}

type evidenceAppender interface {
	AppendEvidence(context.Context, evidenceObservation) error
}

type liveAdmission struct {
	evidence evidenceAppender
	sync     sourceSyncStore
}

func (a liveAdmission) admit(ctx context.Context, ev evidenceObservation, next sourceSyncState) error {
	if err := a.evidence.AppendEvidence(ctx, ev); err != nil {
		return err
	}
	return a.sync.SaveSourceSyncState(ctx, next)
}

type backfillAdmission struct {
	evidence evidenceAppender
	progress backfillProgressStore
}

func (a backfillAdmission) admitSnapshot(ctx context.Context, ev evidenceObservation, next backfillProgress) error {
	if err := a.evidence.AppendEvidence(ctx, ev); err != nil {
		return err
	}
	return a.progress.SaveBackfillProgress(ctx, next)
}

type authorityStore struct {
	mu sync.Mutex

	syncState sourceSyncState
	backfill  map[int64]backfillProgress
	evidence  []evidenceObservation
	events    []string

	// Campo propositalmente legado: nenhum capability usado pelos actors acima
	// expõe este valor. O teste garante que ele não consegue seedar autoridade.
	legacyLastMessageID map[int64]int64

	failEvidenceForAcquisition string
}

func newAuthorityStore(syncState sourceSyncState, progress backfillProgress) *authorityStore {
	return &authorityStore{
		syncState:           cloneSourceSyncState(syncState),
		backfill:            map[int64]backfillProgress{progress.ChannelID: progress},
		legacyLastMessageID: make(map[int64]int64),
	}
}

func cloneSourceSyncState(in sourceSyncState) sourceSyncState {
	out := in
	if in.ChannelPts != nil {
		out.ChannelPts = make(map[int64]int, len(in.ChannelPts))
		for channelID, pts := range in.ChannelPts {
			out.ChannelPts[channelID] = pts
		}
	}
	return out
}

func (s *authorityStore) LoadSourceSyncState(context.Context) (sourceSyncState, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return cloneSourceSyncState(s.syncState), nil
}

func (s *authorityStore) SaveSourceSyncState(_ context.Context, state sourceSyncState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.syncState = cloneSourceSyncState(state)
	s.events = append(s.events, "sync:save")
	return nil
}

func (s *authorityStore) LoadBackfillProgress(_ context.Context, channelID int64) (backfillProgress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.backfill[channelID], nil
}

func (s *authorityStore) SaveBackfillProgress(_ context.Context, progress backfillProgress) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.backfill[progress.ChannelID] = progress
	s.events = append(s.events, "backfill:save")
	return nil
}

func (s *authorityStore) AppendEvidence(_ context.Context, ev evidenceObservation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.failEvidenceForAcquisition != "" && ev.Acquisition == s.failEvidenceForAcquisition {
		return errors.New("evidence persistence failed")
	}
	s.evidence = append(s.evidence, ev)
	s.events = append(s.events, "evidence:"+ev.Acquisition)
	return nil
}

func (s *authorityStore) snapshot() (sourceSyncState, map[int64]backfillProgress, []evidenceObservation, []string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	progress := make(map[int64]backfillProgress, len(s.backfill))
	for channelID, value := range s.backfill {
		progress[channelID] = value
	}
	return cloneSourceSyncState(s.syncState), progress, append([]evidenceObservation(nil), s.evidence...), append([]string(nil), s.events...)
}

func TestBackfillAuthority_ProgressDoesNotMutateSourceSyncState(t *testing.T) {
	ctx := context.Background()
	initialSync := sourceSyncState{Pts: 80, Qts: 7, Seq: 12, Date: 123, ChannelPts: map[int64]int{42: 900}}
	initialBackfill := backfillProgress{ChannelID: 42, NextOffsetMessageID: 5000, NewestObservedID: 5500}
	store := newAuthorityStore(initialSync, initialBackfill)

	backfill := backfillAdmission{evidence: store, progress: store}
	next := backfillProgress{ChannelID: 42, NextOffsetMessageID: 4500, NewestObservedID: 5500}
	if err := backfill.admitSnapshot(ctx, evidenceObservation{Acquisition: "history_snapshot", ChannelID: 42, MessageID: 4999}, next); err != nil {
		t.Fatal(err)
	}

	gotSync, progress, _, events := store.snapshot()
	if !reflect.DeepEqual(gotSync, initialSync) {
		t.Fatalf("backfill mutated SourceSyncState: got=%+v want=%+v", gotSync, initialSync)
	}
	if got := progress[42]; got != next {
		t.Fatalf("backfill progress=%+v want=%+v", got, next)
	}
	wantEvents := []string{"evidence:history_snapshot", "backfill:save"}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("event order=%v want=%v", events, wantEvents)
	}
}

func TestLiveAuthority_SourceSyncDoesNotMutateBackfillProgress(t *testing.T) {
	ctx := context.Background()
	initialBackfill := backfillProgress{ChannelID: 42, NextOffsetMessageID: 4500, NewestObservedID: 5500}
	store := newAuthorityStore(sourceSyncState{Pts: 80, ChannelPts: map[int64]int{42: 900}}, initialBackfill)

	live := liveAdmission{evidence: store, sync: store}
	next := sourceSyncState{Pts: 81, Qts: 1, Seq: 13, Date: 124, ChannelPts: map[int64]int{42: 901}}
	if err := live.admit(ctx, evidenceObservation{Acquisition: "live_update", ChannelID: 42, MessageID: 5600}, next); err != nil {
		t.Fatal(err)
	}

	gotSync, progress, _, events := store.snapshot()
	if !reflect.DeepEqual(gotSync, next) {
		t.Fatalf("SourceSyncState=%+v want=%+v", gotSync, next)
	}
	if got := progress[42]; got != initialBackfill {
		t.Fatalf("live sync mutated backfill progress: got=%+v want=%+v", got, initialBackfill)
	}
	wantEvents := []string{"evidence:live_update", "sync:save"}
	if !reflect.DeepEqual(events, wantEvents) {
		t.Fatalf("event order=%v want=%v", events, wantEvents)
	}
}

func TestBackfillAuthority_EvidenceFailureCannotAdvanceHistoricalProgress(t *testing.T) {
	ctx := context.Background()
	initialSync := sourceSyncState{Pts: 80, ChannelPts: map[int64]int{42: 900}}
	initialBackfill := backfillProgress{ChannelID: 42, NextOffsetMessageID: 5000, NewestObservedID: 5500}
	store := newAuthorityStore(initialSync, initialBackfill)
	store.failEvidenceForAcquisition = "history_snapshot"

	backfill := backfillAdmission{evidence: store, progress: store}
	err := backfill.admitSnapshot(ctx, evidenceObservation{Acquisition: "history_snapshot", ChannelID: 42, MessageID: 4999}, backfillProgress{
		ChannelID:           42,
		NextOffsetMessageID: 4500,
		NewestObservedID:    5500,
	})
	if err == nil {
		t.Fatal("expected Evidence failure")
	}

	gotSync, progress, evidence, events := store.snapshot()
	if !reflect.DeepEqual(gotSync, initialSync) {
		t.Fatalf("failed backfill mutated live state: %+v", gotSync)
	}
	if got := progress[42]; got != initialBackfill {
		t.Fatalf("progress advanced after Evidence failure: %+v", got)
	}
	if len(evidence) != 0 || len(events) != 0 {
		t.Fatalf("failed Evidence leaked durable effects: evidence=%v events=%v", evidence, events)
	}
}

func TestAuthorityRestart_EachLifecycleLoadsOnlyItsOwnProgress(t *testing.T) {
	ctx := context.Background()
	initialSync := sourceSyncState{Pts: 88, Qts: 9, Seq: 14, Date: 130, ChannelPts: map[int64]int{42: 910}}
	initialBackfill := backfillProgress{ChannelID: 42, NextOffsetMessageID: 4200, NewestObservedID: 5500}
	store := newAuthorityStore(initialSync, initialBackfill)
	store.legacyLastMessageID[42] = 999999

	liveRecovered, err := sourceSyncStore(store).LoadSourceSyncState(ctx)
	if err != nil {
		t.Fatal(err)
	}
	backfillRecovered, err := backfillProgressStore(store).LoadBackfillProgress(ctx, 42)
	if err != nil {
		t.Fatal(err)
	}

	if !reflect.DeepEqual(liveRecovered, initialSync) {
		t.Fatalf("live restart used non-sync authority: got=%+v want=%+v", liveRecovered, initialSync)
	}
	if backfillRecovered != initialBackfill {
		t.Fatalf("backfill restart used wrong authority: got=%+v want=%+v", backfillRecovered, initialBackfill)
	}
	if backfillRecovered.NextOffsetMessageID == store.legacyLastMessageID[42] || liveRecovered.Pts == int(store.legacyLastMessageID[42]) {
		t.Fatal("legacy LastMessageID leaked into a current progress authority")
	}
}

func TestOverlap_LiveAndHistoryPreserveSeparateEvidenceAcquisitions(t *testing.T) {
	ctx := context.Background()
	store := newAuthorityStore(sourceSyncState{Pts: 10}, backfillProgress{ChannelID: 42, NextOffsetMessageID: 1000})
	backfill := backfillAdmission{evidence: store, progress: store}
	live := liveAdmission{evidence: store, sync: store}

	const messageID = int64(1234)
	if err := backfill.admitSnapshot(ctx, evidenceObservation{Acquisition: "history_snapshot", ChannelID: 42, MessageID: messageID, Payload: "same logical message"}, backfillProgress{ChannelID: 42, NextOffsetMessageID: 900, NewestObservedID: messageID}); err != nil {
		t.Fatal(err)
	}
	if err := live.admit(ctx, evidenceObservation{Acquisition: "live_update", ChannelID: 42, MessageID: messageID, Payload: "same logical message"}, sourceSyncState{Pts: 11}); err != nil {
		t.Fatal(err)
	}

	_, _, evidence, _ := store.snapshot()
	if len(evidence) != 2 {
		t.Fatalf("evidence count=%d want=2", len(evidence))
	}
	if evidence[0].MessageID != evidence[1].MessageID || evidence[0].Acquisition == evidence[1].Acquisition {
		t.Fatalf("overlap was not preserved as separate acquisitions: %+v", evidence)
	}
}

func TestCoexistence_ConcurrentLiveAndBackfillKeepAuthoritiesIndependent(t *testing.T) {
	ctx := context.Background()
	initialBackfill := backfillProgress{ChannelID: 42, NextOffsetMessageID: 5000, NewestObservedID: 6000}
	store := newAuthorityStore(sourceSyncState{Pts: 0, ChannelPts: map[int64]int{42: 0}}, initialBackfill)
	live := liveAdmission{evidence: store, sync: store}
	backfill := backfillAdmission{evidence: store, progress: store}

	const iterations = 100
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 1; i <= iterations; i++ {
			state := sourceSyncState{Pts: i, Seq: i, ChannelPts: map[int64]int{42: i}}
			if err := live.admit(ctx, evidenceObservation{Acquisition: "live_update", ChannelID: 42, MessageID: int64(6000 + i)}, state); err != nil {
				t.Errorf("live iteration %d: %v", i, err)
				return
			}
		}
	}()
	go func() {
		defer wg.Done()
		for i := 1; i <= iterations; i++ {
			progress := backfillProgress{ChannelID: 42, NextOffsetMessageID: int64(5000 - i), NewestObservedID: 6000}
			if err := backfill.admitSnapshot(ctx, evidenceObservation{Acquisition: "history_snapshot", ChannelID: 42, MessageID: int64(5000 - i)}, progress); err != nil {
				t.Errorf("backfill iteration %d: %v", i, err)
				return
			}
		}
	}()
	wg.Wait()

	gotSync, progress, evidence, _ := store.snapshot()
	if gotSync.Pts != iterations || gotSync.ChannelPts[42] != iterations {
		t.Fatalf("live authority=%+v want final pts=%d", gotSync, iterations)
	}
	wantBackfill := backfillProgress{ChannelID: 42, NextOffsetMessageID: 5000 - iterations, NewestObservedID: 6000}
	if got := progress[42]; got != wantBackfill {
		t.Fatalf("backfill authority=%+v want=%+v", got, wantBackfill)
	}
	if len(evidence) != iterations*2 {
		t.Fatalf("evidence count=%d want=%d", len(evidence), iterations*2)
	}
}

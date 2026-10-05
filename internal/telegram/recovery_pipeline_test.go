package telegram

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/LuigiAPCPereira/limiar2/internal/evidence"
	"github.com/LuigiAPCPereira/limiar2/internal/recovery"
	gotdtelegram "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
)

type pipelineOrder struct {
	mu     sync.Mutex
	events []string
}

func (o *pipelineOrder) add(event string) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.events = append(o.events, event)
}

func (o *pipelineOrder) snapshot() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return append([]string(nil), o.events...)
}

type pipelineAppender struct {
	mu       sync.Mutex
	items    []evidence.Evidence
	failLive error
	order    *pipelineOrder
}

func (a *pipelineAppender) Append(_ context.Context, item evidence.Evidence) (evidence.EvidenceID, error) {
	if item.Acquisition == liveAcquisition && a.failLive != nil {
		return evidence.EvidenceID{}, a.failLive
	}
	a.mu.Lock()
	a.items = append(a.items, item)
	count := len(a.items)
	a.mu.Unlock()
	if a.order != nil && item.Acquisition == liveAcquisition {
		a.order.add("evidence:live")
	}
	var id evidence.EvidenceID
	id[0] = byte(count)
	return id, nil
}

func (a *pipelineAppender) snapshot() []evidence.Evidence {
	a.mu.Lock()
	defer a.mu.Unlock()
	return append([]evidence.Evidence(nil), a.items...)
}

type pipelineStateStorage struct {
	mu       sync.Mutex
	state    updates.State
	found    bool
	channels map[int64]int
	order    *pipelineOrder
}

var _ updates.StateStorage = (*pipelineStateStorage)(nil)

func newPipelineStateStorage(order *pipelineOrder) *pipelineStateStorage {
	return &pipelineStateStorage{
		state:    updates.State{},
		found:    true,
		channels: map[int64]int{},
		order:    order,
	}
}

func (s *pipelineStateStorage) GetState(context.Context, int64) (updates.State, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state, s.found, nil
}

func (s *pipelineStateStorage) SetState(_ context.Context, _ int64, state updates.State) error {
	s.mu.Lock()
	s.state = state
	s.found = true
	s.mu.Unlock()
	if s.order != nil {
		s.order.add("state:full")
	}
	return nil
}

func (s *pipelineStateStorage) SetPts(_ context.Context, _ int64, pts int) error {
	s.mu.Lock()
	s.state.Pts = pts
	s.mu.Unlock()
	if s.order != nil {
		s.order.add("state:pts")
	}
	return nil
}

func (s *pipelineStateStorage) SetQts(_ context.Context, _ int64, qts int) error {
	s.mu.Lock()
	s.state.Qts = qts
	s.mu.Unlock()
	return nil
}

func (s *pipelineStateStorage) SetDate(_ context.Context, _ int64, date int) error {
	s.mu.Lock()
	s.state.Date = date
	s.mu.Unlock()
	return nil
}

func (s *pipelineStateStorage) SetSeq(_ context.Context, _ int64, seq int) error {
	s.mu.Lock()
	s.state.Seq = seq
	s.mu.Unlock()
	return nil
}

func (s *pipelineStateStorage) SetDateSeq(_ context.Context, _ int64, date, seq int) error {
	s.mu.Lock()
	s.state.Date = date
	s.state.Seq = seq
	s.mu.Unlock()
	return nil
}

func (s *pipelineStateStorage) GetChannelPts(_ context.Context, _ int64, channelID int64) (int, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	pts, ok := s.channels[channelID]
	return pts, ok, nil
}

func (s *pipelineStateStorage) SetChannelPts(_ context.Context, _ int64, channelID int64, pts int) error {
	s.mu.Lock()
	s.channels[channelID] = pts
	s.mu.Unlock()
	return nil
}

func (s *pipelineStateStorage) ForEachChannels(
	ctx context.Context,
	_ int64,
	f func(context.Context, int64, int) error,
) error {
	s.mu.Lock()
	channels := make(map[int64]int, len(s.channels))
	for id, pts := range s.channels {
		channels[id] = pts
	}
	s.mu.Unlock()
	for id, pts := range channels {
		if err := f(ctx, id, pts); err != nil {
			return err
		}
	}
	return nil
}

func (s *pipelineStateStorage) pts() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state.Pts
}

type pipelineChannelHasher struct {
	mu     sync.Mutex
	hashes map[int64]int64
}

var _ updates.ChannelAccessHasher = (*pipelineChannelHasher)(nil)

func newPipelineChannelHasher() *pipelineChannelHasher {
	return &pipelineChannelHasher{hashes: map[int64]int64{}}
}

func (h *pipelineChannelHasher) SetChannelAccessHash(_ context.Context, _ int64, channelID, accessHash int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hashes[channelID] = accessHash
	return nil
}

func (h *pipelineChannelHasher) GetChannelAccessHash(_ context.Context, _ int64, channelID int64) (int64, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	hash, ok := h.hashes[channelID]
	return hash, ok, nil
}

type pipelineUserHasher struct {
	mu     sync.Mutex
	hashes map[int64]int64
}

var _ updates.UserAccessHasher = (*pipelineUserHasher)(nil)

func newPipelineUserHasher() *pipelineUserHasher {
	return &pipelineUserHasher{hashes: map[int64]int64{}}
}

func (h *pipelineUserHasher) SetUserAccessHash(_ context.Context, _ int64, targetUserID, accessHash int64) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.hashes[targetUserID] = accessHash
	return nil
}

func (h *pipelineUserHasher) GetUserAccessHash(_ context.Context, _ int64, targetUserID int64) (int64, bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	hash, ok := h.hashes[targetUserID]
	return hash, ok, nil
}

type pipelineAPI struct{}

var _ updates.API = (*pipelineAPI)(nil)

func newPipelineAPI() *pipelineAPI {
	return &pipelineAPI{}
}

func (a *pipelineAPI) UpdatesGetState(context.Context) (*tg.UpdatesState, error) {
	return &tg.UpdatesState{}, nil
}

func (a *pipelineAPI) UpdatesGetDifference(
	context.Context,
	*tg.UpdatesGetDifferenceRequest,
) (tg.UpdatesDifferenceClass, error) {
	return &tg.UpdatesDifferenceEmpty{}, nil
}

func (a *pipelineAPI) UpdatesGetChannelDifference(
	context.Context,
	*tg.UpdatesGetChannelDifferenceRequest,
) (tg.UpdatesChannelDifferenceClass, error) {
	return &tg.UpdatesChannelDifferenceEmpty{}, nil
}

func baseRecoveryPipelineConfig(
	appender evidence.EvidenceAppender,
	classifier LiveSubscriptionClassifier,
	state updates.StateStorage,
	api updates.API,
) RecoveryPipelineConfig {
	return RecoveryPipelineConfig{
		SubscriptionID:                  "acq:ofertas",
		UserID:                          42,
		EvidenceAppender:                appender,
		Classifier:                      classifier,
		RecoveryAPI:                     api,
		StateStorage:                    state,
		ChannelAccessHasher:             newPipelineChannelHasher(),
		UserAccessHasher:                newPipelineUserHasher(),
		OrderedHandler:                  gotdtelegram.UpdateHandlerFunc(func(context.Context, tg.UpdatesClass) error { return nil }),
		MaxChannelDifferenceConcurrency: 2,
	}
}

func waitPipelineReady(t *testing.T, pipeline *RecoveryPipeline, _ *pipelineAPI) {
	t.Helper()
	select {
	case <-pipeline.Ready():
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline não ficou pronta")
	}
}

func stopPipeline(t *testing.T, cancel context.CancelFunc, result <-chan error) {
	t.Helper()
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run após cancelamento=%v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline não encerrou após cancelamento")
	}
}

func TestNewRecoveryPipelineRejectsHiddenOrMissingAuthorities(t *testing.T) {
	appender := &pipelineAppender{}
	classifier := LiveSubscriptionClassifierFunc(func(context.Context, tg.UpdatesClass) ([]string, error) {
		return []string{"acq:ofertas"}, nil
	})
	state := newPipelineStateStorage(nil)
	api := newPipelineAPI()

	tests := []struct {
		name   string
		mutate func(*RecoveryPipelineConfig)
	}{
		{name: "subscription vazia", mutate: func(c *RecoveryPipelineConfig) { c.SubscriptionID = "" }},
		{name: "subscription com whitespace", mutate: func(c *RecoveryPipelineConfig) { c.SubscriptionID = " acq:ofertas " }},
		{name: "user inválido", mutate: func(c *RecoveryPipelineConfig) { c.UserID = 0 }},
		{name: "sem EvidenceAppender", mutate: func(c *RecoveryPipelineConfig) { c.EvidenceAppender = nil }},
		{name: "sem classifier", mutate: func(c *RecoveryPipelineConfig) { c.Classifier = nil }},
		{name: "sem recovery API", mutate: func(c *RecoveryPipelineConfig) { c.RecoveryAPI = nil }},
		{name: "sem state storage", mutate: func(c *RecoveryPipelineConfig) { c.StateStorage = nil }},
		{name: "sem channel access hash store", mutate: func(c *RecoveryPipelineConfig) { c.ChannelAccessHasher = nil }},
		{name: "sem user access hash store", mutate: func(c *RecoveryPipelineConfig) { c.UserAccessHasher = nil }},
		{name: "sem handler derivado", mutate: func(c *RecoveryPipelineConfig) { c.OrderedHandler = nil }},
		{name: "channel difference ilimitado", mutate: func(c *RecoveryPipelineConfig) { c.MaxChannelDifferenceConcurrency = 0 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := baseRecoveryPipelineConfig(appender, classifier, state, api)
			tt.mutate(&cfg)
			got, err := NewRecoveryPipeline(cfg)
			if !errors.Is(err, ErrInvalidRecoveryPipeline) {
				t.Fatalf("erro=%v, esperado ErrInvalidRecoveryPipeline", err)
			}
			if got != nil {
				t.Fatal("configuração inválida retornou pipeline")
			}
		})
	}
}

func TestRecoveryPipelineBeforeRunPreservesEvidenceWithoutAdvancingState(t *testing.T) {
	appender := &pipelineAppender{}
	state := newPipelineStateStorage(nil)
	api := newPipelineAPI()
	classifier := LiveSubscriptionClassifierFunc(func(context.Context, tg.UpdatesClass) ([]string, error) {
		return []string{"acq:ofertas"}, nil
	})
	pipeline, err := NewRecoveryPipeline(baseRecoveryPipelineConfig(appender, classifier, state, api))
	if err != nil {
		t.Fatal(err)
	}

	update := &tg.Updates{Updates: []tg.UpdateClass{
		&tg.UpdateDeleteMessages{Messages: []int{1}, Pts: 1, PtsCount: 1},
	}}
	err = pipeline.Handler().Handle(context.Background(), update)
	if !errors.Is(err, ErrRecoveryPipelineNotRunning) {
		t.Fatalf("Handle antes de Run=%v", err)
	}
	if got := state.pts(); got != 0 {
		t.Fatalf("state avançou antes de manager pronto: pts=%d", got)
	}
	items := appender.snapshot()
	if len(items) != 1 || items[0].Acquisition != liveAcquisition || items[0].SubscriptionID != "acq:ofertas" {
		t.Fatalf("Evidence pré-Run=%+v", items)
	}
	if barrierErr := pipeline.barrier.Err(); barrierErr != nil {
		t.Fatalf("forward não pronto fechou barrier apesar de Evidence durável: %v", barrierErr)
	}
}

func TestRecoveryPipelineLiveEvidencePrecedesStateAdvance(t *testing.T) {
	order := &pipelineOrder{}
	appender := &pipelineAppender{order: order}
	state := newPipelineStateStorage(order)
	api := newPipelineAPI()
	classifier := LiveSubscriptionClassifierFunc(func(context.Context, tg.UpdatesClass) ([]string, error) {
		return []string{"acq:ofertas"}, nil
	})
	pipeline, err := NewRecoveryPipeline(baseRecoveryPipelineConfig(appender, classifier, state, api))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- pipeline.Run(ctx) }()
	waitPipelineReady(t, pipeline, api)

	update := &tg.Updates{Updates: []tg.UpdateClass{
		&tg.UpdateDeleteMessages{Messages: []int{1}, Pts: 1, PtsCount: 1},
	}}
	if err := pipeline.Handler().Handle(context.Background(), update); err != nil {
		cancel()
		t.Fatalf("Handle: %v", err)
	}
	if got := state.pts(); got != 1 {
		cancel()
		t.Fatalf("pts=%d, esperado 1", got)
	}
	events := order.snapshot()
	if len(events) < 2 || events[0] != "evidence:live" || events[1] != "state:pts" {
		cancel()
		t.Fatalf("ordem=%v, esperado Evidence antes de state", events)
	}

	stopPipeline(t, cancel, result)
}

func TestRecoveryPipelineLiveEvidenceFailureStopsLifecycle(t *testing.T) {
	persistErr := errors.New("Evidence live indisponível")
	appender := &pipelineAppender{failLive: persistErr}
	state := newPipelineStateStorage(nil)
	api := newPipelineAPI()
	classifier := LiveSubscriptionClassifierFunc(func(context.Context, tg.UpdatesClass) ([]string, error) {
		return []string{"acq:ofertas"}, nil
	})
	pipeline, err := NewRecoveryPipeline(baseRecoveryPipelineConfig(appender, classifier, state, api))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- pipeline.Run(ctx) }()
	waitPipelineReady(t, pipeline, api)

	update := &tg.Updates{Updates: []tg.UpdateClass{
		&tg.UpdateDeleteMessages{Messages: []int{1}, Pts: 1, PtsCount: 1},
	}}
	err = pipeline.Handler().Handle(context.Background(), update)
	if !errors.Is(err, persistErr) {
		t.Fatalf("Handle=%v, esperado falha de Evidence", err)
	}
	if got := state.pts(); got != 0 {
		t.Fatalf("state avançou após falha de Evidence: pts=%d", got)
	}

	select {
	case runErr := <-result:
		if !errors.Is(runErr, recovery.ErrDurabilityBarrierClosed) || !errors.Is(runErr, persistErr) {
			t.Fatalf("Run=%v não preserva barrier + Evidence failure", runErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Supervisor não encerrou pipeline após falha live")
	}
}

func TestRecoveryPipelineRejectsCrossSubscriptionClassificationAndStopsLifecycle(t *testing.T) {
	appender := &pipelineAppender{}
	state := newPipelineStateStorage(nil)
	api := newPipelineAPI()
	classifier := LiveSubscriptionClassifierFunc(func(context.Context, tg.UpdatesClass) ([]string, error) {
		return []string{"acq:outra"}, nil
	})
	pipeline, err := NewRecoveryPipeline(baseRecoveryPipelineConfig(appender, classifier, state, api))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() { result <- pipeline.Run(ctx) }()
	waitPipelineReady(t, pipeline, api)

	err = pipeline.Handler().Handle(context.Background(), &tg.Updates{})
	if !errors.Is(err, ErrRecoverySubscriptionMismatch) {
		t.Fatalf("Handle=%v, esperado subscription mismatch", err)
	}
	if len(appender.snapshot()) != 0 {
		t.Fatal("classification mismatch persistiu Evidence")
	}

	select {
	case runErr := <-result:
		if !errors.Is(runErr, recovery.ErrDurabilityBarrierClosed) ||
			!errors.Is(runErr, ErrRecoverySubscriptionMismatch) {
			t.Fatalf("Run=%v não preserva mismatch terminal", runErr)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Supervisor não encerrou pipeline após mismatch")
	}
}

func TestRecoveryPipelineIsOneShot(t *testing.T) {
	appender := &pipelineAppender{}
	state := newPipelineStateStorage(nil)
	api := newPipelineAPI()
	classifier := LiveSubscriptionClassifierFunc(func(context.Context, tg.UpdatesClass) ([]string, error) {
		return []string{"acq:ofertas"}, nil
	})
	pipeline, err := NewRecoveryPipeline(baseRecoveryPipelineConfig(appender, classifier, state, api))
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- pipeline.Run(ctx) }()
	waitPipelineReady(t, pipeline, api)
	stopPipeline(t, cancel, result)

	if err := pipeline.Run(context.Background()); !errors.Is(err, ErrRecoveryPipelineAlreadyRun) {
		t.Fatalf("segundo Run=%v, esperado ErrRecoveryPipelineAlreadyRun", err)
	}
}

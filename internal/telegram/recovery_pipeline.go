package telegram

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/LuigiAPCPereira/limiar2/internal/acquisition"
	"github.com/LuigiAPCPereira/limiar2/internal/evidence"
	"github.com/LuigiAPCPereira/limiar2/internal/recovery"
	gotdtelegram "github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/updates"
	"github.com/gotd/td/tg"
)

var (
	ErrInvalidRecoveryPipeline      = errors.New("pipeline de recovery Telegram: configuração inválida")
	ErrRecoveryPipelineAlreadyRun   = errors.New("pipeline de recovery Telegram: lifecycle já iniciado")
	ErrRecoveryPipelineNotRunning   = errors.New("pipeline de recovery Telegram: manager não está pronto")
	ErrRecoverySubscriptionMismatch = errors.New("pipeline de recovery Telegram: classificação fora da subscription do lifecycle")
)

// RecoveryPipelineConfig contém somente dependências já aceitas para um lifecycle de
// recovery de uma Acquisition Subscription.
//
// Channel/User access hash stores são obrigatórios para evitar os fallbacks in-memory do
// gotd. A persistência concreta desses stores continua uma authority separada deste tipo.
type RecoveryPipelineConfig struct {
	SubscriptionID string
	UserID         int64
	IsBot          bool

	EvidenceAppender evidence.EvidenceAppender
	Classifier       LiveSubscriptionClassifier
	RecoveryAPI      updates.API
	StateStorage     updates.StateStorage

	ChannelAccessHasher updates.ChannelAccessHasher
	UserAccessHasher    updates.UserAccessHasher
	OrderedHandler      gotdtelegram.UpdateHandler

	MaxChannelDifferenceConcurrency int
}

// RecoveryPipeline compõe os boundaries S7 sem possuir o telegram.Client.
//
// O Runtime ainda decide quando/como instalar Handler no client. Este tipo apenas garante
// que live admission, recovery admission, state writes e lifecycle usam a mesma barrier.
type RecoveryPipeline struct {
	manager       *updates.Manager
	managerGate   *recoveryManagerGate
	liveIngress   *LiveIngress
	recoveryAPI   *GuardedRecoveryAPI
	supervisor    *recovery.Supervisor
	barrier       *recovery.DurabilityBarrier
	userID        int64
	isBot         bool
	ready         chan struct{}
	readyOnce     sync.Once
	runMu         sync.Mutex
	runStarted    bool
}

// NewRecoveryPipeline constrói uma composição single-subscription.
//
// Sobreposição entre subscriptions continua válida no modelo de Evidence, mas um
// SourceSyncState/updates.Manager deste lifecycle certifica exatamente uma subscription.
// Portanto, o classifier live deve selecionar somente cfg.SubscriptionID.
func NewRecoveryPipeline(cfg RecoveryPipelineConfig) (*RecoveryPipeline, error) {
	if err := validateRecoveryPipelineConfig(cfg); err != nil {
		return nil, err
	}

	barrier := recovery.NewDurabilityBarrier()
	supervisor, err := recovery.NewSupervisor(barrier)
	if err != nil {
		return nil, fmt.Errorf("%w: construir Supervisor: %v", ErrInvalidRecoveryPipeline, err)
	}

	guardedState, err := recovery.NewGuardedStateStorage(cfg.StateStorage, barrier)
	if err != nil {
		return nil, fmt.Errorf("%w: construir GuardedStateStorage: %v", ErrInvalidRecoveryPipeline, err)
	}

	recoveryCore, err := acquisition.NewAdmission(cfg.EvidenceAppender)
	if err != nil {
		return nil, fmt.Errorf("%w: construir admission de recovery: %v", ErrInvalidRecoveryPipeline, err)
	}
	recoveryAdmission, err := acquisition.NewConfiguredAdmission(
		recoveryCore,
		[]acquisition.Subscription{{ID: cfg.SubscriptionID}},
	)
	if err != nil {
		return nil, fmt.Errorf("%w: configurar admission de recovery: %v", ErrInvalidRecoveryPipeline, err)
	}
	guardedAPI, err := NewGuardedRecoveryAPI(
		cfg.RecoveryAPI,
		recoveryAdmission,
		cfg.SubscriptionID,
		barrier,
	)
	if err != nil {
		return nil, fmt.Errorf("%w: construir GuardedRecoveryAPI: %v", ErrInvalidRecoveryPipeline, err)
	}

	manager := updates.New(updates.Config{
		Handler:                         cfg.OrderedHandler,
		Storage:                         guardedState,
		AccessHasher:                    cfg.ChannelAccessHasher,
		UserAccessHasher:                cfg.UserAccessHasher,
		MaxChannelDifferenceConcurrency: cfg.MaxChannelDifferenceConcurrency,
	})
	gate := &recoveryManagerGate{manager: manager}

	liveCore, err := acquisition.NewAdmission(barrierClosingAppender{
		inner:   cfg.EvidenceAppender,
		barrier: barrier,
	})
	if err != nil {
		return nil, fmt.Errorf("%w: construir admission live: %v", ErrInvalidRecoveryPipeline, err)
	}
	liveAdmission, err := acquisition.NewConfiguredAdmission(
		liveCore,
		[]acquisition.Subscription{{ID: cfg.SubscriptionID}},
	)
	if err != nil {
		return nil, fmt.Errorf("%w: configurar admission live: %v", ErrInvalidRecoveryPipeline, err)
	}

	scopedClassifier := &singleSubscriptionClassifier{
		inner:          cfg.Classifier,
		subscriptionID: cfg.SubscriptionID,
		barrier:        barrier,
	}
	liveIngress, err := newLiveIngressWithClockAndFailure(
		liveAdmission,
		scopedClassifier,
		gate,
		time.Now,
		func(sourceErr error) {
			barrier.Close(sourceErr)
		},
	)
	if err != nil {
		return nil, fmt.Errorf("%w: construir LiveIngress: %v", ErrInvalidRecoveryPipeline, err)
	}

	return &RecoveryPipeline{
		manager:     manager,
		managerGate: gate,
		liveIngress: liveIngress,
		recoveryAPI: guardedAPI,
		supervisor:  supervisor,
		barrier:     barrier,
		userID:      cfg.UserID,
		isBot:       cfg.IsBot,
		ready:       make(chan struct{}),
	}, nil
}

func validateRecoveryPipelineConfig(cfg RecoveryPipelineConfig) error {
	switch {
	case strings.TrimSpace(cfg.SubscriptionID) == "":
		return fmt.Errorf("%w: subscription_id ausente", ErrInvalidRecoveryPipeline)
	case strings.TrimSpace(cfg.SubscriptionID) != cfg.SubscriptionID:
		return fmt.Errorf("%w: subscription_id contém whitespace nas bordas", ErrInvalidRecoveryPipeline)
	case cfg.UserID <= 0:
		return fmt.Errorf("%w: user_id deve ser positivo", ErrInvalidRecoveryPipeline)
	case cfg.EvidenceAppender == nil:
		return fmt.Errorf("%w: EvidenceAppender ausente", ErrInvalidRecoveryPipeline)
	case cfg.Classifier == nil:
		return fmt.Errorf("%w: classificador live ausente", ErrInvalidRecoveryPipeline)
	case cfg.RecoveryAPI == nil:
		return fmt.Errorf("%w: updates.API ausente", ErrInvalidRecoveryPipeline)
	case cfg.StateStorage == nil:
		return fmt.Errorf("%w: StateStorage ausente", ErrInvalidRecoveryPipeline)
	case cfg.ChannelAccessHasher == nil:
		return fmt.Errorf("%w: ChannelAccessHasher ausente", ErrInvalidRecoveryPipeline)
	case cfg.UserAccessHasher == nil:
		return fmt.Errorf("%w: UserAccessHasher ausente", ErrInvalidRecoveryPipeline)
	case cfg.OrderedHandler == nil:
		return fmt.Errorf("%w: handler pós-manager ausente", ErrInvalidRecoveryPipeline)
	case cfg.MaxChannelDifferenceConcurrency <= 0:
		return fmt.Errorf("%w: concorrência de channel difference deve ser positiva", ErrInvalidRecoveryPipeline)
	default:
		return nil
	}
}

// Handler retorna o boundary live a ser instalado como UpdateHandler do client.
//
// Antes de Run atingir OnStart do updates.Manager, o envelope ainda é preservado como
// Evidence, mas o forward falha com ErrRecoveryPipelineNotRunning e nenhum sync state
// avança.
func (p *RecoveryPipeline) Handler() gotdtelegram.UpdateHandler {
	if p == nil {
		return nil
	}
	return p.liveIngress
}

// Ready fecha quando updates.Manager conclui seu start e pode receber Handle com ordering.
func (p *RecoveryPipeline) Ready() <-chan struct{} {
	if p == nil || p.ready == nil {
		ch := make(chan struct{})
		close(ch)
		return ch
	}
	return p.ready
}

// Run possui o lifecycle do manager por meio do Supervisor.
//
// Forget permanece false neste path. Resync/reset explícito exige um comando separado e
// Evidence própria; não é exposto como flag casual desta composição.
func (p *RecoveryPipeline) Run(ctx context.Context) error {
	if p == nil || p.manager == nil || p.managerGate == nil || p.recoveryAPI == nil ||
		p.supervisor == nil || p.barrier == nil || p.userID <= 0 {
		return fmt.Errorf("%w: pipeline não inicializado", ErrInvalidRecoveryPipeline)
	}

	p.runMu.Lock()
	if p.runStarted {
		p.runMu.Unlock()
		return ErrRecoveryPipelineAlreadyRun
	}
	p.runStarted = true
	p.runMu.Unlock()

	defer p.managerGate.deactivate()

	return p.supervisor.Run(ctx, func(lifecycleCtx context.Context) error {
		return p.manager.Run(
			lifecycleCtx,
			p.recoveryAPI,
			p.userID,
			updates.AuthOptions{
				IsBot: p.isBot,
				OnStart: func(context.Context) {
					p.managerGate.activate()
					p.readyOnce.Do(func() { close(p.ready) })
				},
			},
		)
	})
}

type recoveryManagerGate struct {
	mu      sync.RWMutex
	manager *updates.Manager
	active  bool
}

var _ gotdtelegram.UpdateHandler = (*recoveryManagerGate)(nil)

func (g *recoveryManagerGate) activate() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.active = true
}

func (g *recoveryManagerGate) deactivate() {
	if g == nil {
		return
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	g.active = false
}

func (g *recoveryManagerGate) Handle(ctx context.Context, update tg.UpdatesClass) error {
	if g == nil {
		return ErrRecoveryPipelineNotRunning
	}
	g.mu.RLock()
	defer g.mu.RUnlock()

	if g.manager == nil || !g.active {
		return ErrRecoveryPipelineNotRunning
	}
	return g.manager.Handle(ctx, update)
}

type barrierClosingAppender struct {
	inner   evidence.EvidenceAppender
	barrier *recovery.DurabilityBarrier
}

var _ evidence.EvidenceAppender = barrierClosingAppender{}

func (a barrierClosingAppender) Append(
	ctx context.Context,
	item evidence.Evidence,
) (evidence.EvidenceID, error) {
	id, err := a.inner.Append(ctx, item)
	if err != nil {
		a.barrier.Close(err)
	}
	return id, err
}

type singleSubscriptionClassifier struct {
	inner          LiveSubscriptionClassifier
	subscriptionID string
	barrier        *recovery.DurabilityBarrier
}

func (c *singleSubscriptionClassifier) SubscriptionIDs(
	ctx context.Context,
	update tg.UpdatesClass,
) ([]string, error) {
	ids, err := c.inner.SubscriptionIDs(ctx, update)
	if err != nil {
		c.barrier.Close(err)
		return nil, err
	}
	if len(ids) != 1 || ids[0] != c.subscriptionID {
		err := fmt.Errorf(
			"%w: esperado somente %q; obtido %v",
			ErrRecoverySubscriptionMismatch,
			c.subscriptionID,
			ids,
		)
		c.barrier.Close(err)
		return nil, err
	}
	return ids, nil
}

package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/models"
	"github.com/kuaizu-team/kuaizu-service/internal/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type atomicDeliveryStateStub struct {
	mu          sync.Mutex
	claimed     bool
	beginCalls  int
	recoverable []*models.Order
	updated     chan string
}

func (s *atomicDeliveryStateStub) BeginOrderPushDeliveryForUser(context.Context, int, int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.beginCalls++
	if s.claimed {
		return false, nil
	}
	s.claimed = true
	return true, nil
}

func (s *atomicDeliveryStateStub) ListRecoverableOrderDeliveries(context.Context, time.Time, int) ([]*models.Order, error) {
	return s.recoverable, nil
}

func (s *atomicDeliveryStateStub) ClaimRecoverableOrderDelivery(context.Context, int, time.Time) (bool, error) {
	return s.BeginOrderPushDeliveryForUser(context.Background(), 0, 0)
}

func (s *atomicDeliveryStateStub) UpdateOrderPushStatus(_ context.Context, _ int, status string, _ *string) error {
	if s.updated != nil {
		s.updated <- status
	}
	return nil
}

type paidOrderDelivererStub struct {
	mu        sync.Mutex
	calls     int
	ctxErr    error
	result    error
	delivered chan struct{}
}

func (s *paidOrderDelivererStub) Deliver(ctx context.Context, _ *models.Order, alreadyPending bool) error {
	s.mu.Lock()
	s.calls++
	s.ctxErr = ctx.Err()
	s.mu.Unlock()
	if s.delivered != nil {
		select {
		case s.delivered <- struct{}{}:
		default:
		}
	}
	if !alreadyPending {
		return errors.New("delivery was not claimed")
	}
	return s.result
}

func deliveryIntentOrder() *models.Order {
	scene := models.OrderDeliverySceneEmailPromotion
	payload := `{"scene":"email_promotion","projectId":42,"strategy":"region"}`
	return &models.Order{
		ID: 9, UserID: 7, Status: models.OrderStatusPaid,
		DeliveryScene: &scene, DeliveryPayload: &payload,
	}
}

func TestEnsurePaidOrderDeliveryClaimsOnlyOnceAcrossConcurrentTriggers(t *testing.T) {
	state := &atomicDeliveryStateStub{}
	deliverer := &paidOrderDelivererStub{delivered: make(chan struct{}, 1)}
	svc := NewPaymentService(&repository.Repository{}, nil, nil)
	svc.deliveryState = state
	svc.SetPaidOrderDeliveryService(deliverer)

	requestCtx, cancel := context.WithCancel(context.Background())
	cancel()
	var callers sync.WaitGroup
	for i := 0; i < 20; i++ {
		callers.Add(1)
		go func() {
			defer callers.Done()
			svc.EnsurePaidOrderDelivery(requestCtx, deliveryIntentOrder())
		}()
	}
	callers.Wait()

	select {
	case <-deliverer.delivered:
	case <-time.After(time.Second):
		t.Fatal("claimed delivery was not executed")
	}
	deliverer.mu.Lock()
	defer deliverer.mu.Unlock()
	assert.Equal(t, 1, deliverer.calls)
	assert.NoError(t, deliverer.ctxErr, "delivery must not inherit the canceled webhook context")
	state.mu.Lock()
	defer state.mu.Unlock()
	assert.Equal(t, 20, state.beginCalls)
}

func TestEnsurePaidOrderDeliveryPersistsFailureWithDetachedContext(t *testing.T) {
	state := &atomicDeliveryStateStub{updated: make(chan string, 1)}
	deliverer := &paidOrderDelivererStub{result: errors.New("message center unavailable")}
	svc := NewPaymentService(&repository.Repository{}, nil, nil)
	svc.deliveryState = state
	svc.SetPaidOrderDeliveryService(deliverer)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	svc.EnsurePaidOrderDelivery(ctx, deliveryIntentOrder())

	select {
	case status := <-state.updated:
		assert.Equal(t, "failed", status)
	case <-time.After(time.Second):
		t.Fatal("delivery failure state was not persisted")
	}
	require.True(t, state.claimed)
}

func TestRecoverOrderDeliveriesClaimsAndRunsCommittedIntent(t *testing.T) {
	order := deliveryIntentOrder()
	state := &atomicDeliveryStateStub{recoverable: []*models.Order{order}}
	deliverer := &paidOrderDelivererStub{delivered: make(chan struct{}, 1)}
	svc := NewPaymentService(&repository.Repository{}, nil, nil)
	svc.deliveryState = state
	svc.SetPaidOrderDeliveryService(deliverer)

	svc.recoverOrderDeliveries(context.Background())

	select {
	case <-deliverer.delivered:
	case <-time.After(time.Second):
		t.Fatal("recoverable paid order was not delivered")
	}
	state.mu.Lock()
	defer state.mu.Unlock()
	assert.True(t, state.claimed)
}

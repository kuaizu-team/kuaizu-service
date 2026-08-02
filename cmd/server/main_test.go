package main

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/kuaizu-team/kuaizu-service/internal/repository"
)

type cleanupPendingInvitationRepo struct {
	repository.PendingInvitationRepo
	calls atomic.Int32
	done  chan struct{}
}

func (f *cleanupPendingInvitationRepo) DeleteExpired(_ context.Context, _ time.Time, limit int) (int64, error) {
	call := f.calls.Add(1)
	if limit != 500 {
		return 0, nil
	}
	if call == 1 {
		return 500, nil
	}
	close(f.done)
	return 2, nil
}

func TestPendingInvitationCleanupDrainsFullBatches(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	fake := &cleanupPendingInvitationRepo{done: make(chan struct{})}

	startPendingInvitationCleanup(ctx, fake)

	select {
	case <-fake.done:
	case <-time.After(time.Second):
		t.Fatal("cleanup did not drain the second batch")
	}
	if calls := fake.calls.Load(); calls != 2 {
		t.Fatalf("cleanup calls = %d, want 2", calls)
	}
}

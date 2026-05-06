package provider_local

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/sirupsen/logrus"
)

func TestLocalGCCleanupRunnerCoalescesTriggerDuringSweep(t *testing.T) {
	acc := &ProviderAccount{
		le: logrus.New().WithField("test", t.Name()),
	}

	var calls atomic.Int32
	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	secondStarted := make(chan struct{})
	acc.gcCleanupCollect = func(ctx context.Context) (*block_gc.Stats, error) {
		switch calls.Add(1) {
		case 1:
			close(firstStarted)
			select {
			case <-ctx.Done():
				return nil, context.Canceled
			case <-firstRelease:
			}
		case 2:
			close(secondStarted)
		}
		return &block_gc.Stats{}, nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- acc.runGCCleanup(ctx)
	}()

	acc.triggerGCCleanup()
	<-firstStarted
	acc.triggerGCCleanup()
	close(firstRelease)
	<-secondStarted
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected two serialized cleanup sweeps, got %d", got)
	}
}

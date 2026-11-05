package provider_local

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	kvtx_volume "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

func TestLocalGCCleanupRunnerCoalescesTriggerDuringSweep(t *testing.T) {
	// Build an account whose cleanup collector the test replaces.
	acc := &ProviderAccount{
		le: logrus.New().WithField("test", t.Name()),
	}

	// Count sweeps and hold the first one until the test releases it.
	var calls atomic.Int32
	firstStarted := make(chan struct{})
	firstRelease := make(chan struct{})
	secondStarted := make(chan struct{})
	acc.gcCleanupCollect = func(ctx context.Context) (block_gc.CollectResult, error) {
		switch calls.Add(1) {
		case 1:
			close(firstStarted)
			select {
			case <-ctx.Done():
				return block_gc.CollectResult{}, context.Canceled
			case <-firstRelease:
			}
		case 2:
			close(secondStarted)
		}
		return block_gc.CollectResult{}, nil
	}
	acc.gcCleanupRunner = acc.newGCCleanupRunner()

	// Run cleanup until the test cancels the account context.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	errCh := make(chan error, 1)
	go func() {
		errCh <- acc.runGCCleanup(ctx)
	}()

	// Trigger a second sweep while the first is held, then release it.
	acc.triggerGCCleanup()
	<-firstStarted
	acc.triggerGCCleanup()
	close(firstRelease)
	<-secondStarted

	// Require both sweeps to finish, then require cancellation and generation 2.
	if err := acc.WaitGCCleanup(context.Background()); err != nil {
		t.Fatalf("wait cleanup: %v", err)
	}
	cancel()
	if err := <-errCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("expected two serialized cleanup sweeps, got %d", got)
	}
	if got := acc.gcCleanupRunner.CompletedGeneration(); got != 2 {
		t.Fatalf("expected completed generation 2, got %d", got)
	}
}

func TestRemoveSharedObjectGCRefsRemovesLocalBucketRoots(t *testing.T) {
	// Hold a root in a shared object's bucket.
	ctx := context.Background()
	vol, err := kvtx_volume.NewVolume(ctx, "test/local-gc", store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	acc := &ProviderAccount{vol: vol, le: logrus.NewEntry(logrus.New())}
	bucketID := "p/local/acc/blk/so-1"
	ref, _, err := vol.PutBlock(ctx, []byte("world"), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := vol.SetBucketRoots(ctx, bucketID, nil, []block.NamedRoot{{Name: "world", Ref: ref}}); err != nil {
		t.Fatal(err)
	}

	// Deleting the shared object releases the bucket's roots.
	acc.removeSharedObjectGCRefs(ctx, "local", bucketID, acc.le)
	var roots []block.NamedRoot
	err = vol.ListBucketRoots(ctx, bucketID, "", func(root block.NamedRoot) error {
		roots = append(roots, root)
		return nil
	})
	if err != nil || len(roots) != 0 {
		t.Fatalf("bucket roots after delete = %v err %v, want none", roots, err)
	}
}

package sobject_world_engine

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/testbed"
)

// observedPreparationStore gates actual stage writes and observes actual
// release. The underlying testbed Volume owns bytes and graph state throughout.
type observedPreparationStore struct {
	block.StoreOps
	block.RootRetainer
	// entered reports a write already owned by the worker.
	entered chan struct{}
	// resume lets an in-flight write return after cancellation.
	resume chan struct{}
	// released closes after the real Volume stage is released.
	released chan struct{}
	// once gates exactly one write while the worker owns the stage.
	once sync.Once
	// fail injects a storage failure after the gated write returns.
	fail error
}

// OpenStage observes the real stage's ownership without replacing its behavior.
func (s *observedPreparationStore) OpenStage(ctx context.Context) (block.StoreOps, func(), error) {
	stage, release, err := s.RootRetainer.OpenStage(ctx)
	if err != nil {
		return nil, nil, err
	}
	return &observedPreparationWrites{StoreOps: stage, owner: s}, func() {
		release()
		close(s.released)
	}, nil
}

// observedPreparationWrites delays one real write until the test allows return.
type observedPreparationWrites struct {
	block.StoreOps
	// owner controls the interleaving and retains the actual stage interface.
	owner *observedPreparationStore
}

// PutBlockBatch permits cancellation while the write is still in flight.
func (s *observedPreparationWrites) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	s.owner.once.Do(func() {
		close(s.owner.entered)
		<-s.owner.resume
	})
	if !block.IsBackground(ctx) {
		return nil, errors.New("preparation bypassed background backpressure")
	}
	if s.owner.fail != nil {
		return nil, s.owner.fail
	}
	return s.StoreOps.PutBlockBatch(ctx, entries)
}

// TestDeadlistPreparationOwnership exercises a blocked real stage write,
// cancellation before it returns, a ready record orphaned behind an admitted
// save, and engine retirement before that receipt completes. Only actual Done
// may release admitted bytes; a subsequent Volume write drains the stage edge.
func TestDeadlistPreparationOwnership(t *testing.T) {
	for _, fixture := range []struct {
		name                                                    string
		admitted, failIO, failName, cancelReady, cancelReceipts bool
	}{
		{name: "cancel-receipts", cancelReceipts: true},
		{name: "cancel-writing"},
		{name: "cancel-admitted", admitted: true},
		{name: "cancel-ready", cancelReady: true},
		{name: "fail-writing", failIO: true},
		{name: "fail-naming", admitted: true, failName: true},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			admitted := fixture.admitted
			// Use the same real Volume and bucket path as World persistence.
			ctx := t.Context()
			space := newReplayTestSpace(t)
			tb, err := testbed.NewTestbed(ctx, space.c.le)
			if err != nil {
				t.Fatal(err)
			}
			defer tb.Release()
			cursor, err := tb.BuildEmptyCursor(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer cursor.Release()
			inner := cursor.GetBucket()
			store := &observedPreparationStore{
				StoreOps: inner, RootRetainer: inner.(block.RootRetainer),
				entered: make(chan struct{}), resume: make(chan struct{}), released: make(chan struct{}),
			}

			if fixture.failIO {
				store.fail = errors.New("injected stage write failure")
			}

			// Start encoding under the writer, then block actual stage I/O.
			workerCtx, cancel := context.WithCancel(ctx)
			defer cancel()
			var writer writerLock
			var ledger deadlistLedger
			unlock, err := writer.Lock(ctx, "preparation-test")
			if err != nil {
				t.Fatal(err)
			}
			worker := newDeadlistPreparer(workerCtx, &writer, &ledger, store, nil)
			ref := testPage(t, "prepared record destination").Ref
			net := &pendingNet{}
			if fixture.cancelReceipts {
				entry := testPage(t, "unsettled publication")
				receipt := block.NewPublicationReceipt()
				net.puts = []worldPut{newWorldPut(receipt, []*block.PutBatchEntry{entry}, []*block.PutBatchEntry{entry}, 0)}
			}
			net.seal(nil)
			ledger.name(nil, ref, net)
			if _, err := ledger.resolve(); err != nil {
				t.Fatal(err)
			}
			unlock()
			if fixture.cancelReceipts {
				unlock, err = writer.Lock(ctx, "cancel-receipt-wait")
				if err != nil {
					t.Fatal(err)
				}
				ledger.move(0, deadlistOrphan)
				unlock()
				<-net.encoded
				if !errors.Is(net.err, context.Canceled) {
					t.Fatalf("receipt wait survived abandonment: %v", net.err)
				}
				select {
				case <-store.entered:
					t.Fatal("unsettled receipt reached stage I/O")
				default:
				}
				cancel()
				worker.close()
				return
			}
			<-net.encoded
			unlock, err = writer.Lock(ctx, "preparation-test")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := ledger.resolve(); err != nil {
				t.Fatal(err)
			}
			unlock()
			<-store.entered

			// A foreground writer enters while stage I/O remains blocked.
			unlock, err = writer.Lock(ctx, "foreground-during-preparation")
			if err != nil {
				t.Fatal(err)
			}
			job := net.preparation
			initialStage, releaseInitial, err := job.stage.Resolve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			releaseInitial()
			if !admitted && !fixture.failIO && !fixture.cancelReady {
				ledger.move(0, deadlistOrphan)
			}
			unlock()
			if !admitted && !fixture.failIO && !fixture.cancelReady {
				stage, releaseProbe, err := job.stage.Resolve(ctx)
				if err != nil || stage != initialStage {
					t.Fatalf("in-flight stage lost its resolved resource: %v", err)
				}
				releaseProbe()
				select {
				case <-store.released:
					t.Fatal("orphaning released a stage before its write returned")
				default:
				}
			}
			close(store.resume)
			<-job.done
			if fixture.cancelReady {
				cancel()
				worker.close()
				unlock, err = writer.Lock(ctx, "retired-preparation")
				if err != nil {
					t.Fatal(err)
				}
				changed, resolveErr := ledger.resolve()
				if resolveErr != nil || changed || ledger.entries[0].record != nil {
					t.Fatalf("retired engine attached a late preparation: changed=%v error=%v", changed, resolveErr)
				}
				unlock()
			}
			if fixture.failIO {
				unlock, err = writer.Lock(ctx, "failed-preparation")
				if err != nil {
					t.Fatal(err)
				}
				changed, resolveErr := ledger.resolve()
				if resolveErr != nil || !changed || !ledger.recount || ledger.entries[0].state != deadlistOrphan {
					t.Fatalf("preparation failure escaped recovery: changed=%v error=%v recount=%v", changed, resolveErr, ledger.recount)
				}
				unlock()
			}

			// Admit a ready record, then discard its engine and ledger ownership.
			var receipt *block.PublicationReceipt
			if admitted {
				unlock, err = writer.Lock(ctx, "admit-prepared-record")
				if err != nil {
					t.Fatal(err)
				}
				if changed, err := ledger.resolve(); err != nil || !changed {
					t.Fatalf("prepared record did not resolve: changed=%v error=%v", changed, err)
				}
				receipt = block.NewPublicationReceipt()
				ledger.retainPrepared(receipt)
				ledger.saved()
				ledger.move(0, deadlistOrphan)
				if ledger.recount {
					t.Fatal("carried naming was treated as already failed")
				}
				unlock()
			}
			if !fixture.failName {
				cancel()
				worker.close()
			}

			// The accepted receipt alone still protects its prepared header.
			if admitted {
				stage, releaseProbe, err := job.stage.Resolve(ctx)
				if err != nil || stage != initialStage {
					t.Fatalf("admitted stage lost its resolved resource: %v", err)
				}
				releaseProbe()
				pulse := testPage(t, "drain released ownership while receipt waits")
				if err := block.SetRetainedRoots(ctx, inner, []*block.PutBatchEntry{pulse}, []block.NamedRoot{{Name: "preparation-test-pulse", Ref: pulse.Ref}}); err != nil {
					t.Fatal(err)
				}
				mark, err := tb.Volume.GetRefGraph().(block_gc.BlockMarker).MarkBlocks(ctx)
				if err != nil || !mark.Live.Has(block_gc.BlockIRI(net.record)) {
					t.Fatalf("admitted record lost its stage: error=%v", err)
				}
				select {
				case <-store.released:
					t.Fatal("engine cancellation released an admitted stage")
				default:
				}
				if fixture.failName {
					receipt.Resolve(errors.New("injected naming failure"))
					unlock, err = writer.Lock(ctx, "failed-naming")
					if err != nil {
						t.Fatal(err)
					}
					ledger.unsave()
					if !ledger.recount || ledger.recountGen != 0 {
						t.Fatal("failed orphan naming did not schedule recount")
					}
					unlock()
					cancel()
					worker.close()
				} else {
					receipt.Resolve(nil)
				}
			}

			// Actual completion releases the stage, and its next changing
			// transaction removes the edge without keeping an orphan alive.
			<-store.released
			pulse := testPage(t, "drain completed preparation ownership")
			if err := block.SetRetainedRoots(ctx, inner, []*block.PutBatchEntry{pulse}, []block.NamedRoot{{Name: "preparation-test-pulse", Ref: pulse.Ref}}); err != nil {
				t.Fatal(err)
			}
			if _, err := block_gc.NewCollector(tb.Volume.GetRefGraph(), tb.Volume, nil).Collect(ctx); err != nil {
				t.Fatal(err)
			}
			mark, err := tb.Volume.GetRefGraph().(block_gc.BlockMarker).MarkBlocks(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if admitted && mark.Live.Has(block_gc.BlockIRI(net.record)) {
				t.Fatal("completed orphan remains held by its preparation stage")
			}
		})
	}
}

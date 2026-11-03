package sobject_world_engine

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/promise"
	"github.com/aperturerobotics/util/refcount"
	"github.com/s4wave/spacewave/db/block"
	"github.com/sirupsen/logrus"
)

// deadlistPreparationWork holds one worker's immutable inputs and stage lease.
// The ledger keeps only the preparation resource after the blocks are written.
type deadlistPreparationWork struct {
	// net supplies the immutable encoded blocks.
	net *pendingNet
	// job publishes completion and owns the ledger's stage reference.
	job *deadlistPreparation
	// ready resolves the real stage opened outside the writer.
	ready promise.PromiseLike[block.StoreOps]
	// worker keeps that stage until this job's last write returns.
	worker *refcount.Ref[block.StoreOps]
}

// deadlistPreparer serializes record I/O outside the controller writer. Only
// short selection and ledger transitions take that writer; stage writes retain
// their own lease until their actual return, including after cancellation.
type deadlistPreparer struct {
	// ctx bounds this engine's preparation work.
	ctx context.Context
	// writer serializes ledger state with foreground World writes.
	writer *writerLock
	// ledger belongs to this engine and is never reused by its successor.
	ledger *deadlistLedger
	// store is the actual bound store, bypassing the kept-page view.
	store block.StoreOps
	// wake coalesces requests without retaining another copy of record blocks.
	wake chan struct{}
	// closeOnce makes cancellation and worker exit share one cleanup.
	closeOnce sync.Once
	// encoded is this ledger's immutable completion source, also after retirement.
	encoded chan<- struct{}
	// le reports preparation failures without failing a foreground save.
	le *logrus.Entry
}

// newDeadlistPreparer starts one worker for this engine. Construction performs
// no storage I/O and is safe while the controller writer is held.
func newDeadlistPreparer(ctx context.Context, writer *writerLock, ledger *deadlistLedger, store block.StoreOps, le *logrus.Entry) *deadlistPreparer {
	// Install the engine-owned selector before starting its lifetime-bound worker.
	if ledger.encoded == nil {
		ledger.encoded = make(chan struct{}, 1)
	}
	p := &deadlistPreparer{ctx: ctx, writer: writer, ledger: ledger, store: store, wake: make(chan struct{}, 1), encoded: ledger.encoded, le: le}
	ledger.preparer = p

	// Cancellation drops ledger ownership even while a stage write still waits.
	context.AfterFunc(ctx, p.close)

	// One worker serializes the remaining bounded preparation writes.
	p.notify()
	go p.run()
	return p
}

// notify asks the worker to select the next fully encoded record.
func (p *deadlistPreparer) notify() {
	select {
	case p.wake <- struct{}{}:
	default:
	}
}

// next claims one encoded record under the writer. The resource resolver opens
// its stage asynchronously; neither selection nor reference acquisition does I/O.
func (p *deadlistPreparer) next() (*deadlistPreparationWork, error) {
	// Claim only the live engine's immutable encoded inputs.
	unlock, err := p.writer.Lock(p.ctx, "deadlist-preparation")
	if err != nil {
		return nil, err
	}
	defer unlock()
	if p.ctx.Err() != nil || p.ledger.preparer != p {
		return nil, p.ctx.Err()
	}

	// Entries retain net through preparation, keeping From and spans live.
	for i := range p.ledger.entries {
		n := p.ledger.entries[i].net
		if n == nil || n.encoded == nil || n.preparation != nil {
			continue
		}
		select {
		case <-n.encoded:
		default:
			continue
		}
		if n.err != nil {
			continue
		}

		// Cleanup may wait for the Volume pin mutex, so the last reference
		// schedules it outside the controller writer as well.
		ctx, cancel := context.WithCancel(block.WithBackground(p.ctx))
		job := &deadlistPreparation{ctx: ctx, cancel: cancel, done: make(chan struct{})}
		job.stage = refcount.NewRefCount[block.StoreOps](context.Background(), false, nil, nil,
			func(_ context.Context, _ func()) (block.StoreOps, func(), error) {
				stage, release, err := block.OpenStage(ctx, p.store)
				if err != nil {
					return nil, nil, err
				}
				return stage, func() { go release() }, nil
			})
		job.ledger = job.stage.AddRef(nil)
		ready, worker := job.stage.AddRefPromise()
		n.preparation = job
		return &deadlistPreparationWork{net: n, job: job, ready: ready, worker: worker}, nil
	}
	return nil, nil
}

// run prepares each leaf, index and header in its own bounded stage write.
// Results become visible only after done closes, before the replay wake.
func (p *deadlistPreparer) run() {
	// Closing abandons local leases while admitted saves keep their own.
	defer p.close()

	// Drain ready work only after its coalesced wake.
	for {
		select {
		case <-p.ctx.Done():
			return
		case <-p.wake:
		}

		// Selection takes the writer briefly; all stage I/O follows outside it.
		for {
			work, err := p.next()
			if err != nil || work == nil {
				break
			}

			// Each prepared block has bounded references and precedes its parents.
			job := work.job
			stage, err := work.ready.Await(job.ctx)
			if err == nil {
				for _, entry := range work.net.blocks {
					if _, err = stage.PutBlockBatch(job.ctx, []*block.PutBatchEntry{entry}); err != nil {
						break
					}
				}
			}

			// Publish before waking replay, retaining in-flight ownership to return.
			job.err = err
			work.worker.Release()
			close(job.done)
			if err != nil && p.ctx.Err() == nil && p.le != nil {
				p.le.WithError(err).Warn("unable to prepare deadlist record; scheduling recount")
			}
			select {
			case p.encoded <- struct{}{}:
			default:
			}
		}
	}
}

// close drops this engine's ledger leases under the writer. Receipt leases
// remain until actual Done, even when the controller has stopped serving it.
func (p *deadlistPreparer) close() {
	// Cancellation and actual worker exit may race; neither releases twice.
	p.closeOnce.Do(func() {
		// The retired ledger is distinct from any successor engine's ledger.
		unlock, _ := p.writer.Lock(context.Background(), "close-deadlist-preparation")
		defer unlock()
		if p.ledger.preparer != p {
			return
		}
		for i := range p.ledger.entries {
			e := &p.ledger.entries[i]
			e.prepared.abandon()
			if e.net != nil {
				if e.net.cancelEncoding != nil {
					e.net.cancelEncoding()
				}
				e.net.preparation.abandon()
			}
		}
	})
}

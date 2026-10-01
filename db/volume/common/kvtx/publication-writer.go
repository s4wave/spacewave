package kvtx

import (
	"bytes"
	"context"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store_kvtx "github.com/s4wave/spacewave/db/block/store/kvtx"
	"github.com/s4wave/spacewave/db/kvtx"
)

const (
	// publicationMaxPending bounds admitted work through physical completion.
	publicationMaxPending = 64
	// publicationMaxBytes bounds retained payloads and reference metadata.
	publicationMaxBytes = 32 << 20
	// publicationMaxEntries bounds the aggregate queued block operations.
	publicationMaxEntries = 32768
	// publicationGroupLimit bounds publications sharing one physical commit.
	publicationGroupLimit = 16
)

// publicationRequest is one admitted publication awaiting a physical group.
type publicationRequest struct {
	// ctx is the uncancelled admission context retained for the physical write.
	ctx context.Context
	// publication is the caller's immutable unit retained until completion.
	publication *block.AtomicPublication
	// receipt resolves only after this request has its final physical result.
	receipt *block.PublicationReceipt
	// bytes is the computed admission cost of the publication.
	bytes int
	// rootOwner keeps this revision alive until its consumer takes a reader pin.
	rootOwner string
}

// publicationWriter serializes atomic publications into bounded physical groups.
// It owns the queue, admission budget, and completion statistics for a volume.
type publicationWriter struct {
	// volume owns the shared physical block and metadata transaction domain.
	volume *Volume
	// bcast guards the queue, budget, statistics, and lifecycle state below.
	bcast broadcast.Broadcast
	// queue retains requests in their admission order until a group takes them.
	queue []*publicationRequest
	// closing rejects new admission while accepted work drains.
	closing bool
	// finished records that the drain goroutine exited after closing.
	finished bool
	// stats retains admitted work in its budget until its group resolves.
	stats PublicationStats
}

// newPublicationWriter constructs the writer and starts its drain goroutine.
func newPublicationWriter(v *Volume) *publicationWriter {
	w := &publicationWriter{volume: v}
	go w.run()
	return w
}

// publicationSize computes the bounded admission cost of a publication.
func publicationSize(p *block.AtomicPublication) (int, error) {
	// Require a bounded block delta and a head replacement in one volume domain.
	if p == nil || p.Head == nil || p.Head.Replace == nil || p.Head.ObjectStoreID == "" || len(p.Head.Key) == 0 || (p.TrackGC && p.BucketID == "") {
		return 0, errInvalidPublication
	}
	if len(p.Entries) > publicationMaxEntries {
		return 0, block.ErrPublicationTooLarge
	}

	// Account for every retained payload and reference before queue admission.
	size := len(p.Head.Key) + len(p.Head.ObjectStoreID) + len(p.BucketID) + len(p.RootName) + p.Root.SizeVT() + 128
	for _, entry := range p.Entries {
		if entry == nil {
			return 0, errInvalidPublication
		}
		// Count both data and reference metadata, not just payload bytes.
		size += len(entry.Data) + entry.Ref.SizeVT() + 64
		for _, ref := range entry.Refs {
			size += ref.SizeVT() + 16
		}
		if size > publicationMaxBytes {
			return 0, block.ErrPublicationTooLarge
		}
	}
	if size > publicationMaxBytes {
		return 0, block.ErrPublicationTooLarge
	}
	return size, nil
}

// submit admits a publication under the bounded budget, returning its receipt.
// It blocks while the queue is full and the admission context stays live.
func (w *publicationWriter) submit(ctx context.Context, p *block.AtomicPublication, rootOwner string, release func()) (*block.PublicationReceipt, error) {
	// Validate the publication and its complete admission cost before waiting.
	size, err := publicationSize(p)
	if err != nil {
		return nil, err
	}

	// Wait on the queue owner's broadcast until this complete request fits.
	for {
		var waitCh <-chan struct{}
		var receipt *block.PublicationReceipt
		w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			if ctxErr := ctx.Err(); ctxErr != nil {
				err = ctxErr
				return
			}
			if w.closing {
				err = block.ErrPublicationClosed
				return
			}
			if w.stats.Pending < publicationMaxPending && w.stats.PendingBytes+size <= publicationMaxBytes && w.stats.PendingEntries+len(p.Entries) <= publicationMaxEntries {
				receipt = block.NewRetainedPublicationReceipt(release)
				w.queue = append(w.queue, &publicationRequest{
					ctx:         context.WithoutCancel(ctx),
					publication: p,
					receipt:     receipt,
					bytes:       size,
					rootOwner:   rootOwner,
				})
				w.stats.Accepted++
				w.stats.Pending++
				w.stats.PendingBytes += size
				w.stats.PendingEntries += len(p.Entries)
				broadcast()
				return
			}
			waitCh = getWaitCh()
		})

		// Return the admission result, or await a capacity or lifetime change.
		if err != nil || receipt != nil {
			return receipt, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-waitCh:
		}
	}
}

// popLocked removes the next queued request. The caller holds bcast.
func (w *publicationWriter) popLocked() *publicationRequest {
	// Leave an idle queue available for later admissions.
	if len(w.queue) == 0 {
		return nil
	}

	// Release the consumed slot while preserving the remaining admission order.
	r := w.queue[0]
	w.queue[0] = nil
	w.queue = w.queue[1:]
	if len(w.queue) == 0 {
		w.queue = nil
	}
	return r
}

// run drains queued publications one physical group at a time until closing.
func (w *publicationWriter) run() {
	for {
		// Take the next request or publish final shutdown while holding queue state.
		var first *publicationRequest
		var waitCh <-chan struct{}
		w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			first = w.popLocked()
			if first != nil {
				return
			}
			if w.closing {
				w.finished = true
				broadcast()
				return
			}
			waitCh = getWaitCh()
		})

		// An idle writer waits on its owner event; a closed empty writer exits.
		if first == nil {
			if waitCh == nil {
				return
			}
			<-waitCh
			continue
		}

		// Resolve the physical group before releasing its admission budget.
		group, results, committed := w.applyGroup(first)
		w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if committed {
				w.stats.PhysicalCommits++
			}
			for i, r := range group {
				if results[i] != nil {
					w.stats.Rejected++
				}
				w.stats.Completed++
				w.stats.Pending--
				w.stats.PendingBytes -= r.bytes
				w.stats.PendingEntries -= len(r.publication.Entries)
				r.receipt.Resolve(results[i])
			}
			broadcast()
		})

		// Notify volume observers only after successful physical publication.
		if committed {
			w.volume.broadcastStorageStatsChanged()
		}
	}
}

// applyGroup never treats TxStore.Discard as a savepoint. Each request is
// preflighted before mutation. A comparison/validation rejection affects only
// that request; any error after mutations start aborts the entire physical group.
// Ready requests are collected opportunistically, with no latency timer.
func (w *publicationWriter) applyGroup(first *publicationRequest) (group []*publicationRequest, results []error, committed bool) {
	// Open the physical writer before constructing any transaction-scoped consumer.
	group = []*publicationRequest{first}
	tx, err := w.volume.kvtxStore.NewTransaction(first.ctx, true)
	if err != nil {
		return group, []error{err}, false
	}
	defer tx.Discard()

	// Share that transaction across addressed blocks and lazily opened graph indexes.
	store := kvtx.NewTxStore(tx)
	blocks := block_store_kvtx.NewKVTxBlock(w.volume.kvKey, store, w.volume.GetHashType(), w.volume.atomicHashGet)
	var rg *block_gc.RefGraph
	defer func() {
		if rg != nil {
			_ = rg.Close()
		}
	}()

	// Preflight each request against prior accepted changes in this physical group.
	accepted := 0
	prior := make(map[*block.PublicationReceipt]error)
	for i := 0; i < len(group); i++ {
		req := group[i]
		p := req.publication
		ctx := req.ctx
		var result error
		if p.After != nil {
			if previous, ok := prior[p.After]; ok {
				if previous != nil {
					result = block.NewPublicationDependencyError(previous)
				}
			} else {
				select {
				case <-p.After.Done():
					if previous := p.After.Wait(ctx); previous != nil {
						result = block.NewPublicationDependencyError(previous)
					}
				default:
					result = block.ErrPublicationDependency
				}
			}
		}
		key := append(w.volume.kvKey.GetObjectStorePrefixByID(p.Head.ObjectStoreID), p.Head.Key...)
		var replacement []byte
		var prepared *publicationOverlay
		if result == nil {
			prepared, result = newPublicationOverlay(blocks, p.Entries)
		}
		if result == nil {
			var current []byte
			var found bool
			current, found, result = tx.Get(ctx, key)
			if result == nil {
				replacement, result = p.Head.Replace(ctx, bytes.Clone(current), found)
			}
		}
		if result == nil && p.Validate != nil {
			result = p.Validate(ctx, prepared)
		}
		results = append(results, result)
		prior[req.receipt] = result
		if result == nil {
			// All failures from here onward are physical-group failures: there are no
			// per-request savepoints. Do not carry a partially written request onward.
			if p.TrackGC {
				if p.BucketID == "" {
					err = errInvalidPublication
				} else if rg == nil {
					rg, err = block_gc.NewRefGraph(ctx, store, volumeRefGraphPrefix())
				}
				if err == nil {
					owner := block_gc.BucketIRI(p.BucketID)
					err = rg.AddRef(ctx, block_gc.NodeGCRoot, owner)
					if err == nil {
						gc := block_gc.NewGCStoreOpsWithParentAndTraceTask(blocks, rg, owner, block_gc.BucketFlushTask())
						err = gc.PutBlockBatch(ctx, p.Entries)
						if err == nil {
							err = gc.FlushPending(ctx)
						}
						if err == nil && p.RootName != "" {
							// A temporary reader owner bridges publication and
							// consumer handoff, and can be reaped after a crash.
							err = setBucketRoot(ctx, blocks, rg, p.BucketID, p.RootName, p.Root)
							if err == nil && req.rootOwner != "" {
								err = rg.ApplyRefBatch(ctx, []block_gc.RefEdge{
									{Subject: block_gc.NodeGCRoot, Object: req.rootOwner},
									{Subject: req.rootOwner, Object: block_gc.BlockIRI(p.Root)},
								}, nil)
							}
							if err == nil && !p.Root.GetEmpty() {
								err = rg.AddRef(ctx, block_gc.BlockIRI(p.Root), completeWorldNode)
							}
						}
					}
				}
			} else {
				err = blocks.PutBlockBatch(ctx, p.Entries)
			}
			if err == nil {
				err = tx.Set(ctx, key, replacement)
			}
			if err != nil {
				break
			}
			accepted++
		}
		if len(group) < publicationGroupLimit {
			w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				next := w.popLocked()
				if next != nil {
					group = append(group, next)
				}
			})
		}
	}

	// Publish all accepted requests together, or discard the entire physical group.
	if err == nil && accepted != 0 {
		err = tx.Commit(first.ctx)
		committed = err == nil
	}

	// Preserve preflight rejections while assigning physical failure to accepted work.
	if err != nil {
		for i := range results {
			if results[i] == nil {
				results[i] = err
			}
		}
	}
	return group, results, committed
}

// fence waits until every publication admitted before the call completes.
// Rejected publications are reported by their receipts; they must not poison
// unrelated writers or future fences on this volume. The store fence below
// reports physical durability failures.
func (w *publicationWriter) fence(ctx context.Context) error {
	// Capture this fence's admitted prefix before later submissions can extend it.
	locked := w.bcast.Lock()
	accepted := w.stats.Accepted
	locked.Unlock()

	// Wait for that prefix to resolve without including unrelated later work.
	return w.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return w.stats.Completed >= accepted, nil
	})
}

// close stops admission and waits for the drain goroutine to exit.
func (w *publicationWriter) close() {
	// Close admission once, then recheck writer completion after every broadcast.
	_ = w.bcast.Wait(context.Background(), func(broadcast func(), _ func() <-chan struct{}) (bool, error) {
		if !w.closing {
			w.closing = true
			broadcast()
		}
		return w.finished, nil
	})
}

package kvtx

import (
	"bytes"
	"context"
	"slices"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
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
	// entries indexes the publication's verified entries by block key. The
	// last entry for a key wins, as it does when the group writes them.
	entries map[string]*block.PutBatchEntry
	// failed records that the request is known to fail, guarded by the
	// writer's bcast. Readers skip a failed request while its group resolves.
	failed bool
	// receipt resolves only after this request has its final physical result.
	receipt *block.PublicationReceipt
	// bytes is the computed admission cost of the publication.
	bytes int
	// rootOwner keeps this revision alive until its consumer takes a reader pin.
	rootOwner string
}

// publicationWriter serializes atomic publications into bounded physical groups.
// It owns the queue, admission budget, and completion statistics for a volume.
// Every admitted publication stays readable through find until its group
// resolves, so a reader sees admitted blocks before they are applied and loses
// them when their group fails.
type publicationWriter struct {
	// volume owns the shared physical block and metadata transaction domain.
	volume *Volume
	// bcast guards the queue, budget, statistics, and lifecycle state below.
	bcast broadcast.Broadcast
	// queue retains requests in their admission order until their group
	// resolves. The requests before next belong to the group being applied.
	queue []*publicationRequest
	// next indexes the first queued request no group has taken.
	next int
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
	// Require a bounded block delta and distinct head replacements in one
	// volume domain.
	if p == nil || len(p.Heads) == 0 || (p.TrackGC && p.BucketID == "") {
		return 0, errInvalidPublication
	}
	if len(p.Entries) > publicationMaxEntries {
		return 0, block.ErrPublicationTooLarge
	}
	keys := make(map[string]struct{}, len(p.Heads))
	for _, head := range p.Heads {
		if head == nil || head.Replace == nil || head.ObjectStoreID == "" || len(head.Key) == 0 {
			return 0, errInvalidPublication
		}
		key := head.ObjectStoreID + "\x00" + string(head.Key)
		if _, ok := keys[key]; ok {
			return 0, errInvalidPublication
		}
		keys[key] = struct{}{}
	}

	// Account for every retained payload and reference before queue admission.
	size := len(p.BucketID) + len(p.RootName) + p.Root.SizeVT() + 128
	for _, head := range p.Heads {
		size += len(head.Key) + len(head.ObjectStoreID) + 64
	}
	for _, root := range p.Roots {
		size += len(root.Name) + root.Ref.SizeVT() + 64
	}
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

// indexPublicationEntries verifies every entry of p against its reference and
// indexes the entries by block key. A reader is served from the index, so no
// entry that fails verification is ever indexed.
func indexPublicationEntries(p *block.AtomicPublication) (map[string]*block.PutBatchEntry, error) {
	entries := make(map[string]*block.PutBatchEntry, len(p.Entries))
	for _, entry := range p.Entries {
		// Require the entry to match its reference before any group writes it.
		if err := entry.Ref.Validate(false); err != nil {
			return nil, err
		}
		if !entry.Tombstone {
			if len(entry.Data) == 0 {
				return nil, block.ErrEmptyBlock
			}
			if err := entry.Ref.VerifyData(entry.Data, true); err != nil {
				return nil, err
			}
		}

		// Index the entry, the last for a key winning.
		key, err := entry.Ref.MarshalKey()
		if err != nil {
			return nil, err
		}
		entries[string(key)] = entry
	}
	return entries, nil
}

// submit admits a publication under the bounded budget, returning its receipt.
// It blocks while the queue is full and the admission context stays live.
func (w *publicationWriter) submit(ctx context.Context, p *block.AtomicPublication, rootOwner string, release func()) (*block.PublicationReceipt, error) {
	// Validate the publication and its complete admission cost before waiting.
	size, err := publicationSize(p)
	if err != nil {
		return nil, err
	}

	// Verify and index the entries by block key for reads while queued.
	entries, err := indexPublicationEntries(p)
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
					entries:     entries,
					failed:      w.afterFailedLocked(p.After),
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

// takeLocked takes the next queued request for a group, or returns nil when
// every queued request is taken. The request stays queued, and readable,
// until its group resolves. The caller holds bcast.
func (w *publicationWriter) takeLocked() *publicationRequest {
	if w.next == len(w.queue) {
		return nil
	}
	r := w.queue[w.next]
	w.next++
	return r
}

// pin returns the unresolved requests in admission order, never nil for a
// writer. A read scope serves its pinned requests for its whole life, so a
// block it read from a group stays readable after the group applies, as the
// scope's store snapshot does not hold it.
func (w *publicationWriter) pin() []*publicationRequest {
	// A Volume without atomic publication has no admitted requests.
	if w == nil {
		return nil
	}

	// Copy the queue under the writer's lock.
	var pinned []*publicationRequest
	w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		pinned = append(make([]*publicationRequest, 0, len(w.queue)), w.queue...)
	})
	return pinned
}

// find returns the entry for ref of the newest request that has not failed, or
// nil when none writes ref. It searches the pinned requests, or the live queue
// when pinned is nil. A tombstone entry is returned too: the block is absent
// once it applies.
func (w *publicationWriter) find(ref *block.BlockRef, pinned []*publicationRequest) (*block.PutBatchEntry, error) {
	// A Volume without atomic publication has no admitted entries.
	if w == nil {
		return nil, nil
	}

	// Scan the requests under the writer's lock.
	var entry *block.PutBatchEntry
	var err error
	w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// Skip encoding the key while nothing is admitted.
		requests := pinned
		if requests == nil {
			requests = w.queue
		}
		if len(requests) == 0 {
			return
		}

		// Search the newest admission first, as later groups overwrite earlier ones.
		var key []byte
		key, err = ref.MarshalKey()
		if err != nil {
			return
		}
		for _, r := range slices.Backward(requests) {
			if r.failed {
				continue
			}
			if entry = r.entries[string(key)]; entry != nil {
				return
			}
		}
	})
	return entry, err
}

// failDependentsLocked marks failed the queued requests that follow a failed
// request, as each fails with ErrPublicationDependency when its group runs.
// The caller holds bcast.
func (w *publicationWriter) failDependentsLocked() {
	failed := make(map[*block.PublicationReceipt]struct{})
	for _, r := range w.queue {
		if _, ok := failed[r.publication.After]; ok {
			r.failed = true
		}
		if r.failed {
			failed[r.receipt] = struct{}{}
		}
	}
}

// afterFailedLocked reports whether the publication of after is known to
// fail: it resolved with an error, or it is queued and marked failed. A
// request after it is hidden from readers from admission, since it fails with
// ErrPublicationDependency when its group runs. The caller holds bcast, under
// which a request leaves the queue as its receipt resolves.
func (w *publicationWriter) afterFailedLocked(after *block.PublicationReceipt) bool {
	// A request without a predecessor depends on nothing.
	if after == nil {
		return false
	}

	// Check a resolved predecessor's result, else its queued request.
	select {
	case <-after.Done():
		return after.Err() != nil
	default:
	}
	for _, r := range w.queue {
		if r.receipt == after {
			return r.failed
		}
	}
	return false
}

// run drains queued publications one physical group at a time until closing.
func (w *publicationWriter) run() {
	for {
		// Take the next request or publish final shutdown while holding queue state.
		var first *publicationRequest
		var waitCh <-chan struct{}
		w.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			first = w.takeLocked()
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

		// Resolve the physical group before releasing its admission budget,
		// and drop it from the read set as its receipts resolve.
		group, results, committed := w.applyGroup(first)
		w.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			// Count the physical commit.
			if committed {
				w.stats.PhysicalCommits++
			}

			// Hide failed requests from readers before they leave the queue,
			// along with the queued requests that depend on them.
			for i, r := range group {
				r.failed = results[i] != nil
			}
			w.failDependentsLocked()

			// Account for each request and resolve its receipt.
			for i, r := range group {
				if results[i] != nil {
					w.stats.Rejected++
				} else {
					w.stats.CommittedBytes += uint64(r.bytes) // #nosec G115 -- an admission cost is not negative.
				}
				w.stats.Completed++
				w.stats.Pending--
				w.stats.PendingBytes -= r.bytes
				w.stats.PendingEntries -= len(r.publication.Entries)
				r.receipt.Resolve(results[i])
			}
			w.queue = slices.Delete(w.queue, 0, len(group))
			w.next -= len(group)
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
// Ready requests are collected opportunistically, with no latency timer. The
// group also carries the Volume's deferred edits, the edges of released reader
// pins and pending root proofs, which therefore cost no commit of their own.
func (w *publicationWriter) applyGroup(first *publicationRequest) (group []*publicationRequest, results []error, committed bool) {
	// Open one physical transaction shared by the blocks and the reference
	// graph of every request in the group.
	group = []*publicationRequest{first}
	p, err := w.volume.openPhysical(first.ctx)
	if err != nil {
		return group, []error{err}, false
	}
	defer p.tx.Discard()
	tx, blocks, rg := p.tx, p.blocks, p.rg

	// Preflight each request against prior accepted changes in this physical group.
	accepted := 0
	ordered := true
	prior := make(map[*block.PublicationReceipt]error)
	for i := 0; i < len(group); i++ {
		req := group[i]
		p := req.publication
		ctx := req.ctx
		var replacements [][]byte
		result := w.preflight(ctx, req, prior, tx, blocks, &replacements)
		results = append(results, result)
		prior[req.receipt] = result
		if result != nil {
			// Hide the rejected request from readers while the group runs.
			w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				req.failed = true
				w.failDependentsLocked()
			})
		} else {
			// All failures from here onward are physical-group failures: there are no
			// per-request savepoints. Do not carry a partially written request onward.
			err = w.apply(ctx, req, tx, blocks, rg, replacements)
			if err != nil {
				results[i] = err
				break
			}
			accepted++
			ordered = ordered && p.Ordered
		}
		if len(group) < publicationGroupLimit {
			w.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				next := w.takeLocked()
				if next != nil {
					group = append(group, next)
				}
			})
		}
	}

	// Carry the Volume's deferred edits with the accepted requests.
	var edits deferredEdits
	if err == nil && accepted != 0 {
		edits, err = w.volume.applyDeferred(first.ctx, blocks, rg)
	}

	// Publish all accepted requests together, or discard the entire physical group.
	if err == nil && accepted != 0 {
		if ordered && w.volume.ordered != nil {
			err = kvtx.CommitOrdered(first.ctx, tx)
		} else {
			err = tx.Commit(first.ctx)
		}
		committed = err == nil
	}
	w.volume.settleDeferred(edits, committed)

	// Preserve preflight rejections while assigning physical failure to
	// accepted work. A request whose predecessor shared the group failed
	// through it.
	if err != nil {
		for i, req := range group {
			if results[i] != nil {
				continue
			}
			results[i] = err
			if _, ok := prior[req.publication.After]; ok {
				results[i] = block.NewPublicationDependencyError(err)
			}
		}
	}
	return group, results, committed
}

// preflight checks one request's dependency, prepared overlay, head
// comparisons and validation without mutating the group transaction. It
// stores each head's replacement in replacements, in head order.
func (w *publicationWriter) preflight(
	ctx context.Context,
	req *publicationRequest,
	prior map[*block.PublicationReceipt]error,
	tx kvtx.Tx,
	blocks block.StoreOps,
	replacements *[][]byte,
) error {
	// Require the publication this one depends on to have succeeded.
	p := req.publication
	if p.After != nil {
		if previous, ok := prior[p.After]; ok {
			if previous != nil {
				return block.NewPublicationDependencyError(previous)
			}
		} else {
			select {
			case <-p.After.Done():
				if previous := p.After.Wait(ctx); previous != nil {
					return block.NewPublicationDependencyError(previous)
				}
			default:
				return block.ErrPublicationDependency
			}
		}
	}

	// Overlay the verified entries on the group's prior writes.
	prepared := newPublicationOverlay(blocks, req.entries)

	// Compare each head with its committed record.
	for _, head := range p.Heads {
		current, found, err := tx.Get(ctx, w.headKey(head))
		if err != nil {
			return err
		}
		replacement, err := head.Replace(ctx, bytes.Clone(current), found)
		if err != nil {
			return err
		}
		*replacements = append(*replacements, replacement)
	}

	// Validate the prepared overlay.
	if p.Validate != nil {
		return p.Validate(ctx, prepared)
	}
	return nil
}

// apply writes one preflighted request into the group transaction: its
// entries with bucket ownership, its named roots and its head replacements.
func (w *publicationWriter) apply(
	ctx context.Context,
	req *publicationRequest,
	tx kvtx.Tx,
	blocks block.StoreOps,
	rg *block_gc.RefGraph,
	replacements [][]byte,
) error {
	// Write untracked entries directly.
	p := req.publication
	if !p.TrackGC {
		if _, err := blocks.PutBlockBatch(ctx, p.Entries); err != nil {
			return err
		}
		return w.setHeads(ctx, tx, p, replacements)
	}

	// Write the entries owned by the bucket.
	owner := block_gc.BucketIRI(p.BucketID)
	if err := rg.AddRef(ctx, block_gc.NodeGCRoot, owner); err != nil {
		return err
	}
	gc := block_gc.NewGCStoreOpsWithParentAndTraceTask(blocks, rg, owner, block_gc.BucketFlushTask())
	if _, err := gc.PutBlockBatch(ctx, p.Entries); err != nil {
		return err
	}
	if err := gc.FlushPending(ctx); err != nil {
		return err
	}

	// Move the published root under its name. A temporary reader owner
	// bridges publication and consumer handoff, and can be reaped after a
	// crash.
	if p.RootName != "" {
		if err := setBucketRoot(ctx, blocks, rg, p.BucketID, p.RootName, p.Root); err != nil {
			return err
		}
		if req.rootOwner != "" {
			err := rg.ApplyRefBatch(ctx, []block_gc.RefEdge{
				{Subject: block_gc.NodeGCRoot, Object: req.rootOwner},
				{Subject: req.rootOwner, Object: block_gc.BlockIRI(p.Root)},
			}, nil)
			if err != nil {
				return err
			}
		}
		if !p.Root.GetEmpty() {
			if err := rg.AddRef(ctx, block_gc.BlockIRI(p.Root), completeWorldNode); err != nil {
				return err
			}
		}
	}

	// Set the further named roots.
	for _, root := range p.Roots {
		if err := setBucketRoot(ctx, blocks, rg, p.BucketID, root.Name, root.Ref); err != nil {
			return err
		}
	}
	return w.setHeads(ctx, tx, p, replacements)
}

// setHeads writes each head's replacement.
func (w *publicationWriter) setHeads(ctx context.Context, tx kvtx.Tx, p *block.AtomicPublication, replacements [][]byte) error {
	for i, head := range p.Heads {
		if err := tx.Set(ctx, w.headKey(head), replacements[i]); err != nil {
			return err
		}
	}
	return nil
}

// headKey returns the physical key of a head's metadata record.
func (w *publicationWriter) headKey(head *block.AtomicHeadUpdate) []byte {
	return append(w.volume.kvKey.GetObjectStorePrefixByID(head.ObjectStoreID), head.Key...)
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

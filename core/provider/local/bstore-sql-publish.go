package provider_local

import (
	"bytes"
	"context"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/util/broadcast"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/sirupsen/logrus"
)

// Defaults of the SQL-primary publisher's drain and admission bound.
const (
	defaultSQLPublishWorkers   = 4
	defaultSQLPublishChunkRows = 64
	sqlPublishMaxPending       = 64
	sqlPublishMaxPendingBytes  = 64 << 20
)

// sqlPublishWorkers and sqlPublishChunkRows set the drain of the SQL-primary
// stores mounted after they are set, zero for the defaults.
var sqlPublishWorkers, sqlPublishChunkRows atomic.Int32

// SetSQLPublishWorkers sets how many inserts the publisher of each SQL-primary
// store mounted after the call keeps in flight; zero or less uses the default.
// Keep it under the pool's connections, which reads share.
func SetSQLPublishWorkers(n int) {
	sqlPublishWorkers.Store(int32(min(max(n, 0), 1024))) // #nosec G115 -- clamped.
}

// SetSQLPublishChunkRows sets the rows per insert of the publisher of each
// SQL-primary store mounted after the call; zero or less uses the default.
func SetSQLPublishChunkRows(n int) {
	sqlPublishChunkRows.Store(int32(min(max(n, 0), 1<<16))) // #nosec G115 -- clamped.
}

// SQLPublishStats reports the publisher of an SQL-primary block store.
type SQLPublishStats struct {
	// Workers is the number of inserts the drain keeps in flight.
	Workers int
	// ChunkRows is the most rows one insert writes.
	ChunkRows int
	// Publications is the number admitted and not yet resolved.
	Publications int
	// Blocks is the admitted blocks MariaDB does not hold yet.
	Blocks int
	// Bytes is the size of those blocks.
	Bytes int64
	// Admitted counts the publications admitted since the mount.
	Admitted uint64
	// Committed counts the publications whose heads committed.
	Committed uint64
	// Failed counts the publications whose receipts failed.
	Failed uint64
	// Drained counts the publications whose rows all landed.
	Drained uint64
	// DrainWait sums the drained publications' time from admission until
	// their rows landed.
	DrainWait time.Duration
	// CommitWait sums their time from then until the receipt resolved.
	CommitWait time.Duration
	// Statements counts the inserts.
	Statements uint64
	// StatementFailures counts the failed inserts.
	StatementFailures uint64
	// StatementTime sums the inserts' time.
	StatementTime time.Duration
	// LandedHits counts the reads past the overlay the landed blocks answered.
	LandedHits uint64
	// LandedMisses counts the reads past the overlay that went to MariaDB.
	LandedMisses uint64
	// LandedBytes is the bytes the landed blocks hold.
	LandedBytes int64
}

// sqlPublisher publishes the writes of a store whose SQL backend is primary.
// Admission takes a publication's blocks and returns its receipt at once, so
// the World writer lock is released before MariaDB sees a row. Workers then
// insert the blocks of every admitted publication, several statements in
// flight, and the committer writes each publication's heads to the account
// volume in admission order.
//
// In-process readers see an admitted revision through the overlay before
// MariaDB holds its blocks; a restart drops it with its unwritten heads, and
// the World reopens at the last committed revision. The acknowledgement still
// waits for MariaDB: a write's receipt resolves only after its rows land, so
// the publisher moves the insert time off the writer lock, not off the write.
type sqlPublisher struct {
	le *logrus.Entry
	// sql is the store the blocks are inserted into.
	sql block.StoreOps
	// local publishes the heads to the account volume.
	local block.AtomicPublisher
	// workers and chunkRows are the drain's settings.
	workers, chunkRows int

	// bcast guards and signals the fields below.
	bcast broadcast.Broadcast
	// pubs are the admitted publications not yet submitted to local, in
	// admission order.
	pubs []*sqlPublication
	// chunks are the inserts no worker has taken, in admission order.
	chunks []*sqlChunk
	// blocks holds the admitted blocks MariaDB does not hold yet, by ref.
	blocks map[string]*block.PutBatchEntry
	// landed keeps the newest blocks MariaDB holds after they leave blocks.
	landed *sqlLanded
	// pending counts the unresolved publications, pendingBytes their blocks.
	pending      int
	pendingBytes int64
	// locals maps an admitted receipt to the local receipt of its heads,
	// until it resolves.
	locals map[*block.PublicationReceipt]*block.PublicationReceipt
	// closed is set once the store unmounts.
	closed error
	// stats are the counters SQLPublishStats reports.
	stats SQLPublishStats
}

// sqlPublication is an admitted publication.
type sqlPublication struct {
	// heads are written to the account volume after the blocks land.
	heads []*block.AtomicHeadUpdate
	// after is the earlier receipt the publication depends on, or nil.
	after *block.PublicationReceipt
	// ordered allows an ordered rather than durable head commit.
	ordered bool
	// receipt resolves with the head commit's result.
	receipt *block.PublicationReceipt
	// remaining counts the chunks not landed yet.
	remaining int
	// size is the publication's block bytes.
	size int64
	// admitted and drained time the publication.
	admitted, drained time.Time
}

// sqlChunk is one insert of a publication's blocks.
type sqlChunk struct {
	pub     *sqlPublication
	entries []*block.PutBatchEntry
}

// newSQLPublisher builds the publisher of an SQL-primary store with the
// configured drain settings.
func newSQLPublisher(le *logrus.Entry, sql block.StoreOps, local block.AtomicPublisher) *sqlPublisher {
	// Read the settings, defaulting those unset.
	workers := int(sqlPublishWorkers.Load())
	if workers <= 0 {
		workers = defaultSQLPublishWorkers
	}
	chunkRows := int(sqlPublishChunkRows.Load())
	if chunkRows <= 0 {
		chunkRows = defaultSQLPublishChunkRows
	}

	// Build the publisher.
	p := &sqlPublisher{
		le:        le,
		sql:       sql,
		local:     local,
		workers:   workers,
		chunkRows: chunkRows,
		blocks:    make(map[string]*block.PutBatchEntry),
		landed:    newSQLLanded(sqlLandedBytes.Load()),
		locals:    make(map[*block.PublicationReceipt]*block.PublicationReceipt),
	}
	p.stats.Workers, p.stats.ChunkRows = workers, chunkRows
	return p
}

// run drains and commits until ctx ends, then fails every publication not
// yet submitted to local.
func (p *sqlPublisher) run(ctx context.Context) {
	// Start the insert workers, then commit heads until ctx ends.
	done := make(chan struct{})
	for range p.workers {
		go func() {
			p.runWorker(ctx)
			done <- struct{}{}
		}()
	}
	p.runCommitter(ctx)

	// Fail what was not submitted and wait for the workers to stop.
	p.close(block.ErrPublicationClosed)
	for range p.workers {
		<-done
	}
}

// close fails the publications not yet submitted to local with err, in
// admission order, and refuses new ones.
func (p *sqlPublisher) close(err error) {
	// Take the unsubmitted publications and refuse new ones.
	var failed []*sqlPublication
	p.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Refuse new publications with the first error.
		if p.closed == nil {
			p.closed = err
		}

		// Take the queue and uncount it.
		failed, p.pubs, p.chunks = p.pubs, nil, nil
		p.pending -= len(failed)
		for _, pub := range failed {
			p.pendingBytes -= pub.size
		}
		p.stats.Failed += uint64(len(failed))
		broadcast()
	})

	// Fail their receipts outside the lock.
	for _, pub := range failed {
		pub.receipt.Resolve(err)
	}
}

// submit admits a publication: its blocks become readable through the
// overlay and queue for insertion, and the returned receipt resolves once
// its heads commit. Admission waits while the publisher holds its bound, so
// a stalled database stalls writers rather than memory. A publication with a
// tombstone is unsupported: SQL rows are never removed.
func (p *sqlPublisher) submit(ctx context.Context, pub *block.AtomicPublication) (*block.PublicationReceipt, error) {
	// Validate the publication.
	if len(pub.Heads) == 0 {
		return nil, errors.New("sql publication without a head")
	}
	var size int64
	for _, entry := range pub.Entries {
		if entry == nil || entry.Tombstone {
			return nil, block.ErrAtomicPublicationUnsupported
		}
		size += int64(len(entry.Data))
	}

	// Wait for room, then admit it with its chunks. A block already queued
	// by an earlier publication is not inserted again: that publication
	// commits first, after its rows land.
	adm := &sqlPublication{
		heads:    pub.Heads,
		after:    pub.After,
		ordered:  pub.Ordered,
		receipt:  block.NewPublicationReceipt(),
		size:     size,
		admitted: time.Now(),
	}
	for {
		var wait <-chan struct{}
		var err error
		p.bcast.HoldLock(func(broadcast func(), getWait func() <-chan struct{}) {
			if p.closed != nil {
				err = p.closed
				return
			}
			if p.pending != 0 && (p.pending >= sqlPublishMaxPending || p.pendingBytes+size > sqlPublishMaxPendingBytes) {
				wait = getWait()
				return
			}
			p.admitLocked(adm, pub.Entries)
			broadcast()
		})
		if err != nil {
			return nil, err
		}
		if wait == nil {
			return adm.receipt, nil
		}
		select {
		case <-ctx.Done():
			return nil, context.Canceled
		case <-wait:
		}
	}
}

// admitLocked queues adm and the chunks of its entries not already queued.
// The caller holds bcast.
func (p *sqlPublisher) admitLocked(adm *sqlPublication, entries []*block.PutBatchEntry) {
	// Chunk the entries not already queued, each up to chunkRows.
	var chunk []*block.PutBatchEntry
	queue := func() {
		if len(chunk) != 0 {
			p.chunks = append(p.chunks, &sqlChunk{pub: adm, entries: chunk})
			adm.remaining++
			chunk = nil
		}
	}
	for _, entry := range entries {
		key := entry.Ref.MarshalString()
		if p.blocks[key] != nil {
			continue
		}
		p.blocks[key] = entry
		p.stats.Blocks++
		p.stats.Bytes += int64(len(entry.Data))
		chunk = append(chunk, entry)
		if len(chunk) >= p.chunkRows {
			queue()
		}
	}
	queue()

	// A publication with nothing to insert is drained at admission.
	if adm.remaining == 0 {
		adm.drained = adm.admitted
		p.stats.Drained++
	}

	// Queue the publication for its heads' commit.
	p.pubs = append(p.pubs, adm)
	p.pending++
	p.pendingBytes += adm.size
	p.stats.Admitted++
}

// runWorker inserts queued chunks until ctx ends. A failed insert keeps its
// blocks in the overlay and is retried by this worker after a back-off; it
// fails no receipt and blocks no other chunk.
func (p *sqlPublisher) runWorker(ctx context.Context) {
	for {
		// Take the oldest chunk.
		var chunk *sqlChunk
		var wait <-chan struct{}
		p.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			if len(p.chunks) == 0 {
				wait = getWait()
				return
			}
			chunk = p.chunks[0]
			p.chunks[0] = nil
			p.chunks = p.chunks[1:]
		})
		if chunk == nil {
			select {
			case <-ctx.Done():
				return
			case <-wait:
			}
			continue
		}

		// Insert it until it lands, taking the removal generation before the
		// insert that lands it.
		var gen uint64
		for failures := 0; ; {
			gen = block.DurableReadGeneration()
			start := time.Now()
			_, err := p.sql.PutBlockBatch(ctx, chunk.entries)
			took := time.Since(start)
			if ctx.Err() != nil {
				return
			}
			p.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				p.stats.Statements++
				p.stats.StatementTime += took
				if err != nil {
					p.stats.StatementFailures++
				}
			})
			if err == nil {
				break
			}
			failures++
			backoff := drainRetryBackoff(failures)
			p.le.WithError(err).WithField("backoff", backoff).Warn("insert of published blocks failed, retrying")
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}

		// Move the landed blocks from the overlay to the landed blocks and
		// count the chunk. They enter the landed blocks first, so a reader
		// that misses the overlay finds them there or in MariaDB.
		p.landed.add(chunk.entries, gen)
		p.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			for _, entry := range chunk.entries {
				key := entry.Ref.MarshalString()
				if p.blocks[key] == entry {
					delete(p.blocks, key)
					p.stats.Blocks--
					p.stats.Bytes -= int64(len(entry.Data))
				}
			}
			pub := chunk.pub
			if pub.remaining--; pub.remaining == 0 {
				pub.drained = time.Now()
				p.stats.Drained++
				p.stats.DrainWait += pub.drained.Sub(pub.admitted)
			}
			broadcast()
		})
	}
}

// drainRetryBackoff returns the wait after n failed inserts of a chunk in a
// row, doubling from 50ms to at most 5s.
func drainRetryBackoff(n int) time.Duration {
	return min(50*time.Millisecond<<min(n-1, 7), 5*time.Second)
}

// runCommitter submits the heads of each publication to local once its rows
// landed, in admission order, until ctx ends.
//
// Invariants, which SQLPrimary.tla models:
//   - A write is acknowledged only after MariaDB holds its blocks: its receipt
//     resolves with the head commit, submitted after its rows landed.
//   - The stored state never names blocks MariaDB lacks: the heads of
//     publication N are submitted only after N's rows landed and N-1's heads
//     were submitted, and local commits heads in submission order, so a
//     durable head N follows the rows of every publication up to N. A crash
//     before then leaves only rows no head names, which INSERT IGNORE makes
//     harmless when the write is redone.
//   - A failed insert stays queued and retries; it fails no receipt. Only
//     unmount fails a publication, or a failed one it depends on: local fails
//     the heads submitted after a failed receipt they name with
//     ErrPublicationDependency, and the SharedObject state machine fails the
//     admissions built on it.
func (p *sqlPublisher) runCommitter(ctx context.Context) {
	for {
		// Wait for the oldest publication's rows.
		var pub *sqlPublication
		var wait <-chan struct{}
		p.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
			if len(p.pubs) != 0 && p.pubs[0].remaining == 0 {
				pub = p.pubs[0]
				p.pubs[0] = nil
				p.pubs = p.pubs[1:]
				return
			}
			wait = getWait()
		})
		if pub == nil {
			select {
			case <-ctx.Done():
				return
			case <-wait:
			}
			continue
		}

		// Submit its heads after the local commit of the publication it
		// depends on. That publication was admitted earlier, so it was
		// submitted to local or resolved already, unless it came from
		// elsewhere: then wait for it here, holding the order.
		var after *block.PublicationReceipt
		if pub.after != nil {
			p.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				after = p.locals[pub.after]
			})
			if after == nil {
				select {
				case <-ctx.Done():
					p.resolve(pub, nil, block.ErrPublicationClosed)
					return
				case <-pub.after.Done():
				}
				if err := pub.after.Err(); err != nil {
					p.resolve(pub, nil, block.NewPublicationDependencyError(err))
					continue
				}
			}
		}
		// The heads replace records of their own size and carry no block, as
		// the SharedObject's own head write does without a publisher, so the
		// disk reserve does not refuse them: MariaDB holds the blocks.
		local, err := p.local.SubmitAtomic(volume.WithReserveExempt(ctx), &block.AtomicPublication{
			Heads:   pub.heads,
			After:   after,
			Ordered: pub.ordered,
		})
		if err != nil {
			p.resolve(pub, nil, err)
			continue
		}
		p.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			p.locals[pub.receipt] = local
		})
		go func() {
			<-local.Done()
			p.resolve(pub, local, local.Err())
		}()
	}
}

// resolve resolves pub's receipt with err once its heads committed or
// failed, and releases local.
func (p *sqlPublisher) resolve(pub *sqlPublication, local *block.PublicationReceipt, err error) {
	// Retire the publication and count its result.
	p.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Drop it from the pending set.
		delete(p.locals, pub.receipt)
		p.pending--
		p.pendingBytes -= pub.size

		// Count its result.
		if err != nil {
			p.stats.Failed++
		} else {
			p.stats.Committed++
			p.stats.CommitWait += time.Since(pub.drained)
		}
		broadcast()
	})

	// Answer its receipt, then release local's.
	pub.receipt.Resolve(err)
	if local != nil {
		local.Release()
	}
}

// getStats returns the publisher's counters.
func (p *sqlPublisher) getStats() SQLPublishStats {
	// Copy the guarded counters, then read the landed blocks' own.
	var stats SQLPublishStats
	p.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		stats = p.stats
		stats.Publications = p.pending
	})
	stats.LandedHits = p.landed.hits.Load()
	stats.LandedMisses = p.landed.misses.Load()
	stats.LandedBytes = p.landed.held()
	return stats
}

// getPending returns the admitted block for ref that MariaDB does not hold
// yet, or nil.
func (p *sqlPublisher) getPending(ref *block.BlockRef) *block.PutBatchEntry {
	// A nil ref is never pending.
	if ref == nil {
		return nil
	}

	// Look the block up in the overlay.
	key := ref.MarshalString()
	var entry *block.PutBatchEntry
	p.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		entry = p.blocks[key]
	})
	return entry
}

// getLanded returns the landed block for ref, marking ctx's read durable at
// its insert's removal generation, or nil.
func (p *sqlPublisher) getLanded(ctx context.Context, ref *block.BlockRef) *block.PutBatchEntry {
	if ref == nil {
		return nil
	}
	entry, gen := p.landed.get(ref)
	if entry != nil {
		block.MarkDurableRead(ctx, ref, gen)
	}
	return entry
}

// sqlPublishReads serves the admitted blocks MariaDB does not hold yet ahead
// of the store's read chain, so the next write reads the revision it forks
// from, and then the newest landed blocks, so the first reads of a new head
// do not each read a row.
type sqlPublishReads struct {
	block.StoreOps
	pub *sqlPublisher
}

// newSQLPublishReads serves pub's pending blocks ahead of lower.
func newSQLPublishReads(pub *sqlPublisher, lower block.StoreOps) *sqlPublishReads {
	return &sqlPublishReads{StoreOps: lower, pub: pub}
}

// BeginReadOperation scopes the lower store, keeping the overlay ahead of it.
func (r *sqlPublishReads) BeginReadOperation(ctx context.Context) (block.StoreOps, func(), error) {
	scoped, release, err := r.StoreOps.BeginReadOperation(ctx)
	if err != nil {
		return nil, nil, err
	}
	return newSQLPublishReads(r.pub, scoped), release, nil
}

// GetBlock reads a pending block, then a landed block, then the lower store.
func (r *sqlPublishReads) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	if entry := r.pub.getPending(ref); entry != nil {
		return bytes.Clone(entry.Data), true, nil
	}
	if entry := r.pub.getLanded(ctx, ref); entry != nil {
		return bytes.Clone(entry.Data), true, nil
	}
	return r.StoreOps.GetBlock(ctx, ref)
}

// GetStoredBlock reads a pending block with its refs, then a landed block,
// then the lower store.
func (r *sqlPublishReads) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	entry := r.pub.getPending(ref)
	if entry == nil {
		entry = r.pub.getLanded(ctx, ref)
	}
	if entry != nil {
		return &block.StoredBlock{
			Data:      bytes.Clone(entry.Data),
			Refs:      block.CloneBlockRefs(entry.Refs),
			RefsKnown: entry.RefsKnown,
		}, nil
	}
	return r.StoreOps.GetStoredBlock(ctx, ref)
}

// GetBlockExists reports a pending block, then asks the lower store.
func (r *sqlPublishReads) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	if r.pub.getPending(ref) != nil {
		return true, nil
	}
	return r.StoreOps.GetBlockExists(ctx, ref)
}

// GetBlockExistsBatch reports the pending blocks and asks the lower store
// about the rest.
func (r *sqlPublishReads) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	// Answer the pending blocks and collect the rest.
	found := make([]bool, len(refs))
	var rest []*block.BlockRef
	var restAt []int
	for i, ref := range refs {
		if r.pub.getPending(ref) != nil {
			found[i] = true
			continue
		}
		rest = append(rest, ref)
		restAt = append(restAt, i)
	}
	if len(rest) == 0 {
		return found, nil
	}

	// Ask the lower store about the rest.
	lower, err := r.StoreOps.GetBlockExistsBatch(ctx, rest)
	if err != nil {
		return nil, err
	}
	for j, ok := range lower {
		found[restAt[j]] = ok
	}
	return found, nil
}

// StatBlock stats a pending block, then the lower store.
func (r *sqlPublishReads) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	if entry := r.pub.getPending(ref); entry != nil {
		return &block.BlockStat{Ref: ref, Size: int64(len(entry.Data))}, nil
	}
	return r.StoreOps.StatBlock(ctx, ref)
}

// _ is a type assertion
var _ block.StoreOps = (*sqlPublishReads)(nil)

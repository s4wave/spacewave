package packahead

import (
	"container/list"
	"context"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/db/coord/filelock"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/writer"
)

// minBufBytes is the smallest buffer a pack grows to.
const minBufBytes = 1 << 20

// keptBufs bounds the buffers kept for reuse by the next open packs.
const keptBufs = 2

// retryDelay is the wait before the maintainer retries a failed step.
const retryDelay = time.Second

// Log is a directory of append-only packs that new block values go into.
//
// Append writes batches to the open pack and returns once they are durable,
// with one fdatasync shared by the writers waiting at once. A value is
// readable through Get from the moment its batch is written, before it is
// durable, and stays readable while its pack is local. Remove forgets a
// value the caller has freed once its pack is uploaded; the bytes stay in the
// pack, and after a restart the value reads again, which keeps data when
// unsure.
type Log struct {
	// dir holds the pack files.
	dir string
	// releaseDir releases exclusive ownership of dir after the pack files close.
	releaseDir func()
	// opts are the options with their defaults.
	opts Options
	// writeMtx serializes writers to the open pack for one encode and write.
	writeMtx csync.Mutex
	// failureVersion advances when a pack first fails. Append samples it
	// across its waits to detect failure in any pack it touched.
	failureVersion atomic.Uint64
	// bcast guards the fields below and every pack's fields, and wakes
	// writers waiting on a sync and the maintainer.
	bcast broadcast.Broadcast
	// open is the pack that takes batches.
	open *pack
	// spare is the next open pack, created and synced into dir, or nil.
	spare *pack
	// packs indexes every rotated or finalized pack by sequence.
	packs map[uint64]*pack
	// rotated queues packs that can still finalize, oldest first.
	rotated list.List
	// finals holds finalized packs, oldest first.
	finals list.List
	// pending holds finalized packs whose upload is still pending.
	pending list.List
	// held orders rotated and finalized packs that still hold unuploaded
	// data for HoldsBefore, with unknown write times at its head.
	held list.List
	// finalVersion advances when the finalized pack view changes.
	finalVersion uint64
	// localBytes is the total of the packs indexed by sequence.
	localBytes int64
	// finalBytes is the total of the finalized packs.
	finalBytes int64
	// pendingBytes is the total of the finalized packs awaiting upload.
	pendingBytes int64
	// poisoned counts local packs whose sync failed.
	poisoned int
	// table locates every value a local pack holds and no Remove forgot.
	table map[keyTail]location
	// syncs counts the fdatasyncs writers have run.
	syncs uint64
	// syncFailures counts the fdatasyncs that failed.
	syncFailures uint64
	// leaders counts the writers running an fdatasync, which Close waits
	// out before it closes the files.
	leaders int
	// evictions counts claimed evictions that Close waits out.
	evictions int
	// nextSeq is the seq of the next pack created.
	nextSeq uint64
	// bufs are buffers kept for the next open packs.
	bufs [][]byte
	// closed is set by Close.
	closed bool
	// maintErr is the maintainer's last failure, cleared by its next success.
	maintErr error
	// maintDone is set when the maintainer has exited.
	maintDone bool
	// cancel ends the maintainer.
	cancel context.CancelFunc
}

// Open opens the Log in dir, creating dir if needed. It requires exclusive
// ownership of dir until Close and returns immediately if dir is locked.
// Open recovers the verified batches from unfinalized packs, loads the table
// from finalized packs, and opens a new pack. ctx bounds the opening and the
// Log's background work, which also ends at Close.
func Open(ctx context.Context, dir string, opts Options) (*Log, error) {
	// Own the directory before recovery can remove or finalize any pack.
	if err := os.MkdirAll(dir, 0o700); err != nil { // #nosec G703 -- the caller chooses the directory.
		return nil, err
	}
	release, err := filelock.TryOpen(filepath.Join(dir, ".packahead.lock"))
	if err != nil {
		return nil, errors.Wrapf(err, "lock packahead directory %q", dir)
	}
	if release == nil {
		return nil, errors.Errorf("packahead directory %q is already open", dir)
	}
	l := &Log{
		dir: dir, releaseDir: release, opts: opts.withDefaults(), nextSeq: 1,
		packs: make(map[uint64]*pack), table: make(map[keyTail]location),
	}

	// Release ownership on failure after closing any opened packs.
	ok := false
	defer func() {
		if !ok {
			l.closeFiles()
		}
	}()

	// Recover or load each pack in name order.
	block.PrepareBlockRemovalTracking()
	names, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	for _, ent := range names {
		seq, ext, id, valid := parseName(ent.Name())
		if !valid {
			continue
		}
		l.nextSeq = max(l.nextSeq, seq+1)
		path := filepath.Join(dir, ent.Name())
		switch ext {
		case openExt:
			err = l.recoverPack(seq, path)
		case packExt:
			err = l.loadPack(seq, path, id)
		}
		if err != nil {
			return nil, errors.Wrapf(err, "open pack %s", ent.Name())
		}
	}

	// Open the first pack.
	p, err := l.createPack(l.nextSeq)
	if err != nil {
		return nil, err
	}
	l.nextSeq++
	p.state = packOpen
	l.open = p

	// Start the maintainer.
	mctx, cancel := context.WithCancel(ctx)
	l.cancel = cancel
	go l.maintain(mctx)
	ok = true
	return l, nil
}

// parseName splits a pack file name into its seq, extension and pack id.
func parseName(name string) (uint64, string, string, bool) {
	// Read the seq from the first 16 hex digits.
	if len(name) < 16 {
		return 0, "", "", false
	}
	seq, err := strconv.ParseUint(name[:16], 16, 64)
	if err != nil {
		return 0, "", "", false
	}

	// Read the extension and, for a finalized pack, its id.
	rest := name[16:]
	switch {
	case rest == openExt:
		return seq, openExt, "", true
	case strings.HasPrefix(rest, "-") && strings.HasSuffix(rest, packExt) && len(rest) > 1+len(packExt):
		return seq, packExt, rest[1 : len(rest)-len(packExt)], true
	}
	return 0, "", "", false
}

// recoverPack finalizes the verified batches of an unfinalized pack at path,
// or removes it when it holds none. It runs before the Log is
// shared.
func (l *Log) recoverPack(seq uint64, path string) error {
	// Read the pack and keep its good batches.
	f, err := os.OpenFile(path, os.O_RDWR, 0) // #nosec G703 -- a pack name listed in the caller's directory.
	if err != nil {
		return err
	}
	p := &pack{seq: seq, path: path, f: f, state: packRotated}
	data, entries, err := p.recoverFile(l.opts.SyncData, func(disk []byte) ([]entry, int64, bool, error) {
		entries, end, damaged := scanBatches(disk)

		// I2: a Log can reopen in the same process as its old decoded cache.
		// A damaged record can hide the keys, so fence those reads globally.
		if damaged {
			block.InvalidateAllRemovedBlocks()
		}
		return entries, end, false, nil
	})
	if err != nil {
		_ = f.Close()
		return err
	}

	// Remove a pack with no good batch. No append waiter survives Open.
	if len(entries) == 0 {
		_ = f.Close()
		if err := os.Remove(path); err != nil { // #nosec G703 -- a pack name listed in the caller's directory.
			return err
		}
		return dirSync(l.dir)
	}

	// Finalize the kept batches and locate their values.
	p.entries, p.written, p.synced = entries, int64(len(data)), int64(len(data))
	fin, err := p.finalize(l.dir, data, l.opts)
	if err != nil {
		_ = f.Close()
		return err
	}
	l.applyFinal(p, fin)
	for _, e := range entries {
		l.table[tailOf(e.key)] = location{p: p, off: uint32(e.off), size: uint32(e.size)} //nolint:gosec // bounded by the pack size.
	}
	p.entries = nil
	return nil
}

// loadPack opens a finalized pack and locates its values from its index, read
// with one read of its tail. It runs before the Log is shared.
func (l *Log) loadPack(seq uint64, path, id string) error {
	// Read the pack's index.
	f, err := os.Open(path) // #nosec G703 -- a pack name listed in the caller's directory.
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	index, err := packfile.ReadIndex(f, uint64(info.Size())) //nolint:gosec // non-negative.
	if err != nil {
		_ = f.Close()
		return err
	}

	// Locate the values and rebuild the bloom filter the upload sends.
	p := &pack{seq: seq, path: path, f: f, state: packFinal, id: id, written: info.Size(), blocks: len(index)}
	bf := writer.DefaultPolicy().NewBloomFilter(uint64(len(index)))
	for _, e := range index {
		bf.Add(e.GetKey())
		l.table[tailOf(e.GetKey())] = location{p: p, off: uint32(e.GetOffset()), size: uint32(e.GetSize())} //nolint:gosec // bounded by the pack size.
	}
	p.bloom, err = bloom.NewBloom(bf).MarshalVT()
	if err != nil {
		_ = f.Close()
		return err
	}
	l.trackFinal(p)
	return nil
}

// createPack creates pack seq's file and syncs it into the directory, so a
// batch written to it is never in a file a crash drops.
func (l *Log) createPack(seq uint64) (*pack, error) {
	// Create the file, failing if a pack already has its name.
	path := filepath.Join(l.dir, openName(seq))
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G703 -- a pack name in the caller's directory.
	if err != nil {
		return nil, err
	}

	// Make its directory entry durable before any batch is written to it.
	if err := dirSync(l.dir); err != nil {
		_ = f.Close()
		_ = os.Remove(path) // #nosec G703 -- the pack just created.
		return nil, errors.Wrap(err, "sync pack directory")
	}
	return &pack{seq: seq, path: path, f: f, state: packSpare}, nil
}

// Append writes blocks and returns once every one is durable in a local
// pack. A block whose key the Log already holds is not written again; Append
// waits for that copy to be durable instead. Blocks that do not fit the open
// pack go to the next.
func (l *Log) Append(ctx context.Context, blocks []Block) error {
	// Write the blocks, collecting the syncs they wait for.
	waits, version, err := l.write(ctx, blocks, nil)
	if err != nil {
		return err
	}

	// Wait for each pack the blocks reached to be durable through them.
	for _, w := range waits {
		if err := l.waitSynced(ctx, w.p, w.end); err != nil {
			return err
		}
		if err := l.checkAppendFailures(waits, &version); err != nil {
			return err
		}
	}
	return nil
}

// AppendUnsynced writes blocks as Append does and returns once they are
// written, without waiting for a sync. They become durable at the next sync
// a writer leads or when the maintainer finalizes their pack; a crash before
// that drops them, and recovery truncates the pack to the batches that
// reached the drive. Only a Log of copies, such as a cache, appends this way.
func (l *Log) AppendUnsynced(ctx context.Context, blocks []Block) error {
	_, _, err := l.write(ctx, blocks, nil)
	return err
}

// write checks blocks and writes them under writeMtx, returning the syncs
// they wait for and the failure version before the write. A held key
// located in pack replace is written again and relocated, as a carry needs;
// every other held key is skipped.
func (l *Log) write(ctx context.Context, blocks []Block, replace *pack) ([]syncWait, uint64, error) {
	// Check every key and size before writing any block.
	for _, b := range blocks {
		if len(b.Key) < block.BlockDigestTailLen {
			return nil, 0, ErrKeyTooShort
		}
		if int64(recordLen([]Block{b})+len(b.Value)+len(b.Key)+indexEntryReserve+indexTrailerReserve) > l.opts.MaxPackBytes {
			return nil, 0, ErrValueTooLarge
		}
	}

	// Refuse blocks that would leave less free disk than the reserve.
	if err := l.checkReserve(blocks, replace); err != nil {
		return nil, 0, err
	}

	// Write the blocks under writeMtx, collecting the syncs they wait for.
	release, err := l.writeMtx.Lock(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer release()
	version := l.failureVersion.Load()
	waits, err := l.writeBlocks(ctx, blocks, replace)
	return waits, version, err
}

// checkAppendFailures confirms all touched packs stayed healthy through a
// single failure version. The common path needs only one atomic load.
func (l *Log) checkAppendFailures(waits []syncWait, version *uint64) error {
	for current := l.failureVersion.Load(); current != *version; current = l.failureVersion.Load() {
		// Failures are permanent. Read every touched pack in bounded holds,
		// then repeat if a pack could have failed while the scan was running.
		for batch := range slices.Chunk(waits, locationBatchSize) {
			var err error
			l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				for _, w := range batch {
					if err = w.p.err; err != nil {
						return
					}
				}
			})
			if err != nil {
				return errors.Wrap(err, "sync pack")
			}
		}
		*version = current
	}
	return nil
}

// checkReserve returns ErrReserve when writing blocks would leave less free
// disk than Options.ReserveBytes, and the measurement's error when the free
// disk cannot be read. A block held in pack replace counts as written.
func (l *Log) checkReserve(blocks []Block, replace *pack) error {
	// Keep no reserve when none is set.
	if l.opts.ReserveBytes <= 0 {
		return nil
	}

	// Measure the free disk and the bytes of the blocks the Log does not
	// hold, which Append writes; a held block writes nothing.
	free, err := l.opts.FreeBytes(l.dir)
	if err != nil {
		return errors.Wrap(err, "measure free disk")
	}
	var need uint64
	for batch := range slices.Chunk(blocks, locationBatchSize) {
		l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			for _, b := range batch {
				if !l.skipsLocked(tailOf(b.Key), replace) {
					need += uint64(len(b.Key) + len(b.Value)) //nolint:gosec // lengths are not negative.
				}
			}
		})
	}
	if free < need || free-need < uint64(l.opts.ReserveBytes) { //nolint:gosec // positive.
		return ErrReserve
	}
	return nil
}

// syncWait is a pack offset a caller waits to be durable.
type syncWait struct {
	// p is the pack.
	p *pack
	// end is the offset that must be durable.
	end int64
}

// writeBlocks writes blocks as batches to the open pack, rotating when one
// fills, and returns the syncs they need. A held key located in replace is
// written again. The caller holds writeMtx.
func (l *Log) writeBlocks(ctx context.Context, blocks []Block, replace *pack) ([]syncWait, error) {
	// Index the waits so a held value adds fixed work under bcast.
	var waits []syncWait
	waitIndex := make(map[*pack]int)
	addWait := func(p *pack, end int64) {
		if i, ok := waitIndex[p]; ok {
			waits[i].end = max(waits[i].end, end)
			return
		}
		waitIndex[p] = len(waits)
		waits = append(waits, syncWait{p: p, end: end})
	}
	for len(blocks) != 0 {
		// I4: a writer plans at most one bounded batch per lock hold,
		// including duplicate keys that consume no pack bytes.
		var p *pack
		var plan batchPlan
		var err error
		var held [locationBatchSize]syncWait
		n := 0
		l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if l.closed {
				err = ErrClosed
				return
			}
			p = l.open
			plan = l.planBatchLocked(p, blocks[:min(len(blocks), locationBatchSize)], replace, func(p *pack, end int64) {
				held[n] = syncWait{p: p, end: end}
				n++
			})
		})
		if err != nil {
			return nil, err
		}

		// Merge waits outside the log lock even when the append spans many packs.
		for _, wait := range held[:n] {
			addWait(wait.p, wait.end)
		}

		// Rotate when the open pack failed or is too full for the next block.
		if plan.rotate {
			if err := l.rotate(); err != nil {
				return nil, err
			}
			blocks = blocks[plan.skipped:]
			continue
		}

		// Write the batch and locate its values.
		if len(plan.batch) != 0 {
			end, err := l.writeBatch(p, plan.batch)
			if err != nil {
				return nil, err
			}
			addWait(p, end)
		}
		blocks = blocks[plan.consumed:]
	}
	return waits, nil
}

// batchPlan is the part of a call's blocks the open pack takes next.
type batchPlan struct {
	// batch are the blocks to write.
	batch []Block
	// consumed counts the leading blocks the plan covers, held ones included.
	consumed int
	// skipped counts leading held blocks before a rotation.
	skipped int
	// rotate is set when the open pack must rotate before any write.
	rotate bool
}

// planBatchLocked chooses the leading blocks pack p takes: held keys add a
// wait for their copy and are skipped, unless their copy is in replace, and
// the rest are taken while the pack stays within its bounds. The caller holds
// bcast.
func (l *Log) planBatchLocked(p *pack, blocks []Block, replace *pack, addWait func(*pack, int64)) batchPlan {
	// A failed pack takes nothing more.
	if p.err != nil {
		return batchPlan{rotate: true}
	}

	// Take blocks while the batch fits, tracking the record's size.
	var plan batchPlan
	var entryBytes, data, keyBytes int64
	for plan.consumed < len(blocks) {
		b := blocks[plan.consumed]
		if t := tailOf(b.Key); l.skipsLocked(t, replace) {
			loc := l.table[t]
			loc.p.revive(t)
			if loc.p.state != packFinal {
				addWait(loc.p, loc.end())
			}
			plan.consumed++
			continue
		}
		n := len(plan.batch) + 1
		nextEntry := entryBytes + int64(protobuf_go_lite.SizeOfVarint(uint64(len(b.Key)))+len(b.Key)+protobuf_go_lite.SizeOfVarint(uint64(len(b.Value))))
		recLen := int64(recordHeaderLen+recordFixedBody+recordTrailerLen+protobuf_go_lite.SizeOfVarint(uint64(n))) + nextEntry
		need := p.written + recLen + data + int64(len(b.Value)) + p.indexReserve(n, keyBytes+int64(len(b.Key)))
		if len(p.entries)+n > l.opts.MaxPackBlocks || need > l.opts.MaxPackBytes {
			break
		}
		plan.batch = append(plan.batch, b)
		entryBytes, data, keyBytes = nextEntry, data+int64(len(b.Value)), keyBytes+int64(len(b.Key))
		plan.consumed++
	}

	// Rotate when the next block fits no further in this pack.
	if len(plan.batch) == 0 && plan.consumed < len(blocks) {
		return batchPlan{rotate: true, skipped: plan.consumed}
	}
	return plan
}

// holdsLocked reports whether the table holds a copy of t that a put may
// skip writing. The caller holds bcast.
func (l *Log) holdsLocked(t keyTail) bool {
	loc, ok := l.table[t]
	return ok && (loc.p.state == packFinal || loc.p.err == nil)
}

// skipsLocked reports whether a put skips writing t: the table holds a copy
// a put may skip, outside pack replace. The caller holds bcast.
func (l *Log) skipsLocked(t keyTail, replace *pack) bool {
	return l.holdsLocked(t) && (replace == nil || l.table[t].p != replace)
}

// dropLocationLocked forgets the table's current location for t. The caller
// holds bcast and fences decoded reads before releasing it.
func (l *Log) dropLocationLocked(t keyTail, loc location) {
	loc.p.revive(t)
	delete(l.table, t)
}

// writeBatch encodes batch at the end of open pack p, writes it, and locates
// its values, returning the pack's new end. The caller holds writeMtx.
func (l *Log) writeBatch(p *pack, batch []Block) (int64, error) {
	// Encode the record and values into the pack's buffer.
	start := p.written
	end := start + int64(recordLen(batch)+dataLen(batch))
	buf := l.growBuf(p, end)
	entries := encodeBatch(buf[start:end], start, batch)

	// Write once and grow the entry slice outside the log lock. Only this
	// writer changes the slice, and earlier entries remain immutable.
	_, werr := p.f.WriteAt(buf[start:end], start)
	var nextEntries []entry
	if werr == nil {
		nextEntries = append(p.entries, entries...)
	}

	// Publish the batch, or fail the pack so the next write rotates.
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// A failed write fails the pack, and a pack whose sync failed since
		// the batch was planned takes it no further.
		defer broadcast()
		if werr != nil {
			l.failLocked(p, werr)
			return
		}
		if p.err != nil {
			werr = errors.Wrap(p.err, "pack failed")
			return
		}

		// Advance the pack and locate each value.
		if p.firstAt.IsZero() {
			p.firstAt = time.Now()
		}
		p.written = end
		p.entries = nextEntries
		for _, e := range entries {
			p.keyBytes += int64(len(e.key))
			t := tailOf(e.key)
			if loc, ok := l.table[t]; ok {
				block.InvalidateRemovedBlockStripe(e.stripe)
				l.dropLocationLocked(t, loc)
			}
			l.table[t] = location{p: p, off: uint32(e.off), size: uint32(e.size)} //nolint:gosec // bounded by the pack size.
		}
	})
	if werr != nil {
		// Keep writeMtx until the failed open pack's locations are withdrawn.
		l.forgetUnsynced(p)
		return 0, errors.Wrap(werr, "write batch")
	}
	return end, nil
}

// growBuf returns p's buffer with room for end bytes, growing it when needed.
// The caller holds writeMtx, so only it changes p.buf.
func (l *Log) growBuf(p *pack, end int64) []byte {
	// Use the buffer as it is while it has room.
	if int64(len(p.buf)) >= end {
		return p.buf
	}

	// Grow into a larger buffer holding the written bytes, and publish it.
	size := max(int64(len(p.buf))*2, end, minBufBytes)
	size = min(size, max(l.opts.MaxPackBytes, end))
	grown := make([]byte, size)
	copy(grown, p.buf[:p.written])
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		p.buf = grown
	})
	return grown
}

// failLocked freezes a pack's accepted sync frontier and stops new batches.
// The caller holds bcast. The failure owner withdraws unsynced locations
// outside this lock hold before releasing its claim on the pack.
func (l *Log) failLocked(p *pack, err error) {
	if p.err == nil {
		p.err = err
		l.failureVersion.Add(1)
	}
}

// waitSynced implements D1: a waiter can acknowledge a synced batch only
// while its pack has no known failure. A failed write sends the pack through
// recovery, which can discard bytes covered by the old sync watermark.
// The first waiter without a running sync syncs the written batches.
func (l *Log) waitSynced(ctx context.Context, p *pack, end int64) error {
	for {
		// Read the pack's sync state and lead a sync if none runs.
		var done, lead bool
		var target int64
		var err error
		var waitCh <-chan struct{}
		l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			switch {
			case p.err != nil:
				err = p.err
			case p.synced >= end:
				done = true
			case l.closed:
				err = ErrClosed
			case !p.syncing:
				p.syncing, lead, target = true, true, p.written
				l.leaders++
			default:
				waitCh = getWaitCh()
			}
		})
		if done {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "sync pack")
		}

		// Lead: sync the pack and publish the new durable mark.
		if lead {
			serr := l.opts.SyncData(p.f)
			if serr != nil {
				l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
					l.poisonLocked(p, serr)
				})
				l.forgetUnsynced(p)
			}
			l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				// End the sync, which Close may wait out.
				p.syncing = false
				l.leaders--
				l.syncs++

				// A later write failure can freeze the frontier during the sync.
				if p.err == nil {
					p.synced = max(p.synced, target)
				}
				broadcast()
			})
			continue
		}

		// Follow: wait for the running sync.
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		}
	}
}

// rotate moves the writer to the spare pack, creating it when the maintainer
// has not, and leaves the old pack to the maintainer to finalize. The caller
// holds writeMtx.
func (l *Log) rotate() error {
	// Take the spare, or reserve a seq to create one.
	var spare *pack
	var seq uint64
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		spare, l.spare = l.spare, nil
		if spare == nil {
			seq = l.nextSeq
			l.nextSeq++
		}
	})
	if spare == nil {
		var err error
		spare, err = l.createPack(seq)
		if err != nil {
			return err
		}
	}

	// Swap the open pack for the spare.
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Leave the old pack to the maintainer.
		l.trackRotated(l.open)

		// Open the spare with a kept buffer.
		spare.state = packOpen
		if n := len(l.bufs); n != 0 {
			spare.buf, l.bufs = l.bufs[n-1], l.bufs[:n-1]
		}
		l.open = spare
		broadcast()
	})
	return nil
}

// Get returns the value under key, from the buffer of a pack not yet
// finalized or from a local pack file, and whether a local pack holds it.
func (l *Log) Get(key []byte) ([]byte, bool, error) {
	value, found, _, err := l.GetWithDurability(key)
	return value, found, err
}

// GetWithDurability returns the value and whether the location that served
// it was synced when looked up. The caller may admit only a synced read to
// the shared decoded-block cache. A hit sets the value's read bit, which
// EvictCarry keeps a value by.
func (l *Log) GetWithDurability(key []byte) (value []byte, found, synced bool, err error) {
	// Capture the location and its sync state in the same lock acquisition.
	if len(key) < block.BlockDigestTailLen {
		return nil, false, false, ErrKeyTooShort
	}
	var loc location
	var buf []byte
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		t := tailOf(key)
		loc, found = l.table[t]
		if found {
			if !loc.read {
				loc.read = true
				l.table[t] = loc
			}
			synced = loc.synced()
			if loc.p.buf != nil {
				buf = loc.p.buf[loc.off:loc.end()]
				loc.p.bufReaders.Add(1)
			}
		}
	})
	if !found {
		return nil, false, false, nil
	}

	// I4: clone the value without holding bcast. A pack with a borrowed
	// buffer leaves that buffer out of the reuse pool at finalization.
	if buf != nil {
		out := slices.Clone(buf)
		loc.p.bufReaders.Add(-1)
		return out, true, synced, nil
	}

	// Read it from the pack file. A pack evicted since the lookup no longer
	// holds the value, which the remote holds instead.
	out := make([]byte, loc.size)
	if _, err := loc.p.f.ReadAt(out, int64(loc.off)); err != nil {
		var gone bool
		l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			gone = loc.p.state == packGone
		})
		if gone {
			return nil, false, false, nil
		}
		return nil, false, false, errors.Wrapf(err, "read pack %s", seqText(loc.p.seq))
	}
	return out, true, synced, nil
}

// Has reports whether a local pack holds the value under key.
func (l *Log) Has(key []byte) bool {
	if len(key) < block.BlockDigestTailLen {
		return false
	}
	var found bool
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		_, found = l.table[tailOf(key)]
	})
	return found
}

// HasDurable reports whether a local pack holds the value under key durably:
// its pack is finalized, or a sync covered its batch. A writer may skip
// storing a value only on this, since Has also reports a value whose batch is
// written and not yet synced, which a crash or a failed sync loses.
func (l *Log) HasDurable(key []byte) bool {
	if len(key) < block.BlockDigestTailLen {
		return false
	}
	var durable bool
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		t := tailOf(key)
		loc, ok := l.table[t]
		if ok && !loc.p.poisoned {
			_, removed := loc.p.removed[t]
			durable = !removed && loc.synced()
		}
	})
	return durable
}

// Remove forgets the value under key, which the caller has freed, once the
// remote holds its pack. Its bytes stay in their pack. A value whose pack
// is not uploaded is marked removed and stays readable until the pack
// uploads: a writer that found it held may have stored it again, and until
// the upload no other copy serves it. A marked value reports no durable
// copy, so a put of it again appends it, which clears the mark and keeps it
// past the upload.
func (l *Log) Remove(key []byte) {
	// A short key cannot name a location.
	if len(key) < block.BlockDigestTailLen {
		return
	}

	// Prepare the stripe before taking the lock shared with reads and writes.
	t := tailOf(key)
	stripe := block.BlockRemovalStripeForDigest(key)

	// Drop the location now if its pack uploaded, or defer to the upload.
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		// A missed location has nothing to remove.
		loc, ok := l.table[t]
		if !ok {
			return
		}

		// An uploaded pack can release its location now.
		if loc.p.uploaded {
			block.InvalidateRemovedBlockStripe(stripe)
			l.dropLocationLocked(t, loc)
			return
		}

		// Remember the deferred removal in the pack that holds it.
		loc.p.deferRemoval(t, stripe)
	})
}

// maintain runs the background steps until ctx ends: it opens the spare,
// finalizes rotated packs, and rotates a lingering open pack.
func (l *Log) maintain(ctx context.Context) {
	defer l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		l.maintDone = true
		broadcast()
	})
	for {
		// Choose the next step, or the time and channel to wait on.
		var step func(context.Context) error
		var waitCh <-chan struct{}
		var due time.Time
		l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
			step, due = l.nextStepLocked()
		})

		// Run the step, retrying after a delay when it fails.
		if step != nil {
			err := step(ctx)
			if ctx.Err() != nil {
				return
			}
			l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				l.maintErr = err
				broadcast()
			})
			if err != nil {
				// Error publication closes the watch. Backoff waits on its
				// own deadline so that publication and new writes cannot bypass it.
				timer := time.NewTimer(retryDelay)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			continue
		}

		// Wait for a change or the linger deadline.
		var timer *time.Timer
		var expired <-chan time.Time
		if !due.IsZero() {
			timer = time.NewTimer(time.Until(due))
			expired = timer.C
		}
		select {
		case <-ctx.Done():
		case <-waitCh:
		case <-expired:
		}
		if timer != nil {
			timer.Stop()
		}
		if ctx.Err() != nil {
			return
		}
	}
}

// nextStepLocked returns the maintainer's next step, or nil and the time the
// open pack lingers until. The caller holds bcast.
func (l *Log) nextStepLocked() (func(context.Context) error, time.Time) {
	// Finalize the oldest rotated pack first, so packs finalize in order.
	if head := l.rotated.Front(); head != nil {
		p := head.Value.(*pack)
		return func(ctx context.Context) error { return l.finalizeRotated(ctx, p) }, time.Time{}
	}

	// Open the spare, then rotate a lingering open pack.
	if l.spare == nil {
		return l.makeSpare, time.Time{}
	}
	if l.opts.Linger <= 0 || len(l.open.entries) == 0 {
		return nil, time.Time{}
	}
	due := l.open.firstAt.Add(l.opts.Linger)
	if time.Now().Before(due) {
		return nil, due
	}
	return l.rotateLingering, time.Time{}
}

// makeSpare creates the next pack and offers it to the writer.
func (l *Log) makeSpare(context.Context) error {
	// Reserve a seq and create the pack.
	var seq uint64
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		seq = l.nextSeq
		l.nextSeq++
	})
	p, err := l.createPack(seq)
	if err != nil {
		return err
	}

	// Offer it to the writer, or close it when the Log closed meanwhile.
	var offered bool
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// A writer that already reserved a newer pack must keep the sequence
		// order even if this creation finishes first.
		if offered = !l.closed && l.nextSeq == p.seq+1; offered {
			l.spare = p
			broadcast()
		}
	})
	if !offered {
		_ = p.f.Close()
		return os.Remove(p.path) // #nosec G703 -- an empty pack in the Log directory.
	}
	return nil
}

// rotateLingering rotates the open pack once its first batch is older than
// the linger.
func (l *Log) rotateLingering(ctx context.Context) error {
	// Hold the writer out while checking the deadline again.
	release, err := l.writeMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	var due bool
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		due = len(l.open.entries) != 0 && !time.Now().Before(l.open.firstAt.Add(l.opts.Linger))
	})

	// Rotate a pack that still lingers.
	if !due {
		return nil
	}
	return l.rotate()
}

// finalizeRotated makes rotated pack p durable and finalizes it, recovering
// it from its file first when a write or sync failed.
func (l *Log) finalizeRotated(ctx context.Context, p *pack) error {
	// Sync any batch a writer wrote but no waiter synced.
	var written int64
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		written = p.written
	})
	syncErr := l.waitSynced(ctx, p, written)
	if ctx.Err() != nil {
		return context.Cause(ctx)
	}

	// Wait out any sync that was still running when a write failed. The
	// recovery scan must see its outcome before retracting the watermark.
	var data []byte
	var entries []entry
	var failed, poisoned bool
	if err := l.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		if p.syncing {
			return false, nil
		}
		failed, poisoned = p.err != nil, p.poisoned
		data, entries = p.buf[:p.written], p.entries
		return true, nil
	}); err != nil {
		return err
	}
	if syncErr != nil && !failed {
		return syncErr
	}

	// A pack a sync failed is never finalized here: its file may read back
	// pages the disk lost. The next Open recovers it from its batch checksums.
	if poisoned {
		return nil
	}

	// A failed data sync here poisons the pack as a writer's would: it may
	// have dropped the pages of a batch written but never synced.
	opts := l.opts
	var syncFailed bool
	opts.SyncData = func(f *os.File) error {
		err := l.opts.SyncData(f)
		syncFailed = syncFailed || err != nil
		return err
	}
	poison := func(err error) error {
		if syncFailed {
			l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				l.poisonLocked(p, err)
				broadcast()
			})
			l.forgetUnsynced(p)
		}
		return err
	}

	// Recover a failed pack from its file, unless an earlier try renamed it:
	// the renamed file ends in the index, which recovery would truncate. A
	// pack an earlier try dropped has no file left to recover.
	if failed && p.renamed == nil && !p.dropped {
		scan := func(disk []byte) ([]entry, int64, bool, error) {
			// Fence discarded locations before repairing their framing.
			kept, end, damaged := scanWrittenBatches(disk, data, entries)
			l.forgetDamagedBatches(p, entries, kept, end)
			if !damaged {
				return kept, end, false, nil
			}

			// A crash can leave the old name even after the index was synced.
			// Make the same kept batches reachable from that name first.
			changed, err := p.sealRecoveryGaps(disk, entries, kept, end)
			return kept, end, changed, err
		}
		var err error
		data, entries, err = p.recoverFile(opts.SyncData, scan)
		if err != nil {
			return poison(err)
		}
		// A finalization retry must use the same recovered bytes and offsets.
		l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			l.localBytes += int64(len(data)) - p.written
			p.buf, p.entries, p.written = data, entries, int64(len(data))
		})

		// Reinstall surviving locations in bounded steps. A recovered pack
		// stays staged to readers until finalization commits.
		for batch := range slices.Chunk(entries, locationBatchSize) {
			l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				l.locateRecoveredLocked(p, batch)
			})
		}
	}

	// A pack with no good batch has nothing to finalize.
	if len(entries) == 0 {
		return l.dropEmpty(p)
	}

	// Finalize, then serve the pack from its file and keep its buffer.
	fin, err := p.finalize(l.dir, data, opts)
	if err != nil {
		return poison(err)
	}
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Publish the finalized pack after all its locations are installed.
		l.applyFinal(p, fin)

		// Keep the buffer for a later open pack and serve from the file.
		if !failed && p.bufReaders.Load() == 0 && len(l.bufs) < keptBufs {
			l.bufs = append(l.bufs, p.buf)
		}
		p.buf, p.entries = nil, nil
		broadcast()
	})
	return nil
}

// forgetDamagedBatches fences locations that the recovery scan discarded.
// The rotated pack's entry lists are immutable during the scan.
func (l *Log) forgetDamagedBatches(p *pack, original, kept []entry, end int64) {
	// Retract the end before an unsuccessful recovery can poison the pack.
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		p.synced = min(p.synced, end)
	})

	// Compare the record offsets outside the lock. Retained bytes keep the
	// same offsets, so an in-flight read does not need to move with them.
	var damaged [locationBatchSize]entry
	n := 0
	drop := func() {
		l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			l.dropDamagedEntriesLocked(p, damaged[:n])
		})
		n = 0
	}
	for _, e := range original {
		for len(kept) != 0 && kept[0].batch < e.batch {
			kept = kept[1:]
		}
		if len(kept) != 0 && kept[0].batch == e.batch {
			continue
		}
		damaged[n] = e
		n++
		if n == len(damaged) {
			drop()
		}
	}
	if n != 0 {
		drop()
	}
}

// dropDamagedEntriesLocked implements I2: publish the removal generation
// before readers can observe a dropped location or recovery can truncate it.
func (l *Log) dropDamagedEntriesLocked(p *pack, damaged []entry) {
	var removals block.BlockRemovalBatch
	for _, e := range damaged {
		t := tailOf(e.key)
		if loc, ok := l.table[t]; ok && loc.p == p && int64(loc.off) == e.off {
			removals.Add(e.stripe)
			l.dropLocationLocked(t, loc)
		}
	}
	removals.Publish()
}

// forgetUnsynced withdraws the entries above a failed pack's frozen sync
// frontier. Its caller still holds the writer, sync or maintenance claim,
// so recovery cannot replace these entry offsets during the walk.
func (l *Log) forgetUnsynced(p *pack) {
	// Identify the unsynced suffix without searching under bcast.
	var entries []entry
	var synced int64
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		entries, synced = p.entries, p.synced
	})
	first := sort.Search(len(entries), func(i int) bool {
		return entries[i].off+entries[i].size > synced
	})

	// I2 and I4: publish the exact removal stripes as each fixed batch drops.
	for batch := range slices.Chunk(entries[first:], locationBatchSize) {
		l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			l.dropDamagedEntriesLocked(p, batch)
		})
	}
}

// locateRecoveredLocked locates the values recovery kept in pack p that the
// table has no other copy of, after failLocked forgot those not yet durable.
// The caller holds bcast.
func (l *Log) locateRecoveredLocked(p *pack, entries []entry) {
	for _, e := range entries {
		t := tailOf(e.key)
		if loc, ok := l.table[t]; !ok || loc.p == p {
			l.table[t] = location{p: p, off: uint32(e.off), size: uint32(e.size)} //nolint:gosec // bounded by the pack size.
		}
	}
}

// dropEmpty removes rotated pack p, which holds no good batch.
func (l *Log) dropEmpty(p *pack) error {
	// Close and remove the file and make the removal durable. A retry after a
	// failed remove or directory sync does not recover the closed file again:
	// it finds the file gone and syncs the directory.
	if !p.dropped {
		_ = p.f.Close()
		p.dropped = true
	}
	if err := os.Remove(p.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := dirSync(l.dir); err != nil {
		return err
	}

	// Forget the pack.
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		l.forgetRotated(p)
		p.state, p.buf, p.entries = packGone, nil, nil
		broadcast()
	})
	return nil
}

// Close stops the background work, waits for the writer in progress and the
// syncs running, then closes the pack files and releases the directory lock.
// A writer waiting for a sync that has not started returns ErrClosed. Written
// batches stay in their packs for the next Open to recover.
func (l *Log) Close() {
	// Refuse new work and stop the maintainer.
	var waitCh <-chan struct{}
	var done bool
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		l.closed = true
		broadcast()
	})
	l.cancel()
	for {
		l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			done, waitCh = l.maintDone, getWaitCh()
		})
		if done {
			break
		}
		<-waitCh
	}

	// Wait for active writers, syncs and evictions to release the files.
	// Once closed is set, no new operation can claim a file for writing.
	release, err := l.writeMtx.Lock(context.Background())
	if err == nil {
		defer release()
	}
	for {
		l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			done, waitCh = l.leaders == 0 && l.evictions == 0, getWaitCh()
		})
		if done {
			break
		}
		<-waitCh
	}
	l.closeFiles()
}

// closeFiles closes the registry and releases directory ownership, after all
// writers have stopped or during Open before the registry is shared.
func (l *Log) closeFiles() {
	// The registry no longer changes, so disk closes need no log lock.
	for _, p := range l.packs {
		_ = p.f.Close()
	}
	for _, p := range [2]*pack{l.open, l.spare} {
		if p != nil {
			_ = p.f.Close()
		}
	}

	// Leave the lock file in place so all openers lock the same inode.
	if l.releaseDir != nil {
		l.releaseDir()
		l.releaseDir = nil
	}
}

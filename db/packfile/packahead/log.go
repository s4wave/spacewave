package packahead

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	protobuf_go_lite "github.com/aperturerobotics/protobuf-go-lite"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/writer"
)

// minBufBytes is the smallest buffer a pack grows to.
const minBufBytes = 1 << 20

// keptBufs bounds the buffers kept for reuse by the next open packs.
const keptBufs = 2

// salvageMargin is the free disk a salvage keeps beyond its copy.
const salvageMargin = 1 << 20

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
	// opts are the options with their defaults.
	opts Options
	// writeMtx serializes writers to the open pack for one encode and write.
	writeMtx csync.Mutex
	// bcast guards the fields below and every pack's fields, and wakes
	// writers waiting on a sync and the maintainer.
	bcast broadcast.Broadcast
	// open is the pack that takes batches.
	open *pack
	// spare is the next open pack, created and synced into dir, or nil.
	spare *pack
	// packs are the rotated and finalized packs, in seq order.
	packs []*pack
	// table locates every value a local pack holds and no Remove forgot.
	table map[keyTail]location
	// syncs counts the fdatasyncs writers have run.
	syncs uint64
	// syncFailures counts the fdatasyncs that failed.
	syncFailures uint64
	// salvaged counts the poisoned packs salvaged in process.
	salvaged uint64
	// salvageFailures counts the salvages that failed.
	salvageFailures uint64
	// failErr is set when a salvage failed: every Append returns it until
	// the Log is opened again.
	failErr error
	// leaders counts the writers running an fdatasync, which Close waits
	// out before it closes the files.
	leaders int
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

// Open opens the Log in dir, creating dir if needed. It recovers every pack a
// crash left unfinalized to its last good batch and finalizes it, loads the
// table from every finalized pack, and opens a new pack. ctx bounds the
// opening and the Log's background work, which also ends at Close.
func Open(ctx context.Context, dir string, opts Options) (*Log, error) {
	// Create the directory and list the packs in it, oldest first.
	if err := os.MkdirAll(dir, 0o700); err != nil { // #nosec G703 -- the caller chooses the directory.
		return nil, err
	}
	names, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	l := &Log{dir: dir, opts: opts.withDefaults(), table: make(map[keyTail]location), nextSeq: 1}

	// Recover or load each pack, closing what was opened on failure.
	ok := false
	defer func() {
		if !ok {
			l.closeFiles()
		}
	}()
	if err := l.removeStale(names); err != nil {
		return nil, err
	}
	finals := make(map[uint64]bool)
	for _, ent := range names {
		if seq, ext, _, valid := parseName(ent.Name()); valid && ext == packExt {
			finals[seq] = true
		}
	}
	for _, ent := range names {
		seq, ext, id, valid := parseName(ent.Name())
		if !valid {
			continue
		}
		l.nextSeq = max(l.nextSeq, seq+1)
		path := filepath.Join(dir, ent.Name())
		switch {
		case ext == openExt && finals[seq]:
			err = l.removeSalvaged(path)
		case ext == openExt:
			err = l.recoverPack(seq, path)
		case ext == packExt:
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

// removeStale removes the copies a salvage left unfinished at a crash. It
// runs before the Log is shared.
func (l *Log) removeStale(names []os.DirEntry) error {
	var removed bool
	for _, ent := range names {
		if strings.HasSuffix(ent.Name(), salvageExt) {
			if err := os.Remove(filepath.Join(l.dir, ent.Name())); err != nil && !os.IsNotExist(err) { // #nosec G703 -- a name listed in the caller's directory.
				return err
			}
			removed = true
		}
	}
	if !removed {
		return nil
	}
	return dirSync(l.dir)
}

// removeSalvaged removes an unfinalized pack whose salvaged copy is
// finalized under the same seq: a crash came between the copy's rename and
// the old file's removal. It runs before the Log is shared.
func (l *Log) removeSalvaged(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) { // #nosec G703 -- a pack name listed in the caller's directory.
		return err
	}
	return dirSync(l.dir)
}

// recoverPack salvages the batches of an unfinalized pack at path whose
// records and values check, or removes the pack when it holds none. The file
// is never finalized in place: in the same boot it may read back from the
// page cache bytes a failed sync left off the disk, so the kept batches are
// copied into a fresh file and synced there. It runs before the Log is
// shared.
func (l *Log) recoverPack(seq uint64, path string) error {
	// Read the pack's good batches.
	f, err := os.Open(path) // #nosec G703 -- a pack name listed in the caller's directory.
	if err != nil {
		return err
	}
	p := &pack{seq: seq, path: path, f: f, state: packRotated}
	data, entries, err := p.readGood()
	if err != nil {
		_ = f.Close()
		return err
	}

	// Remove a pack with no good batch: no write in it was acknowledged.
	if len(entries) == 0 {
		_ = f.Close()
		if err := os.Remove(path); err != nil { // #nosec G703 -- a pack name listed in the caller's directory.
			return err
		}
		return dirSync(l.dir)
	}

	// Copy the kept batches into a finalized pack and locate their values.
	q, fin, err := l.salvage(p, data, entries)
	_ = f.Close()
	if err != nil {
		return errors.Wrap(err, "salvage pack")
	}
	l.applyFinal(q, fin)
	for _, e := range entries {
		l.table[tailOf(e.key)] = location{p: q, off: uint32(e.off), size: uint32(e.size)} //nolint:gosec // bounded by the pack size.
	}
	q.entries = nil
	l.packs = append(l.packs, q)
	return nil
}

// salvage copies data, the leading batches of pack p to keep with their
// entries, into a fresh file under p's seq, syncs it, finalizes it, and
// removes p's file. The copy is written and synced only through its own new
// descriptor, so the finalized pack holds only bytes a sync that did not fail
// covered. The copy needs free disk for its bytes, which the removal of p's
// file returns. It returns the finalized copy, not yet published.
func (l *Log) salvage(p *pack, data []byte, entries []entry) (*pack, *finalized, error) {
	// Check the disk has room for the copy and a fixed margin. Salvage is
	// recovery, so it ignores Options.ReserveBytes: it copies one pack at a
	// time and removes the old file after each, so it needs one pack's space
	// beyond what it frees. With less, it fails and names what it needs.
	size := int64(len(data)) + p.indexReserve(len(entries), keyBytesOf(entries)) + salvageMargin
	free, err := l.opts.FreeBytes(l.dir)
	if err != nil {
		return nil, nil, errors.Wrap(err, "measure free disk")
	}
	if free < uint64(size) { //nolint:gosec // positive.
		return nil, nil, errors.Wrapf(ErrReserve, "salvage of %s needs %d bytes free and %d are: free disk space and restart", p.path, size, free)
	}

	// Create the copy, replacing one an earlier try left.
	path := filepath.Join(l.dir, salvageName(p.seq))
	_ = os.Remove(path)                                                 // #nosec G703 -- a pack name in the Log's directory.
	f, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0o600) // #nosec G703 -- a pack name in the Log's directory.
	if err != nil {
		return nil, nil, err
	}
	q := &pack{seq: p.seq, path: path, f: f, state: packRotated, entries: entries, written: int64(len(data)), synced: int64(len(data)), firstAt: p.firstAt}
	fail := func(err error) (*pack, *finalized, error) {
		_ = f.Close()
		_ = os.Remove(path) // #nosec G703 -- the copy just created.
		if q.renamed != nil {
			_ = os.Remove(q.renamed.path)
		}
		return nil, nil, err
	}

	// Write and sync the kept batches, then finalize the copy.
	if _, err := f.WriteAt(data, 0); err != nil {
		return fail(errors.Wrap(err, "write salvaged pack"))
	}
	if err := l.opts.SyncData(f); err != nil {
		return fail(errors.Wrap(err, "sync salvaged pack"))
	}
	fin, err := q.finalize(l.dir, data, l.opts)
	if err != nil {
		return fail(err)
	}

	// Remove the old file. A crash before the removal is durable leaves it
	// beside its finalized copy, and the next Open removes it.
	if err := os.Remove(p.path); err == nil || os.IsNotExist(err) {
		_ = dirSync(l.dir)
	}
	return q, fin, nil
}

// keyBytesOf sums the keys of entries.
func keyBytesOf(entries []entry) int64 {
	var n int64
	for _, e := range entries {
		n += int64(len(e.key))
	}
	return n
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
	p := &pack{seq: seq, path: path, f: f, state: packFinal, id: id, written: info.Size(), synced: info.Size(), blocks: len(index)}
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
	l.packs = append(l.packs, p)
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

// applyFinal records what finalize made of p. The caller holds bcast or has
// not shared the Log.
func (l *Log) applyFinal(p *pack, fin *finalized) {
	p.state, p.id, p.path = packFinal, fin.id, fin.path
	p.written, p.synced, p.blocks, p.bloom = fin.size, fin.size, fin.blocks, fin.bloom
}

// Append writes blocks and returns once every one is durable in a local
// pack. A block whose key the Log already holds is not written again; Append
// waits for that copy to be durable instead. Blocks that do not fit the open
// pack go to the next.
func (l *Log) Append(ctx context.Context, blocks []Block) error {
	// Check every key and size before writing any block.
	for _, b := range blocks {
		if len(b.Key) < keyTailLen {
			return ErrKeyTooShort
		}
		if int64(recordLen([]Block{b})+len(b.Value)+len(b.Key)+indexEntryReserve+indexTrailerReserve) > l.opts.MaxPackBytes {
			return ErrValueTooLarge
		}
	}

	// Refuse blocks that would leave less free disk than the reserve.
	if err := l.checkReserve(blocks); err != nil {
		return err
	}

	// Write the blocks under writeMtx, collecting the syncs they wait for.
	release, err := l.writeMtx.Lock(ctx)
	if err != nil {
		return err
	}
	waits, err := l.writeBlocks(ctx, blocks)
	release()
	if err != nil {
		return err
	}

	// Wait for each pack the blocks reached to be durable through them.
	for _, w := range waits {
		if err := l.waitSynced(ctx, w.p, w.end); err != nil {
			return err
		}
	}
	return nil
}

// checkReserve returns ErrReserve when writing blocks would leave less free
// disk than Options.ReserveBytes, and the measurement's error when the free
// disk cannot be read.
func (l *Log) checkReserve(blocks []Block) error {
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
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, b := range blocks {
			if !l.holdsLocked(tailOf(b.Key)) {
				need += uint64(len(b.Key) + len(b.Value)) //nolint:gosec // lengths are not negative.
			}
		}
	})
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
// fills, and returns the syncs they need. The caller holds writeMtx.
func (l *Log) writeBlocks(ctx context.Context, blocks []Block) ([]syncWait, error) {
	var waits []syncWait
	addWait := func(p *pack, end int64) {
		for i := range waits {
			if waits[i].p == p {
				waits[i].end = max(waits[i].end, end)
				return
			}
		}
		waits = append(waits, syncWait{p: p, end: end})
	}
	for len(blocks) != 0 {
		// Choose the blocks that fit the open pack, skipping held keys.
		var p *pack
		var plan batchPlan
		var err error
		l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			if l.closed {
				err = ErrClosed
				return
			}
			if l.failErr != nil {
				err = l.failErr
				return
			}
			p = l.open
			plan = l.planBatchLocked(p, blocks, addWait)
		})
		if err != nil {
			return nil, err
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
// wait for their copy and are skipped, and the rest are taken while the pack
// stays within its bounds. The caller holds bcast.
func (l *Log) planBatchLocked(p *pack, blocks []Block, addWait func(*pack, int64)) batchPlan {
	// A failed pack takes nothing more.
	if p.err != nil {
		return batchPlan{rotate: true}
	}

	// Take blocks while the batch fits, tracking the record's size.
	var plan batchPlan
	var entryBytes, data, keyBytes int64
	for plan.consumed < len(blocks) {
		b := blocks[plan.consumed]
		if t := tailOf(b.Key); l.holdsLocked(t) {
			loc := l.table[t]
			if loc.removed {
				l.reviveLocked(t, loc)
			}
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
	return ok && !loc.p.poisoned
}

// reviveLocked clears the removed mark of t's location loc, which a put
// stores again. The caller holds bcast.
func (l *Log) reviveLocked(t keyTail, loc location) {
	loc.removed = false
	loc.p.removed--
	l.table[t] = loc
}

// writeBatch encodes batch at the end of open pack p, writes it, and locates
// its values, returning the pack's new end. The caller holds writeMtx.
func (l *Log) writeBatch(p *pack, batch []Block) (int64, error) {
	// Encode the record and values into the pack's buffer.
	start := p.written
	end := start + int64(recordLen(batch)+dataLen(batch))
	buf := l.growBuf(p, end)
	entries := encodeBatch(buf[start:end], start, batch)

	// Write them with one positioned write.
	_, werr := batchWriteAt(p.f, buf[start:end], start)

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
		p.entries = append(p.entries, entries...)
		for _, e := range entries {
			p.keyBytes += int64(len(e.key))
			if old, ok := l.table[tailOf(e.key)]; ok && old.removed {
				old.p.removed--
			}
			l.table[tailOf(e.key)] = location{p: p, off: uint32(e.off), size: uint32(e.size)} //nolint:gosec // bounded by the pack size.
		}
	})
	if werr != nil {
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

// failLocked records p's first failure and forgets the values of its batches
// not yet durable, which recovery may drop. The caller holds bcast.
func (l *Log) failLocked(p *pack, err error) {
	if p.err == nil {
		p.err = err
	}
	p.poisoned = true
	for i := len(p.entries) - 1; i >= 0 && p.entries[i].off >= p.synced; i-- {
		t := tailOf(p.entries[i].key)
		if loc, ok := l.table[t]; ok && loc.p == p {
			if loc.removed {
				p.removed--
			}
			delete(l.table, t)
		}
	}
}

// waitSynced returns once p is durable through end. The first waiter to find
// no sync running runs one covering every batch written so far, and the rest
// wait for it.
func (l *Log) waitSynced(ctx context.Context, p *pack, end int64) error {
	for {
		// Read the pack's sync state and lead a sync if none runs.
		var done, lead bool
		var target int64
		var err error
		var waitCh <-chan struct{}
		l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			switch {
			case p.synced >= end:
				done = true
			case p.err != nil:
				err = p.err
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
			l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				// End the sync, which Close may wait out.
				p.syncing = false
				l.leaders--
				l.syncs++

				// A failed sync poisons the pack. A good one advances its mark
				// only while the pack has no error: a write that failed during
				// the sync already dropped the batches past the mark from the
				// table, and salvage keeps only bytes below it.
				if serr != nil {
					l.syncFailures++
					l.failLocked(p, serr)
				} else if p.err == nil {
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
		old := l.open
		old.state = packRotated
		l.packs = append(l.packs, old)

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
	// Look the value up again when its pack was salvaged or evicted
	// meanwhile: a salvaged value is located in its copy.
	if len(key) < keyTailLen {
		return nil, false, ErrKeyTooShort
	}
	out, ok, gone, err := l.get(key)
	if gone {
		out, ok, _, err = l.get(key)
	}
	return out, ok, err
}

// get reads the value under key once, and reports whether its pack was gone
// by the time it read the pack file.
func (l *Log) get(key []byte) ([]byte, bool, bool, error) {
	// Locate the value, copying it out of a pack buffer.
	var loc location
	var found, copied bool
	var out []byte
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		loc, found = l.table[tailOf(key)]
		if found && loc.p.buf != nil {
			out, copied = slices.Clone(loc.p.buf[loc.off:loc.end()]), true
		}
	})
	if !found || copied {
		return out, found, false, nil
	}

	// Read it from the pack file. A pack evicted since the lookup no longer
	// holds the value, which the remote holds instead.
	out = make([]byte, loc.size)
	if _, err := loc.p.f.ReadAt(out, int64(loc.off)); err != nil {
		var gone bool
		l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			gone = loc.p.state == packGone
		})
		if gone {
			return nil, false, true, nil
		}
		return nil, false, false, errors.Wrapf(err, "read pack %s", seqText(loc.p.seq))
	}
	return out, true, false, nil
}

// Has reports whether a local pack holds the value under key.
func (l *Log) Has(key []byte) bool {
	if len(key) < keyTailLen {
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
	if len(key) < keyTailLen {
		return false
	}
	var durable bool
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		loc, ok := l.table[tailOf(key)]
		durable = ok && !loc.removed && !loc.p.poisoned && (loc.p.state == packFinal || loc.end() <= loc.p.synced)
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
	if len(key) < keyTailLen {
		return
	}
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		t := tailOf(key)
		loc, ok := l.table[t]
		switch {
		case !ok || loc.removed:
		case loc.p.uploaded:
			delete(l.table, t)
		default:
			loc.removed = true
			loc.p.removed++
			l.table[t] = loc
		}
	})
}

// GetPacks returns the finalized packs, oldest first, and a channel closed
// when the Log next changes.
func (l *Log) GetPacks() ([]PackInfo, <-chan struct{}) {
	var infos []PackInfo
	var waitCh <-chan struct{}
	l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
		for _, p := range l.packs {
			if p.state == packFinal {
				infos = append(infos, PackInfo{Seq: p.seq, ID: p.id, Path: p.path, Size: p.written, Blocks: p.blocks, BloomFilter: p.bloom, FirstAt: p.firstAt, Uploaded: p.uploaded})
			}
		}
		waitCh = getWaitCh()
	})
	return infos, waitCh
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
			if err == nil {
				continue
			}
			due = time.Now().Add(retryDelay)
		}

		// Wait for a change, the linger deadline or the retry.
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
	// A Log failed closed does nothing until it is opened again.
	if l.failErr != nil {
		return nil, time.Time{}
	}

	// Finalize the oldest rotated pack first, so packs finalize in order.
	for _, p := range l.packs {
		if p.state == packRotated {
			return func(ctx context.Context) error { return l.finalizeRotated(ctx, p) }, time.Time{}
		}
	}

	// Rotate a failed open pack, so it is salvaged without waiting for a
	// writer.
	if l.open.err != nil {
		return l.rotateFailed, time.Time{}
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

	// Offer it to the writer, or remove it when the Log closed meanwhile:
	// Close leaves no empty pack.
	var closed bool
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if closed = l.closed; !closed {
			l.spare = p
			broadcast()
		}
	})
	if closed {
		_ = p.f.Close()
		_ = os.Remove(p.path) // #nosec G703 -- the pack just created.
		_ = dirSync(l.dir)
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

// rotateFailed rotates the open pack once a write or sync failed it.
func (l *Log) rotateFailed(ctx context.Context) error {
	// Hold the writer out while checking the failure again.
	release, err := l.writeMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	var failed bool
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		failed = l.open.err != nil
	})

	// Rotate a pack that still failed.
	if !failed {
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

	// Take the pack's bytes from its buffer.
	var data []byte
	var failed bool
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		failed = p.err != nil
		data = p.buf[:p.written]
	})
	if syncErr != nil && !failed {
		return syncErr
	}

	// Finalize a pack with no failure, then serve it from its file and keep
	// its buffer. A failed index write or sync poisons it; a failed directory
	// sync is retried alone.
	if !failed {
		fin, err := p.finalize(l.dir, data, l.opts)
		var ioErr *ioError
		if err != nil && !errors.As(err, &ioErr) {
			return err
		}
		if err == nil {
			l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				l.applyFinal(p, fin)
				if len(l.bufs) < keptBufs {
					l.bufs = append(l.bufs, p.buf)
				}
				p.buf, p.entries = nil, nil
				broadcast()
			})
			return nil
		}
		l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			l.syncFailures++
			l.failLocked(p, err)
		})
	}

	// Salvage a poisoned pack, failing the Log closed when that fails.
	if err := l.salvageRotated(p); err != nil {
		l.failClosed(p, err)
		return err
	}
	return nil
}

// salvageRotated copies the acknowledged batches of poisoned pack p, those a
// sync covered, from its buffer into a finalized pack that takes its place,
// or removes p when it holds none.
func (l *Log) salvageRotated(p *pack) error {
	// Wait out a sync leader still running on p: no new one starts on a
	// failed pack, and the mark it leaves is final.
	for {
		var waitCh <-chan struct{}
		l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			if p.syncing {
				waitCh = getWaitCh()
			}
		})
		if waitCh == nil {
			break
		}
		<-waitCh
	}

	// Take the synced batches from the buffer: the file is not trusted.
	var data []byte
	var keep []entry
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		data = p.buf[:p.synced]
		for _, e := range p.entries {
			if e.off < p.synced {
				keep = append(keep, e)
			}
		}
	})
	if len(keep) == 0 {
		return l.dropEmpty(p)
	}

	// Copy them into a finalized pack.
	q, fin, err := l.salvage(p, data, keep)
	if err != nil {
		return errors.Wrapf(err, "salvage pack %s", seqText(p.seq))
	}

	// Put the copy in p's place, point p's values at it, and drop p.
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		// Replace p with its finalized copy.
		l.applyFinal(q, fin)
		q.entries = nil
		for i := range l.packs {
			if l.packs[i] == p {
				l.packs[i] = q
			}
		}

		// Locate the kept values still at their place in p in the copy,
		// keeping their removed marks.
		for _, e := range keep {
			t := tailOf(e.key)
			if loc, ok := l.table[t]; ok && loc.p == p && int64(loc.off) == e.off {
				loc.p = q
				if loc.removed {
					q.removed++
				}
				l.table[t] = loc
			}
		}

		// Drop p.
		p.state, p.buf, p.entries = packGone, nil, nil
		l.salvaged++
		broadcast()
	})
	_ = p.f.Close()
	return nil
}

// failClosed records that salvaging pack p failed: every Append returns err
// from now on, the maintainer stops, and p's values stay readable and not
// durable. The operator restarts the Log, whose Open salvages p again.
func (l *Log) failClosed(p *pack, err error) {
	var first bool
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		l.salvageFailures++
		if first = l.failErr == nil; first {
			l.failErr = errors.Wrapf(err, "packahead failed closed")
		}
		broadcast()
	})
	if first {
		l.opts.Logger.WithError(err).WithField("pack", seqText(p.seq)).Error("packahead salvage failed: refusing appends until restart")
	}
}

// dropEmpty removes rotated pack p, which holds no good batch.
func (l *Log) dropEmpty(p *pack) error {
	// Remove the file and make the removal durable. A retry after a failed
	// directory sync finds the file gone.
	_ = p.f.Close()
	if err := os.Remove(p.path); err != nil && !os.IsNotExist(err) {
		return err
	}
	if err := dirSync(l.dir); err != nil {
		return err
	}

	// Forget the pack.
	l.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		l.packs = slices.DeleteFunc(l.packs, func(q *pack) bool { return q == p })
		p.state, p.buf, p.entries = packGone, nil, nil
		broadcast()
	})
	return nil
}

// Close stops the background work, waits for the writer in progress and the
// syncs running, finalizes the packs not yet finalized, and closes the pack
// files. A writer waiting for a sync that has not started returns ErrClosed.
// A failure while finalizing is logged, and the next Open salvages the pack.
func (l *Log) Close() {
	if err := l.CloseContext(context.Background()); err != nil {
		l.opts.Logger.WithError(err).Warn("packahead close left an unfinalized pack for the next open")
	}
}

// CloseContext is Close, finalizing the packs that took batches through the
// normal finalize path while ctx lasts, so an ordinary restart finds no
// unfinalized pack and copies nothing. An empty pack is removed. A pack that
// failed, whose finalize fails, or that ctx ends first stays unfinalized for
// the next Open to salvage; CloseContext returns the first such error.
func (l *Log) CloseContext(ctx context.Context) error {
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

	// Wait for the writer in progress and the syncs running, which use the
	// files. No sync starts once closed is set.
	release, err := l.writeMtx.Lock(context.Background())
	if err == nil {
		defer release()
	}
	for {
		l.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			done, waitCh = l.leaders == 0, getWaitCh()
		})
		if done {
			break
		}
		<-waitCh
	}

	// Finalize the packs still unfinalized, then close the files.
	err = l.finalizeAtClose(ctx)
	l.closeFiles()
	return err
}

// finalizeAtClose finalizes each unfinalized pack that has no failure,
// removes the empty ones, and leaves the rest for the next Open. The Log is
// closed and nothing else runs.
func (l *Log) finalizeAtClose(ctx context.Context) error {
	// Collect the unfinalized packs.
	var packs []*pack
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, p := range append(slices.Clone(l.packs), l.open, l.spare) {
			if p != nil && p.state != packFinal && p.state != packGone {
				packs = append(packs, p)
			}
		}
	})

	// Finalize each, keeping the first failure.
	var firstErr error
	for _, p := range packs {
		if err := l.finalizeOne(ctx, p); err != nil && firstErr == nil {
			firstErr = errors.Wrapf(err, "finalize pack %s at close", seqText(p.seq))
		}
	}
	return firstErr
}

// finalizeOne removes empty pack p, or syncs and finalizes it when it has no
// failure.
func (l *Log) finalizeOne(ctx context.Context, p *pack) error {
	// Remove a pack that took no batch: nothing in it was acknowledged.
	if err := context.Cause(ctx); err != nil {
		return err
	}
	if p.written == 0 && p.renamed == nil {
		_ = p.f.Close()
		if err := os.Remove(p.path); err != nil && !os.IsNotExist(err) {
			return err
		}
		p.state = packGone
		return dirSync(l.dir)
	}

	// Leave a failed pack for salvage, and sync and finalize the rest.
	if p.err != nil {
		return errors.Wrap(p.err, "pack failed")
	}
	if p.synced < p.written {
		if err := l.opts.SyncData(p.f); err != nil {
			return errors.Wrap(err, "sync pack")
		}
	}
	if err := context.Cause(ctx); err != nil {
		return err
	}
	fin, err := p.finalize(l.dir, p.buf[:p.written], l.opts)
	if err != nil {
		return err
	}
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		l.applyFinal(p, fin)
	})
	return nil
}

// closeFiles closes every pack file the Log holds.
func (l *Log) closeFiles() {
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for _, p := range append(slices.Clone(l.packs), l.open, l.spare) {
			if p != nil && p.f != nil {
				_ = p.f.Close()
			}
		}
	})
}

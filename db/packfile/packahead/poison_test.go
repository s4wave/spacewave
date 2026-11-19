package packahead

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_mock "github.com/s4wave/spacewave/db/block/mock"
	"github.com/s4wave/spacewave/db/packfile"
)

// failingSync is a SyncData that fails the sync numbered fail, counting from
// one, and runs every other.
type failingSync struct {
	n    atomic.Int64
	fail int64
}

// sync is the SyncData hook.
func (s *failingSync) sync(f *os.File) error {
	if s.n.Add(1) == s.fail {
		return syscall.EIO
	}
	return f.Sync()
}

// TestPackAheadSyncPoisons fails the second sync of a pack, after a first
// batch synced: the pack is poisoned, so it is never finalized, none of its
// values reports durable, and a put of its durable value writes a new copy.
// The next Open recovers the poisoned pack from its file.
func TestPackAheadSyncPoisons(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	hook := &failingSync{fail: 2}
	l, err := Open(ctx, dir, Options{SyncData: hook.sync, Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// The first batch syncs; the second's sync fails and poisons the pack.
	a, b := testBlock(t, 1, 3000), testBlock(t, 2, 3000)
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		l.opts.Linger = 0
	})
	if err := l.Append(ctx, []Block{a}); err != nil {
		t.Fatal(err)
	}
	if err := l.Append(ctx, []Block{b}); err == nil {
		t.Fatal("append succeeded on a failed sync")
	}
	if l.HasDurable(a.Key) || l.HasDurable(b.Key) {
		t.Fatal("a poisoned pack's value reports durable")
	}
	if got, ok, err := l.Get(a.Key); err != nil || !ok || !bytes.Equal(got, a.Value) {
		t.Fatalf("synced value of a poisoned pack does not read: %v %v", ok, err)
	}

	// A put of the durable value writes a new copy, which a later pack
	// finalizes; the poisoned pack stays unfinalized.
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		l.opts.Linger = 10 * time.Millisecond
	})
	if err := l.Append(ctx, []Block{a}); err != nil {
		t.Fatal(err)
	}
	if !l.HasDurable(a.Key) {
		t.Fatal("value stored again is not durable")
	}
	infos := waitFinalized(t, l, 1)
	if slices.ContainsFunc(infos, func(i PackInfo) bool { return i.Seq == 1 }) {
		t.Fatal("poisoned pack finalized")
	}
	if st := l.GetStats(); st.Poisoned != 1 || st.SyncFailures != 1 {
		t.Fatalf("stats: %+v", st)
	}
	if _, err := os.Stat(filepath.Join(dir, openName(1))); err != nil {
		t.Fatalf("poisoned pack file: %v", err)
	}

	// The next Open recovers the poisoned pack.
	l.Close()
	l2, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	if got, ok, err := l2.Get(a.Key); err != nil || !ok || !bytes.Equal(got, a.Value) {
		t.Fatalf("value after reopen: %v %v", ok, err)
	}
}

// TestPackAheadWriteBatchFailedPack writes a batch to a pack whose sync
// failed after the batch was planned: the batch is refused and not located.
func TestPackAheadWriteBatchFailedPack(t *testing.T) {
	ctx := t.Context()
	l, err := Open(ctx, t.TempDir(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	a := testBlock(t, 1, 3000)
	release, err := l.writeMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var p *pack
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		p = l.open
		p.poisoned = true
		l.failLocked(p, syscall.EIO)
	})
	_, werr := l.writeBatch(p, []Block{a})
	release()
	if werr == nil {
		t.Fatal("batch written to a failed pack")
	}
	if l.Has(a.Key) {
		t.Fatal("batch of a failed pack located")
	}
}

// TestPackAheadFinalizeSyncHook finalizes a pack with a SyncData that counts
// its calls: the index sync goes through the hook too, so a test that fails
// syncs reaches every one.
func TestPackAheadFinalizeSyncHook(t *testing.T) {
	ctx := t.Context()
	var calls atomic.Int64
	hook := func(f *os.File) error {
		calls.Add(1)
		return f.Sync()
	}
	l, err := Open(ctx, t.TempDir(), Options{SyncData: hook, Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := l.Append(ctx, []Block{testBlock(t, 1, 3000)}); err != nil {
		t.Fatal(err)
	}
	waitFinalized(t, l, 1)
	if n := calls.Load(); n < 2 {
		t.Fatalf("finalize bypassed SyncData: %d calls", n)
	}
}

// TestPackAheadFinalizeSyncPoisons fails the index sync of a pack a write
// failed on, after a batch written but never synced: the failed sync may have
// dropped that batch's pages, so the pack is poisoned, is never finalized,
// and none of its values reports durable.
func TestPackAheadFinalizeSyncPoisons(t *testing.T) {
	ctx := t.Context()
	dir := t.TempDir()
	var armed, fired atomic.Bool
	path := filepath.Join(dir, openName(1))
	hook := func(f *os.File) error {
		if armed.Load() && f.Name() == path && fired.CompareAndSwap(false, true) {
			return syscall.EIO
		}
		return f.Sync()
	}
	l, err := Open(ctx, dir, Options{SyncData: hook, Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		l.opts.Linger = 0
	})

	// Batch a syncs; batch b is written but not synced, then a write fails.
	a, b, c := testBlock(t, 1, 3000), testBlock(t, 2, 3000), testBlock(t, 3, 3000)
	if err := l.Append(ctx, []Block{a}); err != nil {
		t.Fatal(err)
	}
	release, err := l.writeMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var p *pack
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		p = l.open
	})
	if _, err := l.writeBatch(p, []Block{b}); err != nil {
		release()
		t.Fatal(err)
	}
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		l.failLocked(p, syscall.EIO)
	})
	release()

	// The next Append rotates; the failed pack's index sync fails.
	armed.Store(true)
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		l.opts.Linger = 10 * time.Millisecond
	})
	if err := l.Append(ctx, []Block{c}); err != nil {
		t.Fatal(err)
	}
	infos := waitFinalized(t, l, 1)
	if !fired.Load() {
		t.Fatal("the index sync never failed")
	}
	if slices.ContainsFunc(infos, func(i PackInfo) bool { return i.Seq == 1 }) {
		t.Fatal("pack whose index sync failed finalized")
	}
	if l.HasDurable(a.Key) || l.HasDurable(b.Key) {
		t.Fatal("a value of the poisoned pack reports durable")
	}
	if st := l.GetStats(); st.Poisoned != 1 || st.SyncFailures != 1 {
		t.Fatalf("stats: %+v", st)
	}

	// The next Open recovers the poisoned pack from its file.
	l.Close()
	l2, err := Open(ctx, dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer l2.Close()
	if !l2.HasDurable(a.Key) || !l2.HasDurable(c.Key) {
		t.Fatal("value lost after reopen")
	}
}

// TestPackAheadFailedWriteCannotAcknowledgeCorruptBatch holds an append's
// sync open until a later write fails and recovery can no longer keep it.
func TestPackAheadFailedWriteCannotAcknowledgeCorruptBatch(t *testing.T) {
	// Pause the first append after its write and before publishing its sync.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	file := make(chan *os.File, 1)
	resume := make(chan struct{})
	var calls atomic.Int32
	l, err := Open(ctx, t.TempDir(), Options{SyncData: func(f *os.File) error {
		if calls.Add(1) == 1 {
			file <- f
			select {
			case <-resume:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return f.Sync()
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		l.Close()
	}()

	// Damage the in-flight batch and fail a later append to the same pack.
	lost := testBlock(t, 1, 64)
	result := make(chan error, 1)
	go func() { result <- l.Append(ctx, []Block{lost}) }()
	var f *os.File
	select {
	case f = <-file:
	case <-ctx.Done():
		t.Fatal("append never reached sync")
	}
	info, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte{^lost.Value[len(lost.Value)-1]}, info.Size()-1); err != nil {
		t.Fatal(err)
	}
	if err := FailAppendWrite(t, l, []Block{testBlock(t, 2, 64)}); !errors.Is(err, syscall.EBADF) {
		t.Fatalf("later write returned %v", err)
	}

	// Let the sync succeed before rotation. Its watermark covers the lost
	// batch, but cannot acknowledge it once the later write failed.
	close(resume)
	select {
	case err := <-result:
		if !errors.Is(err, syscall.EBADF) {
			t.Fatalf("append to failed pack returned %v", err)
		}
	case <-ctx.Done():
		t.Fatal("append never finished")
	}

	// Rotate and wait for the empty recovered pack to disappear.
	if err := l.Append(ctx, []Block{testBlock(t, 3, 64)}); err != nil {
		t.Fatal(err)
	}
	if err := l.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return l.packs[1] == nil, nil
	}); err != nil {
		t.Fatal(err)
	}
}

// TestPackAheadMultiPackAppendRejectsFailure fails the newer pack while
// the same append is waiting for the older pack's sync.
func TestPackAheadMultiPackAppendRejectsFailure(t *testing.T) {
	// Gate the older pack's sync so the mixed append drives it.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	syncing, resume := make(chan struct{}), make(chan struct{})
	l, err := Open(ctx, t.TempDir(), Options{MaxPackBlocks: 2, SyncData: func(f *os.File) error {
		if filepath.Base(f.Name()) == openName(1) {
			close(syncing)
			select {
			case <-resume:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return f.Sync()
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		l.Close()
	}()
	l.cancel()
	if err := l.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return l.maintDone, nil
	}); err != nil {
		t.Fatal(err)
	}

	// Stage the full older pack without syncing it, then rotate and sync a
	// block in the newer pack.
	older, newer := testBlock(t, 1, 64), testBlock(t, 3, 64)
	release, err := l.writeMtx.Lock(ctx)
	if err != nil {
		t.Fatal(err)
	}
	_, err = l.writeBlocks(ctx, []Block{older, testBlock(t, 2, 64)}, nil)
	release()
	if err != nil {
		t.Fatal(err)
	}
	if err := l.Append(ctx, []Block{newer}); err != nil {
		t.Fatal(err)
	}

	// The mixed append accepts the newer sync first and leads the older sync.
	result := make(chan error, 1)
	go func() { result <- l.Append(ctx, []Block{newer, older}) }()
	select {
	case <-syncing:
	case <-ctx.Done():
		t.Fatal("mixed append never reached the older sync")
	}

	// Damage the newer batch on disk and fail the next write to that pack.
	if _, err := l.open.f.WriteAt([]byte{0}, 0); err != nil {
		t.Fatal(err)
	}
	if err := FailAppendWrite(t, l, []Block{testBlock(t, 4, 64)}); !errors.Is(err, syscall.EBADF) {
		t.Fatalf("later write returned %v", err)
	}

	// The successful older sync cannot acknowledge the failed newer pack.
	close(resume)
	select {
	case err := <-result:
		if !errors.Is(err, syscall.EBADF) {
			t.Fatalf("mixed append returned %v", err)
		}
	case <-ctx.Done():
		t.Fatal("mixed append never finished")
	}
}

// TestPackAheadRecoveryKeepsAcknowledgedBatches tests live and startup
// recovery after damage to a batch's data or its record header.
func TestPackAheadRecoveryKeepsAcknowledgedBatches(t *testing.T) {
	for _, damage := range []string{"data", "header"} {
		for _, recovery := range []string{"live", "startup"} {
			t.Run(damage+"/"+recovery, func(t *testing.T) {
				checkRecoveryKeepsAcknowledgedBatches(t, damage, recovery)
			})
		}
	}
}

// checkRecoveryKeepsAcknowledgedBatches verifies local and indexed reads of
// the intact batch, and I2 for the corrupt batch's existing decoded entry.
func checkRecoveryKeepsAcknowledgedBatches(t *testing.T, damage, recovery string) {
	// Gate a live recovery between removal of the bad location and publication
	// of the finalized pack.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	dir := t.TempDir()
	path := filepath.Join(dir, openName(1))
	syncing, resume := make(chan struct{}), make(chan struct{})
	var armed atomic.Bool
	l, err := Open(ctx, dir, Options{SyncData: func(f *os.File) error {
		if f.Name() == path && armed.CompareAndSwap(true, false) {
			close(syncing)
			select {
			case <-resume:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		return f.Sync()
	}})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		cancel()
		if l != nil {
			l.Close()
		}
	}()

	// Store three decoded values in separate acknowledged batches.
	write := func(label string) (*block.BlockRef, Block) {
		bytes, err := block_mock.NewExample(label).MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		ref, err := block.BuildBlockRef(bytes, nil)
		if err != nil {
			t.Fatal(err)
		}
		value := Block{Key: packfile.BlockKey(ref.GetHash()), Value: bytes}
		if err := l.Append(ctx, []Block{value}); err != nil {
			t.Fatal(err)
		}
		return ref, value
	}
	_, prefix := write("intact prefix batch")
	badRef, bad := write("damaged batch")
	goodRef, good := write("intact acknowledged batch")

	// Prime the shared cache from both synced entries.
	cache := block.NewDecodedBlockCache()
	defer cache.Close()
	read := func(ref *block.BlockRef) (block.ReadCounterSnapshot, error) {
		ctx, counter := block.WithReadCounter(ctx)
		tx, cursor := block.NewTransaction(recoveryCacheStore{log: l}, nil, ref, &block.PutOpts{HashType: block.DefaultHashType})
		tx.SetDecodedBlockCache(cache)
		_, err := cursor.Unmarshal(ctx, block_mock.NewExampleBlock)
		return counter.Snapshot(), err
	}
	for _, ref := range []*block.BlockRef{badRef, goodRef} {
		if got, err := read(ref); err != nil || got.DecodedBlockStoreAcceptedCount != 1 {
			t.Fatalf("cache admission = %+v, %v", got, err)
		}
	}

	// Damage the middle batch on disk. The other batches remain intact.
	var loc location
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		loc = l.table[tailOf(bad.Key)]
	})
	at, replacement := int64(loc.off), ^bad.Value[0]
	if damage == "header" {
		at, replacement = int64(recordLen([]Block{prefix})+len(prefix.Value)), 0
	}
	if _, err := loc.p.f.WriteAt([]byte{replacement}, at); err != nil {
		t.Fatal(err)
	}

	// Restart from the open file or trigger and pause live recovery.
	if recovery == "startup" {
		l.Close()
		l, err = Open(ctx, dir, Options{})
		if err != nil {
			t.Fatal(err)
		}
	}
	if recovery == "live" {
		if err := FailAppendWrite(t, l, []Block{testBlock(t, 2, 64)}); !errors.Is(err, syscall.EBADF) {
			t.Fatalf("failed write returned %v", err)
		}
		armed.Store(true)
		if err := l.Append(ctx, []Block{testBlock(t, 3, 64)}); err != nil {
			t.Fatal(err)
		}
		select {
		case <-syncing:
		case <-ctx.Done():
			t.Fatal("recovery never reached sync")
		}

		// A fresh transaction must miss the bad block while finalization is
		// still paused. The intact block remains readable throughout.
		if got, err := read(badRef); !errors.Is(err, block.ErrNotFound) || got.DecodedBlockCacheHitCount != 0 {
			t.Fatalf("read of bad block during recovery = %+v, %v", got, err)
		}
		if got, err := read(goodRef); err != nil || got.DecodedBlockCacheHitCount != 1 {
			t.Fatalf("read of intact block during recovery = %+v, %v", got, err)
		}
		close(resume)
	}

	// A fresh transaction misses the bad block even after an in-process reopen.
	if got, err := read(badRef); !errors.Is(err, block.ErrNotFound) || got.DecodedBlockCacheHitCount != 0 {
		t.Fatalf("read of bad block after recovery = %+v, %v", got, err)
	}

	// Keep the later batch only when recovery can establish its start.
	want := map[string][]byte{string(prefix.Key): prefix.Value}
	if damage == "header" && recovery == "startup" {
		if l.Has(good.Key) {
			t.Fatal("startup indexed a batch beyond an unreadable record")
		}
	} else {
		want[string(good.Key)] = good.Value
		if value, found, err := l.Get(good.Key); err != nil || !found || !bytes.Equal(value, good.Value) {
			t.Fatalf("intact block after recovery: found=%v err=%v", found, err)
		}
	}

	// A finalized pack reader sees exactly the batches recovery kept.
	infos := waitFinalized(t, l, 1)
	checkPack(t, infos[0], want, Options{})
	if l.Has(bad.Key) {
		t.Fatal("bad batch still has a location")
	}
}

// recoveryCacheStore passes pack locations to a block transaction, with
// the same durability marking as the packed store.
type recoveryCacheStore struct {
	block.StoreOps
	log *Log
}

// GetBlock marks a read according to the sync state captured by the log.
func (s recoveryCacheStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	// Sample the generation before the log lookup.
	gen := block.DurableReadGeneration()
	data, found, synced, err := s.log.GetDurable(packfile.BlockKey(ref.GetHash()))

	// Only the durable copy can enter the shared cache.
	if found && synced {
		block.MarkDurableRead(ctx, ref, gen)
	} else if found {
		block.MarkStagedRead(ctx)
	}
	return data, found, err
}

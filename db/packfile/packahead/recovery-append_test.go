package packahead_test

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
)

// appendWaitGate pauses a sync follower after Append released the write lock.
type appendWaitGate struct {
	context.Context
	// once pauses only the first wait.
	once sync.Once
	// paused signals that the follower is waiting.
	paused chan struct{}
	// resume releases the follower after finalization.
	resume chan struct{}
}

// Done pauses the follower as it enters the wait for a running sync.
func (g *appendWaitGate) Done() <-chan struct{} {
	// Let the test run recovery before this append rechecks its watermark.
	g.once.Do(func() {
		close(g.paused)
		select {
		case <-g.resume:
		case <-g.Context.Done():
		}
	})

	// Return the real cancellation signal for the append's select.
	return g.Context.Done()
}

// recoveryTestBucket stores a batch with bucket ownership in a real Volume.
type recoveryTestBucket struct {
	*volume_kvtx.Volume
}

// PutBlockBatch commits an external entry's ownership and references.
func (b recoveryTestBucket) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	return b.PrepareOwnedBlockBatch(ctx, "bucket", entries)
}

// TestPackAheadRecoveredIndexCannotAcknowledgeAppend holds a packed put
// across recovery. The final index must not stand in for lost batch bytes.
func TestPackAheadRecoveredIndexCannotAcknowledgeAppend(t *testing.T) {
	// Hold the first sync after capturing its target, then let later packs sync.
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	firstFile := make(chan *os.File, 1)
	finishSync := make(chan struct{})
	var first sync.Once
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{SyncData: func(f *os.File) error {
		first.Do(func() {
			firstFile <- f
			select {
			case <-finishSync:
			case <-ctx.Done():
			}
		})
		return f.Sync()
	}})
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	defer func() {
		cancel()
		log.Close()
	}()

	// Write a prefix with enough keys that its index spans the next batch.
	prefix := make([]packahead.Block, 64)
	for i := range prefix {
		key := make([]byte, block.BlockDigestTailLen)
		binary.BigEndian.PutUint64(key, uint64(i)) //nolint:gosec // i is non-negative.
		prefix[i] = packahead.Block{Key: key, Value: []byte{1}}
	}
	prefixDone := make(chan error, 1)
	go func() { prefixDone <- log.Append(ctx, prefix) }()
	var f *os.File
	select {
	case f = <-firstFile:
	case <-ctx.Done():
		t.Fatal("prefix never reached sync")
	}
	before, err := f.Stat()
	if err != nil {
		t.Fatal(err)
	}

	// Pause a real PackedStore put with its batch written behind the prefix.
	local, err := volume_kvtx.NewVolume(ctx, "test/pack-recovery", store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), &store_kvtx.Config{}, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = local.Close() })
	packed := block_store_writeback.NewPackedStore(log, recoveryTestBucket{local}, true)
	data := []byte("discarded batch")
	ref, err := block.BuildBlockRef(data, nil)
	if err != nil {
		t.Fatal(err)
	}
	gate := &appendWaitGate{Context: ctx, paused: make(chan struct{}), resume: make(chan struct{})}
	putDone := make(chan error, 1)
	go func() {
		_, err := packed.PutBlockBatch(gate, []*block.PutBatchEntry{{Ref: ref, Data: data, RefsKnown: true}})
		putDone <- err
	}()
	select {
	case <-gate.paused:
	case err := <-putDone:
		t.Fatalf("put returned before the prefix sync: %v", err)
	case <-ctx.Done():
		t.Fatal("put never waited for sync")
	}
	after, err := f.Stat()
	if err != nil || after.Size() <= before.Size() {
		t.Fatalf("put did not write a suffix: %v, %v", after, err)
	}

	// Fail a later write, lose the unsynced suffix and let the prefix sync.
	if err := packahead.FailAppendWrite(t, log, []packahead.Block{{Key: bytesKey(1)}}); !errors.Is(err, syscall.EBADF) {
		t.Fatalf("later write did not fail: %v", err)
	}
	if err := f.Truncate(before.Size()); err != nil {
		t.Fatal(err)
	}
	close(finishSync)
	if err := log.Append(ctx, []packahead.Block{{Key: bytesKey(2), Value: data}}); err != nil {
		t.Fatalf("rotate: %v", err)
	}
	select {
	case err := <-prefixDone:
		if !errors.Is(err, syscall.EBADF) {
			t.Fatalf("pending append to failed pack returned %v", err)
		}
	case <-ctx.Done():
		t.Fatal("pending append never finished")
	}

	// Wait for finalization and prove the index spans the vanished batch end.
	for {
		packs, changed := log.GetPacks()
		if len(packs) != 0 {
			if packs[0].Blocks != len(prefix) || packs[0].Size < after.Size() {
				t.Fatalf("finalized pack misses the test's overlap: %+v, batch end %d", packs[0], after.Size())
			}
			break
		}
		select {
		case <-changed:
		case <-ctx.Done():
			t.Fatal("failed pack never finalized")
		}
	}

	// Resume the put. The write error must fail it before its entries land.
	close(gate.resume)
	select {
	case err := <-putDone:
		if !errors.Is(err, syscall.EBADF) {
			t.Errorf("discarded put returned %v", err)
		}
	case <-ctx.Done():
		t.Fatal("discarded put never finished")
	}
	if log.Has(packfile.BlockKey(ref.GetHash())) {
		t.Fatal("discarded batch is still in the log")
	}
}

// bytesKey returns a key for a later append outside the prefix's key range.
func bytesKey(tag byte) []byte {
	key := make([]byte, block.BlockDigestTailLen)
	key[0] = tag
	return key
}

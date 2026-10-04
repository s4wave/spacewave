package provider_spacewave

import (
	"strconv"
	"testing"

	"github.com/pkg/errors"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/sirupsen/logrus"
)

// TestPendingUploadCommitFailure preserves the durable queue and its projection.
func TestPendingUploadCommitFailure(t *testing.T) {
	ctx := t.Context()
	store := &syncMeasuredStore{store: newSyncTestKvStore()}
	syncer := &syncController{store: store, upper: newSyncTestBlockStore()}
	ref, _, err := syncer.upper.PutBlock(ctx, []byte("pending"), nil)
	if err != nil {
		t.Fatal(err)
	}
	marks := []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: 7}}
	injected := errors.New("failed durable commit")
	store.commitErr = injected
	if err := syncer.MarkDirty(ctx, marks); !errors.Is(err, injected) {
		t.Fatalf("mark error: %v", err)
	}
	if first, size, _ := syncer.pendingSnapshot(); !first.IsZero() || size != 0 {
		t.Fatal("failed commit published pending state")
	}
	store.commitErr = nil
	if err := syncer.MarkDirty(ctx, marks); err != nil {
		t.Fatal(err)
	}
	first, size, _ := syncer.pendingSnapshot()
	acquired, err := syncer.scanDirtyCandidates(ctx)
	if err != nil || len(acquired) != 1 {
		t.Fatalf("acquire: %d %v", len(acquired), err)
	}

	// An acknowledgement failure retains both accounting and recoverable work.
	store.commitErr = injected
	if err := syncer.cleanupDirtyCandidates(ctx, acquired); !errors.Is(err, injected) {
		t.Fatalf("ack error: %v", err)
	}
	reopened := &syncController{store: store}
	if err := reopened.updateDirtyState(ctx); err != nil {
		t.Fatal(err)
	}
	if at, bytes, _ := reopened.pendingSnapshot(); !at.Equal(first) || bytes != size {
		t.Fatal("failed acknowledgement changed the reopened queue")
	}
	store.commitErr = nil
	if err := syncer.cleanupDirtyCandidates(ctx, acquired); err != nil {
		t.Fatal(err)
	}
	if err := syncer.MarkDirty(ctx, marks); err != nil {
		t.Fatal(err)
	}
	if err := syncer.cleanupDirtyCandidates(ctx, acquired); err != nil {
		t.Fatal(err)
	}
	if at, bytes, _ := syncer.pendingSnapshot(); at.IsZero() || bytes != size {
		t.Fatal("stale acknowledgement removed a later insertion")
	}
}

// TestPendingUploadCutoff drains its captured backlog while new writes keep arriving.
func TestPendingUploadCutoff(t *testing.T) {
	ctx := t.Context()
	syncer := &syncController{
		le: logrus.NewEntry(logrus.New()), store: newSyncTestKvStore(), resourceID: "cutoff",
		upper: newSyncTestBlockStore(), lower: packfile_store.NewPackfileStore(nil, nil),
	}
	const count = syncDirtyPageLimit*2 + 17
	marks := make([]block_store_writeback.Mark, 0, count)
	for n := range count {
		data := []byte(strconv.Itoa(n))
		ref, _, err := syncer.upper.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatal(err)
		}
		marks = append(marks, block_store_writeback.Mark{Hash: ref.GetHash(), Size: int64(len(data))})
	}
	if err := syncer.MarkDirty(ctx, marks); err != nil {
		t.Fatal(err)
	}
	seen := make(map[uint64]bool, count)
	chunks := 0
	err := syncer.prepareChunks(ctx, count, false, func(chunk *preparedSyncChunk) error {
		chunks++
		for _, block := range chunk.blocks {
			if block.sequence > count || seen[block.sequence] {
				t.Fatalf("invalid captured insertion %d", block.sequence)
			}
			seen[block.sequence] = true
		}
		// New writes arrive after every pack; they cannot replace initial work.
		ref, _, err := syncer.upper.PutBlock(ctx, []byte("later-"+strconv.Itoa(chunks)), nil)
		if err != nil {
			return err
		}
		return syncer.MarkDirty(ctx, []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: 7}})
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != count || chunks != 3 {
		t.Fatalf("captured %d/%d in %d packs", len(seen), count, chunks)
	}
}

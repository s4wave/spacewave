package provider_spacewave

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"sync"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/packfile/writer"
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
	if at, bytes, _ := syncer.pendingSnapshot(); !at.Equal(first) || bytes != size {
		t.Fatal("failed acknowledgement changed the published queue")
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
	// Fail any payload read or pack handoff made while a page transaction is open.
	ctx := t.Context()
	store := &syncMeasuredStore{store: newSyncTestKvStore()}
	assertClosed := func() {
		if store.reading.Load() != 0 {
			t.Error("remote work started with a metadata read transaction open")
		}
	}
	syncer := &syncController{
		le: logrus.NewEntry(logrus.New()), store: store, resourceID: "cutoff",
		upper: &syncCountingBlockStore{StoreOps: newSyncTestBlockStore(), onGet: func(*block.BlockRef) { assertClosed() }},
		lower: packfile_store.NewPackfileStore(nil, nil),
	}
	const count = syncDirtyPageLimit*2 + 17
	if err := syncer.MarkDirty(ctx, putQueueBlocks(t, syncer.upper, "", count)); err != nil {
		t.Fatal(err)
	}
	seen := make(map[uint64]bool, count)
	chunks := 0
	err := syncer.prepareChunks(ctx, count, false, func(chunk *preparedSyncChunk) error {
		assertClosed()
		chunks++
		for _, queued := range chunk.blocks {
			if queued.sequence > count || seen[queued.sequence] {
				t.Fatalf("invalid captured insertion %d", queued.sequence)
			}
			seen[queued.sequence] = true
		}
		// New writes arrive after every pack; they cannot replace initial work.
		return syncer.MarkDirty(ctx, putQueueBlocks(t, syncer.upper, "later-"+strconv.Itoa(chunks)+"-", 1))
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(seen) != count || chunks != 3 {
		t.Fatalf("captured %d/%d in %d packs", len(seen), count, chunks)
	}
}

// TestPendingUploadPagesFillPacks keeps a byte-limited pack open across a page boundary.
func TestPendingUploadPagesFillPacks(t *testing.T) {
	// Size each recorded block so exactly 3000 fit one pack; pages hold 4096.
	ctx := t.Context()
	const perPack = 3000
	syncer := &syncController{
		le: logrus.NewEntry(logrus.New()), store: newSyncTestKvStore(), resourceID: "fill",
		upper: newSyncTestBlockStore(), lower: packfile_store.NewPackfileStore(nil, nil),
	}
	marks := putQueueBlocks(t, syncer.upper, "", 3*perPack)
	for i := range marks {
		marks[i].Size = syncFlushMaxPackBytes / perPack
	}
	if err := syncer.MarkDirty(ctx, marks); err != nil {
		t.Fatal(err)
	}

	// The second pack spans the first page boundary, so no pack may end there.
	var sizes []int
	err := syncer.prepareChunks(ctx, uint64(len(marks)), false, func(chunk *preparedSyncChunk) error {
		sizes = append(sizes, len(chunk.blocks))
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(sizes) != 3 || sizes[0] != perPack || sizes[1] != perPack || sizes[2] != perPack {
		t.Fatalf("pack sizes %v, want three of %d", sizes, perPack)
	}
}

// TestPendingUploadFlushKeepsConcurrentMarks acknowledges exactly the captured backlog.
func TestPendingUploadFlushKeepsConcurrentMarks(t *testing.T) {
	// Queue two packs of blocks and record the pending deadline.
	ctx := t.Context()
	store := newSyncTestKvStore()
	push := startTestPushServer(t)
	syncer := newPushQueueController(t, store, push)
	const count = int(writer.DefaultMaxBlocksPerPack) + 5
	if err := syncer.MarkDirty(ctx, putQueueBlocks(t, syncer.upper, "initial-", count)); err != nil {
		t.Fatal(err)
	}
	first, _, _ := syncer.pendingSnapshot()

	// Mark one more block while the first pack uploads.
	later := putQueueBlocks(t, syncer.upper, "later-", 1)
	var once sync.Once
	var laterErr error
	push.onUpload = func(*packfile.PushRequest) {
		once.Do(func() { laterErr = syncer.MarkDirty(ctx, later) })
	}
	if err := syncer.FlushNowUnordered(ctx); err != nil {
		t.Fatal(err)
	}
	if laterErr != nil {
		t.Fatal(laterErr)
	}

	// Every captured block uploaded; only the later one remains, under the
	// original deadline, in the persisted counters and the records.
	var uploaded int
	for _, pack := range push.committed() {
		uploaded += int(pack.req.GetBlockCount()) //nolint:gosec // test counts are small
	}
	if uploaded != count {
		t.Fatalf("uploaded %d blocks, want %d", uploaded, count)
	}
	state := readPersistedPending(t, store)
	if state.GetCount() != 1 || state.GetSizeBytes() != later[0].Size || state.GetPendingSinceNanos() != first.UnixNano() {
		t.Fatalf("persisted queue %v, want one later block pending since %v", state, first)
	}
	remaining, err := syncer.scanDirtyCandidates(ctx)
	if err != nil || len(remaining) != 1 || remaining[0].hash.MarshalString() != later[0].Hash.MarshalString() {
		t.Fatalf("remaining records %d: %v", len(remaining), err)
	}
}

// TestPendingUploadRefusesUnaccountedRecords keeps a store whose queue has no summary.
func TestPendingUploadRefusesUnaccountedRecords(t *testing.T) {
	// Leave a pending record in a store written before queue summaries existed.
	ctx := t.Context()
	store := newSyncTestKvStore()
	syncer := &syncController{store: store, upper: newSyncTestBlockStore()}
	marks := putQueueBlocks(t, syncer.upper, "", 1)
	err := kvtx.RunTransaction(ctx, true,
		func(ctx context.Context) (kvtx.Tx, error) { return store.NewTransaction(ctx, true) },
		func(ctx context.Context, tx kvtx.Tx) error {
			return tx.Set(ctx, []byte("dirty/"+marks[0].Hash.MarshalString()), nil)
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Marking refuses the store instead of replacing the unaccounted record.
	if err := syncer.MarkDirty(ctx, marks); err == nil {
		t.Fatal("mark accepted a queue with no summary")
	}
	if n := countSyncDirtyKeys(t, ctx, store); n != 1 {
		t.Fatalf("refused mark left %d records, want the original 1", n)
	}
}

// putQueueBlocks stores n small blocks and returns their marks.
func putQueueBlocks(t *testing.T, upper block.StoreOps, prefix string, n int) []block_store_writeback.Mark {
	t.Helper()
	marks := make([]block_store_writeback.Mark, 0, n)
	for i := range n {
		data := []byte(prefix + strconv.Itoa(i))
		ref, _, err := upper.PutBlock(t.Context(), data, nil)
		if err != nil {
			t.Fatal(err)
		}
		marks = append(marks, block_store_writeback.Mark{Hash: ref.GetHash(), Size: int64(len(data))})
	}
	return marks
}

// newPushQueueController returns a controller that uploads through the production client.
func newPushQueueController(t *testing.T, store kvtx.Store, push *testPushServer) *syncController {
	t.Helper()
	catalog, err := manifest.New(t.Context(), newSyncTestKvStore())
	if err != nil {
		t.Fatal(err)
	}
	key, peer := generateTestKeypair(t)
	logger := logrus.New()
	logger.SetOutput(io.Discard)
	return &syncController{
		le: logrus.NewEntry(logger), store: store, resourceID: "queue",
		client: NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, key, peer.String()),
		mfst:   catalog, lower: push.lower(t), upper: newSyncTestBlockStore(),
	}
}

// readPersistedPending reads the committed queue summary.
func readPersistedPending(t *testing.T, store kvtx.Store) *PendingUploadState {
	t.Helper()
	var state *PendingUploadState
	err := kvtx.RunTransaction(t.Context(), false,
		func(ctx context.Context) (kvtx.Tx, error) { return store.NewTransaction(ctx, false) },
		func(ctx context.Context, tx kvtx.Tx) error {
			var err error
			state, err = readPendingUploadState(ctx, tx)
			return err
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return state
}

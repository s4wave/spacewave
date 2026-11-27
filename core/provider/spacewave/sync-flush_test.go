package provider_spacewave

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	packfile_delta "github.com/s4wave/spacewave/core/provider/spacewave/packfile/delta"
	packfile_manifest "github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/db/block"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/sirupsen/logrus"
)

// TestSyncControllerFlushChunksLargeDirtySet uploads bounded packs before
// reading all dirty data, and packs the next chunk while the first uploads.
func TestSyncControllerFlushChunksLargeDirtySet(t *testing.T) {
	// Open the dirty store and pack manifest.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	// Count block reads. secondChunkLoaded closes once the packer has
	// loaded the second chunk.
	const blockCount = 6
	var getCount, firstPushGetCount atomic.Int64
	secondChunkLoaded := make(chan struct{})
	var secondChunkOnce, firstUpload sync.Once
	upper := &syncCountingBlockStore{
		StoreOps: newSyncTestBlockStore(),
		onGet: func(*block.BlockRef) {
			if getCount.Add(1) == 4 {
				secondChunkOnce.Do(func() { close(secondChunkLoaded) })
			}
		},
	}
	addSyncChunkBlocks(t, ctx, dirtyStore, upper, blockCount)

	// Hold the first upload until the second chunk is packed.
	push := startTestPushServer(t)
	push.onUpload = func(*packfile.PushRequest) {
		firstUpload.Do(func() {
			select {
			case <-secondChunkLoaded:
			case <-time.After(10 * time.Second):
				t.Error("the second chunk was not packed during the first upload")
			}
			firstPushGetCount.Store(getCount.Load())
		})
	}

	// Flush the dirty set through a controller.
	priv, pid := generateTestKeypair(t)
	s := &syncController{
		le:    logrus.NewEntry(logrus.New()),
		store: dirtyStore,
		client: func() (*SessionClient, error) {
			return NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, priv, pid.String()), nil
		},
		resourceID: "test-res",
		mfst:       mfst,
		lower:      push.lower(t),
		upper:      upper,
	}
	if _, err := s.flush(ctx, true); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// The flush pushed three packs, the first before reading every block.
	pushed := push.committed()
	if len(pushed) != 3 {
		t.Fatalf("sync pushes = %d, want 3", len(pushed))
	}
	if firstPushGetCount.Load() >= blockCount {
		t.Fatalf("expected first push before all dirty blocks loaded, loaded %d", firstPushGetCount.Load())
	}

	// Every pack fits the pack and wire limits.
	for i, pack := range pushed {
		size := len(pack.data)
		if int64(size) > syncFlushMaxPackBytes {
			t.Fatalf("push %d exceeded the sync pack target: %d", i, size)
		}
		if int64(size) > packfile_delta.DefaultMaxChunkBytes {
			t.Fatalf("push %d exceeded wire chunk limit: %d", i, size)
		}
	}

	// The manifest holds every pack and the dirty set is empty.
	if len(mfst.GetEntries()) != len(pushed) {
		t.Fatalf("manifest entries = %d, want %d", len(mfst.GetEntries()), len(pushed))
	}
	assertSyncPackEntryMetadata(t, mfst.GetEntries())
	if n := countSyncDirtyKeys(t, ctx, dirtyStore); n != 0 {
		t.Fatalf("expected dirty set to be empty, got %d entries", n)
	}
}

// TestSyncControllerFlushCommitsPushedPacksBeforeFailure keeps the packs pushed
// before a failed push in the manifest and clears their dirty markers.
func TestSyncControllerFlushCommitsPushedPacksBeforeFailure(t *testing.T) {
	// Mark six chunk-sized blocks dirty.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}
	const blockCount = 6
	upper := newSyncTestBlockStore()
	addSyncChunkBlocks(t, ctx, dirtyStore, upper, blockCount)

	// Refuse the third push.
	push := newTestPushServer(t, "")
	var pushCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/sync/push") {
			pushCount++
			if pushCount > 2 {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
		}
		push.ServeHTTP(w, r)
	}))
	defer srv.Close()
	push.base = srv.URL

	// Flush the dirty set until the refusal.
	priv, pid := generateTestKeypair(t)
	s := &syncController{
		le:    logrus.NewEntry(logrus.New()),
		store: dirtyStore,
		client: func() (*SessionClient, error) {
			return NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()), nil
		},
		resourceID: "test-res",
		mfst:       mfst,
		lower:      push.lower(t),
		upper:      upper,
	}

	// Drain the queue through the production packer and HTTP upload.
	if _, err := s.flush(ctx, false); err == nil {
		t.Fatal("expected flush to fail on the third push")
	}

	// The manifest kept both committed packs.
	if got := len(mfst.GetEntries()); got != 2 {
		t.Fatalf("manifest entries = %d, want the 2 pushed packs", got)
	}

	// Only the blocks outside the committed packs stay dirty.
	var pushedBlocks int
	for _, pack := range push.committed() {
		pushedBlocks += int(pack.req.GetBlockCount()) //nolint:gosec // test counts are small
	}
	want := blockCount - pushedBlocks
	if n := countSyncDirtyKeys(t, ctx, dirtyStore); n != want {
		t.Fatalf("dirty entries = %d, want %d unpushed blocks", n, want)
	}

	// The persisted summary counts the same blocks and keeps the first deadline.
	state := readPersistedPending(t, dirtyStore)
	if int(state.GetCount()) != want || state.GetSizeBytes() != int64(want*syncTestChunkBlockBytes) || state.GetPendingSinceNanos() == 0 {
		t.Fatalf("persisted queue %v, want %d blocks of %d bytes", state, want, syncTestChunkBlockBytes)
	}
}

// TestSyncControllerFlushDedupesLowerBlocks filters stored blocks before reading upper data.
func TestSyncControllerFlushDedupesLowerBlocks(t *testing.T) {
	// Open the dirty store and pack manifest.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	// Start the push server and a client for it.
	push := startTestPushServer(t)
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, priv, pid.String())

	// Mark a duplicate and a fresh block dirty.
	baseUpper := newSyncTestBlockStore()
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()
	duplicate := addSyncDirtyBlock(t, ctx, wtx, baseUpper, "duplicate dirty block")
	fresh := addSyncDirtyBlock(t, ctx, wtx, baseUpper, "fresh dirty block")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}

	// Record whether the flush reads the duplicate from the upper store.
	var duplicateFetched atomic.Bool
	upper := &syncCountingBlockStore{
		StoreOps: baseUpper,
		onGet: func(ref *block.BlockRef) {
			if ref.GetHash().MarshalString() == duplicate.GetHash().MarshalString() {
				duplicateFetched.Store(true)
			}
		},
	}

	// Flush against a lower store that already holds the duplicate.
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     func() (*SessionClient, error) { return cli, nil },
		resourceID: "test-res",
		mfst:       mfst,
		lower: newSyncTestLowerPackfileStore(t, map[string][]byte{
			"duplicate": []byte("duplicate dirty block"),
		}),
		upper: upper,
	}
	telemetry := &ProviderAccount{}
	s.telemetry = &telemetry.syncTelemetry
	if _, err := s.flush(ctx, true); err != nil {
		t.Fatalf("flush: %v", err)
	}
	if duplicateFetched.Load() {
		t.Fatal("expected lower-existing duplicate to be filtered before reading upper data")
	}

	// Only the fresh block was packed and the dirty set is empty.
	if len(mfst.GetEntries()) != 1 {
		t.Fatalf("expected 1 manifest entry, got %d", len(mfst.GetEntries()))
	}
	want := []string{fresh.GetHash().MarshalString()}
	if got := readPackPhysicalKeys(t, testPushedBody(t, push)); !slices.Equal(got, want) {
		t.Fatalf("physical pack keys = %v, want %v; duplicate=%s", got, want, duplicate.GetHash().MarshalString())
	}
	if n := countSyncDirtyKeys(t, ctx, dirtyStore); n != 0 {
		t.Fatalf("expected dirty set to be empty, got %d entries", n)
	}

	// Telemetry counts one upload and one deduped block.
	snap := telemetry.GetSyncTelemetrySnapshot()
	if snap.PushCount != 1 || snap.PushedBytes == 0 {
		t.Fatalf("expected one uploaded pack in telemetry, got %+v", snap)
	}
	if snap.DedupedUploadCount != 1 || snap.DedupedUploadBytes != int64(len("duplicate dirty block")) {
		t.Fatalf("unexpected dedup telemetry: %+v", snap)
	}
	if snap.PendingUploadBytes != 0 || snap.PendingUploadCount != 0 {
		t.Fatalf("expected no pending upload after cleanup, got %+v", snap)
	}
}

// TestSyncControllerFlushAllDuplicateDirtyBlocksSkipsPush clears duplicate work without uploading.
func TestSyncControllerFlushAllDuplicateDirtyBlocksSkipsPush(t *testing.T) {
	// Open the dirty store and pack manifest.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	// Count every request to the cloud.
	var pushCount int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pushCount++
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String())

	// Mark two blocks the lower store already holds dirty.
	upper := newSyncTestBlockStore()
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()
	addSyncDirtyBlock(t, ctx, wtx, upper, "duplicate-a")
	addSyncDirtyBlock(t, ctx, wtx, upper, "duplicate-b")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}

	// Flush the dirty set.
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     func() (*SessionClient, error) { return cli, nil },
		resourceID: "test-res",
		mfst:       mfst,
		lower: newSyncTestLowerPackfileStore(t, map[string][]byte{
			"a": []byte("duplicate-a"),
			"b": []byte("duplicate-b"),
		}),
		upper: upper,
	}
	telemetry := &ProviderAccount{}
	s.telemetry = &telemetry.syncTelemetry
	if _, err := s.flush(ctx, true); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Nothing was pushed and the dirty set is empty.
	if pushCount != 0 {
		t.Fatalf("expected no sync pushes, got %d", pushCount)
	}
	if len(mfst.GetEntries()) != 0 {
		t.Fatalf("expected no manifest entries, got %d", len(mfst.GetEntries()))
	}
	if n := countSyncDirtyKeys(t, ctx, dirtyStore); n != 0 {
		t.Fatalf("expected dirty set to be empty, got %d entries", n)
	}

	// Telemetry counts both blocks as deduped.
	snap := telemetry.GetSyncTelemetrySnapshot()
	if snap.PushCount != 0 || snap.PushedBytes != 0 {
		t.Fatalf("expected no uploaded pack telemetry, got %+v", snap)
	}
	if snap.DedupedUploadCount != 2 ||
		snap.DedupedUploadBytes != int64(len("duplicate-a")+len("duplicate-b")) {
		t.Fatalf("unexpected dedup telemetry: %+v", snap)
	}
	if snap.PendingUploadBytes != 0 || snap.PendingUploadCount != 0 {
		t.Fatalf("expected no pending upload after all-duplicate cleanup, got %+v", snap)
	}
}

// TestSyncControllerFlushDuplicateProbeErrorPreservesDirty retains dirty work when deduplication fails.
func TestSyncControllerFlushDuplicateProbeErrorPreservesDirty(t *testing.T) {
	// Open the dirty store and pack manifest.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	// Mark one block dirty.
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, "https://example.invalid", DefaultSigningEnvPrefix, priv, pid.String())
	upper := newSyncTestBlockStore()
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()
	addSyncDirtyBlock(t, ctx, wtx, upper, "dirty block")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}

	// Flush against a lower store whose probe fails.
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     func() (*SessionClient, error) { return cli, nil },
		resourceID: "test-res",
		mfst:       mfst,
		lower:      newSyncTestErrorLowerPackfileStore(),
		upper:      upper,
	}
	if _, err := s.flush(ctx, true); err == nil {
		t.Fatal("expected duplicate probe error")
	}

	// The dirty block remains for the next flush.
	if n := countSyncDirtyKeys(t, ctx, dirtyStore); n != 1 {
		t.Fatalf("expected dirty key to remain after probe error, got %d", n)
	}
}

// TestSyncControllerFlushOrdersBlocksByGCGraph writes parent blocks before their children.
func TestSyncControllerFlushOrdersBlocksByGCGraph(t *testing.T) {
	// Open the dirty store and pack manifest.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	// Start the push server and a client for it.
	push := startTestPushServer(t)
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, priv, pid.String())

	// Open a dirty transaction over the upper store.
	upper := newSyncTestBlockStore()
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()

	// Mark four blocks dirty out of graph order.
	stray := addSyncDirtyBlock(t, ctx, wtx, upper, "stray")
	childA := addSyncDirtyBlock(t, ctx, wtx, upper, "child-a")
	rootB := addSyncDirtyBlock(t, ctx, wtx, upper, "root-b")
	rootA := addSyncDirtyBlock(t, ctx, wtx, upper, "root-a")
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}

	// List child-a under root-a.
	graph := syncTestLists{rootA.MarshalString(): {childA}}

	// Flush in subtree order.
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     func() (*SessionClient, error) { return cli, nil },
		resourceID: "test-res",
		mfst:       mfst,
		lower:      push.lower(t),
		upper:      upper,
		childLists: graph,
	}
	if _, err := s.flush(ctx, true); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Each root's children follow it at once, and every block is packed.
	got := readPackPhysicalKeys(t, testPushedBody(t, push))
	i := slices.Index(got, rootA.GetHash().MarshalString())
	if len(got) != 4 || i < 0 || i+1 >= len(got) || got[i+1] != childA.GetHash().MarshalString() {
		t.Fatalf("physical pack order = %v, want child-a right after root-a", got)
	}
	for _, ref := range []*block.BlockRef{rootB, stray} {
		if !slices.Contains(got, ref.GetHash().MarshalString()) {
			t.Fatalf("physical pack order = %v, missing %s", got, ref.MarshalString())
		}
	}
}

// TestSyncControllerFlushChunksBlockCountCeiling splits packs at the wire block limit.
func TestSyncControllerFlushChunksBlockCountCeiling(t *testing.T) {
	// Open the dirty store and pack manifest.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}

	// Start the push server and a client for it.
	push := startTestPushServer(t)
	priv, pid := generateTestKeypair(t)
	cli := NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, priv, pid.String())

	// Mark one block more than a pack may hold dirty.
	blockCount := int(writer.DefaultMaxBlocksPerPack) + 1

	// Store blocks and their upload markers in one committed queue.
	upper := newSyncTestBlockStore()
	wtx, err := dirtyStore.NewTransaction(ctx, true)
	if err != nil {
		t.Fatalf("new dirty tx: %v", err)
	}
	defer wtx.Discard()

	// Fill the queue with distinct payloads under the same transaction.
	for i := range blockCount {
		data := []byte("small dirty block " + strconv.Itoa(i))
		ref, _, err := upper.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatalf("put upper block: %v", err)
		}
		if _, err := markPendingUploads(ctx, wtx, []block_store_writeback.Mark{{Hash: ref.GetHash(), Size: int64(len(data))}}); err != nil {
			t.Fatalf("set dirty key: %v", err)
		}
	}
	if err := wtx.Commit(ctx); err != nil {
		t.Fatalf("commit dirty tx: %v", err)
	}

	// Flush the dirty set.
	s := &syncController{
		le:         logrus.NewEntry(logrus.New()),
		store:      dirtyStore,
		client:     func() (*SessionClient, error) { return cli, nil },
		resourceID: "test-res",
		mfst:       mfst,
		lower:      push.lower(t),
		upper:      upper,
	}
	if _, err := s.flush(ctx, true); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// The flush split the blocks across two packs within the ceiling.
	pushed := push.committed()
	if len(pushed) != 2 {
		t.Fatalf("expected 2 sync pushes, got %d", len(pushed))
	}
	total := 0
	for i, pack := range pushed {
		if len(pack.req.GetBloomFilter()) == 0 {
			t.Fatal("missing bloom filter")
		}
		count := int(pack.req.GetBlockCount()) //nolint:gosec // test counts are small
		if count > int(writer.DefaultMaxBlocksPerPack) {
			t.Fatalf("push %d exceeded block ceiling: %d", i, count)
		}
		total += count
	}

	// Every block was pushed and the manifest holds both packs.
	if total != blockCount {
		t.Fatalf("pushed block total = %d, want %d", total, blockCount)
	}
	if len(mfst.GetEntries()) != 2 {
		t.Fatalf("expected 2 manifest entries, got %d", len(mfst.GetEntries()))
	}
	assertSyncPackEntryMetadata(t, mfst.GetEntries())
}

// TestSyncControllerFlushSkipsCorruptDirtyBlock puts a dirty block whose local
// bytes fail their hash among good ones: the good ones upload and clear, the
// corrupt one stays dirty and counted, the flush reports it held back as a
// count, not an error, and the upper store keeps every block.
func TestSyncControllerFlushSkipsCorruptDirtyBlock(t *testing.T) {
	// Mark three blocks dirty and read the second as corrupt.
	ctx := context.Background()
	dirtyStore := newSyncTestKvStore()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}
	const blockCount = 3
	inner := newSyncTestBlockStore()
	addSyncChunkBlocks(t, ctx, dirtyStore, inner, blockCount)
	var refs []*block.BlockRef
	for i := range blockCount {
		ref, err := block.BuildBlockRef(bytes.Repeat([]byte{byte(i + 1)}, syncTestChunkBlockBytes), nil)
		if err != nil {
			t.Fatal(err)
		}
		refs = append(refs, ref)
	}
	upper := &corruptUpperStore{StoreOps: inner, bad: refs[1].GetHash()}

	// Flush through the production packer and HTTP upload.
	push := newTestPushServer(t, "")
	srv := httptest.NewServer(push)
	defer srv.Close()
	push.base = srv.URL
	priv, pid := generateTestKeypair(t)
	s := &syncController{
		le:    logrus.NewEntry(logrus.New()),
		store: dirtyStore,
		client: func() (*SessionClient, error) {
			return NewSessionClient(http.DefaultClient, srv.URL, DefaultSigningEnvPrefix, priv, pid.String()), nil
		},
		resourceID: "test-res",
		mfst:       mfst,
		lower:      push.lower(t),
		upper:      upper,
	}
	if corrupt, err := s.flush(ctx, false); err != nil || corrupt != 1 {
		t.Fatalf("flush = %d corrupt, %v; want 1, nil", corrupt, err)
	}

	// The two good blocks uploaded, and no pushed pack holds the corrupt one.
	var pushedBlocks int
	for _, pack := range push.committed() {
		pushedBlocks += int(pack.req.GetBlockCount()) //nolint:gosec // test counts are small
		if slices.Contains(readPackPhysicalKeys(t, pack.data), refs[1].GetHash().MarshalString()) {
			t.Fatal("pushed pack holds the corrupt block")
		}
	}
	if pushedBlocks != blockCount-1 {
		t.Fatalf("pushed %d blocks, want %d", pushedBlocks, blockCount-1)
	}

	// Only the corrupt block stays dirty, and the queue still counts it.
	pending, err := s.scanDirtyCandidates(ctx)
	if err != nil || len(pending) != 1 || !pending[0].hash.EqualVT(refs[1].GetHash()) {
		t.Fatalf("dirty after flush = %d blocks, %v; want only the corrupt block", len(pending), err)
	}
	if state := readPersistedPending(t, dirtyStore); state.GetCount() != 1 {
		t.Fatalf("persisted queue counts %d blocks, want 1", state.GetCount())
	}

	// Nothing was freed: the upper store holds every block.
	for i, ref := range refs {
		found, err := inner.GetBlockExists(ctx, ref)
		if err != nil || !found {
			t.Fatalf("block %d after flush: found = %v, err = %v", i, found, err)
		}
	}

	// A second flush skips it again and still uploads nothing twice.
	if corrupt, err := s.flush(ctx, false); err != nil || corrupt != 1 {
		t.Fatalf("second flush = %d corrupt, %v; want 1, nil", corrupt, err)
	}
	if n := countSyncDirtyKeys(t, ctx, dirtyStore); n != 1 {
		t.Fatalf("dirty entries after second flush = %d, want 1", n)
	}
}

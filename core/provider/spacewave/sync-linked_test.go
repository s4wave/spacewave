package provider_spacewave

import (
	"bytes"
	"context"
	"math/rand/v2"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	packfile_manifest "github.com/s4wave/spacewave/core/provider/spacewave/packfile/manifest"
	"github.com/s4wave/spacewave/db/block"
	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/sirupsen/logrus"
)

// newLinkedTestController returns a sync controller over push whose linked
// lane keeps its index and queue in kv and reads with a Space transform.
func newLinkedTestController(t *testing.T, push *testPushServer, kv kvtx.Store) (*syncController, block.Transformer) {
	// Build the controller over an empty manifest.
	t.Helper()
	ctx := t.Context()
	mfst, err := packfile_manifest.New(ctx, newSyncTestKvStore())
	if err != nil {
		t.Fatalf("new manifest: %v", err)
	}
	priv, pid := generateTestKeypair(t)
	s := &syncController{
		le:    logrus.NewEntry(logrus.New()),
		store: newSyncTestKvStore(),
		client: func() (*SessionClient, error) {
			return NewSessionClient(http.DefaultClient, push.base, DefaultSigningEnvPrefix, priv, pid.String()), nil
		},
		resourceID: "test-res",
		mfst:       mfst,
		lower:      push.lower(t),
		upper:      newSyncTestBlockStore(),
	}

	// Open the lane and give its files the Space's transform.
	if s.linked, err = newLinkedLane(ctx, s, kv); err != nil {
		t.Fatalf("open linked lane: %v", err)
	}
	enc, err := transform_blockenc.NewBlockEnc(&transform_blockenc.Config{
		BlockEnc: blockenc.DefaultBlockEnc,
		Key:      bytes.Repeat([]byte{7}, 32),
	})
	if err != nil {
		t.Fatalf("new transform: %v", err)
	}
	xfrm := block_transform.NewTransformerWithSteps([]block_transform.Step{enc})
	s.linked.files.SetTransformer(xfrm)
	return s, xfrm
}

// testFileData returns n random bytes from seed.
func testFileData(n int, seed uint64) []byte {
	data := make([]byte, n)
	r := rand.New(rand.NewPCG(seed, seed))
	for i := range data {
		data[i] = byte(r.Uint32())
	}
	return data
}

// linkTestFile writes data to the file at path, or a new file when path is
// empty, and links it through idx, returning its path.
func linkTestFile(t *testing.T, idx *block_store_filesegment.Index, xfrm block.Transformer, path string, data []byte) string {
	// Write the file.
	t.Helper()
	if path == "" {
		path = filepath.Join(t.TempDir(), "linked.bin")
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	// Link it.
	if _, err := block_store_filesegment.LinkFile(t.Context(), idx, path, nil, xfrm, 0, nil); err != nil {
		t.Fatalf("link file: %v", err)
	}
	return path
}

// startLinkedLane runs the lane until the test ends and returns a func that
// waits for its queue to empty.
func startLinkedLane(t *testing.T, l *linkedLane) func() {
	// Run the lane.
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() {
		defer close(done)
		l.Execute(ctx)
	}()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	// Wait for an empty queue with no reconcile due.
	return func() {
		t.Helper()
		deadline := time.After(10 * time.Second)
		for {
			var idle bool
			var changed <-chan struct{}
			l.bcast.HoldLock(func(_ func(), getWait func() <-chan struct{}) {
				idle, changed = l.pending == 0 && !l.reconcile, getWait()
			})
			if idle {
				return
			}
			select {
			case <-changed:
			case <-deadline:
				t.Fatal("linked queue did not empty")
			}
		}
	}
}

// indexedRefs returns every ref the lane's index holds.
func indexedRefs(t *testing.T, l *linkedLane) []*block.BlockRef {
	t.Helper()
	refs, err := l.files.GetIndex().Refs(t.Context(), nil, 1<<20)
	if err != nil {
		t.Fatalf("index refs: %v", err)
	}
	return refs
}

// queuedRefs returns the hashes of the lane's queued refs, and fails unless
// the lane's count matches them.
func queuedRefs(t *testing.T, l *linkedLane) map[string]struct{} {
	// Read the queue.
	t.Helper()
	page, err := l.readQueue(t.Context(), nil)
	if err != nil {
		t.Fatalf("read queue: %v", err)
	}
	queued := make(map[string]struct{}, len(page))
	for _, e := range page {
		queued[e.ref.GetHash().MarshalString()] = struct{}{}
	}

	// Compare the count.
	var pending int
	l.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) { pending = l.pending })
	if pending != len(page) {
		t.Fatalf("lane counts %d queued chunks, queue holds %d", pending, len(page))
	}
	return queued
}

// requireInCloud fails unless the cloud's catalog holds each ref.
func requireInCloud(t *testing.T, s *syncController, refs []*block.BlockRef, want bool) {
	t.Helper()
	held, err := s.lower.SnapshotManifest().GetBlockExistsBatch(t.Context(), refs)
	if err != nil {
		t.Fatalf("check catalog: %v", err)
	}
	for i, ok := range held {
		if ok != want {
			t.Fatalf("chunk %d in the cloud: %v, want %v", i, ok, want)
		}
	}
}

// TestLinkedLaneUploadsBesideFlush checks that a flush of a tree's own blocks
// succeeds while its linked chunks exist only in their file and the lane is
// still uploading them, that an edit meanwhile deletes the queue entries of
// the chunks it supersedes in its own write, and that every current chunk
// ends in the cloud.
func TestLinkedLaneUploadsBesideFlush(t *testing.T) {
	// Hold the first upload, the lane's, until the flush is done.
	ctx := t.Context()
	push := startTestPushServer(t)
	laneUploading, releaseLane := make(chan struct{}), make(chan struct{})
	var held atomic.Bool
	push.onUpload = func(*packfile.PushRequest) {
		if held.CompareAndSwap(false, true) {
			close(laneUploading)
			<-releaseLane
		}
	}

	// Link a file and start the lane.
	s, xfrm := newLinkedTestController(t, push, newSyncTestKvStore())
	idx := s.linked.files.GetIndex()
	data := testFileData(3<<20, 1)
	path := linkTestFile(t, idx, xfrm, "", data)
	old := indexedRefs(t, s.linked)
	if len(old) < 2 {
		t.Fatalf("linked %d chunks, want several", len(old))
	}
	idle := startLinkedLane(t, s.linked)
	select {
	case <-laneUploading:
	case <-time.After(10 * time.Second):
		t.Fatal("the lane did not upload")
	}

	// Edit the file's middle and link it again: the superseded chunks leave
	// the queue in that write.
	copy(data[1<<20:], testFileData(1<<20, 4))
	linkTestFile(t, idx, xfrm, path, data)
	chunks := indexedRefs(t, s.linked)
	current := make(map[string]struct{}, len(chunks))
	for _, ref := range chunks {
		current[ref.GetHash().MarshalString()] = struct{}{}
	}
	queued := queuedRefs(t, s.linked)
	var superseded int
	for _, ref := range old {
		key := ref.GetHash().MarshalString()
		if _, ok := current[key]; ok {
			continue
		}
		superseded++
		if _, ok := queued[key]; ok {
			t.Fatalf("superseded chunk %s is still queued", key)
		}
	}
	if superseded == 0 {
		t.Fatal("the edit superseded no chunk")
	}

	// Flush a dirty tree block while the lane's pack is in flight.
	addSyncChunkBlocks(t, ctx, s.store, s.upper, 1)
	if err := s.FlushNow(ctx); err != nil {
		t.Fatalf("flush while the lane uploads: %v", err)
	}
	if packs := push.committed(); len(packs) != 1 {
		t.Fatalf("committed %d packs during the lane's upload, want the flush's 1", len(packs))
	}
	requireInCloud(t, s, chunks, false)

	// Release the lane: every current chunk ends in the cloud and the queue
	// empties.
	close(releaseLane)
	idle()
	requireInCloud(t, s, chunks, true)
	if queued := queuedRefs(t, s.linked); len(queued) != 0 {
		t.Fatalf("%d chunks still queued", len(queued))
	}
}

// TestLinkedLaneQueuesUnheldChunks checks that the lane's reconcile uploads
// indexed chunks that were never queued, as after a crash between the index
// write and the World write, and drops the entries of a path the index no
// longer names.
func TestLinkedLaneQueuesUnheldChunks(t *testing.T) {
	// Link a file through an index with no queue, then open the lane over it.
	push := startTestPushServer(t)
	kv := newSyncTestKvStore()
	s, xfrm := newLinkedTestController(t, push, kv)
	unqueued := block_store_filesegment.NewIndex(kv, nil)
	linkTestFile(t, unqueued, xfrm, "", testFileData(2<<20, 2))
	kept := indexedRefs(t, s.linked)

	// Link and drop a second file: the drop deletes its queue entries.
	idx := s.linked.files.GetIndex()
	dropped := linkTestFile(t, idx, xfrm, "", testFileData(2<<20, 3))
	if err := idx.DropPath(t.Context(), dropped); err != nil {
		t.Fatalf("drop path: %v", err)
	}
	if queued := queuedRefs(t, s.linked); len(queued) != 0 {
		t.Fatalf("%d chunks of the dropped file still queued", len(queued))
	}

	// The lane uploads the kept file's chunks and empties its queue.
	startLinkedLane(t, s.linked)()
	requireInCloud(t, s, kept, true)
	if packs := push.committed(); len(packs) != 1 {
		t.Fatalf("committed %d packs, want 1", len(packs))
	}
}

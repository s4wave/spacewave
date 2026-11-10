package provider_local

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store "github.com/s4wave/spacewave/db/block/store"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	block_store_writeback "github.com/s4wave/spacewave/db/block/store/writeback"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"github.com/sirupsen/logrus"
)

// TestPackaheadPlacedStore publishes through a placed store whose new values
// divert to its packahead log, and drains the log to a fake bucket with a
// budget that keeps no pack. The publication queues nothing for the block
// upload and leaves no bytes in the Volume; UploadedBefore holds only once the
// packs upload; the drained blocks read back from the bucket through the
// store's read chain; and a reclaim keeps the published blocks and drops
// only a value no child list names, as a crash between the append and the
// publication leaves.
func TestPackaheadPlacedStore(t *testing.T) {
	// Open the placed store over a Volume, with its writes diverted to a log.
	ctx := t.Context()
	local, vol := newOwnedVolume(t)
	markers, release, err := vol.AccessObjectStore(ctx, "test/markers", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(release)
	log, err := packahead.Open(ctx, t.TempDir(), packahead.Options{Linger: 10 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(log.Close)
	packed := block_store_writeback.NewPackedStore(log, local, true)
	wb, err := block_store_writeback.NewStoreWithPacks(ctx, packed, vol, markers, "test/markers", true)
	if err != nil {
		t.Fatal(err)
	}
	tracker, _ := vol.GetRefGraph().(block_gc.BlockTracker)
	p := &placementState{wb: wb, local: packed, packed: packed, tracker: tracker}
	p.backend.Store(&account_settings.StorageBackend{Id: "packs"})
	b := &BlockStore{store: block_store.NewStore("local", wb), decodedBlocks: block.NewDecodedBlockCache(), placement: p}
	t.Cleanup(b.decodedBlocks.Close)

	// Open the pack store on a fake bucket, and read the bucket after the
	// local store.
	srv := httptest.NewServer(&fakeS3{objects: make(map[string][]byte)})
	t.Cleanup(srv.Close)
	openRemote := func() backendStore {
		t.Helper()
		store, err := buildS3BlockStore(logrus.NewEntry(logrus.New()), &account_settings.S3Location{
			Endpoint:   strings.TrimPrefix(srv.URL, "http://"),
			Region:     "us-east-1",
			Bucket:     "spaces",
			DisableSsl: true,
			KeepPacks:  true,
		}, "world", &block_store_s3.Credentials{AccessKeyId: "key", SecretAccessKey: "secret"}, nil)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(store.Close)
		return store
	}
	remote := openRemote()
	recorded := &readRecorder{StoreOps: packed, reads: &p.reads}
	reads, _ := p.readChains(recorded, packed, func() block.StoreOps { return remote }, func() block.StoreOps { return nil }, nil)

	// Publish a leaf and its parent under a head.
	leaf, parent := []byte("published leaf"), []byte("published parent")
	leafRef, _ := block.BuildBlockRef(leaf, nil)
	parentRef, _ := block.BuildBlockRef(parent, nil)
	err = b.PublishAtomic(ctx, &block.AtomicPublication{
		Entries: []*block.PutBatchEntry{
			{Ref: leafRef, Data: leaf, RefsKnown: true},
			{Ref: parentRef, Data: parent, Refs: []*block.BlockRef{leafRef}, RefsKnown: true},
		},
		BucketID: "bucket",
		TrackGC:  true,
		Heads: []*block.AtomicHeadUpdate{{ObjectStoreID: "head", Key: []byte("head"), Replace: func(context.Context, []byte, bool) ([]byte, error) {
			return []byte("published"), nil
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := wb.GetStatus(); status.Pending != 0 {
		t.Fatalf("publication queued %d blocks for the block upload", status.Pending)
	}
	for _, ref := range []*block.BlockRef{leafRef, parentRef} {
		if _, found, err := vol.GetBlock(ctx, ref); err != nil || found {
			t.Fatalf("Volume holds the bytes of %s: %v %v", ref.MarshalString(), found, err)
		}
	}

	// Append a value no child list names.
	garbage := []byte("appended, never published")
	garbageRef, _ := block.BuildBlockRef(garbage, nil)
	value, err := block.EncodeBlockObject(garbage, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := log.Append(ctx, []packahead.Block{{Key: packfile.BlockKey(garbageRef.GetHash()), Value: value}}); err != nil {
		t.Fatal(err)
	}

	// Nothing is uploaded until the drain runs.
	p.drain = packahead.NewDrain(log, packahead.DrainOptions{KeepBytes: 1})
	if uploaded, err := b.UploadedBefore(ctx, time.Now()); err != nil || uploaded {
		t.Fatalf("UploadedBefore before the drain = %v, %v", uploaded, err)
	}

	// Drain every pack, evicting each past the budget.
	packRemote, ok := remote.(packahead.Remote)
	if !ok {
		t.Fatal("the S3 pack store takes no packfiles")
	}
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- p.drain.Run(runCtx, packRemote) }()
	deadline := time.After(10 * time.Second)
	for {
		status, changed := b.GetPackStatus()
		if status.LocalBytes == 0 && status.Pending == 0 && !log.Has(packfile.BlockKey(garbageRef.GetHash())) {
			break
		}
		select {
		case <-changed:
		case <-time.After(10 * time.Millisecond):
		case <-deadline:
			t.Fatalf("drain did not empty the log: %+v", *status)
		}
	}
	cancel()
	<-done
	if st, ok := b.GetPackaheadStats(); !ok || !st.Divert || st.Diverted != 2 || st.DivertedBytes == 0 || st.Pending != 0 || st.Written == 0 || st.WrittenBytes == 0 || st.Failures != 0 {
		t.Fatalf("packahead stats after the drain = %+v, %v; want 2 diverted, none pending, some written, no failure", st, ok)
	}
	if uploaded, err := b.UploadedBefore(ctx, time.Now()); err != nil || !uploaded {
		t.Fatalf("UploadedBefore after the drain = %v, %v", uploaded, err)
	}

	// The read chain reads the published blocks from the bucket.
	for ref, want := range map[*block.BlockRef][]byte{leafRef: leaf, parentRef: parent} {
		data, found, err := reads.GetBlock(ctx, ref)
		if err != nil || !found || !bytes.Equal(data, want) {
			t.Fatalf("read %s = %q, %v, %v", ref.MarshalString(), data, found, err)
		}
	}
	fresh := openRemote()
	for _, ref := range []*block.BlockRef{leafRef, parentRef, garbageRef} {
		if found, err := fresh.GetBlockExists(ctx, ref); err != nil || !found {
			t.Fatalf("bucket lacks %s: %v, %v", ref.MarshalString(), found, err)
		}
	}

	// A reclaim keeps the published blocks and drops only the garbage.
	live, err := p.live(ctx, []*block.BlockRef{leafRef, parentRef, garbageRef})
	if err != nil {
		t.Fatal(err)
	}
	if !live[0] || !live[1] || live[2] {
		t.Fatalf("live = %v, want the published blocks alone", live)
	}
}

package provider_local

import (
	"bytes"
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ulid"
	account_settings "github.com/s4wave/spacewave/core/account/settings"
	"github.com/s4wave/spacewave/core/sobject"
	"github.com/s4wave/spacewave/db/block"
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
	tracker, _ := any(vol).(childListReader)
	p := &placementState{wb: wb, local: packed, tracker: tracker}
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
	err = b.SetRoots(ctx, []*block.PutBatchEntry{
		{Ref: leafRef, Data: leaf, RefsKnown: true},
		{Ref: parentRef, Data: parent, Refs: []*block.BlockRef{leafRef}, RefsKnown: true},
	}, []block.NamedRoot{{Name: "main", Ref: parentRef}})
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

// TestMountedStoreDivertsToPacks mounts a Space's block store with a pack
// directory and the store listed, writes through the mounted handle, and walks
// the switch: with it on the Volume holds the child list but not the bytes and
// the write leaves no upload marker; remounted with it off, new writes land in
// the Volume while the packed block still reads from the reopened log; and
// remounted with it on again, both read.
func TestMountedStoreDivertsToPacks(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()

	// Start a local account whose provider keeps packs in a temp directory.
	packDir := t.TempDir()
	_, _, acc, _, release := setupProviderAndSessionInternal(ctx, t, func(p *Provider) {
		p.SetPackDir(packDir, nil)
	})
	t.Cleanup(release)
	prov := acc.t.p
	soRef, err := acc.CreateSharedObject(ctx, ulid.NewULID(), &sobject.SharedObjectMeta{BodyType: "space"}, "", "")
	if err != nil {
		t.Fatal(err)
	}
	storeID := acc.lookupSharedObjectBlockStoreID(soRef.GetProviderResourceRef().GetId())

	// mount sets the switch and mounts the store afresh. The store's first
	// mount, which CreateSharedObject may have started with the switch off,
	// is awaited before the restart, so the store returned is always the one
	// mounted after the switch was set, and its log was closed before.
	ref, tkr, _ := acc.bstores.AddKeyRef(storeID)
	t.Cleanup(ref.Release)
	await := func(prev *BlockStore) *BlockStore {
		t.Helper()
		for {
			bs, err := tkr.bstoreCtr.WaitValueChange(ctx, prev, nil)
			if err != nil {
				t.Fatal(err)
			}
			if bs != nil {
				return bs
			}
			prev = nil
		}
	}
	cur := await(nil)
	mount := func(divert bool) *BlockStore {
		t.Helper()
		if divert {
			prov.SetPackDir(packDir, []string{storeID})
		} else {
			prov.SetPackDir(packDir, nil)
		}
		acc.bstores.RestartRoutine(storeID)
		cur = await(cur)
		return cur
	}
	// volumeHolds reports whether the account Volume holds the bytes of ref,
	// past the store's log.
	volumeHolds := func(ref *block.BlockRef) bool {
		t.Helper()
		_, found, err := acc.vol.GetBlock(ctx, ref)
		if err != nil {
			t.Fatal(err)
		}
		return found
	}
	requireRead := func(bs *BlockStore, ref *block.BlockRef, want []byte) {
		t.Helper()
		got, found, err := bs.GetBlock(ctx, ref)
		if err != nil || !found || !bytes.Equal(got, want) {
			t.Fatalf("read %s = %q found %v err %v, want %q", ref.MarshalString(), got, found, err, want)
		}
	}

	// With the switch on, the value reaches the log and not the Volume.
	bs := mount(true)
	packed := []byte("packed block")
	packedRef, _, err := bs.PutBlock(ctx, packed, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	requireRead(bs, packedRef, packed)
	if volumeHolds(packedRef) {
		t.Fatal("the Volume holds a diverted value")
	}
	if status, _ := bs.GetUploadStatus(); status.Pending != 0 {
		t.Fatalf("diverted write left %d upload markers", status.Pending)
	}

	// Unmount and remount with the switch off: the log reopens and still
	// serves the packed block, and a new write lands in the Volume.
	bs = mount(false)
	requireRead(bs, packedRef, packed)
	plain := []byte("plain block")
	plainRef, _, err := bs.PutBlock(ctx, plain, &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	if !volumeHolds(plainRef) {
		t.Fatal("the Volume lacks a block written with the switch off")
	}

	// Switch on again: both blocks read.
	bs = mount(true)
	requireRead(bs, packedRef, packed)
	requireRead(bs, plainRef, plain)
}

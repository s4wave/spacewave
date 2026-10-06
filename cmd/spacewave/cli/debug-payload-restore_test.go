//go:build !js

package spacewave_cli

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aperturerobotics/controllerbus/controller"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	unixfs_sync "github.com/s4wave/spacewave/db/unixfs/sync"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// restoreTestSpace is the Space of the restore test volume.
const restoreTestSpace = "space"

// restoreTestBucket is the bucket that owns the Space's blocks.
const restoreTestBucket = "p/provider/account/blk/" + restoreTestSpace

// restoreFixture is a stopped volume holding a Space with a replay cursor.
type restoreFixture struct {
	le       *logrus.Entry
	path     string
	hashType hash.HashType
	xfrm     block.Transformer
}

// newRestoreFixture builds a stopped volume with the Space's bucket and a
// replay cursor naming a fresh World transform. Each block of preset is
// written owned by the bucket.
func newRestoreFixture(t *testing.T, preset ...[]byte) *restoreFixture {
	// Open a fresh volume with a Space bucket.
	ctx := t.Context()
	f := &restoreFixture{
		le:   logrus.NewEntry(logrus.New()),
		path: filepath.Join(t.TempDir(), "volume.s4wave"),
	}
	vol, err := volume_s4db.NewVolume(ctx, f.le, &volume_s4db.Config{Path: f.path})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := vol.ApplyBucketConfig(ctx, &bucket.Config{Id: restoreTestBucket, Rev: 1}); err != nil {
		t.Fatal(err)
	}

	// Build a World state that names a fresh transform.
	initOp, err := sobject_world_engine.NewInitWorldOp(nil)
	if err != nil {
		t.Fatal(err)
	}
	state, err := sobject_world_engine.BuildInitialInnerState(initOp)
	if err != nil {
		t.Fatal(err)
	}
	f.hashType = vol.GetHashType()
	f.xfrm, err = block_transform.NewTransformer(controller.ConstructOpts{Logger: f.le}, transform_all.BuildFactorySet(), state.GetHeadRef().GetTransformConf())
	if err != nil {
		t.Fatal(err)
	}

	// Store the cursor under the Space's key in an account object store.
	cursor, err := (&sobject_world_engine.ReplayCursor{Base: state, Head: state}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	conf := kvkey.DefaultConfig()
	key := slices.Concat(conf.GetPrefix(), conf.GetObjectStorePrefix(), []byte("account/so/"+restoreTestSpace+"/ls/world-replay/cursor"))
	if err := putKey(ctx, vol, key, cursor); err != nil {
		t.Fatal(err)
	}

	// Write the preset payloads.
	for _, data := range preset {
		_, entries, err := encodePayload(ctx, f.xfrm, f.hashType, data)
		if err != nil {
			t.Fatal(err)
		}
		if err := vol.PrepareOwnedBlockBatch(ctx, restoreTestBucket, entries); err != nil {
			t.Fatal(err)
		}
	}

	// Release the volume as a stopped daemon would.
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}
	return f
}

// open opens the fixture volume and closes it when the test ends.
func (f *restoreFixture) open(t *testing.T) *volume_s4db.Volume {
	vol, err := volume_s4db.NewVolume(t.Context(), f.le, &volume_s4db.Config{Path: f.path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { vol.Close() })
	return vol
}

// requireOwned fails unless the Space's bucket owns the root of data's blob
// and the root reaches each of its other blocks.
func (f *restoreFixture) requireOwned(t *testing.T, vol *volume_s4db.Volume, data []byte) {
	// Rebuild the blob's blocks.
	ctx := t.Context()
	root, entries, err := encodePayload(ctx, f.xfrm, f.hashType, data)
	if err != nil {
		t.Fatal(err)
	}

	// Collect the blocks reachable from the bucket through the blob.
	rg := vol.GetRefGraph()
	reached, err := rg.GetOutgoingRefs(ctx, block_gc.BucketIRI(restoreTestBucket))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(reached, block_gc.BlockIRI(root)) {
		t.Fatalf("bucket does not own %s", root.MarshalString())
	}
	for _, e := range entries {
		refs, err := rg.GetOutgoingRefs(ctx, block_gc.BlockIRI(e.Ref))
		if err != nil {
			t.Fatal(err)
		}
		reached = append(reached, refs...)
	}

	// Require every block of the blob.
	for _, e := range entries {
		if !slices.Contains(reached, block_gc.BlockIRI(e.Ref)) {
			t.Fatalf("bucket does not reach %s", e.Ref.MarshalString())
		}
	}
}

// TestDebugPayloadRestore checks that the restore rebuilds a payload with the
// Space World's transform, refuses a digest it does not reproduce, and writes
// the block owned by the Space's bucket so it decodes to the file's bytes.
func TestDebugPayloadRestore(t *testing.T) {
	// Build a volume without the payload.
	ctx := t.Context()
	f := newRestoreFixture(t)

	// Refuse a digest the file does not reproduce.
	data := bytes.Repeat([]byte("package imports\n"), 1024)
	_, _, _, err := restorePayload(ctx, f.le, f.path, restoreTestSpace, "", data, make([]byte, 32))
	if err == nil || !strings.Contains(err.Error(), "rebuilds digest") {
		t.Fatalf("expected a digest mismatch, got %v", err)
	}

	// Restore the payload under its own digest.
	want, _, err := encodePayload(ctx, f.xfrm, f.hashType, data)
	if err != nil {
		t.Fatal(err)
	}
	ref, _, restored, err := restorePayload(ctx, f.le, f.path, restoreTestSpace, "", data, want.GetHash().GetHash())
	if err != nil {
		t.Fatal(err)
	}
	if !ref.EqualsRef(want) || restored != 1 {
		t.Fatalf("restored %d blocks of %s, expected 1 of %s", restored, ref.MarshalString(), want.MarshalString())
	}

	// Find the payload present on a second restore.
	if _, _, restored, err := restorePayload(ctx, f.le, f.path, restoreTestSpace, "", data, want.GetHash().GetHash()); err != nil || restored != 0 {
		t.Fatalf("second restore wrote %d blocks, err %v", restored, err)
	}

	// Check the bucket owns the block.
	vol := f.open(t)
	f.requireOwned(t, vol, data)

	// Decode the stored block with the World transform.
	encoded, found, err := vol.GetBlock(ctx, ref)
	if err != nil || !found {
		t.Fatalf("read restored block: found %v, err %v", found, err)
	}
	decoded, err := f.xfrm.DecodeBlock(encoded)
	if err != nil {
		t.Fatal(err)
	}

	// Check the blob holds the file's bytes.
	blb := &blob.Blob{}
	if err := blb.UnmarshalVT(decoded); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(blb.GetRawData(), data) {
		t.Fatal("restored blob does not hold the file's bytes")
	}
}

// TestDebugPayloadRestoreSourceTree checks that the source tree restore skips
// blocks the volume holds, counts a repeated file once, writes every block of
// each write extent's chunked blob, and writes nothing in a dry run.
func TestDebugPayloadRestoreSourceTree(t *testing.T) {
	// Make a present file, a missing file, and a missing file written in two
	// extents.
	ctx := t.Context()
	present := []byte("package present\n")
	missing := []byte("package missing\n")
	large := make([]byte, unixfs_sync.CopyBufferSize+2<<20)
	for i := range large {
		large[i] = byte(i * 7 / 13)
	}

	// Lay them out in a tree with a copy of the missing file.
	root := t.TempDir()
	files := map[string][]byte{
		"present.go":     present,
		"a/missing.go":   missing,
		"b/missing.go":   missing,
		"vendor/big.bin": large,
	}
	for name, data := range files {
		p := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	f := newRestoreFixture(t, present)

	// Count the distinct blocks of the large file's extents.
	extents := slices.Collect(slices.Chunk(large, unixfs_sync.CopyBufferSize))
	largeBlocks := make(map[string]struct{})
	for _, extent := range extents {
		_, entries, err := encodePayload(ctx, f.xfrm, f.hashType, extent)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) < 2 {
			t.Fatalf("expected a chunked blob, got %d blocks", len(entries))
		}
		for _, e := range entries {
			largeBlocks[e.Ref.MarshalString()] = struct{}{}
		}
	}
	wantMissing := 1 + len(largeBlocks)

	// Report the missing blocks without writing them.
	res, err := restoreSourceTree(ctx, f.le, f.path, restoreTestSpace, "", root, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.files != 4 || res.present != 1 || res.restored != wantMissing {
		t.Fatalf("dry run: %+v, expected 4 files, 1 present, %d missing", res, wantMissing)
	}

	// Restore them, then find nothing missing.
	res, err = restoreSourceTree(ctx, f.le, f.path, restoreTestSpace, "", root, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.restored != wantMissing {
		t.Fatalf("restored %d blocks, expected %d", res.restored, wantMissing)
	}
	res, err = restoreSourceTree(ctx, f.le, f.path, restoreTestSpace, "", root, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.restored != 0 || res.present != 1+wantMissing {
		t.Fatalf("after restore: %+v, expected none missing", res)
	}

	// Check the bucket owns every restored block.
	vol := f.open(t)
	f.requireOwned(t, vol, missing)
	for _, extent := range extents {
		f.requireOwned(t, vol, extent)
	}
}

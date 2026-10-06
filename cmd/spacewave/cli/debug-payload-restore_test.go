//go:build !js

package spacewave_cli

import (
	"bytes"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/aperturerobotics/bbolt"
	"github.com/aperturerobotics/controllerbus/controller"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block/blob"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	"github.com/sirupsen/logrus"
)

// TestDebugPayloadRestore checks that the restore rebuilds a payload with the
// Space World's transform, refuses a digest it does not reproduce, and writes
// the block owned by the Space's bucket so it decodes to the file's bytes.
func TestDebugPayloadRestore(t *testing.T) {
	// Open a fresh volume with a Space bucket.
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "volume.s4wave")
	le := logrus.NewEntry(logrus.New())
	vol, err := volume_bolt.NewBolt(ctx, le, &volume_bolt.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	const spaceID = "space"
	const bucketID = "p/provider/account/blk/" + spaceID
	if _, _, err := vol.PrepareOwnedBlock(ctx, bucketID, []byte("world"), nil); err != nil {
		t.Fatal(err)
	}

	// Save a replay cursor whose World names a fresh transform.
	initOp, err := sobject_world_engine.NewInitWorldOp(nil)
	if err != nil {
		t.Fatal(err)
	}
	state, err := sobject_world_engine.BuildInitialInnerState(initOp)
	if err != nil {
		t.Fatal(err)
	}

	// Store the cursor under the Space's key in an account object store.
	cursor, err := (&sobject_world_engine.ReplayCursor{Base: state, Head: state}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	conf := kvkey.DefaultConfig()
	key := slices.Concat(conf.GetPrefix(), conf.GetObjectStorePrefix(), []byte("account/so/"+spaceID+"/ls/world-replay/cursor"))
	err = volume_bolt.GetBoltDB(vol).Update(func(tx *bbolt.Tx) error {
		return tx.Bucket([]byte("hydra")).Put(key, cursor)
	})
	if err != nil {
		t.Fatal(err)
	}

	// Release the volume as a stopped daemon would.
	hashType := vol.GetHashType()
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	// Refuse a digest the file does not reproduce.
	data := bytes.Repeat([]byte("package imports\n"), 1024)
	_, err = restorePayload(ctx, le, path, spaceID, "", data, make([]byte, 32))
	if err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("expected a digest mismatch, got %v", err)
	}

	// Restore the payload under its own digest.
	xfrmConf := state.GetHeadRef().GetTransformConf()
	want, _, err := encodePayload(ctx, le, xfrmConf, hashType, data)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := restorePayload(ctx, le, path, spaceID, "", data, want.GetHash().GetHash())
	if err != nil {
		t.Fatal(err)
	}
	if !ref.EqualsRef(want) {
		t.Fatalf("restored %s, expected %s", ref.MarshalString(), want.MarshalString())
	}

	// Check the bucket owns the block.
	vol, err = volume_bolt.NewBolt(ctx, le, &volume_bolt.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	owned, err := vol.GetRefGraph().GetOutgoingRefs(ctx, block_gc.BucketIRI(bucketID))
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(owned, block_gc.BlockIRI(ref)) {
		t.Fatalf("bucket does not own %s", ref.MarshalString())
	}

	// Read the stored block.
	encoded, found, err := vol.GetBlock(ctx, ref)
	if err != nil || !found {
		t.Fatalf("read restored block: found %v, err %v", found, err)
	}

	// Decode it with the World transform.
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: le}, transform_all.BuildFactorySet(), xfrmConf)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := xfrm.DecodeBlock(encoded)
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

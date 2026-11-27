package forge_target

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"

	timestamp "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/prng"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/file"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	hydra_all "github.com/s4wave/spacewave/db/core/all"
	"github.com/s4wave/spacewave/db/testbed"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	"github.com/s4wave/spacewave/db/volume"
	"github.com/s4wave/spacewave/db/world"
	world_block_engine "github.com/s4wave/spacewave/db/world/block/engine"
	forge_value "github.com/s4wave/spacewave/forge/value"
	"github.com/sirupsen/logrus"
)

// buildTestbedHandle builds a testbed with a handle.
func buildTestbedHandle(t *testing.T) (*testbed.Testbed, world.WorldState, ExecControllerHandle) {
	// Create the context and logger for the execution-handle testbed.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// Start the storage testbed and register its controller factories.
	tb, err := testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(tb.Release)
	hydra_all.AddFactories(tb.Bus, tb.StaticResolver)

	// Construct and mount the world controller used by the handle.
	engineID := "forge-target-test"
	volumeID := tb.Volume.GetID()
	bucketID := tb.BucketId
	objectStoreID := engineID
	wh, worldCtrlRef, err := world.StartEngine(
		ctx,
		tb.Bus,
		world_block_engine.NewConfig(
			engineID,
			volumeID, bucketID,
			objectStoreID,
			nil,
			nil,
			false,
		),
		nil,
	)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(worldCtrlRef.Release)

	// Construct the execution handle over a stage of the mounted World state
	// held until the test ends.
	worldState := world.NewEngineWorldState(wh, true)
	stage, err := worldState.StageWorldState(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	t.Cleanup(stage.Release)
	ts := timestamp.Now()
	uniqueID := "test-handle"
	handle := ExecControllerHandleWithAccess(uniqueID, tb.Volume.GetPeerID(), stage.AccessWorldState, ts)
	return tb, worldState, handle
}

// TestStoreBlobValue tests storing a byte slice as a blob value.
func TestStoreBlobValue(t *testing.T) {
	// Create the storage testbed and execution handle for the blob round trip.
	tb, _, handle := buildTestbedHandle(t)
	ctx := tb.Context

	// Store a large blob value.
	rnd := prng.BuildSeededReader([]byte("test-store-blob-value"))

	// Fill a large byte slice from the seeded blob reader.
	dat := make([]byte, 250000) // 250kb
	_, err := io.ReadFull(rnd, dat)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Store the large byte slice as a blob value.
	fv, err := StoreBlobValueFromBytes(ctx, handle, dat)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Load the blob and compare its bytes with the stored value.
	outData, err := LoadBlobValueToBytes(ctx, handle, fv)
	if err != nil {
		t.Fatal(err.Error())
	}

	// Verify the loaded blob matches the stored bytes.
	if !bytes.Equal(outData, dat) {
		t.Fatalf("output value was different: len(%d) and expected len(%d)", len(outData), len(dat))
	}
}

// TestCopyValueToBucketTree copies a UnixFS output of thousands of files from
// a bucket on another volume into the World, reads every file back with that
// volume closed, and copies it again with the source gone, which succeeds
// only because the repeat copy reads nothing from the source.
func TestCopyValueToBucketTree(t *testing.T) {
	// Create the World and its execution handle.
	tb, worldState, handle := buildTestbedHandle(t)
	ctx := tb.Context

	// Start a second volume to hold the Task's output bucket. The World's
	// volume carries the bucket's config too, since a write cursor that
	// follows a ref into a bucket requires the bucket in its own volume, but
	// only the second volume stores the output's blocks.
	const srcBucketID = "forge-task-output"
	srcVol, srcRelease, err := tb.StartVolume(ctx, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	releaseSrc := sync.OnceFunc(srcRelease)
	t.Cleanup(releaseSrc)
	for _, vol := range []volume.Volume{srcVol, tb.Volume} {
		if _, _, _, err := vol.ApplyBucketConfig(ctx, &bucket.Config{Id: srcBucketID, Rev: 1}); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Build the output tree: 16 directories of 4 subdirectories of 32 files.
	files := fstest.MapFS{}
	for d := range 16 {
		for s := range 4 {
			for f := range 32 {
				name := fmt.Sprintf("d%02d/s%d/f%02d.txt", d, s, f)
				files[name] = &fstest.MapFile{Data: []byte(strings.Repeat(name+"\n", 8)), Mode: 0o644}
			}
		}
	}

	// Write the tree into the output bucket, with a canary block outside it
	// that only the second volume holds.
	srcCursor, _, err := bucket_lookup.BuildEmptyCursor(ctx, tb.Bus, tb.Logger, tb.StepFactorySet, srcBucketID, srcVol.GetID(), nil, nil)
	if err != nil {
		t.Fatal(err.Error())
	}
	writeTs := timestamp.Now()
	btx, bcs := srcCursor.BuildTransaction(nil)
	bcs.SetBlock(unixfs_block.NewFSNode(unixfs_block.NodeType_NodeType_DIRECTORY, 0, writeTs.CloneVT()), true)
	fsTree, err := unixfs_block.NewFSTree(ctx, bcs, unixfs_block.NodeType_NodeType_DIRECTORY)
	if err == nil {
		err = unixfs_block.CopyFSToFSTree(ctx, files, fsTree, nil, writeTs.CloneVT())
	}
	var root *block.BlockRef
	if err == nil {
		root, _, err = btx.Write(ctx, true)
	}
	var canary *block.BlockRef
	if err == nil {
		canary, _, err = srcCursor.PutBlock(ctx, []byte("canary"), &block.PutOpts{RefsKnown: true})
	}
	srcCursor.Release()
	if err != nil {
		t.Fatal(err.Error())
	}
	outRef := &bucket.ObjectRef{BucketId: srcBucketID, RootRef: root}

	// Copy the output into the World and adopt it as a World object.
	val, err := CopyValueToBucket(ctx, handle, forge_value.NewValueWithBucketRef("output", outRef))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !val.GetBlockRef().EqualsRef(root) {
		t.Fatalf("copied root %s, want %s", val.GetBlockRef().MarshalString(), root.MarshalString())
	}
	created, err := worldState.CreateObject(ctx, "forge-output", &bucket.ObjectRef{RootRef: root})
	if err != nil {
		t.Fatal(err.Error())
	}
	world.ReleaseObjectState(created)

	// Remove the source, and wait until the bucket lookup, which drops a
	// closed volume asynchronously, no longer reads the canary.
	releaseSrc()
	canaryReadable := func() bool {
		probeCtx, probeCancel := context.WithTimeout(ctx, 250*time.Millisecond)
		defer probeCancel()
		var found bool
		_ = handle.AccessStorage(probeCtx, &bucket.ObjectRef{BucketId: srcBucketID}, func(cursor *bucket_lookup.Cursor) error {
			_, ok, err := cursor.GetBlock(probeCtx, canary)
			found = err == nil && ok
			return nil
		})
		return found
	}
	for deadline := time.Now().Add(10 * time.Second); canaryReadable(); {
		if time.Now().After(deadline) {
			t.Fatal("source canary still readable after its volume closed")
		}
		time.Sleep(50 * time.Millisecond)
	}

	// Read every file back from the World object.
	readCtx, readCancel := context.WithTimeout(ctx, time.Minute)
	defer readCancel()
	obj, err := world.MustGetObject(readCtx, worldState, "forge-output")
	if err != nil {
		t.Fatal(err.Error())
	}
	objRef, _, err := obj.GetRootRef(readCtx)
	world.ReleaseObjectState(obj)
	if err != nil {
		t.Fatal(err.Error())
	}
	err = worldState.AccessWorldState(readCtx, objRef, func(cursor *bucket_lookup.Cursor) error {
		_, bcs := cursor.BuildTransaction(nil)
		tree, err := unixfs_block.NewFSTree(readCtx, bcs, unixfs_block.NodeType_NodeType_DIRECTORY)
		if err != nil {
			return err
		}
		for name, want := range files {
			node, _, err := unixfs_block.LookupFSTreePath(tree, strings.Split(name, "/"))
			if err != nil {
				return errors.Wrap(err, name)
			}
			fh, err := node.BuildFileHandle(readCtx)
			if err != nil {
				return errors.Wrap(err, name)
			}
			var buf bytes.Buffer
			if err := file.FetchToBuffer(readCtx, fh.GetCursor(), &buf); err != nil {
				return errors.Wrap(err, name)
			}
			if !bytes.Equal(buf.Bytes(), want.Data) {
				return errors.Errorf("%s: read %q", name, buf.String())
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err.Error())
	}

	// Copy again with the source gone: only the root's proof is read.
	copyCtx, copyCancel := context.WithTimeout(ctx, 10*time.Second)
	defer copyCancel()
	val, err = CopyValueToBucket(copyCtx, handle, forge_value.NewValueWithBucketRef("output", outRef))
	if err != nil {
		t.Fatal(err.Error())
	}
	if !val.GetBlockRef().EqualsRef(root) {
		t.Fatalf("second copy root %s, want %s", val.GetBlockRef().MarshalString(), root.MarshalString())
	}
}

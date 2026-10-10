//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/aperturerobotics/util/prng"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

// untrackedCopyBucket copies each block written to the wrapped bucket into a
// volume without recording its edges, as a volume with GC tracking off does.
type untrackedCopyBucket struct {
	bucket.BucketOps
	// vol receives the copies.
	vol *volume_s4db.Volume
}

// PutBlock writes the block and copies it.
func (b *untrackedCopyBucket) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, exists, err := b.BucketOps.PutBlock(ctx, data, opts)
	if err != nil {
		return nil, exists, err
	}
	_, _, err = b.vol.PutBlock(ctx, data, &block.PutOpts{ForceBlockRef: ref})
	return ref, exists, err
}

// PutBlockBatch writes the batch and copies each block.
func (b *untrackedCopyBucket) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	existed, err := b.BucketOps.PutBlockBatch(ctx, entries)
	if err != nil {
		return nil, err
	}
	for _, e := range entries {
		if e.Tombstone {
			continue
		}
		if _, _, err := b.vol.PutBlock(ctx, e.Data, &block.PutOpts{ForceBlockRef: e.Ref}); err != nil {
			return nil, err
		}
	}
	return existed, nil
}

// TestDebugRefRepair checks that the repair counts the edges a volume without
// GC tracking lacks for a UnixFS World, writes nothing on a dry run, adds them
// all once applied, and finds none left afterward.
func TestDebugRefRepair(t *testing.T) {
	// Open a volume configuring the Space's bucket, which GC tracking never
	// rooted.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	path := filepath.Join(t.TempDir(), "volume.s4wave")
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	if _, _, _, err := vol.ApplyBucketConfig(ctx, &bucket.Config{Id: restoreTestBucket, Rev: 1}); err != nil {
		t.Fatal(err)
	}

	// Start a storage testbed whose blocks are copied into the volume
	// untracked.
	htb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer htb.Release()
	base, err := htb.BuildEmptyCursor(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer base.Release()

	// Start the World engine on a cursor over the copying bucket.
	cursor := bucket_lookup.NewCursor(
		ctx,
		htb.Bus,
		le,
		htb.StepFactorySet,
		&untrackedCopyBucket{BucketOps: base.GetBucket(), vol: vol},
		base.GetTransformer(),
		base.GetRef(),
		base.GetOpArgs(),
		base.GetTransformConf(),
	)
	defer cursor.Release()
	eng, err := world_block.NewEngine(ctx, le, cursor, unixfs_world.LookupFsOp, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	defer eng.Close()
	ws := world.NewEngineWorldState(eng, true)

	// Initialize the filesystem.
	const objKey = "fs"
	now := time.Now()
	sender := htb.Volume.GetPeerID()
	fsType := unixfs_world.FSType_FSType_FS_NODE
	if _, _, err := unixfs_world.FsInit(ctx, ws, sender, objKey, fsType, nil, true, now); err != nil {
		t.Fatal(err)
	}

	// Add a chunked file and commit it.
	content := make([]byte, 1<<20)
	if _, err := prng.BuildSeededReader([]byte("ref repair")).Read(content); err != nil {
		t.Fatal(err)
	}
	bw := unixfs_world.NewBatchFSWriter(ws, objKey, fsType, sender)
	defer bw.Release()
	if err := bw.AddFile(ctx, nil, "large.bin", unixfs.NewFSCursorNodeType_File(), int64(len(content)), bytes.NewReader(content), 0o644, now); err != nil {
		t.Fatal(err)
	}
	if err := bw.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Store a replay cursor naming the World and release the volume.
	state := &sobject_world_engine.InnerState{HeadRef: &bucket.ObjectRef{
		RootRef:       eng.GetRootRef().GetRootRef(),
		TransformConf: base.GetTransformConf(),
	}}
	data, err := (&sobject_world_engine.ReplayCursor{Base: state, Head: state}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	conf := kvkey.DefaultConfig()
	key := slices.Concat(conf.GetPrefix(), conf.GetObjectStorePrefix(), []byte("account/so/"+restoreTestSpace+"/ls/world-replay/cursor"))
	if err := putKey(ctx, vol, key, data); err != nil {
		t.Fatal(err)
	}
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	// A dry run counts the missing edges and writes none.
	dry, err := repairVolumeRefs(ctx, le, path, "", true)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("dry run: %+v", dry)
	if dry.spaces != 1 || dry.lacking == 0 || dry.edges == 0 || dry.owned != 1 || dry.rooted != 1 || dry.written != 0 {
		t.Fatalf("dry run: %+v", dry)
	}
	if dry.absent != 0 || dry.undecodable != 0 || dry.untyped["object type "+unixfs_world.FSNodeTypeID] != 0 {
		t.Fatalf("dry run did not walk the whole filesystem: %+v", dry)
	}

	// The dry run left the graph as it was.
	again, err := repairVolumeRefs(ctx, le, path, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if again.edges != dry.edges || again.owned != dry.owned || again.rooted != dry.rooted {
		t.Fatalf("dry run wrote edges: %+v then %+v", dry, again)
	}

	// Applying adds every missing edge.
	applied, err := repairVolumeRefs(ctx, le, path, "", false)
	if err != nil {
		t.Fatal(err)
	}
	want := dry.edges + dry.owned + uint64(dry.rooted)
	if applied.written != want {
		t.Fatalf("applied %d edges, expected %d", applied.written, want)
	}

	// Nothing is missing afterward.
	after, err := repairVolumeRefs(ctx, le, path, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if after.lacking != 0 || after.edges != 0 || after.owned != 0 || after.rooted != 0 || after.blocks != dry.blocks {
		t.Fatalf("after repair: %+v", after)
	}
}

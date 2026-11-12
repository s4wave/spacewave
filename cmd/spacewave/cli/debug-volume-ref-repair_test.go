//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/util/prng"
	"github.com/pkg/errors"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// untrackedCopyBucket copies each block written to the wrapped bucket into a
// volume's store without its child list, as a volume written before puts
// carried child lists holds it.
type untrackedCopyBucket struct {
	// BucketOps writes the testbed's tracked blocks.
	bucket.BucketOps
	// vol receives the copies.
	vol *volume_s4db.Volume
}

// PutBlock writes the block and copies it.
func (b *untrackedCopyBucket) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	// Copy the tracked write's bytes into the untracked volume.
	ref, exists, err := b.BucketOps.PutBlock(ctx, data, opts)
	if err != nil {
		return nil, exists, err
	}
	_, _, err = b.vol.Store.PutBlock(ctx, data, &block.PutOpts{ForceBlockRef: ref})
	return ref, exists, err
}

// PutBlockBatch writes the batch and copies each block.
func (b *untrackedCopyBucket) PutBlockBatch(ctx context.Context, entries []*block.PutBatchEntry) ([]bool, error) {
	// Write the batch into the tracked testbed bucket.
	existed, err := b.BucketOps.PutBlockBatch(ctx, entries)
	if err != nil {
		return nil, err
	}

	// Copy the stored entries without their child lists.
	for _, e := range entries {
		if e.Tombstone {
			continue
		}
		if _, _, err := b.vol.Store.PutBlock(ctx, e.Data, &block.PutOpts{ForceBlockRef: e.Ref}); err != nil {
			return nil, err
		}
	}
	return existed, nil
}

// TestDebugRefRepair checks that the repair counts the child lists an
// unadopted volume lacks for a UnixFS World, writes nothing on a dry run,
// writes them all once applied, and finds none left afterward. Its
// volume-wide inventory counts private-state prefix keys and raw blocks
// without child lists.
func TestDebugRefRepair(t *testing.T) {
	// Open a volume configuring the Space's bucket, and mark it unadopted with
	// its lists unverified, as the upgrade leaves a volume written before puts
	// carried child lists.
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
	writeRecords(t, vol, func(rec *block_gc.Records) error {
		return rec.SetState(ctx, &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_UNADOPTED, UnverifiedLists: true})
	})

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
	eng, err := world_block.NewEngine(ctx, le, cursor, world.NewSchema(unixfs_world.UnixfsTypes).LookupOp, nil, false)
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

	// Store a replay cursor pointing to the World.
	state := &space_world.InnerState{HeadRef: &bucket.ObjectRef{
		RootRef:       eng.GetRootRef().GetRootRef(),
		TransformConf: base.GetTransformConf(),
	}}
	data, err := (&space_world.ReplayCursor{Base: state, Head: state}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	conf := kvkey.DefaultConfig()
	key := slices.Concat(conf.GetPrefix(), conf.GetObjectStorePrefix(), []byte("account/so/"+restoreTestSpace+"/ls/space-world-replay/cursor"))
	if err := putKey(ctx, vol, key, data); err != nil {
		t.Fatal(err)
	}

	// Store raw blocks outside the typed walk. A shorter hash puts the
	// untracked block first in the scan's bounded sample.
	orphanData := []byte("outside the ref graph")
	orphan, _, err := vol.Store.PutBlock(ctx, orphanData, &block.PutOpts{HashType: hash.HashType_HashType_SHA1})
	if err != nil {
		t.Fatal(err)
	}

	// Store raw blocks outside the walk: two with child lists, and one that
	// only a parent's list names, whose own references stay unknown.
	trackedData := [][]byte{[]byte("outgoing only"), []byte("staging only")}
	tracked := make([]*block.BlockRef, len(trackedData))
	for i, data := range trackedData {
		tracked[i], _, err = vol.Store.PutBlock(ctx, data, nil)
		if err != nil {
			t.Fatal(err)
		}
	}
	listed, _, err := vol.Store.PutBlock(ctx, []byte("listed only"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Give two raw blocks a child list.
	writeRecords(t, vol, func(rec *block_gc.Records) error {
		if err := rec.PutChildren(ctx, tracked[0], []*block.BlockRef{listed}); err != nil {
			return err
		}
		return rec.PutChildren(ctx, tracked[1], nil)
	})

	// Store root pointers in two stores of one plugin and one of another.
	pluginKeys := map[string][]string{
		"plugin-volume/instance/plugin/state":       {"head", "roots/private"},
		"plugin-volume/instance/plugin/other/store": {"head"},
		"plugin-volume/instance/other-plugin/":      {"head"},
	}
	for id, keys := range pluginKeys {
		for _, key := range keys {
			key := append(vol.GetKvKey().GetObjectStorePrefixByID(id), key...)
			if err := putKey(ctx, vol, key, data); err != nil {
				t.Fatal(err)
			}
		}
	}

	// The original World copies are all untracked; only the two raw blocks
	// with child lists are excluded from the stored-block totals.
	var storedCount, storedBytes uint64
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	err = tx.ScanPrefix(ctx, vol.GetKvKey().GetBlockFullPrefix(), func(_, data []byte) error {
		storedCount++
		storedBytes += uint64(len(data))
		return nil
	})
	tx.Discard()
	if err != nil {
		t.Fatal(err)
	}

	// Exclude the two blocks whose child lists exist.
	wantUntracked := storedCount - uint64(len(tracked))
	wantBytes := storedBytes
	for _, data := range trackedData {
		wantBytes -= uint64(len(data))
	}

	// Release the fixture volume before running the offline command.
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	// A dry run counts the missing edges and writes none.
	dry, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("dry run: %+v", dry)
	if dry.spaces != 1 || dry.lacking == 0 || dry.edges == 0 || dry.rebuild.Verified != 0 {
		t.Fatalf("dry run: %+v", dry)
	}
	if dry.absent != 0 || dry.undecodable != 0 || len(dry.untyped) != 0 {
		t.Fatalf("dry run did not walk the whole filesystem: %+v", dry)
	}

	// The inventory measures every block without a child list without
	// decoding it.
	if dry.untracked != wantUntracked || dry.untrackedBytes != wantBytes {
		t.Fatalf("untracked inventory: %d blocks, %d bytes; want %d blocks, %d bytes", dry.untracked, dry.untrackedBytes, wantUntracked, wantBytes)
	}
	if len(dry.untrackedSample) != min(refRepairSample, int(wantUntracked)) {
		t.Fatalf("untracked sample length: %d", len(dry.untrackedSample))
	}
	if !slices.ContainsFunc(dry.untrackedSample, func(blk refRepairStoredBlock) bool {
		return blk.ref.EqualVT(orphan) && blk.size == uint64(len(orphanData))
	}) {
		t.Fatalf("untracked sample lacks %s (%d bytes): %+v", orphan.MarshalString(), len(orphanData), dry.untrackedSample)
	}
	for _, ref := range tracked {
		if slices.ContainsFunc(dry.untrackedSample, func(blk refRepairStoredBlock) bool { return blk.ref.EqualVT(ref) }) {
			t.Fatalf("tracked block %s appears in untracked sample", ref.MarshalString())
		}
	}

	// Prefix groups count keys across store boundaries without sizing trees.
	if len(dry.pluginStores) != 2 || dry.pluginStores["plugin-volume/instance/plugin/"] != 3 || dry.pluginStores["plugin-volume/instance/other-plugin/"] != 1 {
		t.Fatalf("plugin prefix groups: %+v", dry.pluginStores)
	}

	// Render the same table as the command and check its diagnostic rows.
	fields := refRepairFields(path, true, dry)
	expectedFields := [][2]string{
		{"Plugin prefix groups", "2"},
		{"Plugin prefix group", "plugin-volume/instance/plugin/ (3 keys)"},
		{"Stored blocks without a child list", strconv.FormatUint(wantUntracked, 10)},
		{"Their bytes", strconv.FormatUint(wantBytes, 10)},
		{"Listless sample", orphan.MarshalString() + " (" + strconv.Itoa(len(orphanData)) + " bytes)"},
	}
	for _, field := range expectedFields {
		if !slices.Contains(fields, field) {
			t.Fatalf("dry-run table lacks %v", field)
		}
	}

	// Inspect the command's aligned output from the fixture's dry run.
	var report bytes.Buffer
	writeFields(&report, fields)
	t.Logf("dry-run table:\n%s", report.String())
	if !strings.Contains(report.String(), "Plugin prefix group:") || !strings.Contains(report.String(), orphan.MarshalString()) {
		t.Fatal("dry-run table lacks plugin prefix label or sample ref")
	}

	// The dry run left the child lists as they were.
	again, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType)
	if err != nil {
		t.Fatal(err)
	}
	if again.lacking != dry.lacking || again.edges != dry.edges || again.untracked != dry.untracked || again.untrackedBytes != dry.untrackedBytes {
		t.Fatalf("dry run wrote child lists: %+v then %+v", dry, again)
	}

	// Applying writes every missing child list, those of the leaves the dry
	// run only checks for presence too, verifies every walked block's list,
	// and clears the volume's unverified lists mark.
	applied, err := repairVolumeRefs(ctx, le, path, "", false, space_world_optypes.LookupBlockType)
	if err != nil {
		t.Fatal(err)
	}
	if applied.rebuild.Rewritten < dry.lacking || applied.rebuild.Verified != dry.blocks || applied.rebuild.Unverified != 0 || !applied.rebuild.Cleared {
		t.Fatalf("applied rebuild %+v, expected at least %d written of %d verified and the mark cleared", applied.rebuild, dry.lacking, dry.blocks)
	}

	// Mark the lists unverified again, hold the World under a named root as a
	// Space's replay base holds it, and rebuild with every object root typed as raw bytes, a
	// registered type that decodes without refs. The rebuild refuses the
	// shortened root list, which the held World reaches, and keeps the mark;
	// a correct rebuild after it finds every list whole and clears the mark.
	markVol, err := openStoppedVolume(ctx, le, path)
	if err != nil {
		t.Fatal(err)
	}
	writeRecords(t, markVol, func(rec *block_gc.Records) error {
		return rec.SetState(ctx, &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_UNADOPTED, UnverifiedLists: true})
	})
	held := []block.NamedRoot{{Name: space_world.RootReplayBase, Ref: state.GetHeadRef().GetRootRef()}}
	err = markVol.SetBucketRoots(ctx, restoreTestBucket, nil, held)
	if cerr := markVol.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		t.Fatal(err)
	}
	asBytes := func(_ context.Context, typeID string) (blocktype.BlockType, error) {
		return blocktype.New[byteslice.ByteSlice](typeID), nil
	}
	wrong, err := repairVolumeRefs(ctx, le, path, "", false, asBytes)
	if err != nil {
		t.Fatal(err)
	}
	if wrong.rebuild.Dropped == 0 || wrong.rebuild.Rewritten != 0 || wrong.rebuild.Unverified == 0 || wrong.rebuild.Cleared {
		t.Fatalf("rebuild with a wrong root type: %+v", wrong.rebuild)
	}
	right, err := repairVolumeRefs(ctx, le, path, "", false, space_world_optypes.LookupBlockType)
	if err != nil {
		t.Fatal(err)
	}
	if right.rebuild.Rewritten != 0 || right.rebuild.Dropped != 0 || right.rebuild.Verified == 0 || !right.rebuild.Cleared {
		t.Fatalf("rebuild after a wrong root type: %+v", right.rebuild)
	}

	// Nothing is missing afterward.
	after, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType)
	if err != nil {
		t.Fatal(err)
	}
	if after.lacking != 0 || after.edges != 0 || after.blocks != dry.blocks {
		t.Fatalf("after repair: %+v", after)
	}

	// Adoption refuses while a stored child list names a block stored
	// without one: the raw block only a parent's list names.
	refused, err := adoptVolume(ctx, le, path)
	if !errors.Is(err, block_gc.ErrAdoptUnknown) || !slices.ContainsFunc(refused.UnknownSample, listed.EqualVT) {
		t.Fatalf("adopt with a listless child: %+v, %v", refused, err)
	}

	// Give that block its list, then adopt; a second adoption changes nothing.
	fixVol, err := openStoppedVolume(ctx, le, path)
	if err != nil {
		t.Fatal(err)
	}
	writeRecords(t, fixVol, func(rec *block_gc.Records) error {
		return rec.PutChildren(ctx, listed, nil)
	})
	if err := fixVol.Close(); err != nil {
		t.Fatal(err)
	}
	adopted, err := adoptVolume(ctx, le, path)
	if err != nil {
		t.Fatalf("adopt the repaired volume: %v (%+v)", err, adopted)
	}
	if adopted.Already || adopted.Unknown != 0 || adopted.Roots != 1 || adopted.Queued == 0 {
		t.Fatalf("adopt: %+v", adopted)
	}
	if again, err := adoptVolume(ctx, le, path); err != nil || !again.Already {
		t.Fatalf("adopt again: %+v, %v", again, err)
	}

	// The audit reaches the whole World from the root, finds no count that
	// would free a live block, and names the orphan, which has no child list
	// for the collector to free it by.
	audit, err := auditVolume(ctx, le, path)
	if err != nil {
		t.Fatal(err)
	}
	var auditReport bytes.Buffer
	writeFields(&auditReport, auditFields(path, audit))
	t.Logf("audit table:\n%s", auditReport.String())
	if audit.LiveQueued != 0 || audit.Undercount != 0 || uint64(len(audit.Mark.Live)) != dry.blocks {
		t.Fatalf("audit after adoption reached %d of %d blocks: %+v", len(audit.Mark.Live), dry.blocks, audit)
	}
	if audit.Unlisted == 0 || !strings.Contains(auditReport.String(), orphan.MarshalString()) {
		t.Fatalf("audit does not name the orphan %s: %+v", orphan.MarshalString(), audit)
	}
}

// writeRecords runs fn over the liveness records of vol in one committed
// transaction.
func writeRecords(t *testing.T, vol *volume_s4db.Volume, fn func(rec *block_gc.Records) error) {
	t.Helper()
	tx, err := vol.GetKvtxStore().NewTransaction(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := fn(volume_kvtx.OpenRecords(tx)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// putKey writes key and value straight to the volume's database.
func putKey(ctx context.Context, vol volume.Volume, key, value []byte) error {
	// Open a write transaction, discarded unless it commits.
	tx, err := volume_s4db.GetDB(vol).NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()

	// Set the key and commit.
	if err := tx.Set(ctx, key, value); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/util/prng"
	"github.com/pkg/errors"
	entrypoint_state "github.com/s4wave/spacewave/bldr/entrypoint/state"
	space_world "github.com/s4wave/spacewave/core/space/world"
	space_world_optypes "github.com/s4wave/spacewave/core/space/world/optypes"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/byteslice"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/s4db"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/db/world/refwalk"
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

	// Write a World whose blocks the volume stores without their lists.
	root, xfrmConf := buildUntrackedWorld(t, le, vol, nil)

	// Store a replay cursor pointing to the World.
	state := &space_world.InnerState{HeadRef: &bucket.ObjectRef{
		RootRef:       root,
		TransformConf: xfrmConf,
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

	// Hash the file before the read-only phases.
	sum := func() [sha256.Size]byte {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		return sha256.Sum256(data)
	}
	before := sum()

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

	// The raw block with a list naming another raw block without one is a
	// leaked lister: no root reaches it, and adoption would refuse it.
	if dry.roots != 0 || dry.holders.roots != 0 || dry.holders.reached != 0 || dry.holders.leaked != 1 || !slices.ContainsFunc(dry.holders.leakedSample, tracked[0].EqualVT) {
		t.Fatalf("dry-run holders: %+v", dry.holders)
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

	// An audit opens the file read-only too, and a write through a read-only
	// open fails, so the read-only phases left every byte of the file as it
	// was.
	if _, err := auditVolume(ctx, le, path); err != nil {
		t.Fatal(err)
	}
	roVol, err := openStoppedVolume(ctx, le, path, volumeReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	wtx, werr := volume_s4db.GetDB(roVol).NewTransaction(ctx, true)
	if werr == nil {
		wtx.Discard()
	}
	if cerr := roVol.Close(); cerr != nil {
		t.Fatal(cerr)
	}
	if !errors.Is(werr, s4db.ErrReadOnly) {
		t.Fatalf("write transaction on a read-only open: %v, want %v", werr, s4db.ErrReadOnly)
	}
	if sum() != before {
		t.Fatal("a read-only phase changed the volume file")
	}

	// Applying writes every missing child list, leaves' included, verifies
	// every walked block's list, and clears the volume's unverified lists mark.
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
	markVol, err := openStoppedVolume(ctx, le, path, volumeWritable)
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
	if after.lacking != 0 || after.edges != 0 || after.blocks != dry.blocks || after.roots != 1 || after.spaceRoots != 1 {
		t.Fatalf("after repair: %+v", after)
	}

	// Adoption refuses while a stored child list names a block stored
	// without one: the raw block only a parent's list names.
	refused, err := adoptVolume(ctx, le, path)
	if !errors.Is(err, block_gc.ErrAdoptUnknown) || !slices.ContainsFunc(refused.UnknownSample, listed.EqualVT) || refused.Unknown != after.holders.total() {
		t.Fatalf("adopt with a listless child: %+v, %v; predicted %+v", refused, err, after.holders)
	}

	// Give that block its list, then adopt; a second adoption changes nothing.
	fixVol, err := openStoppedVolume(ctx, le, path, volumeWritable)
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

// TestDebugRefRepairWorldRoot checks that the repair of a volume with no
// replay cursor walks the World its entrypoint engine's head root holds, and
// that it neither decodes a root it cannot type nor invents a list: a head
// root name in a bucket that is not its engine's, and a leaked list naming a
// block without one, stay as they were, and the dry run predicts them as the
// holders that refuse the adoption.
func TestDebugRefRepairWorldRoot(t *testing.T) {
	// Open a volume unadopted with its lists unverified.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	path := filepath.Join(t.TempDir(), "volume.s4wave")
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	writeRecords(t, vol, func(rec *block_gc.Records) error {
		return rec.SetState(ctx, &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_UNADOPTED, UnverifiedLists: true})
	})

	// Write the entrypoint World without its lists, held by its engine's head
	// root as a LocalAuthority holds it.
	engineID := entrypoint_state.EngineID("test")
	worldRef, err := entrypoint_state.WorldRef(engineID)
	if err != nil {
		t.Fatal(err)
	}
	root, _ := buildUntrackedWorld(t, le, vol, worldRef.GetTransformConf())
	rootName := world_block.WorldRootName(engineID)
	if err := vol.SetBucketRoots(ctx, engineID, nil, []block.NamedRoot{{Name: rootName, Ref: root}}); err != nil {
		t.Fatal(err)
	}

	// Hold a raw block without a list under the same root name in another
	// bucket, which the rule must not type, and give a raw block no root
	// reaches a list naming another raw block without one.
	refs := make([]*block.BlockRef, 3)
	for i, data := range []string{"held by a root of the wrong bucket", "named by a leaked list", "leaked lister"} {
		if refs[i], _, err = vol.Store.PutBlock(ctx, []byte(data), nil); err != nil {
			t.Fatal(err)
		}
	}
	wrongRoot, leakedChild, lister := refs[0], refs[1], refs[2]
	if err := vol.SetBucketRoots(ctx, "other-bucket", nil, []block.NamedRoot{{Name: rootName, Ref: wrongRoot}}); err != nil {
		t.Fatal(err)
	}
	writeRecords(t, vol, func(rec *block_gc.Records) error {
		return rec.PutChildren(ctx, lister, []*block.BlockRef{leakedChild})
	})
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	// The dry run walks the World from its head root alone, counts the other
	// root unwalked, and predicts the two holders adoption would refuse.
	dry, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("dry run: %+v", dry)
	if dry.spaces != 0 || dry.roots != 2 || dry.typedRoots != 1 || dry.unwalkedRoots["world:"].count != 1 || dry.lacking == 0 || dry.rebuild.Verified != 0 {
		t.Fatalf("dry run: %+v", dry)
	}
	if dry.absent != 0 || dry.undecodable != 0 || len(dry.untyped) != 0 {
		t.Fatalf("dry run did not walk the whole World: %+v", dry)
	}
	wantHolders := func(name string, h refRepairHolders) {
		t.Helper()
		if h.roots != 1 || h.reached != 0 || h.leaked != 1 || !slices.ContainsFunc(h.heldSample, wrongRoot.EqualVT) ||
			!slices.ContainsFunc(h.heldSample, leakedChild.EqualVT) || !slices.ContainsFunc(h.leakedSample, lister.EqualVT) {
			t.Fatalf("%s holders: %+v", name, h)
		}
	}
	wantHolders("dry-run", dry.holders)
	var report bytes.Buffer
	writeFields(&report, refRepairFields(path, true, dry))
	t.Logf("dry-run table:\n%s", report.String())

	// A second dry run finds the same: the first wrote nothing.
	again, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType)
	if err != nil {
		t.Fatal(err)
	}
	if again.lacking != dry.lacking || again.blocks != dry.blocks || again.untracked != dry.untracked {
		t.Fatalf("dry run wrote child lists: %+v then %+v", dry, again)
	}

	// Applying verifies every walked list, refuses none, clears the mark, and
	// leaves the holders the dry run predicted.
	applied, err := repairVolumeRefs(ctx, le, path, "", false, space_world_optypes.LookupBlockType)
	if err != nil {
		t.Fatal(err)
	}
	if applied.rebuild.Rewritten < dry.lacking || applied.rebuild.Verified != dry.blocks || applied.rebuild.Dropped != 0 || applied.rebuild.Unverified != 0 || !applied.rebuild.Cleared {
		t.Fatalf("applied rebuild %+v, expected at least %d written of %d verified and the mark cleared", applied.rebuild, dry.lacking, dry.blocks)
	}
	wantHolders("applied", applied.holders)

	// The repair invented no list: the blocks it could not type have none, and
	// the leaked list names what it named.
	checkVol, err := openStoppedVolume(ctx, le, path, volumeReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	for _, ref := range []*block.BlockRef{wrongRoot, leakedChild} {
		if _, found, err := checkVol.GetChildren(ctx, ref); err != nil || found {
			t.Fatalf("list of %s: found %v, %v", ref.MarshalString(), found, err)
		}
	}
	children, found, err := checkVol.GetChildren(ctx, lister)
	if cerr := checkVol.Close(); err == nil {
		err = cerr
	}
	if err != nil || !found || len(children) != 1 || !children[0].EqualVT(leakedChild) {
		t.Fatalf("leaked list: %v, %v, %v", children, found, err)
	}

	// Nothing walked is missing afterward.
	after, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType)
	if err != nil {
		t.Fatal(err)
	}
	if after.lacking != 0 || after.blocks != dry.blocks {
		t.Fatalf("after repair: %+v", after)
	}
	wantHolders("after", after.holders)

	// Adoption refuses with exactly the holders the dry run predicted.
	refused, err := adoptVolume(ctx, le, path)
	if !errors.Is(err, block_gc.ErrAdoptUnknown) || refused.Unknown != dry.holders.total() ||
		!slices.ContainsFunc(refused.UnknownSample, wrongRoot.EqualVT) || !slices.ContainsFunc(refused.UnknownSample, leakedChild.EqualVT) {
		t.Fatalf("adopt: %+v, %v; predicted %+v", refused, err, dry.holders)
	}

	// The audit reaches the World and the root of the wrong bucket, which it
	// finds stored without a list.
	audit, err := auditVolume(ctx, le, path)
	if err != nil {
		t.Fatal(err)
	}
	if uint64(len(audit.Mark.Live)) != dry.blocks+1 || audit.Mark.Unknown != 1 || !slices.ContainsFunc(audit.Mark.UnknownSample, wrongRoot.EqualVT) {
		t.Fatalf("audit reached %d of %d blocks: %+v", len(audit.Mark.Live), dry.blocks+1, audit.Mark)
	}
}

// buildUntrackedWorld writes a UnixFS World holding a chunked file through a
// storage testbed, copying each block into vol without its child list, as a
// volume written before puts carried child lists holds it. It encodes the
// blocks with xfrmConf, or the testbed's transform when nil, and returns the
// World's root and the transform config.
func buildUntrackedWorld(t *testing.T, le *logrus.Entry, vol *volume_s4db.Volume, xfrmConf *block_transform.Config) (*block.BlockRef, *block_transform.Config) {
	// Start a storage testbed whose blocks are copied into the volume
	// untracked.
	ctx := t.Context()
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

	// Encode with the testbed's transform, or the one given.
	xfrm := base.GetTransformer()
	if xfrmConf == nil {
		xfrmConf = base.GetTransformConf()
	} else {
		w := refwalk.NewWalker(le, nil, nil)
		defer w.Close()
		if xfrm, err = w.Transformer(xfrmConf); err != nil {
			t.Fatal(err)
		}
	}

	// Start the World engine on a cursor over the copying bucket.
	cursor := bucket_lookup.NewCursor(
		ctx,
		htb.Bus,
		le,
		htb.StepFactorySet,
		&untrackedCopyBucket{BucketOps: base.GetBucket(), vol: vol},
		xfrm,
		base.GetRef(),
		base.GetOpArgs(),
		xfrmConf,
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
	return eng.GetRootRef().GetRootRef(), xfrmConf
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

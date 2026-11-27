//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
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
		return rec.SetState(ctx, &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_UNADOPTED, Generation: block_gc.Generation_GENERATION_OPEN})
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
	worldRoot := state.GetHeadRef().GetRootRef()
	outcome := &space_world.ReplayCursorOutcome{World: worldRoot, Payloads: []*block.BlockRef{worldRoot}}
	data, err := (&space_world.ReplayCursor{Base: state, Head: state, Outcomes: []*space_world.ReplayCursorOutcome{outcome}}).MarshalVT()
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

	// A dry run counts the missing edges and writes none, and its histogram
	// counts each stored block once, with every stored byte. The World root,
	// guessed as a payload blob, counts as the World it is also walked as.
	hist := refwalk.NewHistogram()
	dry, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType, refRepairOpts{histogram: hist}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("dry run: %+v", dry)
	if dry.spaces != 1 || dry.lacking == 0 || dry.edges == 0 || dry.rebuild.Rewritten == 0 {
		t.Fatalf("dry run: %+v", dry)
	}
	total := hist.Total()
	var typed, unsure refwalk.HistogramRow
	for _, row := range hist.Rows() {
		typed.Blocks += row.Blocks
		typed.Bytes += row.Bytes
		if row.Unsure {
			unsure.Blocks += row.Blocks
		}
	}
	if total.Blocks == 0 || total.Blocks > dry.blocks || unsure.Blocks != 0 || total.Bytes == 0 || total.Leaves == 0 || typed.Blocks != total.Blocks || typed.Bytes != total.Bytes {
		t.Fatalf("histogram: total %+v, types %+v, walked %d", total, hist.Rows(), dry.blocks)
	}

	// The World root, named as a payload, is read as a guessed blob, is not
	// certainly one, and is verified for nothing.
	if dry.notBlob != 1 || dry.payloadBlobs != 0 || !dry.notBlobSample[0].EqualVT(worldRoot) {
		t.Fatalf("payload root that is not a blob: %+v", dry)
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
	again, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType, refRepairOpts{}, nil)
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

	// Applying writes every missing child list, those of the leaves the dry
	// run only checks for presence too, and verifies every walked block's
	// list, which leaves no reached block on the settle's front.
	applied, err := repairVolumeRefs(ctx, le, path, "", false, space_world_optypes.LookupBlockType, refRepairOpts{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if applied.rebuild.Rewritten < dry.lacking || applied.rebuild.Verified != dry.blocks || applied.rebuild.Front != 0 {
		t.Fatalf("applied rebuild %+v, expected at least %d written of %d verified and an empty front", applied.rebuild, dry.lacking, dry.blocks)
	}

	// Return the lists to unverified, as a format 6 store wrote them, hold
	// the World under a named root as a Space's replay base holds it, and
	// rebuild with every object root typed as raw bytes, a registered type
	// that decodes without refs. The rebuild refuses the shortened root
	// list, which the held World reaches, and leaves it on the front; a
	// correct rebuild after it finds every list whole and empties the front.
	markVol, err := openStoppedVolume(ctx, le, path, volumeWritable)
	if err != nil {
		t.Fatal(err)
	}
	writeRecords(t, markVol, func(rec *block_gc.Records) error {
		return rec.SetState(ctx, &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_UNADOPTED, Generation: block_gc.Generation_GENERATION_OPEN})
	})
	unmarkLists(t, markVol)
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
	wrong, err := repairVolumeRefs(ctx, le, path, "", false, asBytes, refRepairOpts{frontRoots: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if wrong.rebuild.Dropped == 0 || wrong.rebuild.Rewritten != 0 || wrong.rebuild.Front == 0 {
		t.Fatalf("rebuild with a wrong root type: %+v", wrong.rebuild)
	}

	// The walk from the named roots finds the one root and names it as the
	// cause of every front entry, since the root holds the refused World.
	under := wrong.front[frontRest+frontRootOf+restoreTestBucket+" "+space_world.RootReplayBase]
	if !wrong.rooted || wrong.roots != 1 || wrong.rootListed < under.count || len(wrong.front) != 1 ||
		under.count != wrong.rebuild.Front {
		t.Fatalf("front of a rebuild with a wrong root type: %d roots reach %d listed blocks, front %+v", wrong.roots, wrong.rootListed, wrong.front)
	}
	right, err := repairVolumeRefs(ctx, le, path, "", false, space_world_optypes.LookupBlockType, refRepairOpts{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if right.rebuild.Rewritten != 0 || right.rebuild.Dropped != 0 || right.rebuild.Verified == 0 || right.rebuild.Front != 0 {
		t.Fatalf("rebuild after a wrong root type: %+v", right.rebuild)
	}

	// Nothing is missing afterward.
	after, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType, refRepairOpts{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if after.lacking != 0 || after.edges != 0 || after.blocks != dry.blocks {
		t.Fatalf("after repair: %+v", after)
	}

	// A marked list can lack a child its block's bytes name, as Land marks a
	// list from the refs its caller hands it: drop a child from the World
	// root's list.
	rootRef := held[0].Ref
	children := lsChildren(t, path, rootRef)
	if len(children) == 0 {
		t.Fatal("the World root lists no children")
	}
	missing := children[0]
	writeStopped := func(fn func(rec *block_gc.Records) error) {
		t.Helper()
		stopped, err := openStoppedVolume(ctx, le, path, volumeWritable)
		if err != nil {
			t.Fatal(err)
		}
		writeRecords(t, stopped, fn)
		if err := stopped.Close(); err != nil {
			t.Fatal(err)
		}
	}
	writeStopped(func(rec *block_gc.Records) error {
		return rec.PutChildren(ctx, rootRef, children[1:])
	})

	// A dry run counts it and writes nothing.
	dryRe, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType, refRepairOpts{frontRoots: true, scanUntyped: true}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Its scan finds every ref the types name, and only a run that walked
	// from the named roots reports the blocks no root reaches.
	looseRow := func(res refRepairResult) bool {
		return slices.ContainsFunc(refRepairFields(path, true, res), func(f [2]string) bool {
			return f[0] == "Scan: of them no named root reaches"
		})
	}
	unrooted := dryRe
	unrooted.rooted = false
	if !dryRe.scan.on || dryRe.scan.typed == 0 || dryRe.scan.missedRefs != 0 || dryRe.roots != 1 || !looseRow(dryRe) || looseRow(unrooted) {
		t.Fatalf("dry run that scans: %+v, %d roots", dryRe.scan, dryRe.roots)
	}
	re := dryRe.rebuild
	if re.MarkedLacking != 1 || re.MarkedRefused != 0 || re.MarkedRead != re.MarkedMatched+1 || re.Rewritten != 1 ||
		len(re.MarkedLackingSample) != 1 || !re.MarkedLackingSample[0].Ref.EqualVT(rootRef) || !re.MarkedLackingSample[0].Missing.EqualVT(missing) {
		t.Fatalf("dry run over a short marked list: %+v", re)
	}
	if slices.ContainsFunc(lsChildren(t, path, rootRef), missing.EqualVT) {
		t.Fatal("the dry run wrote the list")
	}

	// The World root's list is marked by now, so its visit as a payload,
	// which is not certainly a blob, counts a marked list left uncompared.
	if re.MarkedUndecodable != 1 || len(re.MarkedUndecodableSample) != 1 || !re.MarkedUndecodableSample[0].Ref.EqualVT(worldRoot) {
		t.Fatalf("marked list of a payload root that is not a blob: %+v", re)
	}

	// The repair rewrites the list with the child.
	fixed, err := repairVolumeRefs(ctx, le, path, "", false, space_world_optypes.LookupBlockType, refRepairOpts{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if fixed.rebuild.MarkedLacking != 1 || fixed.rebuild.Rewritten != 1 || fixed.rebuild.Front != 0 ||
		!slices.ContainsFunc(lsChildren(t, path, rootRef), missing.EqualVT) {
		t.Fatalf("repair of a short marked list: %+v", fixed.rebuild)
	}

	// A short marked list can also name a child the decode lacks, as a list
	// a second type of the block marked does: give the root's short list a
	// raw block its content does not name, whose own list is marked.
	extra := tracked[1]
	writeStopped(func(rec *block_gc.Records) error {
		if err := rec.PutChildren(ctx, extra, nil); err != nil {
			return err
		}
		return rec.PutChildren(ctx, rootRef, append(slices.Clone(children[1:]), extra))
	})

	// A dry run counts the disagreement and a rewrite, not a refusal.
	dryMixed, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType, refRepairOpts{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if re := dryMixed.rebuild; re.MarkedLacking != 1 || re.MarkedRefused != 1 || re.Rewritten != 1 || re.Dropped != 0 {
		t.Fatalf("dry run over a short marked list naming another child: %+v", re)
	}

	// The repair writes the union: a refusal would leave the list short and
	// marked, off the front, and the sweep would free the child it lacks.
	mixed, err := repairVolumeRefs(ctx, le, path, "", false, space_world_optypes.LookupBlockType, refRepairOpts{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	union := lsChildren(t, path, rootRef)
	if mixed.rebuild.MarkedRefused != 1 || mixed.rebuild.Rewritten != 1 || mixed.rebuild.Dropped != 0 || mixed.rebuild.Front != 0 ||
		!slices.ContainsFunc(union, missing.EqualVT) || !slices.ContainsFunc(union, extra.EqualVT) {
		t.Fatalf("repair of a short marked list naming another child: %+v", mixed.rebuild)
	}
	writeStopped(func(rec *block_gc.Records) error {
		return rec.PutChildren(ctx, rootRef, children)
	})

	// Adoption refuses while a stored child list names a block stored
	// without one: the raw block only a parent's list names.
	refused, err := adoptVolume(ctx, le, path, nil)
	if !errors.Is(err, block_gc.ErrAdoptUnknown) || !slices.ContainsFunc(refused.UnknownSample, listed.EqualVT) {
		t.Fatalf("adopt with a listless child: %+v, %v", refused, err)
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
	adopted, err := adoptVolume(ctx, le, path, nil)
	if err != nil {
		t.Fatalf("adopt the repaired volume: %v (%+v)", err, adopted)
	}
	if adopted.Already || adopted.Unknown != 0 || adopted.Roots != 1 || adopted.Queued == 0 {
		t.Fatalf("adopt: %+v", adopted)
	}
	if again, err := adoptVolume(ctx, le, path, nil); err != nil || !again.Already {
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

// lsChildren reads the child list of ref in the stopped volume at path.
func lsChildren(t *testing.T, path string, ref *block.BlockRef) []*block.BlockRef {
	t.Helper()
	vol, err := openStoppedVolume(t.Context(), logrus.NewEntry(logrus.New()), path, volumeReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	tx, err := vol.GetKvtxStore().NewTransaction(t.Context(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	children, _, err := volume_kvtx.OpenRecords(tx).GetChildren(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	return children
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

// unmarkLists rewrites every child list of vol without the mark a list
// written from its block's content carries, as a format 6 store wrote them.
// The key is the kvtx Volume's liveness prefix, 2, the child list kind 'c',
// then the block's key.
func unmarkLists(t *testing.T, vol *volume_s4db.Volume) {
	t.Helper()
	type list struct {
		ref      *block.BlockRef
		children []*block.BlockRef
	}
	var lists []list
	tx, err := vol.GetKvtxStore().NewTransaction(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	err = volume_kvtx.OpenRecords(tx).ScanChildren(t.Context(), func(ref *block.BlockRef, children []*block.BlockRef) error {
		lists = append(lists, list{ref, children})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, l := range lists {
		key := append([]byte{2, 'c'}, block_gc.BlockKey(l.ref)...)
		if err := tx.Set(t.Context(), key, block_gc.EncodeChildren(l.children)); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
}

// TestDebugVolumeRefill restores verified bytes while keeping the target's child list intact.
func TestDebugVolumeRefill(t *testing.T) {
	// Give the target a parked block with bytes and four listed blocks without bytes.
	ctx := t.Context()
	dir := t.TempDir()
	target := filepath.Join(dir, "target.s4db")
	from := filepath.Join(dir, "source.s4db")
	input := filepath.Join(dir, "refs.tsv")
	lacks := filepath.Join(dir, "lacks.tsv")
	data := [][]byte{
		[]byte("restore me"), []byte("check my hash"), []byte("missing from source"),
		[]byte("unlisted"), []byte("already stored"), bytes.Repeat([]byte{1}, volume_kvtx.MaxFillBatchBytes+1),
	}
	refs := make([]*block.BlockRef, len(data))
	for i, dat := range data {
		ref, err := block.BuildBlockRef(dat, nil)
		if err != nil {
			t.Fatal(err)
		}
		refs[i] = ref
	}
	list := makeVolumeRefillTarget(t, target, refs, data)

	// Give the older raw source good bytes for an ordinary and an oversized
	// parked fill, a corrupt copy, and good bytes for an unlisted ref.
	makeVolumeRefillSource(t, from, refs, data)
	fromSum := sumVolumeRefillFile(t, from)
	targetSum := sumVolumeRefillFile(t, target)
	lines := []string{"", refs[0].MarshalString() + "\tfront entry\textra", refs[3].MarshalString(),
		" \t ", refs[1].MarshalString(), refs[2].MarshalString(), refs[4].MarshalString(), refs[5].MarshalString()}
	if err := os.WriteFile(input, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
		t.Fatal(err)
	}

	// Exercise the registered help and preview, then compare the target file byte for byte.
	help, err := runVolumeRefillCLI(t, "--help")
	if err != nil || !strings.Contains(help, "--upgrade-format") || !strings.Contains(help, "--lacks-out") {
		t.Fatalf("refill help: %v\n%s", err, help)
	}
	args := []string{"--from", from, "--refs", input, "--lacks-out", lacks, target}
	dry, err := runVolumeRefillCLI(t, append([]string{"--dry-run"}, args...)...)
	if err != nil {
		t.Fatal(err)
	}
	checkVolumeRefillReport(t, dry, []string{"Refs read", "Would skip (already stored)", "Would lack from source",
		"Would reject (hash mismatch)", "Would restore", "Would restore under a hold, parked",
		"Would drop", "Bytes that would be restored"}, len(data[0])+len(data[5]))
	if got := sumVolumeRefillFile(t, target); got != targetSum {
		t.Fatal("dry run changed the target file")
	}

	// Report a malformed ref by its physical line number, including skipped blanks.
	badRefs := filepath.Join(dir, "bad-refs.tsv")
	if err := os.WriteFile(badRefs, []byte("\nnot-a-ref\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = runVolumeRefillCLI(t, "--dry-run", "--from", from, "--refs", badRefs, target)
	if err == nil || !strings.Contains(err.Error(), "refs line 2 has an invalid B58 block ref") {
		t.Fatalf("parse malformed refs file: %v", err)
	}

	// Reuse the dry run's output path and check the real run's counts.
	if err := os.Chmod(lacks, 0o644); err != nil {
		t.Fatal(err)
	}
	real, err := runVolumeRefillCLI(t, args...)
	if err != nil {
		t.Fatal(err)
	}
	checkVolumeRefillReport(t, real, []string{"Refs read", "Already stored", "Source lacks", "Hash mismatch",
		"Restored", "Restored under a hold, parked", "Dropped", "Bytes restored"}, len(data[0])+len(data[5]))
	if got := sumVolumeRefillFile(t, from); got != fromSum {
		t.Fatal("refill changed the source file")
	}

	// Read the exception file as a caller would, in input order and with private permissions.
	out, err := os.ReadFile(lacks)
	if err != nil {
		t.Fatal(err)
	}
	want := refs[3].MarshalString() + "\trecords do not accept a copied fill\n" +
		refs[1].MarshalString() + "\thash mismatch\n" +
		refs[2].MarshalString() + "\tsource lacks block\n"
	if string(out) != want {
		t.Fatalf("lacks output %q, want %q", out, want)
	}
	info, err := os.Stat(lacks)
	if err != nil {
		t.Fatal(err)
	}
	if mode := info.Mode().Perm(); mode != 0o600 {
		t.Fatalf("lacks mode %o, want 600", mode)
	}

	// Check physical presence, restored bytes and the original serialized child list.
	vol, err := openStoppedVolume(ctx, logrus.NewEntry(logrus.New()), target, volumeReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	found, err := vol.Store.GetBlockExistsBatch(ctx, refs)
	if err != nil || !slices.Equal(found, []bool{true, false, false, false, true, true}) {
		t.Fatalf("stored blocks %v: %v", found, err)
	}
	if visible, err := vol.GetBlockExists(ctx, refs[4]); err != nil || visible {
		t.Fatalf("the skipped block should stay parked: visible=%t, err=%v", visible, err)
	}
	got, foundBlock, err := vol.GetBlock(ctx, refs[0])
	if err != nil || !foundBlock || !bytes.Equal(got, data[0]) {
		t.Fatalf("restored block found=%t, size=%d: %v", foundBlock, len(got), err)
	}
	if got := readVolumeRefillList(t, vol, refs[0]); !bytes.Equal(got, list) {
		t.Fatal("refill changed the original child list")
	}

	// Read back the oversized parked fill and the sole root its hold created.
	got, foundBlock, err = vol.GetBlock(ctx, refs[5])
	if err != nil || !foundBlock || !bytes.Equal(got, data[5]) {
		t.Fatalf("parked fill found=%t, size=%d: %v", foundBlock, len(got), err)
	}
	var roots []block.NamedRoot
	err = vol.ListRoots(ctx, "fill:volume-refill/", func(root block.NamedRoot) error {
		roots = append(roots, root)
		return nil
	})
	if err != nil || len(roots) != 1 || !roots[0].Ref.EqualVT(refs[5]) || roots[0].Lease == "" {
		t.Fatalf("parked fill roots: %+v, %v", roots, err)
	}
}

// makeVolumeRefillTarget leaves child lists intact while preparing ordinary and parked fills.
func makeVolumeRefillTarget(t *testing.T, path string, refs []*block.BlockRef, data [][]byte) []byte {
	// Set up a stopped target using the physical store and recorded child lists.
	t.Helper()
	ctx := t.Context()
	vol, err := volume_s4db.NewVolume(ctx, logrus.NewEntry(logrus.New()), &volume_s4db.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	for _, i := range []int{0, 1, 2, 4} {
		if ref, _, err := vol.Store.PutBlock(ctx, data[i], nil); err != nil || !ref.EqualVT(refs[i]) {
			t.Fatalf("store target fixture block %d: %v", i, err)
		}
	}

	// Record a nonempty list for the block the command will restore.
	writeRecords(t, vol, func(rec *block_gc.Records) error {
		if err := rec.SetState(ctx, &block_gc.LivenessState{Generation: block_gc.Generation_GENERATION_OPEN}); err != nil {
			return err
		}
		if err := rec.PutChildren(ctx, refs[0], []*block.BlockRef{refs[2]}); err != nil {
			return err
		}
		for _, i := range []int{1, 2, 4, 5} {
			if err := rec.PutChildren(ctx, refs[i], nil); err != nil {
				return err
			}
		}
		for _, ref := range refs[4:] {
			if err := rec.SetHolders(ctx, ref, 0, 1); err != nil {
				return err
			}
		}
		return nil
	})

	// Park both leaves as an Open generation does. Only the first has bytes.
	for _, ref := range refs[4:] {
		key := block_gc.BlockKey(ref)
		parked := binary.BigEndian.AppendUint64([]byte{2, 'X'}, 1)
		if err := putKey(ctx, vol, append(parked, key...), nil); err != nil {
			t.Fatal(err)
		}
		if err := putKey(ctx, vol, append([]byte{2, 'I'}, key...), nil); err != nil {
			t.Fatal(err)
		}
	}

	// Lose only the bytes, just as a volume whose child lists outlive the blocks does.
	for _, ref := range refs[:3] {
		if err := vol.Store.RmBlock(ctx, ref); err != nil {
			t.Fatal(err)
		}
	}
	list := readVolumeRefillList(t, vol, refs[0])
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}
	return list
}

// makeVolumeRefillSource writes raw block keys in a file with an older volume format.
func makeVolumeRefillSource(t *testing.T, path string, refs []*block.BlockRef, data [][]byte) {
	// Open only the s4db engine so the fixture can put bytes under any key.
	t.Helper()
	ctx := t.Context()
	db, err := s4db.Open(path, s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			t.Error(err)
		}
	}()
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Write the selected bytes and format in one commit, then stop the source.
	keys := kvkey.NewDefaultKVKey()
	for _, i := range []int{0, 1, 3, 5} {
		key, err := refs[i].MarshalKey()
		if err != nil {
			t.Fatal(err)
		}
		dat := data[i]
		if i == 1 {
			dat = []byte("corrupt copy")
		}
		if err := tx.Set(ctx, keys.GetBlockKey(key), dat); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Set(ctx, keys.GetFormatVersionKey(), kvkey.MarshalFormatVersion(kvkey.FormatVersion-1)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}

// readVolumeRefillList returns the serialized child list from the target fixture.
func readVolumeRefillList(t *testing.T, vol *volume_s4db.Volume, ref *block.BlockRef) []byte {
	// Read the child list's physical record, including its encoding marker.
	t.Helper()
	ctx := t.Context()
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	value, found, err := tx.Get(ctx, append([]byte{2, 'c'}, block_gc.BlockKey(ref)...))
	if err != nil || !found {
		t.Fatalf("read child list: found=%t, err=%v", found, err)
	}
	return value
}

// sumVolumeRefillFile returns the fixture file's checksum.
func sumVolumeRefillFile(t *testing.T, path string) [sha256.Size]byte {
	// Include every byte the engine changes on open or close.
	t.Helper()
	dat, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return sha256.Sum256(dat)
}

// runVolumeRefillCLI invokes the real debug command tree and captures its report.
func runVolumeRefillCLI(t *testing.T, args ...string) (string, error) {
	// Use a fresh command tree for each flag parse.
	t.Helper()
	var out bytes.Buffer
	app := &cli.App{Writer: &out, Commands: []*cli.Command{newDebugCommand(nil, &FormatUpgrade{})}}
	err := app.RunContext(t.Context(), append([]string{"spacewave", "debug", "volume-refill"}, args...))
	return out.String(), err
}

// checkVolumeRefillReport checks every row and its count, independently of padding.
func checkVolumeRefillReport(t *testing.T, report string, labels []string, size int) {
	// Decode the sibling command report's label and value columns.
	t.Helper()
	counts := []string{"6", "1", "1", "1", "1", "1", "1", strconv.Itoa(size)}
	lines := strings.Split(strings.TrimSpace(report), "\n")
	if len(lines) != len(labels) {
		t.Fatalf("refill report has %d rows, want %d:\n%s", len(lines), len(labels), report)
	}
	for i, line := range lines {
		label, count, found := strings.Cut(line, ":")
		if !found || label != labels[i] || strings.TrimSpace(count) != counts[i] {
			t.Fatalf("refill row %q, want %s = %s", line, labels[i], counts[i])
		}
	}
}

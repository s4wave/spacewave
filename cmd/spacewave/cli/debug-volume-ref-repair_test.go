//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"maps"
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
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/block/byteslice"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_blockenc "github.com/s4wave/spacewave/db/block/transform/blockenc"
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/s4db"
	kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/util/blockenc"
	"github.com/s4wave/spacewave/db/volume"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/volume/restore"
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

	// Preserve both failure reasons when neither the typed read nor the scan succeeded.
	t.Run("scan fate includes both causes", func(t *testing.T) {
		// Give an unreached ref each failure independently and then together.
		ref, err := block.BuildBlockRef([]byte("failed scan fate"), nil)
		if err != nil {
			t.Fatal(err)
		}
		repair := newRefRepair(le, vol, nil, space_world_optypes.LookupBlockType)
		repair.scanUntyped()
		for _, tc := range []struct {
			why, failed, want string
		}{
			{"", "", "not reached"},
			{"untyped payload", "", "left, untyped payload"},
			{"", "not a message", "left, scanned and not a message"},
			{"untyped payload", "not a message", "left, untyped payload; scanned and not a message"},
		} {
			// Install just the causes this case reports.
			key := ref.MarshalString()
			delete(repair.unread, key)
			delete(repair.res.scan.failed, key)
			if tc.why != "" {
				repair.unread[key] = tc.why
			}
			if tc.failed != "" {
				repair.res.scan.failed[key] = tc.failed
			}

			// Keep the unchanged single-cause and unreached cases exact.
			if got := repair.scanFate(ref, nil, false); got != tc.want {
				t.Fatalf("scan fate = %q, want %q", got, tc.want)
			}
		}
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
	cursor := base.Rebind(ctx, &untrackedCopyBucket{BucketOps: base.GetBucket(), vol: vol}, base.GetRef(), base.GetOpArgs(), nil)
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
	state := &space_world.InnerState{HeadRef: base.RefWithInlineTransform(&bucket.ObjectRef{
		RootRef: eng.GetRootRef().GetRootRef(),
	})}
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
	// only a parent's list references. That block's own references stay
	// unknown.
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

	// The original World copies are all untracked. Only the two raw blocks
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

	// The World root, recorded as a payload, is read as a guessed blob, is
	// not certainly one, and is verified for nothing.
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
	// list, so the repair leaves no reached block on the settle's front.
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
	// list, which the held World reaches, and leaves the list on the front.
	// A correct rebuild afterward finds every list whole and empties the
	// front.
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

	// The walk from the named roots finds the one root and reports the root
	// as the cause of every front entry, since the root holds the refused
	// World.
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

	// A marked list can lack a child its block's bytes reference, as Land
	// marks a list from the refs its caller hands it: drop a child from the
	// World root's list.
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

	// A dry run counts the short marked list and writes nothing.
	dryRe, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType, refRepairOpts{frontRoots: true, scanUntyped: true}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Its scan finds every ref the types identify, and only a run that
	// walked from the named roots reports the blocks no root reaches.
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

	// A short marked list can also reference a child the decode lacks, as a
	// list a second type of the block marked does: give the root's short
	// list a raw block the root's content does not reference, whose own
	// list is marked.
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

	// The verifier needs the scan to read the untyped blocks.
	scanArgs := &debugRefRepairArgs{scanVerify: true}
	if _, err := scanArgs.opts(); err == nil || err.Error() != "--scan-verify requires --scan-untyped" {
		t.Fatalf("scan verify without the scan: %v", err)
	}
	scanArgs.scanUntyped = true

	// Give the peer directory a child whose bytes the Volume never stores.
	peerData, err := (&space_world.ReplayCursorOutcome{Seqno: 1}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	peerChild, err := block.BuildBlockRef(peerData, nil)
	if err != nil {
		t.Fatal(err)
	}
	scanArgs.peerBlocks = t.TempDir()
	scanArgs.frontOut = filepath.Join(t.TempDir(), "front.tsv")
	if err := os.WriteFile(filepath.Join(scanArgs.peerBlocks, peerChild.MarshalString()), peerData, 0o600); err != nil {
		t.Fatal(err)
	}
	scanOpts, err := scanArgs.opts()
	if err != nil {
		t.Fatal(err)
	}

	// Copy an opaque block with a non-empty list onto the front. Use a marked
	// child so the fixture owes only the copied blocks.
	stopped, err := openStoppedVolume(ctx, le, path, volumeWritable)
	if err != nil {
		t.Fatal(err)
	}
	defer stopped.Close()
	opaqueData := []byte("raw bytes outside protobuf")
	if _, ok := refwalk.ScanHits(opaqueData); ok {
		t.Fatal("opaque fixture bytes parsed as a message")
	}
	copied := &block.PutOpts{Refs: []*block.BlockRef{extra}}
	opaque, _, err := stopped.PutBlock(ctx, opaqueData, copied)
	if err != nil {
		t.Fatal(err)
	}

	// Evict a child's bytes, leaving its marked list on the Volume.
	evicted, _, err := stopped.PutBlock(ctx, []byte("evicted scan child"), &block.PutOpts{RefsKnown: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := stopped.RmBlock(ctx, evicted); err != nil {
		t.Fatal(err)
	}

	// Copy a parent whose bytes name the opaque block, the evicted child,
	// the peer child and an unknown hash. Its list names the marked child.
	stranger, err := block.BuildBlockRef([]byte("absent scan child"), nil)
	if err != nil {
		t.Fatal(err)
	}
	parentData, err := (&space_world.HeldRoots{Roots: []*block.BlockRef{opaque, evicted, peerChild}, Prev: stranger}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	parent, _, err := stopped.PutBlock(ctx, parentData, copied)
	if err != nil {
		t.Fatal(err)
	}

	// Point the replay cursor at the payload so the walk reaches it by scan.
	scanCursor, err := (&space_world.ReplayCursor{Base: state, Head: state, Outcomes: []*space_world.ReplayCursorOutcome{
		{World: worldRoot, Payloads: []*block.BlockRef{parent}},
	}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := putKey(ctx, stopped, key, scanCursor); err != nil {
		t.Fatal(err)
	}
	if err := stopped.Close(); err != nil {
		t.Fatal(err)
	}

	// Follow a peer parent to its peer child in a single walk, in both source forms.
	checkRefRepairPeer(t, path, scanArgs.peerBlocks, parent, peerChild, parentData, peerData)

	// A listing path that aliases the Volume must leave its bytes intact.
	scanBefore := sum()
	alias := filepath.Join(filepath.Dir(scanOpts.frontOut), "volume-link")
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	file, err := openRefRepairListing(path, alias)
	if err == nil {
		file.discard()
		t.Fatal("opened a Volume alias for the front listing")
	}
	if sum() != scanBefore {
		t.Fatal("opening the listing changed the volume")
	}

	// The dry scan predicts the opaque block's fate even in measuring mode.
	measuringOpts := scanOpts
	measuringOpts.scanVerify = false
	measuringOpts.frontOut = ""
	measured, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType, measuringOpts, nil)
	if err != nil {
		t.Fatal(err)
	}
	opaqueFate := "left, scanned and not a message, under a transform that does not authenticate"
	if measured.scan.front[opaqueFate] != 1 || measured.scan.front["read by scanning"] != 1 {
		t.Fatalf("measuring scan front: %v", measured.scan.front)
	}

	// The dry scan counts the parent rewrite and the peer child's new list
	// without changing the file.
	scanDry, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType, scanOpts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if scanDry.rebuild.Rewritten != 2 || scanDry.rebuild.Dropped != 0 || sum() != scanBefore {
		t.Fatalf("dry scan verification: %+v", scanDry.rebuild)
	}
	if !maps.Equal(measured.scan.front, scanDry.scan.front) {
		t.Fatalf("dry verification front: %v, want %v", scanDry.scan.front, measured.scan.front)
	}

	// The listing follows front order and gives the opaque block the same
	// cause and fate as the report.
	listing, err := os.ReadFile(scanOpts.frontOut)
	if err != nil {
		t.Fatal(err)
	}
	body, terminated := strings.CutSuffix(string(listing), "\n")
	lines := strings.Split(body, "\n")
	if !terminated || uint64(len(lines)) != scanDry.rebuild.Front {
		t.Fatalf("dry front listing has %d lines, want %d newline-terminated entries", len(lines), scanDry.rebuild.Front)
	}
	opaqueFields := []string{opaque.MarshalString(), frontRestDry, opaqueFate}
	var foundOpaque bool
	for i, line := range lines {
		// Check field boundaries before comparing the ref or its classification.
		fields := strings.Split(line, "\t")
		if len(fields) != 3 {
			t.Fatalf("front listing line %q has %d fields, want 3", line, len(fields))
		}
		if i < len(scanDry.rebuild.FrontSample) && fields[0] != scanDry.rebuild.FrontSample[i].MarshalString() {
			t.Fatalf("front listing ref %s at line %d is out of order", fields[0], i+1)
		}
		foundOpaque = foundOpaque || slices.Equal(fields, opaqueFields)
	}
	if !foundOpaque {
		t.Fatalf("front listing lacks the opaque block and its report classification: %q", listing)
	}

	// The first write creates a private file.
	info, err := os.Stat(scanOpts.frontOut)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("front listing mode %o, want 600", info.Mode().Perm())
	}

	// The report locates the absent hash and samples the unauthenticated reads.
	strangerRow := [2]string{
		"Scan: hashes naming no known block, outside the list, below payload root that is not certainly a blob, field 2.1",
		"1 (" + parent.MarshalString() + ")",
	}
	opaqueSample := "1 (" + opaque.MarshalString() + ")"
	messageSample := "2 (" + parent.MarshalString() + ")"
	scanRows := [][2]string{
		{"Front entries listed", strconv.Itoa(len(lines))},
		strangerRow,
		{"Scan: hashes naming no known block, outside the list", "1"},
		{"Scan: hashes naming blocks with a stored list and no bytes, outside the list", "1 (" + evicted.MarshalString() + ")"},
		{"Scan: hashes naming blocks known only by the peer, outside the list", "1 (" + peerChild.MarshalString() + ")"},
		{"Scan: scanned blocks that are not messages, with a non-empty list", opaqueSample},
		{"Scan: scanned blocks that are not messages, under a transform that does not authenticate", opaqueSample},
		{"Scan: scanned messages under a transform that does not authenticate", messageSample},
	}
	reportRows := make(map[[2]string]struct{})
	for _, row := range refRepairFields(path, true, scanDry) {
		reportRows[row] = struct{}{}
	}
	for _, row := range scanRows {
		if _, ok := reportRows[row]; !ok {
			t.Fatalf("scan report lacks %v", row)
		}
	}

	// The real scan removes the parent's front entry and leaves the opaque
	// block on the front with the predicted cause.
	scanFixed, err := repairVolumeRefs(ctx, le, path, "", false, space_world_optypes.LookupBlockType, scanOpts, nil)
	if err != nil {
		t.Fatal(err)
	}
	if scanFixed.rebuild.Front != 1 || !slices.ContainsFunc(scanFixed.rebuild.FrontSample, opaque.EqualVT) ||
		scanFixed.scan.front[opaqueFate] != 1 {
		t.Fatalf("verification front: %+v, fates %v", scanFixed.rebuild, scanFixed.scan.front)
	}

	// The writing run replaces the earlier listing with the remaining entry.
	listing, err = os.ReadFile(scanOpts.frontOut)
	if err != nil {
		t.Fatal(err)
	}
	if want := opaque.MarshalString() + "\t" + frontRest + "\t" + opaqueFate + "\n"; string(listing) != want {
		t.Fatalf("verification front listing %q, want %q", listing, want)
	}

	// Verify the marked list holds every known child, including the evicted
	// child and the peer child. The unknown hash stays outside the list.
	restored, err := openStoppedVolume(ctx, le, path, volumeWritable)
	if err != nil {
		t.Fatal(err)
	}
	defer restored.Close()
	scannedList, source, err := restored.GetChildList(ctx, parent)
	if err != nil {
		t.Fatal(err)
	}
	if scanFixed.rebuild.Rewritten != 2 || source != block.RefsAsserted || len(scannedList) != 4 ||
		!slices.ContainsFunc(scannedList, opaque.EqualVT) || !slices.ContainsFunc(scannedList, extra.EqualVT) ||
		!slices.ContainsFunc(scannedList, evicted.EqualVT) || !slices.ContainsFunc(scannedList, peerChild.EqualVT) {
		t.Fatalf("scan verification: %+v, list %v, source %v", scanFixed.rebuild, scannedList, source)
	}

	// The scan records the peer child's list while its bytes stay on the peer.
	if children, source, err := restored.GetChildList(ctx, peerChild); err != nil || source != block.RefsAsserted || len(children) != 0 {
		t.Fatalf("peer child list %v from %v, err %v", children, source, err)
	}
	if exists, err := restored.Store.GetBlockExists(ctx, peerChild); err != nil || exists {
		t.Fatalf("peer child's bytes in the Volume: %v, err %v", exists, err)
	}

	// The opaque block's original list remains unverified.
	opaqueList, source, err := restored.GetChildList(ctx, opaque)
	if err != nil {
		t.Fatal(err)
	}
	if source != block.RefsListed || !slices.EqualFunc(opaqueList, copied.Refs, (*block.BlockRef).EqualVT) {
		t.Fatalf("opaque list %v from %v, want the original unverified list", opaqueList, source)
	}
	checkAuthenticatedOpaqueScan(t, le, restored, opaqueData, copied)

	// Give a typed blob the same missing child that the scanned parent names.
	// Both walks must report it only once, alongside the scan's evicted child.
	blobData, err := (&blob.Blob{
		BlobType:   blob.BlobType_BlobType_CHUNKED,
		TotalSize:  1,
		ChunkIndex: &blob.ChunkIndex{Chunks: []*blob.Chunk{{Size: 1, DataRef: stranger}}},
	}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	typedParent, _, err := restored.PutBlock(ctx, blobData, &block.PutOpts{RefsKnown: true, Refs: []*block.BlockRef{stranger}})
	if err != nil {
		t.Fatal(err)
	}
	lacksCursor, err := (&space_world.ReplayCursor{Base: state, Head: state, Outcomes: []*space_world.ReplayCursorOutcome{
		{World: worldRoot, Payloads: []*block.BlockRef{typedParent, parent, typedParent}},
	}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := putKey(ctx, restored, key, lacksCursor); err != nil {
		t.Fatal(err)
	}
	if err := restored.Close(); err != nil {
		t.Fatal(err)
	}

	// Exercise both modes with and without the scan using the same --refs reader.
	for _, dryRun := range []bool{true, false} {
		for _, scan := range []bool{false, true} {
			// Each run publishes a private, complete listing with no duplicate refs.
			opts := scanOpts
			opts.frontOut = ""
			opts.scanUntyped, opts.scanVerify = scan, false
			opts.lacksOut = filepath.Join(t.TempDir(), "lacks.tsv")
			res, err := repairVolumeRefs(ctx, le, path, "", dryRun, space_world_optypes.LookupBlockType, opts, nil)
			if err != nil {
				t.Fatal(err)
			}
			checkRefRepairLacks(t, opts.lacksOut, scan, res, stranger, evicted)
		}
	}

	// Refuse the Volume itself and its hard link before the walk writes anything.
	lacksBefore := sum()
	for _, output := range []string{path, alias} {
		_, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType, refRepairOpts{lacksOut: output}, nil)
		if err == nil || !strings.Contains(err.Error(), "names the volume file") {
			t.Fatalf("lacks output naming Volume %q: %v", output, err)
		}
	}
	if sum() != lacksBefore {
		t.Fatal("refused lacks listing changed the Volume")
	}

	// A failure after opening both listings removes every temporary file.
	failedDir := t.TempDir()
	failedOpts := refRepairOpts{frontOut: filepath.Join(failedDir, "front.tsv"), lacksOut: filepath.Join(failedDir, "lacks.tsv")}
	if _, err := repairVolumeRefs(ctx, le, path, "missing-space", true, space_world_optypes.LookupBlockType, failedOpts, nil); err == nil {
		t.Fatal("repair of an absent Space succeeded")
	}
	entries, err := os.ReadDir(failedDir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("failed run left files: %v, %v", entries, err)
	}

	// Restore the original replay cursor for the adoption and audit.
	restored, err = openStoppedVolume(ctx, le, path, volumeWritable)
	if err != nil {
		t.Fatal(err)
	}
	writeRecords(t, restored, func(rec *block_gc.Records) error {
		return rec.DeleteChildren(ctx, typedParent)
	})
	if err := restored.RmBlock(ctx, typedParent); err != nil {
		t.Fatal(err)
	}
	if err := putKey(ctx, restored, key, data); err != nil {
		t.Fatal(err)
	}
	if err := restored.Close(); err != nil {
		t.Fatal(err)
	}

	// An empty result replaces an earlier listing with an empty private file.
	emptyOpts := refRepairOpts{lacksOut: filepath.Join(t.TempDir(), "lacks.tsv")}
	if err := os.WriteFile(emptyOpts.lacksOut, []byte("old listing"), 0o644); err != nil {
		t.Fatal(err)
	}
	empty, err := repairVolumeRefs(ctx, le, path, "", true, space_world_optypes.LookupBlockType, emptyOpts, nil)
	if err != nil {
		t.Fatal(err)
	}
	checkRefRepairLacks(t, emptyOpts.lacksOut, false, empty, nil, nil)

	// Adoption refuses while a stored child list references a block stored
	// without a child list: the raw block only a parent's list references.
	refused, err := adoptVolume(ctx, le, path, nil)
	if !errors.Is(err, block_gc.ErrAdoptUnknown) || !slices.ContainsFunc(refused.UnknownSample, listed.EqualVT) {
		t.Fatalf("adopt with a listless child: %+v, %v", refused, err)
	}

	// Give that block its list, then adopt. A second adoption changes
	// nothing.
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
	// would free a live block, and reports the orphan, which has no child
	// list for the collector to free it by.
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

	// Release a legacy span without losing a World a current root also holds:
	// on an adopted Volume, where the release logs a down, and on an unadopted
	// one, where the adoption counts the holders after the release.
	dropSpans := func(t *testing.T, unadopted bool) {
		// Store a minimal World and a payload held only by the span.
		ctx := t.Context()
		path := filepath.Join(t.TempDir(), "legacy.s4wave")
		vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { vol.Close() })
		if _, _, _, err := vol.ApplyBucketConfig(ctx, &bucket.Config{Id: restoreTestBucket, Rev: 1}); err != nil {
			t.Fatal(err)
		}

		// Write the fixture with no counts kept, as the upgrade leaves a
		// Volume written before the collector.
		if unadopted {
			writeRecords(t, vol, func(rec *block_gc.Records) error {
				return rec.SetState(ctx, &block_gc.LivenessState{Adoption: block_gc.Adoption_ADOPTION_UNADOPTED, Generation: block_gc.Generation_GENERATION_OPEN})
			})
		}

		// Keep the fixture's bytes and complete child lists in one root commit.
		entry := func(msg block.Block) *block.PutBatchEntry {
			t.Helper()
			data, err := msg.MarshalVT()
			if err != nil {
				t.Fatal(err)
			}

			// Address the bytes and derive their child list from content.
			ref, err := block.BuildBlockRef(data, nil)
			if err != nil {
				t.Fatal(err)
			}
			refs, err := block.ExtractBlockRefs(msg)
			if err != nil {
				t.Fatal(err)
			}
			return &block.PutBatchEntry{Ref: ref, Data: data, Refs: refs, RefsKnown: true}
		}
		live := entry(&world_block.World{LastChangeDisable: true})
		payload := entry(&blob.Blob{RawData: []byte("legacy payload"), TotalSize: uint64(len("legacy payload"))})
		span := entry(&space_world.ReplaySpan{Worlds: []*block.BlockRef{live.Ref}, Payloads: []*block.BlockRef{payload.Ref}})
		roots := []block.NamedRoot{
			{Name: space_world.RootReplayBase, Ref: live.Ref},
			{Name: "private", Ref: live.Ref},
			{Name: legacyReplaySpanRootName, Ref: span.Ref},
		}
		if err := vol.SetBucketRoots(ctx, restoreTestBucket, []*block.PutBatchEntry{live, payload, span}, roots); err != nil {
			t.Fatal(err)
		}

		// A legacy-named root outside the Space bucket must also stay held.
		if err := vol.SetBucketRoots(ctx, "other", nil, []block.NamedRoot{{Name: legacyReplaySpanRootName, Ref: live.Ref}}); err != nil {
			t.Fatal(err)
		}
		state := &space_world.InnerState{HeadRef: &bucket.ObjectRef{BucketId: restoreTestBucket, RootRef: live.Ref}}
		cursor, err := (&space_world.ReplayCursor{Base: state, Head: state}).MarshalVT()
		if err != nil {
			t.Fatal(err)
		}
		conf := kvkey.DefaultConfig()
		key := slices.Concat(conf.GetPrefix(), conf.GetObjectStorePrefix(), []byte("account/so/"+restoreTestSpace+"/ls/space-world-replay/cursor"))
		if err := putKey(ctx, vol, key, cursor); err != nil {
			t.Fatal(err)
		}

		// Model an older Volume whose lists still need the typed walk.
		writeRecords(t, vol, func(rec *block_gc.Records) error {
			state, err := rec.GetState(ctx)
			if err != nil {
				return err
			}
			state.Generation = block_gc.Generation_GENERATION_OPEN
			return rec.SetState(ctx, state)
		})
		unmarkLists(t, vol)
		if err := vol.Close(); err != nil {
			t.Fatal(err)
		}

		// Exercise the real flag parser and require exactly one release row.
		run := func(flags []string, count string) {
			t.Helper()
			var output bytes.Buffer
			app := &cli.App{Writer: &output, Commands: []*cli.Command{newDebugRefRepairCommand(&FormatUpgrade{})}}
			args := append([]string{"spacewave", "ref-repair"}, flags...)
			if err := app.RunContext(ctx, append(args, path)); err != nil {
				t.Fatal(err)
			}

			// Ignore column padding while checking the count and row uniqueness.
			var rows int
			for line := range strings.Lines(output.String()) {
				label, value, ok := strings.Cut(line, ":")
				if ok && strings.TrimSpace(label) == "Legacy replay-span roots released" {
					rows++
					if strings.TrimSpace(value) != count {
						t.Fatalf("release count = %q, want %s", value, count)
					}
				}
			}
			if rows != 1 {
				t.Fatalf("release rows = %d, want 1", rows)
			}
		}
		checkRoots := func(dropped bool) {
			t.Helper()
			vol, err := openStoppedVolume(ctx, le, path, volumeReadOnly)
			if err != nil {
				t.Fatal(err)
			}
			defer vol.Close()

			// Compare every original Space root and the root outside its bucket.
			for _, root := range roots {
				want := root.Ref
				if dropped && root.Name == legacyReplaySpanRootName {
					want = nil
				}
				got, err := vol.GetBucketRoot(ctx, restoreTestBucket, root.Name)
				if err != nil || !block_gc.SameRef(got, want) {
					t.Fatalf("root %s changed unexpectedly: %v", root.Name, err)
				}
			}
			got, err := vol.GetBucketRoot(ctx, "other", legacyReplaySpanRootName)
			if err != nil || !live.Ref.EqualVT(got) {
				t.Fatalf("root outside the Space changed: %v", err)
			}
		}

		// Without the flag even a writing repair keeps the span root.
		run(nil, "0")
		checkRoots(false)
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}

		// A dry run counts the release and leaves every byte unchanged.
		run([]string{"--drop-legacy-spans", "--dry-run"}, "1")
		checkRoots(false)
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatalf("dry run changed the Volume: %v", err)
		}

		// Leave the span's lists unverified again so a drop must skip them.
		vol, err = openStoppedVolume(ctx, le, path, volumeWritable)
		if err != nil {
			t.Fatal(err)
		}
		unmarkLists(t, vol)
		if err := vol.Close(); err != nil {
			t.Fatal(err)
		}
		run([]string{"--drop-legacy-spans"}, "1")
		checkRoots(true)

		// Check the World's exact holders and its raw bytes before and after
		// the collector's sweep. Its three roots hold it throughout, and the
		// span's list holds it until the span parks. Zero holders skips the
		// count, which is exact only once the collector has netted the logs
		// or adopted the Volume.
		checkLive := func(holders uint64) {
			t.Helper()
			vol, err := openStoppedVolume(ctx, le, path, volumeReadOnly)
			if err != nil {
				t.Fatal(err)
			}
			defer vol.Close()

			// The World is the only block reachable from any surviving root.
			_, err = vol.InspectHolders(ctx, []*block.BlockRef{live.Ref}, 1, func(level int, _ *block.BlockRef, row block_gc.HolderRow) error {
				if level == 0 && (holders != 0 && row.Holders != holders || !row.Stored || !row.Listed || row.Queued || row.Parked || row.Released) {
					t.Fatalf("current root's World lost its liveness: %+v", row)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}

			// Read through both the Volume and its raw store to detect hiding or deletion.
			for _, store := range []block.StoreOps{vol, vol.Store} {
				data, found, err := store.GetBlock(ctx, live.Ref)
				if err != nil || !found || !bytes.Equal(data, live.Data) {
					t.Fatalf("current root's World cannot be read: found %v, err %v", found, err)
				}
			}
		}
		checkLive(0)
		checkDiscarded := func(stored bool) {
			t.Helper()
			vol, err := openStoppedVolume(ctx, le, path, volumeReadOnly)
			if err != nil {
				t.Fatal(err)
			}
			defer vol.Close()

			// The drop must leave the discarded lists unverified, and collection deletes their bytes.
			_, err = vol.InspectHolders(ctx, []*block.BlockRef{span.Ref, payload.Ref}, 1, func(level int, _ *block.BlockRef, row block_gc.HolderRow) error {
				if level == 0 && (row.Marked || row.Stored != stored) {
					t.Fatalf("discarded block verified or stored incorrectly: marked %v, stored %v, want stored %v", row.Marked, row.Stored, stored)
				}
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
		}
		checkDiscarded(true)
		rep, err := collectVolume(ctx, le, path, &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"}, 0, volumeLoadOpts{})
		if err != nil {
			t.Fatal(err)
		}
		if res := rep.res; res.Parked != 2 || !res.Flipped {
			t.Fatalf("collector did not park just the span and payload and flip: %+v", res)
		}
		checkLive(3)
		checkDiscarded(true)

		// The flip reports More false, so the helper stops there. A new pass
		// enters Sweeping and deletes the unverified span and its payload.
		rep, err = collectVolume(ctx, le, path, &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"}, 0, volumeLoadOpts{})
		if err != nil {
			t.Fatal(err)
		}
		if res := rep.res; res.Swept != 2 || !res.Settled {
			t.Fatalf("collector did not sweep just the span and payload and settle: %+v", res)
		}
		checkLive(3)
		checkDiscarded(false)
		checkRoots(true)
	}
	t.Run("drop legacy spans", func(t *testing.T) { dropSpans(t, false) })
	t.Run("drop legacy spans unadopted", func(t *testing.T) { dropSpans(t, true) })

	// Use a separate Volume with frees on to exercise an actual park.
	t.Run("parked child", func(t *testing.T) { checkParkedChildScan(t, le) })
}

// checkAuthenticatedOpaqueScan verifies an authenticated opaque block only
// when the Volume has an empty list to preserve.
func checkAuthenticatedOpaqueScan(t *testing.T, le *logrus.Entry, vol *volume_s4db.Volume, raw []byte, copied *block.PutOpts) {
	// Copy the sealed bytes and their unverified list into the fixture.
	t.Helper()
	ctx := t.Context()
	enc, err := transform_blockenc.NewBlockEnc(&transform_blockenc.Config{BlockEnc: blockenc.DefaultBlockEnc, Key: make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	xfrm := block_transform.NewTransformerWithSteps([]block_transform.Step{enc})
	sealed, err := xfrm.EncodeBlock(raw)
	if err != nil {
		t.Fatal(err)
	}
	ref, _, err := vol.PutBlock(ctx, sealed, copied)
	if err != nil {
		t.Fatal(err)
	}

	// Put a second authenticated opaque block onto the front, then remove
	// its list to reproduce a block held without a recorded list.
	sealed, err = xfrm.EncodeBlock([]byte{0})
	if err != nil {
		t.Fatal(err)
	}
	unlisted, _, err := vol.PutBlock(ctx, sealed, copied)
	if err != nil {
		t.Fatal(err)
	}
	writeRecords(t, vol, func(rec *block_gc.Records) error {
		return rec.DeleteChildren(ctx, unlisted)
	})

	// Copy an authenticated opaque block with an empty list onto the front.
	sealed, err = xfrm.EncodeBlock([]byte{0, 0})
	if err != nil {
		t.Fatal(err)
	}
	empty, _, err := vol.PutBlock(ctx, sealed, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Both modes classify the opaque blocks by what their lists prove.
	nodes := []refwalk.Node{
		{Ref: ref, Untyped: "authenticated copied payload", Xfrm: xfrm},
		{Ref: unlisted, Untyped: "authenticated copied payload", Xfrm: xfrm},
		{Ref: empty, Untyped: "authenticated copied payload", Xfrm: xfrm},
	}
	measured := runUntypedScan(t, le, vol, true, nodes...)
	verified := runUntypedScan(t, le, vol, false, nodes...)
	fate := "left, scanned and not a message, without a stored list"
	listedFate := "left, scanned and not a message, with a stored list that names children"
	if measured.scan.front[fate] != 1 || verified.scan.front[fate] != 1 {
		t.Fatalf("unlisted opaque block's front fates: measuring %v, verifying %v", measured.scan.front, verified.scan.front)
	}
	if measured.scan.front[listedFate] != 1 || verified.scan.front[listedFate] != 1 {
		t.Fatalf("listed opaque block's front fates: measuring %v, verifying %v", measured.scan.front, verified.scan.front)
	}
	row := [2]string{"Scan: scanned blocks that are not messages, without a stored list", "1 (" + unlisted.MarshalString() + ")"}
	if !slices.Contains(scanFields(verified.scan, false), row) {
		t.Fatalf("scan report lacks %v", row)
	}

	// The non-empty list stays unverified and keeps its front entry.
	children, source, err := vol.GetChildList(ctx, ref)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(verified.rebuild.FrontSample, ref.EqualVT) ||
		source != block.RefsListed || !slices.EqualFunc(children, copied.Refs, (*block.BlockRef).EqualVT) {
		t.Fatalf("listed opaque block: front %+v, list %v from %v", verified.rebuild, children, source)
	}

	// The unlisted opaque block keeps its front entry and gains no list.
	if _, has, err := vol.GetChildren(ctx, unlisted); err != nil || has {
		t.Fatalf("unlisted opaque block: has list %v, err %v", has, err)
	}
	if !slices.ContainsFunc(verified.rebuild.FrontSample, unlisted.EqualVT) {
		t.Fatalf("unlisted opaque block left the front: %+v", verified.rebuild)
	}

	// Verification marks the empty list and removes its front entry.
	if children, source, err := vol.GetChildList(ctx, empty); err != nil || source != block.RefsAsserted || len(children) != 0 {
		t.Fatalf("empty opaque block list %v from %v, err %v", children, source, err)
	}
	if slices.ContainsFunc(verified.rebuild.FrontSample, empty.EqualVT) {
		t.Fatalf("empty opaque block stayed on the front: %+v", verified.rebuild)
	}
}

// checkParkedChildScan parks the unheld child of a short list and verifies
// that the scanner keeps its hold.
func checkParkedChildScan(t *testing.T, le *logrus.Entry) {
	// Let the test drive frees on an isolated Volume with a short grace.
	t.Helper()
	ctx := t.Context()
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{
		Path:        filepath.Join(t.TempDir(), "parked.s4wave"),
		StoreConfig: &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"},
	})
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()

	// Store the child in the same batch that gives it a root.
	childData := []byte("parked scan child")
	child, err := block.BuildBlockRef(childData, nil)
	if err != nil {
		t.Fatal(err)
	}
	childEntries := []*block.PutBatchEntry{{Ref: child, Data: childData, RefsKnown: true}}
	childRoots := []block.NamedRoot{{Name: "child", Ref: child}}
	if err := vol.SetBucketRoots(ctx, "scan", childEntries, childRoots); err != nil {
		t.Fatal(err)
	}

	// Root the parent's copied list to open the generation and keep the
	// front owed. Release the child's only holder in the same batch.
	parentData, err := (&space_world.HeldRoots{Roots: []*block.BlockRef{child}}).MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	parent, err := block.BuildBlockRef(parentData, nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := []*block.PutBatchEntry{{Ref: parent, Data: parentData}}
	roots := []block.NamedRoot{{Name: "parent", Ref: parent}, {Name: "child", Previous: child}}
	if err := vol.SetBucketRoots(ctx, "scan", entries, roots); err != nil {
		t.Fatal(err)
	}

	// The collector parks the child and the writer's existence check hides it.
	res, err := vol.Collect(ctx)
	if err != nil || res.Parked != 1 || res.Flipped {
		t.Fatalf("collect before scan: %+v, err %v", res, err)
	}
	if exists, err := vol.GetBlockExists(ctx, child); err != nil || exists {
		t.Fatalf("writer sees parked child: %v, err %v", exists, err)
	}
	if exists, err := vol.Store.GetBlockExists(ctx, child); err != nil || !exists {
		t.Fatalf("raw store sees parked child: %v, err %v", exists, err)
	}

	// The scan adds the parked child to the parent's list using the raw bytes.
	verified := runUntypedScan(t, le, vol, false, refwalk.Node{Ref: parent, Untyped: "copied parent"})
	children, source, err := vol.GetChildList(ctx, parent)
	if err != nil {
		t.Fatal(err)
	}
	if source != block.RefsAsserted || len(children) != 1 || !child.EqualVT(children[0]) ||
		verified.rebuild.Rewritten != 1 || verified.scan.knownByList.count != 0 {
		t.Fatalf("parked child's parent: list %v from %v, rebuild %+v", children, source, verified.rebuild)
	}
}

// runUntypedScan runs the repair's walker and front report for explicit
// untyped roots in the fixture's Volume.
func runUntypedScan(t *testing.T, le *logrus.Entry, vol *volume_s4db.Volume, dryRun bool, nodes ...refwalk.Node) refRepairResult {
	// Open the measuring or verifying rebuild on the same walker.
	t.Helper()
	ctx := t.Context()
	repair := newRefRepair(le, vol, nil, space_world_optypes.LookupBlockType)
	defer repair.close()
	repair.scanUntyped()
	repair.scanVerify = !dryRun
	switch {
	case dryRun:
		repair.rebuild = vol.OpenListCheck()
	default:
		var err error
		repair.rebuild, err = vol.OpenListRebuild(ctx)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Walk each root, finish the rebuild and report the front's fates.
	for _, node := range nodes {
		if err := repair.walker.Walk(ctx, node, repair.walkVisit); err != nil {
			t.Fatal(err)
		}
	}
	var err error
	repair.res.rebuild, err = repair.rebuild.Finish(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := repair.measureFront(ctx, dryRun, false, nil); err != nil {
		t.Fatal(err)
	}
	return repair.res
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

// checkRefRepairLacks checks the private TSV and the shared --refs contract.
func checkRefRepairLacks(t *testing.T, path string, scan bool, res refRepairResult, typed, scanned *block.BlockRef) {
	// Require private output even when the result is empty or replaces a file.
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("lacks listing mode %o, want 600", info.Mode().Perm())
	}

	// Compare every row's identity and how it was reached without relying on order.
	want := make(map[string]string)
	if typed != nil {
		want[typed.MarshalString()] = "*byteslice.ByteSlice"
	}
	if scan && scanned != nil {
		want[scanned.MarshalString()] = "payload root that is not certainly a blob"
	}
	listing, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	got := make(map[string]string)
	for line := range strings.Lines(string(listing)) {
		fields := strings.Split(strings.TrimSuffix(line, "\n"), "\t")
		if len(fields) != 2 || fields[1] == "" || strings.ContainsAny(fields[1], "\r\n") {
			t.Fatalf("invalid lacks row %q", line)
		}
		if _, duplicate := got[fields[0]]; duplicate {
			t.Fatalf("duplicate lacks ref %s", fields[0])
		}
		got[fields[0]] = fields[1]
	}
	if !maps.Equal(got, want) || !res.lacksListing || res.lacksListed != uint64(len(want)) {
		t.Fatalf("lacks listing %v, count %d, enabled %v, want %v", got, res.lacksListed, res.lacksListing, want)
	}

	// The report places the listing count directly after Blocks absent.
	fields := refRepairFields(path, true, res)
	at := slices.IndexFunc(fields, func(row [2]string) bool { return row[0] == "Blocks absent" })
	if fields[at+1] != [2]string{"Lacks listed", strconv.Itoa(len(want))} {
		t.Fatalf("lacks report row %v", fields[at+1])
	}

	// Parse the same rows with the refill and export input reader.
	parsed := make(map[string]bool)
	if err := restore.StreamRefs(t.Context(), bytes.NewReader(listing), 1, func(refs []*block.BlockRef) error {
		for _, ref := range refs {
			parsed[ref.MarshalString()] = true
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(parsed) != len(want) {
		t.Fatalf("--refs parsed %v, want %v", parsed, want)
	}
	for ref := range want {
		if !parsed[ref] {
			t.Fatalf("--refs missed %s", ref)
		}
	}
}

// checkRefRepairPeer follows two absent levels and protects all listing inputs.
func checkRefRepairPeer(t *testing.T, path, directory string, parent, child *block.BlockRef, parentData, childData []byte) {
	// Check that help describes both peer flags and source forms.
	t.Helper()
	ctx := t.Context()
	help, err := runVolumeBlockCLI(t, "ref-repair", "--help")
	if err != nil || !strings.Contains(help, "--peer-out") || !strings.Contains(help, "or a stopped Volume file") {
		t.Fatalf("ref repair peer help %q: %v", help, err)
	}

	// Open the stopped target and evict the parent's bytes.
	le := logrus.NewEntry(logrus.New())
	vol, err := openStoppedVolume(ctx, le, path, volumeWritable)
	if err != nil {
		t.Fatal(err)
	}
	if err := vol.RmBlock(ctx, parent); err != nil {
		t.Fatal(err)
	}
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	// Give both source forms the missing parent and child.
	if err := os.WriteFile(filepath.Join(directory, parent.MarshalString()), parentData, 0o600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "peer.s4db")
	makeVolumeBlockSource(t, source, []*block.BlockRef{parent, child}, [][]byte{parentData, childData})
	targetData, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}

	// Exercise flag wiring, traversal order, private permissions and the report in both modes.
	for _, peer := range []string{directory, source} {
		for _, dryRun := range []bool{true, false} {
			// Use an independent target so a writing run cannot change later expectations.
			target := filepath.Join(t.TempDir(), "target.s4db")
			if err := os.WriteFile(target, targetData, 0o600); err != nil {
				t.Fatal(err)
			}
			listing := filepath.Join(t.TempDir(), "peer.refs")
			args := []string{"--peer-blocks", peer, "--peer-out", listing, "--scan-untyped"}
			if dryRun {
				args = append(args, "--dry-run")
			}
			report, err := runVolumeBlockCLI(t, "ref-repair", append(args, target)...)
			if err != nil {
				t.Fatal(err)
			}

			// The file is directly usable as --refs and includes each served block once.
			got, err := os.ReadFile(listing)
			want := parent.MarshalString() + "\n" + child.MarshalString() + "\n"
			if err != nil || string(got) != want {
				t.Fatalf("peer %s, dry %v: listing %q, want %q: %v", peer, dryRun, got, want, err)
			}
			info, err := os.Stat(listing)
			if err != nil || info.Mode().Perm() != 0o600 {
				t.Fatalf("peer listing permissions: %v, %v", info, err)
			}
			if !slices.ContainsFunc(strings.Split(report, "\n"), func(line string) bool {
				return strings.Join(strings.Fields(line), " ") == "Peer blocks listed: 2"
			}) {
				t.Fatalf("peer listing report: %s", report)
			}
		}
	}

	// Reject direct paths and file aliases before opening either input for writing.
	for _, input := range []string{path, source} {
		before := sumVolumeRefillFile(t, input)
		alias := filepath.Join(t.TempDir(), "alias")
		if err := os.Link(input, alias); err != nil {
			t.Fatal(err)
		}
		symlink := filepath.Join(t.TempDir(), "symlink")
		if err := os.Symlink(input, symlink); err != nil {
			t.Fatal(err)
		}
		for _, output := range []string{input, alias, symlink} {
			if _, err := runVolumeBlockCLI(t, "ref-repair", "--peer-blocks", source, "--peer-out", output, path); err == nil {
				t.Fatalf("accepted peer listing alias %s", output)
			}

			// Reject the same aliases when used as a peer of the target itself.
			if input == path {
				if _, err := runVolumeBlockCLI(t, "ref-repair", "--peer-blocks", output, path); err == nil {
					t.Fatalf("accepted target as peer %s", output)
				}
			}
			if sumVolumeRefillFile(t, input) != before {
				t.Fatalf("refused peer or listing changed %s", input)
			}
		}
	}

	// Restore the parent's bytes so the existing scan run-through keeps its fixture.
	vol, err = openStoppedVolume(ctx, le, path, volumeWritable)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	if _, _, err := vol.Store.PutBlock(ctx, parentData, &block.PutOpts{ForceBlockRef: parent}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(directory, parent.MarshalString())); err != nil {
		t.Fatal(err)
	}
}

//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"slices"
	"strings"

	"github.com/pkg/errors"
	entrypoint_state "github.com/s4wave/spacewave/bldr/entrypoint/state"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/s4wave/spacewave/db/world/refwalk"
	"github.com/sirupsen/logrus"
)

const (
	// refRepairProgress is the number of blocks walked between progress logs.
	refRepairProgress = 100_000
	// refRepairSample limits the listless blocks printed in the report.
	refRepairSample = 10
)

// refRepairStoredBlock describes a listless block without decoding its type.
type refRepairStoredBlock struct {
	// ref addresses the stored block.
	ref *block.BlockRef
	// size is the number of stored bytes.
	size uint64
}

// refRepairResult counts what one ref repair found and wrote.
type refRepairResult struct {
	// spaces is the number of Spaces walked.
	spaces int
	// blocks is the number of distinct blocks walked.
	blocks uint64
	// absent is the number of referenced blocks the volume lacks.
	absent uint64
	// undecodable is the number of blocks that failed to decode as their type.
	// A block whose type holds no refs is only checked for presence.
	undecodable uint64
	// lacking is the number of walked blocks stored without a child list.
	lacking uint64
	// edges is the number of children those lists name.
	edges uint64
	// rebuild is what the list rebuild verified and wrote, empty on a dry run
	// and on a volume whose lists were verified already.
	rebuild block_gc.ListRebuildResult
	// verified reports that the volume's lists were verified before the run,
	// so it rebuilt nothing.
	verified bool
	// untyped counts the refs whose type the walk cannot name, by reason, with
	// a sample of each. Their edges are recorded; the blocks below them are
	// not walked, so their lists stay unverified until their type is known.
	untyped map[string]refwalk.Untyped
	// pluginStores counts object-store keys by plugin-volume/<instance>/<plugin>/ prefix.
	// The prefix groups stores whose boundaries are not encoded in their keys.
	pluginStores map[string]uint64
	// untracked counts the stored blocks without a child list.
	untracked uint64
	// untrackedBytes is the total stored bytes of those blocks.
	untrackedBytes uint64
	// untrackedSample holds up to refRepairSample of them in key order.
	untrackedSample []refRepairStoredBlock
	// roots counts the named roots of the volume, zero with --space.
	roots uint64
	// spaceRoots counts those in the bucket of a Space the run walked from its
	// replay cursor, which types them.
	spaceRoots uint64
	// typedRoots counts those outside them that refRepair.rootNode types and
	// the run walked.
	typedRoots uint64
	// unwalkedRoots counts the other named roots by refRepairRootKind, with a
	// sample of each. The run decodes none of them.
	unwalkedRoots map[string]refRepairRoots
	// holders splits the holders an adoption counts of the blocks the run
	// leaves stored without a child list.
	holders refRepairHolders
}

// refRepairRoots counts the named roots of one kind.
type refRepairRoots struct {
	// count is the number of roots.
	count uint64
	// sample is the bucket and name of the first.
	sample string
}

// refRepairHolders counts the holders an adoption counts of the blocks stored
// without a child list after the run, each of which refuses the adoption. On a
// dry run it predicts them: a block the walk decoded or found stored as a leaf
// gets its list from a real run, and every other block keeps the list it has.
type refRepairHolders struct {
	// roots counts the named roots that hold such a block.
	roots uint64
	// reached counts the entries naming one in a child list a named root
	// reaches through lists after the run.
	reached uint64
	// leaked counts the entries naming one in a child list no named root
	// reaches: a leaked lister.
	leaked uint64
	// heldSample holds up to refRepairSample of the held blocks.
	heldSample []*block.BlockRef
	// leakedSample holds up to refRepairSample of the leaked listers.
	leakedSample []*block.BlockRef
}

// total returns the holders an adoption counts as unknown.
func (h refRepairHolders) total() uint64 {
	return h.roots + h.reached + h.leaked
}

// refRepair walks the World graphs the Spaces of a stopped volume retain, and
// those its typed named roots hold, with a refwalk.Walker, and hands each
// decoded block's refs to a list rebuild, which writes the list when it is
// missing or short and marks it verified. A block's list is the refs
// block.ExtractBlockRefs reads from the decoded block, which are the refs a
// write records for it.
//
// It keeps these invariants:
//   - It only writes child lists and verified marks, through the
//     block_gc.ListRebuild, and deletes nothing. A dry run opens no rebuild
//     and writes nothing.
//   - A list comes only from the bytes of a block decoded as the type its
//     root names: a Space's roots typed by its replay cursor, which holds the
//     Space's transform, or a named root rootNode types by its writer's rule.
//   - It types a named root only by that rule, so a World head root is
//     walked only in its own engine's bucket, with the transform that engine
//     writes. Every other named root is counted and never decoded, so the
//     blocks below it keep the lists they have; the run keeps what it cannot
//     type rather than guess.
//   - The walk streams the named roots in slices and holds no read snapshot
//     while it decodes or writes.
type refRepair struct {
	// le is the logger.
	le *logrus.Entry
	// vol is the stopped volume.
	vol *volume_s4db.Volume
	// buckets lists the volume's bucket configs.
	buckets bucket_store.Store
	// walker walks the blocks below the Worlds.
	walker *refwalk.Walker
	// rebuild verifies the decoded blocks' lists, nil on a dry run.
	rebuild *block_gc.ListRebuild
	// spaceBuckets holds the buckets of the Spaces walked from their replay
	// cursors.
	spaceBuckets map[string]struct{}
	// undecodable holds the walked blocks that failed to decode, which get no
	// list, by ref string.
	undecodable map[string]struct{}
	// res counts the findings.
	res refRepairResult
}

// newRefRepair constructs a ref repair over a stopped volume, typing objects'
// roots with lookup.
func newRefRepair(le *logrus.Entry, vol *volume_s4db.Volume, lookup refwalk.LookupBlockType) *refRepair {
	return &refRepair{
		le:           le,
		vol:          vol,
		buckets:      vol,
		walker:       refwalk.NewWalker(le, vol, lookup),
		spaceBuckets: make(map[string]struct{}),
		undecodable:  make(map[string]struct{}),
		res:          refRepairResult{unwalkedRoots: make(map[string]refRepairRoots)},
	}
}

// walkNamedRoots streams every named root of the volume and walks each that
// rootNode types, outside the buckets of the walked Spaces, whose replay
// cursors typed their roots. It counts the rest by kind and decodes none of
// them.
func (r *refRepair) walkNamedRoots(ctx context.Context) error {
	return r.vol.EachRoot(ctx, func(bucketID, name string, ref *block.BlockRef) error {
		// Leave a walked Space's roots to its cursor.
		r.res.roots++
		if _, ok := r.spaceBuckets[bucketID]; ok {
			r.res.spaceRoots++
			return nil
		}

		// Walk a typed root, and count the others.
		node, ok, err := r.rootNode(bucketID, name, ref)
		if err != nil {
			return err
		}
		if !ok {
			kind := refRepairRootKind(name)
			roots := r.res.unwalkedRoots[kind]
			if roots.count == 0 {
				roots.sample = bucketID + " " + name
			}
			roots.count++
			r.res.unwalkedRoots[kind] = roots
			return nil
		}
		r.res.typedRoots++
		r.le.Infof("walking the World root %s of bucket %s", name, bucketID)
		return r.walker.Walk(ctx, node, r.visit)
	})
}

// rootNode types the named root name of bucketID, holding ref, by the rule of
// its writer, and reports whether it could. The one rule is the World head
// root a LocalAuthority installs for an entrypoint World engine: its name is
// world_block.WorldRootName of the engine, its bucket is the engine's own,
// and entrypoint_state.WorldRef gives the World's transform. A Space's roots
// need the Space's transform, which only its replay cursor holds, and the
// other writers' roots name no type, so it types no other root.
func (r *refRepair) rootNode(bucketID, name string, ref *block.BlockRef) (refwalk.Node, bool, error) {
	// Match the World head root of an entrypoint engine in its own bucket.
	if name != world_block.WorldRootName(bucketID) {
		return refwalk.Node{}, false, nil
	}
	world, err := entrypoint_state.WorldRef(bucketID)
	if err != nil || world == nil {
		return refwalk.Node{}, false, err
	}

	// Type it as that World.
	node, err := r.walker.WorldNode(world, ref)
	return node, err == nil, err
}

// refRepairRootKind returns the kind of a root name for the report: the name
// up to and including its first ':' or '/', or the whole name.
func refRepairRootKind(name string) string {
	if i := strings.IndexAny(name, ":/"); i >= 0 {
		return name[:i+1]
	}
	return name
}

// splitHolders counts the holders an adoption would count of the blocks the
// run leaves stored without a child list, by kind, as refRepairHolders
// describes. It reads one snapshot and walks the volume's lists from its
// named roots in memory, one key per reached block, as the audit does, then
// streams every root and child list past it. After a real run it measures;
// on a dry run, predict is set and it predicts.
func (r *refRepair) splitHolders(ctx context.Context, predict bool) error {
	// Read one snapshot, and walk it from the roots through lists.
	tx, err := r.vol.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	rec := volume_kvtx.OpenRecords(tx)
	mark, err := block_gc.Reachable(ctx, rec, r.vol.GetBlockExists)
	if err != nil {
		return err
	}

	// left reports whether ref stays stored without a list after the run.
	listedByRun := func(ref *block.BlockRef) bool {
		_, bad := r.undecodable[ref.MarshalString()]
		return predict && r.walker.Visited(ref) && !bad
	}
	left := func(ref *block.BlockRef) (bool, error) {
		listed, err := rec.HasChildren(ctx, ref)
		if err != nil || listed || listedByRun(ref) {
			return false, err
		}
		return r.vol.GetBlockExists(ctx, ref)
	}

	// Count the roots holding one.
	h := &r.res.holders
	err = rec.ScanRoots(ctx, func(_, _ string, entry *block_gc.RootEntry) error {
		// Count and sample a root whose block stays listless.
		ref, _, err := block_gc.ParseBlockKey(entry.GetRef())
		if err != nil {
			return err
		}
		held, err := left(ref)
		if err != nil || !held {
			return err
		}
		h.roots++
		h.heldSample = sampleRef(h.heldSample, ref)
		return nil
	})
	if err != nil {
		return err
	}

	// Count the list entries naming one, split by whether a root reaches the
	// lister through lists, or will once the run writes the walked lists.
	return rec.ScanChildren(ctx, func(lister *block.BlockRef, children []*block.BlockRef) error {
		for _, child := range children {
			held, err := left(child)
			if err != nil {
				return err
			}
			if !held {
				continue
			}
			h.heldSample = sampleRef(h.heldSample, child)
			if mark.Has(lister) || listedByRun(lister) {
				h.reached++
				continue
			}
			h.leaked++
			h.leakedSample = sampleRef(h.leakedSample, lister)
		}
		return nil
	})
}

// sampleRef appends ref to s unless s holds refRepairSample refs or ref
// already.
func sampleRef(s []*block.BlockRef, ref *block.BlockRef) []*block.BlockRef {
	if len(s) == refRepairSample || slices.ContainsFunc(s, ref.EqualVT) {
		return s
	}
	return append(s, ref)
}

// measurePluginStores counts object-store keys under each plugin's volume prefix.
// It reads only keys; the values are root pointers whose trees are not measured.
func (r *refRepair) measurePluginStores(ctx context.Context, vol *volume_s4db.Volume) error {
	// Scan the plugin object-store namespace in one read transaction.
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	prefix := vol.GetKvKey().GetObjectStorePrefixByID("plugin-volume")
	r.res.pluginStores = make(map[string]uint64)

	// Group by the escaped instance and plugin components, not store boundaries.
	return tx.ScanPrefixKeys(ctx, prefix, func(key []byte) error {
		// Count only keys with both escaped plugin-volume components.
		instance, rest, ok := strings.Cut(string(key[len(prefix):]), "/")
		if !ok {
			return nil
		}
		plugin, _, ok := strings.Cut(rest, "/")
		if ok {
			r.res.pluginStores["plugin-volume/"+instance+"/"+plugin+"/"]++
		}
		return nil
	})
}

// measureUntracked scans every stored block, including blocks outside the typed
// walk, and measures those stored without a child list.
func (r *refRepair) measureUntracked(ctx context.Context, vol *volume_s4db.Volume) error {
	// Iterate block keys and their child lists in one snapshot.
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	rec := volume_kvtx.OpenRecords(tx)
	prefix := vol.GetKvKey().GetBlockFullPrefix()
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()

	// Count each block without a list.
	var scanned uint64
	for it.Next() {
		ref := &block.BlockRef{}
		if err := ref.UnmarshalVT(bytes.Clone(it.Key()[len(prefix):])); err != nil {
			return errors.Wrap(err, "decode stored block key")
		}
		listed, err := rec.HasChildren(ctx, ref)
		if err != nil {
			return err
		}
		scanned++
		if scanned%refRepairProgress == 0 {
			r.le.Infof("scanned %d stored blocks, %d without a child list", scanned, r.res.untracked)
		}
		if listed {
			continue
		}
		value, err := it.Value()
		if err != nil {
			return err
		}
		blk := refRepairStoredBlock{ref: ref, size: uint64(len(value))}
		r.res.untracked++
		r.res.untrackedBytes += blk.size
		if len(r.res.untrackedSample) < refRepairSample {
			r.res.untrackedSample = append(r.res.untrackedSample, blk)
		}
	}
	return it.Err()
}

// walkSpace walks the roots of one Space's replay: the replay span its bucket
// holds, the base and head Worlds of the cursor with their retained roots, and
// the World and payloads of each outcome. A root nothing else holds is then given to the Space's bucket.
func (r *refRepair) walkSpace(ctx context.Context, sc spaceReplayCursor) error {
	// Find the bucket that owns the Space's blocks.
	bucketID, err := findSpaceBucket(ctx, r.buckets, sc.spaceID)
	if err != nil {
		return err
	}
	spaceWorld := sc.world()
	r.res.spaces++
	r.spaceBuckets[bucketID] = struct{}{}

	// Collect the Worlds and payloads of the replay span the bucket holds as
	// a named root.
	var roots []refwalk.Node
	span, err := r.vol.GetBucketRoot(ctx, bucketID, legacyReplaySpanRootName)
	if err != nil {
		return err
	}
	spanRoots, err := r.spanRoots(ctx, spaceWorld, span)
	if err != nil {
		return err
	}
	roots = append(roots, spanRoots...)

	// Collect the cursor's Worlds with their retained roots, and the World and
	// payloads of each outcome.
	for _, state := range []*space_world.InnerState{sc.cursor.GetBase(), sc.cursor.GetHead()} {
		head := state.GetHeadRef()
		if head == nil {
			continue
		}
		refs := []*block.BlockRef{head.GetRootRef()}
		for _, retained := range state.GetRetainedRoots() {
			refs = append(refs, retained.GetRootRef())
		}
		for _, ref := range refs {
			node, err := r.walker.WorldNode(head, ref)
			if err != nil {
				return err
			}
			roots = append(roots, node)
		}
	}
	for _, outcome := range sc.cursor.GetOutcomes() {
		node, err := r.walker.WorldNode(spaceWorld, outcome.GetWorld())
		if err != nil {
			return err
		}
		roots = append(roots, node)
		for _, payload := range outcome.GetPayloads() {
			node, err := r.walker.BlobNode(spaceWorld, payload)
			if err != nil {
				return err
			}
			roots = append(roots, node)
		}
	}

	// Walk each root.
	r.le.Infof("walking %d roots of space %s in bucket %s", len(roots), sc.spaceID, bucketID)
	for _, root := range roots {
		if err := r.walker.Walk(ctx, root, r.visit); err != nil {
			return err
		}
	}
	return nil
}

// legacyReplaySpanRootName names the root a Space engine before the adapter
// held its replay span under.
const legacyReplaySpanRootName = "replay-span"

// spanRoots reads the replay span at ref, a raw protobuf block, verifies its
// list, and returns its Worlds and payload blobs, which the Space's World
// transform encodes. An empty ref has none.
func (r *refRepair) spanRoots(ctx context.Context, spaceWorld *bucket.ObjectRef, ref *block.BlockRef) ([]refwalk.Node, error) {
	// Read and decode the span.
	if ref.GetEmpty() {
		return nil, nil
	}
	data, found, err := r.vol.GetBlock(ctx, ref)
	if err != nil {
		return nil, err
	}
	if !found {
		r.res.absent++
		return nil, nil
	}
	span := &space_world.ReplaySpan{}
	if err := span.UnmarshalVT(data); err != nil {
		r.le.WithError(err).Warnf("cannot decode replay span %s", ref.MarshalString())
		r.res.undecodable++
		r.undecodable[ref.MarshalString()] = struct{}{}
		return nil, nil
	}

	// Count and verify the span's list.
	refs := slices.Concat(span.GetWorlds(), span.GetPayloads())
	if err := r.visitDecoded(ctx, ref, refs); err != nil {
		return nil, err
	}

	// Return each World and payload blob of the span.
	out := make([]refwalk.Node, 0, len(refs))
	for _, ref := range span.GetWorlds() {
		node, err := r.walker.WorldNode(spaceWorld, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	for _, ref := range span.GetPayloads() {
		node, err := r.walker.BlobNode(spaceWorld, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	return out, nil
}

// visit counts a batch of reads and verifies each decoded block's list.
func (r *refRepair) visit(ctx context.Context, batch []refwalk.Visit) error {
	for _, rd := range batch {
		// Count a block the volume lacks, a leaf, and an undecodable block.
		if !rd.Found {
			r.res.absent++
			continue
		}
		if rd.DecodeErr != nil {
			r.le.WithError(rd.DecodeErr).Warnf("cannot decode %s", rd.Node.Ref.MarshalString())
			r.countBlock()
			r.res.undecodable++
			r.undecodable[rd.Node.Ref.MarshalString()] = struct{}{}
			continue
		}

		// Count and verify the decoded block's list, an empty one for a leaf.
		if err := r.visitDecoded(ctx, rd.Node.Ref, rd.Refs); err != nil {
			return err
		}
	}
	return nil
}

// visitDecoded counts a decoded block, and its list when the volume lacks
// one, and verifies the list.
func (r *refRepair) visitDecoded(ctx context.Context, ref *block.BlockRef, refs []*block.BlockRef) error {
	// Count the block, and its refs when the volume lacks its list.
	r.countBlock()
	listed, err := r.vol.HasChildLists(ctx, []*block.BlockRef{ref})
	if err != nil {
		return err
	}
	if !listed[0] {
		r.res.lacking++
		r.res.edges += uint64(len(refs))
	}
	return r.verify(ctx, ref, refs)
}

// countBlock counts one walked block that the volume stores.
func (r *refRepair) countBlock() {
	r.res.blocks++
	if r.res.blocks%refRepairProgress == 0 {
		r.le.Infof("walked %d blocks", r.res.blocks)
	}
}

// verify hands a decoded block's refs to the list rebuild, unless this is a
// dry run.
func (r *refRepair) verify(ctx context.Context, ref *block.BlockRef, refs []*block.BlockRef) error {
	if r.rebuild == nil {
		return nil
	}
	return r.rebuild.Verify(ctx, ref, refs)
}

// close releases the walker and keeps what it could not type.
func (r *refRepair) close() {
	r.res.untyped = r.walker.Untyped()
	r.walker.Close()
}

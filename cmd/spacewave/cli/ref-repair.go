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
	// refRepairListSample limits the mismatched stored lists printed in the
	// report.
	refRepairListSample = 20
)

// refRepairList names a walked block whose stored list differs from its
// decoded refs.
type refRepairList struct {
	// ref is the block.
	ref *block.BlockRef
	// diff is how its stored list differs.
	diff block_gc.ListDiff
}

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
	// absentHolders is the number of Space root blocks readHolder found the
	// volume lacks: replay spans, retained sets and held root chains.
	absentHolders uint64
	// absentWalked is the number of blocks below the roots the walker found
	// the volume lacks.
	absentWalked uint64
	// absentBySpace splits absentWalked by the Space whose root the walk
	// entered from, numbered from 1 in walk order, 0 for a named root outside
	// the Spaces. A block reached from several roots counts under the first.
	absentBySpace map[int]uint64
	// absentByKind splits absentWalked by the kind of that root.
	absentByKind map[string]uint64
	// absentByParentType splits absentWalked by the decoded type of the block
	// naming the absent one, refRepairRootParent for an absent root.
	absentByParentType map[string]uint64
	// absentParents is the number of distinct blocks naming an absent block.
	absentParents uint64
	// absentMaxPerParent is the most absent blocks one of them names.
	absentMaxPerParent uint64
	// absentHead is the number of absent blocks below the current head World
	// of a Space's cursor, which the walk enters first in each Space.
	absentHead uint64
	// absentOther is the number of the rest, reached only from older,
	// retained or other roots.
	absentOther uint64
	// undecodable is the number of blocks that failed to decode as their type.
	// A block whose type holds no refs is only checked for presence.
	undecodable uint64
	// corrupt is the number of reads that failed with block.ErrBlockCorrupt.
	corrupt uint64
	// corruptRefs is the number of distinct refs among those reads.
	corruptRefs uint64
	// leafReads is the number of leaf blocks read for their bytes.
	leafReads uint64
	// leafCorrupt is the number of leaf reads that failed with
	// block.ErrBlockCorrupt.
	leafCorrupt uint64
	// leafCorruptRefs is the number of distinct refs among those reads.
	leafCorruptRefs uint64
	// bytesRead is the number of block bytes the walk read.
	bytesRead uint64
	// lacking is the number of walked blocks stored without a child list.
	lacking uint64
	// edges is the number of children those lists name.
	edges uint64
	// lists counts the walked blocks stored with a list, by how the list
	// compares with the decoded refs, read before the run writes. A wrong
	// list the volume marked verified is never rewritten, so this is how one
	// is found.
	lists map[block_gc.ListDiff]uint64
	// listSample holds up to refRepairListSample of the blocks whose stored
	// list differs, in walk order.
	listSample []refRepairList
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
	// replay cursor whose kind names the types they hold, which walkSpace
	// walked. The others in such a bucket, an import or fence root holding
	// roots of several types, count as unwalked.
	spaceRoots uint64
	// typedRoots counts those outside them that refRepair.rootNode types and
	// the run walked.
	typedRoots uint64
	// unwalkedRoots counts the other named roots by refRepairRootKind, with a
	// sample of each. The run decodes none of them.
	unwalkedRoots map[string]refRepairRoots
	// opPayloads counts the operation payload roots the walked Spaces name,
	// which the run decodes none of: their type is the one the operation that
	// carried each names, and the run reads only the refs.
	opPayloads uint64
	// opPayloadSample holds up to refRepairSample of them.
	opPayloadSample []*block.BlockRef
	// opPayloadsDistinct is the number of distinct refs among them.
	opPayloadsDistinct uint64
	// opPayloadsReached is the number of those the run walked anyway, through
	// a root other than the operation that carried it.
	opPayloadsReached uint64
	// holders splits the holders an adoption counts of the blocks the run
	// leaves stored without a child list.
	holders refRepairHolders
}

// refRepairOrigin describes a root the walk enters from.
type refRepairOrigin struct {
	// space is the Space's number in walk order, 0 outside the Spaces.
	space int
	// kind names what holds the root.
	kind string
}

// refRepairHeadKind is the kind of a Space cursor's current head World.
const refRepairHeadKind = "cursor head World"

// refRepairRootParent is the parent type of an absent root.
const refRepairRootParent = "(a root)"

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
	// basis says whether the counts were measured or predicted, and what a
	// prediction assumed.
	basis string
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
//     writer named: a Space's named roots typed by the kind
//     space_world.HeldRootKind gives each name, read with the transform of
//     the Space's replay cursor, or a named root rootNode types by its
//     writer's rule. An operation's payload root has the type its operation
//     names, a blob, a Git repository or a manifest among others, and the run
//     reads only the ref, so it counts each in opPayloads and decodes none.
//   - The list rebuild clears the volume's mark only when every stored block
//     a named root reaches has a verified list, so a block this run did not
//     decode, a payload root among them, keeps the mark.
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
	// holders holds the Space root blocks readHolder decoded outside the
	// walker, by ref string: a few per Space.
	holders map[string]struct{}
	// corrupt holds the refs whose reads failed with block.ErrBlockCorrupt.
	corrupt map[string]struct{}
	// corruptLeaves holds the leaf refs whose reads failed so.
	corruptLeaves map[string]struct{}
	// unlistedPayloads holds, on a dry run, each payload entry of a holder
	// block the volume has no list for: one ref per operation payload a
	// Space's chains hold above its checkpoint, at most the count the report
	// prints as operation payloads not walked. While checkpoints land that is
	// about one checkpoint's stable operations and the unstable ones after
	// them per Space; while a Space's checkpoint stalls it grows with every
	// operation. splitHolders cannot take them as a stream, because whether
	// one stays listless depends on the finished walk.
	unlistedPayloads []*block.BlockRef
	// origins describes each root the walk enters from, by Node.Origin. Entry
	// 0 is a root no caller tagged.
	origins []refRepairOrigin
	// absentPerParent counts the absent blocks each parent names, by ref
	// string.
	absentPerParent map[string]uint64
	// payloads holds each distinct operation payload root countPayloads
	// counted, by ref string: one per operation the walked Spaces name.
	payloads map[string]*block.BlockRef
	// res counts the findings.
	res refRepairResult
}

// newRefRepair constructs a ref repair over a stopped volume, typing objects'
// roots with lookup.
func newRefRepair(le *logrus.Entry, vol *volume_s4db.Volume, lookup refwalk.LookupBlockType) *refRepair {
	return &refRepair{
		le:              le,
		vol:             vol,
		buckets:         vol,
		walker:          refwalk.NewWalker(le, vol, lookup),
		spaceBuckets:    make(map[string]struct{}),
		undecodable:     make(map[string]struct{}),
		holders:         make(map[string]struct{}),
		corrupt:         make(map[string]struct{}),
		corruptLeaves:   make(map[string]struct{}),
		payloads:        make(map[string]*block.BlockRef),
		origins:         []refRepairOrigin{{kind: "untagged"}},
		absentPerParent: make(map[string]uint64),
		res: refRepairResult{
			unwalkedRoots:      make(map[string]refRepairRoots),
			lists:              make(map[block_gc.ListDiff]uint64),
			absentBySpace:      make(map[int]uint64),
			absentByKind:       make(map[string]uint64),
			absentByParentType: make(map[string]uint64),
		},
	}
}

// walkNamedRoots streams every named root of the volume and walks each that
// rootNode types, outside the buckets of the walked Spaces. walkSpace walked
// the roots of such a bucket whose kind spaceRootTyped types. It counts the
// rest by kind and decodes none of them.
func (r *refRepair) walkNamedRoots(ctx context.Context) error {
	return r.vol.EachRoot(ctx, func(bucketID, name string, ref *block.BlockRef) error {
		// Count a walked Space's typed roots, which walkSpace walked.
		r.res.roots++
		_, inSpace := r.spaceBuckets[bucketID]
		if inSpace && spaceRootTyped(name) {
			r.res.spaceRoots++
			return nil
		}

		// Walk a typed root, and count the others.
		var node refwalk.Node
		ok := false
		if !inSpace {
			var err error
			node, ok, err = r.rootNode(bucketID, name, ref)
			if err != nil {
				return err
			}
		}
		if !ok {
			r.countUnwalked(bucketID, name)
			return nil
		}
		r.res.typedRoots++
		r.le.Infof("walking the World root %s of bucket %s", name, bucketID)
		return r.walker.Walk(ctx, r.tag([]refwalk.Node{node}, 0, "entrypoint World root")[0], r.visit)
	})
}

// countUnwalked counts the named root name of bucketID, which the run did not
// walk, by its kind.
func (r *refRepair) countUnwalked(bucketID, name string) {
	// Count the root under its kind, sampling the first of each kind.
	kind := refRepairRootKind(name)
	roots := r.res.unwalkedRoots[kind]
	if roots.count == 0 {
		roots.sample = bucketID + " " + name
	}
	roots.count++
	r.res.unwalkedRoots[kind] = roots
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
// streams every root and child list past it. After a real run walked is nil
// and it measures. On a dry run walked reports the blocks the walk reached,
// and it predicts: a real run lists each one it decoded or found stored as a
// leaf, unless the volume's lists are verified or an adoption is counting, in
// which case a real run writes no list and it counts the volume as it is.
func (r *refRepair) splitHolders(ctx context.Context, walked func(*block.BlockRef) bool) error {
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

	// Decide which walked blocks a real run would list.
	h := &r.res.holders
	h.basis = "measured after the run"
	if walked != nil {
		state, err := rec.GetState(ctx)
		if err != nil {
			return err
		}
		h.basis = "predicted for a real run"
		if !state.GetUnverifiedLists() || state.GetAdoption() == block_gc.Adoption_ADOPTION_ADOPTING {
			h.basis = "predicted for a real run, which writes no list: the volume's lists are verified or an adoption is counting"
			walked = nil
		}
	}

	// left reports whether ref stays stored without a list after the run.
	listedByRun := func(ref *block.BlockRef) bool {
		_, bad := r.undecodable[ref.MarshalString()]
		return walked != nil && walked(ref) && !bad
	}
	left := func(ref *block.BlockRef) (bool, error) {
		listed, err := rec.HasChildren(ctx, ref)
		if err != nil || listed || listedByRun(ref) {
			return false, err
		}
		return r.vol.GetBlockExists(ctx, ref)
	}

	// Count the roots holding one.
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

	// Count the payload entries of the holder lists a real run would write,
	// which no stored list shows yet.
	if walked != nil {
		for _, ref := range r.unlistedPayloads {
			held, err := left(ref)
			if err != nil {
				return err
			}
			if held {
				h.reached++
				h.heldSample = sampleRef(h.heldSample, ref)
			}
		}
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

// walkSpace walks the roots of one Space: what each named root of its bucket
// holds, by the kind space_world.HeldRootKind gives the root's name, the
// replay span an engine before the adapter held, the base and head Worlds of
// the cursor with their retained roots, and the World of each outcome. A
// mixed root records no types, so walkNamedRoots counts it unwalked, and an
// operation payload root has its operation's type, so it is counted in
// opPayloads; the blocks below either keep their lists.
func (r *refRepair) walkSpace(ctx context.Context, sc spaceReplayCursor) error {
	// Find the bucket that owns the Space's blocks.
	bucketID, err := findSpaceBucket(ctx, r.buckets, sc.spaceID)
	if err != nil {
		return err
	}
	spaceWorld := sc.world()
	r.res.spaces++
	r.spaceBuckets[bucketID] = struct{}{}

	// Collect what each of the bucket's named roots holds.
	var roots []refwalk.Node
	space := r.res.spaces
	err = r.vol.ListBucketRoots(ctx, bucketID, "", func(root block.NamedRoot) error {
		nodes, err := r.spaceRootNodes(ctx, spaceWorld, root.Name, root.Ref)
		roots = append(roots, r.tag(nodes, space, "named root "+refRepairRootKind(root.Name))...)
		return err
	})
	if err != nil {
		return err
	}

	// Collect the cursor's Worlds with their retained roots and the World of
	// each outcome, and count each outcome's payloads. The current head World
	// walks first, so a block below it counts under it however many other
	// roots of the Space also reach it.
	for i, state := range []*space_world.InnerState{sc.cursor.GetHead(), sc.cursor.GetBase()} {
		head := state.GetHeadRef()
		if head == nil {
			continue
		}
		name := [...]string{"cursor head", "cursor base"}[i]
		for j, ref := range slices.Concat([]*block.BlockRef{head.GetRootRef()}, retainedRefs(state)) {
			node, err := r.walker.WorldNode(head, ref)
			if err != nil {
				return err
			}
			kind := name + " retained"
			if j == 0 {
				kind = name + " World"
			}
			node = r.tag([]refwalk.Node{node}, space, kind)[0]
			if kind == refRepairHeadKind {
				roots = append([]refwalk.Node{node}, roots...)
				continue
			}
			roots = append(roots, node)
		}
	}
	for _, outcome := range sc.cursor.GetOutcomes() {
		node, err := r.walker.WorldNode(spaceWorld, outcome.GetWorld())
		if err != nil {
			return err
		}
		roots = append(roots, r.tag([]refwalk.Node{node}, space, "outcome World")[0])
		if err := r.countPayloads(ctx, nil, outcome.GetPayloads()); err != nil {
			return err
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

// retainedRefs returns the retained World roots of state.
func retainedRefs(state *space_world.InnerState) []*block.BlockRef {
	refs := make([]*block.BlockRef, 0, len(state.GetRetainedRoots()))
	for _, retained := range state.GetRetainedRoots() {
		refs = append(refs, retained.GetRootRef())
	}
	return refs
}

// tag records an origin of space and kind and marks nodes with it.
func (r *refRepair) tag(nodes []refwalk.Node, space int, kind string) []refwalk.Node {
	r.origins = append(r.origins, refRepairOrigin{space: space, kind: kind})
	for i := range nodes {
		nodes[i].Origin = len(r.origins) - 1
	}
	return nodes
}

// countAbsent counts a block below a root that the volume lacks, by where the
// walk reached it from.
func (r *refRepair) countAbsent(n refwalk.Node) {
	// Count it under the root it was reached from.
	r.res.absentWalked++
	origin := r.origins[n.Origin]
	r.res.absentBySpace[origin.space]++
	r.res.absentByKind[origin.kind]++
	if origin.kind == refRepairHeadKind {
		r.res.absentHead++
	} else {
		r.res.absentOther++
	}

	// Count it under the block that names it.
	if n.Parent == "" {
		r.res.absentByParentType[refRepairRootParent]++
		return
	}
	r.res.absentByParentType[n.ParentType]++
	r.absentPerParent[n.Parent]++
	c := r.absentPerParent[n.Parent]
	if c == 1 {
		r.res.absentParents++
	}
	r.res.absentMaxPerParent = max(r.res.absentMaxPerParent, c)
}

// legacyReplaySpanRootName names the root a Space engine before the adapter
// held its replay span under.
const legacyReplaySpanRootName = "replay-span"

// spaceRootTyped reports whether spaceRootNodes types the Space's named root
// name: the replay span, or a root whose kind names the types it holds.
func spaceRootTyped(name string) bool {
	kind, _ := space_world.HeldRootKind(name)
	return name == legacyReplaySpanRootName || (kind != 0 && kind != space_world.RootKindMixed)
}

// spaceRootNodes reads the block the Space's named root name holds at ref by
// the root's kind, verifies the list of each holder block it decodes, and
// returns the World roots below them, which the Space's World transform
// encodes. It counts the operation payload roots below them in opPayloads. A
// root spaceRootTyped does not type returns none.
func (r *refRepair) spaceRootNodes(ctx context.Context, spaceWorld *bucket.ObjectRef, name string, ref *block.BlockRef) ([]refwalk.Node, error) {
	// Type a single World root, and read a span.
	if name == legacyReplaySpanRootName {
		return r.spanRoots(ctx, spaceWorld, ref)
	}
	kind, _ := space_world.HeldRootKind(name)
	switch kind {
	case space_world.RootKindWorld:
		return r.rootNodes(spaceWorld, []*block.BlockRef{ref})
	case space_world.RootKindRetained:
		return r.retainedRoots(ctx, spaceWorld, ref)
	case space_world.RootKindWorlds:
		var worlds []*block.BlockRef
		err := r.heldRoots(ctx, ref, func(_ *block.BlockRef, roots []*block.BlockRef) error {
			worlds = append(worlds, roots...)
			return nil
		})
		if err != nil {
			return nil, err
		}
		return r.rootNodes(spaceWorld, worlds)
	case space_world.RootKindOpPayloads:
		return nil, r.heldRoots(ctx, ref, func(holder *block.BlockRef, roots []*block.BlockRef) error {
			return r.countPayloads(ctx, holder, roots)
		})
	}
	return nil, nil
}

// heldRoots reads the HeldRoots chain at ref, verifies the list of each of
// its blocks, the block's roots and its Prev, and passes each block with the
// roots it holds to take.
func (r *refRepair) heldRoots(ctx context.Context, ref *block.BlockRef, take func(holder *block.BlockRef, roots []*block.BlockRef) error) error {
	for !ref.GetEmpty() {
		// Read one block of the chain.
		held := &space_world.HeldRoots{}
		ok, err := r.readHolder(ctx, ref, held)
		if err != nil || !ok {
			return err
		}

		// Verify its list and take its roots.
		refs := held.GetRoots()
		if prev := held.GetPrev(); !prev.GetEmpty() {
			refs = append(slices.Clone(refs), prev)
		}
		if err := r.visitDecoded(ctx, ref, refs); err != nil {
			return err
		}
		if err := take(ref, held.GetRoots()); err != nil {
			return err
		}
		ref = held.GetPrev()
	}
	return nil
}

// retainedRoots reads the RetainedRootSet at ref, verifies its list, and
// returns its World roots.
func (r *refRepair) retainedRoots(ctx context.Context, spaceWorld *bucket.ObjectRef, ref *block.BlockRef) ([]refwalk.Node, error) {
	// Read the set.
	set := &space_world.RetainedRootSet{}
	ok, err := r.readHolder(ctx, ref, set)
	if err != nil || !ok {
		return nil, err
	}

	// Verify its list and return its Worlds.
	refs := make([]*block.BlockRef, 0, len(set.GetRoots()))
	for _, root := range set.GetRoots() {
		refs = append(refs, root.GetRootRef())
	}
	if err := r.visitDecoded(ctx, ref, refs); err != nil {
		return nil, err
	}
	return r.rootNodes(spaceWorld, refs)
}

// spanRoots reads the replay span at ref, verifies its list, returns its
// Worlds, and counts its payloads in opPayloads.
func (r *refRepair) spanRoots(ctx context.Context, spaceWorld *bucket.ObjectRef, ref *block.BlockRef) ([]refwalk.Node, error) {
	// Read the span.
	span := &space_world.ReplaySpan{}
	ok, err := r.readHolder(ctx, ref, span)
	if err != nil || !ok {
		return nil, err
	}

	// Verify its list, count its payloads and return its Worlds.
	if err := r.visitDecoded(ctx, ref, slices.Concat(span.GetWorlds(), span.GetPayloads())); err != nil {
		return nil, err
	}
	if err := r.countPayloads(ctx, ref, span.GetPayloads()); err != nil {
		return nil, err
	}
	return r.rootNodes(spaceWorld, span.GetWorlds())
}

// readHolder reads the raw protobuf block at ref into msg, counting it absent
// or undecodable when it cannot, and reports whether it decoded. An empty ref
// holds nothing, and a block it decoded before was walked already, with what
// it names.
func (r *refRepair) readHolder(ctx context.Context, ref *block.BlockRef, msg interface{ UnmarshalVT([]byte) error }) (bool, error) {
	// Read the block once.
	key := ref.MarshalString()
	if _, ok := r.holders[key]; ok || ref.GetEmpty() {
		return false, nil
	}
	data, found, err := r.vol.GetBlock(ctx, ref)
	r.res.bytesRead += uint64(len(data))
	if errors.Is(err, block.ErrBlockCorrupt) {
		r.countCorrupt(key, false)
		return false, nil
	}
	if err != nil || !found {
		if err == nil {
			r.res.absentHolders++
		}
		return false, err
	}

	// Decode it, counting a block that does not decode.
	if err := msg.UnmarshalVT(data); err != nil {
		r.le.WithError(err).Warnf("cannot decode root block %s", ref.MarshalString())
		r.countBlock()
		r.res.undecodable++
		r.undecodable[key] = struct{}{}
		return false, nil
	}
	r.holders[key] = struct{}{}
	return true, nil
}

// walked returns a report of whether the run decoded a block or found it
// stored as a leaf, through the walker or as a Space's root block. It stays
// valid after close.
func (r *refRepair) walked() func(*block.BlockRef) bool {
	visited, holders := r.walker.Visited, r.holders
	return func(ref *block.BlockRef) bool {
		_, ok := holders[ref.MarshalString()]
		return ok || visited(ref)
	}
}

// countReachedPayloads counts the distinct operation payload roots, and those
// walked reports the run reached through another root.
func (r *refRepair) countReachedPayloads(walked func(*block.BlockRef) bool) {
	r.res.opPayloadsDistinct = uint64(len(r.payloads))
	for _, ref := range r.payloads {
		if walked(ref) {
			r.res.opPayloadsReached++
		}
	}
}

// rootNodes returns a World node of each of worlds, read with spaceWorld's
// transform.
func (r *refRepair) rootNodes(spaceWorld *bucket.ObjectRef, worlds []*block.BlockRef) ([]refwalk.Node, error) {
	out := make([]refwalk.Node, 0, len(worlds))
	for _, ref := range worlds {
		node, err := r.walker.WorldNode(spaceWorld, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, node)
	}
	return out, nil
}

// countPayloads counts operation payload roots the run does not decode: each
// has the type its operation names, which the run cannot read from the ref,
// so the blocks below it keep the lists they have. lister is the holder block
// whose list names them, or nil when no block list does. On a dry run it keeps
// the payloads of a lister the volume holds no list for: a real run writes
// that list, and splitHolders counts the payloads it would name.
func (r *refRepair) countPayloads(ctx context.Context, lister *block.BlockRef, payloads []*block.BlockRef) error {
	var keep bool
	if r.rebuild == nil && lister != nil {
		_, listed, err := r.vol.GetChildren(ctx, lister)
		if err != nil {
			return err
		}
		keep = !listed
	}
	for _, ref := range payloads {
		if ref.GetEmpty() {
			continue
		}
		r.res.opPayloads++
		r.res.opPayloadSample = sampleRef(r.res.opPayloadSample, ref)
		r.payloads[ref.MarshalString()] = ref
		if keep {
			r.unlistedPayloads = append(r.unlistedPayloads, ref)
		}
	}
	return nil
}

// visit counts a batch of reads and verifies each decoded block's list.
func (r *refRepair) visit(ctx context.Context, batch []refwalk.Visit) error {
	for _, rd := range batch {
		// Count a corrupt read, a block the volume lacks, a leaf, and an
		// undecodable block.
		r.res.bytesRead += rd.Size
		if rd.Leaf {
			r.res.leafReads++
		}
		if rd.Corrupt {
			r.countCorrupt(rd.Node.Ref.MarshalString(), rd.Leaf)
			continue
		}
		if !rd.Found {
			r.countAbsent(rd.Node)
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

// visitDecoded counts a decoded block, its refs when the volume lacks its
// list, and how its stored list compares with them otherwise, then verifies
// the list.
func (r *refRepair) visitDecoded(ctx context.Context, ref *block.BlockRef, refs []*block.BlockRef) error {
	// Count the block, and its refs when the volume lacks its list.
	r.countBlock()
	stored, listed, err := r.vol.GetChildren(ctx, ref)
	if err != nil {
		return err
	}
	if !listed {
		r.res.lacking++
		r.res.edges += uint64(len(refs))
		return r.verify(ctx, ref, refs)
	}

	// Compare the stored list with the decoded refs, and sample a mismatch.
	diff := block_gc.CompareChildren(stored, refs)
	r.res.lists[diff]++
	if diff != block_gc.ListEqual && len(r.res.listSample) < refRepairListSample {
		r.res.listSample = append(r.res.listSample, refRepairList{ref: ref, diff: diff})
	}
	return r.verify(ctx, ref, refs)
}

// countCorrupt counts one read that failed with block.ErrBlockCorrupt, and
// its ref the first time it is seen, a leaf apart from a decoded block.
func (r *refRepair) countCorrupt(key string, leaf bool) {
	if leaf {
		r.res.leafCorrupt++
		if _, ok := r.corruptLeaves[key]; !ok {
			r.corruptLeaves[key] = struct{}{}
			r.res.leafCorruptRefs++
		}
		return
	}
	r.res.corrupt++
	if _, ok := r.corrupt[key]; !ok {
		r.corrupt[key] = struct{}{}
		r.res.corruptRefs++
	}
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

// close keeps what the walker could not type and releases it, with the set of
// blocks it walked unless a caller kept its Visited for a prediction.
func (r *refRepair) close() {
	if r.walker == nil {
		return
	}
	r.res.untyped = r.walker.Untyped()
	r.walker.Close()
	r.walker = nil
}

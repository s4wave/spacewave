//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"maps"
	"slices"
	"strings"

	"github.com/pkg/errors"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_store "github.com/s4wave/spacewave/db/block/store"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
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
	// absent is the number of referenced blocks no read resolves.
	absent uint64
	// peerRead is the number of walked blocks the volume lacks that the
	// peer read resolved, whose lists the rebuild writes without bytes.
	peerRead uint64
	// synthetic is the number of blocks listed with no children because the
	// synthetic set names them, as refRepair.synthetic describes.
	synthetic uint64
	// undecodable is the number of blocks that failed to decode as their type.
	// A block whose type holds no refs is only checked for presence.
	undecodable uint64
	// lacking is the number of walked blocks stored without a child list.
	lacking uint64
	// edges is the number of children those lists name.
	edges uint64
	// rebuild is what the list rebuild verified and wrote, empty on a dry run
	// and on a volume whose old generation had left Open.
	rebuild block_gc.ListRebuildResult
	// verified reports that the volume's old generation had left Open before
	// the run, so it rebuilt nothing.
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
}

// refRepair walks the World graphs the Spaces of a stopped volume retain with
// a refwalk.Walker, and hands each decoded block's refs to a list rebuild,
// which writes the list when it is missing or short and marks it verified. A
// block's list is the refs block.ExtractBlockRefs reads from the decoded
// block, which are the refs a write records for it.
type refRepair struct {
	// le is the logger.
	le *logrus.Entry
	// vol is the stopped volume.
	vol *volume_s4db.Volume
	// buckets lists the volume's bucket configs.
	buckets bucket_store.Store
	// walker walks the blocks below the Worlds.
	walker *refwalk.Walker
	// peer reads the blocks the volume lacks, nil to read the volume alone.
	// The walk never fills the volume from it.
	peer block.StoreOps
	// synthetic names the blocks a rehearsal lists with no children when no
	// read decodes them, by their keys, and records whether the walk did.
	// It proves nothing about them: it lets a rehearsal reach the flip on a
	// copy whose unresolved blocks hold it.
	synthetic map[string]bool
	// rebuild verifies the decoded blocks' lists, nil on a dry run.
	rebuild *block_gc.ListRebuild
	// res counts the findings.
	res refRepairResult
}

// newRefRepair constructs a ref repair over a stopped volume, typing objects'
// roots with lookup. With peer set the walk reads each block the volume
// lacks from peer, and never fills the volume.
func newRefRepair(le *logrus.Entry, vol *volume_s4db.Volume, peer block.StoreOps, lookup refwalk.LookupBlockType) *refRepair {
	var reads block.StoreOps = vol
	if peer != nil {
		reads = block_store.NewStoreReadThrough(
			func() block.StoreOps { return vol },
			func() block.StoreOps { return peer },
			nil,
		)
	}
	return &refRepair{
		le:      le,
		vol:     vol,
		buckets: vol,
		walker:  refwalk.NewWalker(le, reads, lookup),
		peer:    peer,
	}
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

// walkSpace walks the roots of one Space's replay: the roots its bucket holds
// by name, the base and head Worlds of the cursor with their retained roots,
// and the World and payloads of each outcome. It walks every World before any
// payload, so a payload the Worlds name as an object root is walked under its
// object's type, as refwalk.Walker.PayloadNode describes.
func (r *refRepair) walkSpace(ctx context.Context, sc spaceReplayCursor) error {
	// Find the bucket that owns the Space's blocks.
	bucketID, err := findSpaceBucket(ctx, r.buckets, sc.spaceID)
	if err != nil {
		return err
	}
	spaceWorld := sc.world()
	r.res.spaces++

	// Collect the Worlds and payloads the bucket's named roots hold.
	worlds, payloads, err := r.bucketRoots(ctx, bucketID, spaceWorld)
	if err != nil {
		return err
	}

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
			worlds = append(worlds, node)
		}
	}
	for _, outcome := range sc.cursor.GetOutcomes() {
		node, err := r.walker.WorldNode(spaceWorld, outcome.GetWorld())
		if err != nil {
			return err
		}
		worlds = append(worlds, node)
		payloads = append(payloads, outcome.GetPayloads()...)
	}

	// Walk each World, then each payload the Worlds did not type.
	r.le.Infof("walking %d worlds and %d payloads of space %s in bucket %s", len(worlds), len(payloads), sc.spaceID, bucketID)
	for _, root := range worlds {
		if err := r.walker.Walk(ctx, root, r.visit); err != nil {
			return err
		}
	}
	for _, ref := range payloads {
		root, ok, err := r.walker.PayloadNode(spaceWorld, ref)
		if err != nil {
			return err
		}
		if !ok {
			continue
		}
		if err := r.walker.Walk(ctx, root, r.visit); err != nil {
			return err
		}
	}
	return nil
}

// legacyReplaySpanRootName names the root a Space engine before the adapter
// held its replay span under.
const legacyReplaySpanRootName = "replay-span"

// bucketRoots reads every root a Space's bucket holds by name, as
// space_world's roots.go lists them, and the legacy replay span. It verifies
// the list of each block that holds roots, and returns the Worlds and
// payload roots those blocks name.
func (r *refRepair) bucketRoots(ctx context.Context, bucketID string, spaceWorld *bucket.ObjectRef) ([]refwalk.Node, []*block.BlockRef, error) {
	// Each named root adds its Worlds or payload roots.
	var worlds []refwalk.Node
	var payloads []*block.BlockRef
	addWorlds := func(refs []*block.BlockRef) error {
		for _, ref := range refs {
			node, err := r.walker.WorldNode(spaceWorld, ref)
			if err != nil {
				return err
			}
			worlds = append(worlds, node)
		}
		return nil
	}
	for _, name := range spaceRootNames {
		ref, err := r.vol.GetBucketRoot(ctx, bucketID, name)
		if err != nil {
			return nil, nil, err
		}
		if ref.GetEmpty() {
			continue
		}
		switch name {
		case space_world.RootReplayBase:
			err = addWorlds([]*block.BlockRef{ref})
		case space_world.RootRetained:
			var roots []*block.BlockRef
			roots, err = r.retainedRoots(ctx, ref)
			if err == nil {
				err = addWorlds(roots)
			}
		case space_world.RootReplayPayloads:
			var roots []*block.BlockRef
			roots, err = r.heldRoots(ctx, ref)
			payloads = append(payloads, roots...)
		case legacyReplaySpanRootName:
			var spanWorlds, spanPayloads []*block.BlockRef
			spanWorlds, spanPayloads, err = r.spanRoots(ctx, ref)
			if err == nil {
				err = addWorlds(spanWorlds)
			}
			payloads = append(payloads, spanPayloads...)
		default:
			var roots []*block.BlockRef
			roots, err = r.heldRoots(ctx, ref)
			if err == nil {
				err = addWorlds(roots)
			}
		}
		if err != nil {
			return nil, nil, err
		}
	}
	return worlds, payloads, nil
}

// spaceRootNames are the names a Space's bucket holds roots under. The
// payloads root holds operation payloads, the replay base a World, the
// retained root a RetainedRootSet and the legacy span a ReplaySpan chain;
// every other root holds a HeldRoots chain of Worlds.
var spaceRootNames = []string{
	space_world.RootReplayBase,
	space_world.RootCheckpoint,
	space_world.RootImport,
	space_world.RootReplayCursor,
	space_world.RootReplayPayloads,
	space_world.RootRetained,
	space_world.RootPending,
	space_world.RootBackfill,
	legacyReplaySpanRootName,
}

// readRootBlock reads a block a named root holds, or reports it absent.
func (r *refRepair) readRootBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	data, found, err := r.vol.GetBlock(ctx, ref)
	if err != nil || found {
		return data, found, err
	}
	r.res.absent++
	return nil, false, nil
}

// undecodableRoot counts a root block that does not decode as its root's type.
func (r *refRepair) undecodableRoot(ref *block.BlockRef, err error) {
	r.le.WithError(err).Warnf("cannot decode root block %s", ref.MarshalString())
	r.res.undecodable++
}

// heldRoots reads the HeldRoots chain at ref, verifying each block's list,
// and returns its roots.
func (r *refRepair) heldRoots(ctx context.Context, ref *block.BlockRef) ([]*block.BlockRef, error) {
	var roots []*block.BlockRef
	for !ref.GetEmpty() {
		data, found, err := r.readRootBlock(ctx, ref)
		if err != nil || !found {
			return roots, err
		}
		msg := &space_world.HeldRoots{}
		if err := msg.UnmarshalVT(data); err != nil {
			r.undecodableRoot(ref, err)
			return roots, nil
		}
		if err := r.visitDecoded(ctx, ref, appendRef(msg.GetRoots(), msg.GetPrev())); err != nil {
			return nil, err
		}
		roots = append(roots, msg.GetRoots()...)
		ref = msg.GetPrev()
	}
	return roots, nil
}

// retainedRoots reads the RetainedRootSet at ref, verifying its list, and
// returns its World roots.
func (r *refRepair) retainedRoots(ctx context.Context, ref *block.BlockRef) ([]*block.BlockRef, error) {
	// Read and decode the set, then verify its list.
	data, found, err := r.readRootBlock(ctx, ref)
	if err != nil || !found {
		return nil, err
	}
	msg := &space_world.RetainedRootSet{}
	if err := msg.UnmarshalVT(data); err != nil {
		r.undecodableRoot(ref, err)
		return nil, nil
	}
	roots := make([]*block.BlockRef, 0, len(msg.GetRoots()))
	for _, root := range msg.GetRoots() {
		roots = appendRef(roots, root.GetRootRef())
	}
	return roots, r.visitDecoded(ctx, ref, roots)
}

// spanRoots reads the ReplaySpan chain at ref, raw protobuf blocks, verifying
// each block's list, and returns its Worlds and payload roots.
func (r *refRepair) spanRoots(ctx context.Context, ref *block.BlockRef) ([]*block.BlockRef, []*block.BlockRef, error) {
	var worlds, payloads []*block.BlockRef
	for !ref.GetEmpty() {
		data, found, err := r.readRootBlock(ctx, ref)
		if err != nil || !found {
			return worlds, payloads, err
		}
		span := &space_world.ReplaySpan{}
		if err := span.UnmarshalVT(data); err != nil {
			r.undecodableRoot(ref, err)
			return worlds, payloads, nil
		}
		refs := appendRef(slices.Concat(span.GetWorlds(), span.GetPayloads()), span.GetPrev())
		if err := r.visitDecoded(ctx, ref, refs); err != nil {
			return nil, nil, err
		}
		worlds = append(worlds, span.GetWorlds()...)
		payloads = append(payloads, span.GetPayloads()...)
		ref = span.GetPrev()
	}
	return worlds, payloads, nil
}

// appendRef appends ref to refs unless it is empty.
func appendRef(refs []*block.BlockRef, ref *block.BlockRef) []*block.BlockRef {
	if ref.GetEmpty() {
		return refs
	}
	return append(refs, ref)
}

// visit counts a batch of reads and verifies each decoded block's list.
func (r *refRepair) visit(ctx context.Context, batch []refwalk.Visit) error {
	for _, rd := range batch {
		// List a synthetic block no read decodes with no children.
		if (!rd.Found || rd.DecodeErr != nil) && r.listSynthetic(rd.Node.Ref) {
			if err := r.verify(ctx, rd.Node.Ref, nil); err != nil {
				return err
			}
			continue
		}

		// Count a block no read resolves, a leaf, and an undecodable block.
		if !rd.Found {
			r.res.absent++
			continue
		}
		if rd.DecodeErr != nil {
			r.le.WithError(rd.DecodeErr).Warnf("cannot decode %s", rd.Node.Ref.MarshalString())
			r.countBlock()
			r.res.undecodable++
			continue
		}
		if rd.Leaf {
			r.countBlock()
			if err := r.verify(ctx, rd.Node.Ref, nil); err != nil {
				return err
			}
			continue
		}

		// Count and verify the decoded block's list.
		if err := r.visitDecoded(ctx, rd.Node.Ref, rd.Refs); err != nil {
			return err
		}
	}
	return nil
}

// listSynthetic reports whether the synthetic set names ref, and records
// that the walk listed it.
func (r *refRepair) listSynthetic(ref *block.BlockRef) bool {
	// Mark the named ref listed and count it.
	key := ref.MarshalString()
	if _, ok := r.synthetic[key]; !ok {
		return false
	}
	r.synthetic[key] = true
	r.res.synthetic++
	return true
}

// listUnwalkedSynthetic lists with no children each block of the synthetic
// set the walk never reached, such as an object root whose type does not
// read.
func (r *refRepair) listUnwalkedSynthetic(ctx context.Context) error {
	for _, key := range slices.Sorted(maps.Keys(r.synthetic)) {
		if r.synthetic[key] {
			continue
		}
		ref, err := block.UnmarshalBlockRefB58(key)
		if err != nil {
			return err
		}
		r.listSynthetic(ref)
		if err := r.verify(ctx, ref, nil); err != nil {
			return err
		}
	}
	return nil
}

// visitDecoded counts a decoded block, and its list when the volume lacks
// one, and verifies the list.
func (r *refRepair) visitDecoded(ctx context.Context, ref *block.BlockRef, refs []*block.BlockRef) error {
	// Count the block, and a block only the peer read resolved.
	r.countBlock()
	if r.peer != nil {
		stored, err := r.vol.GetBlockExists(ctx, ref)
		if err != nil {
			return err
		}
		if !stored {
			r.res.peerRead++
		}
	}

	// Count its refs when the volume lacks its list.
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

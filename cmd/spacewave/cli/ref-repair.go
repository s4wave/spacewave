//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"slices"
	"strings"

	"github.com/pkg/errors"
	space_world "github.com/s4wave/spacewave/core/space/world"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
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
	// rebuild verifies the decoded blocks' lists, nil on a dry run.
	rebuild *block_gc.ListRebuild
	// res counts the findings.
	res refRepairResult
}

// newRefRepair constructs a ref repair over a stopped volume, typing objects'
// roots with lookup.
func newRefRepair(le *logrus.Entry, vol *volume_s4db.Volume, lookup refwalk.LookupBlockType) *refRepair {
	return &refRepair{
		le:      le,
		vol:     vol,
		buckets: vol,
		walker:  refwalk.NewWalker(le, vol, lookup),
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

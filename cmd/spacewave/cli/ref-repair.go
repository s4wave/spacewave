//go:build !js

package spacewave_cli

import (
	"bytes"
	"context"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	sobject_world_engine "github.com/s4wave/spacewave/core/sobject/world/engine"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	bucket_store "github.com/s4wave/spacewave/db/bucket/store"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_block_iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	// refRepairBatch is the number of blocks taken from the top of the walk's
	// stack and read together.
	refRepairBatch = 64
	// refRepairReaders bounds the concurrent block and ref graph reads. The
	// reads are random preads, so a few in flight hide the disk latency.
	refRepairReaders = 16
	// refRepairProgress is the number of blocks walked between progress logs.
	refRepairProgress = 100_000
	// refRepairSample limits the untracked blocks printed in the report.
	refRepairSample = 10
)

// refRepairNode is a block the ref repair walks, with what decodes it.
type refRepairNode struct {
	// ref addresses the block.
	ref *block.BlockRef
	// ctor constructs the block's type. It is nil for the replay span.
	ctor block.Ctor
	// xfrm decodes the stored bytes. It is nil for the raw replay span.
	xfrm block.Transformer
	// world is the World whose graph types the objects below the block.
	world *bucket.ObjectRef
	// values constructs the values of the tree the block belongs to, nil when
	// the block is not a tree node or the tree's values are untyped.
	values block.Ctor
}

// refRepairRead is what reading one block found. The reads of a batch run
// concurrently; the walk then applies each read in stack order.
type refRepairRead struct {
	// node is the block read.
	node refRepairNode
	// iri is the block's ref graph node.
	iri string
	// found is false when the volume lacks the block.
	found bool
	// leaf is true when the block's type holds no refs, so it was not read.
	leaf bool
	// blk is the decoded block, nil for a leaf or an undecodable block.
	blk any
	// decodeErr is why the block did not decode.
	decodeErr error
	// refs are the refs a write records for the block.
	refs []*block.BlockRef
	// have are the edges the graph holds for the block, read when refs is not
	// empty.
	have []string
}

// refRepairStoredBlock describes an untracked block without decoding its type.
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
	// lacking is the number of blocks missing at least one outgoing edge.
	lacking uint64
	// edges is the number of missing edges from a block to a child.
	edges uint64
	// owned is the number of roots nothing held, given to their bucket.
	owned uint64
	// rooted is the number of Space buckets the GC root did not hold.
	rooted int
	// written is the number of edges added to the graph.
	written uint64
	// untyped counts the refs whose type the walk cannot name, by reason.
	// Their edges are recorded; the blocks below them are not walked.
	untyped map[string]uint64
	// pluginStores counts object-store keys by plugin-volume/<instance>/<plugin>/ prefix.
	// The prefix groups stores whose boundaries are not encoded in their keys.
	pluginStores map[string]uint64
	// untracked counts stored blocks with neither incoming nor outgoing ref
	// graph edges, including staging edges.
	untracked uint64
	// untrackedBytes is the total stored bytes of untracked blocks.
	untrackedBytes uint64
	// untrackedSample holds up to refRepairSample untracked blocks in key order.
	untrackedSample []refRepairStoredBlock
}

// refRepair walks the World graphs the Spaces of a stopped volume retain,
// decoding each block by its type, and collects the ref graph edges the volume
// lacks. A block's edges are the refs block.ExtractBlockRefs reads from the
// decoded block, which are the refs a write records for it. The repair only
// adds edges.
type refRepair struct {
	// le is the logger.
	le *logrus.Entry
	// store reads the volume's blocks.
	store block.StoreOps
	// rg is the volume's ref graph.
	rg block_gc.RefGraphOps
	// buckets lists the volume's bucket configs.
	buckets bucket_store.Store
	// visited holds the IRIs of the blocks already walked.
	visited map[string]struct{}
	// adds are the missing edges found, in walk order.
	adds []block_gc.RefEdge
	// planned holds the objects of adds.
	planned map[string]struct{}
	// types caches object type IDs by object key across Worlds.
	types map[string]string
	// xfrms caches transformers by their marshaled config.
	xfrms map[string]block.Transformer
	// spaceWorld is the World of the Space being walked.
	spaceWorld *bucket.ObjectRef
	// typesWorld is the World typesTx reads, nil before the first lookup.
	typesWorld *bucket.ObjectRef
	// typesEngine is the snapshot engine typesTx belongs to.
	typesEngine *world_block.Engine
	// typesTx reads object types from typesWorld.
	typesTx world.Tx
	// res counts the findings.
	res refRepairResult
}

// newRefRepair constructs a ref repair over a stopped volume.
func newRefRepair(le *logrus.Entry, vol *volume_s4db.Volume) *refRepair {
	return &refRepair{
		le:      le,
		store:   vol,
		rg:      vol.GetRefGraph(),
		buckets: vol,
		visited: make(map[string]struct{}),
		planned: make(map[string]struct{}),
		types:   make(map[string]string),
		xfrms:   make(map[string]block.Transformer),
		res:     refRepairResult{untyped: make(map[string]uint64)},
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
// walk, and measures blocks with no incoming or outgoing graph edges.
func (r *refRepair) measureUntracked(ctx context.Context, vol *volume_s4db.Volume) error {
	// Iterate block keys without loading their values into the scan transaction.
	tx, err := vol.GetKvtxStore().NewTransaction(ctx, false)
	if err != nil {
		return err
	}
	defer tx.Discard()
	prefix := vol.GetKvKey().GetBlockFullPrefix()
	it := tx.Iterate(ctx, prefix, true, false)
	defer it.Close()

	// Read graph membership and untracked block sizes in bounded batches.
	var scanned uint64
	for {
		// Decode only the refs in the next batch's keys, never block types.
		batch := make([]refRepairStoredBlock, 0, refRepairBatch)
		for len(batch) < refRepairBatch && it.Next() {
			ref := &block.BlockRef{}
			if err := ref.UnmarshalVT(bytes.Clone(it.Key()[len(prefix):])); err != nil {
				return errors.Wrap(err, "decode stored block key")
			}
			batch = append(batch, refRepairStoredBlock{ref: ref})
		}
		if err := it.Err(); err != nil {
			return err
		}
		if len(batch) == 0 {
			return nil
		}

		// Include staging owners when checking whether a block has a graph node.
		untracked := make([]bool, len(batch))
		eg, egCtx := errgroup.WithContext(ctx)
		eg.SetLimit(refRepairReaders)
		for i := range batch {
			eg.Go(func() error {
				// An edge in either direction establishes the node's presence.
				// HasIncomingRefs stops at the first owner, which settles most
				// blocks; it skips the unreferenced edge, so the full incoming
				// list is read only for blocks with no owner and no children.
				iri := block_gc.BlockIRI(batch[i].ref)
				owned, err := r.rg.HasIncomingRefs(egCtx, iri)
				if err != nil || owned {
					return err
				}
				outgoing, err := r.rg.GetOutgoingRefs(egCtx, iri)
				if err != nil || len(outgoing) != 0 {
					return err
				}
				incoming, err := r.rg.GetIncomingRefs(egCtx, iri)
				if err != nil || len(incoming) != 0 {
					return err
				}

				// Measure raw stored bytes only for blocks outside the graph.
				data, found, err := r.store.GetBlock(egCtx, batch[i].ref)
				if err != nil {
					return err
				}
				if !found {
					return errors.Errorf("stored block %s disappeared during scan", batch[i].ref.MarshalString())
				}
				batch[i].size = uint64(len(data))
				untracked[i] = true
				return nil
			})
		}
		if err := eg.Wait(); err != nil {
			return err
		}

		// Count the completed reads and retain a small sample in key order.
		for i, blk := range batch {
			if !untracked[i] {
				continue
			}
			r.res.untracked++
			r.res.untrackedBytes += blk.size
			if len(r.res.untrackedSample) < refRepairSample {
				r.res.untrackedSample = append(r.res.untrackedSample, blk)
			}
		}

		// Log progress each time the scan crosses another progress interval.
		prev := scanned
		scanned += uint64(len(batch))
		if scanned/refRepairProgress != prev/refRepairProgress {
			r.le.Infof("scanned %d stored blocks, %d untracked", scanned, r.res.untracked)
		}
	}
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
	r.spaceWorld = sc.world()
	xfrm, err := r.transformer(r.spaceWorld.GetTransformConf())
	if err != nil {
		return err
	}
	r.res.spaces++

	// Collect the replay span the bucket holds as a named root.
	var roots []refRepairNode
	spans, err := r.rg.GetOutgoingRefs(ctx, volume_kvtx.RootOwnerIRI(bucketID, sobject_world_engine.ReplaySpanRootName))
	if err != nil {
		return err
	}
	for _, iri := range spans {
		if ref, ok := block_gc.ParseBlockIRI(iri); ok {
			roots = append(roots, refRepairNode{ref: ref})
		}
	}

	// Collect the cursor's Worlds with their retained roots, and the World and
	// payloads of each outcome.
	for _, state := range []*sobject_world_engine.InnerState{sc.cursor.GetBase(), sc.cursor.GetHead()} {
		head := state.GetHeadRef()
		if head == nil {
			continue
		}
		refs := []*block.BlockRef{head.GetRootRef()}
		for _, retained := range state.GetRetainedRoots() {
			refs = append(refs, retained.GetRootRef())
		}
		for _, ref := range refs {
			node, err := r.worldNode(head, ref)
			if err != nil {
				return err
			}
			roots = append(roots, node)
		}
	}
	for _, outcome := range sc.cursor.GetOutcomes() {
		node, err := r.worldNode(r.spaceWorld, outcome.GetWorld())
		if err != nil {
			return err
		}
		roots = append(roots, node)
		for _, payload := range outcome.GetPayloads() {
			roots = append(roots, refRepairNode{ref: payload, ctor: blob.NewBlobBlock, xfrm: xfrm})
		}
	}

	// Walk each root, then own each root nothing else holds.
	r.le.Infof("walking %d roots of space %s in bucket %s", len(roots), sc.spaceID, bucketID)
	for _, root := range roots {
		if err := r.walk(ctx, root); err != nil {
			return err
		}
	}
	bucketIRI := block_gc.BucketIRI(bucketID)
	if err := r.rootBucket(ctx, bucketIRI); err != nil {
		return err
	}
	for _, root := range roots {
		if err := r.own(ctx, bucketIRI, root.ref); err != nil {
			return err
		}
	}
	return nil
}

// worldNode returns the node of the World at ref, read with the bucket and
// transform of world.
func (r *refRepair) worldNode(world *bucket.ObjectRef, ref *block.BlockRef) (refRepairNode, error) {
	xfrm, err := r.transformer(world.GetTransformConf())
	if err != nil {
		return refRepairNode{}, err
	}
	at := &bucket.ObjectRef{
		RootRef:       ref,
		BucketId:      world.GetBucketId(),
		TransformConf: world.GetTransformConf(),
	}
	return refRepairNode{ref: ref, ctor: world_block.NewWorldBlock, xfrm: xfrm, world: at}, nil
}

// walk visits the blocks below root depth first, so the objects of one World
// resolve their types in one snapshot. It reads a batch from the top of the
// stack concurrently, then plans edges and queues children in stack order.
func (r *refRepair) walk(ctx context.Context, root refRepairNode) error {
	stack := []refRepairNode{root}
	for len(stack) != 0 {
		// Take the unvisited blocks of the next batch from the top of the stack.
		var batch []refRepairNode
		for len(stack) != 0 && len(batch) != refRepairBatch {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			iri := block_gc.BlockIRI(n.ref)
			if iri == "" {
				continue
			}
			if _, ok := r.visited[iri]; ok {
				continue
			}
			r.visited[iri] = struct{}{}
			batch = append(batch, n)
		}

		// Read the batch concurrently.
		reads := make([]refRepairRead, len(batch))
		eg, egCtx := errgroup.WithContext(ctx)
		eg.SetLimit(refRepairReaders)
		for i, n := range batch {
			eg.Go(func() (err error) {
				reads[i], err = r.read(egCtx, n)
				return err
			})
		}
		if err := eg.Wait(); err != nil {
			return err
		}

		// Apply each read and queue its children.
		for i := range reads {
			children, err := r.visit(ctx, &reads[i])
			if err != nil {
				return err
			}
			stack = append(stack, children...)
		}
	}
	return nil
}

// read reads, decodes and extracts the refs of one block, with the edges the
// graph holds for it. It touches no repair state, so reads run concurrently.
func (r *refRepair) read(ctx context.Context, n refRepairNode) (refRepairRead, error) {
	rd := refRepairRead{node: n, iri: block_gc.BlockIRI(n.ref)}

	// Check only the presence of a block whose type holds no refs.
	if n.ctor != nil && !holdsRefs(n.ctor()) {
		var err error
		rd.leaf = true
		rd.found, err = r.store.GetBlockExists(ctx, n.ref)
		return rd, err
	}

	// Read and decode the stored bytes.
	data, found, err := r.store.GetBlock(ctx, n.ref)
	if err != nil || !found {
		return rd, err
	}
	rd.found = true
	rd.blk, rd.decodeErr = decodeRefRepairNode(n, data)
	if rd.decodeErr != nil {
		rd.blk = nil
		return rd, nil
	}

	// Extract the refs a write records and read the edges the graph holds.
	rd.refs, err = block.ExtractBlockRefs(rd.blk)
	if err != nil {
		return rd, err
	}
	if span, ok := rd.blk.(*sobject_world_engine.ReplaySpan); ok {
		rd.refs = slices.Concat(span.GetWorlds(), span.GetPayloads())
	}
	if len(rd.refs) != 0 {
		rd.have, err = r.rg.GetOutgoingRefs(ctx, rd.iri)
	}
	return rd, err
}

// holdsRefs reports whether a block of blk's type can reference other blocks.
func holdsRefs(blk any) bool {
	switch blk.(type) {
	case block.BlockWithRefs, block.BlockWithSubBlocks:
		return true
	}
	return false
}

// visit counts one read, plans the edges the graph lacks for the block, and
// returns its children.
func (r *refRepair) visit(ctx context.Context, rd *refRepairRead) ([]refRepairNode, error) {
	// Count a block the volume lacks, a leaf, and an undecodable block.
	if !rd.found {
		r.res.absent++
		return nil, nil
	}
	r.res.blocks++
	if r.res.blocks%refRepairProgress == 0 {
		r.le.Infof("walked %d blocks", r.res.blocks)
	}
	if rd.leaf {
		return nil, nil
	}
	if rd.decodeErr != nil {
		r.le.WithError(rd.decodeErr).Warnf("cannot decode %s", rd.node.ref.MarshalString())
		r.res.undecodable++
		return nil, nil
	}

	// Plan the edges a write records for the block, then return its children.
	r.recordEdges(rd.iri, rd.refs, rd.have)
	return r.children(ctx, rd.blk, rd.node, rd.node.values)
}

// decodeRefRepairNode decodes the stored bytes of n by its type.
func decodeRefRepairNode(n refRepairNode, data []byte) (any, error) {
	// The replay span is a raw protobuf block.
	if n.ctor == nil {
		span := &sobject_world_engine.ReplaySpan{}
		return span, span.UnmarshalVT(data)
	}

	// Other blocks are encoded with their World's transform.
	if n.xfrm != nil {
		var err error
		data, err = n.xfrm.DecodeBlock(data)
		if err != nil {
			return nil, err
		}
	}
	blk := n.ctor()
	return blk, blk.UnmarshalBlock(data)
}

// recordEdges plans each edge from the block at iri to refs that have, the
// edges the graph holds for it, lacks.
func (r *refRepair) recordEdges(iri string, refs []*block.BlockRef, have []string) {
	var missing uint64
	for _, ref := range refs {
		child := block_gc.BlockIRI(ref)
		if child == "" || slices.Contains(have, child) {
			continue
		}
		have = append(have, child)
		r.plan(iri, child)
		missing++
	}
	if missing != 0 {
		r.res.lacking++
		r.res.edges += missing
	}
}

// children returns the blocks blk references, each with the constructor and
// transform that decode it. values constructs the values of the tree blk
// belongs to, nil when they are untyped.
func (r *refRepair) children(ctx context.Context, blk any, n refRepairNode, values block.Ctor) ([]refRepairNode, error) {
	// The replay span and Objects name their children's types themselves.
	// Only tree blocks pass the tree's value constructor down.
	switch b := blk.(type) {
	case *sobject_world_engine.ReplaySpan:
		return r.spanChildren(b)
	case *world_block.Object:
		return r.objectRoot(ctx, b, n)
	case *kvtx_block.KeyValueStore, *kvtx_block_iavl.Node, *kvtx_block_okra.Root, *kvtx_block_okra.Page:
	default:
		values = nil
	}

	// Queue each ref with the constructor its holder names, or the tree's
	// value constructor for a tree value.
	var out []refRepairNode
	if holder, ok := blk.(block.BlockWithRefs); ok {
		refs, err := holder.GetBlockRefs()
		if err != nil {
			return nil, err
		}
		for _, id := range slices.Sorted(maps.Keys(refs)) {
			ref := refs[id]
			if ref.GetEmpty() {
				continue
			}
			ctor := holder.GetBlockRefCtor(id)
			if ctor == nil {
				ctor = treeValueCtor(blk, id, ref, values)
			}
			if ctor == nil {
				r.res.untyped[reflect.TypeOf(blk).String()+" ref "+strconv.FormatUint(uint64(id), 10)]++
				continue
			}
			out = append(out, refRepairNode{ref: ref, ctor: ctor, xfrm: n.xfrm, world: n.world, values: values})
		}
	}

	// Queue the refs of each inline sub-block. A World's object tree holds
	// Objects; its graph tree and change log hold no typed values.
	if holder, ok := blk.(block.BlockWithSubBlocks); ok {
		subs := holder.GetSubBlocks()
		for _, id := range slices.Sorted(maps.Keys(subs)) {
			sub := subs[id]
			if sub == nil || sub.IsNil() {
				continue
			}
			subValues := values
			if _, ok := blk.(*world_block.World); ok {
				subValues = nil
				if id == 1 {
					subValues = world_block.NewObjectBlock
				}
			}
			subChildren, err := r.children(ctx, sub, n, subValues)
			if err != nil {
				return nil, err
			}
			out = append(out, subChildren...)
		}
	}
	return out, nil
}

// treeValueCtor returns the constructor of a value ref a tree node leaves
// untyped: a blob when an Okra entry marks it so, values otherwise.
func treeValueCtor(blk any, id uint32, ref *block.BlockRef, values block.Ctor) block.Ctor {
	switch b := blk.(type) {
	case *kvtx_block_iavl.Node:
		if id == 7 {
			return values
		}
	case *kvtx_block_okra.Page:
		for _, ent := range b.GetEntries() {
			if ent.GetValueRef() != ref {
				continue
			}
			if ent.GetValueIsBlob() {
				return blob.NewBlobBlock
			}
			return values
		}
	}
	return nil
}

// spanChildren returns the Worlds and payload blobs of a replay span, which
// the Space's World transform encodes.
func (r *refRepair) spanChildren(span *sobject_world_engine.ReplaySpan) ([]refRepairNode, error) {
	// Queue each World of the span.
	out := make([]refRepairNode, 0, len(span.GetWorlds())+len(span.GetPayloads()))
	for _, ref := range span.GetWorlds() {
		node, err := r.worldNode(r.spaceWorld, ref)
		if err != nil {
			return nil, err
		}
		out = append(out, node)
	}

	// Queue each payload blob of the span.
	xfrm, err := r.transformer(r.spaceWorld.GetTransformConf())
	if err != nil {
		return nil, err
	}
	for _, ref := range span.GetPayloads() {
		out = append(out, refRepairNode{ref: ref, ctor: blob.NewBlobBlock, xfrm: xfrm})
	}
	return out, nil
}

// objectRoot returns the root of a World object, decoded by the object's type.
func (r *refRepair) objectRoot(ctx context.Context, o *world_block.Object, n refRepairNode) ([]refRepairNode, error) {
	// A root in another bucket is not a ref of this block, and a root whose
	// transform is itself a block is beyond this walk.
	root := o.GetRootRef()
	if root.GetRootRef().GetEmpty() || root.GetBucketId() != "" {
		return nil, nil
	}
	if !root.GetTransformConfRef().GetEmpty() {
		r.res.untyped["object root with a transform config ref"]++
		return nil, nil
	}

	// Resolve the root's type from the object's type.
	ctor, reason, err := r.objectRootCtor(ctx, o.GetKey(), n.world)
	if err != nil {
		return nil, err
	}
	if ctor == nil {
		r.res.untyped[reason]++
		return nil, nil
	}

	// Decode the root with its own transform when it names one.
	xfrm := n.xfrm
	if conf := root.GetTransformConf(); !conf.GetEmpty() {
		xfrm, err = r.transformer(conf)
		if err != nil {
			return nil, err
		}
	}
	return []refRepairNode{{ref: root.GetRootRef(), ctor: ctor, xfrm: xfrm, world: n.world}}, nil
}

// objectRootCtor returns the root constructor of the object at key from its
// type in world, or nil and the reason to count the root under. Only UnixFS
// object types name a root constructor.
func (r *refRepair) objectRootCtor(ctx context.Context, key string, world *bucket.ObjectRef) (block.Ctor, string, error) {
	// Read the object type, caching it across Worlds.
	typeID, ok := r.types[key]
	if !ok {
		tx, err := r.worldTypes(ctx, world)
		if err != nil {
			r.le.WithError(err).Warn("cannot open world to read object types")
			return nil, "object in an unreadable World", nil
		}
		typeID, err = world_types.GetObjectType(ctx, tx, key)
		if err != nil {
			if ctx.Err() != nil {
				return nil, "", ctx.Err()
			}
			r.le.WithError(err).Warnf("cannot read type of object %s", key)
			return nil, "object with an unreadable type", nil
		}
		if typeID != "" {
			r.types[key] = typeID
		}
	}
	if typeID == "" {
		return nil, "object without a type", nil
	}

	// Map the UnixFS types to their root blocks.
	fsType, err := unixfs_world.TypeIDToFSType(typeID)
	if err != nil {
		return nil, "object type " + typeID, nil
	}
	ctor, _, err := unixfs_world.GetFSRootWithType(fsType)
	return ctor, "", err
}

// worldTypes returns a read transaction on world, reopening the snapshot
// when world changed since the last lookup.
func (r *refRepair) worldTypes(ctx context.Context, world *bucket.ObjectRef) (world.Tx, error) {
	// Reuse the snapshot of the same World.
	if r.typesTx != nil && r.typesWorld.EqualVT(world) {
		return r.typesTx, nil
	}

	// Open a snapshot of the World in place of the last one.
	r.closeTypes()
	engine, err := world_block.OpenSnapshot(ctx, r.le, r.store, world)
	if err != nil {
		return nil, err
	}
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		_ = engine.Close()
		return nil, err
	}
	r.typesWorld, r.typesEngine, r.typesTx = world, engine, tx
	return tx, nil
}

// closeTypes releases the snapshot object types were read from.
func (r *refRepair) closeTypes() {
	if r.typesTx == nil {
		return
	}
	r.typesTx.Discard()
	_ = r.typesEngine.Close()
	r.typesWorld, r.typesEngine, r.typesTx = nil, nil, nil
}

// transformer returns the transformer of conf, built once per config.
func (r *refRepair) transformer(conf *block_transform.Config) (block.Transformer, error) {
	// Reuse a transformer built for the same config.
	data, err := conf.MarshalVT()
	if err != nil {
		return nil, err
	}
	if xfrm, ok := r.xfrms[string(data)]; ok {
		return xfrm, nil
	}

	// Build and cache the transformer.
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: r.le}, transform_all.BuildFactorySet(), conf)
	if err != nil {
		return nil, errors.Wrap(err, "build world transform")
	}
	r.xfrms[string(data)] = xfrm
	return xfrm, nil
}

// rootBucket roots the bucket at bucketIRI under the GC root, as a bucket
// handle with GC tracking does when it opens.
func (r *refRepair) rootBucket(ctx context.Context, bucketIRI string) error {
	// Plan the GC root edge unless the root holds the bucket.
	held, err := r.rg.GetOutgoingRefs(ctx, block_gc.NodeGCRoot)
	if err != nil || slices.Contains(held, bucketIRI) {
		return err
	}
	r.plan(block_gc.NodeGCRoot, bucketIRI)
	r.res.rooted++
	return nil
}

// own gives a present root that nothing holds to the bucket at bucketIRI, as
// a write records a block it owns.
func (r *refRepair) own(ctx context.Context, bucketIRI string, ref *block.BlockRef) error {
	// Skip an empty root and a root a planned edge holds.
	iri := block_gc.BlockIRI(ref)
	if iri == "" {
		return nil
	}
	if _, ok := r.planned[iri]; ok {
		return nil
	}

	// Skip a root the volume lacks or another node holds.
	found, err := r.store.GetBlockExists(ctx, ref)
	if err != nil || !found {
		return err
	}
	held, err := r.rg.HasIncomingRefsExcluding(ctx, iri, block_gc.NodeUnreferenced)
	if err != nil || held {
		return err
	}

	// Plan the bucket's ownership edge.
	r.plan(bucketIRI, iri)
	r.res.owned++
	return nil
}

// plan queues the edge from subject to object.
func (r *refRepair) plan(subject, object string) {
	r.adds = append(r.adds, block_gc.RefEdge{Subject: subject, Object: object})
	r.planned[object] = struct{}{}
}

// apply adds the planned edges in bounded transactions.
func (r *refRepair) apply(ctx context.Context) error {
	for chunk := range slices.Chunk(r.adds, volumeRepairEdgeBatch) {
		if err := r.rg.ApplyRefBatch(ctx, chunk, nil); err != nil {
			return errors.Wrap(err, "add ref edges")
		}
		r.res.written += uint64(len(chunk))
	}
	return nil
}

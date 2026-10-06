//go:build !js

package spacewave_cli

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"strconv"

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
	volume_bolt "github.com/s4wave/spacewave/db/volume/bolt"
	volume_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
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

// refRepairResult counts what one ref repair found and wrote.
type refRepairResult struct {
	// spaces is the number of Spaces walked.
	spaces int
	// blocks is the number of distinct blocks walked.
	blocks uint64
	// absent is the number of referenced blocks the volume lacks.
	absent uint64
	// undecodable is the number of blocks that failed to decode as their type.
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
func newRefRepair(le *logrus.Entry, vol *volume_bolt.Bolt) *refRepair {
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
// resolve their types in one snapshot.
func (r *refRepair) walk(ctx context.Context, root refRepairNode) error {
	stack := []refRepairNode{root}
	for len(stack) != 0 {
		// Stop when the repair is canceled.
		if err := ctx.Err(); err != nil {
			return err
		}

		// Visit the next block and queue its children.
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		children, err := r.visit(ctx, n)
		if err != nil {
			return err
		}
		stack = append(stack, children...)
	}
	return nil
}

// visit decodes one block, plans the edges the graph lacks for it, and returns
// its children.
func (r *refRepair) visit(ctx context.Context, n refRepairNode) ([]refRepairNode, error) {
	// Skip an empty ref and a block already walked.
	iri := block_gc.BlockIRI(n.ref)
	if iri == "" {
		return nil, nil
	}
	if _, ok := r.visited[iri]; ok {
		return nil, nil
	}
	r.visited[iri] = struct{}{}

	// Read the stored bytes, counting a block the volume lacks.
	data, found, err := r.store.GetBlock(ctx, n.ref)
	if err != nil {
		return nil, err
	}
	if !found {
		r.res.absent++
		return nil, nil
	}
	r.res.blocks++

	// Decode the block by its type.
	blk, err := decodeRefRepairNode(n, data)
	if err != nil {
		r.le.WithError(err).Warnf("cannot decode %s", n.ref.MarshalString())
		r.res.undecodable++
		return nil, nil
	}

	// Plan the edges a write records for the block, then return its children.
	refs, err := block.ExtractBlockRefs(blk)
	if err != nil {
		return nil, err
	}
	if span, ok := blk.(*sobject_world_engine.ReplaySpan); ok {
		refs = slices.Concat(span.GetWorlds(), span.GetPayloads())
	}
	if err := r.recordEdges(ctx, iri, refs); err != nil {
		return nil, err
	}
	return r.children(ctx, blk, n, n.values)
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

// recordEdges plans each edge from the block at iri to refs that the graph
// lacks.
func (r *refRepair) recordEdges(ctx context.Context, iri string, refs []*block.BlockRef) error {
	// Read the edges the graph holds for the block.
	if len(refs) == 0 {
		return nil
	}
	have, err := r.rg.GetOutgoingRefs(ctx, iri)
	if err != nil {
		return err
	}

	// Plan each edge it lacks once.
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
	return nil
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
			return nil, "", errors.Wrapf(err, "read type of object %s", key)
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

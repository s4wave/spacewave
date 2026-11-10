// Package refwalk walks the blocks below a World, decoding each by its type,
// and reports each decoded block's refs.
//
// Below an object's root the walk is generic: a block names its children's
// types through GetBlockRefCtor, and a tree passes its value type down. An
// object's root is typed by the object's type ID, which by the contract on
// objecttype.ObjectType.GetObjectTypeID is the type ID of its root block: the
// walker resolves it with a LookupBlockType func, the same lookup a World
// schema and a plugin's body service serve. A type the lookup
// does not know is counted and sampled, and the blocks below it are not
// walked, so whoever reads the result can register the type or retire it.
package refwalk

import (
	"context"
	"maps"
	"reflect"
	"slices"
	"strconv"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/blocktype"
	"github.com/s4wave/spacewave/db/bucket"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_block_iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
	"golang.org/x/sync/errgroup"
)

const (
	// batchSize is the number of blocks taken from the top of the walk's stack
	// and read together.
	batchSize = 64
	// readers bounds the concurrent block reads. The reads are random preads,
	// so a few in flight hide the disk latency.
	readers = 16
)

// LookupBlockType resolves a type ID to its block type, nil when it does not
// know the ID.
type LookupBlockType = func(ctx context.Context, typeID string) (blocktype.BlockType, error)

// Walker walks blocks below Worlds, visiting each block once across walks.
type Walker struct {
	// le logs unreadable Worlds and types.
	le *logrus.Entry
	// store reads the blocks.
	store block.StoreOps
	// lookup types objects' roots.
	lookup LookupBlockType
	// visited holds the keys of the blocks already walked.
	visited map[string]struct{}
	// untyped counts the refs whose type the walk cannot name, by reason.
	untyped map[string]*Untyped
	// types caches object type IDs by object key across Worlds.
	types map[string]string
	// xfrms caches transformers by their marshaled config.
	xfrms map[string]block.Transformer
	// typesWorld is the World typesTx reads, nil before the first lookup.
	typesWorld *bucket.ObjectRef
	// typesEngine is the snapshot engine typesTx belongs to.
	typesEngine *world_block.Engine
	// typesTx reads object types from typesWorld.
	typesTx world.Tx
}

// NewWalker constructs a Walker reading store, typing objects' roots with
// lookup. Close releases it.
func NewWalker(le *logrus.Entry, store block.StoreOps, lookup LookupBlockType) *Walker {
	return &Walker{
		le:      le,
		store:   store,
		lookup:  lookup,
		visited: make(map[string]struct{}),
		untyped: make(map[string]*Untyped),
		types:   make(map[string]string),
		xfrms:   make(map[string]block.Transformer),
	}
}

// Untyped returns the refs whose type the walk could not name, by reason. The
// blocks below them were not walked.
func (w *Walker) Untyped() map[string]Untyped {
	out := make(map[string]Untyped, len(w.untyped))
	for reason, u := range w.untyped {
		out[reason] = *u
	}
	return out
}

// Transformer returns the transformer of conf, built once per config.
func (w *Walker) Transformer(conf *block_transform.Config) (block.Transformer, error) {
	// Reuse a transformer built for the same config.
	data, err := conf.MarshalVT()
	if err != nil {
		return nil, err
	}
	if xfrm, ok := w.xfrms[string(data)]; ok {
		return xfrm, nil
	}

	// Build and cache the transformer.
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: w.le}, transform_all.BuildFactorySet(), conf)
	if err != nil {
		return nil, errors.Wrap(err, "build world transform")
	}
	w.xfrms[string(data)] = xfrm
	return xfrm, nil
}

// WorldNode returns the node of the World at ref, read with the bucket and
// transform of world.
func (w *Walker) WorldNode(world *bucket.ObjectRef, ref *block.BlockRef) (Node, error) {
	xfrm, err := w.Transformer(world.GetTransformConf())
	if err != nil {
		return Node{}, err
	}
	at := &bucket.ObjectRef{
		RootRef:       ref,
		BucketId:      world.GetBucketId(),
		TransformConf: world.GetTransformConf(),
	}
	return Node{Ref: ref, Ctor: world_block.NewWorldBlock, Xfrm: xfrm, World: at}, nil
}

// BlobNode returns the node of a blob encoded with world's transform.
func (w *Walker) BlobNode(world *bucket.ObjectRef, ref *block.BlockRef) (Node, error) {
	xfrm, err := w.Transformer(world.GetTransformConf())
	if err != nil {
		return Node{}, err
	}
	return Node{Ref: ref, Ctor: blob.NewBlobBlock, Xfrm: xfrm}, nil
}

// Walk visits the blocks below root not visited yet, depth first, so the
// objects of one World resolve their types in one snapshot. It reads a batch
// from the top of the stack concurrently, then passes the batch to visit in
// stack order and queues the children of each decoded block.
func (w *Walker) Walk(ctx context.Context, root Node, visit func(ctx context.Context, batch []Visit) error) error {
	stack := []Node{root}
	for len(stack) != 0 {
		// Take the unvisited blocks of the next batch from the top of the stack.
		var batch []Node
		for len(stack) != 0 && len(batch) != batchSize {
			n := stack[len(stack)-1]
			stack = stack[:len(stack)-1]
			if n.Ref.GetEmpty() {
				continue
			}
			key := n.Ref.MarshalString()
			if _, ok := w.visited[key]; ok {
				continue
			}
			w.visited[key] = struct{}{}
			batch = append(batch, n)
		}
		if len(batch) == 0 {
			continue
		}

		// Read the batch concurrently and hand it to visit.
		reads := make([]Visit, len(batch))
		eg, egCtx := errgroup.WithContext(ctx)
		eg.SetLimit(readers)
		for i, n := range batch {
			eg.Go(func() (err error) {
				reads[i], err = w.read(egCtx, n)
				return err
			})
		}
		if err := eg.Wait(); err != nil {
			return err
		}
		if err := visit(ctx, reads); err != nil {
			return err
		}

		// Queue the children of each decoded block.
		for i := range reads {
			if reads[i].blk == nil {
				continue
			}
			children, err := w.children(ctx, reads[i].blk, reads[i].Node, reads[i].Node.values)
			if err != nil {
				return err
			}
			stack = append(stack, children...)
		}
	}
	return nil
}

// Close releases the snapshot object types were read from.
func (w *Walker) Close() {
	if w.typesTx == nil {
		return
	}
	w.typesTx.Discard()
	_ = w.typesEngine.Close()
	w.typesWorld, w.typesEngine, w.typesTx = nil, nil, nil
}

// read reads, decodes and extracts the refs of one block. It touches no walk
// state, so reads run concurrently.
func (w *Walker) read(ctx context.Context, n Node) (Visit, error) {
	// Check only the presence of a block whose type holds no refs.
	rd := Visit{Node: n}
	if !holdsRefs(n.Ctor()) {
		var err error
		rd.Leaf = true
		rd.Found, err = w.store.GetBlockExists(ctx, n.Ref)
		return rd, err
	}

	// Read and decode the stored bytes.
	data, found, err := w.store.GetBlock(ctx, n.Ref)
	if err != nil || !found {
		return rd, err
	}
	rd.Found = true
	if n.Xfrm != nil {
		data, rd.DecodeErr = n.Xfrm.DecodeBlock(data)
	}
	if rd.DecodeErr == nil {
		blk := n.Ctor()
		if rd.DecodeErr = blk.UnmarshalVT(data); rd.DecodeErr == nil {
			rd.blk = blk
		}
	}
	if rd.blk == nil {
		return rd, nil
	}

	// Extract the refs a write records.
	rd.Refs, err = block.ExtractBlockRefs(rd.blk)
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

// children returns the blocks blk references, each with the constructor and
// transform that decode it. values constructs the values of the tree blk
// belongs to, nil when they are untyped.
func (w *Walker) children(ctx context.Context, blk any, n Node, values block.Ctor) ([]Node, error) {
	// An Object names its root's type by its own. Only tree blocks pass the
	// tree's value constructor down.
	switch b := blk.(type) {
	case *world_block.Object:
		return w.objectRoot(ctx, b, n)
	case *kvtx_block.KeyValueStore, *kvtx_block_iavl.Node, *kvtx_block_okra.Root, *kvtx_block_okra.Page:
	default:
		values = nil
	}

	// Queue each ref with the constructor its holder names, or the tree's
	// value constructor for a tree value.
	var out []Node
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
				w.count(reflect.TypeOf(blk).String()+" ref "+strconv.FormatUint(uint64(id), 10), ref)
				continue
			}
			out = append(out, Node{Ref: ref, Ctor: ctor, Xfrm: n.Xfrm, World: n.World, values: values})
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
			subChildren, err := w.children(ctx, sub, n, subValues)
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

// objectRoot returns the root of a World object, typed by the object's type.
func (w *Walker) objectRoot(ctx context.Context, o *world_block.Object, n Node) ([]Node, error) {
	// A root in another bucket is not a ref of this block, and a root whose
	// transform is itself a block is beyond this walk.
	root := o.GetRootRef()
	ref := root.GetRootRef()
	if ref.GetEmpty() || root.GetBucketId() != "" {
		return nil, nil
	}
	if !root.GetTransformConfRef().GetEmpty() {
		w.count("object root with a transform config ref", ref)
		return nil, nil
	}

	// Resolve the root's type from the object's type.
	ctor, reason, err := w.objectRootCtor(ctx, o.GetKey(), n.World)
	if err != nil {
		return nil, err
	}
	if ctor == nil {
		w.count(reason, ref)
		return nil, nil
	}

	// Decode the root with its own transform when it names one.
	xfrm := n.Xfrm
	if conf := root.GetTransformConf(); !conf.GetEmpty() {
		xfrm, err = w.Transformer(conf)
		if err != nil {
			return nil, err
		}
	}
	return []Node{{Ref: ref, Ctor: ctor, Xfrm: xfrm, World: n.World}}, nil
}

// objectRootCtor returns the root constructor of the object at key from its
// type in world, or nil and the reason to count the root under.
func (w *Walker) objectRootCtor(ctx context.Context, key string, world *bucket.ObjectRef) (block.Ctor, string, error) {
	// Read the object type, caching it across Worlds.
	typeID, ok := w.types[key]
	if !ok {
		tx, err := w.worldTypes(ctx, world)
		if err != nil {
			w.le.WithError(err).Warn("cannot open world to read object types")
			return nil, "object in an unreadable World", nil
		}
		typeID, err = world_types.GetObjectType(ctx, tx, key)
		if err != nil {
			if ctx.Err() != nil {
				return nil, "", ctx.Err()
			}
			w.le.WithError(err).Warnf("cannot read type of object %s", key)
			return nil, "object with an unreadable type", nil
		}
		if typeID != "" {
			w.types[key] = typeID
		}
	}
	if typeID == "" {
		return nil, "object without a type", nil
	}

	// The object's type ID names its root block's type.
	bt, err := w.lookup(ctx, typeID)
	if err != nil {
		return nil, "", errors.Wrapf(err, "look up block type %s", typeID)
	}
	if bt == nil {
		return nil, "object type " + typeID, nil
	}
	return bt.Constructor, "", nil
}

// worldTypes returns a read transaction on world, reopening the snapshot
// when world changed since the last lookup.
func (w *Walker) worldTypes(ctx context.Context, world *bucket.ObjectRef) (world.Tx, error) {
	// Reuse the snapshot of the same World.
	if w.typesTx != nil && w.typesWorld.EqualVT(world) {
		return w.typesTx, nil
	}

	// Open a snapshot of the World in place of the last one.
	w.Close()
	engine, err := world_block.OpenSnapshot(ctx, w.le, w.store, world)
	if err != nil {
		return nil, err
	}
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		_ = engine.Close()
		return nil, err
	}
	w.typesWorld, w.typesEngine, w.typesTx = world, engine, tx
	return tx, nil
}

// count records a ref whose type the walk cannot name.
func (w *Walker) count(reason string, ref *block.BlockRef) {
	u := w.untyped[reason]
	if u == nil {
		u = &Untyped{Sample: ref}
		w.untyped[reason] = u
	}
	u.Count++
}

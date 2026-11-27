// Package refwalk walks the blocks below a World, decoding each by its type,
// and reports each decoded block's refs.
//
// Below an object's root the walk is generic: a block declares its children's
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

// Walker walks blocks below Worlds, visiting each block once per type it is
// read as across walks.
type Walker struct {
	// le logs unreadable Worlds and types.
	le *logrus.Entry
	// store reads the blocks.
	store block.StoreOps
	// lookup types objects' roots.
	lookup LookupBlockType
	// visited holds the blocks already walked, each by its ref and the type
	// it was read as, so a block read as one type is still read as another.
	visited map[visitKey]struct{}
	// readAs holds every type the walk read a block as.
	readAs map[reflect.Type]struct{}
	// roots holds the refs the walk typed as object roots, by their keys,
	// whether or not their type resolved.
	roots map[string]struct{}
	// untyped counts the refs whose type the walk cannot name, by reason.
	untyped map[string]*Untyped
	// stops holds the reason of each such ref, by the ref's string.
	stops map[string]string
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
	// keepBytes reads every block's bytes, leaves included, into its Visit.
	keepBytes bool
	// scan walks below a ref whose type the walk cannot name by scanning each
	// block's bytes, and scans each decoded block beside its typed refs.
	scan bool
}

// NewWalker constructs a Walker reading store, typing objects' roots with
// lookup. Close releases it.
func NewWalker(le *logrus.Entry, store block.StoreOps, lookup LookupBlockType) *Walker {
	return &Walker{
		le:      le,
		store:   store,
		lookup:  lookup,
		visited: make(map[visitKey]struct{}),
		readAs:  make(map[reflect.Type]struct{}),
		roots:   make(map[string]struct{}),
		untyped: make(map[string]*Untyped),
		stops:   make(map[string]string),
		types:   make(map[string]string),
		xfrms:   make(map[string]block.Transformer),
	}
}

// KeepBytes makes the walk read every block's stored bytes, leaves included,
// into Visit.Data, as a caller that rewrites each block needs. Call it before
// the first Walk.
func (w *Walker) KeepBytes() {
	w.keepBytes = true
}

// ScanUntyped makes the walk go on below a ref whose type it cannot name:
// it reads that block and the blocks below it with ScanHits, each as a Node
// whose Untyped gives the reason, decoded with its holder's transform. It
// also scans every block it decodes by type and reports in Visit.ScanMissed
// the typed refs the scan did not find. This comparison measures the scan
// against the types. Call it before the first Walk.
func (w *Walker) ScanUntyped() {
	w.scan = true
}

// ScannedOnly returns how many blocks the walk read by scanning alone and
// under no type. It looks each scanned block up under every type the walk
// read, so it costs the scanned blocks times those types.
func (w *Walker) ScannedOnly() uint64 {
	var n uint64
	for key := range w.visited {
		if key.typ == nil && !w.typed(key.ref) {
			n++
		}
	}
	return n
}

// Reached reports whether the walk visited ref under a type and whether it
// visited it by scanning. A visit is not a read: the block may be absent or
// fail to decode. It still answers after Close.
func (w *Walker) Reached(ref *block.BlockRef) (typed, scanned bool) {
	key := ref.MarshalString()
	_, scanned = w.visited[visitKey{ref: key}]
	return w.typed(key), scanned
}

// typed reports whether the walk visited the ref of key under a type.
func (w *Walker) typed(key string) bool {
	for typ := range w.readAs {
		if _, ok := w.visited[visitKey{ref: key, typ: typ}]; ok {
			return true
		}
	}
	return false
}

// Untyped returns the refs whose type the walk could not name, by reason. The
// blocks below them were not walked, unless the walk scans.
func (w *Walker) Untyped() map[string]Untyped {
	out := make(map[string]Untyped, len(w.untyped))
	for reason, u := range w.untyped {
		out[reason] = *u
	}
	return out
}

// UntypedReason returns why the walk could not name the type of ref and so
// did not walk below it, or "" when it stopped at no such ref. It still
// answers after Close.
func (w *Walker) UntypedReason(ref *block.BlockRef) string {
	return w.stops[ref.MarshalString()]
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

// PayloadNode returns the node of an operation's payload root, encoded with
// world's transform, and false for one the walk already typed. A payload root
// is an object root the operation set, referenced by the resulting World, or
// the payload of a world.PayloadOperation, which is a blob: so once the Worlds
// that hold an operation's outcome are walked, a payload the walk typed as an
// object root is walked under its object's type, and the rest are blobs. An
// object root that no walked World references, such as one a later transaction
// in its batch replaced, is read as a blob too, so the node is a guess, which
// the walker keeps only when the block is certainly a blob.
func (w *Walker) PayloadNode(world *bucket.ObjectRef, ref *block.BlockRef) (Node, bool, error) {
	if _, ok := w.roots[ref.MarshalString()]; ok {
		return Node{}, false, nil
	}
	n, err := w.BlobNode(world, ref)
	n.Guess = true
	return n, err == nil, err
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
			key := visitKey{ref: n.Ref.MarshalString()}
			if n.Untyped == "" {
				key.typ = reflect.TypeOf(n.Ctor())
				w.readAs[key.typ] = struct{}{}
			}
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

		// Queue the children of each decoded block, and of each scanned one
		// the refs its bytes hold, untyped like it. A scanning walk scans a
		// payload root it could not confirm as a blob.
		for i := range reads {
			n := reads[i].Node
			if n.Untyped != "" {
				for _, ref := range reads[i].Refs {
					stack = append(stack, Node{Ref: ref, Untyped: n.Untyped, Xfrm: n.Xfrm, World: n.World})
				}
				continue
			}
			if reads[i].Unsure && w.scan {
				stack = append(stack, Node{Ref: n.Ref, Untyped: "payload root that is not certainly a blob", Xfrm: n.Xfrm, World: n.World})
			}
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

// read reads, decodes and extracts the refs of one block. For a leaf it reads
// only presence, or the bytes when the walker keeps them. It touches no walk
// state, so reads run concurrently.
func (w *Walker) read(ctx context.Context, n Node) (Visit, error) {
	// Scan a block the walk has no type for.
	if n.Untyped != "" {
		return w.readUntyped(ctx, n)
	}

	// Check only the presence of a block whose type holds no refs, unless
	// the walker keeps bytes.
	rd := Visit{Node: n, Leaf: !holdsRefs(n.Ctor())}
	if rd.Leaf && !w.keepBytes {
		var err error
		rd.Found, err = w.store.GetBlockExists(ctx, n.Ref)
		return rd, err
	}

	// Read the stored bytes, keeping a copy the decode cannot overwrite.
	data, found, err := w.store.GetBlock(ctx, n.Ref)
	if err != nil || !found {
		return rd, err
	}
	rd.Found = true
	if w.keepBytes {
		rd.Data = data
		if n.Xfrm != nil {
			data = slices.Clone(data)
		}
	}
	if rd.Leaf {
		return rd, nil
	}

	// Decode the block as its type.
	if n.Xfrm != nil {
		data, rd.DecodeErr = n.Xfrm.DecodeBlock(data)
	}
	if rd.DecodeErr == nil {
		blk := n.Ctor()
		if rd.DecodeErr = blk.UnmarshalVT(data); rd.DecodeErr == nil {
			rd.blk = blk
		}
	}
	if n.Guess && !certain(rd.blk, data) {
		rd.Unsure, rd.blk = true, nil
	}
	if rd.blk == nil {
		return rd, nil
	}

	// Extract the refs a write records, and on a scanning walk the ones a
	// scan of the same bytes does not find.
	rd.Refs, err = block.ExtractBlockRefs(rd.blk)
	if err != nil || !w.scan {
		return rd, err
	}
	rd.ScanMissed, rd.ScanExtra = scanCompare(rd.Refs, data)
	if len(rd.ScanMissed) != 0 {
		rd.ScanShape = ScanShape(data)
	}
	return rd, nil
}

// readUntyped reads one block the walk has no type for and scans its bytes
// for refs. Bytes that are not a message hold none. It keeps the stored bytes
// when the walker keeps them.
func (w *Walker) readUntyped(ctx context.Context, n Node) (Visit, error) {
	// Read the stored bytes and decode them as the holder's were.
	rd := Visit{Node: n}
	data, found, err := w.store.GetBlock(ctx, n.Ref)
	if err != nil || !found {
		return rd, err
	}
	rd.Found = true
	if w.keepBytes {
		rd.Data = data
		if n.Xfrm != nil {
			data = slices.Clone(data)
		}
	}
	if n.Xfrm != nil {
		if data, rd.DecodeErr = n.Xfrm.DecodeBlock(data); rd.DecodeErr != nil {
			return rd, nil
		}
	}

	// Scan them.
	var ok bool
	rd.ScanHits, ok = ScanHits(data)
	rd.Refs = make([]*block.BlockRef, len(rd.ScanHits))
	for i, hit := range rd.ScanHits {
		rd.Refs[i] = hit.Ref
	}
	rd.Opaque = !ok
	return rd, nil
}

// certain reports whether blk, decoded from data under a guessed type, is as
// sure a Blob as its bytes allow: a Blob that validates, that is not an empty
// blob read from non-empty bytes, and that holds only the fields its types
// define. Any other type, or no block, is never certain. Bytes whose wire form
// is a valid Blob's still pass, such as a block of another type whose fields 1,
// 2 and 3 read as a raw blob's type, size and data. A stored child list bounds
// that case, since the merge never shrinks it: a block whose own writer listed
// its refs keeps every child that list records, and a decode that returns fewer
// refs is reported. A block with no stored list gets one from the guessed refs.
func certain(blk any, data []byte) bool {
	b, ok := blk.(*blob.Blob)
	if !ok || b.Validate() != nil || b.IsEmpty() && len(data) != 0 {
		return false
	}
	return b.DefinedFieldsOnly()
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
	// An Object declares its root's type through its own type. Only tree
	// blocks pass the tree's value constructor down.
	switch b := blk.(type) {
	case *world_block.Object:
		return w.objectRoot(ctx, b, n)
	case *kvtx_block.KeyValueStore, *kvtx_block_iavl.Node, *kvtx_block_okra.Root, *kvtx_block_okra.Page:
	default:
		values = nil
	}

	// Queue each ref with the constructor its holder declares, or the
	// tree's value constructor for a tree value.
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
				ctor = kvtx_block.TreeValueCtor(blk, id, values)
			}
			if ctor == nil {
				reason := reflect.TypeOf(blk).String() + " ref " + strconv.FormatUint(uint64(id), 10)
				w.count(reason, ref)
				if w.scan {
					out = append(out, Node{Ref: ref, Untyped: reason, Xfrm: n.Xfrm, World: n.World})
				}
				continue
			}
			out = append(out, Node{Ref: ref, Ctor: ctor, Xfrm: n.Xfrm, World: n.World, values: values})
		}
	}

	// Queue the refs of each inline sub-block. A World's object tree holds
	// Objects. Its graph tree and change log hold no typed values.
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

// objectRoot returns the root of a World object, typed by the object's type.
func (w *Walker) objectRoot(ctx context.Context, o *world_block.Object, n Node) ([]Node, error) {
	// A root in another bucket is not a ref of this block, and a root whose
	// transform is itself a block is beyond this walk.
	root := o.GetRootRef()
	ref := root.GetRootRef()
	if ref.GetEmpty() || root.GetBucketId() != "" {
		return nil, nil
	}
	w.roots[ref.MarshalString()] = struct{}{}
	if !root.GetTransformConfRef().GetEmpty() {
		w.count("object root with a transform config ref", ref)
		return nil, nil
	}

	// Resolve the root's type from the object's type. Only a scanning walk
	// goes on below a root it cannot type.
	ctor, reason, err := w.objectRootCtor(ctx, o.GetKey(), n.World)
	if err != nil {
		return nil, err
	}
	if ctor == nil {
		w.count(reason, ref)
		if !w.scan {
			return nil, nil
		}
	}

	// Decode the root with its own transform when the root contains one.
	xfrm := n.Xfrm
	if conf := root.GetTransformConf(); !conf.GetEmpty() {
		xfrm, err = w.Transformer(conf)
		if err != nil {
			return nil, err
		}
	}

	// Read a root with no type by its bytes.
	if ctor == nil {
		return []Node{{Ref: ref, Untyped: reason, Xfrm: xfrm, World: n.World}}, nil
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

	// The object's type ID identifies its root block's type.
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
	w.stops[ref.MarshalString()] = reason
}

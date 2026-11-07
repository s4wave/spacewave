// Package packcopy_world types the blocks of a World for a pack copy, so the
// copy learns the refs of a block whose store does not know them. It follows
// the World's own types: the World block, its object tree and graph, each
// Object, and each Object's root by the type the World's graph gives it.
package packcopy_world

import (
	"context"
	"maps"
	"slices"
	"sync"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/blob"
	"github.com/s4wave/spacewave/db/block/packcopy"
	block_transform "github.com/s4wave/spacewave/db/block/transform"
	transform_all "github.com/s4wave/spacewave/db/block/transform/all"
	"github.com/s4wave/spacewave/db/bucket"
	kvtx_block "github.com/s4wave/spacewave/db/kvtx/block"
	kvtx_block_iavl "github.com/s4wave/spacewave/db/kvtx/block/iavl"
	kvtx_block_okra "github.com/s4wave/spacewave/db/kvtx/block/okra"
	world_block "github.com/s4wave/spacewave/db/world/block"
	world_types "github.com/s4wave/spacewave/db/world/types"
	"github.com/sirupsen/logrus"
)

// RootTypes maps an object type ID to the constructor of its root block.
type RootTypes map[string]block.Ctor

// Typer types the blocks under one World head. It is safe for concurrent use.
type Typer struct {
	// le logs what the typer cannot type.
	le *logrus.Entry
	// worlds holds the type of each World root by marshaled ref.
	worlds map[string]*nodeType
	// objects holds each object's type ID by key.
	objects map[string]string
	// roots maps object types to their root constructors.
	roots RootTypes

	// mtx guards xfrms and untyped.
	mtx sync.Mutex
	// xfrms caches transformers by marshaled config.
	xfrms map[string]block.Transformer
	// untyped counts the sightings of refs left without a type, by reason. A
	// block reached from several holders counts once per holder.
	untyped map[string]uint64
}

// nodeType is the type of one block: what constructs it, what decodes its
// bytes, and the type of the values of the tree it belongs to.
type nodeType struct {
	// ctor constructs the block.
	ctor block.Ctor
	// xfrm decodes the stored bytes.
	xfrm block.Transformer
	// values constructs the values of the tree, nil for none.
	values block.Ctor
}

// New builds a Typer for the World roots in heads, the head first and then
// the earlier roots it retains. It reads the type of every object of a type in
// roots from a snapshot of the head read through store; an object only an
// earlier root holds has no listed type.
func New(ctx context.Context, le *logrus.Entry, store block.StoreOps, heads []*bucket.ObjectRef, roots RootTypes) (*Typer, error) {
	if len(heads) == 0 {
		return nil, errors.New("no World head")
	}
	t := &Typer{
		le:      le,
		worlds:  make(map[string]*nodeType, len(heads)),
		objects: make(map[string]string),
		roots:   roots,
		xfrms:   make(map[string]block.Transformer),
		untyped: make(map[string]uint64),
	}
	for _, h := range heads {
		if h.GetRootRef().GetEmpty() {
			continue
		}
		xfrm, err := t.transformer(h.GetTransformConf())
		if err != nil {
			return nil, err
		}
		t.worlds[h.GetRootRef().MarshalString()] = &nodeType{ctor: world_block.NewWorldBlock, xfrm: xfrm}
	}
	head := heads[0]

	// List the objects of each type from a snapshot of the head.
	engine, err := world_block.OpenSnapshot(ctx, le, store, head)
	if err != nil {
		return nil, errors.Wrap(err, "open world snapshot")
	}
	defer engine.Close()
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		return nil, err
	}
	defer tx.Discard()
	// A type whose listing meets a block no source holds keeps the objects it
	// listed; the rest stay untyped, and the copy leaves their roots as holes.
	var failed []string
	for _, typeID := range slices.Sorted(maps.Keys(roots)) {
		err := world_types.IterateObjectsWithType(ctx, tx, typeID, func(key string) (bool, error) {
			t.objects[key] = typeID
			return true, nil
		})
		if err != nil {
			if ctx.Err() != nil {
				return nil, context.Cause(ctx)
			}
			le.WithError(err).WithField("type", typeID).Warn("list objects of type")
			failed = append(failed, typeID)
		}
	}
	le.WithField("objects", len(t.objects)).WithField("types", len(roots)).WithField("types-failed", len(failed)).Info("read the world's object types")
	return t, nil
}

// Untyped returns the counts of refs left untyped, by reason.
func (t *Typer) Untyped() map[string]uint64 {
	t.mtx.Lock()
	defer t.mtx.Unlock()
	return maps.Clone(t.untyped)
}

// RootType returns the World's type for a World root, nil for another root.
func (t *Typer) RootType(ref *block.BlockRef) any {
	if nt, ok := t.worlds[ref.MarshalString()]; ok {
		return nt
	}
	return nil
}

// Decode decodes a block by its type and returns its refs, typed where the
// block names its children's types.
func (t *Typer) Decode(ctx context.Context, typ any, ref *block.BlockRef, data []byte) ([]packcopy.TypedRef, bool, error) {
	nt, ok := typ.(*nodeType)
	if !ok {
		return nil, false, nil
	}

	// A type that cannot hold refs is a leaf.
	if !holdsRefs(nt.ctor()) {
		return nil, true, nil
	}

	// Decode the bytes by the type.
	if nt.xfrm != nil {
		var err error
		if data, err = nt.xfrm.DecodeBlock(data); err != nil {
			t.count("undecodable bytes")
			return nil, false, nil
		}
	}
	blk := nt.ctor()
	if err := blk.UnmarshalVT(data); err != nil {
		t.count("undecodable block")
		return nil, false, nil
	}

	// Take the refs a write records, then type each from the block.
	refs, err := block.ExtractBlockRefs(blk)
	if err != nil {
		t.count("refs not extracted")
		return nil, false, nil
	}
	kids, err := t.children(blk, nt, nt.values)
	if err != nil {
		return nil, false, err
	}
	types := make(map[string]any, len(kids))
	for _, kid := range kids {
		types[kid.Ref.MarshalString()] = kid.Type
	}
	out := make([]packcopy.TypedRef, 0, len(refs)+len(kids))
	seen := make(map[string]struct{}, len(refs))
	for _, r := range refs {
		k := r.MarshalString()
		seen[k] = struct{}{}
		out = append(out, packcopy.TypedRef{Ref: r, Type: types[k]})
	}
	for _, kid := range kids {
		if _, ok := seen[kid.Ref.MarshalString()]; !ok {
			out = append(out, kid)
		}
	}
	return out, true, nil
}

// holdsRefs reports whether a block of blk's type can reference other blocks.
func holdsRefs(blk any) bool {
	switch blk.(type) {
	case block.BlockWithRefs, block.BlockWithSubBlocks:
		return true
	}
	return false
}

// children returns the typed children of a decoded block.
func (t *Typer) children(blk any, nt *nodeType, values block.Ctor) ([]packcopy.TypedRef, error) {
	// Objects name their roots' types through the World graph. Only tree
	// blocks pass the tree's value constructor down.
	switch b := blk.(type) {
	case *world_block.Object:
		return t.objectRoot(b, nt)
	case *kvtx_block.KeyValueStore, *kvtx_block_iavl.Node, *kvtx_block_okra.Root, *kvtx_block_okra.Page:
	default:
		values = nil
	}

	// Type each ref by the constructor its holder names, or the tree's value
	// constructor for a tree value.
	var out []packcopy.TypedRef
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
				t.count("untyped ref")
				out = append(out, packcopy.TypedRef{Ref: ref})
				continue
			}
			out = append(out, packcopy.TypedRef{Ref: ref, Type: &nodeType{ctor: ctor, xfrm: nt.xfrm, values: values}})
		}
	}

	// Type the refs of each inline sub-block. A World's object tree holds
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
			kids, err := t.children(sub, nt, subValues)
			if err != nil {
				return nil, err
			}
			out = append(out, kids...)
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

// objectRoot returns an Object's root, typed by the object's type.
func (t *Typer) objectRoot(o *world_block.Object, nt *nodeType) ([]packcopy.TypedRef, error) {
	// A root in another bucket is not a ref of this block.
	root := o.GetRootRef()
	if root.GetRootRef().GetEmpty() || root.GetBucketId() != "" {
		return nil, nil
	}
	ref := root.GetRootRef()
	if !root.GetTransformConfRef().GetEmpty() {
		t.count("object root with a transform config ref")
		return []packcopy.TypedRef{{Ref: ref}}, nil
	}

	// Type the root by the object's type.
	typeID, ok := t.objects[o.GetKey()]
	if !ok {
		t.count("object without a listed type")
		return []packcopy.TypedRef{{Ref: ref}}, nil
	}
	ctor := t.roots[typeID]
	if ctor == nil {
		t.count("object type " + typeID)
		return []packcopy.TypedRef{{Ref: ref}}, nil
	}

	// Decode the root with its own transform when it names one.
	xfrm := nt.xfrm
	if conf := root.GetTransformConf(); !conf.GetEmpty() {
		var err error
		if xfrm, err = t.transformer(conf); err != nil {
			return nil, err
		}
	}
	return []packcopy.TypedRef{{Ref: ref, Type: &nodeType{ctor: ctor, xfrm: xfrm}}}, nil
}

// transformer returns the transformer for conf, built once per config.
func (t *Typer) transformer(conf *block_transform.Config) (block.Transformer, error) {
	data, err := conf.MarshalVT()
	if err != nil {
		return nil, err
	}
	t.mtx.Lock()
	defer t.mtx.Unlock()
	if xfrm, ok := t.xfrms[string(data)]; ok {
		return xfrm, nil
	}
	xfrm, err := block_transform.NewTransformer(controller.ConstructOpts{Logger: t.le}, transform_all.BuildFactorySet(), conf)
	if err != nil {
		return nil, errors.Wrap(err, "build world transform")
	}
	t.xfrms[string(data)] = xfrm
	return xfrm, nil
}

// count counts one ref or block left untyped.
func (t *Typer) count(reason string) {
	t.mtx.Lock()
	t.untyped[reason]++
	t.mtx.Unlock()
}

// _ is a type assertion.
var _ packcopy.Typer = (*Typer)(nil)

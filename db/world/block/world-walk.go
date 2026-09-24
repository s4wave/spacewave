package world_block

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_types "github.com/s4wave/spacewave/db/world/types"
)

// WalkBlocksOptions retains immutable subtrees separately for each decoding
// domain. Known is valid only while the corresponding bytes remain durable.
// Complete runs after every dependency in that domain has been visited.
type WalkBlocksOptions struct {
	Known    func(domain string, ref *block.BlockRef) (bool, error)
	Complete func(domain string, ref *block.BlockRef) error
}

// WalkBlocks visits World metadata and each current object's complete block graph.
// resolve supplies dynamic object decoders; an unknown type prevents completion.
// visit receives encoded bytes and outgoing dependencies after their descendants.
// Callbacks are sequential. Completion records prune unchanged Merkle branches.
// The caller serializes World access and keeps retained bytes through later walks.
func (t *WorldState) WalkBlocks(ctx context.Context, resolve func(context.Context, string) (block.Ctor, error), visit func(*block.BlockRef, []byte, []*block.BlockRef) error, options ...*WalkBlocksOptions) error {
	var opts *WalkBlocksOptions
	if len(options) != 0 {
		opts = options[0]
	}
	var walk func(*bucket_lookup.WalkObjectBlocksEntry, block.StoreOps, block.Transformer, string) error
	walk = func(root *bucket_lookup.WalkObjectBlocksEntry, store block.StoreOps, xfrm block.Transformer, domain string) error {
		// Dynamic object bodies can name another bucket. Once copied, their
		// local bytes are dependencies of the copied object record as well.
		dynamicRefs := make(map[*bucket_lookup.WalkObjectBlocksEntry][]*block.BlockRef)
		return bucket_lookup.WalkObjectGraph(ctx, root, func(entry *bucket_lookup.WalkObjectBlocksEntry) (bool, error) {
			if entry.Err != nil {
				return false, entry.Err
			}
			if entry.IsSubBlock || entry.Ref.GetEmpty() {
				return true, nil
			}
			if !entry.Found {
				return false, errors.Wrap(block.ErrNotFound, entry.Ref.MarshalString())
			}
			if opts != nil && opts.Known != nil {
				known, err := opts.Known(domain, entry.Ref)
				if err != nil || known {
					return false, err
				}
			}
			if domain != "objects" || entry.Ctor != nil {
				return true, nil
			}

			// Object-index leaves are opaque to the generic key/value decoder.
			// Resolve their complete payload here, independent of the changelog.
			data := entry.Data
			if xfrm != nil {
				var err error
				data, err = xfrm.DecodeBlock(data)
				if err != nil {
					return false, err
				}
			}
			objectBlock := &Object{}
			if err := objectBlock.UnmarshalVT(data); err != nil {
				return false, err
			}
			object, found, err := t.GetObject(ctx, objectBlock.GetKey())
			defer world.ReleaseObjectState(object)
			if err != nil {
				return false, err
			}
			if !found {
				return false, world.ErrObjectNotFound
			}
			objectRef, _, err := object.GetRootRef(ctx)
			if err != nil || objectRef.GetRootRef().GetEmpty() {
				return err == nil, err
			}
			typeID, err := world_types.GetObjectType(ctx, t, object.GetKey())
			if err != nil {
				return false, err
			}
			if typeID == "" {
				return false, errors.Errorf("object %s has a body but no type", object.GetKey())
			}
			ctor, err := resolve(ctx, typeID)
			if err != nil {
				return false, err
			}
			if ctor == nil {
				return false, errors.Errorf("block decoder unavailable for object %s (%s)", object.GetKey(), typeID)
			}
			err = object.AccessWorldState(ctx, nil, func(cursor *bucket_lookup.Cursor) error {
				return walk(bucket_lookup.NewWalkObjectBlocksWithRef(objectRef.GetRootRef(), ctor), cursor.GetBucket(), cursor.GetTransformer(), "type/"+typeID)
			})
			if err == nil {
				dynamicRefs[entry] = []*block.BlockRef{objectRef.GetRootRef(), objectRef.GetTransformConfRef()}
			}
			return err == nil, err
		}, func(entry *bucket_lookup.WalkObjectBlocksEntry) error {
			if entry.IsSubBlock || entry.Ref.GetEmpty() {
				return nil
			}
			refs, err := block.ExtractBlockRefs(entry.Blk)
			if err != nil {
				return err
			}
			refs = append(refs, dynamicRefs[entry]...)
			delete(dynamicRefs, entry)
			if err := visit(entry.Ref, entry.Data, refs); err != nil {
				return err
			}
			if opts != nil && opts.Complete != nil {
				return opts.Complete(domain, entry.Ref)
			}
			return nil
		}, store, xfrm)
	}

	// Metadata and dynamic object graphs have distinct completion domains: a
	// copied opaque value does not prove that its typed descendants were copied.
	root, err := t.GetRoot(ctx)
	if err != nil {
		return err
	}
	if err := walk(bucket_lookup.NewWalkObjectBlocksWithSubBlock(root.GetObjectKeyValue()), t.store, t.xfrm, "objects"); err != nil {
		return err
	}
	return walk(bucket_lookup.NewWalkObjectBlocksWithRef(t.GetRootRef(), NewWorldBlock), t.store, t.xfrm, "metadata")
}

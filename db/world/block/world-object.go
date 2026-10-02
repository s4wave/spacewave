package world_block

import (
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (t *WorldState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	// Preserve the interface's nil result when the concrete object is absent.
	val, ok, err := t.getObject(ctx, key)
	if val == nil {
		return nil, ok, err
	}
	return val, ok, err
}

// getObject looks up an object by key.
// Returns nil, false if not found.
func (t *WorldState) getObject(ctx context.Context, key string) (*ObjectState, bool, error) {
	// Reject a discarded transaction before accessing its object tree.
	if t.discarded.Load() {
		return nil, false, tx.ErrDiscarded
	}

	// Resolve the object's stored metadata through the existing tree cursor.
	ot := t.objTree
	k := []byte(objectKeyPrefix + key)
	bcs, err := ot.GetCursorAtKey(ctx, k)
	if err != nil || bcs == nil {
		return nil, false, err
	}
	ost, err := NewObjectState(ctx, t, bcs)
	if err != nil {
		return nil, false, err
	}
	return ost, true, nil
}

// mustGetObject returns an error if not found.
func (t *WorldState) mustGetObject(ctx context.Context, key string) (*ObjectState, error) {
	// Turn an absent object into the required-object contract's error.
	obj, found, err := t.getObject(ctx, key)
	if err == nil && !found {
		err = world.ErrObjectNotFound
	}
	if err != nil {
		return nil, err
	}
	return obj, nil
}

// IterateObjects returns an iterator with the given object key prefix.
// The prefix is NOT clipped from the output keys.
// Keys are returned in sorted order.
// Must call Next() or Seek() before valid.
// Call Close when done with the iterator.
// Any init errors will be available via the iterator's Err() method.
func (t *WorldState) IterateObjects(ctx context.Context, prefix string, reversed bool) world.ObjectIterator {
	return NewObjectIterator(t, ctx, prefix, reversed)
}

// CreateObject creates an object with a key and initial root ref.
// Returns ErrObjectExists if the object already exists.
// Appends an OBJECT_SET change to the changelog.
func (t *WorldState) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	// Require an active writer before changing the object index.
	if !t.write {
		return nil, tx.ErrNotWrite
	}
	if t.discarded.Load() {
		return nil, tx.ErrDiscarded
	}

	// Reject an existing key before constructing new object metadata.
	ot := t.objTree
	k := []byte(objectKeyPrefix + key)
	exists, err := ot.Exists(ctx, k)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, world.ErrObjectExists
	}

	// Insert the initial object and retain its cursor for the change record.
	obj := NewObject(key, t.localObjectRef(rootRef))
	nbcs := t.bcs.Detach(false)
	nbcs.ClearAllRefs()
	nbcs.SetBlock(obj, true)
	err = t.objTree.SetCursorAtKey(ctx, k, nbcs, false)
	if err != nil {
		return nil, err
	}
	objState, err := NewObjectState(ctx, t, nbcs)
	if err != nil {
		return nil, err
	}

	// Record the new object's exact metadata in the World changelog.
	changeBcs, err := t.queueWorldChange(ctx, &WorldChange{
		Key:        key,
		ChangeType: WorldChangeType_WorldChange_OBJECT_SET,
	})
	if err != nil {
		return nil, err
	}
	if changeBcs != nil {
		changeBcs.SetRef(5, nbcs)
	}

	// The object now exists for the rest of the transaction.
	t.markObjectExists(key)

	return objState, nil
}

// RenameObject renames an object key and updates associated graph quads.
func (t *WorldState) RenameObject(ctx context.Context, oldKey, newKey string, descendants bool) (world.ObjectState, error) {
	// Require an active writer and two nonempty object keys.
	if !t.write {
		return nil, tx.ErrNotWrite
	}
	if t.discarded.Load() {
		return nil, tx.ErrDiscarded
	}
	if oldKey == "" || newKey == "" {
		return nil, world.ErrEmptyObjectKey
	}

	// Dispatch the requested rename scope after validating the shared inputs.
	if descendants {
		return t.renameObjectDescendants(ctx, oldKey, newKey)
	}

	return t.renameObjectSingle(ctx, oldKey, newKey)
}

// renameObjectSingle moves one object's metadata and its incident relationships.
func (t *WorldState) renameObjectSingle(ctx context.Context, oldKey, newKey string) (world.ObjectState, error) {
	// Resolve the source and preserve an existing object on an identity rename.
	oldObj, found, err := t.getObject(ctx, oldKey)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}
	if oldKey == newKey {
		return oldObj, nil
	}

	// Reject destination collisions before making a new index entry.
	ot := t.objTree
	newTreeKey := []byte(objectKeyPrefix + newKey)
	exists, err := ot.Exists(ctx, newTreeKey)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, world.ErrObjectExists
	}

	// Copy the metadata under the new key without changing its revision.
	oldRoot, err := oldObj.GetRoot(ctx)
	if err != nil {
		return nil, err
	}
	newRoot := oldRoot.Clone()
	newRoot.Key = newKey

	// Install the replacement entry so rewritten graph endpoints can resolve it.
	newBcs := t.bcs.Detach(false)
	newBcs.ClearAllRefs()
	newBcs.SetBlock(newRoot, true)
	if err := ot.SetCursorAtKey(ctx, newTreeKey, newBcs, false); err != nil {
		return nil, err
	}

	// Rewrite relationships before removing their original object entry.
	if err := t.renameGraphObject(ctx, oldKey, newKey); err != nil {
		return nil, err
	}

	// Remove the original entry after every incident relationship was rewritten.
	oldTreeKey := []byte(objectKeyPrefix + oldKey)
	if err := ot.Delete(ctx, oldTreeKey); err != nil {
		return nil, err
	}

	// The old key no longer resolves; drop any stale object memo entry.
	t.forgetObject(oldKey)

	// Record both object revisions and the changed key in one rename entry.
	changeBcs, err := t.queueWorldChange(ctx, &WorldChange{
		Key:        oldKey,
		NewKey:     newKey,
		ChangeType: WorldChangeType_WorldChange_OBJECT_RENAME,
	})
	if err != nil {
		return nil, err
	}
	if changeBcs != nil {
		changeBcs.SetRef(5, newBcs)
		changeBcs.SetRef(6, oldObj.bcs)
	}

	return NewObjectState(ctx, t, newBcs)
}

// renameObjectDescendants moves a complete object-key subtree after preflight.
func (t *WorldState) renameObjectDescendants(ctx context.Context, oldKey, newKey string) (world.ObjectState, error) {
	// Reject an absent source and moving an object underneath itself.
	if oldKey == newKey {
		return t.renameObjectSingle(ctx, oldKey, newKey)
	}
	if strings.HasPrefix(newKey, oldKey+"/") {
		return nil, world.ErrObjectExists
	}
	obj, found, err := t.getObject(ctx, oldKey)
	world.ReleaseObjectState(obj)
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, world.ErrObjectNotFound
	}

	// Resolve every destination collision before changing the subtree.
	renames, err := t.collectObjectRenames(ctx, oldKey, newKey)
	if err != nil {
		return nil, err
	}
	if err := t.checkObjectRenameCollisions(ctx, renames); err != nil {
		return nil, err
	}

	// Retain the renamed parent while applying its ordered descendant changes.
	out, err := t.renameObjectSingle(ctx, oldKey, newKey)
	if err != nil {
		world.ReleaseObjectState(out)
		return nil, err
	}
	for _, rename := range renames[1:] {
		obj, err := t.renameObjectSingle(ctx, rename.oldKey, rename.newKey)
		world.ReleaseObjectState(obj)
		if err != nil {
			world.ReleaseObjectState(out)
			return nil, err
		}
	}
	return out, nil
}

// collectObjectRenames lists the parent and descendants in parent-first order.
func (t *WorldState) collectObjectRenames(ctx context.Context, oldKey, newKey string) ([]objectRename, error) {
	// Read only the source subtree and retain each exact destination key.
	renames := []objectRename{{oldKey: oldKey, newKey: newKey}}
	iter := t.IterateObjects(ctx, oldKey+"/", false)
	defer iter.Close()
	for iter.Next() {
		key := iter.Key()
		next, ok := rewriteObjectKeyPrefix(key, oldKey, newKey)
		if !ok {
			continue
		}
		renames = append(renames, objectRename{oldKey: key, newKey: next})
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}

	// Move shorter parent keys before their descendants.
	slices.SortFunc(renames, func(a, b objectRename) int {
		return len(a.oldKey) - len(b.oldKey)
	})
	return renames, nil
}

// checkObjectRenameCollisions rejects occupied destinations and overlapping moves.
func (t *WorldState) checkObjectRenameCollisions(ctx context.Context, renames []objectRename) error {
	// Record all source keys so a destination cannot overwrite another source.
	oldKeys := make(map[string]struct{}, len(renames))
	for _, rename := range renames {
		oldKeys[rename.oldKey] = struct{}{}
	}

	// Resolve every remaining destination through the current object index.
	for _, rename := range renames {
		if _, ok := oldKeys[rename.newKey]; ok {
			return world.ErrObjectExists
		}
		obj, found, err := t.getObject(ctx, rename.newKey)
		world.ReleaseObjectState(obj)
		if err != nil {
			return err
		}
		if found {
			return world.ErrObjectExists
		}
	}
	return nil
}

// rewriteObjectKeyPrefix replaces one complete object-key prefix.
func rewriteObjectKeyPrefix(key, oldKey, newKey string) (string, bool) {
	// The parent itself has no descendant separator to preserve.
	if key == oldKey {
		return newKey, true
	}

	// Match a complete path component before retaining the descendant suffix.
	prefix := oldKey + "/"
	if !strings.HasPrefix(key, prefix) {
		return key, false
	}
	return newKey + key[len(oldKey):], true
}

// objectRename identifies one source object and its proposed destination.
type objectRename struct {
	// oldKey is the existing object key.
	oldKey string
	// newKey is the replacement object key.
	newKey string
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (t *WorldState) DeleteObject(ctx context.Context, key string) (bool, error) {
	// Require an active writer before resolving or removing the object.
	if !t.write {
		return false, tx.ErrNotWrite
	}
	if t.discarded.Load() {
		return false, tx.ErrDiscarded
	}

	// Resolve the stored object, retaining its metadata until deletion is recorded.
	ot := t.objTree
	k := []byte(objectKeyPrefix + key)
	objState, found, err := t.GetObject(ctx, key)
	defer world.ReleaseObjectState(objState)
	if err != nil && err != world.ErrObjectNotFound {
		return false, err
	}
	if !found {
		return false, nil
	}

	// Keep the object's block cursor for the deletion change record.
	objs, ok := objState.(*ObjectState)
	if !ok {
		return false, block.ErrUnexpectedType
	}
	nbcs := objs.bcs

	// Remove graph links that refer to the object.
	err = t.DeleteGraphObject(ctx, key)
	if err != nil {
		return true, err
	}

	// Delete the object entry and clear its memoized state.
	err = ot.Delete(ctx, k)
	if err != nil {
		return true, err
	}

	// A deleted object no longer exists; drop any stale object memo entry.
	t.forgetObject(key)

	// Record the object deletion in the world changelog.
	changeBcs, err := t.queueWorldChange(ctx, &WorldChange{
		Key:        key,
		ChangeType: WorldChangeType_WorldChange_OBJECT_DELETE,
	})
	if err != nil {
		return false, err
	}
	if changeBcs != nil {
		changeBcs.SetRef(6, nbcs)
	}

	// Report that the object was deleted.
	return true, nil
}

// renameGraphObject rewrites each complete incident quad exactly once.
func (t *WorldState) renameGraphObject(ctx context.Context, oldKey, newKey string) error {
	// Capture both endpoint directions before changing any indexed relationship.
	oldValue := world.KeyToGraphValue(oldKey).String()
	newValue := world.KeyToGraphValue(newKey).String()
	subjQuads, err := t.LookupGraphQuads(ctx, world.NewGraphQuad(oldValue, "", "", ""), 0)
	if err != nil {
		return err
	}
	objQuads, err := t.LookupGraphQuads(ctx, world.NewGraphQuad("", "", oldValue, ""), 0)
	if err != nil {
		return err
	}

	// Keep field boundaries in the identity, including embedded NUL bytes.
	seen := make(map[[4]string]struct{}, len(subjQuads)+len(objQuads))
	for _, q := range append(subjQuads, objQuads...) {
		// A self-edge appears in both direction scans but must change only once.
		key := [4]string{q.GetSubject(), q.GetPredicate(), q.GetObj(), q.GetLabel()}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		// Rewrite only endpoints that identify the renamed object.
		subj := q.GetSubject()
		obj := q.GetObj()
		if subj == oldValue {
			subj = newValue
		}
		if obj == oldValue {
			obj = newValue
		}
		next := world.NewGraphQuad(subj, q.GetPredicate(), obj, q.GetLabel())

		// Validate both graph values before removing the old relationship.
		prevQuad, err := world.GraphQuadToCayleyQuad(q, true)
		if err != nil {
			return err
		}
		nextQuad, err := world.GraphQuadToCayleyQuad(next, true)
		if err != nil {
			return err
		}

		// Record the removal even when the rewritten relationship already exists.
		if err := t.graphHd.RemoveQuad(ctx, prevQuad); err != nil && !graph.IsQuadNotExist(err) {
			return err
		}
		if _, err := t.queueWorldChange(ctx, &WorldChange{
			ChangeType: WorldChangeType_WorldChange_GRAPH_DELETE,
			Quad:       world.GraphQuadToQuad(q),
		}); err != nil {
			return err
		}

		// Add and record the replacement only when it is a new relationship.
		exists, err := world.CheckQuadExists(ctx, t.graphHd, nextQuad)
		if err != nil {
			return err
		}
		if exists {
			continue
		}
		if err := t.graphHd.AddQuad(ctx, nextQuad); err != nil {
			return err
		}
		if _, err := t.queueWorldChange(ctx, &WorldChange{
			ChangeType: WorldChangeType_WorldChange_GRAPH_SET,
			Quad:       world.GraphQuadToQuad(next),
		}); err != nil {
			return err
		}
	}
	return nil
}

// _ is a type assertion
var _ world.WorldStateObject = (*WorldState)(nil)

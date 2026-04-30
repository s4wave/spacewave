package world_block

import (
	"context"

	trace "github.com/s4wave/spacewave/db/traceutil"
	"slices"
	"strings"

	"github.com/aperturerobotics/cayley/graph"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/bucket"
	"github.com/s4wave/spacewave/db/tx"
	"github.com/s4wave/spacewave/db/world"
)

// GetObject looks up an object by key.
// Returns nil, false if not found.
func (t *WorldState) GetObject(ctx context.Context, key string) (world.ObjectState, bool, error) {
	val, ok, err := t.getObject(ctx, key)
	if val == nil {
		return nil, ok, err
	}
	return val, ok, err
}

// getObject looks up an object by key.
// Returns nil, false if not found.
func (t *WorldState) getObject(ctx context.Context, key string) (*ObjectState, bool, error) {
	ctx, task := trace.NewTask(ctx, "hydra/world-block/world-state/get-object")
	defer task.End()

	if t.discarded.Load() {
		return nil, false, tx.ErrDiscarded
	}
	ot := t.objTree
	k := []byte(objectKeyPrefix + key)
	taskCtx, subtask := trace.NewTask(ctx, "hydra/world-block/world-state/get-object/get-cursor-at-key")
	bcs, err := ot.GetCursorAtKey(taskCtx, k)
	subtask.End()
	if err != nil || bcs == nil {
		return nil, false, err
	}
	taskCtx, subtask = trace.NewTask(ctx, "hydra/world-block/world-state/get-object/new-object-state")
	ost, err := NewObjectState(taskCtx, t, bcs)
	subtask.End()
	if err != nil {
		return nil, false, err
	}
	return ost, true, nil
}

// mustGetObject returns an error if not found.
func (t *WorldState) mustGetObject(ctx context.Context, key string) (*ObjectState, error) {
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

// CreateObject creates a object with a key and initial root ref.
// Returns ErrObjectExists if the object already exists.
// Appends a OBJECT_SET change to the changelog.
func (t *WorldState) CreateObject(ctx context.Context, key string, rootRef *bucket.ObjectRef) (world.ObjectState, error) {
	if !t.write {
		return nil, tx.ErrNotWrite
	}
	if t.discarded.Load() {
		return nil, tx.ErrDiscarded
	}

	ot := t.objTree
	k := []byte(objectKeyPrefix + key)
	exists, err := ot.Exists(ctx, k)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, world.ErrObjectExists
	}
	obj := NewObject(key, rootRef)
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
	changeBcs, err := t.queueWorldChange(ctx, &WorldChange{
		Key:        key,
		ChangeType: WorldChangeType_WorldChange_OBJECT_SET,
	})
	if err != nil {
		return nil, err
	}
	changeBcs.SetRef(5, nbcs)

	// GC: world -> object, object -> root block.
	if rg := t.refGraph; rg != nil {
		objIRI := block_gc.ObjectIRI(key)
		if err := rg.AddRef(ctx, "world", objIRI); err != nil {
			return nil, err
		}
		rootBlockRef := rootRef.GetRootRef()
		if err := rg.AddObjectRoot(ctx, key, rootBlockRef); err != nil {
			return nil, err
		}
		if rootBlockRef != nil && !rootBlockRef.GetEmpty() {
			_ = rg.RemoveRef(ctx, block_gc.NodeUnreferenced, block_gc.BlockIRI(rootBlockRef))
		}
	}

	return objState, nil
}

// RenameObject renames an object key and updates associated graph quads.
func (t *WorldState) RenameObject(ctx context.Context, oldKey, newKey string, descendants bool) (world.ObjectState, error) {
	if !t.write {
		return nil, tx.ErrNotWrite
	}
	if t.discarded.Load() {
		return nil, tx.ErrDiscarded
	}
	if oldKey == "" || newKey == "" {
		return nil, world.ErrEmptyObjectKey
	}
	if descendants {
		return t.renameObjectDescendants(ctx, oldKey, newKey)
	}

	return t.renameObjectSingle(ctx, oldKey, newKey)
}

func (t *WorldState) renameObjectSingle(ctx context.Context, oldKey, newKey string) (world.ObjectState, error) {
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

	ot := t.objTree
	newTreeKey := []byte(objectKeyPrefix + newKey)
	exists, err := ot.Exists(ctx, newTreeKey)
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, world.ErrObjectExists
	}

	oldRoot, err := oldObj.GetRoot(ctx)
	if err != nil {
		return nil, err
	}
	newRoot := oldRoot.Clone()
	newRoot.Key = newKey

	newBcs := t.bcs.Detach(false)
	newBcs.ClearAllRefs()
	newBcs.SetBlock(newRoot, true)
	if err := ot.SetCursorAtKey(ctx, newTreeKey, newBcs, false); err != nil {
		return nil, err
	}

	if err := t.renameGraphObject(ctx, oldKey, newKey); err != nil {
		return nil, err
	}

	oldTreeKey := []byte(objectKeyPrefix + oldKey)
	if err := ot.Delete(ctx, oldTreeKey); err != nil {
		return nil, err
	}

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

	if rg := t.refGraph; rg != nil {
		rootBlockRef := oldRoot.GetRootRef().GetRootRef()
		oldObjIRI := block_gc.ObjectIRI(oldKey)
		newObjIRI := block_gc.ObjectIRI(newKey)
		adds := []block_gc.RefEdge{
			{Subject: "world", Object: newObjIRI},
			{Subject: block_gc.NodeUnreferenced, Object: oldObjIRI},
		}
		removes := []block_gc.RefEdge{
			{Subject: "world", Object: oldObjIRI},
		}
		if rootBlockRef != nil && !rootBlockRef.GetEmpty() {
			rootBlockIRI := block_gc.BlockIRI(rootBlockRef)
			adds = append(adds, block_gc.RefEdge{Subject: newObjIRI, Object: rootBlockIRI})
			removes = append(removes, block_gc.RefEdge{Subject: block_gc.NodeUnreferenced, Object: rootBlockIRI})
		}
		if err := rg.ApplyRefBatch(ctx, adds, removes); err != nil {
			return nil, err
		}
	}

	return NewObjectState(ctx, t, newBcs)
}

func (t *WorldState) renameObjectDescendants(ctx context.Context, oldKey, newKey string) (world.ObjectState, error) {
	if oldKey == newKey {
		return t.renameObjectSingle(ctx, oldKey, newKey)
	}
	if strings.HasPrefix(newKey, oldKey+"/") {
		return nil, world.ErrObjectExists
	}
	if _, found, err := t.getObject(ctx, oldKey); err != nil {
		return nil, err
	} else if !found {
		return nil, world.ErrObjectNotFound
	}

	renames, err := t.collectObjectRenames(ctx, oldKey, newKey)
	if err != nil {
		return nil, err
	}
	if err := t.checkObjectRenameCollisions(ctx, renames); err != nil {
		return nil, err
	}

	var out world.ObjectState
	for _, rename := range renames {
		obj, err := t.renameObjectSingle(ctx, rename.oldKey, rename.newKey)
		if err != nil {
			return nil, err
		}
		if rename.oldKey == oldKey {
			out = obj
		}
	}
	return out, nil
}

func (t *WorldState) collectObjectRenames(ctx context.Context, oldKey, newKey string) ([]objectRename, error) {
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
	slices.SortFunc(renames, func(a, b objectRename) int {
		return len(a.oldKey) - len(b.oldKey)
	})
	return renames, nil
}

func (t *WorldState) checkObjectRenameCollisions(ctx context.Context, renames []objectRename) error {
	oldKeys := make(map[string]struct{}, len(renames))
	for _, rename := range renames {
		oldKeys[rename.oldKey] = struct{}{}
	}
	for _, rename := range renames {
		if _, ok := oldKeys[rename.newKey]; ok {
			return world.ErrObjectExists
		}
		_, found, err := t.getObject(ctx, rename.newKey)
		if err != nil {
			return err
		}
		if found {
			return world.ErrObjectExists
		}
	}
	return nil
}

func rewriteObjectKeyPrefix(key, oldKey, newKey string) (string, bool) {
	if key == oldKey {
		return newKey, true
	}
	prefix := oldKey + "/"
	if !strings.HasPrefix(key, prefix) {
		return key, false
	}
	return newKey + key[len(oldKey):], true
}

type objectRename struct {
	oldKey string
	newKey string
}

// DeleteObject deletes an object and associated graph quads by ID.
// Calls DeleteGraphObject internally.
// Returns false, nil if not found.
func (t *WorldState) DeleteObject(ctx context.Context, key string) (bool, error) {
	if !t.write {
		return false, tx.ErrNotWrite
	}
	if t.discarded.Load() {
		return false, tx.ErrDiscarded
	}

	ot := t.objTree
	k := []byte(objectKeyPrefix + key)

	objState, found, err := t.GetObject(ctx, key)
	if err != nil {
		if err != world.ErrObjectNotFound {
			return false, err
		}
	}
	if !found {
		return false, nil
	}

	objs, ok := objState.(*ObjectState)
	if !ok {
		return false, block.ErrUnexpectedType
	}
	nbcs := objs.bcs

	// GC: remove world -> object edge, mark object unreferenced.
	if rg := t.refGraph; rg != nil {
		objIRI := block_gc.ObjectIRI(key)
		if err := rg.RemoveRef(ctx, "world", objIRI); err != nil {
			return false, err
		}
		if err := rg.AddRef(ctx, block_gc.NodeUnreferenced, objIRI); err != nil {
			return false, err
		}
	}

	// delete any graph links with the object as subject or object
	err = t.DeleteGraphObject(ctx, key)
	if err != nil {
		return true, err
	}

	// delete the object
	err = ot.Delete(ctx, k)
	if err != nil {
		return true, err
	}

	// update the changelog
	changeBcs, err := t.queueWorldChange(ctx, &WorldChange{
		Key:        key,
		ChangeType: WorldChangeType_WorldChange_OBJECT_DELETE,
	})
	if err != nil {
		return false, err
	}
	// changeBcs may be nil here but this is checked in SetRef.
	changeBcs.SetRef(7, nbcs)

	// success
	return true, nil
}

func (t *WorldState) renameGraphObject(ctx context.Context, oldKey, newKey string) error {
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

	seen := make(map[string]struct{}, len(subjQuads)+len(objQuads))
	for _, q := range append(subjQuads, objQuads...) {
		key := graphQuadKey(q)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		subj := q.GetSubject()
		obj := q.GetObj()
		if subj == oldValue {
			subj = newValue
		}
		if obj == oldValue {
			obj = newValue
		}
		next := world.NewGraphQuad(subj, q.GetPredicate(), obj, q.GetLabel())

		prevQuad, err := world.GraphQuadToCayleyQuad(q, true)
		if err != nil {
			return err
		}
		nextQuad, err := world.GraphQuadToCayleyQuad(next, true)
		if err != nil {
			return err
		}
		if err := t.graphHd.RemoveQuad(ctx, prevQuad); err != nil && !graph.IsQuadNotExist(err) {
			return err
		}
		if _, err := t.queueWorldChange(ctx, &WorldChange{
			ChangeType: WorldChangeType_WorldChange_GRAPH_DELETE,
			Quad:       world.GraphQuadToQuad(q),
		}); err != nil {
			return err
		}

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

func graphQuadKey(q world.GraphQuad) string {
	return q.GetSubject() + "\x00" + q.GetPredicate() + "\x00" + q.GetObj() + "\x00" + q.GetLabel()
}

// _ is a type assertion
var _ world.WorldStateObject = ((*WorldState)(nil))

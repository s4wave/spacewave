package s4wave_flowgraph

import (
	"context"
	"maps"
	"slices"

	"github.com/s4wave/spacewave/db/block"
	bucket_lookup "github.com/s4wave/spacewave/db/bucket/lookup"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
)

// ReadFlowgraphChanges reads up to limit revisions of the graph at key from
// the World changelog, newest first. A zero limit reads every retained
// revision. A World without a changelog keeps no history and reports so.
//
// Each revision ends with a World change to the graph's object and includes
// the placement edges changed since the preceding revision. UpdateFlowgraph
// writes the object last, so each revision is one authored edit.
func ReadFlowgraphChanges(ctx context.Context, ws world.WorldState, key string, limit uint32) (*ListFlowgraphChangesResponse, error) {
	resp := &ListFlowgraphChangesResponse{}
	err := ws.AccessWorldState(ctx, nil, func(root *bucket_lookup.Cursor) error {
		// Report a World that keeps no changelog.
		_, rootBcs := root.BuildTransaction(nil)
		worldRoot, err := world_block.UnmarshalWorld(ctx, rootBcs)
		if err != nil {
			return err
		}
		if worldRoot.GetLastChangeDisable() {
			resp.ChangelogDisabled = true
			return nil
		}

		// Replay the retained entries oldest first.
		entries, err := world_block.ReadChangeLogEntriesFromCursor(ctx, rootBcs, world_block.ChangeLogReadOptions{})
		if err != nil {
			return err
		}
		h := &historyReader{ctx: ctx, root: root, key: key}
		for _, entry := range slices.Backward(entries) {
			for _, change := range entry.Changes {
				if err := h.apply(entry.Seqno, change); err != nil {
					return err
				}
			}
		}
		h.closeRevision(h.seqno, false)
		resp.Changes, resp.Complete = h.changes, h.complete
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Return the newest revisions first, up to the limit.
	slices.Reverse(resp.Changes)
	if limit != 0 && len(resp.Changes) > int(limit) {
		resp.Changes = resp.Changes[:limit]
		resp.Complete = false
	}
	return resp, nil
}

// historyReader replays World changes into one graph's revisions.
type historyReader struct {
	ctx context.Context
	// root is the World root, used to read object and body blocks.
	root *bucket_lookup.Cursor
	// key identifies the graph.
	key string
	// body is the graph body at the latest revision, nil until known.
	body *Flowgraph
	// edit collects the changes since the latest revision, nil when none.
	edit *UpdateFlowgraphRequest
	// seqno is the seqno of the newest change in edit.
	seqno uint64
	// changes contains the revisions, oldest first.
	changes []*FlowgraphChange
	// complete is set when changes start at the graph's creation.
	complete bool
}

// apply adds one World change recorded at seqno.
func (h *historyReader) apply(seqno uint64, change *world_block.WorldChange) error {
	// Collect placement edges and skip changes to other objects.
	switch change.GetChangeType() {
	case world_block.WorldChangeType_WorldChange_GRAPH_SET, world_block.WorldChangeType_WorldChange_GRAPH_DELETE:
		return h.applyPlacement(seqno, change)
	case world_block.WorldChangeType_WorldChange_OBJECT_SET,
		world_block.WorldChangeType_WorldChange_OBJECT_INC_REV,
		world_block.WorldChangeType_WorldChange_OBJECT_DELETE:
		if change.GetKey() != h.key {
			return nil
		}
	default:
		return nil
	}

	// A deletion ends the graph's history; a revision change keeps its body.
	switch change.GetChangeType() {
	case world_block.WorldChangeType_WorldChange_OBJECT_DELETE:
		*h = historyReader{ctx: h.ctx, root: h.root, key: h.key}
		return nil
	case world_block.WorldChangeType_WorldChange_OBJECT_INC_REV:
		h.closeRevision(seqno, false)
		return nil
	}

	// Start the history at the creation, or read the body it continues from.
	created := change.GetPrevObjectRef().GetEmpty()
	if created {
		h.changes, h.complete, h.edit = nil, true, nil
		h.body = &Flowgraph{}
	}
	if h.body == nil {
		body, err := h.readBody(change.GetPrevObjectRef())
		if err != nil {
			return err
		}
		h.body = body
	}

	// Record the body changes as this revision's update.
	next, err := h.readBody(change.GetObjectRef())
	if err != nil {
		return err
	}
	edit := h.pending()
	if name := next.GetName(); name != h.body.GetName() {
		edit.Name = &name
	}
	edit.SetNodes, edit.RemoveNodeIds = diffEntries(h.body.GetNodes(), next.GetNodes())
	edit.SetConnections, edit.RemoveConnectionIds = diffEntries(h.body.GetConnections(), next.GetConnections())
	h.body = next
	h.closeRevision(seqno, created)
	return nil
}

// applyPlacement adds a placement edge change of this graph to the edit.
func (h *historyReader) applyPlacement(seqno uint64, change *world_block.WorldChange) error {
	// A subject that is not an object key is no graph's edge.
	edge := world.QuadToGraphQuad(change.GetQuad())
	subject, err := world.GraphValueToKey(edge.GetSubject())
	if err != nil || subject != h.key {
		return nil
	}
	nodeID, placement, err := parsePlacementQuad(edge)
	if err != nil || placement == nil {
		return err
	}

	// Keep the newest placement of each node.
	edit := h.pending()
	h.seqno = seqno
	edit.RemovePlacementNodeIds = slices.DeleteFunc(edit.RemovePlacementNodeIds, func(id string) bool {
		return id == nodeID
	})
	delete(edit.SetPlacements, nodeID)
	if change.GetChangeType() == world_block.WorldChangeType_WorldChange_GRAPH_DELETE {
		edit.RemovePlacementNodeIds = append(edit.RemovePlacementNodeIds, nodeID)
		return nil
	}
	if edit.SetPlacements == nil {
		edit.SetPlacements = make(map[string]*FlowgraphPlacement)
	}
	edit.SetPlacements[nodeID] = placement
	return nil
}

// pending returns the edit collecting changes since the latest revision.
func (h *historyReader) pending() *UpdateFlowgraphRequest {
	if h.edit == nil {
		h.edit = &UpdateFlowgraphRequest{}
	}
	return h.edit
}

// closeRevision records the pending edit as a revision at seqno. Without a
// creation, a revision with no pending edit records nothing.
func (h *historyReader) closeRevision(seqno uint64, created bool) {
	if h.edit == nil && !created {
		return
	}
	h.changes = append(h.changes, &FlowgraphChange{Seqno: seqno, Created: created, Edit: h.pending()})
	h.edit = nil
}

// readBody reads the graph body of the World object block at ref.
func (h *historyReader) readBody(ref *block.BlockRef) (*Flowgraph, error) {
	// Read the object block that the change recorded.
	_, objectBcs := h.root.BuildTransactionAtRef(nil, ref)
	object, err := world_block.UnmarshalObject(h.ctx, objectBcs)
	if err != nil {
		return nil, err
	}

	// Follow the object's root to its body.
	cursor, err := h.root.FollowRef(h.ctx, object.GetRootRef())
	if err != nil {
		return nil, err
	}
	defer cursor.Release()
	_, bodyBcs := cursor.BuildTransaction(nil)
	body, err := UnmarshalFlowgraph(h.ctx, bodyBcs)
	if body == nil {
		body = &Flowgraph{}
	}
	return body, err
}

// diffEntries returns the entries of next that differ from prev and the
// sorted IDs of prev missing from next.
func diffEntries[V interface{ EqualVT(V) bool }](prev, next map[string]V) (map[string]V, []string) {
	var set map[string]V
	for id, entry := range next {
		if entry.EqualVT(prev[id]) {
			continue
		}
		if set == nil {
			set = make(map[string]V)
		}
		set[id] = entry
	}
	var removed []string
	for _, id := range slices.Sorted(maps.Keys(prev)) {
		if _, ok := next[id]; !ok {
			removed = append(removed, id)
		}
	}
	return set, removed
}

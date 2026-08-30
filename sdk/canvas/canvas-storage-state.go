package s4wave_canvas

import (
	"context"
	"maps"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	block_kvtx "github.com/s4wave/spacewave/db/kvtx/block"
)

// UnmarshalCanvasState reads either the current Canvas DAG or a legacy flat
// Canvas state and returns the logical RPC state.
func UnmarshalCanvasState(ctx context.Context, bcs *block.Cursor) (*CanvasState, error) {
	data, found, err := bcs.Fetch(ctx)
	if err != nil || !found {
		return nil, err
	}

	// The storage format is field 1 with a varint wire type. Legacy CanvasState
	// uses a map at field 1, so it cannot be mistaken for this version marker.
	storageProbe := &CanvasStorage{}
	storageErr := storageProbe.UnmarshalVT(data)
	if storageProbe.GetFormat() != CanvasStorageFormat_CANVAS_STORAGE_FORMAT_BLOCK_KVTX_V1 {
		state, err := block.UnmarshalBlock[*CanvasState](ctx, bcs, NewCanvasStateBlock)
		if err != nil {
			return nil, err
		}
		if state == nil {
			state = &CanvasState{}
		}
		return state, nil
	}
	if storageErr != nil {
		return nil, errors.Wrap(storageErr, "unmarshal Canvas storage")
	}
	storage, err := UnmarshalCanvasStorage(ctx, bcs)
	if err != nil {
		return nil, err
	}
	if err := storage.Validate(); err != nil {
		return nil, err
	}
	return materializeCanvasStorage(ctx, bcs, storage)
}

func materializeCanvasStorage(
	ctx context.Context,
	bcs *block.Cursor,
	storage *CanvasStorage,
) (*CanvasState, error) {
	state := &CanvasState{
		Nodes:            make(map[string]*CanvasNode),
		Edges:            cloneCanvasEdges(storage.GetEdges()),
		StrokeTreeRef:    append([]byte(nil), storage.GetStrokeTreeRef()...),
		HiddenGraphLinks: cloneHiddenGraphLinks(storage.GetHiddenGraphLinks()),
		LayoutMetadata:   cloneCanvasLayoutMetadata(storage.GetLayoutMetadata()),
	}
	nodes, err := block_kvtx.BuildKvTransaction(ctx, bcs.FollowSubBlock(2), false)
	if err != nil {
		return nil, err
	}
	defer nodes.Discard()

	it := nodes.BlockIterate(ctx, nil, false, false)
	defer it.Close()
	for it.Next() {
		node, err := block.UnmarshalBlock[*CanvasNode](ctx, it.ValueCursor(), NewCanvasNodeBlock)
		if err != nil {
			return nil, err
		}
		state.Nodes[string(it.Key())] = node
	}
	if err := it.Err(); err != nil {
		return nil, err
	}
	return state, nil
}

// WriteCanvasState writes a complete logical Canvas state through the
// pluggable block KVTX node index. Previous must be the complete logical state
// read from the same object root. A nil previous value reloads that state.
func WriteCanvasState(
	ctx context.Context,
	bcs *block.Cursor,
	previous *CanvasState,
	next *CanvasState,
) error {
	if next == nil {
		next = &CanvasState{}
	}
	storage, current, err := loadCanvasStorageForWrite(ctx, bcs, previous)
	if err != nil {
		return err
	}

	storage.Edges = cloneCanvasEdges(next.GetEdges())
	storage.StrokeTreeRef = append(storage.StrokeTreeRef[:0], next.GetStrokeTreeRef()...)
	storage.HiddenGraphLinks = cloneHiddenGraphLinks(next.GetHiddenGraphLinks())
	storage.LayoutMetadata = cloneCanvasLayoutMetadata(next.GetLayoutMetadata())
	bcs.SetBlock(storage, true)

	nodes, err := block_kvtx.BuildKvTransaction(ctx, bcs.FollowSubBlock(2), true)
	if err != nil {
		return err
	}
	defer nodes.Discard()

	for _, id := range slices.Sorted(maps.Keys(next.GetNodes())) {
		node := next.GetNodes()[id]
		if old := current.GetNodes()[id]; old != nil && old.EqualVT(node) {
			continue
		}
		if err := setCanvasNode(ctx, nodes, id, node); err != nil {
			return err
		}
	}
	for _, id := range slices.Sorted(maps.Keys(current.GetNodes())) {
		if _, ok := next.GetNodes()[id]; ok {
			continue
		}
		if _, err := nodes.DeleteCursorAtKey(ctx, []byte(id)); err != nil {
			return err
		}
	}
	return nodes.Commit(ctx)
}

func loadCanvasStorageForWrite(
	ctx context.Context,
	bcs *block.Cursor,
	previous *CanvasState,
) (*CanvasStorage, *CanvasState, error) {
	data, found, err := bcs.Fetch(ctx)
	if err != nil {
		return nil, nil, err
	}
	if !found {
		storage := NewCanvasStorage()
		bcs.SetBlock(storage, true)
		return storage, &CanvasState{}, nil
	}

	probe := &CanvasStorage{}
	probeErr := probe.UnmarshalVT(data)
	if probe.GetFormat() == CanvasStorageFormat_CANVAS_STORAGE_FORMAT_BLOCK_KVTX_V1 {
		if probeErr != nil {
			return nil, nil, errors.Wrap(probeErr, "unmarshal Canvas storage")
		}
		storage, err := UnmarshalCanvasStorage(ctx, bcs)
		if err != nil {
			return nil, nil, err
		}
		if err := storage.Validate(); err != nil {
			return nil, nil, err
		}
		if previous == nil {
			previous, err = materializeCanvasStorage(ctx, bcs, storage)
			if err != nil {
				return nil, nil, err
			}
		}
		return storage, previous, nil
	}

	if _, err := block.UnmarshalBlock[*CanvasState](ctx, bcs, NewCanvasStateBlock); err != nil {
		return nil, nil, err
	}
	storage := NewCanvasStorage()
	bcs.SetBlock(storage, true)
	return storage, &CanvasState{}, nil
}

func setCanvasNode(ctx context.Context, nodes kvtx.BlockTx, id string, node *CanvasNode) error {
	if node == nil {
		_, err := nodes.DeleteCursorAtKey(ctx, []byte(id))
		return err
	}
	cursor := nodes.GetCursor().Detach(false)
	cursor.ClearAllRefs()
	cursor.SetBlock(node.CloneVT(), true)
	return nodes.SetCursorAtKey(ctx, []byte(id), cursor, false)
}

func cloneCanvasEdges(edges []*CanvasEdge) []*CanvasEdge {
	cloned := make([]*CanvasEdge, len(edges))
	for i, edge := range edges {
		cloned[i] = edge.CloneVT()
	}
	return cloned
}

func cloneHiddenGraphLinks(links []*HiddenGraphLink) []*HiddenGraphLink {
	cloned := make([]*HiddenGraphLink, len(links))
	for i, link := range links {
		cloned[i] = link.CloneVT()
	}
	return cloned
}

func cloneCanvasLayoutMetadata(metadata map[string]*CanvasLayoutMetadata) map[string]*CanvasLayoutMetadata {
	if metadata == nil {
		return nil
	}
	cloned := make(map[string]*CanvasLayoutMetadata, len(metadata))
	maps.Copy(cloned, metadata)
	for id, item := range cloned {
		cloned[id] = item.CloneVT()
	}
	return cloned
}

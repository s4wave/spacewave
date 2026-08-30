package s4wave_canvas

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
	block_kvtx "github.com/s4wave/spacewave/db/kvtx/block"
	db_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/world"
	world_block "github.com/s4wave/spacewave/db/world/block"
	"github.com/sirupsen/logrus"
)

type canvasStateStream struct {
	ctx  context.Context
	sent chan *WatchCanvasStateResponse
}

func newCanvasStateStream(ctx context.Context) *canvasStateStream {
	return &canvasStateStream{
		ctx:  ctx,
		sent: make(chan *WatchCanvasStateResponse, 8),
	}
}

func (s *canvasStateStream) Context() context.Context {
	return s.ctx
}

func (s *canvasStateStream) MsgSend(srpc.Message) error {
	panic("MsgSend should not be called")
}

func (s *canvasStateStream) MsgRecv(srpc.Message) error {
	panic("MsgRecv should not be called")
}

func (s *canvasStateStream) CloseSend() error {
	return nil
}

func (s *canvasStateStream) Close() error {
	return nil
}

func (s *canvasStateStream) Send(resp *WatchCanvasStateResponse) error {
	select {
	case <-s.ctx.Done():
		return s.ctx.Err()
	case s.sent <- resp.CloneVT():
		return nil
	}
}

func (s *canvasStateStream) SendAndClose(resp *WatchCanvasStateResponse) error {
	if resp != nil {
		if err := s.Send(resp); err != nil {
			return err
		}
	}
	return s.CloseSend()
}

func recvCanvasTestValue[T any](t *testing.T, ch <-chan T, name string) T {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	select {
	case val, ok := <-ch:
		if !ok {
			t.Fatalf("%s channel closed", name)
		}
		return val
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s", name)
	}
	var zero T
	return zero
}

func setupCanvasWatchWorld(
	t *testing.T,
	ctx context.Context,
	objKey string,
	state *CanvasState,
) (*world_block.WorldState, func()) {
	t.Helper()

	log := logrus.New()
	le := logrus.NewEntry(log)
	tb, err := db_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	ocs, err := tb.BuildEmptyCursor(ctx)
	if err != nil {
		tb.Release()
		t.Fatal(err.Error())
	}
	ws, err := world_block.BuildMockWorldState(ctx, le, true, ocs, false)
	if err != nil {
		ocs.Release()
		tb.Release()
		t.Fatal(err.Error())
	}
	_, _, err = world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		bcs.SetBlock(state, true)
		return nil
	})
	if err == nil {
		err = ws.Commit(ctx)
	}
	if err != nil {
		ocs.Release()
		tb.Release()
		t.Fatal(err.Error())
	}
	return ws, func() {
		ocs.Release()
		tb.Release()
	}
}

func setCanvasWatchWorldState(
	t *testing.T,
	ctx context.Context,
	ws *world_block.WorldState,
	objKey string,
	state *CanvasState,
) {
	t.Helper()

	_, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		bcs.SetBlock(state, true)
		return nil
	})
	if err == nil {
		err = ws.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err.Error())
	}
}

func requireCanvasNode(t *testing.T, state *CanvasState, id string) {
	t.Helper()
	if state.GetNodes()[id] == nil {
		t.Fatalf("expected canvas node %q in %#v", id, state.GetNodes())
	}
}

func requireCanvasLayoutMetadata(t *testing.T, state *CanvasState, id string) *CanvasLayoutMetadata {
	t.Helper()
	meta := state.GetLayoutMetadata()[id]
	if meta == nil {
		t.Fatalf("expected canvas layout metadata %q in %#v", id, state.GetLayoutMetadata())
	}
	return meta
}

func requireCanvasNodeEventually(
	t *testing.T,
	ch <-chan *WatchCanvasStateResponse,
	done <-chan error,
	name string,
	id string,
) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()
	var last map[string]*CanvasNode
	for {
		select {
		case resp := <-ch:
			last = resp.GetState().GetNodes()
			if last[id] != nil {
				return
			}
		case err := <-done:
			t.Fatalf("%s watch exited before node %q: %v", name, id, err)
		case <-ctx.Done():
			t.Fatalf("timed out waiting for %s node %q, last nodes %#v", name, id, last)
		}
	}
}

func TestCanvasResourceWatchSharesWorldUpdates(t *testing.T) {
	ctx := t.Context()
	objKey := "canvas/watch-shared"
	initial := &CanvasState{
		Nodes: map[string]*CanvasNode{
			"initial": {Id: "initial", TextContent: "initial"},
		},
		LayoutMetadata: map[string]*CanvasLayoutMetadata{
			"initial": {
				StableNodeId:    "spell-run:initial",
				Lane:            "source",
				Rank:            1,
				Group:           "intent",
				ProjectionOwner: "glados/workflow",
			},
		},
	}
	ws, cleanup := setupCanvasWatchWorld(t, ctx, objKey, initial)
	t.Cleanup(cleanup)

	resource := NewCanvasResource(ws, nil, objKey, initial)
	t.Cleanup(resource.Close)
	streamCtxA, cancelA := context.WithCancel(ctx)
	defer cancelA()
	streamCtxB, cancelB := context.WithCancel(ctx)
	defer cancelB()
	strmA := newCanvasStateStream(streamCtxA)
	strmB := newCanvasStateStream(streamCtxB)
	doneA := make(chan error, 1)
	doneB := make(chan error, 1)
	go func() {
		doneA <- resource.WatchCanvasState(&WatchCanvasStateRequest{}, strmA)
	}()
	go func() {
		doneB <- resource.WatchCanvasState(&WatchCanvasStateRequest{}, strmB)
	}()

	initialA := recvCanvasTestValue(t, strmA.sent, "stream A initial").GetState()
	requireCanvasNode(t, initialA, "initial")
	if got := requireCanvasLayoutMetadata(t, initialA, "initial").GetLane(); got != "source" {
		t.Fatalf("expected stream A initial layout lane source, got %q", got)
	}
	initialB := recvCanvasTestValue(t, strmB.sent, "stream B initial").GetState()
	requireCanvasNode(t, initialB, "initial")
	if got := requireCanvasLayoutMetadata(t, initialB, "initial").GetStableNodeId(); got != "spell-run:initial" {
		t.Fatalf("expected stream B initial stable node id, got %q", got)
	}

	updated := &CanvasState{
		Nodes: map[string]*CanvasNode{
			"updated": {Id: "updated", TextContent: "updated"},
		},
		LayoutMetadata: map[string]*CanvasLayoutMetadata{
			"updated": {
				StableNodeId:    "spell-run:updated",
				Lane:            "proof",
				Rank:            7,
				Group:           "evidence",
				ProjectionOwner: "glados/workflow",
			},
		},
	}
	setCanvasWatchWorldState(t, ctx, ws, objKey, updated)
	updateA := recvCanvasTestValue(t, strmA.sent, "stream A update").GetState()
	requireCanvasNode(t, updateA, "updated")
	if got := requireCanvasLayoutMetadata(t, updateA, "updated").GetRank(); got != 7 {
		t.Fatalf("expected stream A updated rank 7, got %d", got)
	}
	updateB := recvCanvasTestValue(t, strmB.sent, "stream B update").GetState()
	requireCanvasNode(t, updateB, "updated")
	if got := requireCanvasLayoutMetadata(t, updateB, "updated").GetProjectionOwner(); got != "glados/workflow" {
		t.Fatalf("expected stream B updated projection owner, got %q", got)
	}

	burstA := &CanvasState{
		Nodes: map[string]*CanvasNode{
			"burst-a": {Id: "burst-a", TextContent: "burst-a"},
		},
	}
	burstB := &CanvasState{
		Nodes: map[string]*CanvasNode{
			"burst-b": {Id: "burst-b", TextContent: "burst-b"},
		},
	}
	setCanvasWatchWorldState(t, ctx, ws, objKey, burstA)
	setCanvasWatchWorldState(t, ctx, ws, objKey, burstB)
	requireCanvasNodeEventually(t, strmA.sent, doneA, "stream A burst update", "burst-b")
	requireCanvasNodeEventually(t, strmB.sent, doneB, "stream B burst update", "burst-b")

	resource.Close()
	if err := recvCanvasTestValue(t, doneA, "stream A close"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected stream A context.Canceled, got %v", err)
	}
	if err := recvCanvasTestValue(t, doneB, "stream B close"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected stream B context.Canceled, got %v", err)
	}
}

func TestCanvasResourceCloseCancelsWatchersImmediately(t *testing.T) {
	ctx := t.Context()
	resource := NewCanvasResource(nil, nil, "", &CanvasState{})
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	strm := newCanvasStateStream(streamCtx)
	done := make(chan error, 1)
	go func() {
		done <- resource.WatchCanvasState(&WatchCanvasStateRequest{}, strm)
	}()

	recvCanvasTestValue(t, strm.sent, "initial canvas snapshot")
	resource.Close()
	if err := recvCanvasTestValue(t, done, "watch close"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	late := newCanvasStateStream(ctx)
	if err := resource.WatchCanvasState(&WatchCanvasStateRequest{}, late); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected late watcher context.Canceled, got %v", err)
	}
}

func TestCanvasResourceClosePreventsLateStatePublish(t *testing.T) {
	resource := NewCanvasResource(nil, nil, "", &CanvasState{})
	resource.Close()

	resource.setCanvasWatchState(&CanvasState{
		Nodes: map[string]*CanvasNode{
			"late": {Id: "late"},
		},
	})
	late := newCanvasStateStream(t.Context())
	if err := resource.WatchCanvasState(&WatchCanvasStateRequest{}, late); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled after late state publish, got %v", err)
	}
}

func TestUpdateCanvasHiddenGraphLinks(t *testing.T) {
	ctx := context.Background()
	link := &HiddenGraphLink{
		Subject:   "<objects/a>",
		Predicate: "<relatedTo>",
		Object:    "<objects/b>",
		Label:     "main",
	}
	resource := NewCanvasResource(nil, nil, "", &CanvasState{})

	resp, err := resource.UpdateCanvas(ctx, &UpdateCanvasRequest{
		AddHiddenGraphLinks: []*HiddenGraphLink{link, link.CloneVT()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(resp.GetState().GetHiddenGraphLinks()); got != 1 {
		t.Fatalf("expected one hidden graph link after duplicate add, got %d", got)
	}

	resp, err = resource.UpdateCanvas(ctx, &UpdateCanvasRequest{
		RemoveHiddenGraphLinks: []*HiddenGraphLink{link.CloneVT()},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(resp.GetState().GetHiddenGraphLinks()); got != 0 {
		t.Fatalf("expected no hidden graph links after remove, got %d", got)
	}
}

func TestUpdateCanvasHiddenGraphLinksPreservesManualEdges(t *testing.T) {
	ctx := context.Background()
	edge := &CanvasEdge{
		Id:           "edge-1",
		SourceNodeId: "node-1",
		TargetNodeId: "node-2",
		Style:        EdgeStyle_EDGE_STYLE_BEZIER,
	}
	link := &HiddenGraphLink{
		Subject:   "<objects/a>",
		Predicate: "<relatedTo>",
		Object:    "<objects/b>",
	}
	resource := NewCanvasResource(nil, nil, "", &CanvasState{
		Edges: []*CanvasEdge{edge},
	})

	resp, err := resource.UpdateCanvas(ctx, &UpdateCanvasRequest{
		AddHiddenGraphLinks: []*HiddenGraphLink{link},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := len(resp.GetState().GetEdges()); got != 1 {
		t.Fatalf("expected manual edge to be preserved, got %d edges", got)
	}
	if got := len(resp.GetState().GetHiddenGraphLinks()); got != 1 {
		t.Fatalf("expected hidden graph link, got %d", got)
	}
}

func TestUpdateCanvasPreservesLayoutMetadata(t *testing.T) {
	ctx := context.Background()
	resource := NewCanvasResource(nil, nil, "", &CanvasState{
		Nodes: map[string]*CanvasNode{
			"source": {Id: "source", TextContent: "source"},
		},
		Edges: []*CanvasEdge{
			{
				Id:           "edge-1",
				SourceNodeId: "source",
				TargetNodeId: "proof",
				Style:        EdgeStyle_EDGE_STYLE_STRAIGHT,
			},
		},
		HiddenGraphLinks: []*HiddenGraphLink{
			{
				Subject:   "<objects/source>",
				Predicate: "<dependsOn>",
				Object:    "<objects/proof>",
			},
		},
		LayoutMetadata: map[string]*CanvasLayoutMetadata{
			"source": {
				StableNodeId:    "spell-run:source",
				Lane:            "source",
				Rank:            1,
				Group:           "workflow",
				ProjectionOwner: "glados/workflow",
			},
		},
	})

	resp, err := resource.UpdateCanvas(ctx, &UpdateCanvasRequest{
		SetNodes: map[string]*CanvasNode{
			"proof": {Id: "proof", TextContent: "proof"},
		},
		AddEdges: []*CanvasEdge{
			{
				Id:           "edge-2",
				SourceNodeId: "source",
				TargetNodeId: "proof",
				Style:        EdgeStyle_EDGE_STYLE_BEZIER,
			},
		},
		AddHiddenGraphLinks: []*HiddenGraphLink{
			{
				Subject:   "<objects/proof>",
				Predicate: "<summarizes>",
				Object:    "<objects/source>",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := resp.GetState()
	requireCanvasNode(t, state, "source")
	requireCanvasNode(t, state, "proof")
	if got := len(state.GetEdges()); got != 2 {
		t.Fatalf("expected existing and added visual edges, got %d", got)
	}
	if got := len(state.GetHiddenGraphLinks()); got != 2 {
		t.Fatalf("expected existing and added hidden graph links, got %d", got)
	}
	meta := requireCanvasLayoutMetadata(t, state, "source")
	if got := meta.GetStableNodeId(); got != "spell-run:source" {
		t.Fatalf("expected stable node id to be preserved, got %q", got)
	}
	if got := meta.GetLane(); got != "source" {
		t.Fatalf("expected lane to be preserved, got %q", got)
	}
	if got := meta.GetRank(); got != 1 {
		t.Fatalf("expected rank to be preserved, got %d", got)
	}
	if got := meta.GetGroup(); got != "workflow" {
		t.Fatalf("expected group to be preserved, got %q", got)
	}
	if got := meta.GetProjectionOwner(); got != "glados/workflow" {
		t.Fatalf("expected projection owner to be preserved, got %q", got)
	}
}

func TestUpdateCanvasMutatesLayoutMetadata(t *testing.T) {
	ctx := context.Background()
	resource := NewCanvasResource(nil, nil, "", &CanvasState{
		Nodes: map[string]*CanvasNode{
			"old":  {Id: "old", TextContent: "old"},
			"keep": {Id: "keep", TextContent: "keep"},
		},
		LayoutMetadata: map[string]*CanvasLayoutMetadata{
			"old": {
				StableNodeId:    "spell-run:old",
				Lane:            "audit",
				Rank:            1,
				Group:           "workflow",
				ProjectionOwner: "glados/workflow",
			},
			"keep": {
				StableNodeId:    "spell-run:keep",
				Lane:            "proof",
				Rank:            2,
				Group:           "workflow",
				ProjectionOwner: "glados/workflow",
			},
		},
	})

	resp, err := resource.UpdateCanvas(ctx, &UpdateCanvasRequest{
		SetLayoutMetadata: map[string]*CanvasLayoutMetadata{
			"new": {
				StableNodeId:    "spell-run:new",
				Lane:            "source",
				Rank:            0,
				Group:           "workflow",
				ProjectionOwner: "glados/workflow",
			},
		},
		RemoveLayoutMetadataNodeIds: []string{"old"},
		RemoveNodeIds:               []string{"keep"},
	})
	if err != nil {
		t.Fatal(err)
	}
	state := resp.GetState()
	if state.GetNodes()["keep"] != nil {
		t.Fatal("expected removed node to be absent")
	}
	if state.GetLayoutMetadata()["old"] != nil {
		t.Fatal("expected explicitly removed layout metadata to be absent")
	}
	if state.GetLayoutMetadata()["keep"] != nil {
		t.Fatal("expected removed node layout metadata to be absent")
	}
	if got := requireCanvasLayoutMetadata(t, state, "new").GetLane(); got != "source" {
		t.Fatalf("expected new layout metadata lane source, got %q", got)
	}
}

func TestUpdateCanvasAcceptsEmptyLegacyLayoutMetadata(t *testing.T) {
	ctx := context.Background()
	resource := NewCanvasResource(nil, nil, "", &CanvasState{
		Nodes: map[string]*CanvasNode{
			"manual": {Id: "manual", TextContent: "manual"},
		},
	})

	resp, err := resource.UpdateCanvas(ctx, &UpdateCanvasRequest{
		SetLayoutMetadata: map[string]*CanvasLayoutMetadata{
			"workflow": {
				StableNodeId:    "spell-run:workflow",
				Lane:            "source",
				Rank:            0,
				Group:           "workflow",
				ProjectionOwner: "glados/workflow",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	requireCanvasNode(t, resp.GetState(), "manual")
	if got := requireCanvasLayoutMetadata(t, resp.GetState(), "workflow").GetStableNodeId(); got != "spell-run:workflow" {
		t.Fatalf("expected metadata to be added to legacy empty map, got %q", got)
	}
}

func TestCanvasHiddenGraphLinksJSONRoundTrip(t *testing.T) {
	link := &HiddenGraphLink{
		Subject:   "<objects/a>",
		Predicate: "<relatedTo>",
		Object:    "<objects/b>",
		Label:     "main",
	}
	state := &CanvasState{
		HiddenGraphLinks: []*HiddenGraphLink{link},
		LayoutMetadata: map[string]*CanvasLayoutMetadata{
			"node-a": {
				StableNodeId:    "stable-a",
				Lane:            "audit",
				Rank:            3,
				Group:           "checks",
				ProjectionOwner: "glados/workflow",
			},
		},
	}

	data, err := state.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var decoded CanvasState
	if err := decoded.UnmarshalJSON(data); err != nil {
		t.Fatal(err)
	}
	if !state.EqualVT(&decoded) {
		t.Fatal("canvas state hidden graph links and layout metadata did not round trip through JSON")
	}

	req := &UpdateCanvasRequest{
		AddHiddenGraphLinks:    []*HiddenGraphLink{link},
		RemoveHiddenGraphLinks: []*HiddenGraphLink{link.CloneVT()},
		SetLayoutMetadata: map[string]*CanvasLayoutMetadata{
			"node-a": {
				StableNodeId:    "stable-a",
				Lane:            "audit",
				Rank:            3,
				Group:           "checks",
				ProjectionOwner: "glados/workflow",
			},
		},
		RemoveLayoutMetadataNodeIds: []string{"node-b"},
	}
	data, err = req.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var decodedReq UpdateCanvasRequest
	if err := decodedReq.UnmarshalJSON(data); err != nil {
		t.Fatal(err)
	}
	if !req.EqualVT(&decodedReq) {
		t.Fatal("update request hidden graph links did not round trip through JSON")
	}
}

func TestCanvasStorageMigratesFlatStateAndKeepsUnchangedNodeRef(t *testing.T) {
	for _, nodeCount := range []int{1, 100, 1000} {
		t.Run(strconv.Itoa(nodeCount), func(t *testing.T) {
			ctx := t.Context()
			initial := &CanvasState{
				Nodes:         make(map[string]*CanvasNode, nodeCount),
				Edges:         []*CanvasEdge{{Id: "edge", SourceNodeId: "node-0000", TargetNodeId: "node-0000"}},
				StrokeTreeRef: []byte("stroke-root"),
				HiddenGraphLinks: []*HiddenGraphLink{{
					Subject: "subject", Predicate: "predicate", Object: "object", Label: "label",
				}},
				LayoutMetadata: map[string]*CanvasLayoutMetadata{
					"node-0000": {StableNodeId: "node-0000", Lane: "main", Rank: 1},
				},
			}
			for i := range nodeCount {
				id := fmt.Sprintf("node-%04d", i)
				initial.Nodes[id] = &CanvasNode{Id: id, Width: float64(100 + i), Height: 100}
			}
			ws, release := setupCanvasWatchWorld(t, ctx, "canvas", initial)
			defer release()
			legacy, err := LookupCanvasState(ctx, ws, "canvas")
			if err != nil {
				t.Fatal(err)
			}
			if !legacy.EqualVT(initial) {
				t.Fatalf("legacy state has %d nodes, want %d", len(legacy.GetNodes()), nodeCount)
			}

			migrated := initial.CloneVT()
			migrated.Nodes["node-0000"].Width++
			writeCanvasStorageTestState(t, ctx, ws, "canvas", nil, migrated)
			got, err := LookupCanvasState(ctx, ws, "canvas")
			if err != nil {
				t.Fatal(err)
			}
			if !got.EqualVT(migrated) {
				t.Fatalf("migrated state has %d nodes, want %d", len(got.GetNodes()), nodeCount)
			}
			firstRefs := canvasStorageNodeRefs(t, ctx, ws, "canvas")

			next := migrated.CloneVT()
			next.Nodes["node-0000"].Width++
			next.Nodes["added"] = &CanvasNode{Id: "added", Width: 80, Height: 80}
			var removed string
			if nodeCount > 2 {
				removed = fmt.Sprintf("node-%04d", nodeCount-1)
				delete(next.Nodes, removed)
			}
			writeCanvasStorageTestState(t, ctx, ws, "canvas", nil, next)
			secondRefs := canvasStorageNodeRefs(t, ctx, ws, "canvas")
			if _, found := secondRefs[removed]; removed != "" && found {
				t.Fatalf("removed node %q remains in the node DAG", removed)
			}
			if firstRefs["node-0000"].EqualsRef(secondRefs["node-0000"]) {
				t.Fatal("changed node kept its old block ref")
			}
			if nodeCount > 1 {
				unchanged := "node-0001"
				if !firstRefs[unchanged].EqualsRef(secondRefs[unchanged]) {
					t.Fatalf("unchanged node %q block ref changed", unchanged)
				}
			}
		})
	}
}

func TestCanvasStorageMigratesEmptyLegacyState(t *testing.T) {
	ctx := t.Context()
	initial := &CanvasState{}
	ws, release := setupCanvasWatchWorld(t, ctx, "canvas", initial)
	defer release()

	legacy, err := LookupCanvasState(ctx, ws, "canvas")
	if err != nil {
		t.Fatal(err)
	}
	if !legacy.EqualVT(initial) {
		t.Fatalf("empty legacy state = %#v", legacy)
	}
	next := &CanvasState{Edges: []*CanvasEdge{{Id: "edge"}}}
	writeCanvasStorageTestState(t, ctx, ws, "canvas", nil, next)
	got, err := LookupCanvasState(ctx, ws, "canvas")
	if err != nil {
		t.Fatal(err)
	}
	if !got.EqualVT(next) {
		t.Fatalf("migrated empty state = %#v, want %#v", got, next)
	}
}

func writeCanvasStorageTestState(
	t *testing.T,
	ctx context.Context,
	ws *world_block.WorldState,
	objKey string,
	previous, next *CanvasState,
) {
	t.Helper()
	_, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		return WriteCanvasState(ctx, bcs, previous, next)
	})
	if err == nil {
		err = ws.Commit(ctx)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func canvasStorageNodeRefs(
	t *testing.T,
	ctx context.Context,
	ws *world_block.WorldState,
	objKey string,
) map[string]*block.BlockRef {
	t.Helper()
	refs := make(map[string]*block.BlockRef)
	_, _, err := world.AccessWorldObject(ctx, ws, objKey, false, func(bcs *block.Cursor) error {
		storage, err := UnmarshalCanvasStorage(ctx, bcs)
		if err != nil {
			return err
		}
		if got, want := storage.GetNodes().GetImplType(), block_kvtx.DefaultKeyValueStoreImplForWorkload(block_kvtx.WorkloadClassWriteChurn); got != want {
			t.Fatalf("Canvas node backend = %s, want workload policy %s", got, want)
		}
		tx, err := block_kvtx.BuildKvTransaction(ctx, bcs.FollowSubBlock(2), false)
		if err != nil {
			return err
		}
		defer tx.Discard()
		it := tx.BlockIterate(ctx, nil, false, false)
		defer it.Close()
		for it.Next() {
			refs[string(it.Key())] = it.ValueCursor().GetRef().CloneVT()
		}
		return it.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	return refs
}

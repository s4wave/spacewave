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
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
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
	// Bound the wait for a Canvas stream value.
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Second)
	defer cancel()

	// Receive the Canvas stream value or report its closed or expired wait.
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

// setupCanvasWatchWorld creates objKey holding state in an engine-backed
// World. Each write commits its own transaction, so a watcher observes the
// seqno only after the change is readable.
func setupCanvasWatchWorld(
	t *testing.T,
	ctx context.Context,
	objKey string,
	state *CanvasState,
) (world.WorldState, func()) {
	// Start an engine World with transactional writes.
	t.Helper()
	tb, err := world_testbed.Default(ctx, world_testbed.WithWorldVerbose(false))
	if err != nil {
		t.Fatal(err.Error())
	}

	// Create the canvas object holding the initial state.
	ws := tb.WorldState
	createdObject, _, err := world.CreateWorldObject(ctx, ws, objKey, func(bcs *block.Cursor) error {
		return WriteCanvasState(ctx, bcs, nil, state)
	})
	world.ReleaseObjectState(createdObject)
	if err != nil {
		tb.Release()
		t.Fatal(err.Error())
	}
	return ws, tb.Release
}

func setCanvasWatchWorldState(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	state *CanvasState,
) {
	t.Helper()

	_, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		return WriteCanvasState(ctx, bcs, nil, state)
	})
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
	// Consume Canvas watch snapshots until the requested node arrives.
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
	// Create the initial Canvas World with layout metadata.
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
				ProjectionOwner: "gizmo/workflow",
			},
		},
	}
	ws, cleanup := setupCanvasWatchWorld(t, ctx, objKey, initial)
	t.Cleanup(cleanup)

	// Create the Canvas resource and cancellable stream contexts.
	resource := NewCanvasResource(ws, nil, objKey, initial)
	t.Cleanup(resource.Close)
	streamCtxA, cancelA := context.WithCancel(ctx)
	defer cancelA()
	streamCtxB, cancelB := context.WithCancel(ctx)
	defer cancelB()

	// Start two state watches on the shared Canvas resource.
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

	// Verify both Canvas watches receive the initial node and layout metadata.
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

	// Commit updated Canvas nodes and layout metadata to the World.
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
				ProjectionOwner: "gizmo/workflow",
			},
		},
	}
	setCanvasWatchWorldState(t, ctx, ws, objKey, updated)

	// Verify both Canvas watches receive the updated layout metadata.
	updateA := recvCanvasTestValue(t, strmA.sent, "stream A update").GetState()
	requireCanvasNode(t, updateA, "updated")
	if got := requireCanvasLayoutMetadata(t, updateA, "updated").GetRank(); got != 7 {
		t.Fatalf("expected stream A updated rank 7, got %d", got)
	}
	updateB := recvCanvasTestValue(t, strmB.sent, "stream B update").GetState()
	requireCanvasNode(t, updateB, "updated")
	if got := requireCanvasLayoutMetadata(t, updateB, "updated").GetProjectionOwner(); got != "gizmo/workflow" {
		t.Fatalf("expected stream B updated projection owner, got %q", got)
	}

	// Commit two successive Canvas states to the World.
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

	// Verify both Canvas watches reach the latest committed node.
	requireCanvasNodeEventually(t, strmA.sent, doneA, "stream A burst update", "burst-b")
	requireCanvasNodeEventually(t, strmB.sent, doneB, "stream B burst update", "burst-b")

	// Close the Canvas resource and verify both watches stop.
	resource.Close()
	if err := recvCanvasTestValue(t, doneA, "stream A close"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected stream A context.Canceled, got %v", err)
	}
	if err := recvCanvasTestValue(t, doneB, "stream B close"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected stream B context.Canceled, got %v", err)
	}
}

func TestCanvasResourceCloseCancelsWatchersImmediately(t *testing.T) {
	// Start a Canvas state watch on an empty resource.
	ctx := t.Context()
	resource := NewCanvasResource(nil, nil, "", &CanvasState{})
	streamCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	strm := newCanvasStateStream(streamCtx)
	done := make(chan error, 1)
	go func() {
		done <- resource.WatchCanvasState(&WatchCanvasStateRequest{}, strm)
	}()

	// Close the Canvas resource after receiving its initial snapshot.
	recvCanvasTestValue(t, strm.sent, "initial canvas snapshot")
	resource.Close()
	if err := recvCanvasTestValue(t, done, "watch close"); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}

	// Verify a late Canvas watcher observes resource cancellation.
	late := newCanvasStateStream(ctx)
	if err := resource.WatchCanvasState(&WatchCanvasStateRequest{}, late); !errors.Is(err, context.Canceled) {
		t.Fatalf("expected late watcher context.Canceled, got %v", err)
	}
}

func TestCanvasResourceClosePreventsLateStatePublish(t *testing.T) {
	// Close an empty Canvas resource before publishing another state.
	resource := NewCanvasResource(nil, nil, "", &CanvasState{})
	resource.Close()

	// Verify a late Canvas state cannot reopen the closed resource.
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
	// Prepare an empty Canvas and a hidden graph link.
	ctx := context.Background()
	link := &HiddenGraphLink{
		Subject:   "<objects/a>",
		Predicate: "<relatedTo>",
		Object:    "<objects/b>",
		Label:     "main",
	}
	resource := NewCanvasResource(nil, nil, "", &CanvasState{})

	// Add duplicate hidden graph links to the Canvas.
	resp, err := resource.UpdateCanvas(ctx, &UpdateCanvasRequest{
		AddHiddenGraphLinks: []*HiddenGraphLink{link, link.CloneVT()},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Canvas retains one copy of the hidden graph link.
	if got := len(resp.GetState().GetHiddenGraphLinks()); got != 1 {
		t.Fatalf("expected one hidden graph link after duplicate add, got %d", got)
	}

	// Remove the hidden graph link from the Canvas.
	resp, err = resource.UpdateCanvas(ctx, &UpdateCanvasRequest{
		RemoveHiddenGraphLinks: []*HiddenGraphLink{link.CloneVT()},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Canvas has no remaining hidden graph links.
	if got := len(resp.GetState().GetHiddenGraphLinks()); got != 0 {
		t.Fatalf("expected no hidden graph links after remove, got %d", got)
	}
}

func TestUpdateCanvasHiddenGraphLinksPreservesManualEdges(t *testing.T) {
	// Prepare a Canvas with a manual edge and a hidden graph link.
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

	// Add the hidden graph link to the Canvas.
	resp, err := resource.UpdateCanvas(ctx, &UpdateCanvasRequest{
		AddHiddenGraphLinks: []*HiddenGraphLink{link},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Canvas preserves its manual edge and hidden graph link.
	if got := len(resp.GetState().GetEdges()); got != 1 {
		t.Fatalf("expected manual edge to be preserved, got %d edges", got)
	}
	if got := len(resp.GetState().GetHiddenGraphLinks()); got != 1 {
		t.Fatalf("expected hidden graph link, got %d", got)
	}
}

func TestUpdateCanvasPreservesLayoutMetadata(t *testing.T) {
	// Prepare a Canvas with nodes, edges, and layout metadata.
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
				ProjectionOwner: "gizmo/workflow",
			},
		},
	})

	// Add a Canvas node, visual edge, and hidden graph link.
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

	// Verify the Canvas retains existing and added nodes and edges.
	state := resp.GetState()
	requireCanvasNode(t, state, "source")
	requireCanvasNode(t, state, "proof")
	if got := len(state.GetEdges()); got != 2 {
		t.Fatalf("expected existing and added visual edges, got %d", got)
	}
	if got := len(state.GetHiddenGraphLinks()); got != 2 {
		t.Fatalf("expected existing and added hidden graph links, got %d", got)
	}

	// Verify the original Canvas layout metadata is preserved.
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
	if got := meta.GetProjectionOwner(); got != "gizmo/workflow" {
		t.Fatalf("expected projection owner to be preserved, got %q", got)
	}
}

func TestUpdateCanvasMutatesLayoutMetadata(t *testing.T) {
	// Prepare a Canvas with layout metadata for two nodes.
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
				ProjectionOwner: "gizmo/workflow",
			},
			"keep": {
				StableNodeId:    "spell-run:keep",
				Lane:            "proof",
				Rank:            2,
				Group:           "workflow",
				ProjectionOwner: "gizmo/workflow",
			},
		},
	})

	// Replace Canvas layout metadata and remove an existing node.
	resp, err := resource.UpdateCanvas(ctx, &UpdateCanvasRequest{
		SetLayoutMetadata: map[string]*CanvasLayoutMetadata{
			"new": {
				StableNodeId:    "spell-run:new",
				Lane:            "source",
				Rank:            0,
				Group:           "workflow",
				ProjectionOwner: "gizmo/workflow",
			},
		},
		RemoveLayoutMetadataNodeIds: []string{"old"},
		RemoveNodeIds:               []string{"keep"},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify Canvas node removal also removes its layout metadata.
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

func TestUpdateCanvasAcceptsEmptyLayoutMetadata(t *testing.T) {
	// Prepare a Canvas with a manual node and no layout metadata.
	ctx := context.Background()
	resource := NewCanvasResource(nil, nil, "", &CanvasState{
		Nodes: map[string]*CanvasNode{
			"manual": {Id: "manual", TextContent: "manual"},
		},
	})

	// Add workflow layout metadata to the Canvas.
	resp, err := resource.UpdateCanvas(ctx, &UpdateCanvasRequest{
		SetLayoutMetadata: map[string]*CanvasLayoutMetadata{
			"workflow": {
				StableNodeId:    "spell-run:workflow",
				Lane:            "source",
				Rank:            0,
				Group:           "workflow",
				ProjectionOwner: "gizmo/workflow",
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Verify the Canvas retains its manual node and new metadata.
	requireCanvasNode(t, resp.GetState(), "manual")
	if got := requireCanvasLayoutMetadata(t, resp.GetState(), "workflow").GetStableNodeId(); got != "spell-run:workflow" {
		t.Fatalf("expected metadata to be added to empty map, got %q", got)
	}
}

func TestCanvasHiddenGraphLinksJSONRoundTrip(t *testing.T) {
	// Prepare Canvas state with hidden graph links and layout metadata.
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
				ProjectionOwner: "gizmo/workflow",
			},
		},
	}

	// Round trip the Canvas state through its JSON codec.
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

	// Prepare an update request with graph link and layout metadata changes.
	req := &UpdateCanvasRequest{
		AddHiddenGraphLinks:    []*HiddenGraphLink{link},
		RemoveHiddenGraphLinks: []*HiddenGraphLink{link.CloneVT()},
		SetLayoutMetadata: map[string]*CanvasLayoutMetadata{
			"node-a": {
				StableNodeId:    "stable-a",
				Lane:            "audit",
				Rank:            3,
				Group:           "checks",
				ProjectionOwner: "gizmo/workflow",
			},
		},
		RemoveLayoutMetadataNodeIds: []string{"node-b"},
	}

	// Round trip the Canvas update request through its JSON codec.
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

func TestCanvasStorageUpdatesNodeDAGAndKeepsUnchangedNodeRef(t *testing.T) {
	for _, nodeCount := range []int{1, 100, 1000} {
		t.Run(strconv.Itoa(nodeCount), func(t *testing.T) {
			// Populate a Canvas with the requested number of nodes.
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

			// Store the initial Canvas in the World.
			ws, release := setupCanvasWatchWorld(t, ctx, "canvas", initial)
			defer release()

			// Verify the stored Canvas matches its initial state.
			stored, err := LookupCanvasState(ctx, ws, "canvas")
			if err != nil {
				t.Fatal(err)
			}
			if !stored.EqualVT(initial) {
				t.Fatalf("stored state has %d nodes, want %d", len(stored.GetNodes()), nodeCount)
			}

			// Change one Canvas node and write its updated state.
			firstUpdate := initial.CloneVT()
			firstUpdate.Nodes["node-0000"].Width++
			writeCanvasStorageTestState(t, ctx, ws, "canvas", nil, firstUpdate)

			// Verify the stored Canvas reflects the first node update.
			got, err := LookupCanvasState(ctx, ws, "canvas")
			if err != nil {
				t.Fatal(err)
			}
			if !got.EqualVT(firstUpdate) {
				t.Fatalf("stored state has %d nodes, want %d", len(got.GetNodes()), nodeCount)
			}

			// Capture the node and DAG references after the first update.
			firstRefs := canvasStorageNodeRefs(t, ctx, ws, "canvas")
			firstDagRefs := canvasStorageNodeDagRefs(t, ctx, ws, "canvas")

			// Write another update to the same Canvas node.
			next := firstUpdate.CloneVT()
			next.Nodes["node-0000"].Width++
			writeCanvasStorageTestState(t, ctx, ws, "canvas", nil, next)

			// Count the DAG references changed by the second node update.
			secondRefs := canvasStorageNodeRefs(t, ctx, ws, "canvas")
			secondDagRefs := canvasStorageNodeDagRefs(t, ctx, ws, "canvas")
			changedDagRefs := 0
			for ref := range secondDagRefs {
				if _, found := firstDagRefs[ref]; !found {
					changedDagRefs++
				}
			}

			// Verify the single-node update preserves unchanged node references.
			t.Logf("nodes=%d DAG refs=%d changed refs=%d", nodeCount, len(secondDagRefs), changedDagRefs)
			if changedDagRefs >= nodeCount && nodeCount > 1 {
				t.Fatalf("single-node update changed %d DAG refs for %d nodes", changedDagRefs, nodeCount)
			}
			if firstRefs["node-0000"].EqualsRef(secondRefs["node-0000"]) {
				t.Fatal("changed node kept its old block ref")
			}
			if nodeCount > 1 && !firstRefs["node-0001"].EqualsRef(secondRefs["node-0001"]) {
				t.Fatal("unchanged node block ref changed")
			}

			// Prepare Canvas node additions and removals.
			withMembershipChanges := next.CloneVT()
			withMembershipChanges.Nodes["added"] = &CanvasNode{Id: "added", Width: 80, Height: 80}
			var removed string
			if nodeCount > 2 {
				removed = fmt.Sprintf("node-%04d", nodeCount-1)
				delete(withMembershipChanges.Nodes, removed)
			}

			// Write the Canvas membership changes and capture their references.
			writeCanvasStorageTestState(t, ctx, ws, "canvas", nil, withMembershipChanges)
			membershipRefs := canvasStorageNodeRefs(t, ctx, ws, "canvas")
			membershipDagRefs := canvasStorageNodeDagRefs(t, ctx, ws, "canvas")

			// Verify added nodes are reachable and removed nodes are absent.
			if _, found := membershipRefs["added"]; !found {
				t.Fatal("added node is missing from the node DAG")
			}
			if _, found := membershipRefs[removed]; removed != "" && found {
				t.Fatalf("removed node %q remains in the node DAG", removed)
			}
			if removed != "" {
				removedRef := firstRefs[removed].MarshalString()
				if _, reachable := membershipDagRefs[removedRef]; reachable {
					t.Fatalf("removed node %q block ref remains reachable", removed)
				}
			}
		})
	}
}

func TestCanvasStorageRootIsDeterministic(t *testing.T) {
	// Populate equivalent Canvas nodes in opposite insertion orders.
	const nodeCount = 100
	ascending := &CanvasState{Nodes: make(map[string]*CanvasNode, nodeCount)}
	descending := &CanvasState{Nodes: make(map[string]*CanvasNode, nodeCount)}
	for i := range nodeCount {
		id := fmt.Sprintf("node-%04d", i)
		ascending.Nodes[id] = &CanvasNode{Id: id, Width: float64(i)}
	}
	for i := nodeCount - 1; i >= 0; i-- {
		id := fmt.Sprintf("node-%04d", i)
		descending.Nodes[id] = &CanvasNode{Id: id, Width: float64(i)}
	}

	// Verify Canvas storage produces the same root for both orders.
	first := canvasStorageRoot(t, ascending)
	second := canvasStorageRoot(t, descending)
	if first != second {
		t.Fatalf("Canvas storage roots differ: %s != %s", first, second)
	}
}

func canvasStorageRoot(t *testing.T, state *CanvasState) string {
	// Create a World for the Canvas storage root check.
	t.Helper()
	ctx := t.Context()
	ws, release := setupCanvasWatchWorld(t, ctx, "canvas", state)
	defer release()

	// Write the Canvas state and read its root reference.
	writeCanvasStorageTestState(t, ctx, ws, "canvas", nil, state)
	var root string
	_, _, err := world.AccessWorldObject(ctx, ws, "canvas", false, func(bcs *block.Cursor) error {
		root = bcs.GetRef().MarshalString()
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestCanvasStorageWritesEmptyNodeIndex(t *testing.T) {
	// Create an empty Canvas World for node index checks.
	ctx := t.Context()
	initial := &CanvasState{}
	ws, release := setupCanvasWatchWorld(t, ctx, "canvas", initial)
	defer release()

	// Verify the empty Canvas state can be read back.
	stored, err := LookupCanvasState(ctx, ws, "canvas")
	if err != nil {
		t.Fatal(err)
	}
	if !stored.EqualVT(initial) {
		t.Fatalf("empty stored state = %#v", stored)
	}

	// Write Canvas edges while leaving the node index empty.
	next := &CanvasState{Edges: []*CanvasEdge{{Id: "edge"}}}
	writeCanvasStorageTestState(t, ctx, ws, "canvas", nil, next)

	// Verify Canvas edges survive storage with an empty node index.
	got, err := LookupCanvasState(ctx, ws, "canvas")
	if err != nil {
		t.Fatal(err)
	}
	if !got.EqualVT(next) {
		t.Fatalf("stored empty state = %#v, want %#v", got, next)
	}
}

func TestCanvasStorageWritesNilMessageValues(t *testing.T) {
	// Create an empty Canvas World for nil message storage checks.
	ctx := t.Context()
	initial := &CanvasState{}
	ws, release := setupCanvasWatchWorld(t, ctx, "canvas", initial)
	defer release()

	// Write Canvas state containing nil message entries.
	next := &CanvasState{
		Edges:            []*CanvasEdge{nil},
		HiddenGraphLinks: []*HiddenGraphLink{nil},
		LayoutMetadata: map[string]*CanvasLayoutMetadata{
			"node": nil,
		},
	}
	writeCanvasStorageTestState(t, ctx, ws, "canvas", nil, next)

	// Verify Canvas storage retains nil edge, link, and metadata entries.
	got, err := LookupCanvasState(ctx, ws, "canvas")
	if err != nil {
		t.Fatal(err)
	}
	if len(got.GetEdges()) != 1 {
		t.Fatalf("edges = %d, want 1", len(got.GetEdges()))
	}
	if len(got.GetHiddenGraphLinks()) != 1 {
		t.Fatalf("hidden graph links = %d, want 1", len(got.GetHiddenGraphLinks()))
	}
	if _, found := got.GetLayoutMetadata()["node"]; !found {
		t.Fatal("nil layout metadata entry was dropped")
	}
}

func writeCanvasStorageTestState(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
	previous, next *CanvasState,
) {
	t.Helper()
	_, _, err := world.AccessWorldObject(ctx, ws, objKey, true, func(bcs *block.Cursor) error {
		return WriteCanvasState(ctx, bcs, previous, next)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func canvasStorageNodeDagRefs(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
) map[string]struct{} {
	// Collect the reachable node DAG references from the Canvas World.
	t.Helper()
	refs := make(map[string]struct{})
	_, _, err := world.AccessWorldObject(ctx, ws, objKey, false, func(bcs *block.Cursor) error {
		// Load the Canvas storage and open its node transaction.
		if _, err := UnmarshalCanvasStorage(ctx, bcs); err != nil {
			return err
		}
		nodes, err := block_kvtx.BuildKvTransaction(ctx, bcs.FollowSubBlock(1), false)
		if err != nil {
			return err
		}
		defer nodes.Discard()

		// Read each Canvas node to load its block references.
		it := nodes.BlockIterate(ctx, nil, false, false)
		for it.Next() {
			if _, err := block.UnmarshalBlock[*CanvasNode](ctx, it.ValueCursor(), NewCanvasNodeBlock); err != nil {
				it.Close()
				return err
			}
		}
		if err := it.Err(); err != nil {
			it.Close()
			return err
		}
		it.Close()

		// Collect each unique node DAG reference and its descendants.
		var collect func(*block.Cursor) error
		collect = func(cursor *block.Cursor) error {
			// Record the current Canvas DAG block unless it was already visited.
			if ref := cursor.GetRef(); ref != nil && !ref.GetEmpty() {
				key := ref.MarshalString()
				if _, found := refs[key]; found {
					return nil
				}
				refs[key] = struct{}{}
			}

			// Read the Canvas DAG children and collect their references.
			children, err := cursor.GetAllRefs(false)
			if err != nil {
				return err
			}
			for _, child := range children {
				if err := collect(child); err != nil {
					return err
				}
			}
			return nil
		}
		return collect(bcs.FollowSubBlock(1))
	})
	if err != nil {
		t.Fatal(err)
	}
	return refs
}

func canvasStorageNodeRefs(
	t *testing.T,
	ctx context.Context,
	ws world.WorldState,
	objKey string,
) map[string]*block.BlockRef {
	// Collect each Canvas node block reference from the World.
	t.Helper()
	refs := make(map[string]*block.BlockRef)
	_, _, err := world.AccessWorldObject(ctx, ws, objKey, false, func(bcs *block.Cursor) error {
		// Verify the Canvas node index uses the write-churn backend.
		storage, err := UnmarshalCanvasStorage(ctx, bcs)
		if err != nil {
			return err
		}
		if got, want := storage.GetNodes().GetImplType(), block_kvtx.DefaultKeyValueStoreImplForWorkload(block_kvtx.WorkloadClassWriteChurn); got != want {
			t.Fatalf("Canvas node backend = %s, want workload policy %s", got, want)
		}

		// Open a read transaction on the Canvas node index.
		tx, err := block_kvtx.BuildKvTransaction(ctx, bcs.FollowSubBlock(1), false)
		if err != nil {
			return err
		}
		defer tx.Discard()

		// Read and retain each Canvas node block reference.
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

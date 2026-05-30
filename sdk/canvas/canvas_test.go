package s4wave_canvas

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/db/block"
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
			"initial": &CanvasNode{Id: "initial", TextContent: "initial"},
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

	requireCanvasNode(t, recvCanvasTestValue(t, strmA.sent, "stream A initial").GetState(), "initial")
	requireCanvasNode(t, recvCanvasTestValue(t, strmB.sent, "stream B initial").GetState(), "initial")

	updated := &CanvasState{
		Nodes: map[string]*CanvasNode{
			"updated": &CanvasNode{Id: "updated", TextContent: "updated"},
		},
	}
	setCanvasWatchWorldState(t, ctx, ws, objKey, updated)
	requireCanvasNode(t, recvCanvasTestValue(t, strmA.sent, "stream A update").GetState(), "updated")
	requireCanvasNode(t, recvCanvasTestValue(t, strmB.sent, "stream B update").GetState(), "updated")

	burstA := &CanvasState{
		Nodes: map[string]*CanvasNode{
			"burst-a": &CanvasNode{Id: "burst-a", TextContent: "burst-a"},
		},
	}
	burstB := &CanvasState{
		Nodes: map[string]*CanvasNode{
			"burst-b": &CanvasNode{Id: "burst-b", TextContent: "burst-b"},
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
			"late": &CanvasNode{Id: "late"},
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

func TestCanvasHiddenGraphLinksJSONRoundTrip(t *testing.T) {
	link := &HiddenGraphLink{
		Subject:   "<objects/a>",
		Predicate: "<relatedTo>",
		Object:    "<objects/b>",
		Label:     "main",
	}
	state := &CanvasState{
		HiddenGraphLinks: []*HiddenGraphLink{link},
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
		t.Fatal("canvas state hidden graph links did not round trip through JSON")
	}

	req := &UpdateCanvasRequest{
		AddHiddenGraphLinks:    []*HiddenGraphLink{link},
		RemoveHiddenGraphLinks: []*HiddenGraphLink{link.CloneVT()},
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

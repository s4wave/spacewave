package s4wave_canvas

import (
	"context"
	"testing"
)

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

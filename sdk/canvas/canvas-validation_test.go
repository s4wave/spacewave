package s4wave_canvas

import (
	"context"
	"math"
	"strings"
	"testing"
)

func validCanvasGeometryNode() *CanvasNode {
	return &CanvasNode{
		Id: "shape", X: 1, Y: 2, Width: 100, Height: 80,
		Type: NodeType_NODE_TYPE_SHAPE,
		Geometry: &CanvasGeometry{
			Kind:   CanvasGeometryKind_CANVAS_GEOMETRY_KIND_RECTANGLE,
			Color:  "#123456",
			Points: []*CanvasPoint{{X: 0, Y: 0}, {X: 100, Y: 80}},
		},
	}
}

func TestUpdateCanvasValidatesNodesBeforeMutation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*CanvasNode)
	}{
		{"non-finite position", func(node *CanvasNode) { node.X = math.NaN() }},
		{"coordinate bound", func(node *CanvasNode) { node.Y = maxCanvasCoordinate + 1 }},
		{"zero dimension", func(node *CanvasNode) { node.Width = 0 }},
		{"dimension bound", func(node *CanvasNode) { node.Height = maxCanvasDimension + 1 }},
		{"unknown node type", func(node *CanvasNode) { node.Type = NodeType_NODE_TYPE_UNKNOWN }},
		{"node ID bound", func(node *CanvasNode) { node.Id = strings.Repeat("x", maxCanvasNodeIDBytes+1) }},
		{"text bound", func(node *CanvasNode) { node.TextContent = strings.Repeat("x", maxCanvasTextBytes+1) }},
		{"object key bound", func(node *CanvasNode) { node.ObjectKey = strings.Repeat("x", maxCanvasPathBytes+1) }},
		{"view path bound", func(node *CanvasNode) { node.ViewPath = strings.Repeat("x", maxCanvasPathBytes+1) }},
		{"unknown geometry kind", func(node *CanvasNode) { node.Geometry.Kind = CanvasGeometryKind_CANVAS_GEOMETRY_KIND_UNKNOWN }},
		{"invalid color", func(node *CanvasNode) { node.Geometry.Color = "url(https://invalid)" }},
		{"too few points", func(node *CanvasNode) { node.Geometry.Points = node.Geometry.Points[:1] }},
		{"too many points", func(node *CanvasNode) { node.Geometry.Points = make([]*CanvasPoint, maxCanvasGeometryPoints+1) }},
		{"non-finite point", func(node *CanvasNode) { node.Geometry.Points[0].X = math.Inf(1) }},
		{"point bound", func(node *CanvasNode) { node.Geometry.Points[0].Y = maxCanvasCoordinate + 1 }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			node := validCanvasGeometryNode()
			test.mutate(node)
			resource := NewCanvasResource(nil, nil, "", &CanvasState{})
			_, err := resource.UpdateCanvas(context.Background(), &UpdateCanvasRequest{
				SetNodes: map[string]*CanvasNode{node.GetId(): node},
			})
			if err == nil {
				t.Fatal("expected invalid node to be rejected")
			}
			state, stateErr := resource.GetCanvasState(context.Background(), &GetCanvasStateRequest{})
			if stateErr != nil {
				t.Fatal(stateErr)
			}
			if len(state.GetState().GetNodes()) != 0 {
				t.Fatal("invalid mutation changed Canvas state")
			}
		})
	}
}

func TestUpdateCanvasAcceptsBoundedGeometryAndDualWritesLegacyField(t *testing.T) {
	resource := NewCanvasResource(nil, nil, "", &CanvasState{})
	node := validCanvasGeometryNode()
	response, err := resource.UpdateCanvas(context.Background(), &UpdateCanvasRequest{
		SetNodes: map[string]*CanvasNode{node.GetId(): node},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := response.GetState().GetNodes()[node.GetId()]
	if stored.GetGeometry() == nil {
		t.Fatal("authoritative geometry was not retained")
	}
	if got := string(stored.GetShapeData()); !strings.Contains(got, `"kind":"rectangle"`) ||
		!strings.Contains(got, `"color":"#123456"`) {
		t.Fatalf("legacy geometry was not dual-written: %s", got)
	}
	if len(node.GetShapeData()) != 0 {
		t.Fatal("UpdateCanvas mutated its request")
	}
}

func TestUpdateCanvasAcceptsLegacyAndCurrentEdgeStyles(t *testing.T) {
	for _, style := range []EdgeStyle{
		EdgeStyle_EDGE_STYLE_LEGACY_BEZIER,
		EdgeStyle_EDGE_STYLE_STRAIGHT,
		EdgeStyle_EDGE_STYLE_BEZIER,
	} {
		resource := NewCanvasResource(nil, nil, "", &CanvasState{})
		_, err := resource.UpdateCanvas(context.Background(), &UpdateCanvasRequest{
			AddEdges: []*CanvasEdge{{Id: "edge", Style: style}},
		})
		if err != nil {
			t.Fatalf("style %v: %v", style, err)
		}
	}
}

func legacyCanvasGeometryNode(data string) *CanvasNode {
	return &CanvasNode{
		Id: "legacy", X: 1, Y: 2, Width: 100, Height: 80,
		Type: NodeType_NODE_TYPE_DRAWING, ShapeData: []byte(data),
	}
}

func TestUpdateCanvasPromotesReleasedLegacyGeometryForms(t *testing.T) {
	tests := []struct {
		name  string
		data  string
		kind  CanvasGeometryKind
		color string
	}{
		{
			name:  "released point array",
			data:  `[ {"x":0,"y":1}, {"x":2,"y":3} ]`,
			kind:  CanvasGeometryKind_CANVAS_GEOMETRY_KIND_PEN,
			color: "currentColor",
		},
		{
			name:  "geometry object",
			data:  `{"kind":"rectangle","color":"#123456","points":[{"x":0,"y":1},{"x":2,"y":3}]}`,
			kind:  CanvasGeometryKind_CANVAS_GEOMETRY_KIND_RECTANGLE,
			color: "#123456",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			resource := NewCanvasResource(nil, nil, "", &CanvasState{})
			node := legacyCanvasGeometryNode(test.data)
			originalShapeData := append([]byte(nil), node.GetShapeData()...)
			response, err := resource.UpdateCanvas(context.Background(), &UpdateCanvasRequest{
				SetNodes: map[string]*CanvasNode{node.GetId(): node},
			})
			if err != nil {
				t.Fatal(err)
			}
			stored := response.GetState().GetNodes()[node.GetId()]
			if stored.GetGeometry().GetKind() != test.kind || stored.GetGeometry().GetColor() != test.color {
				t.Fatalf("unexpected promoted geometry: %v", stored.GetGeometry())
			}
			if got := string(stored.GetShapeData()); !strings.Contains(got, `"points":[`) {
				t.Fatalf("legacy geometry was not normalized: %s", got)
			}
			if node.GetGeometry() != nil || string(node.GetShapeData()) != string(originalShapeData) {
				t.Fatal("UpdateCanvas mutated its legacy request node")
			}
		})
	}
}

func TestUpdateCanvasRejectsInvalidLegacyGeometryBeforeMutation(t *testing.T) {
	tests := []struct {
		name string
		data string
	}{
		{"malformed", `{"kind":`},
		{"trailing value", `[{"x":0,"y":0},{"x":1,"y":1}] {}`},
		{"unknown object field", `{"kind":"pen","color":"currentColor","points":[{"x":0,"y":0},{"x":1,"y":1}],"unsafe":true}`},
		{"unknown kind", `{"kind":"spline","color":"currentColor","points":[{"x":0,"y":0},{"x":1,"y":1}]}`},
		{"unsafe color", `{"kind":"pen","color":"url(https://invalid)","points":[{"x":0,"y":0},{"x":1,"y":1}]}`},
		{"non-finite point", `{"kind":"pen","color":"currentColor","points":[{"x":1e999,"y":0},{"x":1,"y":1}]}`},
		{"out-of-bounds point", `{"kind":"pen","color":"currentColor","points":[{"x":1000000001,"y":0},{"x":1,"y":1}]}`},
		{"missing coordinate", `[{"x":0},{"x":1,"y":1}]`},
		{"too few points", `[{"x":0,"y":0}]`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			initial := validCanvasGeometryNode()
			initial.Id = "existing"
			resource := NewCanvasResource(nil, nil, "", &CanvasState{
				Nodes: map[string]*CanvasNode{initial.GetId(): initial},
			})
			node := legacyCanvasGeometryNode(test.data)
			_, err := resource.UpdateCanvas(context.Background(), &UpdateCanvasRequest{
				SetNodes: map[string]*CanvasNode{node.GetId(): node},
			})
			if err == nil {
				t.Fatal("expected invalid legacy geometry to be rejected")
			}
			state, stateErr := resource.GetCanvasState(context.Background(), &GetCanvasStateRequest{})
			if stateErr != nil {
				t.Fatal(stateErr)
			}
			if len(state.GetState().GetNodes()) != 1 || state.GetState().GetNodes()[initial.GetId()] == nil {
				t.Fatal("invalid legacy mutation changed Canvas state")
			}
		})
	}
}

func TestUpdateCanvasPrefersAuthoritativeGeometryAndNormalizesLegacyField(t *testing.T) {
	resource := NewCanvasResource(nil, nil, "", &CanvasState{})
	node := validCanvasGeometryNode()
	node.ShapeData = []byte(`{"kind":`)
	response, err := resource.UpdateCanvas(context.Background(), &UpdateCanvasRequest{
		SetNodes: map[string]*CanvasNode{node.GetId(): node},
	})
	if err != nil {
		t.Fatal(err)
	}
	stored := response.GetState().GetNodes()[node.GetId()]
	if stored.GetGeometry().GetKind() != CanvasGeometryKind_CANVAS_GEOMETRY_KIND_RECTANGLE {
		t.Fatal("authoritative geometry was not preferred")
	}
	if got := string(stored.GetShapeData()); !strings.Contains(got, `"kind":"rectangle"`) {
		t.Fatalf("legacy geometry was not normalized from field 13: %s", got)
	}
	if string(node.GetShapeData()) != `{"kind":` {
		t.Fatal("UpdateCanvas mutated its authoritative request node")
	}
}

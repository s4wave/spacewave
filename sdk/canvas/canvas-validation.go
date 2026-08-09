package s4wave_canvas

import (
	"math"
	"strconv"

	"github.com/aperturerobotics/fastjson"
	"github.com/pkg/errors"
)

const (
	maxCanvasMutationNodes  = 1000
	maxCanvasNodeIDBytes    = 1024
	maxCanvasTextBytes      = 1024 * 1024
	maxCanvasPathBytes      = 4096
	maxCanvasGeometryPoints = 10000
	maxCanvasDimension      = 1000000
	maxCanvasCoordinate     = 1000000000
)

// Validate rejects Canvas node mutations that cannot be safely persisted or rendered.
func (x *CanvasNode) Validate() error {
	if x == nil {
		return errors.New("canvas node is nil")
	}
	if len(x.GetId()) == 0 || len(x.GetId()) > maxCanvasNodeIDBytes {
		return errors.New("canvas node ID must contain between 1 and 1024 bytes")
	}
	if !finiteCanvasNumber(x.GetX()) || math.Abs(x.GetX()) > maxCanvasCoordinate ||
		!finiteCanvasNumber(x.GetY()) || math.Abs(x.GetY()) > maxCanvasCoordinate {
		return errors.New("canvas node position must be finite and within +/-1000000000")
	}
	if !finiteCanvasNumber(x.GetWidth()) || x.GetWidth() <= 0 || x.GetWidth() > maxCanvasDimension ||
		!finiteCanvasNumber(x.GetHeight()) || x.GetHeight() <= 0 || x.GetHeight() > maxCanvasDimension {
		return errors.New("canvas node dimensions must be finite, positive, and at most 1000000")
	}
	switch x.GetType() {
	case NodeType_NODE_TYPE_TEXT, NodeType_NODE_TYPE_SHAPE,
		NodeType_NODE_TYPE_WORLD_OBJECT, NodeType_NODE_TYPE_DRAWING:
	default:
		return errors.New("canvas node type is unknown or unsupported")
	}
	if len(x.GetTextContent()) > maxCanvasTextBytes {
		return errors.New("canvas node text content exceeds 1 MiB")
	}
	if len(x.GetObjectKey()) > maxCanvasPathBytes {
		return errors.New("canvas node object key exceeds 4096 bytes")
	}
	if len(x.GetViewPath()) > maxCanvasPathBytes {
		return errors.New("canvas node view path exceeds 4096 bytes")
	}
	if len(x.GetShapeData()) > maxCanvasTextBytes {
		return errors.New("canvas node legacy geometry exceeds 1 MiB")
	}
	if geometry := x.GetGeometry(); geometry != nil {
		if x.GetType() != NodeType_NODE_TYPE_SHAPE && x.GetType() != NodeType_NODE_TYPE_DRAWING {
			return errors.New("canvas geometry requires a shape or drawing node")
		}
		if err := geometry.Validate(); err != nil {
			return err
		}
	}
	return nil
}

// Validate rejects geometry that cannot be safely persisted or rendered.
func (x *CanvasGeometry) Validate() error {
	if x == nil {
		return errors.New("canvas geometry is nil")
	}
	switch x.GetKind() {
	case CanvasGeometryKind_CANVAS_GEOMETRY_KIND_PEN,
		CanvasGeometryKind_CANVAS_GEOMETRY_KIND_LINE,
		CanvasGeometryKind_CANVAS_GEOMETRY_KIND_ARROW,
		CanvasGeometryKind_CANVAS_GEOMETRY_KIND_RECTANGLE,
		CanvasGeometryKind_CANVAS_GEOMETRY_KIND_ELLIPSE:
	default:
		return errors.New("canvas geometry kind is unknown or unsupported")
	}
	if color := x.GetColor(); color != "currentColor" && !validCanvasHexColor(color) {
		return errors.New("canvas geometry color must be currentColor or a six-digit hexadecimal color")
	}
	if len(x.GetPoints()) < 2 || len(x.GetPoints()) > maxCanvasGeometryPoints {
		return errors.New("canvas geometry must contain between 2 and 10000 points")
	}
	for _, point := range x.GetPoints() {
		if point == nil || !finiteCanvasNumber(point.GetX()) || math.Abs(point.GetX()) > maxCanvasCoordinate ||
			!finiteCanvasNumber(point.GetY()) || math.Abs(point.GetY()) > maxCanvasCoordinate {
			return errors.New("canvas geometry points must be finite and within +/-1000000000")
		}
	}
	return nil
}

func validateCanvasUpdate(req *UpdateCanvasRequest) (map[string]*CanvasNode, error) {
	if req == nil {
		return nil, errors.New("update canvas request is nil")
	}
	if len(req.GetSetNodes()) > maxCanvasMutationNodes {
		return nil, errors.New("update canvas request exceeds 1000 set nodes")
	}
	nodes := make(map[string]*CanvasNode, len(req.GetSetNodes()))
	for id, node := range req.GetSetNodes() {
		if node == nil || id != node.GetId() {
			return nil, errors.Errorf("canvas node map key %q does not match node ID", id)
		}
		node = node.CloneVT()
		if node.Geometry == nil && len(node.ShapeData) != 0 {
			geometry, err := unmarshalLegacyCanvasGeometry(node.ShapeData)
			if err != nil {
				return nil, errors.Wrapf(err, "decode legacy geometry for canvas node %q", id)
			}
			node.Geometry = geometry
		}
		if node.Geometry != nil {
			node.ShapeData = marshalLegacyCanvasGeometry(node.Geometry)
		}
		if err := node.Validate(); err != nil {
			return nil, errors.Wrapf(err, "validate canvas node %q", id)
		}
		nodes[id] = node
	}
	for _, edge := range req.GetAddEdges() {
		if edge == nil {
			return nil, errors.New("canvas edge is nil")
		}
		switch edge.GetStyle() {
		case EdgeStyle_EDGE_STYLE_BEZIER, EdgeStyle_EDGE_STYLE_STRAIGHT:
		default:
			return nil, errors.New("canvas edge style is unsupported")
		}
	}
	return nodes, nil
}

func unmarshalLegacyCanvasGeometry(data []byte) (*CanvasGeometry, error) {
	value, err := fastjson.ParseBytes(data)
	if err != nil {
		return nil, errors.Wrap(err, "parse legacy canvas geometry JSON")
	}

	kind := "pen"
	color := "currentColor"
	var pointValues []*fastjson.Value
	switch value.Type() {
	case fastjson.TypeArray:
		pointValues, err = value.Array()
	case fastjson.TypeObject:
		object, objectErr := value.Object()
		if objectErr != nil {
			return nil, errors.Wrap(objectErr, "read legacy canvas geometry object")
		}
		if object.Len() != 3 || object.Get("kind") == nil || object.Get("color") == nil || object.Get("points") == nil {
			return nil, errors.New("legacy canvas geometry object must contain only kind, color, and points")
		}
		kindBytes, kindErr := object.Get("kind").StringBytes()
		if kindErr != nil {
			return nil, errors.Wrap(kindErr, "read legacy canvas geometry kind")
		}
		colorBytes, colorErr := object.Get("color").StringBytes()
		if colorErr != nil {
			return nil, errors.Wrap(colorErr, "read legacy canvas geometry color")
		}
		kind = string(kindBytes)
		color = string(colorBytes)
		pointValues, err = object.Get("points").Array()
	default:
		return nil, errors.New("legacy canvas geometry must be a point array or geometry object")
	}
	if err != nil {
		return nil, errors.Wrap(err, "read legacy canvas geometry points")
	}

	geometryKind, ok := map[string]CanvasGeometryKind{
		"pen":       CanvasGeometryKind_CANVAS_GEOMETRY_KIND_PEN,
		"line":      CanvasGeometryKind_CANVAS_GEOMETRY_KIND_LINE,
		"arrow":     CanvasGeometryKind_CANVAS_GEOMETRY_KIND_ARROW,
		"rectangle": CanvasGeometryKind_CANVAS_GEOMETRY_KIND_RECTANGLE,
		"ellipse":   CanvasGeometryKind_CANVAS_GEOMETRY_KIND_ELLIPSE,
	}[kind]
	if !ok {
		return nil, errors.New("legacy canvas geometry kind is unknown or unsupported")
	}
	points := make([]*CanvasPoint, len(pointValues))
	for idx, pointValue := range pointValues {
		point, pointErr := pointValue.Object()
		if pointErr != nil || point.Len() != 2 || point.Get("x") == nil || point.Get("y") == nil {
			return nil, errors.Errorf("legacy canvas geometry point %d must contain only numeric x and y", idx)
		}
		x, xErr := point.Get("x").Float64()
		y, yErr := point.Get("y").Float64()
		if xErr != nil || yErr != nil {
			return nil, errors.Errorf("legacy canvas geometry point %d must contain only numeric x and y", idx)
		}
		points[idx] = &CanvasPoint{X: x, Y: y}
	}
	parsed := &CanvasGeometry{Kind: geometryKind, Color: color, Points: points}
	if err := parsed.Validate(); err != nil {
		return nil, err
	}
	return parsed, nil
}

func marshalLegacyCanvasGeometry(geometry *CanvasGeometry) []byte {
	kind := map[CanvasGeometryKind]string{
		CanvasGeometryKind_CANVAS_GEOMETRY_KIND_PEN:       "pen",
		CanvasGeometryKind_CANVAS_GEOMETRY_KIND_LINE:      "line",
		CanvasGeometryKind_CANVAS_GEOMETRY_KIND_ARROW:     "arrow",
		CanvasGeometryKind_CANVAS_GEOMETRY_KIND_RECTANGLE: "rectangle",
		CanvasGeometryKind_CANVAS_GEOMETRY_KIND_ELLIPSE:   "ellipse",
	}[geometry.GetKind()]
	data := make([]byte, 0, 64+len(geometry.GetPoints())*32)
	data = append(data, `{"kind":`...)
	data = strconv.AppendQuote(data, kind)
	data = append(data, `,"color":`...)
	data = strconv.AppendQuote(data, geometry.GetColor())
	data = append(data, `,"points":[`...)
	for idx, point := range geometry.GetPoints() {
		if idx != 0 {
			data = append(data, ',')
		}
		data = append(data, `{"x":`...)
		data = strconv.AppendFloat(data, point.GetX(), 'g', -1, 64)
		data = append(data, `,"y":`...)
		data = strconv.AppendFloat(data, point.GetY(), 'g', -1, 64)
		data = append(data, '}')
	}
	return append(data, ']', '}')
}

func finiteCanvasNumber(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func validCanvasHexColor(color string) bool {
	if len(color) != 7 || color[0] != '#' {
		return false
	}
	for _, char := range color[1:] {
		switch {
		case char >= '0' && char <= '9':
		case char >= 'a' && char <= 'f':
		case char >= 'A' && char <= 'F':
		default:
			return false
		}
	}
	return true
}

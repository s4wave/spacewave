//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/aperturerobotics/cli"
	protojson "github.com/aperturerobotics/protobuf-go-lite/json"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	space_world_ops "github.com/s4wave/spacewave/core/space/world/ops"
	s4wave_canvas "github.com/s4wave/spacewave/sdk/canvas"
	s4wave_space "github.com/s4wave/spacewave/sdk/space"
	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// canvasContext holds the mounted resources for canvas operations.
type canvasContext struct {
	canvasSvc s4wave_canvas.SRPCCanvasResourceServiceClient
	engine    *sdk_engine.SDKEngine
	objectKey string
}

// parseCanvasURI parses the canvas URI, allowing it to be empty for auto-discovery.
// When arg is empty, returns an fsURI with empty objectKey (mountCanvasContext
// will auto-discover the canvas object in the space).
func parseCanvasURI(arg, spaceFlag string, sessFlag int) (fsURI, error) {
	if arg == "" {
		result := fsURI{sessionIdx: 1, spaceID: spaceFlag}
		if sessFlag > 0 {
			result.sessionIdx = uint32(sessFlag)
		}
		return result, nil
	}
	return parseFsURI(arg, spaceFlag, sessFlag)
}

// mountCanvasContext connects to the daemon and mounts the full chain to get
// a CanvasResourceService client for the given URI.
// If uri.objectKey is empty, auto-discovers the canvas by finding exactly one
// canvas-type object in the space.
func mountCanvasContext(c *cli.Context, statePath string, uri fsURI) (*canvasContext, func(), error) {
	ctx := c.Context

	client, err := connectDaemonFromContext(ctx, c, statePath)
	if err != nil {
		return nil, nil, err
	}

	sess, err := client.mountSession(ctx, uri.sessionIdx)
	if err != nil {
		client.close()
		return nil, nil, err
	}

	spaceID := uri.spaceID
	if spaceID == "" {
		spaceID, err = client.getSpaceByName(ctx, sess, "")
		if err != nil {
			sess.Release()
			client.close()
			return nil, nil, errors.Wrap(err, "resolve default space")
		}
	}

	spaceSvc, spaceCleanup, err := client.mountSpace(ctx, sess, spaceID)
	if err != nil {
		sess.Release()
		client.close()
		return nil, nil, err
	}

	objectKey := uri.objectKey
	if objectKey == "" {
		objectKey, err = discoverCanvasObject(ctx, spaceSvc)
		if err != nil {
			spaceCleanup()
			sess.Release()
			client.close()
			return nil, nil, err
		}
	}

	engine, engineRef, engineCleanup, err := client.accessWorldEngineWithRef(ctx, spaceSvc)
	if err != nil {
		spaceCleanup()
		sess.Release()
		client.close()
		return nil, nil, err
	}

	typedClient, _, _, typedCleanup, err := client.accessTypedObject(ctx, engineRef, objectKey)
	if err != nil {
		engineCleanup()
		spaceCleanup()
		sess.Release()
		client.close()
		return nil, nil, errors.Wrap(err, "access typed object for "+objectKey)
	}

	canvasSvc := s4wave_canvas.NewSRPCCanvasResourceServiceClient(typedClient)

	cleanup := func() {
		typedCleanup()
		engineCleanup()
		spaceCleanup()
		sess.Release()
		client.close()
	}

	return &canvasContext{
		canvasSvc: canvasSvc,
		engine:    engine,
		objectKey: objectKey,
	}, cleanup, nil
}

// discoverCanvasObject finds exactly one canvas-type object in the space.
// Returns the object key or an error if zero or multiple canvas objects exist.
func discoverCanvasObject(ctx context.Context, spaceSvc s4wave_space.SRPCSpaceResourceServiceClient) (string, error) {
	strm, err := spaceSvc.WatchSpaceState(ctx, &s4wave_space.WatchSpaceStateRequest{})
	if err != nil {
		return "", errors.Wrap(err, "watch space state")
	}
	defer strm.Close()

	resp, err := strm.Recv()
	if err != nil {
		return "", errors.Wrap(err, "recv space state")
	}

	wc := resp.GetWorldContents()
	if wc == nil {
		return "", errors.New("no canvas objects found; specify --canvas")
	}

	var canvasKeys []string
	for _, obj := range wc.GetObjects() {
		if obj.GetObjectType() == "canvas" {
			canvasKeys = append(canvasKeys, obj.GetObjectKey())
		}
	}

	if len(canvasKeys) == 0 {
		return "", errors.New("no canvas objects found; specify --canvas")
	}
	if len(canvasKeys) > 1 {
		return "", errors.New("multiple canvas objects found; specify --canvas")
	}
	return canvasKeys[0], nil
}

// commonCanvasFlags returns the flags shared by all canvas subcommands.
func commonCanvasFlags(canvasURI *string, statePath *string, spaceID *string, sessIdx *int, outputFormat *string) []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "uri",
			Aliases:     []string{"canvas", "canvas-id"},
			Usage:       "canvas URI",
			EnvVars:     []string{"SPACEWAVE_URI", "SPACEWAVE_CANVAS"},
			Destination: canvasURI,
		},
		statePathFlag(statePath),
		&cli.StringFlag{
			Name:        "space",
			Usage:       "space ID (overrides URI)",
			EnvVars:     []string{"SPACEWAVE_SPACE"},
			Destination: spaceID,
		},
		&cli.IntFlag{
			Name:        "session-index",
			Usage:       "session index (overrides URI)",
			EnvVars:     []string{"SPACEWAVE_SESSION_INDEX"},
			Destination: sessIdx,
		},
		&cli.StringFlag{
			Name:        "output",
			Aliases:     []string{"o"},
			Usage:       "output format (text/json/yaml)",
			EnvVars:     []string{"SPACEWAVE_OUTPUT"},
			Value:       "text",
			Destination: outputFormat,
		},
	}
}

// nodeTypeDisplay returns a short display name for a NodeType.
func nodeTypeDisplay(t s4wave_canvas.NodeType) string {
	switch t {
	case s4wave_canvas.NodeType_NODE_TYPE_TEXT:
		return "text"
	case s4wave_canvas.NodeType_NODE_TYPE_SHAPE:
		return "shape"
	case s4wave_canvas.NodeType_NODE_TYPE_WORLD_OBJECT:
		return "world_object"
	case s4wave_canvas.NodeType_NODE_TYPE_DRAWING:
		return "drawing"
	default:
		return "unknown"
	}
}

// edgeStyleDisplay returns a short display name for an EdgeStyle.
func edgeStyleDisplay(s s4wave_canvas.EdgeStyle) string {
	switch s {
	case s4wave_canvas.EdgeStyle_EDGE_STYLE_BEZIER:
		return "bezier"
	case s4wave_canvas.EdgeStyle_EDGE_STYLE_STRAIGHT:
		return "straight"
	default:
		return "unknown"
	}
}

// newCanvasCommand builds the top-level canvas command group.
func newCanvasCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:  "canvas",
		Usage: "canvas operations",
		Subcommands: []*cli.Command{
			buildCanvasShowCommand(),
			buildCanvasWatchCommand(),
			buildCanvasApplyCommand(),
			buildCanvasNodeCommand(),
			buildCanvasEdgeCommand(),
			buildCanvasExportCommand(),
		},
	}
}

// buildCanvasShowCommand builds the canvas show subcommand.
func buildCanvasShowCommand() *cli.Command {
	var canvasURI, statePath, spaceID, outputFormat string
	var sessIdx int
	return &cli.Command{
		Name:  "show",
		Usage: "show canvas state summary",
		Flags: commonCanvasFlags(&canvasURI, &statePath, &spaceID, &sessIdx, &outputFormat),
		Action: func(c *cli.Context) error {
			uri, err := parseCanvasURI(canvasURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			cc, cleanup, err := mountCanvasContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := c.Context
			resp, err := cc.canvasSvc.GetCanvasState(ctx, &s4wave_canvas.GetCanvasStateRequest{})
			if err != nil {
				return errors.Wrap(err, "get canvas state")
			}

			state := resp.GetState()
			if state == nil {
				return errors.New("no canvas state returned")
			}

			if outputFormat == "json" || outputFormat == "yaml" {
				data, err := state.MarshalJSON()
				if err != nil {
					return errors.Wrap(err, "marshal canvas state")
				}
				return formatOutput(data, outputFormat)
			}

			nodes := state.GetNodes()
			edges := state.GetEdges()
			hiddenGraphLinks := state.GetHiddenGraphLinks()

			w := os.Stdout
			w.WriteString("Canvas: " + cc.objectKey + "\n")
			w.WriteString("Nodes: " + strconv.Itoa(len(nodes)) + "\n")
			w.WriteString("Edges: " + strconv.Itoa(len(edges)) + "\n")
			w.WriteString("Hidden graph links: " + strconv.Itoa(len(hiddenGraphLinks)) + "\n")

			if len(nodes) > 0 {
				w.WriteString("\nNODES:\n")
				writeNodesTable(w, nodes)
			}

			if len(edges) > 0 {
				w.WriteString("\nEDGES:\n")
				writeEdgesTable(w, edges)
			}

			if len(hiddenGraphLinks) > 0 {
				w.WriteString("\nHIDDEN GRAPH LINKS:\n")
				writeHiddenGraphLinksTable(w, hiddenGraphLinks)
			}

			return nil
		},
	}
}

// buildCanvasNodeCommand builds the canvas node command group.
func buildCanvasNodeCommand() *cli.Command {
	return &cli.Command{
		Name:  "node",
		Usage: "manage canvas nodes",
		Subcommands: []*cli.Command{
			buildCanvasNodeListCommand(),
			buildCanvasNodeAddCommand(),
			buildCanvasNodeRmCommand(),
			buildCanvasNodeSetCommand(),
			buildCanvasNodeNavigateCommand(),
		},
	}
}

// buildCanvasNodeListCommand builds the canvas node list subcommand.
func buildCanvasNodeListCommand() *cli.Command {
	var canvasURI, statePath, spaceID, outputFormat, typeFilter string
	var sessIdx int
	return &cli.Command{
		Name:  "list",
		Usage: "list canvas nodes",
		Flags: append(commonCanvasFlags(&canvasURI, &statePath, &spaceID, &sessIdx, &outputFormat),
			&cli.StringFlag{
				Name:        "type",
				Usage:       "filter by node type (text, shape, world-object, drawing)",
				Destination: &typeFilter,
			},
		),
		Action: func(c *cli.Context) error {
			uri, err := parseCanvasURI(canvasURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			cc, cleanup, err := mountCanvasContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := c.Context
			resp, err := cc.canvasSvc.GetCanvasState(ctx, &s4wave_canvas.GetCanvasStateRequest{})
			if err != nil {
				return errors.Wrap(err, "get canvas state")
			}

			state := resp.GetState()
			if state == nil {
				return errors.New("no canvas state returned")
			}

			nodes := state.GetNodes()

			// apply type filter
			if typeFilter != "" {
				filterType := parseNodeTypeFilter(typeFilter)
				filtered := make(map[string]*s4wave_canvas.CanvasNode, len(nodes))
				for id, node := range nodes {
					if node.GetType() == filterType {
						filtered[id] = node
					}
				}
				nodes = filtered
			}

			if outputFormat == "json" || outputFormat == "yaml" {
				buf, ms := newMarshalBuf()
				ms.WriteArrayStart()
				var af bool
				ids := sortedNodeIDs(nodes)
				for _, id := range ids {
					node := nodes[id]
					ms.WriteMoreIf(&af)
					marshalNode(ms, node)
				}
				ms.WriteArrayEnd()
				return formatOutput(buf.Bytes(), outputFormat)
			}

			w := os.Stdout
			writeNodesTable(w, nodes)
			return nil
		},
	}
}

// buildCanvasEdgeCommand builds the canvas edge command group.
func buildCanvasEdgeCommand() *cli.Command {
	return &cli.Command{
		Name:  "edge",
		Usage: "manage canvas edges",
		Subcommands: []*cli.Command{
			buildCanvasEdgeListCommand(),
			buildCanvasEdgeAddCommand(),
			buildCanvasEdgeRmCommand(),
		},
	}
}

// buildCanvasEdgeListCommand builds the canvas edge list subcommand.
func buildCanvasEdgeListCommand() *cli.Command {
	var canvasURI, statePath, spaceID, outputFormat string
	var sessIdx int
	return &cli.Command{
		Name:  "list",
		Usage: "list canvas edges",
		Flags: commonCanvasFlags(&canvasURI, &statePath, &spaceID, &sessIdx, &outputFormat),
		Action: func(c *cli.Context) error {
			uri, err := parseCanvasURI(canvasURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			cc, cleanup, err := mountCanvasContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := c.Context
			resp, err := cc.canvasSvc.GetCanvasState(ctx, &s4wave_canvas.GetCanvasStateRequest{})
			if err != nil {
				return errors.Wrap(err, "get canvas state")
			}

			state := resp.GetState()
			if state == nil {
				return errors.New("no canvas state returned")
			}

			edges := state.GetEdges()

			if outputFormat == "json" || outputFormat == "yaml" {
				buf, ms := newMarshalBuf()
				ms.WriteArrayStart()
				var af bool
				for _, edge := range edges {
					ms.WriteMoreIf(&af)
					marshalEdge(ms, edge)
				}
				ms.WriteArrayEnd()
				return formatOutput(buf.Bytes(), outputFormat)
			}

			w := os.Stdout
			writeEdgesTable(w, edges)
			return nil
		},
	}
}

// buildCanvasExportCommand builds the canvas export subcommand.
func buildCanvasExportCommand() *cli.Command {
	var canvasURI, statePath, spaceID, outputFormat string
	var sessIdx int
	return &cli.Command{
		Name:  "export",
		Usage: "export full canvas state as JSON or YAML",
		Flags: commonCanvasFlags(&canvasURI, &statePath, &spaceID, &sessIdx, &outputFormat),
		Action: func(c *cli.Context) error {
			// default to json for export
			if outputFormat == "text" {
				outputFormat = "json"
			}

			uri, err := parseCanvasURI(canvasURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			cc, cleanup, err := mountCanvasContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := c.Context
			resp, err := cc.canvasSvc.GetCanvasState(ctx, &s4wave_canvas.GetCanvasStateRequest{})
			if err != nil {
				return errors.Wrap(err, "get canvas state")
			}

			state := resp.GetState()
			if state == nil {
				return errors.New("no canvas state returned")
			}

			data, err := state.MarshalJSON()
			if err != nil {
				return errors.Wrap(err, "marshal canvas state")
			}
			return formatOutput(data, outputFormat)
		},
	}
}

// sortedNodeIDs returns the node IDs sorted alphabetically.
func sortedNodeIDs(nodes map[string]*s4wave_canvas.CanvasNode) []string {
	ids := make([]string, 0, len(nodes))
	for id := range nodes {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

// writeNodesTable writes the nodes table to the writer.
func writeNodesTable(w *os.File, nodes map[string]*s4wave_canvas.CanvasNode) {
	rows := [][]string{{"ID", "TYPE", "X", "Y", "W", "H", "Z", "PINNED"}}
	ids := sortedNodeIDs(nodes)
	for _, id := range ids {
		node := nodes[id]
		rows = append(rows, []string{
			id,
			nodeTypeDisplay(node.GetType()),
			strconv.FormatFloat(node.GetX(), 'f', 1, 64),
			strconv.FormatFloat(node.GetY(), 'f', 1, 64),
			strconv.FormatFloat(node.GetWidth(), 'f', 0, 64),
			strconv.FormatFloat(node.GetHeight(), 'f', 0, 64),
			strconv.FormatInt(int64(node.GetZIndex()), 10),
			strconv.FormatBool(node.GetPinned()),
		})
	}
	writeTable(w, "", rows)
}

// writeEdgesTable writes the edges table to the writer.
func writeEdgesTable(w *os.File, edges []*s4wave_canvas.CanvasEdge) {
	rows := [][]string{{"ID", "SOURCE", "TARGET", "LABEL", "STYLE"}}
	for _, edge := range edges {
		rows = append(rows, []string{
			edge.GetId(),
			edge.GetSourceNodeId(),
			edge.GetTargetNodeId(),
			edge.GetLabel(),
			edgeStyleDisplay(edge.GetStyle()),
		})
	}
	writeTable(w, "", rows)
}

// writeHiddenGraphLinksTable writes the hidden graph links table to the writer.
func writeHiddenGraphLinksTable(w *os.File, links []*s4wave_canvas.HiddenGraphLink) {
	rows := [][]string{{"SUBJECT", "PREDICATE", "OBJECT", "LABEL"}}
	for _, link := range links {
		rows = append(rows, []string{
			link.GetSubject(),
			link.GetPredicate(),
			link.GetObject(),
			link.GetLabel(),
		})
	}
	writeTable(w, "", rows)
}

// marshalNode writes a CanvasNode as JSON to the MarshalState.
func marshalNode(ms *protojson.MarshalState, node *s4wave_canvas.CanvasNode) {
	ms.WriteObjectStart()
	var f bool
	writeJSONStringField(ms, &f, "id", node.GetId())
	writeJSONStringField(ms, &f, "type", nodeTypeDisplay(node.GetType()))
	writeJSONFloat64Field(ms, &f, "x", node.GetX())
	writeJSONFloat64Field(ms, &f, "y", node.GetY())
	writeJSONFloat64Field(ms, &f, "width", node.GetWidth())
	writeJSONFloat64Field(ms, &f, "height", node.GetHeight())
	writeJSONInt32Field(ms, &f, "zIndex", node.GetZIndex())
	writeJSONBoolField(ms, &f, "pinned", node.GetPinned())
	writeJSONStringFieldIf(ms, &f, "objectKey", node.GetObjectKey())
	writeJSONStringFieldIf(ms, &f, "textContent", node.GetTextContent())
	ms.WriteObjectEnd()
}

// marshalEdge writes a CanvasEdge as JSON to the MarshalState.
func marshalEdge(ms *protojson.MarshalState, edge *s4wave_canvas.CanvasEdge) {
	ms.WriteObjectStart()
	var f bool
	writeJSONStringField(ms, &f, "id", edge.GetId())
	writeJSONStringField(ms, &f, "sourceNodeId", edge.GetSourceNodeId())
	writeJSONStringField(ms, &f, "targetNodeId", edge.GetTargetNodeId())
	writeJSONStringFieldIf(ms, &f, "label", edge.GetLabel())
	writeJSONStringField(ms, &f, "style", edgeStyleDisplay(edge.GetStyle()))
	ms.WriteObjectEnd()
}

// parseNodeTypeFilter parses a user-provided type filter string into a NodeType.
func parseNodeTypeFilter(filter string) s4wave_canvas.NodeType {
	switch strings.ToLower(filter) {
	case "text":
		return s4wave_canvas.NodeType_NODE_TYPE_TEXT
	case "shape":
		return s4wave_canvas.NodeType_NODE_TYPE_SHAPE
	case "world-object", "world_object":
		return s4wave_canvas.NodeType_NODE_TYPE_WORLD_OBJECT
	case "drawing":
		return s4wave_canvas.NodeType_NODE_TYPE_DRAWING
	default:
		return s4wave_canvas.NodeType_NODE_TYPE_UNKNOWN
	}
}

// parseEdgeStyle parses a user-provided style string into an EdgeStyle.
func parseEdgeStyle(s string) s4wave_canvas.EdgeStyle {
	switch strings.ToLower(s) {
	case "straight":
		return s4wave_canvas.EdgeStyle_EDGE_STYLE_STRAIGHT
	default:
		return s4wave_canvas.EdgeStyle_EDGE_STYLE_BEZIER
	}
}

// nextNodeID generates the next incremental node ID (n1, n2, ...).
func nextNodeID(nodes map[string]*s4wave_canvas.CanvasNode) string {
	max := 0
	for id := range nodes {
		if len(id) > 1 && id[0] == 'n' {
			if n, err := strconv.Atoi(id[1:]); err == nil && n > max {
				max = n
			}
		}
	}
	return "n" + strconv.Itoa(max+1)
}

// nextEdgeID generates the next incremental edge ID (e1, e2, ...).
func nextEdgeID(edges []*s4wave_canvas.CanvasEdge) string {
	max := 0
	for _, e := range edges {
		id := e.GetId()
		if len(id) > 1 && id[0] == 'e' {
			if n, err := strconv.Atoi(id[1:]); err == nil && n > max {
				max = n
			}
		}
	}
	return "e" + strconv.Itoa(max+1)
}

// autoPlaceNode computes position for a new node based on existing nodes of the same type.
// Uses centroid-based placement, expanding down-right from the center of mass.
func autoPlaceNode(nodes map[string]*s4wave_canvas.CanvasNode, nodeType s4wave_canvas.NodeType, width, height float64) (x, y float64) {
	var cx, cy float64
	var count int
	for _, n := range nodes {
		if n.GetType() == nodeType {
			cx += n.GetX() + n.GetWidth()/2
			cy += n.GetY() + n.GetHeight()/2
			count++
		}
	}
	if count == 0 {
		return 100, 100
	}
	cx /= float64(count)
	cy /= float64(count)

	// spiral outward from centroid, preferring down-right
	step := math.Max(width, height) + 20
	for r := step; r < step*20; r += step {
		for _, angle := range []float64{0.25, 0.5, 0.75, 0, 1.0, 1.25, 1.5, 1.75} {
			tx := cx + r*math.Cos(angle*math.Pi) - width/2
			ty := cy + r*math.Sin(angle*math.Pi) - height/2
			if !overlapsAny(nodes, tx, ty, width, height) {
				return tx, ty
			}
		}
	}
	return cx + step, cy + step
}

// overlapsAny checks if a rectangle at (x,y,w,h) overlaps any existing node.
func overlapsAny(nodes map[string]*s4wave_canvas.CanvasNode, x, y, w, h float64) bool {
	for _, n := range nodes {
		nx, ny, nw, nh := n.GetX(), n.GetY(), n.GetWidth(), n.GetHeight()
		if x < nx+nw && x+w > nx && y < ny+nh && y+h > ny {
			return true
		}
	}
	return false
}

// canvasNodeAddSpec parameterizes one canvas-node add subcommand.
type canvasNodeAddSpec struct {
	name          string
	usage         string
	argsUsage     string
	defaultWidth  float64
	defaultHeight float64
	nodeType      s4wave_canvas.NodeType
	// preMountValidate runs before mountCanvasContext to short-circuit on
	// missing args. nil if the subcommand has no positional arg.
	preMountValidate func(c *cli.Context) error
	// applyArg fills type-specific fields after mount, with access to cc for
	// validation that requires the engine. nil if not needed.
	applyArg func(c *cli.Context, cc *canvasContext, node *s4wave_canvas.CanvasNode) error
}

// canvasNodeAddSpecs lists the four canvas-node add subcommand specs in display order.
var canvasNodeAddSpecs = []canvasNodeAddSpec{
	{
		name:          "text",
		usage:         "add a text node",
		argsUsage:     "<content>",
		defaultWidth:  200,
		defaultHeight: 100,
		nodeType:      s4wave_canvas.NodeType_NODE_TYPE_TEXT,
		preMountValidate: func(c *cli.Context) error {
			if c.Args().First() == "" {
				return errors.New("text content required")
			}
			return nil
		},
		applyArg: func(c *cli.Context, _ *canvasContext, node *s4wave_canvas.CanvasNode) error {
			node.TextContent = c.Args().First()
			return nil
		},
	},
	{
		name:          "object",
		usage:         "add a world-object node",
		argsUsage:     "<object-key>",
		defaultWidth:  400,
		defaultHeight: 300,
		nodeType:      s4wave_canvas.NodeType_NODE_TYPE_WORLD_OBJECT,
		applyArg: func(c *cli.Context, cc *canvasContext, node *s4wave_canvas.CanvasNode) error {
			objKey := c.Args().First()
			if objKey == "" {
				return errors.New("object key required")
			}
			ctx := c.Context
			tx, err := cc.engine.NewTransaction(ctx, false)
			if err != nil {
				return errors.Wrap(err, "new transaction")
			}
			_, found, err := tx.GetObject(ctx, objKey)
			tx.Discard()
			if err != nil {
				return errors.Wrap(err, "check object")
			}
			if !found {
				return errors.Errorf("object %q not found", objKey)
			}
			node.ObjectKey = objKey
			return nil
		},
	},
	{
		name:          "shape",
		usage:         "add a shape node",
		defaultWidth:  150,
		defaultHeight: 150,
		nodeType:      s4wave_canvas.NodeType_NODE_TYPE_SHAPE,
	},
	{
		name:          "drawing",
		usage:         "add a drawing node",
		defaultWidth:  300,
		defaultHeight: 300,
		nodeType:      s4wave_canvas.NodeType_NODE_TYPE_DRAWING,
	},
}

// buildCanvasNodeAddCommand builds the canvas node add command group.
func buildCanvasNodeAddCommand() *cli.Command {
	subs := make([]*cli.Command, len(canvasNodeAddSpecs))
	for i := range canvasNodeAddSpecs {
		subs[i] = buildCanvasNodeAddSubcommand(canvasNodeAddSpecs[i])
	}
	return &cli.Command{
		Name:        "add",
		Usage:       "add a canvas node",
		Subcommands: subs,
	}
}

// buildCanvasNodeAddSubcommand builds one canvas node add subcommand from a spec.
func buildCanvasNodeAddSubcommand(spec canvasNodeAddSpec) *cli.Command {
	var canvasURI, statePath, spaceID, outputFormat string
	var sessIdx int
	var nx, ny, nw, nh float64
	var nz int
	return &cli.Command{
		Name:      spec.name,
		Usage:     spec.usage,
		ArgsUsage: spec.argsUsage,
		Flags: append(commonCanvasFlags(&canvasURI, &statePath, &spaceID, &sessIdx, &outputFormat),
			&cli.Float64Flag{Name: "x", Destination: &nx},
			&cli.Float64Flag{Name: "y", Destination: &ny},
			&cli.Float64Flag{Name: "width", Value: spec.defaultWidth, Destination: &nw},
			&cli.Float64Flag{Name: "height", Value: spec.defaultHeight, Destination: &nh},
			&cli.IntFlag{Name: "z", Destination: &nz},
		),
		Action: func(c *cli.Context) error {
			if spec.preMountValidate != nil {
				if err := spec.preMountValidate(c); err != nil {
					return err
				}
			}

			uri, err := parseCanvasURI(canvasURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			cc, cleanup, err := mountCanvasContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := c.Context
			node := &s4wave_canvas.CanvasNode{
				Width:  nw,
				Height: nh,
				ZIndex: int32(nz),
				Type:   spec.nodeType,
				Pinned: true,
			}
			if spec.applyArg != nil {
				if err := spec.applyArg(c, cc, node); err != nil {
					return err
				}
			}

			resp, err := cc.canvasSvc.GetCanvasState(ctx, &s4wave_canvas.GetCanvasStateRequest{})
			if err != nil {
				return errors.Wrap(err, "get canvas state")
			}
			state := resp.GetState()
			if state == nil {
				state = &s4wave_canvas.CanvasState{}
			}

			nodes := state.GetNodes()
			node.Id = nextNodeID(nodes)

			if !c.IsSet("x") && !c.IsSet("y") {
				nx, ny = autoPlaceNode(nodes, spec.nodeType, nw, nh)
			}
			node.X = nx
			node.Y = ny

			op := space_world_ops.NewCanvasAddNodeOp(cc.objectKey, node)
			if err := applyWorldOp(c, cc.engine, op); err != nil {
				return err
			}

			os.Stdout.WriteString(node.Id + "\n")
			return nil
		},
	}
}

// buildCanvasNodeRmCommand builds the canvas node rm subcommand.
func buildCanvasNodeRmCommand() *cli.Command {
	var canvasURI, statePath, spaceID, outputFormat string
	var sessIdx int
	return &cli.Command{
		Name:      "rm",
		Usage:     "remove canvas nodes",
		ArgsUsage: "<node-id>...",
		Flags:     commonCanvasFlags(&canvasURI, &statePath, &spaceID, &sessIdx, &outputFormat),
		Action: func(c *cli.Context) error {
			ids := c.Args().Slice()
			if len(ids) == 0 {
				return errors.New("at least one node ID required")
			}

			uri, err := parseCanvasURI(canvasURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			cc, cleanup, err := mountCanvasContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			op := space_world_ops.NewCanvasRemoveNodeOp(cc.objectKey, ids)
			return applyWorldOp(c, cc.engine, op)
		},
	}
}

// buildCanvasNodeSetCommand builds the canvas node set subcommand.
func buildCanvasNodeSetCommand() *cli.Command {
	var canvasURI, statePath, spaceID, outputFormat, nodeID, text string
	var sessIdx int
	var nx, ny, nw, nh float64
	var nz int
	var pinned bool
	return &cli.Command{
		Name:  "set",
		Usage: "update node properties",
		Flags: append(commonCanvasFlags(&canvasURI, &statePath, &spaceID, &sessIdx, &outputFormat),
			&cli.StringFlag{Name: "node", Usage: "node ID (required)", Destination: &nodeID, Required: true},
			&cli.Float64Flag{Name: "x", Destination: &nx},
			&cli.Float64Flag{Name: "y", Destination: &ny},
			&cli.Float64Flag{Name: "width", Destination: &nw},
			&cli.Float64Flag{Name: "height", Destination: &nh},
			&cli.IntFlag{Name: "z", Destination: &nz},
			&cli.StringFlag{Name: "text", Usage: "text content (text nodes)", Destination: &text},
			&cli.BoolFlag{Name: "pinned", Destination: &pinned},
		),
		Action: func(c *cli.Context) error {
			uri, err := parseCanvasURI(canvasURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			cc, cleanup, err := mountCanvasContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := c.Context
			resp, err := cc.canvasSvc.GetCanvasState(ctx, &s4wave_canvas.GetCanvasStateRequest{})
			if err != nil {
				return errors.Wrap(err, "get canvas state")
			}
			state := resp.GetState()
			if state == nil {
				return errors.Errorf("node %q not found", nodeID)
			}

			existing, ok := state.GetNodes()[nodeID]
			if !ok {
				return errors.Errorf("node %q not found", nodeID)
			}

			// Clone and merge provided flags.
			node := existing.CloneVT()
			if c.IsSet("x") {
				node.X = nx
			}
			if c.IsSet("y") {
				node.Y = ny
			}
			if c.IsSet("width") {
				node.Width = nw
			}
			if c.IsSet("height") {
				node.Height = nh
			}
			if c.IsSet("z") {
				node.ZIndex = int32(nz)
			}
			if c.IsSet("text") {
				node.TextContent = text
			}
			if c.IsSet("pinned") {
				node.Pinned = pinned
			}

			op := space_world_ops.NewCanvasSetNodeOp(cc.objectKey, node)
			return applyWorldOp(c, cc.engine, op)
		},
	}
}

// buildCanvasEdgeAddCommand builds the canvas edge add subcommand.
func buildCanvasEdgeAddCommand() *cli.Command {
	var canvasURI, statePath, spaceID, outputFormat string
	var sessIdx int
	var source, target, edgeID, label, style string
	return &cli.Command{
		Name:  "add",
		Usage: "add an edge between nodes",
		Flags: append(commonCanvasFlags(&canvasURI, &statePath, &spaceID, &sessIdx, &outputFormat),
			&cli.StringFlag{Name: "source", Usage: "source node ID (required)", Destination: &source, Required: true},
			&cli.StringFlag{Name: "target", Usage: "target node ID (required)", Destination: &target, Required: true},
			&cli.StringFlag{Name: "id", Usage: "edge ID (auto-generated if empty)", Destination: &edgeID},
			&cli.StringFlag{Name: "label", Destination: &label},
			&cli.StringFlag{Name: "style", Usage: "bezier or straight", Value: "bezier", Destination: &style},
		),
		Action: func(c *cli.Context) error {
			uri, err := parseCanvasURI(canvasURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			cc, cleanup, err := mountCanvasContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			if edgeID == "" {
				ctx := c.Context
				resp, err := cc.canvasSvc.GetCanvasState(ctx, &s4wave_canvas.GetCanvasStateRequest{})
				if err != nil {
					return errors.Wrap(err, "get canvas state")
				}
				state := resp.GetState()
				if state == nil {
					state = &s4wave_canvas.CanvasState{}
				}
				edgeID = nextEdgeID(state.GetEdges())
			}

			edge := &s4wave_canvas.CanvasEdge{
				Id:           edgeID,
				SourceNodeId: source,
				TargetNodeId: target,
				Label:        label,
				Style:        parseEdgeStyle(style),
			}

			op := space_world_ops.NewCanvasAddEdgeOp(cc.objectKey, edge)
			if err := applyWorldOp(c, cc.engine, op); err != nil {
				return err
			}

			os.Stdout.WriteString(edgeID + "\n")
			return nil
		},
	}
}

// buildCanvasEdgeRmCommand builds the canvas edge rm subcommand.
func buildCanvasEdgeRmCommand() *cli.Command {
	var canvasURI, statePath, spaceID, outputFormat string
	var sessIdx int
	return &cli.Command{
		Name:      "rm",
		Usage:     "remove canvas edges",
		ArgsUsage: "<edge-id>...",
		Flags:     commonCanvasFlags(&canvasURI, &statePath, &spaceID, &sessIdx, &outputFormat),
		Action: func(c *cli.Context) error {
			ids := c.Args().Slice()
			if len(ids) == 0 {
				return errors.New("at least one edge ID required")
			}

			uri, err := parseCanvasURI(canvasURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			cc, cleanup, err := mountCanvasContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			op := space_world_ops.NewCanvasRemoveEdgeOp(cc.objectKey, ids)
			return applyWorldOp(c, cc.engine, op)
		},
	}
}

// buildCanvasWatchCommand builds the canvas watch subcommand.
func buildCanvasWatchCommand() *cli.Command {
	var canvasURI, statePath, spaceID, outputFormat string
	var sessIdx int
	return &cli.Command{
		Name:  "watch",
		Usage: "stream canvas state changes",
		Flags: commonCanvasFlags(&canvasURI, &statePath, &spaceID, &sessIdx, &outputFormat),
		Action: func(c *cli.Context) error {
			uri, err := parseCanvasURI(canvasURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			cc, cleanup, err := mountCanvasContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := c.Context
			strm, err := cc.canvasSvc.WatchCanvasState(ctx, &s4wave_canvas.WatchCanvasStateRequest{})
			if err != nil {
				return errors.Wrap(err, "watch canvas state")
			}
			defer strm.Close()

			w := os.Stdout
			var prev *s4wave_canvas.CanvasState
			for {
				resp, err := strm.Recv()
				if err != nil {
					return errors.Wrap(err, "recv canvas state")
				}

				state := resp.GetState()
				if state == nil {
					continue
				}

				if outputFormat == "json" || outputFormat == "yaml" {
					data, err := state.MarshalJSON()
					if err != nil {
						return errors.Wrap(err, "marshal state")
					}
					if err := formatOutput(data, outputFormat); err != nil {
						return err
					}
					w.WriteString("\n")
				} else {
					ts := time.Now().Format(time.RFC3339)
					writeCanvasDiff(w, ts, prev, state)
				}
				prev = state
			}
		},
	}
}

// logCanvasEvent writes one timestamped diff line: "[ts] body\n".
func logCanvasEvent(w *os.File, ts, body string) {
	w.WriteString("[" + ts + "] " + body + "\n")
}

// formatNodeAddBody formats the body of an "added node" diff line.
func formatNodeAddBody(id string, node *s4wave_canvas.CanvasNode) string {
	body := "+" + id + " " + nodeTypeDisplay(node.GetType())
	if node.GetTextContent() != "" {
		body += " \"" + truncate(node.GetTextContent(), 40) + "\""
	}
	if node.GetObjectKey() != "" {
		body += " obj=" + node.GetObjectKey()
	}
	return body
}

// formatNodeChangeBody formats the body of a "changed node" diff line.
func formatNodeChangeBody(id string, old, node *s4wave_canvas.CanvasNode) string {
	body := "~" + id
	if old.GetX() != node.GetX() || old.GetY() != node.GetY() {
		body += " moved (" +
			strconv.FormatFloat(old.GetX(), 'f', 0, 64) + "," +
			strconv.FormatFloat(old.GetY(), 'f', 0, 64) + ")->(" +
			strconv.FormatFloat(node.GetX(), 'f', 0, 64) + "," +
			strconv.FormatFloat(node.GetY(), 'f', 0, 64) + ")"
	}
	if old.GetWidth() != node.GetWidth() || old.GetHeight() != node.GetHeight() {
		body += " resized"
	}
	if old.GetTextContent() != node.GetTextContent() {
		body += " text=\"" + truncate(node.GetTextContent(), 40) + "\""
	}
	if old.GetPinned() != node.GetPinned() {
		if node.GetPinned() {
			body += " pinned"
		} else {
			body += " unpinned"
		}
	}
	return body
}

// formatEdgeAddBody formats the body of an "added edge" diff line.
func formatEdgeAddBody(e *s4wave_canvas.CanvasEdge) string {
	body := "+" + e.GetId() + " " + e.GetSourceNodeId() + "->" + e.GetTargetNodeId()
	if e.GetLabel() != "" {
		body += " \"" + e.GetLabel() + "\""
	}
	return body
}

// writeCanvasDiff writes compact text diff between two canvas states.
func writeCanvasDiff(w *os.File, ts string, prev, curr *s4wave_canvas.CanvasState) {
	prevNodes := make(map[string]*s4wave_canvas.CanvasNode)
	currNodes := curr.GetNodes()
	if prev != nil {
		prevNodes = prev.GetNodes()
	}

	// added/changed nodes
	for id, node := range currNodes {
		old, existed := prevNodes[id]
		if !existed {
			logCanvasEvent(w, ts, formatNodeAddBody(id, node))
		} else if nodeChanged(old, node) {
			logCanvasEvent(w, ts, formatNodeChangeBody(id, old, node))
		}
	}

	// removed nodes
	for id := range prevNodes {
		if _, exists := currNodes[id]; !exists {
			logCanvasEvent(w, ts, "-"+id)
		}
	}

	// edges
	prevEdges := make(map[string]*s4wave_canvas.CanvasEdge)
	if prev != nil {
		for _, e := range prev.GetEdges() {
			prevEdges[e.GetId()] = e
		}
	}
	for _, e := range curr.GetEdges() {
		if _, existed := prevEdges[e.GetId()]; !existed {
			logCanvasEvent(w, ts, formatEdgeAddBody(e))
		}
	}
	currEdges := make(map[string]struct{})
	for _, e := range curr.GetEdges() {
		currEdges[e.GetId()] = struct{}{}
	}
	for id := range prevEdges {
		if _, exists := currEdges[id]; !exists {
			logCanvasEvent(w, ts, "-"+id)
		}
	}

	// hidden graph links
	prevHidden := make(map[canvasHiddenGraphLinkKey]*s4wave_canvas.HiddenGraphLink)
	if prev != nil {
		for _, link := range prev.GetHiddenGraphLinks() {
			prevHidden[newCanvasHiddenGraphLinkKey(link)] = link
		}
	}
	for _, link := range curr.GetHiddenGraphLinks() {
		if _, existed := prevHidden[newCanvasHiddenGraphLinkKey(link)]; !existed {
			logCanvasEvent(w, ts, "+hidden-graph-link "+hiddenGraphLinkDisplay(link))
		}
	}
	currHidden := make(map[canvasHiddenGraphLinkKey]struct{})
	for _, link := range curr.GetHiddenGraphLinks() {
		currHidden[newCanvasHiddenGraphLinkKey(link)] = struct{}{}
	}
	for key, link := range prevHidden {
		if _, exists := currHidden[key]; !exists {
			logCanvasEvent(w, ts, "-hidden-graph-link "+hiddenGraphLinkDisplay(link))
		}
	}
}

// nodeChanged returns true if two nodes differ in any visible property.
func nodeChanged(a, b *s4wave_canvas.CanvasNode) bool {
	return a.GetX() != b.GetX() || a.GetY() != b.GetY() ||
		a.GetWidth() != b.GetWidth() || a.GetHeight() != b.GetHeight() ||
		a.GetZIndex() != b.GetZIndex() || a.GetType() != b.GetType() ||
		a.GetTextContent() != b.GetTextContent() || a.GetPinned() != b.GetPinned() ||
		a.GetObjectKey() != b.GetObjectKey() || a.GetViewPath() != b.GetViewPath()
}

type canvasHiddenGraphLinkKey struct {
	subject   string
	predicate string
	object    string
	label     string
}

func newCanvasHiddenGraphLinkKey(link *s4wave_canvas.HiddenGraphLink) canvasHiddenGraphLinkKey {
	return canvasHiddenGraphLinkKey{
		subject:   link.GetSubject(),
		predicate: link.GetPredicate(),
		object:    link.GetObject(),
		label:     link.GetLabel(),
	}
}

func hiddenGraphLinkDisplay(link *s4wave_canvas.HiddenGraphLink) string {
	out := link.GetSubject() + " " + link.GetPredicate() + " " + link.GetObject()
	if link.GetLabel() != "" {
		out += " \"" + link.GetLabel() + "\""
	}
	return out
}

// truncate shortens a string to max length with ellipsis.
func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}

// buildCanvasNodeNavigateCommand builds the canvas node navigate subcommand.
func buildCanvasNodeNavigateCommand() *cli.Command {
	var canvasURI, statePath, spaceID, outputFormat, nodeID string
	var sessIdx int
	return &cli.Command{
		Name:      "navigate",
		Usage:     "set node viewer path",
		ArgsUsage: "<path>",
		Flags: append(commonCanvasFlags(&canvasURI, &statePath, &spaceID, &sessIdx, &outputFormat),
			&cli.StringFlag{Name: "node", Usage: "node ID (required)", Destination: &nodeID, Required: true},
		),
		Action: func(c *cli.Context) error {
			viewPath := c.Args().First()
			if viewPath == "" {
				return errors.New("path argument required")
			}

			uri, err := parseCanvasURI(canvasURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			cc, cleanup, err := mountCanvasContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			ctx := c.Context
			resp, err := cc.canvasSvc.GetCanvasState(ctx, &s4wave_canvas.GetCanvasStateRequest{})
			if err != nil {
				return errors.Wrap(err, "get canvas state")
			}
			state := resp.GetState()
			if state == nil {
				return errors.Errorf("node %q not found", nodeID)
			}

			existing, ok := state.GetNodes()[nodeID]
			if !ok {
				return errors.Errorf("node %q not found", nodeID)
			}

			node := existing.CloneVT()
			node.ViewPath = viewPath

			op := space_world_ops.NewCanvasSetNodeOp(cc.objectKey, node)
			return applyWorldOp(c, cc.engine, op)
		},
	}
}

// buildCanvasApplyCommand builds the canvas apply subcommand.
func buildCanvasApplyCommand() *cli.Command {
	var canvasURI, statePath, spaceID, outputFormat, fromFile string
	var sessIdx int
	return &cli.Command{
		Name:  "apply",
		Usage: "apply a world op from stdin or file",
		Flags: append(commonCanvasFlags(&canvasURI, &statePath, &spaceID, &sessIdx, &outputFormat),
			&cli.StringFlag{Name: "from", Usage: "read op from file instead of stdin", Destination: &fromFile},
		),
		Action: func(c *cli.Context) error {
			uri, err := parseCanvasURI(canvasURI, spaceID, sessIdx)
			if err != nil {
				return err
			}

			cc, cleanup, err := mountCanvasContext(c, statePath, uri)
			if err != nil {
				return err
			}
			defer cleanup()

			var data []byte
			if fromFile != "" {
				data, err = os.ReadFile(fromFile)
			} else {
				data, err = io.ReadAll(os.Stdin)
			}
			if err != nil {
				return errors.Wrap(err, "read input")
			}
			if len(data) == 0 {
				return errors.New("empty input")
			}

			// Apply as UpdateCanvas request (JSON).
			req := &s4wave_canvas.UpdateCanvasRequest{}
			if err := req.UnmarshalJSON(data); err != nil {
				return errors.Wrap(err, "parse input as UpdateCanvasRequest JSON")
			}

			ctx := c.Context
			_, err = cc.canvasSvc.UpdateCanvas(ctx, req)
			if err != nil {
				return errors.Wrap(err, "update canvas")
			}

			return nil
		},
	}
}

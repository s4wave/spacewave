//go:build !js

package spacewave_cli

import (
	"context"
	"io"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	cli_entrypoint "github.com/s4wave/spacewave/bldr/cli/entrypoint"
	world_types "github.com/s4wave/spacewave/db/world/types"
	forge_target "github.com/s4wave/spacewave/forge/target"
	flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
	sdk_engine "github.com/s4wave/spacewave/sdk/world/engine"
)

// flowgraphArgs selects the daemon, Space, and Flowgraph for a subcommand.
type flowgraphArgs struct {
	key        string
	statePath  string
	spaceID    string
	sessionIdx int
	output     string
}

// BuildFlags returns the flags shared by every flowgraph subcommand.
func (a *flowgraphArgs) BuildFlags() []cli.Flag {
	return []cli.Flag{
		&cli.StringFlag{
			Name:        "flowgraph",
			Aliases:     []string{"f"},
			Usage:       "Flowgraph object key (default: the only Flowgraph in the Space)",
			EnvVars:     []string{"SPACEWAVE_FLOWGRAPH"},
			Destination: &a.key,
		},
		statePathFlag(&a.statePath),
		socketPathFlag(),
		&cli.StringFlag{
			Name:        "space",
			Usage:       "Space ID or name (default: the only Space)",
			EnvVars:     []string{"SPACEWAVE_SPACE"},
			Destination: &a.spaceID,
		},
		&cli.IntFlag{
			Name:        "session-index",
			Usage:       "session index",
			EnvVars:     []string{"SPACEWAVE_SESSION_INDEX"},
			Value:       1,
			Destination: &a.sessionIdx,
		},
		&cli.StringFlag{
			Name:        "output",
			Aliases:     []string{"o"},
			Usage:       "output format (text/json/yaml)",
			EnvVars:     []string{"SPACEWAVE_OUTPUT"},
			Value:       "text",
			Destination: &a.output,
		},
	}
}

// mountEngine mounts the World engine of the selected Space.
func (a *flowgraphArgs) mountEngine(c *cli.Context) (*sdk_engine.SDKEngine, func(), error) {
	sessionIdx, err := sessionIndexFromInt(a.sessionIdx)
	if err != nil {
		return nil, nil, err
	}
	engine, cleanup, _, err := mountSpaceWorldEngine(c.Context, c, a.statePath, uint(sessionIdx), a.spaceID)
	return engine, cleanup, err
}

// mountResource mounts the selected Flowgraph's typed Resource.
func (a *flowgraphArgs) mountResource(c *cli.Context) (flowgraph.SRPCFlowgraphResourceServiceClient, string, func(), error) {
	// Resolve the session, then discover the Flowgraph when no key was given.
	sessionIdx, err := sessionIndexFromInt(a.sessionIdx)
	if err != nil {
		return nil, "", nil, err
	}
	var discover func(context.Context, *sdk_engine.SDKEngine) (string, error)
	if a.key == "" {
		discover = discoverFlowgraphObject
	}

	// Mount the chain from the daemon to the typed object.
	uri := fsURI{sessionIdx: sessionIdx, spaceID: a.spaceID, objectKey: a.key}
	mount, cleanup, err := mountObjectChain(c, a.statePath, uri, discover)
	if err != nil {
		return nil, "", nil, err
	}
	svc := flowgraph.NewSRPCFlowgraphResourceServiceClient(mount.typedClient)
	return svc, mount.objectKey, cleanup, nil
}

// update applies one edit and prints the resulting snapshot.
func (a *flowgraphArgs) update(c *cli.Context, req *flowgraph.UpdateFlowgraphRequest) error {
	// Apply the edit through the Flowgraph Resource in one transaction.
	svc, key, cleanup, err := a.mountResource(c)
	if err != nil {
		return err
	}
	defer cleanup()
	resp, err := svc.UpdateFlowgraph(c.Context, req)
	if err != nil {
		return errors.Wrap(err, "update flowgraph")
	}
	return a.writeSnapshot(key, resp.GetSnapshot())
}

// writeSnapshot prints a snapshot as a summary or a document.
func (a *flowgraphArgs) writeSnapshot(key string, snapshot *flowgraph.FlowgraphSnapshot) error {
	// Print documents in the requested format.
	if a.output == "json" || a.output == "yaml" {
		data, err := snapshot.MarshalJSON()
		if err != nil {
			return errors.Wrap(err, "marshal flowgraph")
		}
		return formatOutput(data, a.output)
	}

	// Print the header fields.
	w := os.Stdout
	state := snapshot.GetState()
	writeFields(w, [][2]string{
		{"Flowgraph", key},
		{"Name", state.GetName()},
		{"Revision", strconv.FormatUint(snapshot.GetRevision(), 10)},
	})

	// Print the nodes with their ports and placements.
	if nodes := state.GetNodes(); len(nodes) != 0 {
		io.WriteString(w, "\nNODES:\n")
		rows := [][]string{{"ID", "TYPE", "PORTS", "PLACEMENT"}}
		for _, id := range slices.Sorted(maps.Keys(nodes)) {
			placement := snapshot.GetPlacements()[id]
			rows = append(rows, []string{id, nodes[id].GetTypeId(), describePorts(nodes[id]), describePlacement(placement)})
		}
		writeTable(w, "  ", rows)
	}

	// Print the connections from output to input.
	if connections := state.GetConnections(); len(connections) != 0 {
		io.WriteString(w, "\nCONNECTIONS:\n")
		rows := [][]string{{"ID", "FROM", "TO"}}
		for _, id := range slices.Sorted(maps.Keys(connections)) {
			conn := connections[id]
			from := conn.GetOutputNode() + "." + conn.GetOutputPort()
			to := conn.GetInputNode() + "." + conn.GetInputPort()
			rows = append(rows, []string{id, from, to})
		}
		writeTable(w, "  ", rows)
	}
	return nil
}

// discoverFlowgraphObject finds exactly one Flowgraph in the Space.
func discoverFlowgraphObject(ctx context.Context, engine *sdk_engine.SDKEngine) (string, error) {
	// List the Flowgraphs from one read transaction and require exactly one.
	tx, err := engine.NewTransaction(ctx, false)
	if err != nil {
		return "", errors.Wrap(err, "new transaction")
	}
	defer tx.Discard()
	keys, err := world_types.ListObjectsWithType(ctx, tx, flowgraph.FlowgraphTypeID)
	if err != nil {
		return "", errors.Wrap(err, "list flowgraphs")
	}
	if len(keys) != 1 {
		return "", errors.Errorf("found %d flowgraphs; specify --flowgraph", len(keys))
	}
	return keys[0], nil
}

// newFlowgraphCommand builds the top-level flowgraph command group.
func newFlowgraphCommand(_ func() cli_entrypoint.CliBus) *cli.Command {
	return &cli.Command{
		Name:    "flowgraph",
		Aliases: []string{"flowgraphs"},
		Usage:   "author Flowgraphs of Device nodes and Steps",
		Subcommands: []*cli.Command{
			buildFlowgraphCreateCommand(),
			buildFlowgraphShowCommand(),
			buildFlowgraphNodeCommand(),
			buildFlowgraphConnectCommand(),
			buildFlowgraphDisconnectCommand(),
			buildFlowgraphHistoryCommand(),
			buildFlowgraphRunCommand(),
		},
	}
}

// flowgraphCreateArgs creates an independently addressed Flowgraph.
type flowgraphCreateArgs struct {
	flowgraphArgs
	name string
}

// BuildFlags returns the create flags.
func (a *flowgraphCreateArgs) BuildFlags() []cli.Flag {
	return append(a.flowgraphArgs.BuildFlags(), &cli.StringFlag{
		Name:        "name",
		Usage:       "display name",
		Destination: &a.name,
	})
}

// Run creates the Flowgraph named by the key argument.
func (a *flowgraphCreateArgs) Run(c *cli.Context) error {
	// Accept a full object key or a bare ID.
	if c.NArg() != 1 {
		return errors.New("create requires <id-or-object-key>")
	}
	key := c.Args().First()
	if _, err := flowgraph.ParseFlowgraphObjectKey(key); err != nil {
		key, err = flowgraph.FlowgraphObjectKey(key)
		if err != nil {
			return err
		}
	}

	// Apply the create operation in one World transaction.
	engine, cleanup, err := a.mountEngine(c)
	if err != nil {
		return err
	}
	defer cleanup()
	if err := applyWorldOp(c, engine, &flowgraph.CreateFlowgraphOp{ObjectKey: key, Name: a.name}); err != nil {
		return err
	}
	_, err = io.WriteString(os.Stdout, key+"\n")
	return err
}

// buildFlowgraphCreateCommand builds the create subcommand.
func buildFlowgraphCreateCommand() *cli.Command {
	args := &flowgraphCreateArgs{}
	return &cli.Command{
		Name:      "create",
		Usage:     "create a Flowgraph",
		ArgsUsage: "<id-or-object-key>",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

// buildFlowgraphShowCommand builds the show subcommand.
func buildFlowgraphShowCommand() *cli.Command {
	args := &flowgraphArgs{}
	return &cli.Command{
		Name:  "show",
		Usage: "show a Flowgraph's nodes, placements, and connections",
		Flags: args.BuildFlags(),
		Action: func(c *cli.Context) error {
			// Read the snapshot through the Flowgraph Resource.
			svc, key, cleanup, err := args.mountResource(c)
			if err != nil {
				return err
			}
			defer cleanup()
			resp, err := svc.GetFlowgraph(c.Context, &flowgraph.GetFlowgraphRequest{})
			if err != nil {
				return errors.Wrap(err, "get flowgraph")
			}
			return args.writeSnapshot(key, resp.GetSnapshot())
		},
	}
}

// buildFlowgraphNodeCommand builds the node command group.
func buildFlowgraphNodeCommand() *cli.Command {
	setArgs := &flowgraphNodeSetArgs{}
	rmArgs := &flowgraphArgs{}
	return &cli.Command{
		Name:  "node",
		Usage: "edit Flowgraph nodes",
		Subcommands: []*cli.Command{
			{
				Name:      "set",
				Usage:     "add or replace a node and its placement",
				ArgsUsage: "<node-id>",
				Flags:     setArgs.BuildFlags(),
				Action:    setArgs.Run,
			},
			{
				Name:      "rm",
				Usage:     "remove nodes with their placements and connections",
				ArgsUsage: "<node-id>...",
				Flags:     rmArgs.BuildFlags(),
				Action: func(c *cli.Context) error {
					if c.NArg() == 0 {
						return errors.New("rm requires at least one <node-id>")
					}
					return rmArgs.update(c, &flowgraph.UpdateFlowgraphRequest{RemoveNodeIds: c.Args().Slice()})
				},
			},
		},
	}
}

// flowgraphNodeSetArgs replaces one node and optionally its placement.
type flowgraphNodeSetArgs struct {
	flowgraphArgs
	typeID  string
	params  cli.StringSlice
	inputs  cli.StringSlice
	outputs cli.StringSlice
	prompt  string
	skills  cli.StringSlice
	device  string
	actor   string
	// targetFile holds a Target in generated protobuf JSON format.
	targetFile string
	// maxVisits bounds this Step's activations.
	maxVisits uint
	// maxSpend bounds this Step's reported spend.
	maxSpend uint64
}

// BuildFlags returns the node set flags.
func (a *flowgraphNodeSetArgs) BuildFlags() []cli.Flag {
	return append(
		a.flowgraphArgs.BuildFlags(),
		&cli.StringFlag{
			Name:        "type",
			Usage:       "node type (tcp-port, local-port, step, ...)",
			Required:    true,
			Destination: &a.typeID,
		},
		&cli.StringSliceFlag{Name: "param", Usage: "parameter key=value (repeatable)", Destination: &a.params},
		&cli.StringSliceFlag{Name: "in", Usage: "input port name:type (repeatable)", Destination: &a.inputs},
		&cli.StringSliceFlag{
			Name:        "out",
			Usage:       "output port name:type[:condition] (repeatable)",
			Destination: &a.outputs,
		},
		&cli.StringFlag{Name: "prompt", Usage: "Step prompt", Destination: &a.prompt},
		&cli.StringSliceFlag{Name: "skill", Usage: "Step skill name (repeatable)", Destination: &a.skills},
		&cli.StringFlag{Name: "device", Usage: "place the node on this Device object key", Destination: &a.device},
		&cli.StringFlag{Name: "actor", Usage: "place the Step on this actor object key", Destination: &a.actor},
		&cli.StringFlag{Name: "target-file", Usage: "Step Target file in protobuf JSON format", Destination: &a.targetFile},
		&cli.UintFlag{Name: "max-visits", Usage: "Step visit bound (zero is unlimited)", Destination: &a.maxVisits},
		&cli.Uint64Flag{Name: "max-spend", Usage: "Step spend bound (zero is unlimited)", Destination: &a.maxSpend},
	)
}

// Run builds the node from its flags and applies it in one edit.
func (a *flowgraphNodeSetArgs) Run(c *cli.Context) error {
	// Require the node ID and at most one placement.
	if c.NArg() != 1 {
		return errors.New("node set requires <node-id>")
	}
	id := c.Args().First()
	if a.device != "" && a.actor != "" {
		return errors.New("--device and --actor are mutually exclusive")
	}

	// Build the node from its flags.
	node, err := a.buildNode()
	if err != nil {
		return err
	}
	req := &flowgraph.UpdateFlowgraphRequest{SetNodes: map[string]*flowgraph.FlowgraphNode{id: node}}

	// Place the node in the same edit when requested.
	placement := &flowgraph.FlowgraphPlacement{Kind: flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE, ObjectKey: a.device}
	if a.actor != "" {
		placement = &flowgraph.FlowgraphPlacement{Kind: flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR, ObjectKey: a.actor}
	}
	if placement.GetObjectKey() != "" {
		req.SetPlacements = map[string]*flowgraph.FlowgraphPlacement{id: placement}
	}
	return a.update(c, req)
}

// buildNode converts the flags into a node.
func (a *flowgraphNodeSetArgs) buildNode() (*flowgraph.FlowgraphNode, error) {
	// Parse the parameters.
	node := &flowgraph.FlowgraphNode{TypeId: a.typeID}
	for _, param := range a.params.Value() {
		key, value, found := strings.Cut(param, "=")
		if !found {
			return nil, errors.Errorf("parameter %q must have the form key=value", param)
		}
		if node.Parameters == nil {
			node.Parameters = make(map[string]string)
		}
		node.Parameters[key] = value
	}

	// Parse the ports in flag order.
	for _, spec := range a.inputs.Value() {
		port, err := parsePort(spec, flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_INPUT)
		if err != nil {
			return nil, err
		}
		node.Ports = append(node.Ports, port)
	}
	for _, spec := range a.outputs.Value() {
		port, err := parsePort(spec, flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT)
		if err != nil {
			return nil, err
		}
		node.Ports = append(node.Ports, port)
	}

	// Attach the Step instructions to a Step node.
	if a.typeID == flowgraph.StepNodeTypeID {
		node.Step = &flowgraph.FlowgraphStep{Prompt: a.prompt}
		if a.maxVisits > uint(^uint32(0)) {
			return nil, errors.New("--max-visits exceeds uint32")
		}
		if a.maxVisits != 0 || a.maxSpend != 0 {
			node.Step.Bound = &flowgraph.FlowgraphBound{MaxVisits: uint32(a.maxVisits), MaxSpend: a.maxSpend}
		}
		for _, skill := range a.skills.Value() {
			node.Step.Skills = append(node.Step.Skills, &flowgraph.FlowgraphSkill{Name: skill})
		}
	}

	// Decode the execution definition without rebuilding its controller config.
	if a.targetFile != "" {
		if node.GetStep() == nil {
			return nil, errors.New("--target-file requires --type step")
		}
		data, err := os.ReadFile(a.targetFile)
		if err != nil {
			return nil, errors.Wrap(err, "read Step Target")
		}
		target := &forge_target.Target{}
		if err := target.UnmarshalJSON(data); err != nil {
			return nil, errors.Wrap(err, "decode Step Target")
		}
		node.Step.Target = target
	}
	return node, nil
}

// parsePort parses name:type, with an optional :condition on outputs.
func parsePort(spec string, direction flowgraph.FlowgraphPortDirection) (*flowgraph.FlowgraphPort, error) {
	// Split the name, the type, and an output's condition.
	parts := strings.SplitN(spec, ":", 3)
	if len(parts) < 2 || (len(parts) == 3 && direction != flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT) {
		return nil, errors.Errorf("port %q must have the form name:type", spec)
	}
	port := &flowgraph.FlowgraphPort{Name: parts[0], Direction: direction, TypeId: parts[1]}
	if len(parts) == 3 {
		port.Condition = parts[2]
	}
	return port, nil
}

// flowgraphConnectArgs adds or replaces one connection.
type flowgraphConnectArgs struct {
	flowgraphArgs
	from string
	to   string
}

// BuildFlags returns the connect flags.
func (a *flowgraphConnectArgs) BuildFlags() []cli.Flag {
	return append(
		a.flowgraphArgs.BuildFlags(),
		&cli.StringFlag{Name: "from", Usage: "output node.port", Required: true, Destination: &a.from},
		&cli.StringFlag{Name: "to", Usage: "input node.port", Required: true, Destination: &a.to},
	)
}

// Run applies the connection named by the argument.
func (a *flowgraphConnectArgs) Run(c *cli.Context) error {
	// Parse the connection ID and both endpoints.
	if c.NArg() != 1 {
		return errors.New("connect requires <connection-id>")
	}
	outNode, outPort, okOut := strings.Cut(a.from, ".")
	inNode, inPort, okIn := strings.Cut(a.to, ".")
	if !okOut || !okIn {
		return errors.New("--from and --to must have the form node.port")
	}
	conn := &flowgraph.FlowgraphConnection{OutputNode: outNode, OutputPort: outPort, InputNode: inNode, InputPort: inPort}
	return a.update(c, &flowgraph.UpdateFlowgraphRequest{
		SetConnections: map[string]*flowgraph.FlowgraphConnection{c.Args().First(): conn},
	})
}

// buildFlowgraphConnectCommand builds the connect subcommand.
func buildFlowgraphConnectCommand() *cli.Command {
	args := &flowgraphConnectArgs{}
	return &cli.Command{
		Name:      "connect",
		Usage:     "connect an output port to an input port",
		ArgsUsage: "<connection-id>",
		Flags:     args.BuildFlags(),
		Action:    args.Run,
	}
}

// buildFlowgraphDisconnectCommand builds the disconnect subcommand.
func buildFlowgraphDisconnectCommand() *cli.Command {
	args := &flowgraphArgs{}
	return &cli.Command{
		Name:      "disconnect",
		Usage:     "remove connections",
		ArgsUsage: "<connection-id>...",
		Flags:     args.BuildFlags(),
		Action: func(c *cli.Context) error {
			// Remove every named connection in one edit.
			if c.NArg() == 0 {
				return errors.New("disconnect requires at least one <connection-id>")
			}
			return args.update(c, &flowgraph.UpdateFlowgraphRequest{RemoveConnectionIds: c.Args().Slice()})
		},
	}
}

// buildFlowgraphHistoryCommand builds the history subcommand.
func buildFlowgraphHistoryCommand() *cli.Command {
	args := &flowgraphArgs{}
	return &cli.Command{
		Name:  "history",
		Usage: "list a Flowgraph's revisions from the Space changelog, oldest first",
		Flags: args.BuildFlags(),
		Action: func(c *cli.Context) error {
			// Read the retained history through the Flowgraph Resource.
			svc, _, cleanup, err := args.mountResource(c)
			if err != nil {
				return err
			}
			defer cleanup()
			resp, err := svc.ListFlowgraphChanges(c.Context, &flowgraph.ListFlowgraphChangesRequest{})
			if err != nil {
				return errors.Wrap(err, "list flowgraph changes")
			}

			// Print documents in the requested format, newest first.
			if args.output == "json" || args.output == "yaml" {
				data, err := resp.MarshalJSON()
				if err != nil {
					return errors.Wrap(err, "marshal flowgraph changes")
				}
				return formatOutput(data, args.output)
			}

			// Explain a Space that keeps no history.
			if resp.GetChangelogDisabled() {
				os.Stdout.WriteString("This Space does not keep a changelog, so its Flowgraphs have no history.\nTurn it on with: spacewave space settings --changelog on\n")
				return nil
			}

			// Print one row per revision, oldest first.
			rows := [][]string{{"SEQNO", "CHANGE"}}
			for _, change := range slices.Backward(resp.GetChanges()) {
				rows = append(rows, []string{strconv.FormatUint(change.GetSeqno(), 10), describeChange(change)})
			}
			writeTable(os.Stdout, "", rows)
			if !resp.GetComplete() {
				os.Stdout.WriteString("Older revisions are no longer in the Space changelog.\n")
			}
			return nil
		},
	}
}

// describeChange summarizes one revision.
func describeChange(change *flowgraph.FlowgraphChange) string {
	// Name the creation, or list a rename.
	edit := change.GetEdit()
	var parts []string
	switch {
	case change.GetCreated():
		parts = append(parts, "create "+strconv.Quote(edit.GetName()))
	case edit.Name != nil:
		parts = append(parts, "name "+strconv.Quote(edit.GetName()))
	}

	// List the node and connection edits.
	parts = appendEdit(parts, "+node", slices.Sorted(maps.Keys(edit.GetSetNodes())))
	parts = appendEdit(parts, "-node", edit.GetRemoveNodeIds())
	parts = appendEdit(parts, "+connection", slices.Sorted(maps.Keys(edit.GetSetConnections())))
	parts = appendEdit(parts, "-connection", edit.GetRemoveConnectionIds())

	// List the placement edits with each destination.
	var placed []string
	for _, id := range slices.Sorted(maps.Keys(edit.GetSetPlacements())) {
		placed = append(placed, id+"@"+edit.GetSetPlacements()[id].GetObjectKey())
	}
	parts = appendEdit(parts, "+placement", placed)
	parts = appendEdit(parts, "-placement", edit.GetRemovePlacementNodeIds())
	return strings.Join(parts, "; ")
}

// appendEdit appends "label id,id" when ids is not empty.
func appendEdit(parts []string, label string, ids []string) []string {
	if len(ids) == 0 {
		return parts
	}
	return append(parts, label+" "+strings.Join(ids, ","))
}

// describePorts lists a node's ports as direction name:type.
func describePorts(node *flowgraph.FlowgraphNode) string {
	var ports []string
	for _, port := range node.GetPorts() {
		prefix := "in "
		if port.GetDirection() == flowgraph.FlowgraphPortDirection_FLOWGRAPH_PORT_DIRECTION_OUTPUT {
			prefix = "out "
		}
		desc := prefix + port.GetName() + ":" + port.GetTypeId()
		if port.GetCondition() != "" {
			desc += " if " + port.GetCondition()
		}
		ports = append(ports, desc)
	}
	return strings.Join(ports, ", ")
}

// describePlacement names the placement kind and destination.
func describePlacement(placement *flowgraph.FlowgraphPlacement) string {
	switch placement.GetKind() {
	case flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_DEVICE:
		return "device " + placement.GetObjectKey()
	case flowgraph.FlowgraphPlacementKind_FLOWGRAPH_PLACEMENT_KIND_ACTOR:
		return "actor " + placement.GetObjectKey()
	default:
		return ""
	}
}

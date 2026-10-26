package alarm_watch

import (
	"context"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/aperturerobotics/controllerbus/config"
	"github.com/pkg/errors"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// NodeTypeID selects the Alarm watch node type. The node binds the Alarm it
// writes as its object.
const NodeTypeID = "alarm/watch"

// The parameters of an Alarm watch node.
const (
	// ParamFunction names the function/pure object that decides the Alarm.
	ParamFunction = "function"
	// ParamHold is how long the condition must hold before the Alarm fires,
	// as a Go duration such as "2m".
	ParamHold = "hold"
	// ParamTitle is the Alarm's title.
	ParamTitle = "title"
	// ParamSource names the object whose resource answers the queries.
	ParamSource = "source"
	// ParamQueryPrefix prefixes each named query: "query.up" holds the
	// expression of the query named "up".
	ParamQueryPrefix = "query."
	// ParamObjects lists the World objects the function reads, separated by
	// commas.
	ParamObjects = "objects"
)

// NodeType is the Alarm watch node type. Its node runs a watch controller on
// the Device the node is placed on.
type NodeType struct{}

// GetDisplayName returns the name shown for the node type.
func (NodeType) GetDisplayName() string {
	return "Alarm Watch"
}

// GetPorts returns no ports, since the watch names its inputs in parameters.
func (NodeType) GetPorts() []*s4wave_flowgraph.FlowgraphPort {
	return nil
}

// GetConfigIDs returns the watch controller config ID.
func (NodeType) GetConfigIDs() []string {
	return []string{ConfigID}
}

// Compile returns the watch controller config of the node.
func (NodeType) Compile(_ context.Context, node *s4wave_flowgraph.PlacedFlowgraphNode) (map[string]config.Config, error) {
	conf, err := NewConfigFromNode(node)
	if err != nil {
		return nil, err
	}
	return map[string]config.Config{"watch": conf}, nil
}

// NewConfigFromNode builds the watch config of a placed node.
func NewConfigFromNode(node *s4wave_flowgraph.PlacedFlowgraphNode) (*Config, error) {
	// Read the Alarm, the function, the source and the title.
	params := node.Node.GetParameters()
	conf := &Config{
		EngineId:    node.EngineID,
		AlarmKey:    node.Node.GetObjectKey(),
		FunctionKey: params[ParamFunction],
		Title:       params[ParamTitle],
		SourceKey:   params[ParamSource],
	}

	// Parse the hold time.
	if hold := params[ParamHold]; hold != "" {
		d, err := time.ParseDuration(hold)
		if err != nil || d < 0 {
			return nil, errors.Errorf("hold %q is not a duration", hold)
		}
		conf.HoldMs = d.Milliseconds()
	}

	// Collect the named queries in name order, and the objects in the order
	// given.
	for _, param := range slices.Sorted(maps.Keys(params)) {
		if name, ok := strings.CutPrefix(param, ParamQueryPrefix); ok {
			conf.Queries = append(conf.Queries, &Query{Name: name, Expr: params[param]})
		}
	}
	for key := range strings.SplitSeq(params[ParamObjects], ",") {
		if key = strings.TrimSpace(key); key != "" {
			conf.ObjectKeys = append(conf.ObjectKeys, key)
		}
	}
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return conf, nil
}

// _ is a type assertion
var _ s4wave_flowgraph.FlowgraphNodeType = NodeType{}

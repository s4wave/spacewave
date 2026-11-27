// Package unixfs_linkedfolder is the linked-folder Flowgraph node: placed on
// a Device and bound to a Drive, it keeps the Drive equal to a folder on that
// Device, with the files' blocks read from the files.
package unixfs_linkedfolder

import (
	"context"

	"github.com/aperturerobotics/controllerbus/config"
	s4wave_flowgraph "github.com/s4wave/spacewave/sdk/flowgraph"
)

// NodeTypeID selects the linked-folder node type. The node binds the Drive it
// writes as its object.
const NodeTypeID = "unixfs/linked-folder"

// The parameters of a linked-folder node.
const (
	// ParamFolder is the absolute path of the folder on the Device.
	ParamFolder = "folder"
	// ParamPath is the path in the Drive the folder links to.
	ParamPath = "path"
	// ParamTwoWay applies others' changes to the Drive to the folder too when
	// set to "true". Any other value links the folder one way.
	ParamTwoWay = "two-way"
)

// NodeType is the linked-folder node type. Its node runs a linked-folder
// controller on the Device the node is placed on.
type NodeType struct{}

// GetDisplayName returns the name shown for the node type.
func (NodeType) GetDisplayName() string {
	return "Linked Folder"
}

// GetPorts returns no ports, since the node specifies its folder in parameters
// and binds its Drive as its object.
func (NodeType) GetPorts() []*s4wave_flowgraph.FlowgraphPort {
	return nil
}

// GetConfigIDs returns the linked-folder controller config ID.
func (NodeType) GetConfigIDs() []string {
	return []string{ConfigID}
}

// Compile returns the linked-folder controller config of the node.
func (NodeType) Compile(_ context.Context, node *s4wave_flowgraph.PlacedFlowgraphNode) (map[string]config.Config, error) {
	params := node.Node.GetParameters()
	conf := &Config{
		EngineId:  node.EngineID,
		ObjectKey: node.Node.GetObjectKey(),
		Folder:    params[ParamFolder],
		Path:      params[ParamPath],
		TwoWay:    params[ParamTwoWay] == "true",
	}
	if err := conf.Validate(); err != nil {
		return nil, err
	}
	return map[string]config.Config{"linked-folder": conf}, nil
}

// _ is a type assertion
var _ s4wave_flowgraph.FlowgraphNodeType = NodeType{}

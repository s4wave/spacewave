package node

import (
	"github.com/aperturerobotics/controllerbus/controller"
)

// Node tracks volumes and buckets across a running hydra process.
type Node interface{}

// Controller describes the node controller. The node controller manages a
// running Hydra instance, tracking running volumes, reconciling bucket
// configuration versions, constructing bucket handles and starting lookup
// controllers to service lookup requests. There is usually only one controller
// per Hydra node.
type Controller interface {
	// Controller indicates the node controller is a controller.
	controller.Controller
	// Node indicates the controller implements the Node interface.
	Node
}

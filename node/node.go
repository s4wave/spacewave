package node

import (
	"github.com/aperturerobotics/controllerbus/controller"
)

// Controller describes the node controller. The node controller manages a
// tracking available block stores, reconciling bucket configuration versions,
// constructing bucket handles and starting lookup controllers to service lookup
// requests. There is usually only one controller per Hydra node.
type Controller interface {
	// Controller indicates the node controller is a controller.
	controller.Controller
}

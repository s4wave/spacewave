package storage_inmem

import "github.com/aperturerobotics/controllerbus/controller"

// ControllerID is the controller identifier.
const ControllerID = "bldr/storage/inmem"

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

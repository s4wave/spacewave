package hydra_entitygraph

import (
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/entitygraph/reporter"
	"github.com/blang/semver"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "hydra/entitygraph/reporter"

// Controller manages exposing Bifrost resources to the Entity Graph.
// It handles CollectEntityGraph directives.
type Controller = reporter.Controller

// GetControllerInfo returns information about the controller.
func GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"hydra entitygraph reporter controller",
	)
}

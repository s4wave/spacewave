package floodsub_controller

import (
	"github.com/blang/semver/v4"
	pubsub_controller "github.com/s4wave/spacewave/net/pubsub/controller"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "bifrost/floodsub"

// Controller implements the FloodSub controller.
type Controller = pubsub_controller.Controller

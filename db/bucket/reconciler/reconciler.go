package reconciler

import (
	"github.com/aperturerobotics/controllerbus/controller"
)

// Controller is a reconciler controller.
type Controller interface {
	// Controller is the controllerbus controller interface.
	controller.Controller
}

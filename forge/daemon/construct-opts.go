//go:build !js && !wasip1

package daemon

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/sirupsen/logrus"
)

// ConstructOpts are extra options passed to the daemon constructor.
type ConstructOpts struct {
	// LogEntry is the root logger to use. A nil entry selects the default logger.
	LogEntry *logrus.Entry
	// ExtraControllerFactories is a set of extra controller factories to
	// make available to the daemon.
	ExtraControllerFactories []func(bus.Bus) controller.Factory
}

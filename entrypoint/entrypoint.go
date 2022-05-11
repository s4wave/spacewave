package entrypoint

import (
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/sirupsen/logrus"
)

// Environment represents the environment executing the Go runtime.
type Environment interface {
	// GetLogger returns the root log entry.
	GetLogger() *logrus.Entry
	// GetBus returns the root controller bus to use in this process.
	GetBus() bus.Bus
}

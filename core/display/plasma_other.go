//go:build !linux && !js

package display

import (
	"context"
	"errors"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/sirupsen/logrus"
)

// servePlasma reports that the Plasma source requires Linux.
func servePlasma(context.Context, *logrus.Entry, bus.Bus, protocol.ID) error {
	return errors.New("Plasma display capture requires Linux")
}

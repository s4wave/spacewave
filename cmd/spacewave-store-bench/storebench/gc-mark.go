//go:build !js

package storebench

import (
	"context"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/volume"
)

// collectLoop runs collection passes on the account volume back to back until
// ctx ends, so every write of the window overlaps a pass, and logs each pass.
func (l *load) collectLoop(ctx context.Context) error {
	// Collect through the volume, as the account cleanup does.
	collector, ok := l.vol.(volume.Collector)
	if !ok {
		return errors.New("gc: the account volume does not collect")
	}

	// Run passes until the load stops, ignoring the error of the cut pass.
	for passes := 1; ; passes++ {
		res, err := collector.Collect(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			return errors.Wrap(err, "gc")
		}
		l.le.WithField("pass", passes).
			WithField("duration", res.Duration).
			WithField("netted", res.Netted).
			WithField("freed", res.Freed).
			WithField("hold_mark", res.HoldMark).
			Info("gc pass")
	}
}

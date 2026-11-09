package sobject_world_engine

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/core/sobject"
)

// SetSlowCommitThreshold sets the duration after which a World write logs its
// stage times and returns a function that restores the previous one.
func SetSlowCommitThreshold(threshold time.Duration) func() {
	previous := slowCommitThreshold
	slowCommitThreshold = threshold
	return func() { slowCommitThreshold = previous }
}

// SetSlowHoldThreshold sets the writer lock hold after which a World write
// logs its stage times and returns a function that restores the previous one.
func SetSlowHoldThreshold(threshold time.Duration) func() {
	previous := slowHoldThreshold
	slowHoldThreshold = threshold
	return func() { slowHoldThreshold = previous }
}

// ExecuteWorld serves the World of so on c until ctx ends or an error ends it,
// as Execute does for the SharedObject it mounts.
func ExecuteWorld(ctx context.Context, c *Controller, so sobject.SharedObject) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	return c.executeSharedObject(ctx, cancel, so)
}

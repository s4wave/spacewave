package space_world_replica_controller

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
)

// wakeOnChange calls wake each time the value of w changes from initial, until
// ctx ends. The caller reads initial before it next observes w, so no change
// after that read goes unseen.
func wakeOnChange[T comparable](ctx context.Context, w ccontainer.Watchable[T], initial T, wake func()) {
	_ = ccontainer.WatchChanges(ctx, initial, w, func(T) error {
		wake()
		return nil
	}, nil)
}

//go:build !js

package sobject_world_engine_test

import (
	"context"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/core/sobject"
)

// backfillWatch delays replay following while the real restored World is served.
type backfillWatch struct {
	ccontainer.Watchable[sobject.SharedObjectStateSnapshot]
	// wait holds dependency delivery until release or lifetime cancellation.
	wait func(context.Context) error
}

// WaitValueChange delays only the first watch input, not startup snapshot reads.
func (w *backfillWatch) WaitValueChange(ctx context.Context, old sobject.SharedObjectStateSnapshot, errCh <-chan error) (sobject.SharedObjectStateSnapshot, error) {
	// Startup has already replayed the nonempty state through WaitValue.
	if old == nil {
		if err := w.wait(ctx); err != nil {
			return nil, err
		}
	}
	return w.Watchable.WaitValueChange(ctx, old, errCh)
}

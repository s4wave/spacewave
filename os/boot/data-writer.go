//go:build linux

package boot

import (
	"context"
	"time"

	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
)

// dataWriter binds buffered WriteAt commits to Data's controller availability.
// Other operations carry their caller's request context; only WriteAt is also
// called by FSHandle's background idle/release flush. Storage remains borrowed.
type dataWriter struct {
	// FSWriter preserves the ordinary durable World write path and capabilities.
	*unixfs_world.FSWriter
	// ctx ends when a required controller stops, or after Data's final flush.
	ctx context.Context
}

// WriteAt interrupts even a background flush when its writer becomes unavailable.
func (w *dataWriter) WriteAt(ctx context.Context, path []string, offset int64, data []byte, ts time.Time) error {
	// Refuse a stopped writer before attempting another World operation.
	if err := context.Cause(w.ctx); err != nil {
		return err
	}

	// Preserve request cancellation while also watching the required controllers.
	writeCtx, cancel := context.WithCancel(ctx)
	stop := context.AfterFunc(w.ctx, cancel)
	defer stop()
	defer cancel()
	return w.FSWriter.WriteAt(writeCtx, path, offset, data, ts)
}

// _ verifies the production filesystem writer contract.
var _ unixfs.FSWriter = (*dataWriter)(nil)

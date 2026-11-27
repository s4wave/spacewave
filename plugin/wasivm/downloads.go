package wasivm_plugin

import (
	"context"

	"github.com/aperturerobotics/util/broadcast"
	cdn_bstore "github.com/s4wave/spacewave/core/cdn/bstore"
	s4wave_vm "github.com/s4wave/spacewave/sdk/vm"
)

// downloads watches the reads a run makes from the CDN for its root
// filesystem.
type downloads struct {
	// store is the CDN transport of the root filesystem, or nil when the root
	// filesystem lives in the Space.
	store *cdn_bstore.CdnBlockStore
	// bcast wakes watchers when the store's stats change.
	bcast broadcast.Broadcast
}

// newDownloads watches the reads of store, which may be nil. It takes the
// store's one stats callback for the life of the run.
func newDownloads(store *cdn_bstore.CdnBlockStore) *downloads {
	d := &downloads{store: store}
	if store != nil {
		store.SetStatsChangedCallback(func() {
			d.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				broadcast()
			})
		})
	}
	return d
}

// snapshot returns the run's downloads so far.
func (d *downloads) snapshot() *s4wave_vm.WasiVmDownloads {
	if d.store == nil {
		return &s4wave_vm.WasiVmDownloads{}
	}
	stats := d.store.SnapshotStats()
	return &s4wave_vm.WasiVmDownloads{
		Remote:   true,
		InFlight: uint32(stats.InFlightFetches),                                      //nolint:gosec // counts are never negative.
		Bytes:    uint64(stats.RangeResponseBytes + stats.FullResponseFallbackBytes), //nolint:gosec // counts are never negative.
	}
}

// watch sends the run's downloads, then again each time they change, until
// ctx ends. Changes made while send blocks are folded into the next message.
func (d *downloads) watch(ctx context.Context, send func(*s4wave_vm.WasiVmDownloads) error) error {
	var sent *s4wave_vm.WasiVmDownloads
	for {
		var waitCh <-chan struct{}
		d.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
		})
		if next := d.snapshot(); !next.EqualVT(sent) {
			if err := send(next); err != nil {
				return err
			}
			sent = next
		}
		select {
		case <-ctx.Done():
			return context.Cause(ctx)
		case <-waitCh:
		}
	}
}

// vmDownloads serves the downloads of one VM on each of its resources. A
// viewer that attaches before the VM runs waits for it.
type vmDownloads struct {
	consoles  *consoleTable
	objectKey string
}

// WatchDownloads streams the downloads of the running VM's root filesystem
// until the run ends or the viewer leaves.
func (v *vmDownloads) WatchDownloads(_ *s4wave_vm.WatchWasiVmDownloadsRequest, strm s4wave_vm.SRPCWasiVmDownloadService_WatchDownloadsStream) error {
	// Wait for the VM to run.
	ctx := strm.Context()
	c, err := v.consoles.wait(ctx, v.objectKey)
	if err != nil {
		return err
	}

	// Stream the running console's root filesystem downloads.
	return c.watchDownloads(ctx, strm.Send)
}

// _ is a type assertion
var _ s4wave_vm.SRPCWasiVmDownloadServiceServer = (*vmDownloads)(nil)

package kvtx

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/volume"
)

// collection asks, and no longer after Close.
func TestHealthSizes(t *testing.T) {
	// Measure a growing file.
	v, _ := newPublicationTestVolume(t)
	var calls atomic.Int64
	measured := make(chan struct{}, 8)
	v.SetHealthSizes(func(context.Context) (HealthSizes, error) {
		n := calls.Add(1)
		measured <- struct{}{}
		return HealthSizes{FileBytes: uint64(n) * 100, LiveValueBytes: 10, FreeDiskBytes: 1 << 40}, nil
	})
	waitMeasured := func() {
		t.Helper()
		select {
		case <-measured:
		case <-time.After(5 * time.Second):
			t.Fatal("no measurement")
		}
	}

	// The first measurement runs at once and reaches the snapshot.
	waitMeasured()
	h, waitCh := v.GetHealthSnapshotWithWait()
	for !h.GetSizesKnown() {
		<-waitCh
		h, waitCh = v.GetHealthSnapshotWithWait()
	}
	if h.GetFileBytes() != 100 || h.GetLiveValueBytes() != 10 || h.GetFreeDiskBytes() != 1<<40 {
		t.Fatalf("sizes %v", h)
	}

	// A collection's wake measures again.
	v.wakeHealthSizes()
	waitMeasured()

	// Close stops the measurement.
	if err := v.Close(); err != nil {
		t.Fatal(err)
	}
	after := calls.Load()
	v.wakeHealthSizes()
	time.Sleep(2 * healthPoll)
	if calls.Load() != after {
		t.Fatal("measured after Close")
	}
}

// TestHealthDirectWrites checks that new data written outside the publication
// writer makes a measurement due without a wake.
func TestHealthDirectWrites(t *testing.T) {
	// Measure a 16 MiB free disk, so 2 MiB of writes are due a measurement,
	// reporting the count of measurements as the file length.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	var calls atomic.Uint64
	v.SetHealthSizes(func(context.Context) (HealthSizes, error) {
		return HealthSizes{FileBytes: calls.Add(1), FreeDiskBytes: 16 << 20}, nil
	})
	waitHealth(t, v, (*volume.VolumeHealth).GetSizesKnown)
	before := calls.Load()

	// A 3 MiB block through PutBlock is measured on a following poll.
	if _, _, err := v.PutBlock(ctx, make([]byte, 3<<20), leafOpts()); err != nil {
		t.Fatal(err)
	}
	waitHealth(t, v, func(h *volume.VolumeHealth) bool {
		return h.GetFileBytes() > before
	})
}

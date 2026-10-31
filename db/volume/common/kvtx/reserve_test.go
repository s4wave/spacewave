package kvtx

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/volume"
)

// TestReserve moves a volume's free disk across its reserve and checks that
// new writes are refused with the named error while reads and exempt writes
// continue, that the state holds inside the margin, and that writes resume
// above it.
func TestReserve(t *testing.T) {
	// Measure a chosen free disk against a 100 MiB reserve.
	v, _ := newPublicationTestVolume(t)
	ctx := t.Context()
	const reserve = 100 << 20
	var free atomic.Uint64
	free.Store(1 << 30)
	v.SetHealthSizes(func(context.Context) (HealthSizes, error) {
		return HealthSizes{FreeDiskBytes: free.Load(), ReserveBytes: reserve}, nil
	})
	waitWriteState(t, v, volume.VolumeWriteState_VOLUME_WRITE_STATE_WRITABLE)
	if err := v.PublishAtomic(ctx, publicationFor(t, "reserve-a", "", "one")); err != nil {
		t.Fatal(err)
	}

	// Below the reserve, new data is refused and reads continue.
	free.Store(reserve / 2)
	v.wakeHealthSizes()
	waitWriteState(t, v, volume.VolumeWriteState_VOLUME_WRITE_STATE_RESERVE)
	if err := v.PublishAtomic(ctx, publicationFor(t, "reserve-b", "", "one")); !errors.Is(err, volume.ErrReserve) {
		t.Fatalf("publish below the reserve: %v", err)
	}
	if _, _, err := v.PrepareOwnedBlock(ctx, "owner", []byte("refused"), nil); !errors.Is(err, volume.ErrReserve) {
		t.Fatalf("prepare below the reserve: %v", err)
	}
	if _, _, err := v.PutBlock(ctx, []byte("refused"), nil); !errors.Is(err, volume.ErrReserve) {
		t.Fatalf("put below the reserve: %v", err)
	}
	stored := publicationFor(t, "reserve-a", "", "one").Entries[0].Ref
	if _, found, err := v.GetBlock(ctx, stored); err != nil || !found {
		t.Fatalf("read below the reserve: found %v, %v", found, err)
	}

	// An exempt write is admitted below the reserve.
	exempt := volume.WithReserveExempt(ctx)
	if err := v.PublishAtomic(exempt, publicationFor(t, "reserve-c", "", "one")); err != nil {
		t.Fatalf("exempt publish below the reserve: %v", err)
	}

	// Inside the margin the state holds; above it, writes resume.
	free.Store(reserve + reserve/reserveMarginDivisor/2)
	v.wakeHealthSizes()
	waitSizes(t, v, free.Load())
	if err := v.PublishAtomic(ctx, publicationFor(t, "reserve-b", "", "one")); !errors.Is(err, volume.ErrReserve) {
		t.Fatalf("publish inside the margin: %v", err)
	}
	free.Store(2 * reserve)
	v.wakeHealthSizes()
	waitWriteState(t, v, volume.VolumeWriteState_VOLUME_WRITE_STATE_WRITABLE)
	if err := v.PublishAtomic(ctx, publicationFor(t, "reserve-b", "", "one")); err != nil {
		t.Fatalf("publish above the margin: %v", err)
	}
}

// TestInReserve checks the reserve state's entry, margin and exit, and that
// a volume with no reserve is always writable.
func TestInReserve(t *testing.T) {
	// Each row is the free disk and the prior state.
	const reserve = 400
	for _, c := range []struct {
		free     uint64
		reserve  uint64
		was, out bool
	}{
		{free: 500, reserve: reserve, was: false, out: false},
		{free: 399, reserve: reserve, was: false, out: true},
		{free: 450, reserve: reserve, was: true, out: true},
		{free: 500, reserve: reserve, was: true, out: false},
		{free: 0, reserve: 0, was: true, out: false},
	} {
		if got := inReserve(HealthSizes{FreeDiskBytes: c.free, ReserveBytes: c.reserve}, c.was); got != c.out {
			t.Errorf("inReserve(free %d, reserve %d, was %v) = %v", c.free, c.reserve, c.was, got)
		}
	}
}

// waitWriteState waits for the health to report state.
func waitWriteState(t *testing.T, v *Volume, state volume.VolumeWriteState) {
	t.Helper()
	waitHealth(t, v, func(h *volume.VolumeHealth) bool {
		return h.GetSizesKnown() && h.GetWriteState() == state
	})
}

// waitSizes waits for the health to report a measurement of free bytes.
func waitSizes(t *testing.T, v *Volume, free uint64) {
	t.Helper()
	waitHealth(t, v, func(h *volume.VolumeHealth) bool {
		return h.GetSizesKnown() && h.GetFreeDiskBytes() == free
	})
}

// waitHealth waits on the health watch until ok accepts a snapshot.
func waitHealth(t *testing.T, v *Volume, ok func(*volume.VolumeHealth) bool) {
	// Watch the health until ok or the deadline.
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		h, waitCh := v.GetHealthSnapshotWithWait()
		if ok(h) {
			return
		}
		select {
		case <-waitCh:
		case <-deadline:
			t.Fatalf("health %v did not reach the wanted state", h)
		}
	}
}

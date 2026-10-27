package kvtx

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/metrics"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	"github.com/s4wave/spacewave/db/volume"
)

// TestHealthHeldPhase holds a block pass at its opening commit and checks
// that the health reports the mark, that its time in phase grows while held in
// the watch and in the process metrics, and that the volume returns to idle
// with a wake when the pass ends.
func TestHealthHeldPhase(t *testing.T) {
	// Publish a block, then start a block pass held at its opening commit.
	v, s := newPublicationTestVolume(t)
	ctx := t.Context()
	if err := v.PublishAtomic(ctx, publicationFor(t, "health", "", "one")); err != nil {
		t.Fatal(err)
	}
	defer volume.RegisterHealth(v)()
	gate := s.arm(t)
	passErr := make(chan error, 1)
	go func() {
		var stats block_gc.Stats
		passErr <- v.SweepUnreachedBlocks(ctx, v.refGraph, &stats)
	}()
	<-gate.started

	// The health reports the mark, with a fixed start the time in phase
	// grows from.
	held, waitCh := v.GetHealthSnapshotWithWait()
	if held.GetGcPhase() != volume.VolumeGCPhase_VOLUME_GC_PHASE_MARK {
		t.Fatalf("phase %v while the pass is held", held.GetGcPhase())
	}
	time.Sleep(20 * time.Millisecond)
	again, _ := v.GetHealthSnapshotWithWait()
	if !again.GetGcPhaseStart().AsTime().Equal(held.GetGcPhaseStart().AsTime()) {
		t.Fatal("the phase start moved while the phase held")
	}
	if time.Since(again.GetGcPhaseStart().AsTime()) < 20*time.Millisecond {
		t.Fatal("time in phase did not grow")
	}

	// The scrape reports the mark and its time in phase grows.
	first := scrapePhaseSeconds(t, v.GetID())
	time.Sleep(20 * time.Millisecond)
	if second := scrapePhaseSeconds(t, v.GetID()); second < first+0.02 {
		t.Fatalf("scraped time in phase went from %v to %v", first, second)
	}

	// Releasing the pass returns the volume to idle and wakes the watcher.
	gate.unblock()
	if err := <-passErr; err != nil {
		t.Fatal(err)
	}
	select {
	case <-waitCh:
	case <-time.After(5 * time.Second):
		t.Fatal("the watch did not wake")
	}
	done, _ := v.GetHealthSnapshotWithWait()
	if done.GetGcPhase() != volume.VolumeGCPhase_VOLUME_GC_PHASE_IDLE {
		t.Fatalf("phase %v after the pass", done.GetGcPhase())
	}
}

// TestHealthSizes checks that the sizes are measured at start, again when a
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
	if _, _, err := v.PutBlock(ctx, make([]byte, 3<<20), nil); err != nil {
		t.Fatal(err)
	}
	waitHealth(t, v, func(h *volume.VolumeHealth) bool {
		return h.GetFileBytes() > before
	})
}

// scrapePhaseSeconds gathers the process metrics, checks that volume id is in
// the mark, and returns its time in phase.
func scrapePhaseSeconds(t *testing.T, id string) float64 {
	// Gather the families.
	t.Helper()
	families, err := metrics.Gather()
	if err != nil {
		t.Fatal(err)
	}

	// Find the volume's mark series and its time in phase.
	var mark, seconds float64 = -1, -1
	for _, f := range families {
		for _, m := range f.GetMetric() {
			labels := make(map[string]string)
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["volume"] != id {
				continue
			}
			switch {
			case f.GetName() == "spacewave_volume_gc_phase" && labels["phase"] == "mark":
				mark = m.GetGauge().GetValue()
			case f.GetName() == "spacewave_volume_gc_phase_seconds":
				seconds = m.GetGauge().GetValue()
			}
		}
	}
	if mark != 1 || seconds < 0 {
		t.Fatalf("scrape of %q: mark %v, seconds %v", id, mark, seconds)
	}
	return seconds
}

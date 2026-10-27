//go:build !js

package metrics

import (
	"testing"
	"time"

	timestamppb "github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/s4wave/spacewave/db/volume"
)

// heldVolume reports a volume held in the mark since start.
type heldVolume struct {
	// start is when the volume entered the mark.
	start time.Time
}

// GetHealthSnapshotWithWait returns the held health.
func (v *heldVolume) GetHealthSnapshotWithWait() (*volume.VolumeHealth, <-chan struct{}) {
	return &volume.VolumeHealth{
		VolumeId:      "test/held",
		GcPhase:       volume.VolumeGCPhase_VOLUME_GC_PHASE_MARK,
		GcPhaseStart:  timestamppb.New(v.start),
		SizesKnown:    true,
		SizesAt:       timestamppb.New(time.Now()),
		FreeDiskBytes: 1 << 30,
		ReserveBytes:  1 << 28,
		WriteState:    volume.VolumeWriteState_VOLUME_WRITE_STATE_RESERVE,
	}, nil
}

// TestVolumeHealthScrape checks that a scrape reports a registered volume's
// phase, its time in phase, its free disk and its reserve, and drops it once
// released.
func TestVolumeHealthScrape(t *testing.T) {
	// Scrape with a volume held in the mark for a minute.
	release := volume.RegisterHealth(&heldVolume{start: time.Now().Add(-time.Minute)})
	values := scrapeVolume(t, "test/held")
	if values["spacewave_volume_gc_phase{phase=mark}"] != 1 || values["spacewave_volume_gc_phase{phase=idle}"] != 0 {
		t.Fatalf("phase series %v", values)
	}
	if values["spacewave_volume_gc_phase_seconds"] < 60 {
		t.Fatalf("time in phase %v", values["spacewave_volume_gc_phase_seconds"])
	}
	if values["spacewave_volume_free_disk_bytes"] != 1<<30 {
		t.Fatalf("free disk %v", values["spacewave_volume_free_disk_bytes"])
	}
	if values["spacewave_volume_reserve_bytes"] != 1<<28 || values["spacewave_volume_write_reserve"] != 1 {
		t.Fatalf("reserve series %v", values)
	}

	// A released volume leaves the scrape.
	release()
	if values := scrapeVolume(t, "test/held"); len(values) != 0 {
		t.Fatalf("released volume scraped: %v", values)
	}
}

// scrapeVolume gathers the process metrics and returns the gauges of volume
// id, keyed by name and any phase label.
func scrapeVolume(t *testing.T, id string) map[string]float64 {
	// Gather the families.
	t.Helper()
	families, err := Gather()
	if err != nil {
		t.Fatal(err)
	}

	// Keep the series of the volume.
	values := make(map[string]float64)
	for _, f := range families {
		for _, m := range f.GetMetric() {
			labels := make(map[string]string)
			for _, l := range m.GetLabel() {
				labels[l.GetName()] = l.GetValue()
			}
			if labels["volume"] != id {
				continue
			}
			key := f.GetName()
			if phase, ok := labels["phase"]; ok {
				key += "{phase=" + phase + "}"
			}
			values[key] = m.GetGauge().GetValue()
		}
	}
	return values
}

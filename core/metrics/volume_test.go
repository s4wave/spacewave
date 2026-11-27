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

		NotHeldRootBlocks:  2,
		NotHeldChildBlocks: 7,
	}, nil
}

// TestVolumeHealthScrape checks that a scrape reports a registered volume's
// phase, its time in phase, its free disk, its reserve and its not-held
// counters, and drops it once released.
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
	if values["spacewave_volume_not_held_root_blocks_total"] != 2 || values["spacewave_volume_not_held_child_blocks_total"] != 7 {
		t.Fatalf("not-held series %v", values)
	}

	// A released volume leaves the scrape.
	release()
	if values := scrapeVolume(t, "test/held"); len(values) != 0 {
		t.Fatalf("released volume scraped: %v", values)
	}
}

// flowVolume is a held volume that reports flows.
type flowVolume struct {
	heldVolume
}

// GetFlows returns one write path's flow and one key family's flow.
func (v *flowVolume) GetFlows() *volume.VolumeFlows {
	return &volume.VolumeFlows{
		VolumeID: "test/held",
		Puts:     []volume.PutFlow{{Path: "fill", Blocks: 3, Bytes: 4096, Existing: 5}},
		Values:   []volume.ValueFlow{{Family: "graph", Written: 1 << 20, Freed: 1 << 19}},
	}
}

// TestVolumeFlowsScrape checks that a scrape reports a registered volume's
// flows as counters labeled by write path and key family.
func TestVolumeFlowsScrape(t *testing.T) {
	// Scrape a volume that reports flows.
	release := volume.RegisterHealth(&flowVolume{heldVolume{start: time.Now()}})
	defer release()
	values := scrapeVolume(t, "test/held")

	// Each flow appears under its label.
	want := map[string]float64{
		"spacewave_volume_put_blocks_total{path=fill}":             3,
		"spacewave_volume_put_bytes_total{path=fill}":              4096,
		"spacewave_volume_put_existing_blocks_total{path=fill}":    5,
		"spacewave_volume_value_written_bytes_total{family=graph}": 1 << 20,
		"spacewave_volume_value_freed_bytes_total{family=graph}":   1 << 19,
	}
	for key, v := range want {
		if values[key] != v {
			t.Fatalf("%s = %v; want %v in %v", key, values[key], v, values)
		}
	}
}

// scrapeVolume gathers the process metrics and returns the gauges and
// counters of volume id, keyed by name and any phase, path or family label.
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
			for _, name := range []string{"phase", "path", "family"} {
				if value, ok := labels[name]; ok {
					key += "{" + name + "=" + value + "}"
				}
			}
			values[key] = m.GetGauge().GetValue() + m.GetCounter().GetValue()
		}
	}
	return values
}

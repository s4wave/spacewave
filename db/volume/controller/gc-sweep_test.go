package volume_controller

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/volume"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

// stubRefGraph is an empty reference graph.
type stubRefGraph struct{}

func (stubRefGraph) AddRef(context.Context, string, string) error { return nil }
func (stubRefGraph) RemoveRef(context.Context, string, string) error {
	return nil
}

func (stubRefGraph) ApplyRefBatch(context.Context, []block_gc.RefEdge, []block_gc.RefEdge) error {
	return nil
}

func (stubRefGraph) RemoveNodeRefs(context.Context, string, bool) ([]string, error) {
	return nil, nil
}

func (stubRefGraph) HasIncomingRefs(context.Context, string) (bool, error) {
	return false, nil
}

func (stubRefGraph) HasIncomingRefsExcluding(context.Context, string, ...string) (bool, error) {
	return false, nil
}

func (stubRefGraph) GetOutgoingRefs(context.Context, string) ([]string, error) {
	return nil, nil
}

func (stubRefGraph) GetIncomingRefs(context.Context, string) ([]string, error) {
	return nil, nil
}

func (stubRefGraph) GetUnreferencedNodes(context.Context) ([]string, error) {
	return nil, nil
}

func (stubRefGraph) AddBlockRef(context.Context, *block.BlockRef, *block.BlockRef) error {
	return nil
}

func (stubRefGraph) AddObjectRoot(context.Context, string, *block.BlockRef) error {
	return nil
}

func (stubRefGraph) RemoveObjectRoot(context.Context, string, *block.BlockRef) error {
	return nil
}
func (stubRefGraph) Close() error { return nil }

// maintainedVolume is a volume whose storage reclaims removed blocks in a
// separate step.
type maintainedVolume struct {
	*common_kvtx.Volume
	// maintained is called by each Maintenance.
	maintained func()
}

// Maintenance calls maintained.
func (v maintainedVolume) Maintenance(context.Context) error {
	v.maintained()
	return nil
}

// TestRunGCSweepMaintainsStorage checks that a collection that sweeps a node
// lets the volume's storage reclaim the space.
func TestRunGCSweepMaintainsStorage(t *testing.T) {
	// Create an in-memory volume holding one unreferenced node.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	vol := newTestVolume(t)
	err := vol.GetRefGraph().ApplyRefBatch(ctx, []block_gc.RefEdge{{Subject: block_gc.NodeUnreferenced, Object: "orphan"}}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Run collection until the sweep reaches storage maintenance.
	c := newTestController(maintainedVolume{Volume: vol, maintained: cancel}, ctx)
	if err := c.runGCSweep(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("runGCSweep error = %v, want context.Canceled", err)
	}
	refs, err := vol.GetRefGraph().GetIncomingRefs(t.Context(), "orphan")
	if err != nil || len(refs) != 0 {
		t.Fatalf("orphan refs %v err %v, want swept", refs, err)
	}
}

// delegatingVolume is a volume that views storage collected elsewhere.
type delegatingVolume struct {
	*common_kvtx.Volume
}

// DelegatesGC marks the volume as a view of collected storage.
func (delegatingVolume) DelegatesGC() {}

// TestRunGCSweepSkipsDelegatingVolume checks a controller whose volume views
// storage collected elsewhere returns without collecting.
func TestRunGCSweepSkipsDelegatingVolume(t *testing.T) {
	// The sweep ends at once instead of collecting on its short interval.
	ctx := t.Context()
	c := newTestController(delegatingVolume{newTestVolume(t)}, ctx)
	if err := c.runGCSweep(ctx); err != nil {
		t.Fatalf("runGCSweep error = %v, want nil", err)
	}
}

// newTestVolume opens an in-memory volume closed when the test ends.
func newTestVolume(t *testing.T) *common_kvtx.Volume {
	// Open the volume on a fresh in-memory store.
	t.Helper()
	vol, err := common_kvtx.NewVolume(t.Context(), "test-volume", store_kvkey.NewDefaultKVKey(), store_kvtx_inmem.NewStore(), nil, false, false, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = vol.Close() })
	return vol
}

// newTestController returns a controller collecting vol every millisecond.
func newTestController(vol volume.Volume, ctx context.Context) *Controller {
	c := &Controller{
		le:     logrus.NewEntry(logrus.New()),
		config: &Config{GcIntervalDur: "1ms"},
		volume: ccontainer.NewCContainer[*volumeCtxPair](nil),
	}
	c.volume.SetValue(&volumeCtxPair{vol: vol, ctx: ctx})
	return c
}

// TestRunGCSweepResumesSchedule checks that a block pass a restart left
// overdue waits out the quiet start, then runs before a full mark interval and
// saves the next one to the volume.
func TestRunGCSweepResumesSchedule(t *testing.T) {
	// Save a schedule an hour overdue, as a restart mid-pass leaves it.
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	vol := newTestVolume(t)
	start := time.Now()
	if err := writeGcSchedule(ctx, vol, start.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}

	// Collect every 100ms, so an unrestored schedule would hold the pass
	// for 1.5s.
	const interval = 100 * time.Millisecond
	c := newTestController(vol, ctx)
	c.config.GcIntervalDur = interval.String()
	done := make(chan error, 1)
	go func() { done <- c.runGCSweep(ctx) }()

	// Wait, short of that hold, for the pass to save its successor.
	deadline := time.After(markIntervals * interval * 2 / 3)
	var next time.Time
	for !next.After(start) {
		select {
		case err := <-done:
			t.Fatalf("runGCSweep returned early: %v", err)
		case <-deadline:
			t.Fatalf("no block pass saved a schedule, last %v", next)
		case <-time.After(time.Millisecond):
		}
		var err error
		if next, err = readGcSchedule(ctx, vol); err != nil {
			t.Fatal(err)
		}
	}
	cancel()
	<-done

	// The pass waited out the quiet start.
	if ran := time.Since(start); ran < quietIntervals*interval {
		t.Fatalf("block pass ran %v after open, want at least %v", ran, quietIntervals*interval)
	}

	// The saved pass is due within the longest gap after the pass.
	if limit := time.Now().Add(8 * markIntervals * interval); next.After(limit) {
		t.Fatalf("next pass %v after the pass, want at most %v", time.Until(next), 8*markIntervals*interval)
	}
}

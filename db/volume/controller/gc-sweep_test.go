package volume_controller

import (
	"context"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	"github.com/s4wave/spacewave/db/volume"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

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

package volume_controller

import (
	"context"
	"errors"
	"testing"
	"time"

	bbolt_errors "github.com/aperturerobotics/bbolt/errors"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	volume "github.com/s4wave/spacewave/db/volume"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

type stubCollectorGraph struct {
	unreferencedErr error
}

func (stubCollectorGraph) AddRef(context.Context, string, string) error { return nil }
func (stubCollectorGraph) RemoveRef(context.Context, string, string) error {
	return nil
}

func (stubCollectorGraph) ApplyRefBatch(context.Context, []block_gc.RefEdge, []block_gc.RefEdge) error {
	return nil
}

func (stubCollectorGraph) RemoveNodeRefs(context.Context, string, bool) ([]string, error) {
	return nil, nil
}

func (stubCollectorGraph) HasIncomingRefs(context.Context, string) (bool, error) {
	return false, nil
}

func (stubCollectorGraph) HasIncomingRefsExcluding(context.Context, string, ...string) (bool, error) {
	return false, nil
}

func (stubCollectorGraph) GetOutgoingRefs(context.Context, string) ([]string, error) {
	return nil, nil
}

func (stubCollectorGraph) GetIncomingRefs(context.Context, string) ([]string, error) {
	return nil, nil
}

func (g stubCollectorGraph) GetUnreferencedNodes(context.Context) ([]string, error) {
	if g.unreferencedErr != nil {
		return nil, g.unreferencedErr
	}
	return nil, nil
}

func (stubCollectorGraph) AddBlockRef(context.Context, *block.BlockRef, *block.BlockRef) error {
	return nil
}

func (stubCollectorGraph) AddObjectRoot(context.Context, string, *block.BlockRef) error {
	return nil
}

func (stubCollectorGraph) RemoveObjectRoot(context.Context, string, *block.BlockRef) error {
	return nil
}
func (stubCollectorGraph) Close() error { return nil }
func (stubCollectorGraph) IterateNodes(context.Context) ([]string, error) {
	return nil, nil
}

func (stubCollectorGraph) GetRootNodes(context.Context) ([]string, error) {
	return nil, nil
}
func (stubCollectorGraph) RemoveRoot(context.Context, string) error { return nil }
func (stubCollectorGraph) RemoveNode(context.Context, string) error { return nil }

func TestRunGCSweepUsesManagerHooks(t *testing.T) {
	// Give the collection manager a cancellable test lifetime.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Create an in-memory volume for collection manager hooks.
	le := logrus.NewEntry(logrus.New())
	vol, err := common_kvtx.NewVolume(
		ctx,
		"test-volume",
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		nil,
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()

	// Observe WAL replay and cancel collection after manager maintenance.
	replayed := make(chan struct{}, 3)
	maintained := make(chan struct{}, 1)
	vol.SetGCManagerHooks(block_gc.ManagerHooks{
		Graph: stubCollectorGraph{},
		ReplayWAL: func(context.Context, block_gc.CollectorGraph) (int, error) {
			select {
			case replayed <- struct{}{}:
			default:
			}
			return 0, nil
		},
		AcquireSTW: func() (func(), error) {
			return func() {}, nil
		},
		Maintenance: func(context.Context) error {
			// Record that the collection manager reached volume maintenance.
			select {
			case maintained <- struct{}{}:
			default:
			}

			// End the manager lifetime after recording maintenance.
			cancel()
			return nil
		},
	})

	// Publish the test volume to the collection controller.
	c := &Controller{
		le:     le,
		config: &Config{GcIntervalDur: "1ms"},
		volume: ccontainer.NewCContainer[*volumeCtxPair](nil),
	}
	c.volume.SetValue(&volumeCtxPair{vol: vol, ctx: ctx})

	// Run volume collection and verify maintenance cancels the manager.
	err = c.runGCSweep(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runGCSweep error = %v, want context.Canceled", err)
	}

	// Verify the collection manager replayed its WAL on startup.
	select {
	case <-replayed:
	default:
		t.Fatal("expected manager startup replay to run")
	}

	// Verify the collection manager ran volume maintenance.
	select {
	case <-maintained:
	default:
		t.Fatal("expected manager maintenance to run")
	}
}

func TestRunGCSweepReturnsLockFileChanged(t *testing.T) {
	// Bound the test lifetime if collection fails to return its storage error.
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	// Create an in-memory volume for the failing reference graph.
	vol, err := common_kvtx.NewVolume(
		ctx,
		"test-volume",
		store_kvkey.NewDefaultKVKey(),
		store_kvtx_inmem.NewStore(),
		nil,
		false,
		false,
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()

	// Expose a reference graph whose collection reports a changed lock file.
	wrapped := &gcSweepTestVolume{
		Volume: vol,
		rg:     stubCollectorGraph{unreferencedErr: bbolt_errors.ErrLockFileChanged},
	}
	c := &Controller{
		le:     logrus.NewEntry(logrus.New()),
		config: &Config{GcIntervalDur: "1ms"},
		volume: ccontainer.NewCContainer[*volumeCtxPair](nil),
	}
	c.volume.SetValue(&volumeCtxPair{vol: wrapped, ctx: ctx})

	// Verify collection propagates the changed lock file error.
	err = c.runGCSweep(ctx)
	if !errors.Is(err, bbolt_errors.ErrLockFileChanged) {
		t.Fatalf("runGCSweep error = %v, want %v", err, bbolt_errors.ErrLockFileChanged)
	}
}

type gcSweepTestVolume struct {
	*common_kvtx.Volume
	rg block_gc.RefGraphOps
}

func (v *gcSweepTestVolume) GetRefGraph() block_gc.RefGraphOps {
	return v.rg
}

// _ is a type assertion
var (
	_ interface {
		SetGCManagerHooks(block_gc.ManagerHooks)
		GetGCManagerHooks() (block_gc.ManagerHooks, bool)
	} = (*common_kvtx.Volume)(nil)
	_ volume.Volume = (*gcSweepTestVolume)(nil)
)

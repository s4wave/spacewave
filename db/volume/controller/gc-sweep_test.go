package volume_controller

import (
	"context"
	"errors"
	"testing"

	"github.com/aperturerobotics/util/ccontainer"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	common_kvtx "github.com/s4wave/spacewave/db/volume/common/kvtx"
	"github.com/sirupsen/logrus"
)

type stubCollectorGraph struct{}

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

func (stubCollectorGraph) GetUnreferencedNodes(context.Context) ([]string, error) {
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

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

	replayed := make(chan struct{}, 1)
	vol.SetGCManagerHooks(block_gc.ManagerHooks{
		Graph: stubCollectorGraph{},
		ReplayWAL: func(context.Context, block_gc.CollectorGraph) (int, error) {
			select {
			case replayed <- struct{}{}:
			default:
			}
			cancel()
			return 0, nil
		},
		AcquireSTW: func() (func(), error) {
			return func() {}, nil
		},
	})

	c := &Controller{
		le:     le,
		config: &Config{GcIntervalDur: "1ms"},
		volume: ccontainer.NewCContainer[*volumeCtxPair](nil),
	}
	c.volume.SetValue(&volumeCtxPair{vol: vol, ctx: ctx})

	err = c.runGCSweep(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("runGCSweep error = %v, want context.Canceled", err)
	}

	select {
	case <-replayed:
	default:
		t.Fatal("expected manager startup replay to run")
	}
}

var _ interface {
	SetGCManagerHooks(block_gc.ManagerHooks)
	GetGCManagerHooks() (block_gc.ManagerHooks, bool)
} = (*common_kvtx.Volume)(nil)

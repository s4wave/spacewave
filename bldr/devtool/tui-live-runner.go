//go:build !js

package devtool

import (
	"context"
	"sync"

	"github.com/aperturerobotics/util/ccontainer"
	tui "github.com/grindlemire/go-tui"
	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

const devtoolTUIStatusBufferSize = 16

// BldrDevtoolTUIRunner runs the native go-tui dashboard.
type BldrDevtoolTUIRunner struct{}

// NewDevtoolTUIRunner creates the native dashboard runner.
func NewDevtoolTUIRunner() *BldrDevtoolTUIRunner {
	return &BldrDevtoolTUIRunner{}
}

// Start starts the native dashboard runner.
func (r *BldrDevtoolTUIRunner) Start(
	ctx context.Context,
	producer *devtool_status.BldrDevtoolStatusProducer,
) (func(), error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithCancel(ctx)
	statusCh := startDevtoolTUIStatusStream(runCtx, producer)
	initial := devtoolTUIInitialStatus(producer)
	dashboard := NewBldrDevtoolTUIDashboard(initial, statusCh)
	app, err := tui.NewApp(tui.WithRootComponent(dashboard))
	if err != nil {
		cancel()
		return nil, err
	}

	var stopOnce sync.Once
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = app.Run()
		cancel()
	}()

	return func() {
		stopOnce.Do(func() {
			cancel()
			app.Stop()
			<-done
		})
	}, nil
}

func devtoolTUIInitialStatus(
	producer *devtool_status.BldrDevtoolStatusProducer,
) *devtool_status.BldrDevtoolStatus {
	if producer == nil {
		return devtool_status.EmptyBldrDevtoolStatus()
	}
	return producer.GetStatus()
}

func startDevtoolTUIStatusStream(
	ctx context.Context,
	producer *devtool_status.BldrDevtoolStatusProducer,
) <-chan *devtool_status.BldrDevtoolStatus {
	statusCh := make(chan *devtool_status.BldrDevtoolStatus, devtoolTUIStatusBufferSize)
	go func() {
		defer close(statusCh)
		if producer == nil {
			return
		}
		current := producer.GetStatus()
		if !sendDevtoolTUIStatus(ctx, statusCh, current) {
			return
		}
		_ = ccontainer.WatchChanges(
			ctx,
			current,
			producer.GetStatusCtr(),
			func(snapshot *devtool_status.BldrDevtoolStatus) error {
				if !sendDevtoolTUIStatus(ctx, statusCh, snapshot) {
					return ctx.Err()
				}
				return nil
			},
			nil,
		)
	}()
	return statusCh
}

func sendDevtoolTUIStatus(
	ctx context.Context,
	statusCh chan<- *devtool_status.BldrDevtoolStatus,
	snapshot *devtool_status.BldrDevtoolStatus,
) bool {
	select {
	case statusCh <- normalizeDevtoolTUIStatus(snapshot):
		return true
	case <-ctx.Done():
		return false
	}
}

// _ is a type assertion
var _ DevtoolTUIRunner = ((*BldrDevtoolTUIRunner)(nil))

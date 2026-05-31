//go:build !js

package devtool

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"sync"

	"github.com/aperturerobotics/util/ccontainer"
	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
	"github.com/s4wave/spacewave/bldr/util/termui"
)

const devtoolTUIStatusBufferSize = 16

// BldrDevtoolTUIRunner runs the native terminal dashboard.
type BldrDevtoolTUIRunner struct {
	openBrowserURL func(string) error
}

// NewDevtoolTUIRunner creates the native dashboard runner.
func NewDevtoolTUIRunner() *BldrDevtoolTUIRunner {
	return &BldrDevtoolTUIRunner{
		openBrowserURL: openDevtoolTUIBrowserURL,
	}
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

	var stopOnce sync.Once
	done := make(chan struct{})
	go func() {
		defer close(done)
		_ = termui.RunWithKeys(
			runCtx,
			os.Stdin,
			os.Stdout,
			initial,
			statusCh,
			BuildDevtoolTUIDashboard,
			r.handleKey,
		)
		cancel()
	}()

	return func() {
		stopOnce.Do(func() {
			cancel()
			<-done
		})
	}, nil
}

func (r *BldrDevtoolTUIRunner) handleKey(snapshot *devtool_status.BldrDevtoolStatus, key byte) {
	if key != 'o' && key != 'O' {
		return
	}
	browserURL := devtoolTUIBrowserURL(snapshot)
	if browserURL == "" {
		return
	}
	openBrowserURL := r.openBrowserURL
	if openBrowserURL == nil {
		openBrowserURL = openDevtoolTUIBrowserURL
	}
	_ = openBrowserURL(browserURL)
}

func openDevtoolTUIBrowserURL(browserURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", browserURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", browserURL)
	default:
		cmd = exec.Command("xdg-open", browserURL)
	}
	return cmd.Start()
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

//go:build !js

package devtool

import (
	"context"
	"strings"
	"testing"
	"time"

	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

func TestDevtoolArgsDefaultTUIRunnerIsNativeDashboard(t *testing.T) {
	args := NewDevtoolArgs()
	if args.TUIRunner == nil {
		t.Fatal("expected default tui runner")
	}
	if _, ok := args.TUIRunner.(*BldrDevtoolTUIRunner); !ok {
		t.Fatalf("unexpected default tui runner: %T", args.TUIRunner)
	}
}

func TestBldrDevtoolTUIDashboardWatchesStatusChannel(t *testing.T) {
	initial := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:  "build",
			State: devtool_status.BldrDevtoolCommandStateStarting,
		},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	statusCh := make(chan *devtool_status.BldrDevtoolStatus)
	dashboard := NewBldrDevtoolTUIDashboard(initial, statusCh)
	watchers := dashboard.Watchers()
	if len(watchers) != 1 {
		t.Fatalf("expected status watcher, got %d", len(watchers))
	}
	eventQueue := make(chan func(), 1)
	stopCh := make(chan struct{})
	defer close(stopCh)
	watchers[0].Start(eventQueue, stopCh)

	next := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:    "start web",
			State:   devtool_status.BldrDevtoolCommandStateRunning,
			Summary: "web runtime active",
		},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	statusCh <- next
	readDevtoolTUIEvent(t, eventQueue)()
	text := collectDevtoolTUIText(dashboard.Render(nil))
	if !strings.Contains(text, "web runtime active") {
		t.Fatalf("expected updated dashboard text, got:\n%s", text)
	}
}

func TestStartDevtoolTUIStatusStreamPublishesSnapshotChanges(t *testing.T) {
	initial := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:  "build",
			State: devtool_status.BldrDevtoolCommandStateStarting,
		},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	producer := devtool_status.NewBldrDevtoolStatusProducer(initial)
	ctx := t.Context()
	statusCh := startDevtoolTUIStatusStream(ctx, producer)

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	got := readDevtoolTUIStatus(t, waitCtx, statusCh)
	if got.GetCommand().Name != "build" {
		t.Fatalf("expected initial status, got %+v", got.GetCommand())
	}

	next := devtool_status.NewBldrDevtoolStatus(
		devtool_status.BldrDevtoolCommandStatus{
			Name:    "start desktop",
			State:   devtool_status.BldrDevtoolCommandStateRunning,
			Summary: "desktop runtime active",
		},
		nil,
		nil,
		nil,
		nil,
		nil,
	)
	producer.SetStatus(next)
	got = readDevtoolTUIStatus(t, waitCtx, statusCh)
	if got.GetCommand().Summary != "desktop runtime active" {
		t.Fatalf("expected changed status, got %+v", got.GetCommand())
	}
}

func TestStartDevtoolTUIStatusStreamClosesOnCancel(t *testing.T) {
	producer := devtool_status.NewBldrDevtoolStatusProducer(nil)
	ctx, cancel := context.WithCancel(context.Background())
	statusCh := startDevtoolTUIStatusStream(ctx, producer)

	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()
	readDevtoolTUIStatus(t, waitCtx, statusCh)

	cancel()
	select {
	case _, ok := <-statusCh:
		if ok {
			t.Fatal("expected status channel to close")
		}
	case <-waitCtx.Done():
		t.Fatal(waitCtx.Err())
	}
}

func readDevtoolTUIStatus(
	t *testing.T,
	ctx context.Context,
	statusCh <-chan *devtool_status.BldrDevtoolStatus,
) *devtool_status.BldrDevtoolStatus {
	t.Helper()
	select {
	case snapshot, ok := <-statusCh:
		if !ok {
			t.Fatal("status channel closed")
		}
		return snapshot
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	return nil
}

func readDevtoolTUIEvent(
	t *testing.T,
	eventQueue <-chan func(),
) func() {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	select {
	case fn := <-eventQueue:
		return fn
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	return nil
}

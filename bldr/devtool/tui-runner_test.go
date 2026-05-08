//go:build !js

package devtool

import (
	"context"
	"testing"

	"github.com/pkg/errors"
	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

func TestDevtoolArgsStartTUIRunnerSkipsPlainMode(t *testing.T) {
	args := NewDevtoolArgs()
	args.terminalDetector = func() bool { return false }
	called := false
	args.TUIRunner = DevtoolTUIRunnerFunc(func(
		context.Context,
		*devtool_status.BldrDevtoolStatusProducer,
	) (func(), error) {
		called = true
		return nil, nil
	})

	stop, err := args.startTUIRunner(context.Background(), devtool_status.NewBldrDevtoolStatusProducer(nil))
	if err != nil {
		t.Fatal(err)
	}
	stop()
	if called {
		t.Fatal("expected plain mode to skip tui runner")
	}
}

func TestDevtoolArgsStartTUIRunnerSkipsMissingRunner(t *testing.T) {
	args := NewDevtoolArgs()
	args.terminalDetector = func() bool { return true }
	args.TUIRunner = nil

	stop, err := args.startTUIRunner(context.Background(), devtool_status.NewBldrDevtoolStatusProducer(nil))
	if err != nil {
		t.Fatal(err)
	}
	stop()
}

func TestDevtoolArgsStartTUIRunnerStopsActiveRunner(t *testing.T) {
	args := NewDevtoolArgs()
	args.terminalDetector = func() bool { return true }
	called := false
	stopped := false
	args.TUIRunner = DevtoolTUIRunnerFunc(func(
		context.Context,
		*devtool_status.BldrDevtoolStatusProducer,
	) (func(), error) {
		called = true
		return func() {
			stopped = true
		}, nil
	})

	stop, err := args.startTUIRunner(context.Background(), devtool_status.NewBldrDevtoolStatusProducer(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("expected tui runner to start")
	}
	stop()
	if !stopped {
		t.Fatal("expected tui runner stop callback")
	}
}

func TestDevtoolArgsStartTUIRunnerPropagatesErrors(t *testing.T) {
	args := NewDevtoolArgs()
	args.terminalDetector = func() bool { return true }
	want := errors.New("runner failed")
	args.TUIRunner = DevtoolTUIRunnerFunc(func(
		context.Context,
		*devtool_status.BldrDevtoolStatusProducer,
	) (func(), error) {
		return nil, want
	})

	_, err := args.startTUIRunner(context.Background(), devtool_status.NewBldrDevtoolStatusProducer(nil))
	if !errors.Is(err, want) {
		t.Fatalf("expected runner error, got %v", err)
	}
}

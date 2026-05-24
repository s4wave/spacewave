//go:build !js

package devtool

import (
	"context"
	"testing"

	"github.com/aperturerobotics/util/enabled"
	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

func TestDevtoolBusCommandStatusHelpers(t *testing.T) {
	d := &DevtoolBus{statusProducer: devtool_status.NewBldrDevtoolStatusProducer(nil)}

	d.setCommandStarting("build", "initializing")
	command := d.GetStatusProducer().GetStatus().GetCommand()
	if command.Name != "build" || command.State != devtool_status.BldrDevtoolCommandStateStarting {
		t.Fatalf("unexpected starting command: %+v", command)
	}

	d.setCommandRunning("build", "running")
	command = d.GetStatusProducer().GetStatus().GetCommand()
	if command.Summary != "running" || command.State != devtool_status.BldrDevtoolCommandStateRunning {
		t.Fatalf("unexpected running command: %+v", command)
	}

	d.setCommandStartingWithLogFile("build", "initializing", "/tmp/bldr.log")
	command = d.GetStatusProducer().GetStatus().GetCommand()
	if command.LogFile != "/tmp/bldr.log" {
		t.Fatalf("expected command log file, got %+v", command)
	}

	d.finishCommand(context.Background(), "build", nil)
	command = d.GetStatusProducer().GetStatus().GetCommand()
	if command.State != devtool_status.BldrDevtoolCommandStateDone || command.Error != "" {
		t.Fatalf("unexpected done command: %+v", command)
	}
}

func TestDevtoolBusFinishCommandReportsErrorAndCancel(t *testing.T) {
	d := &DevtoolBus{statusProducer: devtool_status.NewBldrDevtoolStatusProducer(nil)}

	d.finishCommand(context.Background(), "build", context.DeadlineExceeded)
	command := d.GetStatusProducer().GetStatus().GetCommand()
	if command.State != devtool_status.BldrDevtoolCommandStateError || command.Error == "" {
		t.Fatalf("unexpected error command: %+v", command)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d.finishCommand(ctx, "build", nil)
	command = d.GetStatusProducer().GetStatus().GetCommand()
	if command.State != devtool_status.BldrDevtoolCommandStateCanceled || command.Error == "" {
		t.Fatalf("unexpected canceled command: %+v", command)
	}
}

func TestDevtoolBusFinishCommandStopsTUIAfterTerminalStatus(t *testing.T) {
	d := &DevtoolBus{statusProducer: devtool_status.NewBldrDevtoolStatusProducer(nil)}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stopped := false
	d.finishCommandThenStopTUI(ctx, "start web", "/tmp/bldr.log", nil, func() {
		command := d.GetStatusProducer().GetStatus().GetCommand()
		if command.State != devtool_status.BldrDevtoolCommandStateCanceled {
			t.Fatalf("expected canceled status before TUI stop, got %+v", command)
		}
		if command.LogFile != "/tmp/bldr.log" {
			t.Fatalf("expected log file before TUI stop, got %+v", command)
		}
		stopped = true
	})

	if !stopped {
		t.Fatal("expected TUI stop callback")
	}
}

func TestBuildCommandSummaryIncludesFiniteBuildInputs(t *testing.T) {
	summary := buildCommandSummary("desktop", "release", "devtool", "desktop/darwin/arm64")
	if summary != "building targets desktop build-type=release remote=devtool targets=desktop/darwin/arm64" {
		t.Fatalf("unexpected build summary: %q", summary)
	}
}

func TestDevtoolArgsBuildPolicyOverride(t *testing.T) {
	args := NewDevtoolArgs()
	args.JSMinification = "disable"
	args.JSSourcemaps = "enable"

	policy, err := args.BuildPolicyOverride()
	if err != nil {
		t.Fatal(err)
	}
	if policy.GetJsMinification() != enabled.Enabled_DISABLE {
		t.Fatalf("js minification: got %s, want DISABLE", policy.GetJsMinification())
	}
	if policy.GetJsSourcemaps() != enabled.Enabled_ENABLE {
		t.Fatalf("js sourcemaps: got %s, want ENABLE", policy.GetJsSourcemaps())
	}
}

func TestDevtoolArgsBuildPolicyOverrideRejectsInvalidValue(t *testing.T) {
	args := NewDevtoolArgs()
	args.JSMinification = "readable"

	if _, err := args.BuildPolicyOverride(); err == nil {
		t.Fatal("expected invalid policy value error")
	}
}

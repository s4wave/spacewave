//go:build !js

package devtool

import (
	"context"
	"testing"

	"github.com/aperturerobotics/util/enabled"
	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

func TestDevtoolBusCommandStatusHelpers(t *testing.T) {
	// Construct a Devtool bus with command status storage.
	d := &DevtoolBus{statusProducer: devtool_status.NewBldrDevtoolStatusProducer(nil)}

	// Publish the starting build command status.
	d.setCommandStartingWithLogFile("build", "initializing", "")
	command := d.GetStatusProducer().GetStatus().GetCommand()

	// Verify the build command name and starting state.
	if command.Name != "build" || command.State != devtool_status.BldrDevtoolCommandStateStarting {
		t.Fatalf("unexpected starting command: %+v", command)
	}

	// Publish the running build command status.
	d.setCommandRunningWithLogFile("build", "running", "")
	command = d.GetStatusProducer().GetStatus().GetCommand()

	// Verify the build command summary and running state.
	if command.Summary != "running" || command.State != devtool_status.BldrDevtoolCommandStateRunning {
		t.Fatalf("unexpected running command: %+v", command)
	}

	// Publish the build command log file path.
	d.setCommandStartingWithLogFile("build", "initializing", "/tmp/bldr.log")
	command = d.GetStatusProducer().GetStatus().GetCommand()

	// Verify the build command retains its log file path.
	if command.LogFile != "/tmp/bldr.log" {
		t.Fatalf("expected command log file, got %+v", command)
	}

	// Finish the build command successfully.
	d.finishCommandWithLogFile(context.Background(), "build", "", nil)
	command = d.GetStatusProducer().GetStatus().GetCommand()

	// Verify the build command is done without an error.
	if command.State != devtool_status.BldrDevtoolCommandStateDone || command.Error != "" {
		t.Fatalf("unexpected done command: %+v", command)
	}
}

func TestDevtoolBusFinishCommandReportsErrorAndCancel(t *testing.T) {
	// Construct a Devtool bus with command status storage.
	d := &DevtoolBus{statusProducer: devtool_status.NewBldrDevtoolStatusProducer(nil)}

	// Finish the build command with a deadline error.
	d.finishCommandWithLogFile(context.Background(), "build", "", context.DeadlineExceeded)
	command := d.GetStatusProducer().GetStatus().GetCommand()

	// Verify the build command reports its error state.
	if command.State != devtool_status.BldrDevtoolCommandStateError || command.Error == "" {
		t.Fatalf("unexpected error command: %+v", command)
	}

	// Finish the build command with an already canceled context.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	d.finishCommandWithLogFile(ctx, "build", "", nil)
	command = d.GetStatusProducer().GetStatus().GetCommand()

	// Verify the build command reports cancellation.
	if command.State != devtool_status.BldrDevtoolCommandStateCanceled || command.Error == "" {
		t.Fatalf("unexpected canceled command: %+v", command)
	}
}

func TestBuildCommandSummaryIncludesFiniteBuildInputs(t *testing.T) {
	summary := buildCommandSummary("desktop", "release", "devtool", "desktop/darwin/arm64")
	if summary != "building targets desktop build-type=release remote=devtool targets=desktop/darwin/arm64" {
		t.Fatalf("unexpected build summary: %q", summary)
	}
}

func TestDevtoolArgsBuildPolicyOverride(t *testing.T) {
	// Configure explicit build policy overrides.
	args := NewDevtoolArgs()
	args.JSMinification = "disable"
	args.JSSourcemaps = "enable"
	args.GoScriptCodeSplitting = "disable"

	// Parse the build policy overrides.
	policy, err := args.BuildPolicyOverride()
	if err != nil {
		t.Fatal(err)
	}

	// Verify the build policy preserves each configured override.
	if policy.GetJsMinification() != enabled.Enabled_DISABLE {
		t.Fatalf("js minification: got %s, want DISABLE", policy.GetJsMinification())
	}
	if policy.GetJsSourcemaps() != enabled.Enabled_ENABLE {
		t.Fatalf("js sourcemaps: got %s, want ENABLE", policy.GetJsSourcemaps())
	}
	if policy.GetGoscriptCodeSplitting() != enabled.Enabled_DISABLE {
		t.Fatalf("goscript code splitting: got %s, want DISABLE", policy.GetGoscriptCodeSplitting())
	}
}

func TestDevtoolArgsBuildPolicyOverrideRejectsInvalidValue(t *testing.T) {
	args := NewDevtoolArgs()
	args.JSMinification = "readable"

	if _, err := args.BuildPolicyOverride(); err == nil {
		t.Fatal("expected invalid policy value error")
	}
}

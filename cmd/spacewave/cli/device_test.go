//go:build !js

package spacewave_cli

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/aperturerobotics/cli"
)

func TestDeviceSetupDockerUsesGlobalStatePath(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "device-state")
	out := runDeviceCommand(t, []string{
		"--state-path", statePath,
		"device",
		"setup",
		"docker",
		"--label", "build-host",
	})

	assertContains(t, out, "Label")
	assertContains(t, out, "build-host")
	assertContains(t, out, "State Path")
	assertContains(t, out, statePath)
	assertContains(t, out, filepath.Join(statePath, socketName))
	assertContains(t, out, deviceDockerStatePath)
	assertContains(t, out, "SESSION_TYPE_DEVICE")
	assertContains(t, out, "WRITER")
	assertContains(t, out, "cli-mediated")
	assertContains(t, out, "not started")
	assertContains(t, out, "not generated")
}

func TestDeviceStatusUsesSocketOverride(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "device-state")
	socketPath := filepath.Join(t.TempDir(), "custom.sock")
	out := runDeviceCommand(t, []string{
		"--state-path", statePath,
		"device",
		"--socket-path", socketPath,
		"status",
	})

	assertContains(t, out, "State Path")
	assertContains(t, out, statePath)
	assertContains(t, out, "Socket")
	assertContains(t, out, socketPath)
	assertContains(t, out, "unconfigured")
	assertContains(t, out, "not linked")
}

func TestDeviceStatusAcceptsLeafStatePathFlag(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "leaf-state")
	out := runDeviceCommand(t, []string{
		"device",
		"status",
		"--state-path", statePath,
	})

	assertContains(t, out, "State Path")
	assertContains(t, out, statePath)
	assertContains(t, out, filepath.Join(statePath, socketName))
}

func TestDeviceSetupDockerAcceptsLeafSocketFlag(t *testing.T) {
	clearStatePathEnv(t)
	clearSocketPathEnv(t)

	statePath := filepath.Join(t.TempDir(), "leaf-state")
	socketPath := filepath.Join(t.TempDir(), "device.sock")
	out := runDeviceCommand(t, []string{
		"device",
		"setup",
		"docker",
		"--state-path", statePath,
		"--socket-path", socketPath,
		"--label", "build-host",
	})

	assertContains(t, out, "State Path")
	assertContains(t, out, statePath)
	assertContains(t, out, "Socket")
	assertContains(t, out, socketPath)
}

func TestDeviceCommandAliasRegistered(t *testing.T) {
	cmd := newDeviceCommand(nil)
	if !hasAlias(cmd.Aliases, "devices") {
		t.Fatal("devices alias missing from device command")
	}
}

func runDeviceCommand(t *testing.T, args []string) string {
	t.Helper()

	var rootStatePath string
	app := cli.NewApp()
	app.Name = "spacewave"
	app.HideVersion = true
	app.Flags = []cli.Flag{
		statePathFlag(&rootStatePath),
		&cli.StringFlag{
			Name:  "output",
			Value: "text",
		},
	}
	app.Commands = []*cli.Command{
		newDeviceCommand(nil),
	}

	out, err := captureStdout(t, func() error {
		return app.RunContext(context.Background(), append([]string{"spacewave"}, args...))
	})
	if err != nil {
		t.Fatalf("device command: %v", err)
	}
	return out
}

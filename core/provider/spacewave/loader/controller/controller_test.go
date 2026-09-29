package spacewave_loader_controller

import (
	"os"
	"path/filepath"
	"testing"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/sirupsen/logrus"
)

func TestExecuteToleratesHelperStartFailure(t *testing.T) {
	hostDir := t.TempDir()
	helperName := "broken-helper"
	if err := os.WriteFile(filepath.Join(hostDir, helperName), []byte("not executable"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(bldr_plugin.HostExecutableEnv, filepath.Join(hostDir, "host"))
	t.Setenv("SPACEWAVE_DATA_DIR", t.TempDir())

	ctrl := NewController(
		logrus.NewEntry(logrus.New()),
		nil,
		&Config{HelperBinaryName: helperName},
	)
	if err := ctrl.Execute(t.Context()); err != nil {
		t.Fatalf("Execute returned helper startup error: %v", err)
	}
}

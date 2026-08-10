//go:build !js

package devtool_test

import (
	"context"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/s4wave/spacewave/bldr/devtool"
	"github.com/sirupsen/logrus"
)

func TestCLIProcessSupervisorExternalConstructionAndTermination(t *testing.T) {
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	readyPath := filepath.Join(t.TempDir(), "ready")
	supervisor := devtool.NewCLIProcessSupervisor(
		context.Background(),
		logrus.NewEntry(logrus.New()),
		executable,
		[]string{"-test.run=TestCLIProcessSupervisorExternalHelper", "--", readyPath},
	)
	if err := supervisor.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	deadline := time.Now().Add(time.Second)
	for {
		if _, err := os.Stat(readyPath); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("helper did not become ready")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := supervisor.Terminate(); err != nil {
		t.Fatalf("terminate helper: %v", err)
	}
}

func TestCLIProcessSupervisorExternalHelper(t *testing.T) {
	args := externalHelperArgs()
	if len(args) != 1 {
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	if err := os.WriteFile(args[0], []byte("ready"), 0o644); err != nil {
		os.Exit(2)
	}
	<-signals
}

func externalHelperArgs() []string {
	for i, arg := range os.Args {
		if arg == "--" {
			return os.Args[i+1:]
		}
	}
	return nil
}

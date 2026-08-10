//go:build !js

package devtool

import (
	"context"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sync/atomic"
	"syscall"
	"testing"
	"time"

	"github.com/aperturerobotics/util/routine"
	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
	"github.com/sirupsen/logrus"
)

func TestDevtoolTUIRunnerWaitsForBrowserProcessOnCancel(t *testing.T) {
	readyPath := filepath.Join(t.TempDir(), "ready")
	exitedPath := filepath.Join(t.TempDir(), "exited")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	input, inputWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer input.Close()
	defer inputWriter.Close()
	output, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer output.Close()

	var launches atomic.Int32
	runner := &devtoolTUIRunner{
		input:          input,
		output:         output,
		openURL:        "http://127.0.0.1:5593",
		le:             logrus.NewEntry(logrus.New()),
		browserProcess: routine.NewRoutineContainer(),
		newBrowserProcess: func(ctx context.Context, le *logrus.Entry, _ string) *CLIProcessSupervisor {
			launches.Add(1)
			return NewCLIProcessSupervisor(ctx, le, executable, []string{
				"-test.run=TestDevtoolTUIBrowserProcessHelper",
				"--", readyPath, exitedPath,
			})
		},
	}
	_, complete := runner.start(
		context.Background(),
		devtool_status.NewBldrDevtoolStatusProducer(nil),
	)
	if _, err := io.WriteString(inputWriter, "oo"); err != nil {
		t.Fatal(err)
	}
	waitForCliSubprocessHelperReady(t, readyPath)
	if got := launches.Load(); got != 1 {
		t.Fatalf("browser launches = %d, want 1 while launch is active", got)
	}

	done := make(chan struct{})
	go func() {
		complete()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("browser process survived TUI cancellation")
	}
	if _, err := os.Stat(exitedPath); err != nil {
		t.Fatalf("browser helper was not waited through signal exit: %v", err)
	}
}

func TestDevtoolTUIBrowserProcessHelper(t *testing.T) {
	args := cliSubprocessSupervisorHelperArgs()
	if len(args) != 2 {
		return
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGTERM)
	markCliSubprocessHelperReady(args[0])
	<-signals
	if err := os.WriteFile(args[1], []byte("exited"), 0o644); err != nil {
		os.Exit(2)
	}
}

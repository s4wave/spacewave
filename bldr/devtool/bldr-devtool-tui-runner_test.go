//go:build !js

package devtool

import (
	"context"
	"os"
	"testing"
	"time"

	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
)

// TestDevtoolTUIRawCtrlCCancelsCommandContext drives the Ctrl-C key byte 0x03
// through the TUI key path. The TUI maps it to the active command cancel
// function, so the command context must cancel and stopTUI must join the TUI
// goroutine without hanging.
func TestDevtoolTUIRawCtrlCCancelsCommandContext(t *testing.T) {
	deadlineCtx, stopDeadline := context.WithTimeout(context.Background(), 10*time.Second)
	defer stopDeadline()
	producer := devtool_status.NewBldrDevtoolStatusProducer(nil)
	inputReader, inputWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = inputReader.Close() }()
	defer func() { _ = inputWriter.Close() }()
	outputReader, outputWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = outputReader.Close() }()
	defer func() { _ = outputWriter.Close() }()

	runner := &devtoolTUIRunner{input: inputReader, output: outputWriter}
	commandCtx, stopTUI := runner.start(deadlineCtx, producer)

	if _, err := inputWriter.Write([]byte{3}); err != nil {
		t.Fatal(err)
	}

	select {
	case <-commandCtx.Done():
	case <-deadlineCtx.Done():
		t.Fatalf("raw Ctrl-C did not cancel the active command context: %v", deadlineCtx.Err())
	}

	stopped := make(chan struct{})
	go func() {
		stopTUI()
		close(stopped)
	}()
	select {
	case <-stopped:
	case <-deadlineCtx.Done():
		t.Fatal("stopTUI did not join the TUI goroutine")
	}
}

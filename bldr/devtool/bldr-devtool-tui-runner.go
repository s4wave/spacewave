//go:build !js

package devtool

import (
	"context"
	"os"
	"os/exec"
	"runtime"

	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
	"github.com/s4wave/spacewave/bldr/util/logfile"
	"github.com/s4wave/spacewave/bldr/util/termui"
	"golang.org/x/term"
)

// devtoolTUIRunner runs the interactive TUI over the devtool status feed.
type devtoolTUIRunner struct {
	input   *os.File
	output  *os.File
	openURL string
}

// startDevtoolTUI starts the interactive TUI when the environment supports
// one, returning a context that is canceled when the TUI exits.
func (a *DevtoolArgs) startDevtoolTUI(
	ctx context.Context,
	producer *devtool_status.BldrDevtoolStatusProducer,
	openURL string,
) (context.Context, func()) {
	if producer == nil || !a.shouldRunTUI(os.Stdin, os.Stderr) {
		return ctx, func() {}
	}
	runner := &devtoolTUIRunner{
		input:   os.Stdin,
		output:  os.Stderr,
		openURL: openURL,
	}
	logfile.DiscardConsoleOutput(a.Logger.Logger)
	return runner.start(ctx, producer)
}

// start runs the TUI loop until the context is canceled or the user quits.
func (r *devtoolTUIRunner) start(
	ctx context.Context,
	producer *devtool_status.BldrDevtoolStatusProducer,
) (context.Context, func()) {
	uiCtx, cancel := context.WithCancel(ctx)
	updates := make(chan *devtool_status.BldrDevtoolStatus, 16)
	done := make(chan struct{}, 1)

	go func() {
		defer close(updates)
		current := producer.GetStatus()
		for {
			next, err := producer.GetStatusCtr().WaitValueChange(uiCtx, current, nil)
			if err != nil {
				return
			}
			current = next
			select {
			case updates <- next:
			case <-uiCtx.Done():
				return
			}
			if next.GetCommand().IsTerminal() {
				return
			}
		}
	}()

	go func() {
		defer func() { done <- struct{}{} }()
		_ = termui.RunWithKeys(
			uiCtx,
			r.input,
			r.output,
			producer.GetStatus(),
			updates,
			r.render,
			r.handleKey(cancel),
		)
	}()

	return uiCtx, func() {
		if !producer.GetStatus().GetCommand().IsTerminal() {
			cancel()
		}
		<-done
	}
}

// render renders the dashboard snapshot at the terminal width.
func (r *devtoolTUIRunner) render(snapshot *devtool_status.BldrDevtoolStatus) string {
	width := 100
	if r.output != nil {
		if w, _, err := term.GetSize(int(r.output.Fd())); err == nil && w > 0 {
			width = w
		}
	}
	return renderDevtoolTUIDashboard(snapshot, r.openURL, width, tuiColorEnabled())
}

// tuiColorEnabled reports whether ANSI styling should be applied. The devtool
// only runs the TUI on a terminal, so color is on unless NO_COLOR is set.
func tuiColorEnabled() bool {
	return os.Getenv("NO_COLOR") == ""
}

// handleKey maps quit and open-browser keys to their actions.
func (r *devtoolTUIRunner) handleKey(cancel context.CancelFunc) termui.KeyHandler[*devtool_status.BldrDevtoolStatus] {
	return func(_ *devtool_status.BldrDevtoolStatus, key byte) {
		switch key {
		case 3:
			cancel()
		case 'o', 'O':
			if r.openURL != "" {
				go openDevtoolBrowser(r.openURL)
			}
		}
	}
}

// openDevtoolBrowser opens the URL with the platform browser opener.
func openDevtoolBrowser(url string) {
	if url == "" {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

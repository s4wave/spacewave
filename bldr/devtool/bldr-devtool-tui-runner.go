//go:build !js

package devtool

import (
	"context"
	"os"
	"runtime"
	"sync"

	"github.com/aperturerobotics/util/routine"
	devtool_status "github.com/s4wave/spacewave/bldr/devtool/status"
	"github.com/s4wave/spacewave/bldr/util/logfile"
	"github.com/s4wave/spacewave/bldr/util/termui"
	"github.com/sirupsen/logrus"
	"golang.org/x/term"
)

type devtoolTUIRunner struct {
	// input receives terminal key events.
	input *os.File
	// output receives the rendered dashboard.
	output *os.File
	// openURL is opened by the browser launcher.
	openURL string
	// le records browser-launch failures.
	le *logrus.Entry

	// browserMtx guards browserRunning.
	browserMtx sync.Mutex
	// browserRunning prevents concurrent browser-launch subprocesses.
	browserRunning bool
	// browserProcess supervises the active browser-launch subprocess.
	browserProcess *routine.RoutineContainer
	// newBrowserProcess constructs the platform browser launcher.
	newBrowserProcess func(context.Context, *logrus.Entry, string) *CLIProcessSupervisor
}

func (a *DevtoolArgs) startDevtoolTUI(
	ctx context.Context,
	producer *devtool_status.BldrDevtoolStatusProducer,
	openURL string,
) (context.Context, func()) {
	if producer == nil || !a.shouldRunTUI(os.Stdin, os.Stderr) {
		return ctx, func() {}
	}
	runner := &devtoolTUIRunner{
		input:             os.Stdin,
		output:            os.Stderr,
		openURL:           openURL,
		le:                a.Logger,
		browserProcess:    routine.NewRoutineContainer(),
		newBrowserProcess: newDevtoolBrowserProcess,
	}
	logfile.DiscardConsoleOutput(a.Logger.Logger)
	return runner.start(ctx, producer)
}

func (r *devtoolTUIRunner) start(
	ctx context.Context,
	producer *devtool_status.BldrDevtoolStatusProducer,
) (context.Context, func()) {
	uiCtx, cancel := context.WithCancel(ctx)
	r.browserProcess.SetContext(uiCtx, false)
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
			r.handleKey(uiCtx, cancel),
		)
	}()

	return uiCtx, func() {
		cancel()
		<-done
		r.stopBrowserProcess()
	}
}

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

func (r *devtoolTUIRunner) handleKey(
	ctx context.Context,
	cancel context.CancelFunc,
) termui.KeyHandler[*devtool_status.BldrDevtoolStatus] {
	return func(_ *devtool_status.BldrDevtoolStatus, key byte) {
		switch key {
		case 3:
			cancel()
		case 'o', 'O':
			r.launchBrowser(ctx)
		}
	}
}

func (r *devtoolTUIRunner) launchBrowser(ctx context.Context) {
	if r.openURL == "" || ctx.Err() != nil {
		return
	}

	r.browserMtx.Lock()
	if r.browserRunning {
		r.browserMtx.Unlock()
		return
	}
	proc := r.newBrowserProcess(ctx, r.le, r.openURL)
	if proc == nil {
		r.browserMtx.Unlock()
		return
	}
	r.browserRunning = true
	r.browserMtx.Unlock()

	r.browserProcess.SetRoutine(func(ctx context.Context) error {
		defer func() {
			r.browserMtx.Lock()
			r.browserRunning = false
			r.browserMtx.Unlock()
		}()

		if err := proc.Start(); err != nil {
			if r.le != nil {
				r.le.WithError(err).Warn("open browser")
			}
			return nil
		}

		select {
		case <-proc.Done():
			err := proc.Wait()
			if err != nil && ctx.Err() == nil && r.le != nil {
				r.le.WithError(err).Warn("open browser")
			}
		case <-ctx.Done():
			_ = proc.Terminate()
		}
		return nil
	})
}

func (r *devtoolTUIRunner) stopBrowserProcess() {
	wait, _ := r.browserProcess.SetRoutine(nil)
	if wait != nil {
		<-wait
	}
}

func newDevtoolBrowserProcess(
	ctx context.Context,
	le *logrus.Entry,
	url string,
) *CLIProcessSupervisor {
	if url == "" {
		return nil
	}
	var binaryPath string
	var args []string
	switch runtime.GOOS {
	case "darwin":
		binaryPath = "open"
		args = []string{url}
	case "windows":
		binaryPath = "rundll32"
		args = []string{"url.dll,FileProtocolHandler", url}
	default:
		binaryPath = "xdg-open"
		args = []string{url}
	}
	return NewCLIProcessSupervisor(ctx, le, binaryPath, args)
}

package main

import (
	"context"
	"os"
	"os/exec"

	"github.com/sirupsen/logrus"
)

// LogLevel is the default log level to use.
var LogLevel = logrus.InfoLevel // logrus.DebugLevel

// main starts the runtime with Electron as a sub-process.
//
// This is useful for debugging as the debugger can start the root process.
// This is not intended to be a CLI application or invoked manually.
// This is not intended to be included in a production build.
func main() {
	ctx := context.Background()
	log := logrus.New()

	if dl, dlOk := os.LookupEnv("LOG_LEVEL"); dlOk {
		if err := (&LogLevel).UnmarshalText([]byte(dl)); err != nil {
			logrus.NewEntry(log).
				WithError(err).
				Errorf("LOG_LEVEL variable is in invalid format: %s", dl)
		}
	}

	log.SetLevel(LogLevel)
	le := logrus.NewEntry(log)

	pipeFile := "./.pipe"
	if _, err := os.Stat(pipeFile); err == nil {
		_ = os.RemoveAll(pipeFile)
	}

	// Initialize the electron sub-process
	electronCmd := exec.Command(
		"node",
		"start-electron.js",
	)
	electronCmd.Stderr = os.Stderr
	inc, err := electronCmd.StdinPipe()
	if err != nil {
		le.WithError(err).Fatal("cannot pipe stdin")
		return
	}
	outc, err := electronCmd.StdoutPipe()
	if err != nil {
		le.WithError(err).Fatal("cannot pipe stdout")
		return
	}
	strm := &stdioStream{
		Writer: inc,
		Reader: outc,
	}
	if err := electronCmd.Start(); err != nil {
		le.WithError(err).Fatal("cannot start electron")
		return
	}
	defer electronCmd.Process.Kill()

	_ = strm
	_ = ctx

	// <-ctx.Done()
	electronCmd.Wait()
	// sess.Close()
}

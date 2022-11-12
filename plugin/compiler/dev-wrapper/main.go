package main

import (
	"context"
	"os"
	"os/exec"
	"os/signal"
	"strings"
)

// DelveAddr is the address to listen with headless delve.
// If empty, uses "go build" then runs the binary.
var DelveAddr string

// BuildFlags is the list of build flags.
// Can be overridden by the compiler.
var BuildFlags []string

func main() {
	err := run()
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}

func run() error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	srcDir := wd
	runCmd := func(entry string, withStdio bool, args ...string) error {
		ecmd := exec.Command(entry, args...)
		ecmd.Env = os.Environ()
		ecmd.Dir = srcDir
		if withStdio {
			ecmd.Stdin = os.Stdin
			ecmd.Stdout = os.Stdout
		} else {
			ecmd.Stdout = os.Stderr
		}
		ecmd.Stderr = os.Stderr
		os.Stderr.WriteString(ecmd.String() + "\n")
		if err := ecmd.Start(); err != nil {
			return err
		}
		subCtx, subCtxCancel := context.WithCancel(context.Background())
		defer subCtxCancel()
		go func() {
			// forward sigint to subprocess
			ch := make(chan os.Signal, 1)
			signal.Notify(ch, os.Interrupt)
			for {
				select {
				case <-subCtx.Done():
					return
				case sig := <-ch:
					_ = ecmd.Process.Signal(sig)
				}
			}
		}()
		return ecmd.Wait()
	}

	if DelveAddr == "wait" {
		if os.Args[len(os.Args)-1] == "exec-plugin" {
			os.Stderr.WriteString("Waiting for you to manually run the plugin entrypoint.\n")
			ch := make(chan os.Signal, 1)
			signal.Notify(ch, os.Interrupt)
			<-ch
			return nil
		}

		// run interactively
		return runCmd(
			"dlv", true,
			"debug",
			"--build-flags", strings.Join(BuildFlags, " "),
			"--",
			"exec-plugin",
		)
	}

	if DelveAddr != "" {
		return runCmd(
			"dlv", true,
			"debug",
			"--listen",
			DelveAddr,
			"--headless",
			"--accept-multiclient",
			"--build-flags", strings.Join(BuildFlags, " "),
			"--",
			"exec-plugin",
		)
	}

	goArgs := []string{"build", "-gcflags=-N -l", "-o", "plugin"}
	goArgs = append(goArgs, BuildFlags...)
	if err := runCmd("go", false, goArgs...); err != nil {
		return err
	}

	return runCmd("./plugin", true, "exec-plugin")
}

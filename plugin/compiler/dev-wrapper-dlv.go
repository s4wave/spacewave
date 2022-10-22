package plugin_compiler

// DevWrapperDlvSrc is a Go program which starts a plugin with Delve.
//
// NOTE: "go run ./" strips debug information (delve cannot attach).
const DevWrapperDlvSrc = `package main

import (
	"os"
	"os/exec"
)

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
		return ecmd.Run()
	}

	if err := runCmd("go", false, "build", "-v", "-mod=readonly", "-trimpath", "-o", "plugin"); err != nil {
		return err
	}

	return runCmd("./plugin", true, "exec-plugin")
}
`

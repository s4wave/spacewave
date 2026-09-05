//go:build windows

package spacewave_launcher_controller

import (
	"os"
	"strconv"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// replaceFile replaces the installed binary without first unlinking it.
// A failed same-volume rename leaves the installed version in place.
func replaceFile(tmpPath, dstPath string) error {
	return errors.Wrap(windows.Rename(tmpPath, dstPath), "replace destination")
}

// startRawUpdateRelay transfers update completion to the staged executable.
func startRawUpdateRelay(tmpPath, targetPath string) error {
	// Start the relay before exiting so launch failures preserve this process.
	proc, err := os.StartProcess(tmpPath, rawUpdateArgs(tmpPath), &os.ProcAttr{
		Env:   rawUpdateRelayEnv(targetPath),
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		return errors.Wrap(err, "start raw update relay")
	}

	// The relay waits for our exit; it must outlive this process handle.
	_ = proc.Release() // Process exit also releases the local handle.
	os.Exit(0)
	return nil
}

// startRawUpdateTarget starts the replacement with relay cleanup instructions.
func startRawUpdateTarget(targetPath, cleanupPath string) error {
	// Start the target with the original command arguments.
	proc, err := os.StartProcess(targetPath, rawUpdateArgs(targetPath), &os.ProcAttr{
		Env:   rawUpdateTargetEnv(cleanupPath),
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		return errors.Wrap(err, "start raw update target")
	}

	// The replacement runs independently after this relay exits.
	_ = proc.Release() // Process exit also releases the local handle.
	os.Exit(0)
	return nil
}

// waitRawUpdateRelayParent waits until Windows releases the installed image.
func waitRawUpdateRelayParent() error {
	// An absent parent identifier means no process needs to exit first.
	raw := os.Getenv(rawUpdateRelayParentEnv)
	if raw == "" {
		return nil
	}
	pid, err := strconv.Atoi(raw)
	if err != nil {
		return errors.Wrap(err, "parse relay parent pid")
	}

	// Wait on the parent process handle rather than polling its identifier.
	proc, err := os.FindProcess(pid)
	if err != nil {
		return errors.Wrap(err, "find relay parent process")
	}
	_, err = proc.Wait()
	if err != nil {
		return errors.Wrap(err, "wait for relay parent")
	}
	return nil
}

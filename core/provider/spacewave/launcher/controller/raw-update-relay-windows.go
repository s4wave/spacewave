//go:build windows

package spacewave_launcher_controller

import (
	"os"
	"strconv"

	"github.com/pkg/errors"
)

func replaceFile(tmpPath, dstPath string) error {
	if err := os.Remove(dstPath); err != nil && !os.IsNotExist(err) {
		return errors.Wrap(err, "remove destination")
	}
	if err := os.Rename(tmpPath, dstPath); err != nil {
		return errors.Wrap(err, "replace destination")
	}
	return nil
}

func startRawUpdateRelay(tmpPath, targetPath string) error {
	proc, err := os.StartProcess(tmpPath, rawUpdateArgs(tmpPath), &os.ProcAttr{
		Env:   rawUpdateRelayEnv(targetPath),
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		return errors.Wrap(err, "start raw update relay")
	}
	_ = proc.Release()
	os.Exit(0)
	return nil
}

func startRawUpdateTarget(targetPath, cleanupPath string) error {
	proc, err := os.StartProcess(targetPath, rawUpdateArgs(targetPath), &os.ProcAttr{
		Env:   rawUpdateTargetEnv(cleanupPath),
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
	})
	if err != nil {
		return errors.Wrap(err, "start raw update target")
	}
	_ = proc.Release()
	os.Exit(0)
	return nil
}

func waitRawUpdateRelayParent() error {
	raw := os.Getenv(rawUpdateRelayParentEnv)
	if raw == "" {
		return nil
	}
	pid, err := strconv.Atoi(raw)
	if err != nil {
		return errors.Wrap(err, "parse relay parent pid")
	}
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

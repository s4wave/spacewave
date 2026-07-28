//go:build !windows

package launcher_helper

import "os/exec"

func prepareHelperCommand(_ *exec.Cmd) {}

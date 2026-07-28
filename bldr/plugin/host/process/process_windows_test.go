//go:build windows

package plugin_host_process

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestPreStartCmdHidesPluginConsole(t *testing.T) {
	cmd := exec.Command("plugin.exe")
	if _, err := preStartCmd(cmd); err != nil {
		t.Fatal(err)
	}
	if cmd.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if flags := cmd.SysProcAttr.CreationFlags; flags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_NO_WINDOW", flags)
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("HideWindow is false")
	}
}

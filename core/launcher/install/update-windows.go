//go:build windows

package launcher_install

import (
	"bytes"
	"os"
	"path/filepath"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
)

// Update replaces the installed launcher with source and the helper with
// helper once process pid, the desktop that asked for the update, has exited.
// It refreshes the Start menu and Settings > Apps entries and starts the new
// launcher, which reopens the desktop.
func (i *Installer) Update(source string, helper []byte, pid uint32) error {
	// Let the desktop exit before its replacement opens.
	if err := waitForExit(pid, processExitTimeout); err != nil {
		return err
	}

	// Replace the helper and refresh the entries that point to the
	// launcher.
	if err := os.MkdirAll(i.dir, 0o700); err != nil {
		return errors.Wrap(err, "create install directory")
	}
	if err := replace(filepath.Join(i.dir, helperName), bytes.NewReader(helper)); err != nil {
		return err
	}
	if err := writeShortcut(i.shortcut, i.launcher, "Spacewave"); err != nil {
		return err
	}
	if err := writeUninstallEntry(i.dir, i.launcher); err != nil {
		return err
	}

	// Replace the launcher from this executable and start it.
	input, err := os.Open(source)
	if err != nil {
		return errors.Wrap(err, "open launcher source")
	}
	defer input.Close()
	if err := replace(i.launcher, input); err != nil {
		return err
	}
	return i.Start()
}

// waitForExit waits up to timeout for process pid to exit. A process that can
// no longer be opened has already exited.
func waitForExit(pid uint32, timeout time.Duration) error {
	// Open the process to wait on it.
	process, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return nil
	}
	defer windows.CloseHandle(process)

	// Wait for its exit until the timeout.
	event, err := windows.WaitForSingleObject(process, uint32(timeout.Milliseconds()))
	if err != nil {
		return errors.Wrap(err, "wait for the desktop to exit")
	}
	if event != windows.WAIT_OBJECT_0 {
		return errors.New("the desktop is still running")
	}
	return nil
}

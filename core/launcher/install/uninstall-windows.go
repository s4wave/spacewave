//go:build windows

package launcher_install

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"syscall"
	"time"

	"github.com/pkg/errors"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// processExitTimeout bounds the wait for Spacewave processes to exit after the
// daemon stops.
const processExitTimeout = 30 * time.Second

// Uninstall removes the install once the daemon serving stateRoot has been
// asked to stop. It waits for Spacewave processes under the install directory
// and stateRoot to exit, removes the Start menu and Settings > Apps entries,
// and starts a temporary helper that deletes the install directory, and
// stateRoot when deleteData is set, after this process exits.
func (i *Installer) Uninstall(stateRoot string, deleteData bool) error {
	// Let the daemon, Electron and any CLI from the install release their files.
	if err := waitForProcessesUnder([]string{i.dir, stateRoot}, processExitTimeout); err != nil {
		return err
	}

	// Remove the Start menu and Settings > Apps entries.
	if err := os.Remove(i.shortcut); err != nil && !errors.Is(err, os.ErrNotExist) {
		return errors.Wrap(err, "remove Start menu shortcut")
	}
	if err := registry.DeleteKey(registry.CURRENT_USER, uninstallKey); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return errors.Wrap(err, "remove uninstall registry key")
	}

	// Specify the directories the helper deletes once this process exits.
	args := []string{"--remove", i.dir}
	if deleteData {
		args = append(args, "--remove", stateRoot)
	}
	args = append(args, "--pid", strconv.Itoa(os.Getpid()))

	// Run the helper from a temporary copy outside the directories it deletes.
	helper, err := copyToTemp(filepath.Join(i.dir, helperName))
	if err != nil {
		return err
	}
	cmd := exec.Command(helper, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW, HideWindow: true}
	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "start uninstall helper")
	}
	return cmd.Process.Release()
}

// copyToTemp copies the file at path to a new executable in the temporary
// directory and returns the copy's path.
func copyToTemp(path string) (string, error) {
	// Open the source and a uniquely named destination.
	input, err := os.Open(path)
	if err != nil {
		return "", errors.Wrap(err, "open helper")
	}
	defer input.Close()
	output, err := os.CreateTemp("", "spacewave-uninstall-*.exe")
	if err != nil {
		return "", errors.Wrap(err, "create temporary helper")
	}

	// Copy and close the destination before it runs.
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return "", errors.Wrap(err, "copy helper")
	}
	if err := output.Close(); err != nil {
		return "", errors.Wrap(err, "close temporary helper")
	}
	return output.Name(), nil
}

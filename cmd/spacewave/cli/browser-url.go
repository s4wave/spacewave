//go:build !js

package spacewave_cli

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// keyCtrlC is the byte a raw-mode terminal sends for Ctrl+C.
const keyCtrlC = 0x03

// showBrowserURL tells the user that action continues in the browser at url.
// When stdin is a terminal it listens for keys until the returned stop is
// called: c copies url and Ctrl+C calls cancel. Call stop before reading
// further input.
func showBrowserURL(cancel context.CancelFunc, action, url string) (stop func()) {
	// Listen for keys while the browser step runs.
	stop, listening := startKeyListener(func(key byte) {
		switch key {
		case 'c', 'C':
			if err := copyText(url); err != nil {
				os.Stderr.WriteString("Copy failed: " + err.Error() + "\r\n")
				return
			}
			os.Stderr.WriteString("Copied the URL.\r\n")
		case keyCtrlC:
			cancel()
		}
	})

	// Show the URL, with the keys when the listener runs.
	var b strings.Builder
	b.WriteString("Opening your browser to " + action + ".\r\n")
	b.WriteString("If it does not open, visit:\r\n\r\n  " + url + "\r\n\r\n")
	if listening {
		b.WriteString("Press c to copy the URL, or Ctrl+C to cancel.\r\n")
	}
	os.Stderr.WriteString(b.String())
	return stop
}

// copyText copies text with the platform clipboard command. Without one it
// asks the terminal to copy with an OSC 52 escape, which also works over SSH.
func copyText(text string) error {
	// Prefer the platform clipboard command.
	if cmd := clipboardCommand(); cmd != nil {
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}

	// Fall back to the terminal's clipboard.
	_, err := os.Stderr.WriteString("\x1b]52;c;" + base64.StdEncoding.EncodeToString([]byte(text)) + "\a")
	return err
}

// clipboardCommand returns the platform command that copies its stdin, or nil
// when none is installed.
func clipboardCommand() *exec.Cmd {
	switch {
	case runtime.GOOS == "darwin":
		return exec.Command("pbcopy")
	case runtime.GOOS == "windows":
		return exec.Command("clip")
	case commandExists("wl-copy"):
		return exec.Command("wl-copy")
	case commandExists("xclip"):
		return exec.Command("xclip", "-selection", "clipboard")
	case commandExists("xsel"):
		return exec.Command("xsel", "--clipboard", "--input")
	default:
		return nil
	}
}

// commandExists reports whether name is on PATH.
func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

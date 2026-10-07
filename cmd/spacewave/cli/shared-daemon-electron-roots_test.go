//go:build !js && !wasip1

package spacewave_cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/bldr/web/plugin/electron"
)

// TestSharedDaemonElectronRoots checks that two roots run independent daemons
// and desktops: each keeps its own profile and window, and neither opening nor
// closing one touches the other.
func TestSharedDaemonElectronRoots(t *testing.T) {
	// Start two roots, each with its own CLI, Space, and status watch.
	app := requireSharedElectronApp(t)
	opts := sharedElectronOptions{shape: "native-core", policy: electron.DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME}
	a := newSharedElectronDaemon(t, app, opts)
	b := newSharedElectronDaemon(t, app, opts)
	cliA := a.attachCLI(t, "Root A Space")
	cliB := b.attachCLI(t, "Root B Space")
	statusA := a.watchStatus(t)
	statusB := b.watchStatus(t)

	// Open both desktops; each window shows its own root's Session and Space.
	viewA := a.open(t, a.desktop)
	a.requireIdentity(t, viewA, cliA.identity("Root A Space"))
	viewB := b.open(t, b.desktop)
	b.requireIdentity(t, viewB, cliB.identity("Root B Space"))
	a.requireIdentity(t, viewA, cliA.identity("Root A Space"))
	for _, d := range []*sharedElectronDaemon{a, b} {
		entries, err := os.ReadDir(filepath.Join(d.statePath, "electron-profile"))
		if err != nil || len(entries) == 0 {
			t.Fatalf("root %s has no Electron profile: %v", d.statePath, err)
		}
	}

	// Closing B's window and client ends only B's desktop and daemon.
	_ = viewB.Remove(b.ctx)
	requireDesktopEnded(t, statusB, 1)
	cliB.close()
	b.waitExited(t)
	a.requireRunning(t)
	a.requireIdentity(t, viewA, cliA.identity("Root A Space"))

	// A still quits on its own terms through its own daemon.
	cliA.close()
	a.waitIdle(t, func(s daemonIdleSnapshot) bool { return s.clients == 0 })
	a.requestQuit(t)
	if got := a.nextQuit(t).GetOtherWork(); len(got) != 0 {
		t.Fatalf("root A Quit named %v", got)
	}
	a.waitExited(t)
	requireDesktopEnded(t, statusA, 1)
}

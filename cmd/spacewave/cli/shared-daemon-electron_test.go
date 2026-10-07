//go:build !js && !wasip1

package spacewave_cli

import (
	"slices"
	"testing"

	resource_state "github.com/s4wave/spacewave/bldr/resource/state"
	"github.com/s4wave/spacewave/bldr/web/plugin/electron"
	web_view "github.com/s4wave/spacewave/bldr/web/view"
)

// TestSharedDaemonElectron checks both core routes through an actual Electron
// window, preload bridge, dedicated Worker, and retained Unix-socket client.
// The distribution fixture loads the core through production plugin RPC while
// keeping the core implementation in-process; it does not select a packaged
// distribution or plugin artifact.
//
// These tests are opt-in. Build the app with
// "go run ./e2e/shareddaemon/prepare <dir>" after "go mod vendor", set
// SPACEWAVE_SHARED_DESKTOP_FIXTURE to that directory and
// SPACEWAVE_SHARED_DESKTOP_ELECTRON to an Electron executable, then run them on
// a display with e2e/shareddaemon/run.sh. Set SPACEWAVE_SHARED_DESKTOP_DEBUG to
// log the daemon and Electron.
func TestSharedDaemonElectron(t *testing.T) {
	// Run sequentially because each shell uses an isolated process environment.
	app := requireSharedElectronApp(t)
	for _, shape := range []string{"native-core", "distribution-core"} {
		t.Run(shape, func(t *testing.T) {
			runSharedDaemonElectron(t, app, shape)
		})
	}
}

// runSharedDaemonElectron retains one CLI Session watch through two shell generations.
func runSharedDaemonElectron(t *testing.T, app sharedElectronApp, shape string) {
	// Start the daemon, seed one Space through the CLI, and observe desktop lifecycle
	// separately from the retained Resource stream.
	d := newSharedElectronDaemon(t, app, sharedElectronOptions{shape: shape, policy: electron.DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME})
	cli := d.attachCLI(t, "CLI seeded Space")
	status := d.watchStatus(t)

	// Open, inspect the displayed Session and Space, then rename from the actual Worker.
	view := d.open(t, cli.desktop)
	d.requireIdentity(t, view, cli.identity("CLI seeded Space"))
	renameFromWorker(t, d, view, "worker first open")
	cli.requireName(t, "worker first open")

	// Close the BrowserWindow; its IPC reply can disappear with the renderer.
	// The daemon status watch confirms the joined shell result, and closing a
	// window never stops the daemon.
	_ = view.Remove(d.ctx)
	requireDesktopEnded(t, status, 1)
	d.requireRunning(t)
	if _, err := cli.session.RenameSpace(d.ctx, cli.spaceID, "CLI while closed"); err != nil {
		t.Fatal(err)
	}
	cli.requireName(t, "CLI while closed")

	// Reopen on the same authority and prove the original CLI stream still receives worker writes.
	view = d.open(t, cli.desktop)
	d.requireIdentity(t, view, cli.identity("CLI while closed"))
	renameFromWorker(t, d, view, "worker after reopen")
	cli.requireName(t, "worker after reopen")

	// Observe the second joined process exit even if the renderer loses its reply.
	_ = view.Remove(d.ctx)
	requireDesktopEnded(t, status, 2)
}

// TestSharedDaemonElectronQuit checks the Quit and idle contract with a real
// shell: the daemon stops only when no other client or service uses it.
func TestSharedDaemonElectronQuit(t *testing.T) {
	app := requireSharedElectronApp(t)
	opts := sharedElectronOptions{shape: "native-core", policy: electron.DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_WINDOW_LIFETIME}

	// A closed window leaves the daemon to the idle rule: it exits once the last client leaves.
	t.Run("idle-exit", func(t *testing.T) {
		// Open a window for a daemon that one CLI also uses.
		d := newSharedElectronDaemon(t, app, opts)
		cli := d.attachCLI(t, "Idle Space")
		status := d.watchStatus(t)
		view := d.open(t, d.desktop)
		d.requireIdentity(t, view, cli.identity("Idle Space"))

		// Closing the window ends the shell but not the daemon.
		_ = view.Remove(d.ctx)
		requireDesktopEnded(t, status, 1)
		d.requireRunning(t)

		// The idle deadline follows the last client leaving.
		cli.close()
		d.waitExited(t)
	})

	// Quit with another client and a background service declines, names both,
	// and leaves the retained CLI watch running; Quit stops the daemon once they leave.
	t.Run("busy-then-stop", func(t *testing.T) {
		// Hold the daemon with a CLI and a background service beside the desktop.
		d := newSharedElectronDaemon(t, app, opts)
		cli := d.attachCLI(t, "Busy Space")
		status := d.watchStatus(t)
		releaseService := d.idle.serviceAttached("fixture sync")
		view := d.open(t, d.desktop)
		d.requireIdentity(t, view, cli.identity("Busy Space"))

		// The shell reports the declined Quit with every hold that keeps the daemon up.
		d.requestQuit(t)
		want := []string{testPeerName(t), "fixture sync"}
		slices.Sort(want)
		if got := d.nextQuit(t).GetOtherWork(); !slices.Equal(got, want) {
			t.Fatalf("busy Quit named %v, want %v", got, want)
		}
		d.requireRunning(t)
		if snapshot, _ := d.idle.observe(); snapshot.stopping || !snapshot.desktop {
			t.Fatalf("declined Quit changed the daemon: %+v", snapshot)
		}
		if _, err := cli.session.RenameSpace(d.ctx, cli.spaceID, "After declined Quit"); err != nil {
			t.Fatal(err)
		}
		cli.requireName(t, "After declined Quit")

		// Once only the desktop remains, Quit wins the claim and the daemon stops.
		cli.close()
		releaseService()
		d.waitIdle(t, func(s daemonIdleSnapshot) bool { return s.clients == 0 && s.services == 1 })
		d.requestQuit(t)
		if got := d.nextQuit(t).GetOtherWork(); len(got) != 0 {
			t.Fatalf("final Quit named %v", got)
		}
		d.waitExited(t)
		requireDesktopEnded(t, status, 1)
	})
}

// TestSharedDaemonElectronTray checks that the tray keeps the shell, and so the
// daemon, alive after its last window closes until an explicit Quit.
func TestSharedDaemonElectronTray(t *testing.T) {
	// Open a tray-policy shell for a daemon that one CLI also uses.
	app := requireSharedElectronApp(t)
	d := newSharedElectronDaemon(t, app, sharedElectronOptions{shape: "native-core", policy: electron.DesktopPresencePolicy_DESKTOP_PRESENCE_POLICY_TRAY_BACKGROUND})
	cli := d.attachCLI(t, "Tray Space")
	status := d.watchStatus(t)
	view := d.open(t, d.desktop)
	d.requireIdentity(t, view, cli.identity("Tray Space"))

	// With no window and no client, the live tray is the daemon's only demand.
	_ = view.Remove(d.ctx)
	cli.close()
	snapshot := d.waitIdle(t, func(s daemonIdleSnapshot) bool { return s.clients == 0 })
	if snapshot.services != 1 || !snapshot.desktop {
		t.Fatalf("tray did not hold the daemon: %+v", snapshot)
	}
	d.requireRunning(t)

	// An explicit Quit from the windowless shell ends it and the daemon.
	d.requestQuit(t)
	if got := d.nextQuit(t).GetOtherWork(); len(got) != 0 {
		t.Fatalf("tray Quit named %v", got)
	}
	d.waitExited(t)
	requireDesktopEnded(t, status, 1)
}

// renameFromWorker renames the Space from the window's dedicated Worker.
func renameFromWorker(t *testing.T, d *sharedElectronDaemon, view web_view.WebView, name string) {
	t.Helper()
	if err := view.GetClient().ExecCall(d.ctx, "test.SharedDesktop", "Rename",
		&resource_state.SetStateRequest{StateJson: name}, &resource_state.SetStateResponse{}); err != nil {
		t.Fatal(err)
	}
}

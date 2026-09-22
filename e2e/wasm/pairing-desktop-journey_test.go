//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"fmt"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/s4wave/spacewave/e2e/electron"
	"github.com/sirupsen/logrus"
)

// TestPairingBrowserDesktopJourney reads a browser-created file in the actual
// desktop runtime, then restarts that runtime with the browser disconnected.
func TestPairingBrowserDesktopJourney(t *testing.T) {
	if !electron.E2EElectronEnabled() {
		t.Skip("set ENABLE_E2E_ELECTRON=true to include the desktop runtime")
	}
	h := harness(t)
	a := h.NewCleanSession(t)
	drive := CreateDriveScenario(t, h, a)
	WaitForDriveReady(t, h, a.Page())
	file := playwright.InputFile{Name: "browser-to-desktop.md", MimeType: "text/markdown", Buffer: []byte("The desktop keeps its own copy after the browser leaves.\n")}
	uploadDriveFileThroughUI(t, a.Page(), file)
	waitForDriveEntry(t, a.Page(), file.Name)
	ctx, cancel := context.WithTimeout(h.Context(), 12*time.Minute)
	defer cancel()
	source, err := a.MountSessionByIdx(ctx, drive.GetSessionIndex())
	if err != nil {
		t.Fatal(err)
	}
	defer source.Release()
	info, err := source.GetSessionInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}
	desktop, err := electron.Boot(ctx, logrus.NewEntry(logrus.New()))
	if err != nil {
		t.Fatal(err)
	}
	defer desktop.Release()
	if err := desktop.ConnectDriver(); err != nil {
		t.Fatal(err)
	}
	page, err := desktop.WaitForPage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	WaitForApp(t, page)
	startPairingPages(t, a.Page(), page, drive.GetSessionIndex(), "#/pair/{offer}")
	confirmPairingPages(t, a.Page(), page)
	if err := page.GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Account connected", Exact: new(true)}).WaitFor(); err != nil {
		t.Fatal(err)
	}
	index := waitPairingPageCopy(t, page, info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId(), drive.GetSpaceID())
	navigatePairingPage(t, page, fmt.Sprintf("#/u/%d/so/%s", index, drive.GetSpaceID()))
	WaitForDriveShell(t, page)
	driveURL := page.URL()
	openDriveEntry(t, page, file.Name)
	waitForUnixFSFileText(t, page, "desktop paired file", string(file.Buffer))
	a.release()
	if err := desktop.Relaunch(ctx); err != nil {
		t.Fatal(err)
	}
	page, err = desktop.WaitForPage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	// Boot the restarted renderer at the saved Drive route before session routing.
	if _, err := page.Goto("about:blank"); err != nil {
		t.Fatal(err)
	}
	if _, err := page.Goto(driveURL); err != nil {
		t.Fatal(err)
	}
	WaitForApp(t, page)
	WaitForDriveShell(t, page)
	openDriveEntry(t, page, file.Name)
	waitForUnixFSFileText(t, page, "desktop file after restart", string(file.Buffer))
}

// waitPairingPageCopy checks the actual SDK stream exposed by either runtime.
// Account transitions can replace a mounted resource, so the watch remounts
// within one deadline. Session totals must equal their peer breakdown.
func waitPairingPageCopy(t *testing.T, page playwright.Page, accountID string, ids ...string) uint32 {
	t.Helper()
	result, err := page.Evaluate(`async ({ accountID, ids }) => {
		const deadline = Date.now() + 120000
		let latest = null
		let lastError = null
		while (Date.now() < deadline) {
			const signal = AbortSignal.timeout(Math.min(10000, deadline - Date.now()))
			let session = null
			let invariantError = null
			try {
				const root = globalThis.__s4wave_debug.root
				const entries = (await root.listSessions(signal)).sessions ?? []
				const entry = entries.find(
					(entry) => entry.sessionRef?.providerResourceRef?.providerAccountId === accountID,
				)
				if (!entry) throw new Error('paired account has no Session')
				const mounted = await root.mountSessionByIdx(
					{ sessionIdx: entry.sessionIndex },
					signal,
				)
				session = mounted.session
				if (!session) throw new Error('paired Session did not mount')
				for await (const state of session.watchSyncStatus({}, signal)) {
					latest = state
					const uploaded = (state.peers ?? []).reduce(
						(sum, peer) => sum + BigInt(peer.uploadedBytes ?? 0),
						0n,
					)
					const downloaded = (state.peers ?? []).reduce(
						(sum, peer) => sum + BigInt(peer.downloadedBytes ?? 0),
						0n,
					)
					const totalsMatch =
						uploaded === BigInt(state.peerUploadBytes ?? 0) &&
						downloaded === BigInt(state.peerDownloadBytes ?? 0)
					if (!totalsMatch) {
						invariantError = new Error('peer byte totals differ from Session totals')
						throw invariantError
					}
					const copied = ids.every((id) => state.localCopies?.some(
						(copy) => copy.sharedObjectId === id && copy.complete,
					))
					if (copied) return entry.sessionIndex
				}
				lastError = new Error('copy stream closed before files became durable')
			} catch (error) {
				if (error === invariantError) throw error
				lastError = error
			} finally {
				try {
					session?.release()
				} catch (error) {
					lastError = error
				}
			}
			await new Promise((resolve) => setTimeout(resolve, 100))
		}
		const state = JSON.stringify(latest, (_, value) => typeof value === 'bigint' ? value.toString() : value)
		throw new Error('copy state for ' + accountID + ' / ' + ids.join(', ') + ': ' + state + '; last error: ' + lastError)
	}`, map[string]any{"accountID": accountID, "ids": ids})
	if err != nil {
		t.Fatal(err)
	}
	index, ok := result.(int)
	if !ok || index <= 0 {
		t.Fatalf("unexpected paired Session index: %T %v", result, result)
	}
	return uint32(index)
}

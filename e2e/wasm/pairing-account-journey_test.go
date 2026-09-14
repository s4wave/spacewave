//go:build !skip_e2e && !js

package wasm

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
	s4wave_session "github.com/s4wave/spacewave/sdk/session"
)

// TestPairingHomeDriveJourney enters from Home, reads its durable copy offline,
// then receives a new file after both clients reconnect with preserved stores.
func TestPairingHomeDriveJourney(t *testing.T) {
	started := time.Now()
	// Retain failures from both independent browser processes.
	h := harness(t)
	t.Logf("harness ready after %s", time.Since(started).Round(time.Millisecond))
	a, b := h.NewCleanSession(t), h.NewCleanSession(t)
	t.Logf("clients ready after %s", time.Since(started).Round(time.Millisecond))
	for _, client := range []*TestSession{a, b} {
		messages, stop := client.WatchConsole()
		done := make(chan struct{})
		go func() {
			defer close(done)
			for message := range messages {
				if strings.Contains(message, "panic") || strings.Contains(message, "error") {
					t.Log(message)
				}
			}
		}()
		t.Cleanup(func() {
			stop()
			<-done
		})
	}
	t.Cleanup(func() {
		if t.Failed() {
			for _, client := range []*TestSession{a, b} {
				if client.Page() == nil {
					continue
				}
				body, err := client.Page().Locator("body").InnerText()
				t.Logf("pairing page %s: %s (read error: %v)", client.Page().URL(), body, err)
				diagnostic, err := client.Page().Evaluate(`() => JSON.stringify({timing: globalThis.__s4waveQuickstartTiming, root: !!globalThis.__s4wave_debug?.root})`)
				t.Logf("quickstart state: %v (read error: %v)", diagnostic, err)
			}
		}
	})

	// Seed the offered account with a file whose bytes must survive disconnection.
	ctx, cancel := context.WithTimeout(h.Context(), 5*time.Minute)
	defer cancel()
	drive := CreateDriveScenario(t, h, a)
	WaitForDriveReady(t, h, a.Page())
	file := playwright.InputFile{Name: "paired-file.md", MimeType: "text/markdown", Buffer: []byte("This file survives after the original pairing client leaves.\n")}
	uploadDriveFileThroughUI(t, a.Page(), file)
	waitForDriveEntry(t, a.Page(), file.Name)
	source, err := a.MountSessionByIdx(ctx, drive.GetSessionIndex())
	if err != nil {
		t.Fatal(err)
	}
	defer source.Release()
	info, err := source.GetSessionInfo(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// Approve the exchange through Home and locate the enrolled account.
	startPairingPages(t, a.Page(), b.Page(), drive.GetSessionIndex(), "#/")
	confirmPairingPages(t, a.Page(), b.Page())
	if err := b.Page().GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Account connected", Exact: new(true)}).WaitFor(); err != nil {
		t.Fatal(err)
	}
	entries, err := b.Root().ListSessions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var index uint32
	for _, entry := range entries {
		if entry.GetSessionRef().GetProviderResourceRef().GetProviderAccountId() == info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId() {
			index = entry.GetSessionIndex()
		}
	}
	if index == 0 {
		t.Fatal("pairing did not register the offered account")
	}

	// Wait for the receiver's own block copy before opening the file.
	receiver, err := b.MountSessionByIdx(ctx, index)
	if err != nil {
		t.Fatal(err)
	}
	defer receiver.Release()
	stream, err := receiver.WatchSyncStatus(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Close()
	waitPairingSpaceCopy(t, stream, drive.GetSpaceID())
	t.Logf("receiver copy durable after %s", time.Since(started).Round(time.Millisecond))
	NavigateHash(t, h, b.Page(), fmt.Sprintf("#/u/%d/so/%s", index, drive.GetSpaceID()))
	WaitForDriveReady(t, h, b.Page())
	openDriveEntry(t, b.Page(), file.Name)
	waitForUnixFSFileText(t, b.Page(), "paired file", string(file.Buffer))

	// Reopen the receiver without a live original client.
	stream.Close()
	receiver.Release()
	source.Release()
	if err := a.ReplacePageInCurrentContext(); err != nil {
		t.Fatal(err)
	}
	b.disconnectResources()
	if _, err := b.Page().Reload(); err != nil {
		t.Fatal(err)
	}
	waitForUnixFSFileText(t, b.Page(), "offline paired file", string(file.Buffer))
	t.Logf("offline read complete after %s", time.Since(started).Round(time.Millisecond))

	// Restart the original process and prove continued synchronization.
	if err := h.loadAppPageURL(a, fmt.Sprintf("%s/#/u/%d/so/%s", h.BaseURL(), drive.GetSessionIndex(), drive.GetSpaceID())); err != nil {
		t.Fatal(err)
	}
	WaitForApp(t, a.Page())
	WaitForDriveReady(t, h, a.Page())
	t.Logf("source restarted after %s", time.Since(started).Round(time.Millisecond))
	update := playwright.InputFile{Name: "after-reconnect.md", MimeType: "text/markdown", Buffer: []byte("This update arrived after both clients restarted.\n")}
	uploadDriveFileThroughUI(t, a.Page(), update)
	waitForDriveEntry(t, a.Page(), update.Name)
	NavigateHash(t, h, b.Page(), fmt.Sprintf("#/u/%d/so/%s", index, drive.GetSpaceID()))
	WaitForDriveReady(t, h, b.Page())
	openDriveEntry(t, b.Page(), update.Name)
	waitForUnixFSFileText(t, b.Page(), "file after reconnect", string(update.Buffer))
	t.Logf("reconnected update received after %s", time.Since(started).Round(time.Millisecond))
}

// waitPairingSpaceCopy waits for the selected Space's complete local block copy.
func waitPairingSpaceCopy(t *testing.T, stream s4wave_session.SRPCSessionResourceService_WatchSyncStatusClient, id string) {
	t.Helper()
	var lastError string
	for {
		state, err := stream.Recv()
		if err != nil {
			t.Fatalf("waiting for copied Space: %v (last copy state: %s)", err, lastError)
		}
		for _, copy := range state.GetLocalCopies() {
			if copy.GetSharedObjectId() != id {
				continue
			}
			if message := copy.GetError(); message != lastError {
				lastError = message
				if message != "" {
					t.Logf("Space copy is retrying: %s", message)
				}
			}
			if copy.GetComplete() {
				return
			}
		}
	}
}

// startPairingPages exchanges real direct payloads through the shared pairing UI.
func startPairingPages(t *testing.T, source, receiving playwright.Page, sourceIndex uint32, receivingRoute string) {
	t.Helper()

	// Both clients run on this host. Gather host candidates without waiting
	// for a public STUN server that the CI runner may be unable to reach.
	// Keep the same network environment when either client reloads.
	hostCandidatesOnly := `(() => {
		const PeerConnection = globalThis.RTCPeerConnection
		globalThis.RTCPeerConnection = class extends PeerConnection {
			constructor(config) { super({ ...config, iceServers: [] }) }
		}
	})()`
	for _, page := range []playwright.Page{source, receiving} {
		if err := page.Context().AddInitScript(playwright.Script{Content: &hostCandidatesOnly}); err != nil {
			t.Fatal(err)
		}
		if _, err := page.Evaluate(hostCandidatesOnly); err != nil {
			t.Fatal(err)
		}
	}

	// Generate the source's direct offer through its signed-in entry point.
	navigatePairingPage(t, source, "#/")
	navigatePairingPage(t, receiving, "#/")
	navigatePairingPage(t, source, fmt.Sprintf("#/u/%d/setup/link-device", sourceIndex))
	if err := source.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Show QR code"}).Click(); err != nil {
		t.Fatal(err)
	}
	offer := source.Locator("input[readonly]").First()
	if err := offer.WaitFor(); err != nil {
		t.Fatal(err)
	}
	payload, err := offer.InputValue()
	if err != nil {
		t.Fatal(err)
	}

	// Submit the offer through the receiving client's selected entry point.
	navigatePairingPage(t, receiving, strings.ReplaceAll(receivingRoute, "{offer}", payload))
	if receivingRoute == "#/" {
		if err := receiving.GetByText("Enter a device pairing code", playwright.PageGetByTextOptions{Exact: new(true)}).Click(); err != nil {
			t.Fatal(err)
		}
		if err := receiving.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Use a QR code or link", Exact: new(true)}).Click(); err != nil {
			t.Fatal(err)
		}
		payload = "https://spacewave.app/#/pair/" + payload
	}
	if strings.HasSuffix(receivingRoute, "/setup/link-device") {
		if err := receiving.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Scan QR code"}).Click(); err != nil {
			t.Fatal(err)
		}
	}
	if !strings.Contains(receivingRoute, "{offer}") {
		if err := receiving.GetByPlaceholder("Paste offer payload here…").Fill(payload); err != nil {
			t.Fatal(err)
		}
		if err := receiving.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Accept offer", Exact: new(true)}).Click(); err != nil {
			t.Fatal(err)
		}
	}

	// Return the direct answer to complete the authenticated transport.
	answer := receiving.Locator("input[readonly]").First()
	if err := answer.WaitFor(); err != nil {
		t.Fatal(err)
	}
	response, err := answer.InputValue()
	if err != nil {
		t.Fatal(err)
	}
	if err := source.GetByPlaceholder("Paste answer payload here…").Fill(response); err != nil {
		t.Fatal(err)
	}
	if err := source.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Connect", Exact: new(true)}).Click(); err != nil {
		t.Fatal(err)
	}
}

// navigatePairingPage changes the hash in browser and app-scheme renderers,
// then waits for the route to commit without importing HTTP-only test assets.
func navigatePairingPage(t *testing.T, page playwright.Page, hash string) {
	t.Helper()
	if _, err := page.Evaluate(`async (hash) => {
		if (location.hash !== hash) {
			await new Promise((resolve) => {
				window.addEventListener('hashchange', resolve, { once: true })
				location.hash = hash
			})
		}
		await new Promise((resolve) => requestAnimationFrame(resolve))
		await new Promise((resolve) => requestAnimationFrame(resolve))
	}`, hash); err != nil {
		t.Fatalf("navigate pairing route %s: %v", hash, err)
	}
}

// TestPairingSignedInDriveJourney rejects an unapproved proposal, then merges
// two populated accounts through the signed-in entry point on the same stores.
func TestPairingSignedInDriveJourney(t *testing.T) {
	// Preserve routing and sync state when the exchange fails.
	h := harness(t)
	a, b := h.NewCleanSession(t), h.NewCleanSession(t)
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		for _, client := range []*TestSession{a, b} {
			if client.Page() != nil {
				body, err := client.Page().Locator("body").InnerText()
				t.Logf("signed-in pairing page %s: %s (read error: %v)", client.Page().URL(), body, err)
				state, err := client.Page().Evaluate(`async () => {
					const root = globalThis.__s4wave_debug.root
					const signal = AbortSignal.timeout(5000)
					const entries = (await root.listSessions(signal)).sessions ?? []
					const snapshots = []
					for (const entry of entries) {
						const { session } = await root.mountSessionByIdx({ sessionIdx: entry.sessionIndex }, signal)
						try {
							for await (const state of session.watchSyncStatus({}, signal)) {
								snapshots.push({ entry, state })
								break
							}
						} finally { session.release() }
					}
					return JSON.stringify(snapshots, (_, value) => typeof value === 'bigint' ? value.toString() : value)
				}`)
				t.Logf("signed-in pairing sync: %v (read error: %v)", state, err)
			}
		}
	})

	// Populate both accounts before choosing a merge destination.
	ctx, cancel := context.WithTimeout(h.Context(), 4*time.Minute)
	defer cancel()
	clients := []*TestSession{a, b}
	indices := make([]uint32, 0, 2)
	ids := make([]string, 0, 2)
	accounts := make([]string, 0, 2)
	files := []playwright.InputFile{
		{Name: "offered-account.md", MimeType: "text/markdown", Buffer: []byte("This file belongs to the selected destination.\n")},
		{Name: "receiving-account.md", MimeType: "text/markdown", Buffer: []byte("This file follows the merged source account.\n")},
	}
	for i, client := range clients {
		drive := CreateDriveScenario(t, h, client)
		WaitForDriveReady(t, h, client.Page())
		uploadDriveFileThroughUI(t, client.Page(), files[i])
		waitForDriveEntry(t, client.Page(), files[i].Name)
		mounted, err := client.MountSessionByIdx(ctx, drive.GetSessionIndex())
		if err != nil {
			t.Fatal(err)
		}
		info, err := mounted.GetSessionInfo(ctx)
		mounted.Release()
		if err != nil {
			t.Fatal(err)
		}
		indices = append(indices, drive.GetSessionIndex())
		ids = append(ids, drive.GetSpaceID())
		accounts = append(accounts, info.GetSessionRef().GetProviderResourceRef().GetProviderAccountId())
	}

	// Refuse the first proof and approve the second on the same persisted accounts.
	for attempt := range 2 {
		startPairingPages(t, a.Page(), b.Page(), indices[0], fmt.Sprintf("#/u/%d/setup/link-device", indices[1]))
		if err := b.Page().GetByRole("radio").Nth(2).Check(); err != nil {
			t.Fatal(err)
		}
		if err := b.Page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "Review and verify", Exact: new(true)}).Click(); err != nil {
			t.Fatal(err)
		}
		if attempt != 0 {
			confirmPairingPages(t, a.Page(), b.Page())
			break
		}
		if err := b.Page().GetByRole("button", playwright.PageGetByRoleOptions{Name: "No, abort", Exact: new(true)}).Click(); err != nil {
			t.Fatal(err)
		}
		if err := a.Page().GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Pairing failed", Exact: new(true)}).WaitFor(); err != nil {
			body, readErr := a.Page().Locator("body").InnerText()
			t.Fatalf("rejected pairing: %v; page: %s (read error: %v)", err, body, readErr)
		}
		for i, client := range clients {
			entries, err := client.Root().ListSessions(ctx)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if entry.GetSessionRef().GetProviderResourceRef().GetProviderAccountId() != accounts[i] {
					t.Fatal("rejected pairing granted access to the other account")
				}
			}
		}
	}

	// Both clients must retain and display files from both original accounts.
	for _, client := range clients {
		if err := client.Page().GetByRole("heading", playwright.PageGetByRoleOptions{Name: "Account connected", Exact: new(true)}).WaitFor(); err != nil {
			t.Fatal(err)
		}
		index := waitPairingPageCopy(t, client.Page(), accounts[0], ids...)
		for i, id := range ids {
			NavigateHash(t, h, client.Page(), fmt.Sprintf("#/u/%d/so/%s", index, id))
			WaitForDriveReady(t, h, client.Page())
			openDriveEntry(t, client.Page(), files[i].Name)
			waitForUnixFSFileText(t, client.Page(), "merged account file", string(files[i].Buffer))
		}
	}

	// Reopen the merging client after the offered client leaves.
	a.release()
	if _, err := b.Page().Reload(); err != nil {
		t.Fatal(err)
	}
	waitForUnixFSFileText(t, b.Page(), "merged file after offline reload", string(files[1].Buffer))
}

// confirmPairingPages compares both displayed proofs before authorizing access.
func confirmPairingPages(t *testing.T, source, receiving playwright.Page) {
	t.Helper()

	// Wait until both participants can inspect the shared proof.
	for _, page := range []playwright.Page{source, receiving} {
		if err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Yes, they match", Exact: new(true)}).WaitFor(); err != nil {
			body, readErr := page.Locator("body").InnerText()
			t.Fatalf("pairing verification: %v; page %s: %s (read error: %v)", err, page.URL(), body, readErr)
		}
	}

	// Authorize access only after comparing the proofs displayed by both clients.
	emojiA, err := source.GetByLabel("Verification emoji").InnerText()
	if err != nil {
		t.Fatal(err)
	}
	emojiB, err := receiving.GetByLabel("Verification emoji").InnerText()
	if err != nil || emojiA != emojiB {
		t.Fatalf("verification differs: %q / %q: %v", emojiA, emojiB, err)
	}
	for _, page := range []playwright.Page{source, receiving} {
		if err := page.GetByRole("button", playwright.PageGetByRoleOptions{Name: "Yes, they match", Exact: new(true)}).Click(); err != nil {
			t.Fatal(err)
		}
	}
}

//go:build !skip_e2e && !js

package releasewasm

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
)

func TestBrowserReleaseDescriptorIncludesPrerenderedShell(t *testing.T) {
	// Read the production browser release descriptor.
	desc, err := testHarness.browserRelease(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	// Verify the release schema and generation identity.
	if desc.SchemaVersion != 1 {
		t.Fatalf("expected schema version 1, got %d", desc.SchemaVersion)
	}
	if desc.GenerationID == "" {
		t.Fatal("expected generation id")
	}

	// Verify the release shell includes its entrypoint and workers.
	if desc.ShellAssets.Entrypoint == "" {
		t.Fatal("expected shellAssets.entrypoint")
	}
	if desc.ShellAssets.ServiceWorker == "" {
		t.Fatal("expected shellAssets.serviceWorker")
	}
	if desc.ShellAssets.SharedWorker == "" {
		t.Fatal("expected shellAssets.sharedWorker")
	}

	// Verify the release includes the root and Drive prerendered routes.
	if !slices.Contains(desc.PrerenderedRoutes, "/") {
		t.Fatalf("expected / in prerendered routes: %v", desc.PrerenderedRoutes)
	}
	if !slices.Contains(desc.PrerenderedRoutes, "/quickstart/drive") {
		t.Fatalf("expected /quickstart/drive in prerendered routes: %v", desc.PrerenderedRoutes)
	}
}

func TestQuickstartShellTabsComposedBrowserProof(t *testing.T) {
	// Seed an obsolete Shell snapshot into the first document.
	pageA := testHarness.newDedicatedWorkerPage(t)
	ctx := pageA.Context()
	legacyShellState := `() => {
		sessionStorage.setItem('shell-tabs-state', JSON.stringify({
			tabs: [{ id: 'legacy-tab', path: '/legacy', name: 'Legacy' }],
			activeTabId: 'legacy-tab',
		}))
	}`
	if err := pageA.AddInitScript(playwright.Script{Content: &legacyShellState}); err != nil {
		t.Fatalf("seed explicit old Shell snapshot reset case: %v", err)
	}

	// Boot the first document as the dedicated-worker host.
	quickstartURL := testHarness.getBaseURL() + "/quickstart/drive"
	openQuickstartReleasePage(t, pageA, quickstartURL)
	assertRuntimeWorkerMode(t, pageA, "dedicated-worker")
	hostGeneration, hostDocumentID := assertDedicatedWorkerHost(t, pageA)
	assertWarmPresentation(t, pageA, hostGeneration, hostDocumentID, false)

	// Check the obsolete snapshot was reset, not imported.
	waitForShellRecordCount(t, pageA, 1)
	initialSnapshot := readBrowserShellTabsSnapshot(t, pageA)
	if len(initialSnapshot.Records) != 1 {
		t.Fatalf("old Shell state was not cleanly initialized: %#v", initialSnapshot)
	}
	if initialSnapshot.Records[0].ID == "legacy-tab" || initialSnapshot.Records[0].Path == "/legacy" {
		t.Fatalf("legacy Shell record was imported instead of reset: %#v", initialSnapshot.Records[0])
	}

	// Check the obsolete storage key was removed.
	legacyAfterInit, err := pageA.Evaluate(`() => sessionStorage.getItem('shell-tabs-state')`)
	if err != nil {
		t.Fatalf("read obsolete Shell snapshot after clean initialization: %v", err)
	}
	if legacyAfterInit != nil {
		t.Fatalf("obsolete shell-tabs-state survived clean initialization: %#v", legacyAfterInit)
	}
	firstURL := pageA.URL()

	// Attach a second document to the warm host.
	pageB := testHarness.newPageInContext(t, ctx)
	openQuickstartReleasePage(t, pageB, quickstartURL)
	assertRuntimeWorkerMode(t, pageB, "dedicated-worker")
	assertWarmPresentation(t, pageB, hostGeneration, hostDocumentID, true)
	waitForShellRecordCount(t, pageA, 2)
	waitForShellRecordCount(t, pageB, 2)
	logWarmAttachCorrectnessMetrics(t, pageB, hostGeneration)

	// Check the second document added one shared record without taking the first URL.
	afterB := readBrowserShellTabsSnapshot(t, pageB)
	if len(afterB.Records) != 2 {
		t.Fatalf("fresh second document did not create exactly one shared record: %#v", afterB)
	}
	secondRecordID := findNewBrowserShellRecord(initialSnapshot, afterB)
	if secondRecordID == "" {
		t.Fatalf("fresh second document did not add one new record: before=%#v after=%#v", initialSnapshot, afterB)
	}
	if pageA.URL() != firstURL {
		dumpPageState(t, pageA)
		dumpPageState(t, pageB)
		t.Fatalf("second document stole first document URL: got %s want %s", pageA.URL(), firstURL)
	}
	assertNoRuntimeWorkerCreated(t, pageB)

	// Move the shared record to /docs from the second document.
	logShellDiagnostic(t, pageA, "before_docs_hash_page_a")
	logShellDiagnostic(t, pageB, "before_docs_hash_page_b")
	setShellHash(t, pageB, "#/docs")
	logShellDiagnostic(t, pageB, "after_docs_hash_page_b")
	waitForBrowserShellRecordPath(t, pageB, secondRecordID, "/docs")
	if pageA.URL() != firstURL {
		t.Fatalf("inactive shared path update changed first document hash: got %s want %s", pageA.URL(), firstURL)
	}

	// Rename the shared record and check both fields converge.
	renameActiveShellTab(t, pageB, "Shared Docs")
	waitForShellLabel(t, pageA, "Shared Docs")
	sharedSnapshot := readBrowserShellTabsSnapshot(t, pageA)
	sharedRecord := findBrowserShellRecord(sharedSnapshot, secondRecordID)
	if sharedRecord == nil || sharedRecord.Path != "/docs" || sharedRecord.CustomName != "Shared Docs" {
		t.Fatalf("shared path/name/customName did not converge: %#v", sharedSnapshot)
	}

	// Create one tab in each document at once.
	beforeConcurrentA := readComposedShellProjection(t, pageA)
	beforeConcurrentB := readComposedShellProjection(t, pageB)
	concurrentCreateShellTabs(t, pageA, pageB)
	waitForShellRecordCount(t, pageA, 4)
	waitForShellRecordCount(t, pageB, 4)

	// Check both documents hold the same four records.
	concurrentSnapshot := readBrowserShellTabsSnapshot(t, pageA)
	if len(concurrentSnapshot.Records) != 4 {
		t.Fatalf("concurrent Shell creation lost a record: %#v", concurrentSnapshot)
	}
	if !concurrentSnapshot.sameRecordIDs(readBrowserShellTabsSnapshot(t, pageB)) {
		t.Fatalf("A/B shared record inventories diverged after concurrent creation")
	}

	// Route each document's new tab to its own static page.
	waitForBrowserShellActiveRecordChange(t, pageA, beforeConcurrentA.ActiveTabID)
	waitForBrowserShellActiveRecordChange(t, pageB, beforeConcurrentB.ActiveTabID)
	setShellHash(t, pageA, "#/pricing")
	setShellHash(t, pageB, "#/licenses")
	waitForBrowserShellActivePath(t, pageA, "/pricing")
	waitForBrowserShellActivePath(t, pageB, "/licenses")
	if pageA.URL() == pageB.URL() {
		t.Fatalf("A/B active selection and hash are not independent: A=%s B=%s", pageA.URL(), pageB.URL())
	}

	// Record page A's settled projection.
	waitForShellLabel(t, pageA, "Shared Docs")
	waitForShellLabel(t, pageB, "Shared Docs")
	independentProjectionA := readComposedShellProjection(t, pageA)

	// Pop out the shared record with its retained Shell Tab ID.
	selectShellTabByText(t, pageB, "Shared Docs")
	retainedURL := ""
	popup, err := ctx.ExpectPage(func() error {
		return pageB.Locator("button[title='Open in new tab']").First().Click()
	})
	if err != nil {
		t.Fatalf("open retained-ID Shell popout: %v", err)
	}
	if _, err := popup.WaitForFunction(`() => location.href !== 'about:blank'`, nil, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		t.Fatalf("wait for retained-ID popup navigation: %v", err)
	}

	// Check the popout kept the retained ID and the shared record.
	openQuickstartReleasePage(t, popup, popup.URL())
	retainedURL = popup.URL()
	if !strings.Contains(retainedURL, "shellTabId="+secondRecordID) {
		t.Fatalf("popout URL lost retained Shell Tab ID: %s", retainedURL)
	}
	waitForShellRecordCount(t, popup, 4)
	waitForShellLabel(t, popup, "Shared Docs")

	// Open the retained-ID URL in a copied document.
	copied := testHarness.newPageInContext(t, ctx)
	if _, err := copied.Goto(retainedURL); err != nil {
		t.Fatalf("goto copied retained-ID URL: %v", err)
	}
	openQuickstartReleasePage(t, copied, retainedURL)
	waitForShellRecordCount(t, copied, 4)
	waitForShellLabel(t, copied, "Shared Docs")

	// Reload the copied document and check it keeps the shared record.
	if _, err := copied.Reload(); err != nil {
		t.Fatalf("reload copied retained-ID URL: %v", err)
	}
	waitForPrerenderRootOrLiveApp(t, copied)
	waitForLiveApp(t, copied)
	waitForShellTabButtons(t, copied)
	waitForShellRecordCount(t, copied, 4)
	waitForShellLabel(t, copied, "Shared Docs")

	// Check the retained-ID transitions left page A unchanged.
	beforeCloseA := readComposedShellProjection(t, pageA)
	assertSameComposedShellProjection(
		t,
		beforeCloseA,
		independentProjectionA,
		"retained-ID transitions changed page A",
	)

	// Close the shared record from page B while page A shows another tab.
	closeShellTabByText(t, pageB, "Shared Docs")
	waitForShellRecordCount(t, pageA, 3)
	waitForShellRecordCount(t, pageB, 3)
	afterCloseA := readComposedShellProjection(t, pageA)
	assertInactiveClosePreservedProjection(t, beforeCloseA, afterCloseA, "Shared Docs")
	assertShellLabelAbsent(t, pageA, "Shared Docs")
	assertShellLabelAbsent(t, pageB, "Shared Docs")

	// Open the retained URL after its record was removed.
	removedIDURL := retainedURL
	removed := testHarness.newPageInContext(t, ctx)
	if _, err := removed.Goto(removedIDURL); err != nil {
		t.Fatalf("goto removed-ID URL: %v", err)
	}
	openQuickstartReleasePage(t, removed, removedIDURL)

	// Check the closed record stays closed and a fresh /docs record replaces it.
	waitForShellRecordCount(t, removed, 4)
	removedSnapshot := readBrowserShellTabsSnapshot(t, removed)
	if findBrowserShellRecord(removedSnapshot, secondRecordID) != nil {
		t.Fatalf("removed-ID handoff resurrected the closed record: %#v", removedSnapshot)
	}
	if removedSnapshot.Records[len(removedSnapshot.Records)-1].Path != "/docs" {
		t.Fatalf("removed-ID fallback did not create a fresh /docs record: %#v", removedSnapshot)
	}

	// Open a malformed-ID URL, which falls back to a new record.
	invalidURL := strings.Replace(retainedURL, "shellTabId="+secondRecordID, "shellTabId=!malformed", 1)
	invalid := testHarness.newPageInContext(t, ctx)
	if _, err := invalid.Goto(invalidURL); err != nil {
		t.Fatalf("goto malformed-ID URL: %v", err)
	}
	openQuickstartReleasePage(t, invalid, invalidURL)
	waitForShellRecordCount(t, invalid, 5)
	invalidHash := invalid.URL()

	// Check a reload keeps the fallback URL stable.
	if _, err := invalid.Reload(); err != nil {
		t.Fatalf("reload malformed-ID fallback: %v", err)
	}
	waitForPrerenderRootOrLiveApp(t, invalid)
	waitForLiveApp(t, invalid)
	waitForShellTabButtons(t, invalid)
	waitForShellRecordCount(t, invalid, 5)
	if invalid.URL() != invalidHash {
		t.Fatalf("malformed-ID fallback reload changed stable URL: got %s want %s", invalid.URL(), invalidHash)
	}

	// Close the retained-ID proof documents.
	for _, page := range []playwright.Page{popup, copied, removed, invalid} {
		if err := page.Close(); err != nil {
			t.Fatalf("close completed retained-ID proof page: %v", err)
		}
	}

	// Open a document attached to the elected host.
	pageAttached := testHarness.newPageInContext(t, ctx)
	if _, err := pageAttached.Goto(retainedURL); err != nil {
		t.Fatalf("goto retained URL for host-loss proof: %v", err)
	}
	waitForPrerenderRootOrLiveApp(t, pageAttached)
	waitForBootFunction(t, pageAttached)
	waitForLiveApp(t, pageAttached)
	waitForStartupMark(t, pageAttached, "dedicated-host.attach-open-ready")

	// Close the host and check the survivor is promoted.
	if err := pageA.Close(); err != nil {
		t.Fatalf("close elected host document: %v", err)
	}
	promotionGeneration := assertWarmPromotion(t, pageB, hostGeneration)
	assertWarmPresentation(t, pageB, promotionGeneration, "", false)

	// Check the attached document rejoins the promoted host without a cold boot
	// and keeps the Shell inventory.
	assertReattachedWithoutColdBoot(t, pageAttached, promotionGeneration)
	waitForShellRecordCount(t, pageAttached, 6)
	attachedSnapshot := readBrowserShellTabsSnapshot(t, pageAttached)
	survivorSnapshot := readBrowserShellTabsSnapshot(t, pageB)
	if !survivorSnapshot.sameRecordIDs(attachedSnapshot) {
		t.Fatalf("Shell inventory did not survive host loss/promotion: attached=%#v survivor=%#v", attachedSnapshot, survivorSnapshot)
	}

	// Check the promoted survivor is usable.
	if err := pageB.BringToFront(); err != nil {
		t.Fatalf("bring survivor document to front: %v", err)
	}
	if err := waitForDriveFrameReady(pageB, browserWaitMS); err != nil {
		t.Fatalf("wait for promoted survivor usability: %v", err)
	}

	// Close every document to reach zero documents.
	preservedSnapshot := readBrowserShellTabsSnapshot(t, pageB)
	for _, page := range ctx.Pages() {
		if page.IsClosed() {
			continue
		}
		if err := page.Close(); err != nil {
			t.Fatalf("close document before zero-document retention check: %v", err)
		}
	}

	// Reopen and check the retained records are unchanged plus one new route record.
	reopen := testHarness.newPageInContext(t, ctx)
	openQuickstartReleasePage(t, reopen, quickstartURL)
	waitForShellRecordCount(t, reopen, len(preservedSnapshot.Records)+1)
	reopenedSnapshot := readBrowserShellTabsSnapshot(t, reopen)
	if findNewBrowserShellRecord(preservedSnapshot, reopenedSnapshot) == "" {
		t.Fatalf("fresh zero-document reopen did not add exactly one route record: before=%#v after=%#v", preservedSnapshot, reopenedSnapshot)
	}
	if !sameBrowserShellRecordValues(preservedSnapshot, reopenedSnapshot) {
		t.Fatalf("zero-document reopen changed existing Shell record fields: before=%#v after=%#v", preservedSnapshot, reopenedSnapshot)
	}

	// Reset Shell tabs and check the epoch advances to one record.
	beforeReset := reopenedSnapshot
	resetShellTabsVisibly(t, reopen)
	waitForShellRecordCount(t, reopen, 1)
	afterReset := readBrowserShellTabsSnapshot(t, reopen)
	if afterReset.Epoch <= beforeReset.Epoch || len(afterReset.Records) != 1 {
		t.Fatalf("visible Shell reset did not advance epoch and replace inventory: before=%#v after=%#v", beforeReset, afterReset)
	}
	waitForShellTabButtons(t, reopen)
}

func TestQuickstartShellTabsPersistentProfileRestart(t *testing.T) {
	// Require Chromium for the persistent-profile restart proof.
	if testHarness.browserName != "chromium" {
		t.Skip("persistent-profile release proof requires Chromium")
	}

	// Open the first persistent browser context with cleanup.
	userDataDir := t.TempDir()
	firstContext := testHarness.newPersistentBrowserContext(t, userDataDir)
	firstContextClosed := false
	t.Cleanup(func() {

		// Leave an explicitly closed persistent context alone during cleanup.
		if firstContextClosed {
			return
		}

		// Close the first persistent context if the test exits early.
		if err := firstContext.Close(); err != nil {
			t.Logf("close first persistent browser context during cleanup: %v", err)
		}
	})

	// Create a second Shell record in the persistent profile.
	firstPage := testHarness.newPageInContext(t, firstContext)
	quickstartURL := testHarness.getBaseURL() + "/quickstart/drive"
	openQuickstartReleasePage(t, firstPage, quickstartURL)
	waitForShellRecordCount(t, firstPage, 1)
	if err := firstPage.Locator("button[title='New tab']").First().Click(); err != nil {
		t.Fatalf("create persistent-profile Shell record: %v", err)
	}

	// Verify both Shell records exist before restarting the browser.
	waitForShellRecordCount(t, firstPage, 2)
	beforeRestart := readBrowserShellTabsSnapshot(t, firstPage)
	if len(beforeRestart.Records) != 2 {
		t.Fatalf("persistent-profile setup did not create two records: %#v", beforeRestart)
	}

	// Terminate the first browser context while preserving its profile.
	if err := firstContext.Close(); err != nil {
		t.Fatalf("close persistent browser context for restart: %v", err)
	}
	firstContextClosed = true

	// Reopen the retained browser profile with cleanup.
	secondContext := testHarness.newPersistentBrowserContext(t, userDataDir)
	t.Cleanup(func() {
		if err := secondContext.Close(); err != nil {
			t.Logf("close relaunched persistent browser context: %v", err)
		}
	})

	// Verify browser restart preserves the Shell records and adds a fresh entry.
	secondPage := testHarness.newPageInContext(t, secondContext)
	openQuickstartReleasePage(t, secondPage, quickstartURL)
	waitForShellRecordCount(t, secondPage, len(beforeRestart.Records)+1)
	afterRestart := readBrowserShellTabsSnapshot(t, secondPage)
	if findNewBrowserShellRecord(beforeRestart, afterRestart) == "" ||
		!sameBrowserShellRecordValues(beforeRestart, afterRestart) {
		t.Fatalf("persistent-profile browser restart did not preserve records while creating fresh entry: before=%#v after=%#v", beforeRestart, afterRestart)
	}
	waitForShellLabel(t, secondPage, beforeRestart.Records[0].Name)

	// Reset the persistent Shell inventory and verify its epoch advances.
	beforeReset := afterRestart
	resetShellTabsVisibly(t, secondPage)
	waitForShellRecordCount(t, secondPage, 1)
	afterReset := readBrowserShellTabsSnapshot(t, secondPage)
	if afterReset.Epoch <= beforeReset.Epoch || len(afterReset.Records) != 1 {
		t.Fatalf("persistent-profile visible reset did not advance epoch: before=%#v after=%#v", beforeReset, afterReset)
	}
	waitForShellTabButtons(t, secondPage)
}

type composedShellRecord struct {
	ID               string
	Path             string
	Name             string
	CustomName       string
	CreationSequence float64
}

type composedShellSnapshot struct {
	SchemaVersion float64
	Epoch         float64
	Revision      float64
	Records       []composedShellRecord
}

type composedShellProjection struct {
	URL            string
	Hash           string
	ActiveTabID    string
	VisiblePanelID string
	Labels         []string
}

func logShellDiagnostic(t *testing.T, page playwright.Page, label string) shellDiagnosticRecord {
	// Collect the document's Shell routes, storage and visible panel.
	t.Helper()
	raw, err := page.Evaluate(`() => {
		const readStorage = (storage, key) => {
			const raw = storage.getItem(key)
			return { present: raw !== null, raw }
		}
		const visiblePanel = [...document.querySelectorAll('[data-tab-id]')].find((element) => {
			const style = getComputedStyle(element)
			const rect = element.getBoundingClientRect()
			return style.display !== 'none' &&
				style.visibility !== 'hidden' &&
				rect.width > 0 &&
				rect.height > 0
		})
		return {
			locationHref: location.href,
			locationHash: location.hash,
			sessionDocumentState: readStorage(sessionStorage, 'shell-document-state'),
			browserShellTabs: readStorage(localStorage, 'browser-shell-tabs'),
			visibleActivePanelID: visiblePanel?.getAttribute('data-tab-id') ?? '',
		}
	}`)
	if err != nil {
		t.Fatalf("shell_diagnostic label=%s collection_error=%v", label, err)
	}

	// Decode the Shell diagnostic record.
	record, err := decodeShellDiagnosticRecord(raw)
	if err != nil {
		t.Fatalf("shell_diagnostic label=%s decode_error=%v payload_type=%T", label, err, raw)
	}

	// Report the Shell diagnostic record with its collection label.
	t.Logf(
		"shell_diagnostic label=%s location_href=%q location_hash=%q "+
			"session_storage_shell_document_state_present=%t "+
			"session_storage_shell_document_state_raw=%q "+
			"local_storage_browser_shell_tabs_present=%t "+
			"local_storage_browser_shell_tabs_raw=%q "+
			"visible_active_panel_id=%q",
		label,
		record.LocationHref,
		record.LocationHash,
		record.SessionDocumentState.Present,
		record.SessionDocumentState.Raw,
		record.BrowserShellTabs.Present,
		record.BrowserShellTabs.Raw,
		record.VisibleActivePanelID,
	)
	return record
}

func decodeShellDiagnosticRecord(raw any) (shellDiagnosticRecord, error) {
	// Require an object payload for the Shell diagnostic record.
	value, ok := raw.(map[string]any)
	if !ok {
		return shellDiagnosticRecord{}, errors.Errorf("expected object, got %T", raw)
	}

	// Decode the Shell document's location fields.
	locationHref, err := requiredShellDiagnosticString(value, "locationHref", true)
	if err != nil {
		return shellDiagnosticRecord{}, err
	}
	locationHash, err := requiredShellDiagnosticString(value, "locationHash", false)
	if err != nil {
		return shellDiagnosticRecord{}, err
	}

	// Decode the document and shared Shell storage records.
	sessionDocumentState, err := decodeShellDiagnosticStorage(value, "sessionDocumentState")
	if err != nil {
		return shellDiagnosticRecord{}, err
	}
	browserShellTabs, err := decodeShellDiagnosticStorage(value, "browserShellTabs")
	if err != nil {
		return shellDiagnosticRecord{}, err
	}

	// Decode the Shell document's visible active panel identity.
	visibleActivePanelID, err := requiredShellDiagnosticString(value, "visibleActivePanelID", false)
	if err != nil {
		return shellDiagnosticRecord{}, err
	}
	return shellDiagnosticRecord{
		LocationHref:         locationHref,
		LocationHash:         locationHash,
		SessionDocumentState: sessionDocumentState,
		BrowserShellTabs:     browserShellTabs,
		VisibleActivePanelID: visibleActivePanelID,
	}, nil
}

func requiredShellDiagnosticString(value map[string]any, field string, nonEmpty bool) (string, error) {
	// Require the requested Shell diagnostic field to exist.
	raw, ok := value[field]
	if !ok {
		return "", errors.Errorf("missing %s", field)
	}

	// Require a string value for the Shell diagnostic field.
	text, ok := raw.(string)
	if !ok {
		return "", errors.Errorf("%s has type %T, want string", field, raw)
	}

	// Enforce the Shell diagnostic field's nonempty contract.
	if nonEmpty && text == "" {
		return "", errors.Errorf("%s is empty", field)
	}
	return text, nil
}

func decodeShellDiagnosticStorage(value map[string]any, field string) (shellDiagnosticStorage, error) {
	// Require the Shell storage diagnostic field to be an object.
	raw, ok := value[field]
	if !ok {
		return shellDiagnosticStorage{}, errors.Errorf("missing %s", field)
	}
	storage, ok := raw.(map[string]any)
	if !ok {
		return shellDiagnosticStorage{}, errors.Errorf("%s has type %T, want object", field, raw)
	}

	// Decode the storage presence flag and raw snapshot value.
	present, ok := storage["present"].(bool)
	if !ok {
		return shellDiagnosticStorage{}, errors.Errorf("%s.present has type %T, want bool", field, storage["present"])
	}
	rawValue, ok := storage["raw"]
	if !ok {
		return shellDiagnosticStorage{}, errors.Errorf("missing %s.raw", field)
	}

	// Verify a null storage value agrees with the presence flag.
	if rawValue == nil {
		if present {
			return shellDiagnosticStorage{}, errors.Errorf("%s.raw is null while present", field)
		}
		return shellDiagnosticStorage{}, nil
	}

	// Verify a non-null storage value is a present string snapshot.
	text, ok := rawValue.(string)
	if !ok {
		return shellDiagnosticStorage{}, errors.Errorf("%s.raw has type %T, want string or null", field, rawValue)
	}
	if !present {
		return shellDiagnosticStorage{}, errors.Errorf("%s.raw is non-null while absent", field)
	}
	return shellDiagnosticStorage{Present: true, Raw: text}, nil
}

type composedStartupMark struct {
	Label    string
	Sequence float64
	Detail   map[string]any
}

func openQuickstartReleasePage(t *testing.T, page playwright.Page, targetURL string) {
	// Open the requested release page and wait for the live application.
	t.Helper()
	if _, err := page.Goto(targetURL); err != nil {
		t.Fatalf("goto composed Shell page %s: %v", targetURL, err)
	}
	waitForPrerenderRootOrLiveApp(t, page)
	waitForBootFunction(t, page)
	waitForLiveApp(t, page)

	// Wait for the Drive frame when the target is a Drive quickstart.
	if strings.Contains(targetURL, "/quickstart/drive") ||
		strings.Contains(targetURL, "#/quickstart/drive") {
		waitForQuickstartAppRoute(t, page)
		completeQuickstartDriveIntroIfPresent(t, page)
		if err := waitForDriveFrameReady(page, browserWaitMS); err != nil {
			dumpPageState(t, page)
			t.Fatalf("wait for composed quickstart frame: %v", err)
		}
		return
	}

	// Wait for Shell tabs on other release routes.
	waitForShellTabButtons(t, page)
}

func readBrowserShellTabsSnapshot(t *testing.T, page playwright.Page) composedShellSnapshot {
	// Read the persisted browser Shell Tabs snapshot.
	t.Helper()
	raw, err := page.Evaluate(`() => {
		const value = localStorage.getItem('browser-shell-tabs')
		return value ? JSON.parse(value) : null
	}`)
	if err != nil {
		t.Fatalf("read browser Shell Tabs snapshot: %v", err)
	}

	// Decode the Shell snapshot's metadata and record inventory.
	value, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("browser Shell Tabs snapshot missing or invalid: %#v", raw)
	}
	snapshot := composedShellSnapshot{
		SchemaVersion: composedNumber(value["schemaVersion"]),
		Epoch:         composedNumber(value["epoch"]),
		Revision:      composedNumber(value["revision"]),
	}
	rawRecords, ok := value["records"].([]any)
	if !ok {
		t.Fatalf("browser Shell Tabs records missing or invalid: %#v", value)
	}

	// Decode each stored Shell record into the snapshot.
	snapshot.Records = make([]composedShellRecord, 0, len(rawRecords))
	for _, rawRecord := range rawRecords {

		// Require each Shell record to be an object.
		record, ok := rawRecord.(map[string]any)
		if !ok {
			t.Fatalf("browser Shell Tabs record invalid: %#v", rawRecord)
		}

		// Append the Shell record's identity and presentation fields.
		snapshot.Records = append(snapshot.Records, composedShellRecord{
			ID:               composedString(record["id"]),
			Path:             composedString(record["path"]),
			Name:             composedString(record["name"]),
			CustomName:       composedString(record["customName"]),
			CreationSequence: composedNumber(record["creationSequence"]),
		})
	}

	// Verify the browser Shell snapshot uses the expected schema.
	if snapshot.SchemaVersion != 1 {
		t.Fatalf("browser Shell Tabs schema version=%v want 1: %#v", snapshot.SchemaVersion, snapshot)
	}
	return snapshot
}

func composedNumber(value any) float64 {
	switch typed := value.(type) {
	case float64:
		return typed
	case int:
		return float64(typed)
	default:
		return 0
	}
}

func composedString(value any) string {
	text, _ := value.(string)
	return text
}

func findNewBrowserShellRecord(before, after composedShellSnapshot) string {
	// Index the Shell record IDs present before the operation.
	beforeIDs := make(map[string]bool, len(before.Records))
	for _, record := range before.Records {
		beforeIDs[record.ID] = true
	}

	// Identify the sole added Shell record.
	var added string
	for _, record := range after.Records {
		if !beforeIDs[record.ID] {
			if added != "" {
				return ""
			}
			added = record.ID
		}
	}

	// Verify the Shell inventory grew by exactly one record.
	if len(after.Records) != len(before.Records)+1 {
		return ""
	}
	return added
}

func findBrowserShellRecord(snapshot composedShellSnapshot, id string) *composedShellRecord {
	for index := range snapshot.Records {
		if snapshot.Records[index].ID == id {
			return &snapshot.Records[index]
		}
	}
	return nil
}

// sameRecordIDs compares the complete identity set without copying the records.
func (snapshot composedShellSnapshot) sameRecordIDs(other composedShellSnapshot) bool {
	// Require equal cardinality before indexing the original identities.
	if len(snapshot.Records) != len(other.Records) {
		return false
	}
	ids := make(map[string]bool, len(snapshot.Records))
	for _, record := range snapshot.Records {
		ids[record.ID] = true
	}

	// Check that every identity in the other snapshot was present.
	for _, record := range other.Records {
		if !ids[record.ID] {
			return false
		}
	}
	return true
}

func sameBrowserShellRecordValues(before, after composedShellSnapshot) bool {
	// Index the updated Shell records by identity.
	afterByID := make(map[string]composedShellRecord, len(after.Records))
	for _, record := range after.Records {
		afterByID[record.ID] = record
	}

	// Verify every retained Shell record preserves its fields.
	for _, record := range before.Records {
		if afterRecord, ok := afterByID[record.ID]; !ok || afterRecord != record {
			return false
		}
	}
	return true
}

func waitForShellRecordCount(t *testing.T, page playwright.Page, count int) {
	// Wait for the persisted Shell inventory to reach the expected size.
	t.Helper()
	_, err := page.WaitForFunction(`(want) => {
		try {
			const value = localStorage.getItem('browser-shell-tabs')
			return value && JSON.parse(value).records?.length === want
		} catch {
			return false
		}
	}`, count, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for %d shared Shell records: %v", count, err)
	}
}

func waitForBrowserShellRecordPath(t *testing.T, page playwright.Page, id, path string) {
	// Record the Shell document before waiting for a shared path update.
	t.Helper()
	t.Logf("shell_record_path_wait phase=before target_id=%q target_path=%q", id, path)
	logShellDiagnostic(t, page, "browser_shell_record_path_wait_before")

	// Wait for the shared Shell record to reach its target path.
	_, err := page.WaitForFunction(`(args) => {
		try {
			const value = localStorage.getItem('browser-shell-tabs')
			const records = value ? JSON.parse(value).records : []
			return records.some((record) => record.id === args.id && record.path === args.path)
		} catch {
			return false
		}
	}`, map[string]any{"id": id, "path": path}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	})
	if err != nil {
		t.Logf("shell_record_path_wait phase=failure target_id=%q target_path=%q error=%v", id, path, err)
		logShellDiagnostic(t, page, "browser_shell_record_path_wait_failure")
		t.Fatalf("wait for shared Shell record %s path %s: %v", id, path, err)
	}

	// Record the Shell document after its shared path update.
	t.Logf("shell_record_path_wait phase=success target_id=%q target_path=%q", id, path)
	logShellDiagnostic(t, page, "browser_shell_record_path_wait_success")
}

func waitForBrowserShellActiveRecordChange(t *testing.T, page playwright.Page, previousID string) {
	// Wait for the newly created Shell record to become active.
	t.Helper()
	_, err := page.WaitForFunction(`(previousID) => {
		try {
			const documentState = JSON.parse(sessionStorage.getItem('shell-document-state') ?? 'null')
			const snapshot = JSON.parse(localStorage.getItem('browser-shell-tabs') ?? 'null')
			const active = snapshot?.records?.find((record) => record.id === documentState?.activeTabId)
			return active &&
				active.id !== previousID &&
				location.hash === '#' + active.path
		} catch {
			return false
		}
	}`, previousID, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for created Shell record selection after %s: %v", previousID, err)
	}
}

func waitForBrowserShellActivePath(t *testing.T, page playwright.Page, path string) {
	// Wait for the active Shell record and document hash to agree.
	t.Helper()
	_, err := page.WaitForFunction(`(want) => {
		try {
			const documentState = JSON.parse(sessionStorage.getItem('shell-document-state') ?? 'null')
			const snapshot = JSON.parse(localStorage.getItem('browser-shell-tabs') ?? 'null')
			const active = snapshot?.records?.find((record) => record.id === documentState?.activeTabId)
			return active?.path === want && location.hash.startsWith('#' + want)
		} catch {
			return false
		}
	}`, path, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for active Shell path %s: %v", path, err)
	}
}

func waitForShellTabButtons(t *testing.T, page playwright.Page) {
	// Wait for a visible Shell tab button.
	t.Helper()
	tabs := page.Locator(".flexlayout__tab_button:visible").First()
	if err := tabs.WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for visible Shell tab buttons: %v", err)
	}

	// Verify the visible Shell tab button accepts interaction.
	enabled, err := tabs.IsEnabled()
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("check visible Shell tab button interactivity: %v", err)
	}
	if !enabled {
		dumpPageState(t, page)
		t.Fatalf("visible Shell tab button is disabled")
	}
}

func setShellHash(t *testing.T, page playwright.Page, hash string) {
	t.Helper()

	if _, err := page.Evaluate(`(next) => {
		window.location.hash = next
		return window.location.hash
	}`, hash); err != nil {
		t.Fatalf("navigate visible Shell hash %s: %v", hash, err)
	}
}

func readComposedShellProjection(t *testing.T, page playwright.Page) composedShellProjection {
	// Read the document's visible Shell projection.
	t.Helper()
	raw, err := page.Evaluate(`() => {
		const documentState = JSON.parse(sessionStorage.getItem('shell-document-state') ?? 'null')
		const visiblePanel = [...document.querySelectorAll('[data-tab-id]')].find((element) => {
			const style = getComputedStyle(element)
			const rect = element.getBoundingClientRect()
			return style.display !== 'none' &&
				style.visibility !== 'hidden' &&
				rect.width > 0 &&
				rect.height > 0
		})
		return {
			url: location.href,
			hash: location.hash,
			activeTabId: documentState?.activeTabId ?? '',
			visiblePanelId: visiblePanel?.getAttribute('data-tab-id') ?? '',
			labels: [...document.querySelectorAll('.flexlayout__tab_button')].map((button) => button.textContent?.trim() || ''),
		}
	}`)
	if err != nil {
		t.Fatalf("read visible Shell projection: %v", err)
	}

	// Decode the Shell projection and its visible labels.
	value, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("visible Shell projection invalid: %#v", raw)
	}
	rawLabels, ok := value["labels"].([]any)
	if !ok {
		t.Fatalf("visible Shell labels invalid: %#v", value)
	}

	// Collect the visible Shell labels in their displayed order.
	labels := make([]string, 0, len(rawLabels))
	for _, rawLabel := range rawLabels {
		labels = append(labels, composedString(rawLabel))
	}
	return composedShellProjection{
		URL:            composedString(value["url"]),
		Hash:           composedString(value["hash"]),
		ActiveTabID:    composedString(value["activeTabId"]),
		VisiblePanelID: composedString(value["visiblePanelId"]),
		Labels:         labels,
	}
}

func assertSameComposedShellProjection(
	t *testing.T,
	got, want composedShellProjection,
	message string,
) {

	// Verify the document's Shell projection matches the expected state.
	t.Helper()
	if got.URL != want.URL ||
		got.Hash != want.Hash ||
		got.ActiveTabID != want.ActiveTabID ||
		got.VisiblePanelID != want.VisiblePanelID ||
		!slices.Equal(got.Labels, want.Labels) {
		t.Fatalf("%s: got=%#v want=%#v", message, got, want)
	}
}

func waitForShellLabel(t *testing.T, page playwright.Page, label string) {
	t.Helper()

	if err := page.Locator(".flexlayout__tab_button").Filter(playwright.LocatorFilterOptions{
		HasText: label,
	}).First().WaitFor(playwright.LocatorWaitForOptions{
		Timeout: playwright.Float(browserWaitMS),
	}); err != nil {
		t.Fatalf("wait for visible Shell label %q: %v", label, err)
	}
}

func assertShellLabelAbsent(t *testing.T, page playwright.Page, label string) {
	// Verify the removed Shell label is absent from the visible tabs.
	t.Helper()
	count, err := page.Locator(".flexlayout__tab_button").Filter(playwright.LocatorFilterOptions{
		HasText: label,
	}).Count()
	if err != nil {
		t.Fatalf("count visible Shell label %q: %v", label, err)
	}
	if count != 0 {
		t.Fatalf("removed Shell label %q remains visible: count=%d", label, count)
	}
}

func renameActiveShellTab(t *testing.T, page playwright.Page, customName string) {
	// Open the active Shell tab's rename editor.
	t.Helper()
	tabs := page.Locator(".flexlayout__tab_button")
	if err := tabs.Last().Dblclick(); err != nil {
		t.Fatalf("start visible Shell tab rename: %v", err)
	}

	// Enter and commit the Shell tab's custom name.
	input := tabs.Last().Locator("input:visible").First()
	if err := input.Fill(customName); err != nil {
		t.Fatalf("fill visible Shell custom name: %v", err)
	}
	if err := input.Press("Enter"); err != nil {
		t.Fatalf("commit visible Shell custom name: %v", err)
	}
}

func selectShellTabByText(t *testing.T, page playwright.Page, label string) {
	t.Helper()

	tab := page.Locator(".flexlayout__tab_button").Filter(playwright.LocatorFilterOptions{
		HasText: label,
	}).First()
	if err := tab.Click(); err != nil {
		t.Fatalf("select visible Shell tab %q: %v", label, err)
	}
}

func closeShellTabByText(t *testing.T, page playwright.Page, label string) {
	// Open the selected Shell tab's context menu.
	t.Helper()
	tab := page.Locator(".flexlayout__tab_button").Filter(playwright.LocatorFilterOptions{
		HasText: label,
	}).First()
	if err := tab.Click(playwright.LocatorClickOptions{Button: playwright.MouseButtonRight}); err != nil {
		t.Fatalf("open Shell tab context menu %q: %v", label, err)
	}

	// Invoke the context menu action that closes the shared tab.
	closeItem := page.Locator("[role='menuitem']:visible").Filter(playwright.LocatorFilterOptions{
		HasText: "Close Tab",
	}).First()
	if err := closeItem.Click(); err != nil {
		t.Fatalf("close shared Shell tab %q: %v", label, err)
	}
}

func concurrentCreateShellTabs(t *testing.T, pageA, pageB playwright.Page) {
	// Create Shell records concurrently in both documents.
	t.Helper()
	errs := make(chan error, 2)
	var waitGroup sync.WaitGroup
	for _, page := range []playwright.Page{pageA, pageB} {

		// Track the lifetime of each document's tab creation.
		waitGroup.Add(1)
		go func(page playwright.Page) {

			// Click the document's visible New tab button and report its result.
			defer waitGroup.Done()
			_, err := page.Evaluate(`() => {
				const button = [...document.querySelectorAll("button[title='New tab']")]
					.find((candidate) => candidate instanceof HTMLElement && candidate.offsetParent !== null)
				if (!button) throw new Error('visible New tab button is missing')
				button.click()
				return true
			}`)
			errs <- err
		}(page)
	}

	// Wait for both Shell tab creations and report their failures.
	waitGroup.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent visible Shell record creation: %v", err)
		}
	}
}

// resetShellTabsVisibly resets the Shell tabs through the View menu of a
// quickstart Drive page.
func resetShellTabsVisibly(t *testing.T, page playwright.Page) {
	// Report visible Shell reset failures at the caller.
	t.Helper()

	// Wait for the quickstart to settle on the Drive file browser. Its route
	// change closes an open menu, detaching the item before the click.
	if err := waitForDriveFrameReady(page, browserWaitMS); err != nil {
		t.Fatalf("wait for Drive file browser before Shell reset: %v", err)
	}

	// Open the View menu and invoke the reset. A late route or Shell render can
	// still close the menu and detach the item mid-click. A missed click leaves
	// the inventory unchanged, so close the menu and try again.
	view := page.Locator("button:visible:has-text('View')").First()
	reset := page.Locator("[role='menuitem']:visible:has-text('Reset Shell Tabs')").First()
	var step string
	var lastErr error
	for attempt := 1; attempt <= 3; attempt++ {
		if err := view.Click(playwright.LocatorClickOptions{
			Timeout: playwright.Float(10000),
		}); err != nil {
			step, lastErr = "open visible View menu", err
		} else if err := reset.Click(playwright.LocatorClickOptions{
			Timeout: playwright.Float(10000),
		}); err != nil {
			step, lastErr = "invoke visible Shell reset", err
		} else {
			return
		}
		t.Logf("Shell reset attempt %d: %s: %v", attempt, step, lastErr)
		if err := page.Keyboard().Press("Escape"); err != nil {
			t.Logf("close View menu after failed Shell reset: %v", err)
		}
	}
	t.Fatalf("%s: %v", step, lastErr)
}

func readComposedStartupMarks(t *testing.T, page playwright.Page) []composedStartupMark {
	// Read the production document's startup marks.
	t.Helper()
	raw, err := page.Evaluate(`() => (globalThis.__swStartupMarks ?? []).map((mark) => ({
		label: mark.label,
		sequence: mark.sequence,
		detail: mark.detail ?? {},
	}))`)
	if err != nil {
		t.Fatalf("read production startup marks: %v", err)
	}

	// Decode the startup mark inventory.
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("production startup marks invalid: %#v", raw)
	}

	// Collect valid startup marks and their details.
	marks := make([]composedStartupMark, 0, len(items))
	for _, item := range items {

		// Ignore startup mark entries with an invalid object shape.
		value, ok := item.(map[string]any)
		if !ok {
			continue
		}

		// Append the startup mark's label, sequence and detail.
		detail, _ := value["detail"].(map[string]any)
		marks = append(marks, composedStartupMark{
			Label:    composedString(value["label"]),
			Sequence: composedNumber(value["sequence"]),
			Detail:   detail,
		})
	}
	return marks
}

func lastComposedStartupMark(t *testing.T, page playwright.Page, label string) composedStartupMark {
	// Find the newest startup mark with the requested label.
	t.Helper()
	marks := readComposedStartupMarks(t, page)
	for _, mark := range slices.Backward(marks) {
		if mark.Label == label {
			return mark
		}
	}

	// Fail the proof when its required startup mark is absent.
	t.Fatalf("production startup mark %q is missing", label)
	return composedStartupMark{}
}

func waitForStartupMark(t *testing.T, page playwright.Page, label string) {
	// Wait for the requested production startup mark.
	t.Helper()
	_, err := page.WaitForFunction(`(want) =>
		(globalThis.__swStartupMarks ?? []).some((mark) => mark.label === want)`,
		label, playwright.PageWaitForFunctionOptions{
			Timeout: playwright.Float(browserWaitMS),
		})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for production startup mark %q: %v", label, err)
	}
}

func assertDedicatedWorkerHost(t *testing.T, page playwright.Page) (string, string) {
	// Verify the host lease includes its generation and document identity.
	t.Helper()
	lease := lastComposedStartupMark(t, page, "dedicated-host.lease-acquired")
	generation := composedString(lease.Detail["generation"])
	documentID := composedString(lease.Detail["documentId"])
	if generation == "" || documentID == "" {
		t.Fatalf("host lease mark lacks generation/document identity: %#v", lease)
	}

	// Verify the host constructed exactly one dedicated runtime worker.
	marks := readComposedStartupMarks(t, page)
	workerCount := 0
	for _, mark := range marks {
		if mark.Label == "runtime.worker-created" &&
			composedString(mark.Detail["mode"]) == "dedicated-worker" {
			workerCount++
		}
	}
	if workerCount != 1 {
		t.Fatalf("supported DedicatedWorker path created %d runtime Workers in host document: %#v", workerCount, marks)
	}
	return generation, documentID
}

func assertNoRuntimeWorkerCreated(t *testing.T, page playwright.Page) {
	t.Helper()

	for _, mark := range readComposedStartupMarks(t, page) {
		if mark.Label == "runtime.worker-created" &&
			composedString(mark.Detail["mode"]) == "dedicated-worker" {
			t.Fatalf("attached document created a runtime Worker: %#v", mark)
		}
	}
}

func assertWarmPresentation(t *testing.T, page playwright.Page, generation, hostDocumentID string, requireAttachment bool) {
	// Verify an attached document acknowledges the expected host generation.
	t.Helper()
	marks := readComposedStartupMarks(t, page)
	if requireAttachment {
		ready := lastComposedStartupMark(t, page, "dedicated-host.attach-open-ready")
		if composedString(ready.Detail["hostGeneration"]) != generation ||
			composedString(ready.Detail["hostDocumentId"]) != hostDocumentID {
			t.Fatalf("attach acknowledged an unexpected runtime generation/host: %#v want generation=%q host=%q", ready, generation, hostDocumentID)
		}
	}

	// Find the latest connection to the current runtime generation.
	var connected composedStartupMark
	for _, mark := range slices.Backward(marks) {
		if mark.Label == "runtime.connected" &&
			composedString(mark.Detail["runtimeGeneration"]) == generation {
			connected = mark
			break
		}
	}
	if connected.Label == "" {
		t.Fatalf("runtime.connected did not carry current generation %q: %#v", generation, marks)
	}

	// Find the current generation's neutral frame and subsequent reveal.
	var neutral, reveal composedStartupMark
	for _, mark := range marks {
		if mark.Label == "webview.neutral-frame" &&
			composedString(mark.Detail["runtimeGeneration"]) == generation &&
			mark.Sequence > connected.Sequence {
			neutral = mark
			break
		}
	}
	for _, mark := range marks {
		if mark.Label == "webview.revealed" &&
			composedString(mark.Detail["runtimeGeneration"]) == generation &&
			mark.Sequence > neutral.Sequence {
			reveal = mark
			break
		}
	}

	// Verify the neutral frame precedes the current generation's reveal.
	if neutral.Label == "" || reveal.Label == "" || neutral.Sequence >= reveal.Sequence {
		t.Fatalf("generation-matched neutral/reveal order missing for %q: connected=%#v neutral=%#v reveal=%#v marks=%#v", generation, connected, neutral, reveal, marks)
	}

	// Verify no connection invalidation followed the current connection.
	for _, mark := range marks {
		if mark.Label == "runtime.connection-invalidated" && mark.Sequence >= connected.Sequence {
			t.Fatalf("runtime presentation advanced before obsolete generation invalidation: invalidated=%#v connected=%#v", mark, connected)
		}
	}

	// Read and verify the document's resume readiness.
	raw, err := page.Evaluate(`() => globalThis.__swWebDocumentResumeReady ?? null`)
	if err != nil {
		t.Fatalf("read production resume-ready state: %v", err)
	}
	resume, ok := raw.(map[string]any)
	if !ok || resume["ready"] != true {
		t.Fatalf("production resume-ready surface is not ready: %#v", raw)
	}

	// Verify resume readiness identifies the selected runtime.
	runtimeMark := lastComposedStartupMark(t, page, "runtime.mode-selected")
	if composedString(resume["runtimeId"]) != composedString(runtimeMark.Detail["runtimeId"]) {
		t.Fatalf("resume-ready runtime identity drifted: resume=%#v mode=%#v", resume, runtimeMark)
	}
}

func assertWarmPromotion(t *testing.T, page playwright.Page, oldGeneration string) string {
	// Wait for promotion to a replacement host generation.
	t.Helper()
	waitForStartupMark(t, page, "dedicated-host.promoted")
	promoted := lastComposedStartupMark(t, page, "dedicated-host.promoted")
	generation := composedString(promoted.Detail["generation"])
	if generation == "" || generation == oldGeneration {
		t.Fatalf("promotion did not replace runtime generation: old=%q mark=%#v", oldGeneration, promoted)
	}

	// Wait for old-generation invalidation and replacement connection.
	_, err := page.WaitForFunction(`(args) => {
		const marks = globalThis.__swStartupMarks ?? []
		return marks.some((mark) =>
			mark.label === 'runtime.connection-invalidated' &&
			mark.detail?.runtimeGeneration === args.oldGeneration &&
			mark.sequence > args.promotedSequence,
		) && marks.some((mark) =>
			mark.label === 'runtime.connected' &&
			mark.detail?.runtimeGeneration === args.generation &&
			mark.sequence > args.promotedSequence,
		)
	}`, map[string]any{
		"oldGeneration":    oldGeneration,
		"generation":       generation,
		"promotedSequence": promoted.Sequence,
	}, playwright.PageWaitForFunctionOptions{
		Timeout: playwright.Float(browserWaitMS),
	})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for old-generation invalidation and new-generation connection after promotion: %v", err)
	}

	// Locate the first invalidation and connection after promotion.
	marks := readComposedStartupMarks(t, page)
	var invalidated, connected composedStartupMark
	for _, mark := range marks {

		// Find the old generation's first post-promotion invalidation.
		if mark.Label == "runtime.connection-invalidated" &&
			composedString(mark.Detail["runtimeGeneration"]) == oldGeneration &&
			mark.Sequence > promoted.Sequence &&
			(invalidated.Label == "" || mark.Sequence < invalidated.Sequence) {
			invalidated = mark
		}

		// Find the new generation's first post-promotion connection.
		if mark.Label == "runtime.connected" &&
			composedString(mark.Detail["runtimeGeneration"]) == generation &&
			mark.Sequence > promoted.Sequence &&
			(connected.Label == "" || mark.Sequence < connected.Sequence) {
			connected = mark
		}
	}

	// Verify promotion invalidates the old runtime before connecting the replacement.
	if invalidated.Label == "" || connected.Label == "" || invalidated.Sequence >= connected.Sequence {
		t.Fatalf("promotion did not invalidate old runtime before new connection: old=%q new=%q promoted=%#v invalidated=%#v connected=%#v marks=%#v", oldGeneration, generation, promoted, invalidated, connected, marks)
	}

	// Verify the obsolete generation cannot advance presentation after promotion.
	for _, mark := range marks {
		if mark.Sequence <= promoted.Sequence {
			continue
		}
		if (mark.Label == "webview.revealed" || mark.Label == "runtime.connected") &&
			composedString(mark.Detail["runtimeGeneration"]) == oldGeneration {
			t.Fatalf("obsolete runtime generation advanced presentation after promotion: %#v", mark)
		}
	}
	return generation
}

// assertReattachedWithoutColdBoot waits for an attached document to rejoin the
// promoted host generation and checks it never started a runtime worker.
func assertReattachedWithoutColdBoot(t *testing.T, page playwright.Page, generation string) {
	// Wait for an attach to the promoted generation.
	t.Helper()
	_, err := page.WaitForFunction(`(generation) =>
		(globalThis.__swStartupMarks ?? []).some((mark) =>
			mark.label === 'dedicated-host.attach-open-ready' &&
			mark.detail?.hostGeneration === generation)`,
		generation, playwright.PageWaitForFunctionOptions{
			Timeout: playwright.Float(browserWaitMS),
		})
	if err != nil {
		dumpPageState(t, page)
		t.Fatalf("wait for attached document to rejoin promoted host %q: %v", generation, err)
	}

	// An attached document relays to the host. A worker of its own is a cold boot.
	for _, mark := range readComposedStartupMarks(t, page) {
		if mark.Label == "runtime.worker-created" {
			t.Fatalf("attached document cold-booted a runtime worker after host loss: %#v", mark)
		}
	}
}

func logWarmAttachCorrectnessMetrics(t *testing.T, page playwright.Page, generation string) {
	// Read and report the warm attachment's readiness intervals.
	t.Helper()
	raw, err := page.Evaluate(`(generation) => {
		const entries = performance.getEntriesByType('mark')
			.filter((entry) => entry.name.startsWith('spacewave.startup.'))
			.map((entry) => ({
				label: entry.name.slice('spacewave.startup.'.length),
				startTime: entry.startTime,
				detail: entry.detail ?? {},
			}))
		const find = (label) => entries.find((entry) =>
			entry.label === label &&
			(entry.detail.runtimeGeneration === generation ||
				entry.detail.hostGeneration === generation),
		)
		const navigation = performance.getEntriesByType('navigation')[0]
		const neutral = find('webview.neutral-frame')
		const attachReady = find('dedicated-host.attach-open-ready')
		const connected = find('runtime.connected')
		const reveal = find('webview.revealed')
		return {
			generation,
			navigationToNeutralMs: neutral && navigation
				? neutral.startTime - navigation.startTime
				: null,
			attachReadyToConnectedMs: attachReady && connected
				? connected.startTime - attachReady.startTime
				: null,
			connectedToRevealMs: connected && reveal
				? reveal.startTime - connected.startTime
				: null,
			navigationToRevealMs: reveal && navigation
				? reveal.startTime - navigation.startTime
				: null,
		}
	}`, generation)
	if err != nil {
		t.Fatalf("read warm-attach correctness metrics: %v", err)
	}
	t.Logf("warm-attach correctness metrics (no speed claim): %#v", raw)
}

func assertReturnVisitorBodyRouteStartupMarks(t testing.TB, page playwright.Page) {
	// Read the return visitor's body route startup marks.
	t.Helper()
	raw, err := page.Evaluate(`() => (globalThis.__swStartupMarks ?? []).map((mark) => ({
		label: mark.label,
		sequence: mark.sequence,
		detail: mark.detail ?? null,
	}))`, nil)
	if err != nil {
		t.Fatalf("read return visitor body route startup marks: %v", err)
	}

	// Decode the return visitor's startup mark inventory.
	items, ok := raw.([]any)
	if !ok {
		t.Fatalf("unexpected return visitor body route startup marks %T: %#v", raw, raw)
	}

	// Collect labels from valid return visitor startup marks.
	labels := make([]string, 0, len(items))
	for _, item := range items {

		// Ignore startup marks with an invalid object shape.
		mark, ok := item.(map[string]any)
		if !ok {
			continue
		}

		// Retain the startup mark's string label.
		if label, ok := mark["label"].(string); ok {
			labels = append(labels, label)
		}
	}

	// Verify the return visitor mounted each required body route component.
	for _, label := range []string{
		"quickstart.session-mount-start",
		"quickstart.session-mount-ready",
		"quickstart.shared-object-mount-start",
		"quickstart.shared-object-mount-ready",
		"quickstart.shared-object-body-mount-start",
		"quickstart.shared-object-body-mount-ready",
		"quickstart.space-resource-created",
		"quickstart.space-world-access-ready",
		"quickstart.space-contents-mount-ready",
		"unixfs.browser-mounted",
		"unixfs.seeded-file-visible",
	} {
		if !slices.Contains(labels, label) {
			t.Fatalf("return visitor body route missing startup mark %q; labels=%v", label, labels)
		}
	}

}

type shellDiagnosticStorage struct {
	Present bool
	Raw     string
}

type shellDiagnosticRecord struct {
	LocationHref         string
	LocationHash         string
	SessionDocumentState shellDiagnosticStorage
	BrowserShellTabs     shellDiagnosticStorage
	VisibleActivePanelID string
}

func assertInactiveClosePreservedProjection(
	t *testing.T,
	before, after composedShellProjection,
	removedLabel string,
) {

	// Remove the closed label from the expected Shell projection.
	t.Helper()
	expectedLabels := make([]string, 0, len(before.Labels))
	removed := false
	for _, label := range before.Labels {
		if label == removedLabel && !removed {
			removed = true
			continue
		}
		expectedLabels = append(expectedLabels, label)
	}

	// Verify the closed label existed in the original projection.
	if !removed {
		t.Fatalf("inactive close precondition lacks label %q: %#v", removedLabel, before)
	}

	// Verify closing an inactive Shell record preserved the document selection.
	assertSameComposedShellProjection(
		t,
		after,
		composedShellProjection{
			URL:            before.URL,
			Hash:           before.Hash,
			ActiveTabID:    before.ActiveTabID,
			VisiblePanelID: before.VisiblePanelID,
			Labels:         expectedLabels,
		},
		"closing inactive shared record changed page A projection",
	)
}

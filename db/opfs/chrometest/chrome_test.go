package chrometest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	playwright "github.com/playwright-community/playwright-go"
)

const (
	runEnv        = "RUN_OPFS_CHROME_TEST"
	profileEnv    = "RUN_OPFS_CHROME_PROFILE"
	tinyGoEnv     = "RUN_OPFS_CHROME_TINYGO"
	chromeSmoke   = "smoke"
	chromeStress  = "stress"
	defaultShards = 4
)

var sharedHarness *chromeHarness

type chromeHarness struct {
	dir     string
	server  *httptest.Server
	pw      *playwright.Playwright
	browser playwright.Browser
}

type chromeSession struct {
	ctx  playwright.BrowserContext
	page playwright.Page
}

// TIER: pr
func TestMain(m *testing.M) {
	if os.Getenv(runEnv) != "1" && !strings.EqualFold(os.Getenv(runEnv), "true") {
		os.Exit(m.Run())
	}
	h, err := startChromeHarness()
	if err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
	sharedHarness = h
	code := m.Run()
	h.close()
	os.Exit(code)
}

func TestOpfsChromeConcurrentBlockReadersWriters(t *testing.T) {
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-block-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})

	const (
		writers    = 4
		readers    = 2
		iterations = 24
		batch      = 12
	)
	var args []workerArgs
	for i := range writers {
		args = append(args, workerArgs{
			scenario:   "block-writer",
			root:       root,
			worker:     i,
			workers:    writers,
			iterations: iterations,
			batch:      batch,
			shards:     defaultShards,
		})
	}
	for i := range readers {
		args = append(args, workerArgs{
			scenario:   "block-reader",
			root:       root,
			worker:     i,
			workers:    writers,
			iterations: iterations,
			batch:      batch,
			shards:     defaultShards,
		})
	}
	s.runWorkersStaged(t, args[writers:], args[:writers])
	s.runWorker(t, workerArgs{
		scenario:   "block-verify",
		root:       root,
		workers:    writers,
		iterations: iterations,
		batch:      batch,
		shards:     defaultShards,
	})
}

func TestOpfsChromeConcurrentMetaWriters(t *testing.T) {
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-meta-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})

	const (
		writers    = 4
		iterations = 32
	)
	var args []workerArgs
	for i := range writers {
		args = append(args, workerArgs{
			scenario:   "meta-writer",
			root:       root,
			worker:     i,
			workers:    writers,
			iterations: iterations,
			batch:      1,
			shards:     defaultShards,
		})
	}
	s.runWorkers(t, args)
	s.runWorker(t, workerArgs{
		scenario:   "meta-verify",
		root:       root,
		workers:    writers,
		iterations: iterations,
		batch:      1,
		shards:     defaultShards,
	})
}

func TestOpfsChromeConcurrentMetaOverflowWriters(t *testing.T) {
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-meta-overflow-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})

	const (
		workers    = 4
		iterations = 12
	)
	var args []workerArgs
	for i := range workers {
		args = append(args, workerArgs{
			scenario:   "meta-mixed-writer",
			root:       root,
			worker:     i,
			workers:    workers,
			iterations: iterations,
			batch:      1,
			shards:     defaultShards,
		})
	}
	s.runWorkers(t, args)
	s.runWorker(t, workerArgs{
		scenario:   "meta-mixed-verify",
		root:       root,
		workers:    workers,
		iterations: iterations,
		batch:      1,
		shards:     defaultShards,
	})
}

func TestOpfsChromeManifestBloomSplitSafety(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-manifest-bloom-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "meta-manifest-bloom-split",
		root:     root,
		shards:   defaultShards,
	})

	s.reopenPage(t)

	s.runWorker(t, workerArgs{
		scenario: "meta-manifest-bloom-verify",
		root:     root,
		shards:   defaultShards,
	})
}

func TestOpfsChromeClassifiesPromiseRejection(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-reject-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "missing-delete-classify",
		root:     root,
	})
}

func TestOpfsChromeReadFileHelperLoop(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-read-helper-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "read-file-helper-loop",
		root:       root,
		iterations: 64,
	})
}

func TestOpfsChromeTinyGoLargeWriteReadList(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo OPFS helper ABI", tinyGoEnv)
	}

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-large-write-read-list-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "large-write-read-list",
		root:       root,
		iterations: 68056093,
		batch:      64,
	})
}

func TestOpfsChromeReadAtHelperLoop(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-read-at-helper-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "read-at-helper-loop",
		root:       root,
		iterations: 64,
	})
}

func TestOpfsChromeGCWalWriteLoop(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-gc-wal-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "gc-wal-write-loop",
		root:       root,
		iterations: 16,
	})
}

func TestOpfsChromePersistsAcrossPageLifecycle(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-lifecycle-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})

	const (
		blockIterations = 8
		blockBatch      = 4
		metaWorkers     = 2
		metaIterations  = 6
	)
	s.runWorker(t, workerArgs{
		scenario:   "block-writer",
		root:       root,
		worker:     0,
		workers:    1,
		iterations: blockIterations,
		batch:      blockBatch,
		shards:     defaultShards,
	})
	var metaArgs []workerArgs
	for i := range metaWorkers {
		metaArgs = append(metaArgs, workerArgs{
			scenario:   "meta-mixed-writer",
			root:       root,
			worker:     i,
			workers:    metaWorkers,
			iterations: metaIterations,
			batch:      1,
			shards:     defaultShards,
		})
	}
	s.runWorkers(t, metaArgs)

	s.reopenPage(t)

	s.runWorker(t, workerArgs{
		scenario:   "block-verify",
		root:       root,
		workers:    1,
		iterations: blockIterations,
		batch:      blockBatch,
		shards:     defaultShards,
	})
	s.runWorker(t, workerArgs{
		scenario:   "meta-mixed-verify",
		root:       root,
		workers:    metaWorkers,
		iterations: metaIterations,
		batch:      1,
		shards:     defaultShards,
	})
}

func TestOpfsChromeFileLockSerializesWorkers(t *testing.T) {
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-lock-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "counter-init",
		root:     root,
	})

	const (
		workers    = 6
		iterations = 12
	)
	var args []workerArgs
	for i := range workers {
		args = append(args, workerArgs{
			scenario:   "counter-increment",
			root:       root,
			worker:     i,
			workers:    workers,
			iterations: iterations,
			batch:      1,
			shards:     defaultShards,
		})
	}
	s.runWorkers(t, args)
	s.runWorker(t, workerArgs{
		scenario:   "counter-verify",
		root:       root,
		workers:    workers,
		iterations: iterations,
		batch:      1,
		shards:     defaultShards,
	})
}

func TestOpfsChromeFileLockQueuedWorkersProgressAfterRelease(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-lock-queued-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "counter-init",
		root:     root,
	})

	const (
		workers    = 4
		iterations = 5
	)
	holder := workerArgs{
		scenario: "counter-hold",
		root:     root,
	}
	var args []workerArgs
	for i := range workers {
		args = append(args, workerArgs{
			scenario:   "counter-queued-increment",
			root:       root,
			worker:     i,
			workers:    workers,
			iterations: iterations,
			batch:      1,
			shards:     defaultShards,
		})
	}
	s.runBlockedLockWorkers(t, holder, args)
	s.runWorker(t, workerArgs{
		scenario:   "counter-verify",
		root:       root,
		workers:    workers,
		iterations: iterations,
		batch:      1,
		shards:     defaultShards,
	})
}

func TestOpfsChromeFileLockQueuedWorkersProgressAfterHolderTermination(t *testing.T) {
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-lock-terminated-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "counter-init",
		root:     root,
	})

	const (
		workers    = 4
		iterations = 5
	)
	holder := workerArgs{
		scenario: "counter-hold",
		root:     root,
	}
	var args []workerArgs
	for i := range workers {
		args = append(args, workerArgs{
			scenario:   "counter-queued-increment",
			root:       root,
			worker:     i,
			workers:    workers,
			iterations: iterations,
			batch:      1,
			shards:     defaultShards,
		})
	}
	s.runTerminatedLockHolderWorkers(t, holder, args)
	s.runWorker(t, workerArgs{
		scenario:   "counter-verify",
		root:       root,
		workers:    workers,
		iterations: iterations,
		batch:      1,
		shards:     defaultShards,
	})
}

func TestOpfsChromeWebLockIfAvailable(t *testing.T) {
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-lock-if-available-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "counter-init",
		root:     root,
	})

	holder := workerArgs{
		scenario: "counter-hold",
		root:     root,
	}
	heldCheck := workerArgs{
		scenario: "counter-try-lock-unavailable",
		root:     root,
	}
	s.runHeldLockCheck(t, holder, heldCheck)
	s.runWorker(t, workerArgs{
		scenario: "counter-try-lock-available",
		root:     root,
	})
}

func TestOpfsChromeTerminatedBlockWriterLeavesRecoverableShard(t *testing.T) {
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-block-terminated-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "block-writer",
		root:       root,
		worker:     0,
		workers:    1,
		iterations: 1,
		batch:      1,
		shards:     defaultShards,
	})
	s.runTerminatedReadyWorker(t, workerArgs{
		scenario: "block-orphan-segment",
		root:     root,
		shards:   defaultShards,
	})
	s.runWorker(t, workerArgs{
		scenario:   "block-verify",
		root:       root,
		workers:    1,
		iterations: 1,
		batch:      1,
		shards:     defaultShards,
	})
	s.runWorker(t, workerArgs{
		scenario: "block-orphan-verify-clean",
		root:     root,
		shards:   defaultShards,
	})
}

func TestOpfsChromeTerminatedMetaWriterRecovery(t *testing.T) {
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-meta-terminated-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "meta-writer",
		root:       root,
		worker:     0,
		workers:    1,
		iterations: 1,
		batch:      1,
		shards:     defaultShards,
	})
	s.runTerminatedReadyWorker(t, workerArgs{
		scenario: "meta-crash-before-superblock",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "meta-verify",
		root:       root,
		workers:    1,
		iterations: 1,
		batch:      1,
		shards:     defaultShards,
	})
	s.runTerminatedReadyWorker(t, workerArgs{
		scenario: "meta-crash-after-superblock",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "meta-crash-verify",
		root:       root,
		workers:    1,
		iterations: 1,
		batch:      1,
		shards:     defaultShards,
	})
}

func TestOpfsChromeTerminationRecoverySurvivesFreshContext(t *testing.T) {
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	profile := "opfs-chrome-termination-profile-" + time.Now().Format("150405.000000000")
	root := "opfs-chrome-termination-reload-" + time.Now().Format("150405.000000000")

	s := h.newPersistentSession(t, profile)
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "block-writer",
		root:       root,
		worker:     0,
		workers:    1,
		iterations: 1,
		batch:      1,
		shards:     defaultShards,
	})
	s.runTerminatedReadyWorker(t, workerArgs{
		scenario: "block-orphan-segment",
		root:     root,
		shards:   defaultShards,
	})
	s.runWorker(t, workerArgs{
		scenario:   "meta-writer",
		root:       root,
		worker:     0,
		workers:    1,
		iterations: 1,
		batch:      1,
		shards:     defaultShards,
	})
	s.runTerminatedReadyWorker(t, workerArgs{
		scenario: "meta-crash-before-superblock",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "counter-init",
		root:     root,
	})

	const (
		workers    = 2
		iterations = 3
	)
	holder := workerArgs{
		scenario: "counter-hold",
		root:     root,
	}
	var args []workerArgs
	for i := range workers {
		args = append(args, workerArgs{
			scenario:   "counter-queued-increment",
			root:       root,
			worker:     i,
			workers:    workers,
			iterations: iterations,
			batch:      1,
			shards:     defaultShards,
		})
	}
	s.runTerminatedLockHolderWorkers(t, holder, args)
	s.close(t)

	reopened := h.newPersistentSession(t, profile)
	defer reopened.close(t)
	reopened.runWorker(t, workerArgs{
		scenario:   "block-verify",
		root:       root,
		workers:    1,
		iterations: 1,
		batch:      1,
		shards:     defaultShards,
	})
	reopened.runWorker(t, workerArgs{
		scenario: "block-orphan-verify-clean",
		root:     root,
		shards:   defaultShards,
	})
	reopened.runWorker(t, workerArgs{
		scenario:   "meta-verify",
		root:       root,
		workers:    1,
		iterations: 1,
		batch:      1,
		shards:     defaultShards,
	})
	reopened.runWorker(t, workerArgs{
		scenario:   "counter-verify",
		root:       root,
		workers:    workers,
		iterations: iterations,
		batch:      1,
		shards:     defaultShards,
	})
}

func TestOpfsChromeVolumeRuntimeSlice(t *testing.T) {
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-volume-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "volume-runtime-write",
		root:     root,
		shards:   defaultShards,
	})
	s.runWorker(t, workerArgs{
		scenario: "volume-runtime-verify",
		root:     root,
		shards:   defaultShards,
	})
}

func TestOpfsChromeWorldInitUnixFS(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-world-unixfs-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "world-init-unixfs",
		root:     root,
		shards:   defaultShards,
	})
}

func newChromeHarness(t testing.TB) *chromeHarness {
	t.Helper()
	if os.Getenv(runEnv) != "1" && !strings.EqualFold(os.Getenv(runEnv), "true") {
		t.Skipf("set %s=1 to run Chrome OPFS stress tests", runEnv)
	}
	if sharedHarness == nil {
		t.Fatal("Chrome OPFS stress harness was not initialized")
	}
	return sharedHarness
}

func requireChromeProfile(t testing.TB, profiles ...string) {
	t.Helper()
	profile := os.Getenv(profileEnv)
	if profile == "" {
		return
	}
	if slices.Contains(profiles, profile) {
		return
	}
	t.Skipf("set %s=%s to run this Chrome OPFS test", profileEnv, strings.Join(profiles, " or "))
}

func startChromeHarness() (*chromeHarness, error) {
	dir, err := os.MkdirTemp("", "opfs-chrometest-*")
	if err != nil {
		return nil, err
	}
	if err := buildAssets(dir); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}

	server := newServer(dir)
	if err := playwright.Install(&playwright.RunOptions{
		Browsers: []string{"chromium"},
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	}); err != nil {
		server.Close()
		os.RemoveAll(dir)
		return nil, errors.Wrap(err, "install playwright chromium")
	}
	pw, err := playwright.Run()
	if err != nil {
		server.Close()
		os.RemoveAll(dir)
		return nil, errors.Wrap(err, "start playwright")
	}
	headless := true
	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: &headless,
	})
	if err != nil {
		pw.Stop()
		server.Close()
		os.RemoveAll(dir)
		return nil, errors.Wrap(err, "launch chromium")
	}
	return &chromeHarness{
		dir:     dir,
		server:  server,
		pw:      pw,
		browser: browser,
	}, nil
}

func (h *chromeHarness) close() {
	if h.browser != nil {
		if err := h.browser.Close(); err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
		}
	}
	if h.pw != nil {
		if err := h.pw.Stop(); err != nil {
			os.Stderr.WriteString(err.Error() + "\n")
		}
	}
	if h.server != nil {
		h.server.Close()
	}
	if h.dir != "" {
		os.RemoveAll(h.dir)
	}
}

func (h *chromeHarness) newSession(t testing.TB) *chromeSession {
	t.Helper()
	ctx, err := h.browser.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	s := &chromeSession{ctx: ctx}
	s.openPage(t, h.server.URL)
	return s
}

func (h *chromeHarness) newPersistentSession(t testing.TB, name string) *chromeSession {
	t.Helper()
	headless := true
	ctx, err := h.pw.Chromium.LaunchPersistentContext(filepath.Join(h.dir, name), playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: &headless,
	})
	if err != nil {
		t.Fatal(err)
	}
	s := &chromeSession{ctx: ctx}
	s.openPage(t, h.server.URL+"/")
	return s
}

func (s *chromeSession) close(t testing.TB) {
	t.Helper()
	if err := s.ctx.Close(); err != nil {
		t.Error(err)
	}
}

func (s *chromeSession) reopenPage(t testing.TB) {
	t.Helper()
	url := s.page.URL()
	if err := s.page.Close(); err != nil {
		t.Fatal(err)
	}
	s.openPage(t, url)
}

func (s *chromeSession) openPage(t testing.TB, url string) {
	t.Helper()
	page, err := s.ctx.NewPage()
	if err != nil {
		if closeErr := s.ctx.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatal(err)
	}
	page.On("console", func(msg playwright.ConsoleMessage) {
		t.Logf("browser console %s: %s", msg.Type(), msg.Text())
	})
	page.On("pageerror", func(err error) {
		t.Errorf("page error: %v", err)
	})
	resp, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateDomcontentloaded,
		Timeout:   playwright.Float(30000),
	})
	if err != nil {
		if closeErr := page.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatal(err)
	}
	if resp != nil && resp.Status() >= 400 {
		if closeErr := page.Close(); closeErr != nil {
			t.Error(closeErr)
		}
		t.Fatalf("GET / returned HTTP %d", resp.Status())
	}
	s.page = page
}

func (s *chromeSession) runWorker(t testing.TB, args workerArgs) workerResult {
	t.Helper()
	results := s.runWorkers(t, []workerArgs{args})
	return results[0]
}

func (s *chromeSession) runWorkers(t testing.TB, args []workerArgs) []workerResult {
	t.Helper()
	return s.runWorkersScript(t, `async ({ workers }) => {
  return await window.runOpfsWorkers(workers)
}`, map[string]any{"workers": mapWorkerArgs(args)})
}

func (s *chromeSession) runWorkersStaged(t testing.TB, readyWorkers, workers []workerArgs) []workerResult {
	t.Helper()
	return s.runWorkersScript(t, `async ({ readyWorkers, workers }) => {
  return await window.runOpfsWorkersStaged(readyWorkers, workers)
	}`, map[string]any{
		"readyWorkers": mapWorkerArgs(readyWorkers),
		"workers":      mapWorkerArgs(workers),
	})
}

func (s *chromeSession) runBlockedLockWorkers(t testing.TB, holder workerArgs, workers []workerArgs) []workerResult {
	t.Helper()
	return s.runWorkersScript(t, `async ({ holder, workers }) => {
  return await window.runOpfsBlockedLockWorkers(holder, workers)
}`, map[string]any{
		"holder":  mapSingleWorkerArg(holder),
		"workers": mapWorkerArgs(workers),
	})
}

func (s *chromeSession) runTerminatedLockHolderWorkers(
	t testing.TB,
	holder workerArgs,
	workers []workerArgs,
) []workerResult {
	t.Helper()
	return s.runWorkersScript(t, `async ({ holder, workers }) => {
  return await window.runOpfsTerminatedLockHolderWorkers(holder, workers)
}`, map[string]any{
		"holder":  mapSingleWorkerArg(holder),
		"workers": mapWorkerArgs(workers),
	})
}

func (s *chromeSession) runHeldLockCheck(t testing.TB, holder, check workerArgs) []workerResult {
	t.Helper()
	return s.runWorkersScript(t, `async ({ holder, check }) => {
  return await window.runOpfsHeldLockCheck(holder, check)
}`, map[string]any{
		"holder": mapSingleWorkerArg(holder),
		"check":  mapSingleWorkerArg(check),
	})
}

func (s *chromeSession) runTerminatedReadyWorker(t testing.TB, worker workerArgs) workerResult {
	t.Helper()
	results := s.runWorkersScript(t, `async ({ worker }) => {
  return await window.runOpfsTerminateReadyWorker(worker)
}`, map[string]any{
		"worker": mapSingleWorkerArg(worker),
	})
	return results[0]
}

func (s *chromeSession) runWorkersScript(t testing.TB, script string, args map[string]any) []workerResult {
	t.Helper()
	raw, err := s.page.Evaluate(script, args)
	if err != nil {
		t.Fatal(err)
	}
	results, err := decodeWorkerResults(raw)
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range results {
		if !res.ok {
			t.Fatalf("worker scenario=%s worker=%d failed: %s", res.scenario, res.worker, res.err)
		}
		t.Logf("worker scenario=%s worker=%d duration=%dms", res.scenario, res.worker, res.durationMS)
	}
	return results
}

func buildAssets(dir string) error {
	if err := buildWasm(filepath.Join(dir, "testprog.wasm")); err != nil {
		return err
	}
	wasmExec, err := wasmExecPath()
	if err != nil {
		return err
	}
	if err := copyFile(wasmExec, filepath.Join(dir, "wasm_exec.js")); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte(indexHTML), 0o644); err != nil {
		return errors.Wrap(err, "write index")
	}
	if err := os.WriteFile(filepath.Join(dir, "worker.js"), []byte(workerJS), 0o644); err != nil {
		return errors.Wrap(err, "write worker")
	}
	return nil
}

func buildWasm(out string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	root, err := repoRoot()
	if err != nil {
		return err
	}
	if os.Getenv(tinyGoEnv) == "1" || strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		cmd := exec.CommandContext(ctx, "tinygo", "build", "-target", "wasm", "-scheduler=asyncify", "-o", out, "./db/opfs/chrometest/testprog")
		cmd.Dir = root
		data, err := cmd.CombinedOutput()
		if err != nil {
			return errors.Errorf("tinygo build js/wasm failed: %v\n%s", err, data)
		}
		return nil
	}
	cmd := exec.CommandContext(ctx, "go", "build", "-o", out, "./db/opfs/chrometest/testprog")
	cmd.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	cmd.Dir = root
	data, err := cmd.CombinedOutput()
	if err != nil {
		return errors.Errorf("go build js/wasm failed: %v\n%s", err, data)
	}
	return nil
}

func wasmExecPath() (string, error) {
	if os.Getenv(tinyGoEnv) == "1" || strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "tinygo", "env", "TINYGOROOT")
		data, err := cmd.Output()
		if err != nil {
			return "", errors.Wrap(err, "tinygo env TINYGOROOT")
		}
		tinyGoRoot := strings.TrimSpace(string(data))
		if tinyGoRoot == "" {
			return "", errors.New("tinygo env TINYGOROOT returned empty path")
		}
		return filepath.Join(tinyGoRoot, "targets", "wasm_exec.js"), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "env", "GOROOT")
	data, err := cmd.Output()
	if err != nil {
		return "", errors.Wrap(err, "go env GOROOT")
	}
	goroot := strings.TrimSpace(string(data))
	if goroot == "" {
		return "", errors.New("go env GOROOT returned empty path")
	}
	return filepath.Join(goroot, "lib", "wasm", "wasm_exec.js"), nil
}

func repoRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", errors.New("go.mod not found")
		}
		dir = parent
	}
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return errors.Wrap(err, "read "+src)
	}
	return os.WriteFile(dst, data, 0o644)
}

func newServer(dir string) *httptest.Server {
	mux := http.NewServeMux()
	fs := http.FileServer(http.Dir(dir))
	mux.HandleFunc("/", func(rw http.ResponseWriter, req *http.Request) {
		rw.Header().Set("Cross-Origin-Opener-Policy", "same-origin")
		rw.Header().Set("Cross-Origin-Embedder-Policy", "require-corp")
		fs.ServeHTTP(rw, req)
	})
	return httptest.NewServer(mux)
}

func decodeWorkerResults(raw any) ([]workerResult, error) {
	list, ok := raw.([]any)
	if !ok {
		return nil, errors.Errorf("unexpected result type %T", raw)
	}
	results := make([]workerResult, len(list))
	for i, item := range list {
		m, ok := item.(map[string]any)
		if !ok {
			return nil, errors.Errorf("unexpected result item %T", item)
		}
		results[i] = workerResult{
			scenario: stringField(m, "scenario"),
			worker:   intField(m, "worker"),
			ok:       boolField(m, "ok"),
			err:      stringField(m, "error"),
			durationMS: intField(
				m,
				"durationMs",
			),
		}
	}
	return results, nil
}

func mapWorkerArgs(args []workerArgs) []map[string]any {
	out := make([]map[string]any, len(args))
	for i, arg := range args {
		out[i] = mapSingleWorkerArg(arg)
	}
	return out
}

func mapSingleWorkerArg(arg workerArgs) map[string]any {
	return map[string]any{
		"scenario":   arg.scenario,
		"root":       arg.root,
		"worker":     arg.worker,
		"workers":    arg.workers,
		"iterations": arg.iterations,
		"batch":      arg.batch,
		"shards":     arg.shards,
	}
}

func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

func boolField(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

func intField(m map[string]any, key string) int {
	switch v := m[key].(type) {
	case int:
		return v
	case float64:
		return int(v)
	default:
		return 0
	}
}

type workerArgs struct {
	scenario   string
	root       string
	worker     int
	workers    int
	iterations int
	batch      int
	shards     int
}

type workerResult struct {
	scenario   string
	worker     int
	ok         bool
	err        string
	durationMS int
}

const indexHTML = `<!doctype html>
<html>
  <head>
    <meta charset="utf-8">
    <title>opfs chrome test</title>
  </head>
  <body>
    <script type="module">
      window.runOpfsWorkers = async (workers) => {
        return await waitWorkers(workers.map((args) => runWorker(args)))
      }

      window.runOpfsWorkersStaged = async (readyWorkers, workers) => {
        const ready = readyWorkers.map((args) => runWorker(args))
        const readyResults = await Promise.all(ready.map((item) => item.ready))
        if (readyResults.some((result) => result.kind === 'result' && !result.ok)) {
          return compactResults(readyResults)
        }
        const started = workers.map((args) => runWorker(args))
        return await waitWorkers([...ready, ...started])
      }

      window.runOpfsBlockedLockWorkers = async (holderArgs, workers) => {
        const holder = runWorker(holderArgs)
        const holderReady = await holder.ready
        if (holderReady.kind === 'result') {
          return compactResults([holderReady])
        }
        const queued = workers.map((args) => runWorker(args))
        const queuedReady = await Promise.all(queued.map((item) => item.ready))
        if (queuedReady.some((result) => result.kind === 'result' && !result.ok)) {
          holder.stop()
          return compactResults(queuedReady)
        }
        const release = new BroadcastChannel('opfs-chrometest-counter-release:' + holderArgs.root)
        release.postMessage({ type: 'release' })
        release.close()
        return await waitWorkers([holder, ...queued])
      }

      window.runOpfsHeldLockCheck = async (holderArgs, checkArgs) => {
        const holder = runWorker(holderArgs)
        const holderReady = await holder.ready
        if (holderReady.kind === 'result') {
          return compactResults([holderReady])
        }
        const check = runWorker(checkArgs)
        const checkResults = await waitWorkers([check])
        if (checkResults.some((result) => !result.ok)) {
          holder.stop()
          return checkResults
        }
        const release = new BroadcastChannel('opfs-chrometest-counter-release:' + holderArgs.root)
        release.postMessage({ type: 'release' })
        release.close()
        const holderResults = await waitWorkers([holder])
        return [...checkResults, ...holderResults]
      }

      window.runOpfsTerminatedLockHolderWorkers = async (holderArgs, workers) => {
        const holder = runWorker(holderArgs)
        const holderReady = await holder.ready
        if (holderReady.kind === 'result') {
          return compactResults([holderReady])
        }
        const queued = workers.map((args) => runWorker(args))
        const queuedReady = await Promise.all(queued.map((item) => item.ready))
        if (queuedReady.some((result) => result.kind === 'result' && !result.ok)) {
          holder.stop()
          return compactResults(queuedReady)
        }
        holder.stop()
        return await waitWorkers(queued)
      }

      window.runOpfsTerminateReadyWorker = async (args) => {
        const worker = runWorker(args)
        const ready = await worker.ready
        if (ready.kind === 'result') {
          return compactResults([ready])
        }
        worker.stop()
        return [{
          kind: 'result',
          scenario: args.scenario,
          worker: args.worker ?? 0,
          ok: true,
          durationMs: 0,
        }]
      }

      function waitWorkers(items) {
        return new Promise((resolve) => {
          const results = new Array(items.length)
          let remaining = items.length
          let resolved = false
          for (const [index, item] of items.entries()) {
            item.done.then((result) => {
              if (resolved) {
                return
              }
              results[index] = result
              if (!result.ok) {
                resolved = true
                for (const other of items) {
                  other.stop()
                }
                resolve(compactResults(results))
                return
              }
              remaining--
              if (remaining === 0) {
                resolved = true
                resolve(results)
              }
            })
          }
        })
      }

      function compactResults(results) {
        return results.filter((result) => result)
      }

      function runWorker(args) {
        let readyResolve
        const worker = new Worker('/worker.js', { type: 'classic' })
        const ready = new Promise((resolve) => {
          readyResolve = resolve
        })
        const done = new Promise((resolve) => {
          worker.onmessage = (event) => {
            const data = event.data
            if (data.kind === 'ready') {
              readyResolve(data)
              return
            }
            if (data.kind === 'result') {
              worker.terminate()
              readyResolve(data)
              resolve(data)
            }
          }
          worker.onerror = (event) => {
            worker.terminate()
            const data = {
              kind: 'result',
              scenario: args.scenario,
              worker: args.worker ?? 0,
              ok: false,
              error: event.message,
            }
            readyResolve(data)
            resolve(data)
          }
          worker.postMessage(args)
        })
        return {
          ready,
          done,
          stop: () => worker.terminate(),
        }
      }
    </script>
  </body>
</html>
`

const workerJS = `importScripts('/wasm_exec.js')

self.__BLDR_TINYGO_STORED_BYTES = new Map()
self.__BLDR_TINYGO_STORED_BYTES_NEXT_ID = 1
self.__BLDR_TINYGO_MEMORY_VIEW = (ptr, len) => {
  const memory = self.__BLDR_TINYGO_MEMORY
  if (!(memory instanceof WebAssembly.Memory)) {
    throw new Error('TinyGo runtime memory is not initialized')
  }
  return new Uint8Array(memory.buffer, ptr, len)
}
self.__BLDR_TINYGO_STORE_BYTES = (bytes) => {
  const id = self.__BLDR_TINYGO_STORED_BYTES_NEXT_ID++
  self.__BLDR_TINYGO_STORED_BYTES.set(id, bytes)
  return id
}
self.BLDR_TINYGO_JS_CALL ??= (target, method, ...args) => {
  const fn = target[method]
  if (typeof fn !== 'function') {
    throw new TypeError('method ' + String(method) + ' is not callable')
  }
  return fn.apply(target, args)
}
self.BLDR_TINYGO_JS_NEW ??= (ctor, ...args) => new ctor(...args)
self.BLDR_TINYGO_PROMISE_ERROR_CODE ??= (reason) => {
  let name = ''
  if (reason && typeof reason === 'object') {
    if (typeof reason.name === 'string') {
      name = reason.name
    }
    if (!name && reason.constructor && typeof reason.constructor.name === 'string') {
      name = reason.constructor.name
    }
  }
  if (!name) {
    name = String(reason)
  }
  if (name.includes('NotFoundError')) {
    return 1
  }
  if (name.includes('NoModificationAllowedError')) {
    return 2
  }
  return 0
}
self.BLDR_TINYGO_PROMISE_AWAIT ??= (promise, resolve, reject) => {
  promise.then(resolve).catch((reason) => reject(self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
}
self.BLDR_TINYGO_COPY_STORED_BYTES ??= (id, ptr, len) => {
  const bytes = self.__BLDR_TINYGO_STORED_BYTES.get(id)
  self.__BLDR_TINYGO_STORED_BYTES.delete(id)
  if (!bytes) {
    return 0
  }
  const n = Math.min(len, bytes.byteLength)
  self.__BLDR_TINYGO_MEMORY_VIEW(ptr, n).set(bytes.subarray(0, n))
  return n
}
self.BLDR_TINYGO_PUSH_BYTES ??= (sink, ptr, len) => {
  const msg = new Uint8Array(len)
  msg.set(self.__BLDR_TINYGO_MEMORY_VIEW(ptr, len))
  sink.push(msg)
}
self.BLDR_TINYGO_POST_BYTES ??= (port, ptr, len) => {
  const msg = new Uint8Array(len)
  msg.set(self.__BLDR_TINYGO_MEMORY_VIEW(ptr, len))
  port.postMessage(msg)
}
self.__BLDR_TINYGO_ENCODE_NAMES ??= (names) => {
  const encoder = new TextEncoder()
  const encoded = names.map((name) => encoder.encode(name))
  let size = 4
  for (const name of encoded) {
    size += 4 + name.byteLength
  }
  const bytes = new Uint8Array(size)
  const writeUint32 = (off, value) => {
    bytes[off] = (value >>> 24) & 0xff
    bytes[off + 1] = (value >>> 16) & 0xff
    bytes[off + 2] = (value >>> 8) & 0xff
    bytes[off + 3] = value & 0xff
    return off + 4
  }
  let off = writeUint32(0, encoded.length)
  for (const name of encoded) {
    off = writeUint32(off, name.byteLength)
    bytes.set(name, off)
    off += name.byteLength
  }
  return bytes
}
self.BLDR_OPFS_READ_FILE ??= (dir, name, opID, resolve, reject) => {
  dir.getFileHandle(name)
    .then((handle) => handle.getFile())
    .then((file) => file.arrayBuffer())
    .then((buf) => {
      const bytes = new Uint8Array(buf)
      resolve(opID, self.__BLDR_TINYGO_STORE_BYTES(bytes), bytes.byteLength)
    })
    .catch((reason) => reject(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
}
self.BLDR_OPFS_READ_AT ??= (handle, ptr, len, off, opID, resolve, reject) => {
  handle.getFile()
    .then(async (file) => {
      if (off >= file.size || len === 0) {
        resolve(opID, 0)
        return
      }
      const end = Math.min(off + len, file.size)
      const buf = await file.slice(off, end).arrayBuffer()
      const bytes = new Uint8Array(buf)
      if (bytes.byteLength !== 0) {
        self.__BLDR_TINYGO_MEMORY_VIEW(ptr, bytes.byteLength).set(bytes)
      }
      resolve(opID, bytes.byteLength)
    })
    .catch((reason) => reject(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
}
self.BLDR_OPFS_LIST_DIRECTORY ??= (dir, opID, resolve, reject) => {
  ;(async () => {
    const names = []
    for await (const [name] of dir.entries()) {
      names.push(name)
    }
    const bytes = self.__BLDR_TINYGO_ENCODE_NAMES(names)
    resolve(opID, self.__BLDR_TINYGO_STORE_BYTES(bytes), bytes.byteLength)
  })().catch((reason) => reject(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
}
self.BLDR_OPFS_WRITE_AT ??= (handle, ptr, len, off, keepExisting, opID, resolve, reject) => {
  const data = new Uint8Array(len)
  if (len !== 0) {
    data.set(self.__BLDR_TINYGO_MEMORY_VIEW(ptr, len))
  }
  let writable
  const opts = keepExisting ? { keepExistingData: true } : undefined
  const writablePromise = opts ? handle.createWritable(opts) : handle.createWritable()
  writablePromise
    .then(async (next) => {
      writable = next
      if (off !== 0) {
        await writable.seek(off)
      }
      if (len !== 0) {
        await writable.write(data)
      }
      await writable.close()
      resolve(opID, len)
    })
    .catch((reason) => {
      if (writable) {
        void writable.close().catch(() => {})
      }
      reject(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
    })
}
self.BLDR_OPFS_WRITE_FILE ??= (dir, name, ptr, len, opID, resolve, reject) => {
  const data = new Uint8Array(len)
  if (len !== 0) {
    data.set(self.__BLDR_TINYGO_MEMORY_VIEW(ptr, len))
  }
  let writable
  dir.getFileHandle(name, { create: true })
    .then(async (handle) => {
      writable = await handle.createWritable()
      if (len !== 0) {
        await writable.write(data)
      }
      await writable.close()
      resolve(opID, len)
    })
    .catch((reason) => {
      if (writable) {
        void writable.close().catch(() => {})
      }
      reject(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
    })
}

self.onmessage = async (event) => {
  const args = event.data
  const go = new Go()
  go.argv = [
    'testprog',
    args.scenario ?? '',
    args.root ?? '',
    String(args.worker ?? 0),
    String(args.workers ?? 1),
    String(args.iterations ?? 1),
    String(args.batch ?? 1),
    String(args.shards ?? 4),
  ]
  self.__OPFS_CHROMETEST_ARGS = go.argv
  if (go.importObject.gojs && typeof go.importObject.gojs['runtime.getRandomData'] !== 'function') {
    go.importObject.gojs['runtime.getRandomData'] = (ptr, len) => {
      const memory = go._inst?.exports.memory
      if (!(memory instanceof WebAssembly.Memory)) {
        throw new Error('TinyGo runtime memory is not initialized')
      }
      crypto.getRandomValues(new Uint8Array(memory.buffer, ptr, len))
    }
  }
  const res = await WebAssembly.instantiateStreaming(fetch('/testprog.wasm'), go.importObject)
  self.__BLDR_TINYGO_MEMORY = res.instance.exports.memory
  await go.run(res.instance)
}
`

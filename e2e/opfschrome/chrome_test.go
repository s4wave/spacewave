package chrometest

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	playwright "github.com/mxschmitt/playwright-go"
	"github.com/pkg/errors"
)

// Browser probe settings and watchdog deadlines.
const (
	runEnv                          = "RUN_OPFS_CHROME_TEST"
	profileEnv                      = "RUN_OPFS_CHROME_PROFILE"
	tinyGoEnv                       = "RUN_OPFS_CHROME_TINYGO"
	tinyGoProfileEnv                = "RUN_OPFS_CHROME_TINYGO_PROFILE"
	tinyGoOptEnv                    = "RUN_OPFS_CHROME_TINYGO_OPT"
	tinyGoGCEnv                     = "RUN_OPFS_CHROME_TINYGO_GC"
	tinyGoLLVMEnv                   = "RUN_OPFS_CHROME_TINYGO_LLVM_FEATURES"
	tinyGoPanicEnv                  = "RUN_OPFS_CHROME_TINYGO_PANIC"
	tinyGoSchedulerEnv              = "RUN_OPFS_CHROME_TINYGO_SCHEDULER"
	tinyGoStackEnv                  = "RUN_OPFS_CHROME_TINYGO_STACK_SIZE"
	resourceReadChunkEnv            = "RUN_OPFS_CHROME_RESOURCE_READ_CHUNK"
	largeSizeEnv                    = "RUN_OPFS_CHROME_LARGE_SIZE"
	tinyGoProfileCustom             = "custom"
	tinyGoTargetDefault             = "target-default"
	tinyGoBldrFeatures              = "bldr-features"
	chromeSmoke                     = "smoke"
	chromeStress                    = "stress"
	runWorkersScriptWatchdogDefault = 30 * time.Second
	runWorkersScriptWatchdogMargin  = 15 * time.Second
	runWorkersScriptWatchdogMin     = 100 * time.Millisecond
)

// sharedHarness owns the browser and generated assets shared by this test process.
var sharedHarness *chromeHarness

// chromeHarness owns the selected browser, asset server, and disposable build directory.
type chromeHarness struct {
	// dir owns the disposable asset and profile directory.
	dir string
	// server serves the compiled worker.
	server *httptest.Server
	// pw owns the Playwright driver process.
	pw *playwright.Playwright
	// browser owns the launched browser.
	browser playwright.Browser
	// browserType creates matching persistent profiles when needed.
	browserType playwright.BrowserType
}

// chromeSession owns an isolated browser context and its current page.
type chromeSession struct {
	// ctx owns this scenario's isolated storage and pages.
	ctx playwright.BrowserContext
	// page is the currently active test page.
	page playwright.Page
}

// storageSnapshot records origin quota and recursively counted OPFS file bytes.
type storageSnapshot struct {
	// usage is the browser's origin usage estimate in bytes.
	usage int64
	// quota is the origin quota in bytes.
	quota int64
	// opfsBytes is the recursive sum of OPFS file sizes.
	opfsBytes int64
	// files counts OPFS files.
	files int64
	// persisted reports the origin's persistent-storage grant.
	persisted bool
}

// TIER: pr
func TestMain(m *testing.M) {
	// Run ordinary tests unless the Chrome harness was explicitly enabled.
	if os.Getenv(runEnv) != "1" && !strings.EqualFold(os.Getenv(runEnv), "true") {
		os.Exit(m.Run())
	}

	// Start the shared Chrome harness and close it after the test run.
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

// newChromeHarness returns the initialized harness or skips when browser tests are disabled.
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

// requireChromeProfile skips scenarios outside the selected test profile.
func requireChromeProfile(t testing.TB, profiles ...string) {
	// Apply the selected browser profile to the requested scenario.
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

// envInt reads a nonnegative workload setting, returning zero when unset.
func envInt(t testing.TB, key string) int {
	// Read and validate the nonnegative workload setting.
	t.Helper()
	val := strings.TrimSpace(os.Getenv(key))
	if val == "" {
		return 0
	}
	n, err := strconv.Atoi(val)
	if err != nil {
		t.Fatalf("invalid %s=%q: %v", key, val, err)
	}
	if n < 0 {
		t.Fatalf("invalid %s=%d: must be non-negative", key, n)
	}
	return n
}

// envIntDefault reads a workload setting or uses the supplied positive default.
func envIntDefault(t testing.TB, key string, def int) int {
	t.Helper()
	val := envInt(t, key)
	if val == 0 {
		return def
	}
	return val
}

// BenchmarkOpfsChromeProductVolumeKVWrite commits one key per write
// transaction on the product OPFS volume inside real Chromium workers. The
// worker reports total elapsed nanos and ops. ns/op is derived here.
func BenchmarkOpfsChromeProductVolumeKVWrite(b *testing.B) {
	benchmarkOpfsChromeProductVolumeKVWrite(b, "volume-kv-write-per-op")
}

// BenchmarkOpfsChromeProductVolumeKVWriteSingleTx puts all values into one
// write transaction and commits once. The delta from
// BenchmarkOpfsChromeProductVolumeKVWrite isolates per-transaction commit
// overhead on the product immutable-engine path.
func BenchmarkOpfsChromeProductVolumeKVWriteSingleTx(b *testing.B) {
	benchmarkOpfsChromeProductVolumeKVWrite(b, "volume-kv-write-single-tx")
}

// benchmarkOpfsChromeProductVolumeKVWrite measures product metadata writes in isolated worker roots.
func benchmarkOpfsChromeProductVolumeKVWrite(b *testing.B, scenario string) {
	// Open a browser session for the selected OPFS worker scenario.
	requireChromeProfile(b, chromeStress)
	h := newChromeHarness(b)
	s := h.newSession(b)
	defer s.close(b)

	// Measure each metadata value size in an independent OPFS root.
	for _, size := range []int{4 << 10, 64 << 10} {
		b.Run(strconv.Itoa(size), func(b *testing.B) {
			// Unique root per size and session so no run reuses another
			// run's tree state.
			root := "opfs-chrome-kv-" + scenario + "-" + strconv.Itoa(size) + "-" + strconv.FormatInt(time.Now().UnixNano(), 10)

			// Pre-delete of any stale root and the final delete below both
			// fail loudly on non-NotFound errors (clearRoot), so a live
			// stale handle aborts instead of measuring contamination.
			clearRes := s.runWorker(b, workerArgs{scenario: "clear", root: root})
			if !clearRes.ok {
				b.Fatal(clearRes.err)
			}

			// Run the metadata write workload and require its successful result.
			res := s.runWorker(b, workerArgs{
				scenario:   scenario,
				root:       root,
				iterations: b.N,
				batch:      size,
			})
			if !res.ok {
				b.Fatal(res.err)
			}

			// Delete the benchmark root and require usable worker timing.
			delRes := s.runWorker(b, workerArgs{scenario: "clear", root: root})
			if !delRes.ok {
				b.Fatalf("final volume root delete: %s", delRes.err)
			}
			if res.ops == 0 || res.opNanos == 0 {
				b.Fatal("worker reported no benchmark timing")
			}

			// Distinct unit name: the default ns/op column would report
			// harness wall time per op, not storage cost.
			b.ReportMetric(float64(res.opNanos)/float64(res.ops), "storage-ns/op")
		})
	}
}

// startChromeHarness builds and serves the assets, then launches the selected browser.
// The caller owns the returned harness and closes it after all sessions finish.
func startChromeHarness() (*chromeHarness, error) {
	// Build browser assets in a disposable harness directory.
	dir, err := os.MkdirTemp("", "opfs-chrometest-*")
	if err != nil {
		return nil, err
	}
	if err := buildAssets(dir); err != nil {
		os.RemoveAll(dir)
		return nil, err
	}

	// Serve the assets and validate the requested browser type.
	server := newServer(dir)
	browserName := os.Getenv("RUN_OPFS_BROWSER")
	if browserName == "" {
		browserName = "chromium"
	}
	if browserName != "chromium" && browserName != "webkit" {
		server.Close()
		os.RemoveAll(dir)
		return nil, errors.New("RUN_OPFS_BROWSER must be chromium or webkit")
	}

	// Install the selected browser through Playwright.
	if err := playwright.Install(&playwright.RunOptions{
		Browsers: []string{browserName},
		Stdout:   os.Stdout,
		Stderr:   os.Stderr,
	}); err != nil {
		server.Close()
		os.RemoveAll(dir)
		return nil, errors.Wrap(err, "install playwright "+browserName)
	}

	// Start the Playwright driver for the selected browser.
	pw, err := playwright.Run()
	if err != nil {
		server.Close()
		os.RemoveAll(dir)
		return nil, errors.Wrap(err, "start playwright")
	}

	// Select browser launch settings, including the requested Chromium GPU mode.
	browserType := pw.Chromium
	if browserName == "webkit" {
		browserType = pw.WebKit
	}
	headless := true
	launchOpts := playwright.BrowserTypeLaunchOptions{
		Headless: &headless,
	}
	if browserName == "chromium" && chrometestGPUEnabled() {
		channel := "chromium"
		launchOpts.Channel = &channel
		headless = false
		launchOpts.Args = []string{
			"--headless=new",
			"--ignore-gpu-blocklist",
			"--use-angle=vulkan",
			"--enable-gpu-rasterization",
			"--enable-zero-copy",
			"--enable-features=Vulkan",
		}
	}

	// Launch the browser and release harness resources if launch fails.
	browser, err := browserType.Launch(launchOpts)
	if err != nil {
		pw.Stop()
		server.Close()
		os.RemoveAll(dir)
		return nil, errors.Wrap(err, "launch "+browserName)
	}
	return &chromeHarness{
		dir:         dir,
		server:      server,
		pw:          pw,
		browser:     browser,
		browserType: browserType,
	}, nil
}

// close releases browser resources before the server and generated assets.
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

// newSession opens an isolated browser context and its initial page.
// WebKit uses a persistent context because its ephemeral context lacks OPFS.
func (h *chromeHarness) newSession(t testing.TB) *chromeSession {
	// Attribute browser session failures to the calling test.
	t.Helper()

	// Use a persistent profile when WebKit needs one for OPFS support.
	if h.browserType.Name() == "webkit" {
		return h.newPersistentSession(t, "webkit-"+strconv.FormatInt(time.Now().UnixNano(), 10))
	}

	// Open an isolated browser context and its initial page.
	ctx, err := h.browser.NewContext()
	if err != nil {
		t.Fatal(err)
	}
	s := &chromeSession{ctx: ctx}
	s.openPage(t, h.server.URL)
	return s
}

// newPersistentSession opens one named browser profile and its initial page.
func (h *chromeHarness) newPersistentSession(t testing.TB, name string) *chromeSession {
	// Attribute persistent browser session failures to the calling test.
	t.Helper()

	// Select persistent browser launch settings for the requested GPU mode.
	headless := true
	persistOpts := playwright.BrowserTypeLaunchPersistentContextOptions{
		Headless: &headless,
	}
	if h.browserType.Name() == "chromium" && chrometestGPUEnabled() {
		channel := "chromium"
		persistOpts.Channel = &channel
		headless = false
		persistOpts.Args = []string{
			"--headless=new",
			"--ignore-gpu-blocklist",
			"--use-angle=vulkan",
			"--enable-gpu-rasterization",
			"--enable-zero-copy",
			"--enable-features=Vulkan",
		}
	}

	// Open the named browser profile and its initial page.
	ctx, err := h.browserType.LaunchPersistentContext(filepath.Join(h.dir, name), persistOpts)
	if err != nil {
		t.Fatal(err)
	}
	s := &chromeSession{ctx: ctx}
	s.openPage(t, h.server.URL+"/")
	return s
}

// close releases this session's page and profile resources through its context.
func (s *chromeSession) close(t testing.TB) {
	t.Helper()

	if err := s.ctx.Close(); err != nil {
		t.Error(err)
	}
}

// openPage creates the session page and waits for its initial document.
func (s *chromeSession) openPage(t testing.TB, url string) {
	// Attribute browser page failures to the calling test.
	t.Helper()

	// Create a browser page and attach console and error reporting.
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

	// Load the initial document and require a successful HTTP response.
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

// readStorageSnapshot measures origin quota and all OPFS files in the isolated context.
func (s *chromeSession) readStorageSnapshot(t testing.TB) storageSnapshot {
	// Measure browser origin quota and OPFS file sizes.
	t.Helper()
	raw, err := s.page.Evaluate(`async () => {
	  const estimate = await navigator.storage.estimate()
	  const root = await navigator.storage.getDirectory()
	  let opfsBytes = 0
	  let files = 0
	  const walk = async (dir) => {
	    for await (const [, handle] of dir.entries()) {
	      if (handle.kind === 'directory') {
	        await walk(handle)
	        continue
	      }
	      const file = await handle.getFile()
	      opfsBytes += file.size
	      files++
	    }
	  }
	  await walk(root)
	  return {
	    usage: estimate.usage ?? 0,
	    quota: estimate.quota ?? 0,
	    opfsBytes,
	    files,
	    persisted: await navigator.storage.persisted(),
	  }
	}`)
	if err != nil {
		t.Fatal(err)
	}

	// Decode the browser storage snapshot and require numeric size fields.
	values, ok := raw.(map[string]any)
	if !ok {
		t.Fatalf("storage snapshot returned %T", raw)
	}
	number := func(name string) int64 {
		switch value := values[name].(type) {
		case int:
			return int64(value)
		case int64:
			return value
		case float64:
			return int64(value)
		default:
			t.Fatalf("storage snapshot %s returned %T", name, values[name])
			return 0
		}
	}
	persisted, ok := values["persisted"].(bool)
	if !ok {
		t.Fatalf("storage snapshot persisted returned %T", values["persisted"])
	}

	return storageSnapshot{
		usage:     number("usage"),
		quota:     number("quota"),
		opfsBytes: number("opfsBytes"),
		files:     number("files"),
		persisted: persisted,
	}
}

// runWorker runs one worker to a successful terminal result.
func (s *chromeSession) runWorker(t testing.TB, args workerArgs) workerResult {
	t.Helper()
	results := s.runWorkers(t, []workerArgs{args})
	return results[0]
}

// runWorkers starts a group of workers concurrently and checks their results.
func (s *chromeSession) runWorkers(t testing.TB, args []workerArgs) []workerResult {
	t.Helper()
	return s.runWorkersScript(t, `async ({ workers }) => {
  return await window.runOpfsWorkers(workers)
}`, map[string]any{"workers": mapWorkerArgs(args)})
}

// runBlockedLockWorkers queues writers behind a holder and then releases it.
func (s *chromeSession) runBlockedLockWorkers(t testing.TB, holder workerArgs, workers []workerArgs) []workerResult {
	t.Helper()
	return s.runWorkersScript(t, `async ({ holder, workers }) => {
  return await window.runOpfsBlockedLockWorkers(holder, workers)
}`, map[string]any{
		"holder":  mapSingleWorkerArg(holder),
		"workers": mapWorkerArgs(workers),
	})
}

// runTerminatedLockHolderWorkers queues writers before terminating the current lock holder.
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

// runHeldLockCheck runs a lock probe while another worker holds the lock.
func (s *chromeSession) runHeldLockCheck(t testing.TB, holder, check workerArgs) []workerResult {
	t.Helper()
	return s.runWorkersScript(t, `async ({ holder, check }) => {
  return await window.runOpfsHeldLockCheck(holder, check)
}`, map[string]any{
		"holder": mapSingleWorkerArg(holder),
		"check":  mapSingleWorkerArg(check),
	})
}

// runWorkersScript checks and logs every worker's terminal result.
func (s *chromeSession) runWorkersScript(t testing.TB, script string, args map[string]any) []workerResult {
	// Collect worker results and report each unsuccessful scenario.
	t.Helper()
	results, err := s.evalWorkersScript(t, script, args, runWorkersScriptTimeout(t))
	if err != nil {
		t.Fatal(err)
	}
	for _, res := range results {
		if !res.ok {
			t.Fatalf("worker scenario=%s worker=%d failed: %s", res.scenario, res.worker, res.err)
		}
		t.Logf("worker scenario=%s worker=%d ok=%t duration=%dms", res.scenario, res.worker, res.ok, res.durationMS)
	}
	return results
}

// evalWorkersScript bounds browser evaluation and preserves the last progress on failure.
func (s *chromeSession) evalWorkersScript(
	t testing.TB,
	script string,
	args map[string]any,
	timeout time.Duration,
) ([]workerResult, error) {
	// Require a live test context and a usable worker watchdog deadline.
	t.Helper()
	if timeout < runWorkersScriptWatchdogMin {
		timeout = runWorkersScriptWatchdogMin
	}
	if err := testContext(t).Err(); err != nil {
		return nil, errors.Wrap(err, "opfs worker script context")
	}

	// Evaluate the worker script and retain watchdog progress on browser failures.
	raw, err := s.page.Evaluate(opfsWorkerScriptEnvelope, map[string]any{
		"script":    script,
		"args":      args,
		"timeoutMs": int(timeout / time.Millisecond),
	})
	if err != nil {
		state := s.readWorkerScriptState()
		if state != nil {
			return nil, errors.Errorf("evaluate opfs worker script: %v; %s", err, state.describe("evaluate error"))
		}
		return nil, errors.Wrap(err, "evaluate opfs worker script")
	}

	// Decode the worker envelope and return its terminal result or failure.
	env, err := decodeWorkerScriptEnvelope(raw)
	if err != nil {
		return nil, err
	}
	switch env.status {
	case "ok":
		return env.results, nil
	case "timeout":
		return nil, errors.New(env.describe("timeout"))
	case "exception":
		return nil, errors.New(env.describe("exception"))
	default:
		return nil, errors.Errorf("unexpected opfs worker script status %q", env.status)
	}
}

// readWorkerScriptState reads the browser watchdog snapshot when evaluation fails.
func (s *chromeSession) readWorkerScriptState() *workerScriptEnvelope {
	// Read and decode the browser watchdog state after evaluation fails.
	raw, err := s.page.Evaluate(`() => window.__opfsChromeWorkerWatchdog?.snapshot?.() ?? null`)
	if err != nil || raw == nil {
		return nil
	}
	env, err := decodeWorkerScriptEnvelope(raw)
	if err != nil {
		return nil
	}
	return env
}

// testContext returns the test lifetime context when the testing implementation supports it.
func testContext(t testing.TB) context.Context {
	ctxT, ok := t.(interface {
		Context() context.Context
	})
	if !ok {
		return context.Background()
	}
	return ctxT.Context()
}

// runWorkersScriptTimeout leaves time for diagnostics before the Go test deadline.
func runWorkersScriptTimeout(t testing.TB) time.Duration {
	// Reserve time for diagnostics before the test deadline.
	deadlineT, ok := t.(interface {
		Deadline() (time.Time, bool)
	})
	if !ok {
		return runWorkersScriptWatchdogDefault
	}
	deadline, ok := deadlineT.Deadline()
	if !ok {
		return runWorkersScriptWatchdogDefault
	}
	timeout := time.Until(deadline) - runWorkersScriptWatchdogMargin
	if timeout < runWorkersScriptWatchdogMin {
		return runWorkersScriptWatchdogMin
	}
	return timeout
}

// newServer serves test assets with the browser isolation headers required by workers.
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

// decodeWorkerScriptEnvelope decodes terminal status, progress, and worker results.
func decodeWorkerScriptEnvelope(raw any) (*workerScriptEnvelope, error) {
	// Decode worker terminal status and any completed results.
	m, ok := raw.(map[string]any)
	if !ok {
		return nil, errors.Errorf("unexpected worker script envelope %T", raw)
	}
	results, err := decodeWorkerResults(m["results"])
	if err != nil && m["results"] != nil {
		return nil, err
	}
	return &workerScriptEnvelope{
		status:    stringField(m, "status"),
		results:   results,
		progress:  decodeWorkerScriptProgress(m["progress"]),
		exception: stringField(m, "exception"),
		timeoutMS: intField(m, "timeoutMs"),
	}, nil
}

// decodeWorkerScriptProgress decodes optional progress while retaining field presence.
func decodeWorkerScriptProgress(raw any) workerScriptProgress {
	// Decode optional worker progress while retaining field presence.
	m, ok := raw.(map[string]any)
	if !ok {
		return workerScriptProgress{}
	}
	worker, hasWorker := intFieldOK(m, "worker")
	offset, hasOffset := intFieldOK(m, "offset")
	total, hasTotal := intFieldOK(m, "total")
	return workerScriptProgress{
		scenario:  stringField(m, "scenario"),
		worker:    worker,
		hasWorker: hasWorker,
		phase:     stringField(m, "phase"),
		offset:    offset,
		hasOffset: hasOffset,
		total:     total,
		hasTotal:  hasTotal,
	}
}

// decodeWorkerResults decodes the worker identity, status, and supported timing fields.
func decodeWorkerResults(raw any) ([]workerResult, error) {
	// Decode each worker result and its supported timing fields.
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
			opNanos: int64Field(m, "opNanos"),
			ops:     int64Field(m, "ops"),
		}
	}

	return results, nil
}

// mapWorkerArgs converts worker configurations to browser argument objects.
func mapWorkerArgs(args []workerArgs) []map[string]any {
	out := make([]map[string]any, len(args))
	for i, arg := range args {
		out[i] = mapSingleWorkerArg(arg)
	}
	return out
}

// mapSingleWorkerArg encodes one worker configuration for browser dispatch.
func mapSingleWorkerArg(arg workerArgs) map[string]any {
	return map[string]any{
		"scenario":   arg.scenario,
		"root":       arg.root,
		"worker":     arg.worker,
		"workers":    arg.workers,
		"iterations": arg.iterations,
		"batch":      arg.batch,
	}
}

// stringField reads an optional string from a browser result object.
func stringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
}

// boolField reads an optional boolean from a browser result object.
func boolField(m map[string]any, key string) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return false
}

// intField reads a browser integer or returns zero when absent.
func intField(m map[string]any, key string) int {
	v, _ := intFieldOK(m, key)
	return v
}

// intFieldOK reads a browser integer while retaining field presence.
func intFieldOK(m map[string]any, key string) (int, bool) {
	switch v := m[key].(type) {
	case int:
		return v, true
	case float64:
		return int(v), true
	default:
		return 0, false
	}
}

// int64Field reads a browser integer as int64 or returns zero when absent.
func int64Field(m map[string]any, key string) int64 {
	v, _ := int64FieldOK(m, key)
	return v
}

// int64FieldOK reads a browser integer as int64 while retaining field presence.
func int64FieldOK(m map[string]any, key string) (int64, bool) {
	switch v := m[key].(type) {
	case int:
		return int64(v), true
	case float64:
		return int64(v), true
	default:
		return 0, false
	}
}

// workerArgs selects one worker action and its workload dimensions.
type workerArgs struct {
	// scenario selects the worker action.
	scenario string
	// root selects the disposable shared OPFS directory.
	root string
	// worker identifies one concurrent worker.
	worker int
	// workers counts the expected publishers.
	workers int
	// iterations sets operation count or total bytes for this scenario.
	iterations int
	// batch sets batch size or readback window for this scenario.
	batch int
}

// workerResult contains one worker's terminal status and optional operation timings.
type workerResult struct {
	// scenario identifies the completed action.
	scenario string
	// worker identifies the worker instance.
	worker int
	// ok reports successful completion.
	ok bool
	// err contains the terminal error when unsuccessful.
	err string
	// durationMS measures total worker duration in milliseconds.
	durationMS int
	// opNanos measures only the requested storage operations in nanoseconds.
	opNanos int64
	// ops counts the measured storage operations.
	ops int64
}

// workerScriptEnvelope contains watchdog status and any completed worker results.
type workerScriptEnvelope struct {
	// status distinguishes success, timeout, and browser exception.
	status string
	// results contains completed worker results.
	results []workerResult
	// progress retains the last reported worker phase.
	progress workerScriptProgress
	// exception contains the browser exception text.
	exception string
	// timeoutMS records the evaluation deadline in milliseconds.
	timeoutMS int
}

// describe renders terminal status together with the last worker progress.
func (e *workerScriptEnvelope) describe(status string) string {
	// Describe the worker script status, progress, and browser exception.
	parts := []string{"opfs worker script " + status}
	if e.timeoutMS != 0 {
		parts = append(parts, "after "+strconv.Itoa(e.timeoutMS)+"ms")
	}
	parts = append(parts, "last progress "+e.progress.describe())
	if e.exception == "" {
		parts = append(parts, "browser exception=none")
	} else {
		parts = append(parts, "browser exception="+e.exception)
	}
	return strings.Join(parts, ": ")
}

// workerScriptProgress retains the latest worker phase and optional progress coordinates.
type workerScriptProgress struct {
	// scenario identifies the reporting action.
	scenario string
	// worker identifies the reporting worker when hasWorker is set.
	worker int
	// hasWorker distinguishes worker zero from an absent worker.
	hasWorker bool
	// phase records the current operation stage.
	phase string
	// offset records completed progress when hasOffset is set.
	offset int
	// hasOffset distinguishes progress zero from absent progress.
	hasOffset bool
	// total records the target count when hasTotal is set.
	total int
	// hasTotal distinguishes a zero target from an absent target.
	hasTotal bool
}

// describe renders the latest progress coordinates for failure diagnostics.
func (p workerScriptProgress) describe() string {
	// Describe the available worker progress coordinates for diagnostics.
	if p.scenario == "" && !p.hasWorker && p.phase == "" && !p.hasOffset && !p.hasTotal {
		return "none"
	}
	parts := make([]string, 0, 5)
	if p.scenario != "" {
		parts = append(parts, "scenario="+p.scenario)
	}
	if p.hasWorker {
		parts = append(parts, "worker="+strconv.Itoa(p.worker))
	}
	if p.phase != "" {
		parts = append(parts, "phase="+p.phase)
	}
	if p.hasOffset {
		parts = append(parts, "offset="+strconv.Itoa(p.offset))
	}
	if p.hasTotal {
		parts = append(parts, "total="+strconv.Itoa(p.total))
	}

	return strings.Join(parts, " ")
}

// chrometestGPUEnabled reports whether the operator opted into the full
// GPU-optimized chromium channel (E2E_CHROMIUM_GPU) instead of the
// CPU-rendered headless shell.
func chrometestGPUEnabled() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("E2E_CHROMIUM_GPU"))) {
	case "false", "0", "no", "off":
		return false
	default:
		return true
	}
}

// resetRoot creates a fresh scenario root and clears it before use.
func (s *chromeSession) resetRoot(t testing.TB, prefix string) string {
	// Give the scenario its own directory and require the clear worker to succeed.
	t.Helper()
	root := prefix + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{scenario: "clear", root: root})
	return root
}

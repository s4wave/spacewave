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

func TestOpfsChromeLargeWriteReadList(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
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
		iterations: 8 * 1024 * 1024,
		batch:      4,
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
		batch:      1,
	})
}

func TestOpfsChromeTinyGoLargeBlockShardBatch(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo blockshard large-upload path", tinyGoEnv)
	}

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-large-block-batch-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "large-block-batch",
		root:       root,
		iterations: 68056093,
		batch:      96,
		shards:     1,
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

func TestOpfsChromeWebLockCancellation(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-lock-cancel-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "counter-init",
		root:     root,
	})

	s.runHeldLockCheck(t, workerArgs{
		scenario: "counter-hold",
		root:     root,
	}, workerArgs{
		scenario: "counter-timeout-lock",
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

func TestOpfsChromeVolumeRuntimeResetsCurrentV1Root(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-volume-reset-v1-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "volume-runtime-seed-current-v1",
		root:     root,
		shards:   defaultShards,
	})
	s.runWorker(t, workerArgs{
		scenario: "volume-runtime-verify-reset",
		root:     root,
		shards:   defaultShards,
	})
}

func TestOpfsChromeVolumeRuntimeResetsIncompatibleRoot(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-volume-reset-incompat-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "volume-runtime-seed-incompatible",
		root:     root,
		shards:   defaultShards,
	})
	s.runWorker(t, workerArgs{
		scenario: "volume-runtime-verify-reset",
		root:     root,
		shards:   defaultShards,
	})
}

func TestOpfsChromeVolumeRuntimeResetsUnknownRoot(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-volume-reset-unknown-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "volume-runtime-seed-unknown",
		root:     root,
		shards:   defaultShards,
	})
	s.runWorker(t, workerArgs{
		scenario: "volume-runtime-verify-reset",
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
self.__BLDR_TINYGO_OPFS_WRITE_SESSIONS = new Map()
self.__BLDR_TINYGO_OPFS_WRITE_SESSION_NEXT_ID = 1
self.__BLDR_TINYGO_WEB_LOCK_RELEASES = new Map()
self.__BLDR_TINYGO_WEB_LOCK_RELEASE_NEXT_ID = 1
self.__BLDR_TINYGO_WEB_LOCK_RELEASE_OPS = new Map()
self.__BLDR_TINYGO_WEB_LOCK_REQUESTS = new Map()
self.__BLDR_TINYGO_COPY_BYTES = (bytes) => {
  if (!(bytes instanceof Uint8Array)) {
    throw new TypeError('expected Uint8Array')
  }
  const copy = new Uint8Array(bytes.byteLength)
  copy.set(bytes)
  return copy
}
self.__BLDR_TINYGO_STORE_BYTES = (bytes) => {
  const id = self.__BLDR_TINYGO_STORED_BYTES_NEXT_ID++
  self.__BLDR_TINYGO_STORED_BYTES.set(id, bytes)
  return id
}
self.__BLDR_TINYGO_STORE_OPFS_WRITE_SESSION = (writable) => {
  const id = self.__BLDR_TINYGO_OPFS_WRITE_SESSION_NEXT_ID++
  self.__BLDR_TINYGO_OPFS_WRITE_SESSIONS.set(id, { writable, written: 0 })
  return id
}
self.__BLDR_TINYGO_TAKE_OPFS_WRITE_SESSION = (id) => {
  const session = self.__BLDR_TINYGO_OPFS_WRITE_SESSIONS.get(id)
  self.__BLDR_TINYGO_OPFS_WRITE_SESSIONS.delete(id)
  return session
}
self.__BLDR_TINYGO_CLOSE_OPFS_WRITABLE_QUIETLY = (writable) => {
  if (!writable) {
    return
  }
  try {
    void writable.close().catch(() => {})
  } catch {
  }
}
self.__BLDR_TINYGO_EXPORT = (go, name) => {
  const fn = go._inst?.exports?.[name]
  if (typeof fn !== 'function') {
    throw new Error('missing TinyGo export ' + name)
  }
  return fn
}
self.__BLDR_TINYGO_READ_STRING = (go, ptr, len) => {
  const memory = go._inst?.exports?.memory
  if (!(memory instanceof WebAssembly.Memory)) {
    throw new Error('TinyGo runtime memory is not initialized')
  }
  return new TextDecoder().decode(new Uint8Array(memory.buffer, ptr >>> 0, len))
}
self.__BLDR_TINYGO_MEMORY_VIEW = (go, ptr, len) => {
  const memory = go._inst?.exports?.memory
  if (!(memory instanceof WebAssembly.Memory)) {
    throw new Error('TinyGo runtime memory is not initialized')
  }
  return new Uint8Array(memory.buffer, ptr >>> 0, len)
}
self.__BLDR_TINYGO_UNBOX_VALUE = (go, rawRef) => {
  const ref = typeof rawRef === 'bigint' ? rawRef : BigInt(rawRef)
  const nanHead = 0x7ff80000n
  if (((ref >> 32n) & nanHead) !== nanHead) {
    throw new Error('TinyGo numeric js.Value refs are unsupported here')
  }
  const id = Number(ref & 0xffffffffn)
  const value = go._values?.[id]
  if (value === undefined) {
    throw new Error('TinyGo js.Value ref ' + id + ' is unavailable')
  }
  return value
}
self.__BLDR_TINYGO_BOX_VALUE = (go, value) => {
  const nanHead = 0x7ff80000n
  if (typeof value === 'number') {
    if (Number.isNaN(value)) {
      return nanHead << 32n
    }
    if (value === 0) {
      return (nanHead << 32n) | 1n
    }
    const buf = new ArrayBuffer(8)
    const view = new DataView(buf)
    view.setFloat64(0, value, true)
    return view.getBigInt64(0, true)
  }
  switch (value) {
    case undefined:
      return 0n
    case null:
      return (nanHead << 32n) | 2n
    case true:
      return (nanHead << 32n) | 3n
    case false:
      return (nanHead << 32n) | 4n
  }
  if (!go._values || !go._ids || !go._goRefCounts || !go._idPool) {
    throw new Error('TinyGo js.Value table is not initialized')
  }
  let id = go._ids.get(value)
  if (id === undefined) {
    id = go._idPool.pop()
    if (id === undefined) {
      id = BigInt(go._values.length)
    }
    const index = Number(id)
    go._values[index] = value
    go._goRefCounts[index] = 0
    go._ids.set(value, id)
  }
  go._goRefCounts[Number(id)]++
  let typeFlag = 1n
  switch (typeof value) {
    case 'string':
      typeFlag = 2n
      break
    case 'symbol':
      typeFlag = 3n
      break
    case 'function':
      typeFlag = 4n
      break
  }
  return id | ((nanHead | typeFlag) << 32n)
}
self.__BLDR_TINYGO_OPFS_RESOLVE_REF = (opID, value) => {
  const go = self.__BLDR_TINYGO_CURRENT_GO
  const ref = self.__BLDR_TINYGO_BOX_VALUE(go, value)
  self.__BLDR_TINYGO_OPFS_RESOLVE(
    opID,
    Number((ref >> 32n) & 0xffffffffn),
    Number(ref & 0xffffffffn),
  )
}
self.__BLDR_TINYGO_DEFER_QUEUE = []
self.__BLDR_TINYGO_DEFER_SCHEDULED = false
self.__BLDR_TINYGO_DEFER_CHANNEL = new MessageChannel()
self.__BLDR_TINYGO_DEFER_CHANNEL.port1.onmessage = () => {
  self.__BLDR_TINYGO_DEFER_SCHEDULED = false
  const cb = self.__BLDR_TINYGO_DEFER_QUEUE.shift()
  if (cb) {
    cb()
  }
  if (self.__BLDR_TINYGO_DEFER_QUEUE.length !== 0) {
    self.__BLDR_TINYGO_DEFER_SCHEDULED = true
    self.__BLDR_TINYGO_DEFER_CHANNEL.port2.postMessage(undefined)
  }
}
self.__BLDR_TINYGO_DEFER = (cb) => {
  self.__BLDR_TINYGO_DEFER_QUEUE.push(cb)
  if (!self.__BLDR_TINYGO_DEFER_SCHEDULED) {
    self.__BLDR_TINYGO_DEFER_SCHEDULED = true
    self.__BLDR_TINYGO_DEFER_CHANNEL.port2.postMessage(undefined)
  }
}
self.__BLDR_TINYGO_CALL_EXPORT = (go, fn, ...args) => {
  fn(...args)
  const scheduler = go._inst?.exports?.go_scheduler
  if (typeof scheduler === 'function') {
    self.__BLDR_TINYGO_DEFER(() => scheduler())
    return
  }
  if (typeof go._resume === 'function') {
    self.__BLDR_TINYGO_DEFER(() => go._resume.call(go))
  }
}
self.__BLDR_TINYGO_OPFS_RESOLVE = (opID, ...values) => {
  const go = self.__BLDR_TINYGO_CURRENT_GO
  const resolve = self.__BLDR_TINYGO_EXPORT(go, 'BLDR_OPFS_HELPER_RESOLVE')
  self.__BLDR_TINYGO_DEFER(() => {
    self.__BLDR_TINYGO_CALL_EXPORT(go, resolve, opID, values.length, values[0] ?? 0, values[1] ?? 0)
  })
}
self.__BLDR_TINYGO_OPFS_REJECT = (opID, code) => {
  const go = self.__BLDR_TINYGO_CURRENT_GO
  const reject = self.__BLDR_TINYGO_EXPORT(go, 'BLDR_OPFS_HELPER_REJECT')
  self.__BLDR_TINYGO_DEFER(() => self.__BLDR_TINYGO_CALL_EXPORT(go, reject, opID, code))
}
self.__BLDR_TINYGO_STORE_WEB_LOCK_RELEASE = (release, opID) => {
  const id = self.__BLDR_TINYGO_WEB_LOCK_RELEASE_NEXT_ID++
  self.__BLDR_TINYGO_WEB_LOCK_RELEASES.set(id, release)
  if (opID !== undefined) {
    self.__BLDR_TINYGO_WEB_LOCK_RELEASE_OPS.set(id, opID)
    const request = self.__BLDR_TINYGO_WEB_LOCK_REQUESTS.get(opID)
    if (request) {
      request.releaseID = id
    }
  }
  return id
}
self.__BLDR_TINYGO_RELEASE_WEB_LOCK = (releaseID) => {
  const opID = self.__BLDR_TINYGO_WEB_LOCK_RELEASE_OPS.get(releaseID)
  self.__BLDR_TINYGO_WEB_LOCK_RELEASE_OPS.delete(releaseID)
  if (opID !== undefined) {
    self.__BLDR_TINYGO_WEB_LOCK_REQUESTS.delete(opID)
  }
  const release = self.__BLDR_TINYGO_WEB_LOCK_RELEASES.get(releaseID)
  self.__BLDR_TINYGO_WEB_LOCK_RELEASES.delete(releaseID)
  if (!release) {
    return 0
  }
  release()
  return 1
}
self.__BLDR_TINYGO_CANCEL_WEB_LOCK = (opID) => {
  const request = self.__BLDR_TINYGO_WEB_LOCK_REQUESTS.get(opID)
  if (!request) {
    return 0
  }
  request.canceled = true
  if (request.releaseID !== undefined) {
    const release = self.__BLDR_TINYGO_WEB_LOCK_RELEASES.get(request.releaseID)
    self.__BLDR_TINYGO_RELEASE_WEB_LOCK(request.releaseID)
    return release ? 1 : 0
  }
  if (request.abort) {
    request.abort.abort()
  }
  return 1
}
self.BLDR_TINYGO_NEW_BYTES ??= (len) => new Uint8Array(len)
self.BLDR_TINYGO_TAKE_STORED_BYTES ??= (id) => {
  const bytes = self.__BLDR_TINYGO_STORED_BYTES.get(id)
  self.__BLDR_TINYGO_STORED_BYTES.delete(id)
  return bytes
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
self.BLDR_TINYGO_PUSH_BYTES ??= (sink, bytes) => {
  try {
    sink.push(self.__BLDR_TINYGO_COPY_BYTES(bytes))
    return true
  } catch {
    return false
  }
}
self.BLDR_TINYGO_POST_BYTES ??= (port, bytes) => {
  try {
    port.postMessage(self.__BLDR_TINYGO_COPY_BYTES(bytes))
    return true
  } catch {
    return false
  }
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
self.BLDR_OPFS_READ_FILE ??= (dir, name, opID) => {
  dir.getFileHandle(name)
    .then((handle) => handle.getFile())
    .then((file) => file.arrayBuffer())
    .then((buf) => {
      const bytes = new Uint8Array(buf)
      self.__BLDR_TINYGO_OPFS_RESOLVE(opID, self.__BLDR_TINYGO_STORE_BYTES(bytes), bytes.byteLength)
    })
    .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
}
self.BLDR_OPFS_READ_AT ??= (handle, dst, off, opID) => {
  const dstLen = dst.byteLength
  handle.getFile()
    .then(async (file) => {
      if (off >= file.size || dstLen === 0) {
        self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 0)
        return
      }
      const end = Math.min(off + dstLen, file.size)
      const buf = await file.slice(off, end).arrayBuffer()
      const bytes = new Uint8Array(buf)
      if (bytes.byteLength !== 0) {
        dst.subarray(0, bytes.byteLength).set(bytes)
      }
      self.__BLDR_TINYGO_OPFS_RESOLVE(opID, bytes.byteLength)
    })
    .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
}
self.BLDR_OPFS_LIST_DIRECTORY ??= (dir, opID) => {
  ;(async () => {
    const names = []
    for await (const [name] of dir.entries()) {
      names.push(name)
    }
    const bytes = self.__BLDR_TINYGO_ENCODE_NAMES(names)
    self.__BLDR_TINYGO_OPFS_RESOLVE(opID, self.__BLDR_TINYGO_STORE_BYTES(bytes), bytes.byteLength)
  })().catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
}
self.BLDR_OPFS_WRITE_AT ??= (handle, data, off, keepExisting, opID) => {
  const writeData = self.__BLDR_TINYGO_COPY_BYTES(data)
  let writable
  const opts = keepExisting ? { keepExistingData: true } : undefined
  const writablePromise = opts ? handle.createWritable(opts) : handle.createWritable()
  writablePromise
    .then(async (next) => {
      writable = next
      if (off !== 0) {
        await writable.seek(off)
      }
      if (writeData.byteLength !== 0) {
        await writable.write(writeData)
      }
      await writable.close()
      self.__BLDR_TINYGO_OPFS_RESOLVE(opID, writeData.byteLength)
    })
    .catch((reason) => {
      if (writable) {
        void writable.close().catch(() => {})
      }
      self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
    })
}
self.BLDR_OPFS_WRITE_FILE ??= (dir, name, data, opID) => {
  const writeData = self.__BLDR_TINYGO_COPY_BYTES(data)
  let writable
  dir.getFileHandle(name, { create: true })
    .then(async (handle) => {
      writable = await handle.createWritable()
      if (writeData.byteLength !== 0) {
        await writable.write(writeData)
      }
      await writable.close()
      self.__BLDR_TINYGO_OPFS_RESOLVE(opID, writeData.byteLength)
    })
    .catch((reason) => {
      if (writable) {
        void writable.close().catch(() => {})
      }
      self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
    })
}
self.BLDR_OPFS_WRITE_FILE_BEGIN ??= (dir, name, opID) => {
  dir.getFileHandle(name, { create: true })
    .then((handle) => handle.createWritable())
    .then((writable) => {
      self.__BLDR_TINYGO_OPFS_RESOLVE(opID, self.__BLDR_TINYGO_STORE_OPFS_WRITE_SESSION(writable))
    })
    .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
}
self.BLDR_OPFS_WRITE_FILE_CHUNK ??= (sessionID, data, opID) => {
  const session = self.__BLDR_TINYGO_OPFS_WRITE_SESSIONS.get(sessionID)
  if (!session) {
    self.__BLDR_TINYGO_OPFS_REJECT(opID, 1)
    return
  }
  let writeData
  try {
    writeData = self.__BLDR_TINYGO_COPY_BYTES(data)
  } catch {
    self.__BLDR_TINYGO_OPFS_REJECT(opID, 0)
    return
  }
  session.writable.write(writeData)
    .then(() => {
      session.written += writeData.byteLength
      self.__BLDR_TINYGO_OPFS_RESOLVE(opID, writeData.byteLength)
    })
    .catch((reason) => {
      self.__BLDR_TINYGO_OPFS_WRITE_SESSIONS.delete(sessionID)
      void session.writable.close().catch(() => {})
      self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
    })
}
self.BLDR_OPFS_WRITE_FILE_CLOSE ??= (sessionID, opID) => {
  const session = self.__BLDR_TINYGO_TAKE_OPFS_WRITE_SESSION(sessionID)
  if (!session) {
    self.__BLDR_TINYGO_OPFS_REJECT(opID, 1)
    return
  }
  session.writable.close()
    .then(() => self.__BLDR_TINYGO_OPFS_RESOLVE(opID, session.written))
    .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
}
self.BLDR_OPFS_WRITE_FILE_ABORT ??= (sessionID) => {
  const session = self.__BLDR_TINYGO_TAKE_OPFS_WRITE_SESSION(sessionID)
  if (!session) {
    return false
  }
  void session.writable.close().catch(() => {})
  return true
}

self.onmessage = async (event) => {
  const args = event.data
  const go = new Go()
  self.__BLDR_TINYGO_CURRENT_GO = go
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
      crypto.getRandomValues(new Uint8Array(memory.buffer, ptr >>> 0, len))
    }
  }
  if (go.importObject.gojs) {
    go.importObject.gojs['bldr.opfs.acquireWebLock'] ??= (opID, namePtr, nameLen, exclusive, ifAvailable) => {
      const resolve = self.__BLDR_TINYGO_EXPORT(go, 'BLDR_OPFS_WEB_LOCK_RESOLVE')
      const reject = self.__BLDR_TINYGO_EXPORT(go, 'BLDR_OPFS_WEB_LOCK_REJECT')
      const locks = self.navigator?.locks
      if (!locks) {
        self.__BLDR_TINYGO_DEFER(() => self.__BLDR_TINYGO_CALL_EXPORT(go, reject, opID, 0))
        return
      }
      const opts = { mode: exclusive ? 'exclusive' : 'shared' }
      const abort = ifAvailable ? undefined : new AbortController()
      if (abort) {
        opts.signal = abort.signal
      }
      if (ifAvailable) {
        opts.ifAvailable = true
      }
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      const request = { abort }
      self.__BLDR_TINYGO_WEB_LOCK_REQUESTS.set(opID, request)
      locks.request(name, opts, (lock) => {
        if (ifAvailable && !lock) {
          self.__BLDR_TINYGO_WEB_LOCK_REQUESTS.delete(opID)
          self.__BLDR_TINYGO_DEFER(() => self.__BLDR_TINYGO_CALL_EXPORT(go, resolve, opID, 0, 0))
          return undefined
        }
        return new Promise((releaseLock) => {
          if (request.canceled) {
            releaseLock()
            self.__BLDR_TINYGO_WEB_LOCK_REQUESTS.delete(opID)
            return
          }
          const releaseID = self.__BLDR_TINYGO_STORE_WEB_LOCK_RELEASE(releaseLock, opID)
          self.__BLDR_TINYGO_DEFER(() => self.__BLDR_TINYGO_CALL_EXPORT(go, resolve, opID, releaseID, 1))
        })
      }).catch((reason) => {
        self.__BLDR_TINYGO_WEB_LOCK_REQUESTS.delete(opID)
        if (request.canceled) {
          return
        }
        self.__BLDR_TINYGO_DEFER(() => self.__BLDR_TINYGO_CALL_EXPORT(go, reject, opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
      })
    }
    go.importObject.gojs['bldr.opfs.cancelWebLock'] ??= (opID) => {
      return self.__BLDR_TINYGO_CANCEL_WEB_LOCK(opID)
    }
    go.importObject.gojs['bldr.opfs.releaseWebLock'] ??= (releaseID) => {
      return self.__BLDR_TINYGO_RELEASE_WEB_LOCK(releaseID)
    }
    go.importObject.gojs['bldr.opfs.getRootRef'] ??= (opID) => {
      self.navigator.storage.getDirectory()
        .then((dir) => self.__BLDR_TINYGO_OPFS_RESOLVE_REF(opID, dir))
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.getDirectoryRef'] ??= (opID, parentRef, namePtr, nameLen, create) => {
      const parent = self.__BLDR_TINYGO_UNBOX_VALUE(go, parentRef)
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      parent.getDirectoryHandle(name, { create: Boolean(create) })
        .then((dir) => self.__BLDR_TINYGO_OPFS_RESOLVE_REF(opID, dir))
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.openFileRef'] ??= (opID, dirRef, namePtr, nameLen, create) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      const opts = create ? { create: true } : undefined
      const filePromise = opts ? dir.getFileHandle(name, opts) : dir.getFileHandle(name)
      filePromise
        .then((handle) => self.__BLDR_TINYGO_OPFS_RESOLVE_REF(opID, handle))
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.fileExistsRef'] ??= (opID, dirRef, namePtr, nameLen) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      dir.getFileHandle(name)
        .then(() => self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 1))
        .catch((reason) => {
          const code = self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)
          if (code === 1) {
            self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 0)
            return
          }
          self.__BLDR_TINYGO_OPFS_REJECT(opID, code)
        })
    }
    go.importObject.gojs['bldr.opfs.deleteEntryRef'] ??= (opID, dirRef, namePtr, nameLen, recursive) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      dir.removeEntry(name, { recursive: Boolean(recursive) })
        .then(() => self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 1))
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.yieldMicrotask'] ??= (opID) => {
      queueMicrotask(() => self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 1))
    }
    go.importObject.gojs['bldr.opfs.sizeRef'] ??= (opID, handleRef) => {
      const handle = self.__BLDR_TINYGO_UNBOX_VALUE(go, handleRef)
      handle.getFile()
        .then((file) => self.__BLDR_TINYGO_OPFS_RESOLVE(opID, file.size))
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.truncateRef'] ??= (opID, handleRef, size) => {
      const handle = self.__BLDR_TINYGO_UNBOX_VALUE(go, handleRef)
      let writable
      handle.createWritable({ keepExistingData: true })
        .then(async (next) => {
          writable = next
          await writable.truncate(Number(size))
          await writable.close()
          self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 1)
        })
        .catch((reason) => {
          self.__BLDR_TINYGO_CLOSE_OPFS_WRITABLE_QUIETLY(writable)
          self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
        })
    }
    go.importObject.gojs['bldr.opfs.takeStoredBytes'] ??= (bytesID, ptr, len) => {
      const bytes = self.BLDR_TINYGO_TAKE_STORED_BYTES(bytesID)
      if (!bytes || bytes.byteLength !== len) {
        return 0
      }
      if (len !== 0) {
        self.__BLDR_TINYGO_MEMORY_VIEW(go, ptr, len).set(bytes)
      }
      return 1
    }
    go.importObject.gojs['bldr.opfs.readFileRef'] ??= (opID, dirRef, namePtr, nameLen) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      dir.getFileHandle(name)
        .then((handle) => handle.getFile())
        .then((file) => file.arrayBuffer())
        .then((buf) => {
          const bytes = new Uint8Array(buf)
          self.__BLDR_TINYGO_OPFS_RESOLVE(opID, self.__BLDR_TINYGO_STORE_BYTES(bytes), bytes.byteLength)
        })
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.readAtRef'] ??= (opID, handleRef, dstPtr, dstLen, off) => {
      const handle = self.__BLDR_TINYGO_UNBOX_VALUE(go, handleRef)
      const offset = Number(off)
      handle.getFile()
        .then(async (file) => {
          if (offset >= file.size || dstLen === 0) {
            self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 0)
            return
          }
          const end = Math.min(offset + dstLen, file.size)
          const buf = await file.slice(offset, end).arrayBuffer()
          const bytes = new Uint8Array(buf)
          if (bytes.byteLength !== 0) {
            self.__BLDR_TINYGO_MEMORY_VIEW(go, dstPtr, bytes.byteLength).set(bytes)
          }
          self.__BLDR_TINYGO_OPFS_RESOLVE(opID, bytes.byteLength)
        })
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.listDirectoryRef'] ??= (opID, dirRef) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      ;(async () => {
        const names = []
        for await (const [name] of dir.entries()) {
          names.push(name)
        }
        const bytes = self.__BLDR_TINYGO_ENCODE_NAMES(names)
        self.__BLDR_TINYGO_OPFS_RESOLVE(opID, self.__BLDR_TINYGO_STORE_BYTES(bytes), bytes.byteLength)
      })().catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
    }
    go.importObject.gojs['bldr.opfs.writeAtRef'] ??= (opID, handleRef, dataPtr, dataLen, off, keepExisting) => {
      const handle = self.__BLDR_TINYGO_UNBOX_VALUE(go, handleRef)
      let writable
      try {
        const writeData = self.__BLDR_TINYGO_COPY_BYTES(self.__BLDR_TINYGO_MEMORY_VIEW(go, dataPtr, dataLen))
        const opts = keepExisting ? { keepExistingData: true } : undefined
        const writablePromise = opts ? handle.createWritable(opts) : handle.createWritable()
        writablePromise
          .then(async (next) => {
            writable = next
            const offset = Number(off)
            if (offset !== 0) {
              await writable.seek(offset)
            }
            if (writeData.byteLength !== 0) {
              await writable.write(writeData)
            }
            await writable.close()
            self.__BLDR_TINYGO_OPFS_RESOLVE(opID, writeData.byteLength)
          })
          .catch((reason) => {
            self.__BLDR_TINYGO_CLOSE_OPFS_WRITABLE_QUIETLY(writable)
            self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
          })
      } catch (reason) {
        self.__BLDR_TINYGO_CLOSE_OPFS_WRITABLE_QUIETLY(writable)
        self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
      }
    }
    go.importObject.gojs['bldr.opfs.writeFileBeginRef'] ??= (opID, dirRef, namePtr, nameLen) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      try {
        dir.getFileHandle(name, { create: true })
          .then((handle) => handle.createWritable())
          .then((writable) => self.__BLDR_TINYGO_OPFS_RESOLVE(opID, self.__BLDR_TINYGO_STORE_OPFS_WRITE_SESSION(writable)))
          .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
      } catch (reason) {
        self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
      }
    }
    go.importObject.gojs['bldr.opfs.writeFileChunkRef'] ??= (opID, sessionID, dataPtr, dataLen) => {
      const session = self.__BLDR_TINYGO_OPFS_WRITE_SESSIONS.get(sessionID)
      if (!session) {
        self.__BLDR_TINYGO_OPFS_REJECT(opID, 1)
        return
      }
      let writeData
      try {
        writeData = self.__BLDR_TINYGO_COPY_BYTES(self.__BLDR_TINYGO_MEMORY_VIEW(go, dataPtr, dataLen))
      } catch {
        self.__BLDR_TINYGO_OPFS_REJECT(opID, 0)
        return
      }
      try {
        session.writable.write(writeData)
          .then(() => {
            session.written += writeData.byteLength
            self.__BLDR_TINYGO_OPFS_RESOLVE(opID, writeData.byteLength)
          })
          .catch((reason) => {
            self.__BLDR_TINYGO_OPFS_WRITE_SESSIONS.delete(sessionID)
            self.__BLDR_TINYGO_CLOSE_OPFS_WRITABLE_QUIETLY(session.writable)
            self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
          })
      } catch (reason) {
        self.__BLDR_TINYGO_OPFS_WRITE_SESSIONS.delete(sessionID)
        self.__BLDR_TINYGO_CLOSE_OPFS_WRITABLE_QUIETLY(session.writable)
        self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
      }
    }
    go.importObject.gojs['bldr.opfs.writeFileCloseRef'] ??= (opID, sessionID) => {
      const session = self.__BLDR_TINYGO_TAKE_OPFS_WRITE_SESSION(sessionID)
      if (!session) {
        self.__BLDR_TINYGO_OPFS_REJECT(opID, 1)
        return
      }
      try {
        session.writable.close()
          .then(() => self.__BLDR_TINYGO_OPFS_RESOLVE(opID, session.written))
          .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
      } catch (reason) {
        self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
      }
    }
    go.importObject.gojs['bldr.opfs.writeFileAbortRef'] ??= (sessionID) => {
      const session = self.__BLDR_TINYGO_TAKE_OPFS_WRITE_SESSION(sessionID)
      if (!session) {
        return 0
      }
      self.__BLDR_TINYGO_CLOSE_OPFS_WRITABLE_QUIETLY(session.writable)
      return 1
    }
    go.importObject.gojs['bldr.opfs.broadcastChannelNewRef'] ??= (namePtr, nameLen) => {
      const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
      return self.__BLDR_TINYGO_BOX_VALUE(go, new BroadcastChannel(name))
    }
    go.importObject.gojs['bldr.opfs.broadcastSendRef'] ??= (channelRef, shardID, generationHi, generationLo) => {
      const channel = self.__BLDR_TINYGO_UNBOX_VALUE(go, channelRef)
      const msg = new Uint8Array(10)
      const sid = shardID & 0xffff
      const hi = generationHi >>> 0
      const lo = generationLo >>> 0
      msg[0] = (sid >>> 8) & 0xff
      msg[1] = sid & 0xff
      msg[2] = (hi >>> 24) & 0xff
      msg[3] = (hi >>> 16) & 0xff
      msg[4] = (hi >>> 8) & 0xff
      msg[5] = hi & 0xff
      msg[6] = (lo >>> 24) & 0xff
      msg[7] = (lo >>> 16) & 0xff
      msg[8] = (lo >>> 8) & 0xff
      msg[9] = lo & 0xff
      channel.postMessage(msg)
    }
    go.importObject.gojs['bldr.opfs.broadcastCloseRef'] ??= (channelRef) => {
      self.__BLDR_TINYGO_UNBOX_VALUE(go, channelRef).close()
    }
  }
  const res = await WebAssembly.instantiateStreaming(fetch('/testprog.wasm'), go.importObject)
  await go.run(res.instance)
}
`

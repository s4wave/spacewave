package chrometest

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/pkg/errors"
	playwright "github.com/playwright-community/playwright-go"
	"github.com/s4wave/spacewave/bldr/util/gocompiler"
)

const (
	runEnv               = "RUN_OPFS_CHROME_TEST"
	profileEnv           = "RUN_OPFS_CHROME_PROFILE"
	tinyGoEnv            = "RUN_OPFS_CHROME_TINYGO"
	tinyGoProfileEnv     = "RUN_OPFS_CHROME_TINYGO_PROFILE"
	tinyGoOptEnv         = "RUN_OPFS_CHROME_TINYGO_OPT"
	tinyGoGCEnv          = "RUN_OPFS_CHROME_TINYGO_GC"
	tinyGoLLVMEnv        = "RUN_OPFS_CHROME_TINYGO_LLVM_FEATURES"
	tinyGoPanicEnv       = "RUN_OPFS_CHROME_TINYGO_PANIC"
	tinyGoSchedulerEnv   = "RUN_OPFS_CHROME_TINYGO_SCHEDULER"
	tinyGoStackEnv       = "RUN_OPFS_CHROME_TINYGO_STACK_SIZE"
	resourceReadChunkEnv = "RUN_OPFS_CHROME_RESOURCE_READ_CHUNK"
	largeSizeEnv         = "RUN_OPFS_CHROME_LARGE_SIZE"
	tinyGoProfileCustom  = "custom"
	tinyGoTargetDefault  = "target-default"
	tinyGoBldrFeatures   = "bldr-features"
	chromeSmoke          = "smoke"
	chromeStress         = "stress"
	defaultShards        = 4
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

// TestOpfsChromeMaterializeFanout measures how the block feed pattern changes
// first-run manifest materialization cost. The pre-refactor scheduler fed
// bucket_lookup.CopyObjectToBucket at maxConcurrency=1 (one foreground-awaited
// PutBlock per block), so each block became its own OPFS Publish with the full
// Web Lock + sync-access-handle + manifest-write tax. This drives identical
// blocks through the real blockshard engine four ways and logs write-only time
// and the Publish-count proxy so the serial vs coalesced gap is measured, not
// asserted by a flaky ratio. The async-serial mode is the landed async-default
// BlockStore.PutBlock feed: a strictly serial PutBackground walk fenced by one
// Sync, the path the storage refactor put on the default so a one-at-a-time copy
// loop coalesces partially even without caller concurrency or batching.
func TestOpfsChromeMaterializeFanout(t *testing.T) {
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	blocks := envIntDefault(t, "OPFS_MATERIALIZE_BLOCKS", 512)
	concurrency := envIntDefault(t, "OPFS_MATERIALIZE_CONCURRENCY", 16)
	batchSize := envIntDefault(t, "OPFS_MATERIALIZE_BATCH", 64)

	modes := []struct {
		scenario string
		batch    int
	}{
		{"materialize-fanout-serial", 1},
		{"materialize-fanout-concurrent", concurrency},
		{"materialize-fanout-batched", batchSize},
		{"materialize-fanout-async-serial", 1},
	}

	type row struct {
		scenario   string
		durationMS int
		writeMS    int
		publishGen int
	}
	rows := make([]row, 0, len(modes))
	for _, m := range modes {
		root := "opfs-chrome-materialize-" + time.Now().Format("150405.000000000")
		s.runWorker(t, workerArgs{scenario: "clear", root: root})
		res := s.runWorker(t, workerArgs{
			scenario:   m.scenario,
			root:       root,
			iterations: blocks,
			batch:      m.batch,
			shards:     defaultShards,
		})
		rows = append(rows, row{m.scenario, res.durationMS, res.writeMS, res.publishGen})
		t.Logf("materialize-fanout scenario=%s blocks=%d batch=%d writeMs=%d durationMs=%d publishGen=%d",
			m.scenario, blocks, m.batch, res.writeMS, res.durationMS, res.publishGen)
	}

	serial := rows[0]
	for _, r := range rows[1:] {
		if r.writeMS > 0 && serial.writeMS > 0 {
			t.Logf("materialize-fanout %s vs serial: writeMs %d -> %d (%.2fx faster), publishGen %d -> %d",
				r.scenario, serial.writeMS, r.writeMS,
				float64(serial.writeMS)/float64(r.writeMS), serial.publishGen, r.publishGen)
		}
	}
}

// TestOpfsChromeCopyWalkWrapperConcurrency probes whether the production
// AccessWorldState -> CopyObjectToBucket -> WalkObjectBlocks wrapper deadlocks
// at a raised maxConcurrency on real OPFS under native Go-WASM, isolating a
// wrapper bug from a GoScript-scheduler bug (the GoScript compiler is held out
// of this path). It runs the copy-walk-wrapper-concurrency scenario, which
// builds a wide source DAG in a bucket distinct from the dest world root, then
// copies it via the production nested-access pattern twice over fresh source
// objects: first at maxConcurrency=1 (control) and then at the suspect
// concurrency (default 16). A wrapper deadlock at the raised concurrency would
// hang the copy, which the chrome harness context deadline turns into a test
// failure. Pass concurrency via OPFS_COPYWALK_CONCURRENCY and source input bytes
// via OPFS_COPYWALK_BLOCKS.
func TestOpfsChromeCopyWalkWrapperConcurrency(t *testing.T) {
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	concurrency := envIntDefault(t, "OPFS_COPYWALK_CONCURRENCY", 16)
	inputBytes := envIntDefault(t, "OPFS_COPYWALK_BLOCKS", 64*1024)

	root := "opfs-chrome-copywalk-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{scenario: "clear", root: root})
	res := s.runWorker(t, workerArgs{
		scenario:   "copy-walk-wrapper-concurrency",
		root:       root,
		iterations: inputBytes,
		batch:      concurrency,
		shards:     defaultShards,
	})
	t.Logf("copy-walk-wrapper concurrency=%d inputBytes=%d durationMs=%d",
		concurrency, inputBytes, res.durationMS)
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
	s := h.newPersistentSession(t, "volume-reset-current-v1")
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
	s := h.newPersistentSession(t, "volume-reset-incompatible")
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
	s := h.newPersistentSession(t, "volume-reset-unknown")
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

func TestOpfsChromeTinyGoPipeWriteLoop(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo io.Pipe scheduling path", tinyGoEnv)
	}

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-pipe-write-loop-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario:   "pipe-write-loop",
		root:       root,
		iterations: 4 * 1024 * 1024,
	})
}

func TestOpfsChromeTinyGoSRPCEchoLoop(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise TinyGo SRPC unary call liveness", tinyGoEnv)
	}

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-srpc-echo-loop-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario:   "srpc-echo-loop",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 128),
		batch:      envIntDefault(t, resourceReadChunkEnv, 4096),
	})
}

func TestOpfsChromeTinyGoSRPCRpcStreamEchoLoop(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise TinyGo SRPC-over-rpcstream liveness", tinyGoEnv)
	}

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-srpc-rpcstream-echo-loop-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario:   "srpc-rpcstream-echo-loop",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 128),
		batch:      envIntDefault(t, resourceReadChunkEnv, 4096),
	})
}

func TestOpfsChromeTinyGoResourceEchoLoop(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise TinyGo resource-routed SRPC liveness", tinyGoEnv)
	}

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-resource-echo-loop-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario:   "resource-echo-loop",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 128),
		batch:      envIntDefault(t, resourceReadChunkEnv, 4096),
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
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
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
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      96,
		shards:     1,
	})
	s.runWorker(t, workerArgs{
		scenario:   "large-block-verify",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      96,
		shards:     1,
	})
}

func TestOpfsChromeTinyGoLargeBlockShardMultiShardBatch(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo blockshard large-upload path", tinyGoEnv)
	}

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-large-block-multishard-batch-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "large-block-batch",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      96,
		shards:     defaultShards,
	})
	s.runWorker(t, workerArgs{
		scenario:   "large-block-verify",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      96,
		shards:     defaultShards,
	})
}

func TestOpfsChromeTinyGoLargeBlockShardWithGCWalContention(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise TinyGo large Blockshard beside GC/WAL writes", tinyGoEnv)
	}

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-large-block-gc-wal-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorkers(t, []workerArgs{
		{
			scenario:   "large-block-batch",
			root:       root,
			iterations: envIntDefault(t, largeSizeEnv, 68056093),
			batch:      96,
			shards:     defaultShards,
		},
		{
			scenario:   "gc-wal-write-loop",
			root:       root,
			iterations: 16,
			shards:     defaultShards,
		},
	})
	s.runWorker(t, workerArgs{
		scenario:   "large-block-verify",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      96,
		shards:     defaultShards,
	})
	s.runWorker(t, workerArgs{
		scenario:   "gc-wal-verify",
		root:       root,
		iterations: 16,
		shards:     defaultShards,
	})
}

func TestOpfsChromeBlockShardCorruptCompactionInput(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-block-corrupt-compaction-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "block-corrupt-compaction",
		root:     root,
	})
}

func TestOpfsChromeBlockShardZeroSizeCompactionInput(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-block-zero-size-compaction-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "block-zero-size-compaction",
		root:     root,
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
	s.runWorker(t, workerArgs{
		scenario: "volume-runtime-delete-verify",
		root:     root,
		shards:   defaultShards,
	})
}

func TestOpfsChromeVolumeCoordinator(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-volume-coord-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "volume-coord-local",
		root:     root,
		shards:   defaultShards,
	})
	s.runWorkersStaged(t, []workerArgs{{
		scenario: "volume-coord-watch",
		root:     root,
		shards:   defaultShards,
	}}, []workerArgs{{
		scenario: "volume-coord-broadcast",
		root:     root,
		shards:   defaultShards,
	}})
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
		scenario: "volume-runtime-verify-incompatible-reset",
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
		scenario: "volume-runtime-verify-unknown-reset",
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

func TestOpfsChromeWorldCoordinatorMultiWriter(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-world-coord-multi-writer-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "world-coord-multi-writer",
		root:     root,
		shards:   defaultShards,
	})
}

func TestOpfsChromeWorldDeferredCrashRecovery(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-world-deferred-crash-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "world-deferred-crash-recovery",
		root:     root,
		shards:   defaultShards,
	})
}

func TestOpfsChromeTinyGoWorldLargeUnixFSUpload(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo UnixFS large-upload path", tinyGoEnv)
	}

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-world-large-unixfs-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "world-large-unixfs-upload",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		shards:     defaultShards,
	})
}

func TestOpfsChromeTinyGoWorldResourceLargeUnixFSUpload(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo UnixFS resource large-upload path", tinyGoEnv)
	}

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-world-resource-large-unixfs-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "world-resource-large-unixfs-upload",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      envInt(t, resourceReadChunkEnv),
		shards:     defaultShards,
	})
}

func TestOpfsChromeTinyGoWorldResourceDirectUploadTreeLargeUnixFSUpload(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo direct UploadTree path", tinyGoEnv)
	}

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-world-resource-direct-upload-tree-large-unixfs-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "world-resource-direct-upload-tree-large-unixfs-upload",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      envInt(t, resourceReadChunkEnv),
		shards:     defaultShards,
	})
}

func TestOpfsChromeTinyGoWorldControllerResourceLargeUnixFSUpload(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo controller bucket large-upload path", tinyGoEnv)
	}

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-world-controller-resource-large-unixfs-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "world-controller-resource-large-unixfs-upload",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      envInt(t, resourceReadChunkEnv),
		shards:     defaultShards,
	})
}

func TestOpfsChromeTinyGoWorldCloudOverlayResourceLargeUnixFSUpload(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo cloud-overlay large-upload path", tinyGoEnv)
	}

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-world-cloud-overlay-resource-large-unixfs-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "world-cloud-overlay-resource-large-unixfs-upload",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      envInt(t, resourceReadChunkEnv),
		shards:     defaultShards,
	})
}

func TestOpfsChromeTinyGoWorldCloudSyncResourceLargeUnixFSUpload(t *testing.T) {
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo cloud-sync large-upload path", tinyGoEnv)
	}

	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	root := "opfs-chrome-world-cloud-sync-resource-large-unixfs-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario: "clear",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario:   "world-cloud-sync-resource-large-unixfs-upload",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      envInt(t, resourceReadChunkEnv),
		shards:     defaultShards,
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

func envInt(t testing.TB, key string) int {
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

func envIntDefault(t testing.TB, key string, def int) int {
	t.Helper()
	val := envInt(t, key)
	if val == 0 {
		return def
	}
	return val
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
		t.Logf("worker scenario=%s worker=%d ok=%t duration=%dms", res.scenario, res.worker, res.ok, res.durationMS)
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
		envelope, err := resolveTinyGoBuildEnvelope()
		if err != nil {
			return err
		}
		args := append([]string{"build"}, envelope.args()...)
		args = append(args, "-o", out, "./db/opfs/chrometest/testprog")
		fmt.Fprintf(
			os.Stderr,
			"opfs chrometest wasm build: compiler=tinygo version=%q envelope=%s args=%q\n",
			tinyGoVersion(ctx),
			envelope.String(),
			args,
		)
		start := time.Now()
		cmd := exec.CommandContext(ctx, "tinygo", args...)
		cmd.Dir = root
		data, err := cmd.CombinedOutput()
		if err != nil {
			return errors.Errorf("tinygo build js/wasm failed: %v\n%s", err, data)
		}
		info, err := os.Stat(out)
		if err != nil {
			return errors.Wrap(err, "stat TinyGo wasm artifact")
		}
		fmt.Fprintf(
			os.Stderr,
			"opfs chrometest wasm build result: compiler=tinygo elapsed=%s artifact_bytes=%d wasm_features=%s\n",
			time.Since(start).Round(time.Millisecond),
			info.Size(),
			wasmFeatureEvidence(ctx, out),
		)
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

type tinyGoBuildEnvelope struct {
	profile       string
	target        string
	opt           string
	panicStrategy string
	gc            string
	scheduler     string
	stackSize     string
	llvmFeatures  string
}

func resolveTinyGoBuildEnvelope() (*tinyGoBuildEnvelope, error) {
	profile := strings.TrimSpace(os.Getenv(tinyGoProfileEnv))
	if profile == "" {
		profile = tinyGoProfileCustom
	}
	if profile != tinyGoProfileCustom && profile != tinyGoTargetDefault && profile != tinyGoBldrFeatures {
		return nil, errors.Errorf("unsupported %s=%q, expected custom, target-default, or bldr-features", tinyGoProfileEnv, profile)
	}

	env := &tinyGoBuildEnvelope{
		profile:       profile,
		target:        "wasm",
		opt:           strings.TrimSpace(os.Getenv(tinyGoOptEnv)),
		panicStrategy: strings.TrimSpace(os.Getenv(tinyGoPanicEnv)),
		gc:            strings.TrimSpace(os.Getenv(tinyGoGCEnv)),
		scheduler:     strings.TrimSpace(os.Getenv(tinyGoSchedulerEnv)),
		stackSize:     strings.TrimSpace(os.Getenv(tinyGoStackEnv)),
		llvmFeatures:  strings.TrimSpace(os.Getenv(tinyGoLLVMEnv)),
	}
	if env.panicStrategy == "" {
		env.panicStrategy = "print"
	}
	if env.scheduler == "" {
		env.scheduler = "asyncify"
	}
	if profile == tinyGoTargetDefault || profile == tinyGoBldrFeatures {
		if env.stackSize == "" {
			env.stackSize = gocompiler.TinyGoDefaultStackSize
		}
	}
	if profile == tinyGoBldrFeatures && env.llvmFeatures == "" {
		env.llvmFeatures = strings.Join(gocompiler.GetDefaultTinygoLlvmFeatures(), ",")
	}
	return env, nil
}

func (e *tinyGoBuildEnvelope) args() []string {
	args := []string{"-target", e.target}
	if e.opt != "" {
		args = append(args, "-opt="+e.opt)
	}
	if e.scheduler != "" {
		args = append(args, "-scheduler="+e.scheduler)
	}
	if e.panicStrategy != "" {
		args = append(args, "-panic="+e.panicStrategy)
	}
	if e.gc != "" {
		args = append(args, "-gc="+e.gc)
	}
	if e.stackSize != "" {
		args = append(args, "-stack-size="+e.stackSize)
	}
	if e.llvmFeatures != "" {
		args = append(args, "-llvm-features="+e.llvmFeatures)
	}
	return args
}

func (e *tinyGoBuildEnvelope) String() string {
	features := e.llvmFeatures
	if features == "" {
		features = "target-default"
	}
	gc := e.gc
	if gc == "" {
		gc = "target-default"
	}
	opt := e.opt
	if opt == "" {
		opt = "target-default"
	}
	stack := e.stackSize
	if stack == "" {
		stack = "target-default"
	}
	return fmt.Sprintf(
		"%s=%s %s=%s %s=%s %s=%s %s=%s %s=%s %s=%s target=%s bldr_concepts=%s,%s,%s,%s,%s,%s,%s",
		tinyGoProfileEnv,
		e.profile,
		tinyGoOptEnv,
		opt,
		tinyGoPanicEnv,
		e.panicStrategy,
		tinyGoGCEnv,
		gc,
		tinyGoSchedulerEnv,
		e.scheduler,
		tinyGoStackEnv,
		stack,
		tinyGoLLVMEnv,
		features,
		e.target,
		gocompiler.TinyGoProfileEnv,
		gocompiler.TinyGoOptEnv,
		gocompiler.TinyGoPanicStrategyEnv,
		gocompiler.TinyGoGCEnv,
		gocompiler.TinyGoSchedulerEnv,
		gocompiler.TinyGoStackSizeEnv,
		gocompiler.TinyGoLLVMFeaturesEnv,
	)
}

func tinyGoVersion(ctx context.Context) string {
	versionCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(versionCtx, "tinygo", "version")
	data, err := cmd.CombinedOutput()
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(data))
}

func wasmFeatureEvidence(ctx context.Context, wasmPath string) string {
	objdumpCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(objdumpCtx, "wasm-objdump", "-x", wasmPath)
	data, err := cmd.CombinedOutput()
	if err != nil {
		return "wasm-objdump-error=" + err.Error()
	}
	var features []string
	inTargetFeatures := false
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if strings.Contains(line, `name: "target_features"`) {
			inTargetFeatures = true
			continue
		}
		if !inTargetFeatures {
			continue
		}
		if strings.HasPrefix(line, "- [+]") || strings.HasPrefix(line, "- [-]") {
			features = append(features, strings.Join(strings.Fields(line), " "))
			continue
		}
		if len(features) != 0 && line != "" {
			break
		}
	}
	if len(features) == 0 {
		return "target_features=none"
	}
	return strings.Join(features, ";")
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
			writeMS:    intField(m, "writeMs"),
			blocks:     intField(m, "blocks"),
			publishGen: intField(m, "publishGen"),
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
	writeMS    int
	blocks     int
	publishGen int
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
            if (data.kind === 'progress') {
              console.log(formatProgress(data))
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

      function formatProgress(data) {
        let msg = 'opfs worker progress scenario=' + (data.scenario ?? '') +
          ' worker=' + (data.worker ?? 0) +
          ' phase=' + (data.phase ?? '')
        if (data.offset !== undefined || data.total !== undefined) {
          msg += ' offset=' + (data.offset ?? 0) + ' total=' + (data.total ?? 0)
        }
        return msg
      }
    </script>
  </body>
</html>
`

const workerJS = `importScripts('/wasm_exec.js')

self.__BLDR_TINYGO_STORED_BYTES = new Map()
self.__BLDR_TINYGO_STORED_BYTES_NEXT_ID = 1
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
self.__BLDR_TINYGO_RUNTIME_EXITED = false
self.__BLDR_TINYGO_OPFS_RUNTIME_TASKS = new Set()
self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE = (writable) => {
  if (!writable) {
    return Promise.resolve()
  }
  try {
    return writable.abort()
  } catch (reason) {
    return Promise.reject(reason)
  }
}
self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE_QUIETLY = (writable) => {
  void self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE(writable).catch(() => {})
}
self.__BLDR_TINYGO_TRACK_OPFS_TASK = (task) => {
  const tracked = Promise.resolve(task)
    .then(() => undefined, () => undefined)
    .finally(() => self.__BLDR_TINYGO_OPFS_RUNTIME_TASKS.delete(tracked))
  self.__BLDR_TINYGO_OPFS_RUNTIME_TASKS.add(tracked)
  return tracked
}
self.__BLDR_TINYGO_AWAIT_OPFS_TASKS = async () => {
  while (self.__BLDR_TINYGO_OPFS_RUNTIME_TASKS.size !== 0) {
    await Promise.all([...self.__BLDR_TINYGO_OPFS_RUNTIME_TASKS])
  }
}
self.__BLDR_TINYGO_CREATE_OPFS_WRITABLE = (handle, opts) => {
  if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
    return Promise.resolve(undefined)
  }
  const created = opts ? handle.createWritable(opts) : handle.createWritable()
  return created.then(async (writable) => {
    if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
      await self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE(writable).catch(() => {})
      return undefined
    }
    return writable
  })
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
    if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
      return
    }
    self.__BLDR_TINYGO_CALL_EXPORT(go, resolve, opID, values.length, values[0] ?? 0, values[1] ?? 0)
  })
}
self.__BLDR_TINYGO_OPFS_REJECT = (opID, code) => {
  const go = self.__BLDR_TINYGO_CURRENT_GO
  const reject = self.__BLDR_TINYGO_EXPORT(go, 'BLDR_OPFS_HELPER_REJECT')
  self.__BLDR_TINYGO_DEFER(() => {
    if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
      return
    }
    self.__BLDR_TINYGO_CALL_EXPORT(go, reject, opID, code)
  })
}
self.__BLDR_TINYGO_REJECT_OPFS_WRITABLE_FAILURE = async (opID, writable, reason) => {
  const abortReason = await self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE(writable)
    .then(() => undefined, (value) => value)
  const report = abortReason === undefined ? reason : abortReason
  if (!self.__BLDR_TINYGO_RUNTIME_EXITED) {
    self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(report))
  }
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
self.__BLDR_TINYGO_OPFS_WRITE_STREAM_ID ??= 1
self.__BLDR_TINYGO_OPFS_WRITE_STREAMS ??= new Map()
self.__BLDR_TINYGO_ABORT_OPFS_WRITE_STREAM ??= (streamID, strict = false) => {
  const stream = self.__BLDR_TINYGO_OPFS_WRITE_STREAMS.get(streamID)
  if (!stream) {
    return Promise.resolve(false)
  }
  self.__BLDR_TINYGO_TRACK_OPFS_TASK(stream.chain)
  const abort = strict
    ? self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE(stream.writable)
    : self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE(stream.writable).catch(() => {})
  const aborted = abort.then(() => {
    self.__BLDR_TINYGO_OPFS_WRITE_STREAMS.delete(streamID)
    return true
  })
  self.__BLDR_TINYGO_TRACK_OPFS_TASK(aborted)
  return aborted
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
  const state = {}
  const opts = keepExisting ? { keepExistingData: true } : undefined
  const task = self.__BLDR_TINYGO_CREATE_OPFS_WRITABLE(handle, opts)
    .then(async (next) => {
      if (!next) {
        return
      }
      state.writable = next
      if (off !== 0) {
        await next.seek(off)
      }
      if (writeData.byteLength !== 0) {
        await next.write(writeData)
      }
      await next.close()
      if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
        return
      }
      self.__BLDR_TINYGO_OPFS_RESOLVE(opID, writeData.byteLength)
    })
    .catch(async (reason) => {
      await self.__BLDR_TINYGO_REJECT_OPFS_WRITABLE_FAILURE(opID, state.writable, reason)
    })
  self.__BLDR_TINYGO_TRACK_OPFS_TASK(task)
}
self.BLDR_OPFS_WRITE_FILE ??= (dir, name, data, opID) => {
  const writeData = self.__BLDR_TINYGO_COPY_BYTES(data)
  const state = {}
  const task = dir.getFileHandle(name, { create: true })
    .then((handle) => self.__BLDR_TINYGO_CREATE_OPFS_WRITABLE(handle))
    .then(async (next) => {
      if (!next) {
        return
      }
      state.writable = next
      if (writeData.byteLength !== 0) {
        await next.write(writeData)
      }
      await next.close()
      if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
        return
      }
      self.__BLDR_TINYGO_OPFS_RESOLVE(opID, writeData.byteLength)
    })
    .catch(async (reason) => {
      await self.__BLDR_TINYGO_REJECT_OPFS_WRITABLE_FAILURE(opID, state.writable, reason)
    })
  self.__BLDR_TINYGO_TRACK_OPFS_TASK(task)
}

let __opfsChrometestCurrentArgs = null
function __opfsChrometestErrorText(reason) {
  if (reason && typeof reason === 'object' && typeof reason.stack === 'string') {
    return reason.stack
  }
  if (reason && typeof reason === 'object' && typeof reason.message === 'string') {
    return reason.message
  }
  return String(reason)
}
function __opfsChrometestPostUnhandled(reason) {
  const args = __opfsChrometestCurrentArgs ?? {}
  self.postMessage({
    kind: 'result',
    scenario: args.scenario ?? '',
    worker: args.worker ?? 0,
    ok: false,
    error: __opfsChrometestErrorText(reason),
  })
}
self.onerror = (message, source, lineno, colno, error) => {
  __opfsChrometestPostUnhandled(error || (String(message) + ' at ' + source + ':' + lineno + ':' + colno))
  return true
}
self.onunhandledrejection = (event) => {
  __opfsChrometestPostUnhandled(event.reason)
}

self.onmessage = async (event) => {
  const args = event.data
  __opfsChrometestCurrentArgs = args
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
      const state = {}
      const task = self.__BLDR_TINYGO_CREATE_OPFS_WRITABLE(handle, { keepExistingData: true })
        .then(async (next) => {
          if (!next) {
            return
          }
          state.writable = next
          await next.truncate(Number(size))
          await next.close()
          if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
            return
          }
          self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 1)
        })
        .catch(async (reason) => {
          await self.__BLDR_TINYGO_REJECT_OPFS_WRITABLE_FAILURE(opID, state.writable, reason)
        })
      self.__BLDR_TINYGO_TRACK_OPFS_TASK(task)
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
      const state = {}
      try {
        const writeData = self.__BLDR_TINYGO_COPY_BYTES(self.__BLDR_TINYGO_MEMORY_VIEW(go, dataPtr, dataLen))
        const opts = keepExisting ? { keepExistingData: true } : undefined
        const task = self.__BLDR_TINYGO_CREATE_OPFS_WRITABLE(handle, opts)
          .then(async (next) => {
            if (!next) {
              return
            }
            state.writable = next
            const offset = Number(off)
            if (offset !== 0) {
              await next.seek(offset)
            }
            if (writeData.byteLength !== 0) {
              await next.write(writeData)
            }
            await next.close()
            if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
              return
            }
            self.__BLDR_TINYGO_OPFS_RESOLVE(opID, writeData.byteLength)
          })
          .catch(async (reason) => {
            await self.__BLDR_TINYGO_REJECT_OPFS_WRITABLE_FAILURE(opID, state.writable, reason)
          })
        self.__BLDR_TINYGO_TRACK_OPFS_TASK(task)
      } catch (reason) {
        self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE_QUIETLY(state.writable)
        if (!self.__BLDR_TINYGO_RUNTIME_EXITED) {
          self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
        }
      }
    }
    go.importObject.gojs['bldr.opfs.writeFileRef'] ??= (opID, dirRef, namePtr, nameLen, dataPtr, dataLen) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      const state = {}
      try {
        const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
        const writeData = self.__BLDR_TINYGO_COPY_BYTES(self.__BLDR_TINYGO_MEMORY_VIEW(go, dataPtr, dataLen))
        const task = dir.getFileHandle(name, { create: true })
          .then((handle) => self.__BLDR_TINYGO_CREATE_OPFS_WRITABLE(handle))
          .then(async (next) => {
            if (!next) {
              return
            }
            state.writable = next
            if (writeData.byteLength !== 0) {
              await next.write(writeData)
            }
            await next.close()
            if (self.__BLDR_TINYGO_RUNTIME_EXITED) {
              return
            }
            self.__BLDR_TINYGO_OPFS_RESOLVE(opID, writeData.byteLength)
          })
          .catch(async (reason) => {
            await self.__BLDR_TINYGO_REJECT_OPFS_WRITABLE_FAILURE(opID, state.writable, reason)
          })
        self.__BLDR_TINYGO_TRACK_OPFS_TASK(task)
    } catch (reason) {
      self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE_QUIETLY(state.writable)
      if (!self.__BLDR_TINYGO_RUNTIME_EXITED) {
        self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
      }
    }
  }
    go.importObject.gojs['bldr.opfs.openWriteStreamRef'] ??= (opID, dirRef, namePtr, nameLen) => {
      const dir = self.__BLDR_TINYGO_UNBOX_VALUE(go, dirRef)
      const state = {}
      try {
        const name = self.__BLDR_TINYGO_READ_STRING(go, namePtr, nameLen)
        const task = dir.getFileHandle(name, { create: true })
          .then((handle) => self.__BLDR_TINYGO_CREATE_OPFS_WRITABLE(handle))
          .then((next) => {
            if (!next) {
              return
            }
            state.writable = next
            const streamID = self.__BLDR_TINYGO_OPFS_WRITE_STREAM_ID++
            self.__BLDR_TINYGO_OPFS_WRITE_STREAMS.set(streamID, {
              writable: next,
              chain: Promise.resolve(),
            })
            self.__BLDR_TINYGO_OPFS_RESOLVE(opID, streamID)
          })
          .catch(async (reason) => {
            await self.__BLDR_TINYGO_REJECT_OPFS_WRITABLE_FAILURE(opID, state.writable, reason)
          })
        self.__BLDR_TINYGO_TRACK_OPFS_TASK(task)
      } catch (reason) {
        self.__BLDR_TINYGO_ABORT_OPFS_WRITABLE_QUIETLY(state.writable)
        if (!self.__BLDR_TINYGO_RUNTIME_EXITED) {
          self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
        }
      }
    }
    go.importObject.gojs['bldr.opfs.writeStreamRef'] ??= (opID, streamID, dataPtr, dataLen) => {
      const stream = self.__BLDR_TINYGO_OPFS_WRITE_STREAMS.get(streamID)
      if (!stream) {
        self.__BLDR_TINYGO_OPFS_REJECT(opID, 1)
        return
      }
      try {
        const writeData = self.__BLDR_TINYGO_COPY_BYTES(self.__BLDR_TINYGO_MEMORY_VIEW(go, dataPtr, dataLen))
        stream.chain = stream.chain
          .then(async () => {
            if (writeData.byteLength !== 0) {
              await stream.writable.write(writeData)
            }
            self.__BLDR_TINYGO_OPFS_RESOLVE(opID, writeData.byteLength)
          })
          .catch((reason) => {
            if (!self.__BLDR_TINYGO_RUNTIME_EXITED) {
              self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
            }
          })
      } catch (reason) {
        if (!self.__BLDR_TINYGO_RUNTIME_EXITED) {
          self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
        }
      }
    }
    go.importObject.gojs['bldr.opfs.closeWriteStreamRef'] ??= (opID, streamID) => {
      const stream = self.__BLDR_TINYGO_OPFS_WRITE_STREAMS.get(streamID)
      if (!stream) {
        self.__BLDR_TINYGO_OPFS_REJECT(opID, 1)
        return
      }
      stream.chain = stream.chain
        .then(async () => {
          await stream.writable.close()
          self.__BLDR_TINYGO_OPFS_WRITE_STREAMS.delete(streamID)
          self.__BLDR_TINYGO_OPFS_RESOLVE(opID, 1)
        })
        .catch((reason) => {
          if (!self.__BLDR_TINYGO_RUNTIME_EXITED) {
            self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason))
          }
        })
    }
    go.importObject.gojs['bldr.opfs.abortWriteStreamRef'] ??= (opID, streamID) => {
      Promise.resolve(self.__BLDR_TINYGO_ABORT_OPFS_WRITE_STREAM(streamID, true))
        .then((aborted) => self.__BLDR_TINYGO_OPFS_RESOLVE(opID, aborted ? 1 : 0))
        .catch((reason) => self.__BLDR_TINYGO_OPFS_REJECT(opID, self.BLDR_TINYGO_PROMISE_ERROR_CODE(reason)))
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
  try {
    await go.run(res.instance)
  } finally {
    self.__BLDR_TINYGO_RUNTIME_EXITED = true
    for (const [streamID] of self.__BLDR_TINYGO_OPFS_WRITE_STREAMS) {
      void self.__BLDR_TINYGO_ABORT_OPFS_WRITE_STREAM(streamID)
    }
    await self.__BLDR_TINYGO_AWAIT_OPFS_TASKS()
  }
}
`

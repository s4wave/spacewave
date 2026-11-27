package chrometest

import (
	"os"
	"strings"
	"testing"
	"time"
)

// TestOpfsChromeClassifiesPromiseRejection checks browser NotFound rejection classification.
func TestOpfsChromeClassifiesPromiseRejection(t *testing.T) {
	// Open a browser session for the selected OPFS worker scenario.
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newPersistentSession(t, "volume-reset-incompatible")
	defer s.close(t)

	// Exercise missing-file deletion in a fresh OPFS root.
	root := s.resetRoot(t, "opfs-chrome-reject-")
	s.runWorker(t, workerArgs{
		scenario: "missing-delete-classify",
		root:     root,
	})
}

// TestOpfsChromeReadFileHelperLoop checks repeated whole-file reads inside a worker.
func TestOpfsChromeReadFileHelperLoop(t *testing.T) {
	// Open a browser session for the selected OPFS worker scenario.
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newPersistentSession(t, "volume-reset-unknown")
	defer s.close(t)

	// Run the file-read helper repeatedly in a fresh OPFS root.
	root := s.resetRoot(t, "opfs-chrome-read-helper-")
	s.runWorker(t, workerArgs{
		scenario:   "read-file-helper-loop",
		root:       root,
		iterations: 64,
	})
}

// TestOpfsChromeLargeWriteReadList checks large file writes, sampled reads, and listing.
func TestOpfsChromeLargeWriteReadList(t *testing.T) {
	// Open a browser session for the selected OPFS worker scenario.
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Run large file writes, sampled reads, and listing in a fresh OPFS root.
	root := s.resetRoot(t, "opfs-chrome-large-write-read-list-")
	s.runWorker(t, workerArgs{
		scenario:   "large-write-read-list",
		root:       root,
		iterations: 8 * 1024 * 1024,
		batch:      4,
	})
}

// TestOpfsChromeTinyGoPipeWriteLoop checks TinyGo pipe streaming independently of storage.
func TestOpfsChromeTinyGoPipeWriteLoop(t *testing.T) {
	// Require the selected browser profile and TinyGo compiler for the worker scenario.
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo io.Pipe scheduling path", tinyGoEnv)
	}

	// Open a browser session for the selected OPFS worker scenario.
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Run the TinyGo pipe streaming scenario in a fresh OPFS root.
	root := "opfs-chrome-pipe-write-loop-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario:   "pipe-write-loop",
		root:       root,
		iterations: 4 * 1024 * 1024,
	})
}

// TestOpfsChromeTinyGoSRPCEchoLoop checks TinyGo calls over a multiplexed connection.
func TestOpfsChromeTinyGoSRPCEchoLoop(t *testing.T) {
	// Require the selected browser profile and TinyGo compiler for the worker scenario.
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise TinyGo SRPC unary call liveness", tinyGoEnv)
	}

	// Open a browser session for the selected OPFS worker scenario.
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Run the TinyGo unary RPC scenario in a fresh OPFS root.
	root := "opfs-chrome-srpc-echo-loop-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario:   "srpc-echo-loop",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 128),
		batch:      envIntDefault(t, resourceReadChunkEnv, 4096),
	})
}

// TestOpfsChromeTinyGoSRPCRpcStreamEchoLoop checks TinyGo calls through nested RPC streams.
func TestOpfsChromeTinyGoSRPCRpcStreamEchoLoop(t *testing.T) {
	// Require the selected browser profile and TinyGo compiler for the worker scenario.
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise TinyGo SRPC-over-rpcstream liveness", tinyGoEnv)
	}

	// Open a browser session for the selected OPFS worker scenario.
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Run the TinyGo nested RPC scenario in a fresh OPFS root.
	root := "opfs-chrome-srpc-rpcstream-echo-loop-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario:   "srpc-rpcstream-echo-loop",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 128),
		batch:      envIntDefault(t, resourceReadChunkEnv, 4096),
	})
}

// TestOpfsChromeTinyGoResourceEchoLoop checks TinyGo resource reference calls.
func TestOpfsChromeTinyGoResourceEchoLoop(t *testing.T) {
	// Require the selected browser profile and TinyGo compiler for the worker scenario.
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise TinyGo resource-routed SRPC liveness", tinyGoEnv)
	}

	// Open a browser session for the selected OPFS worker scenario.
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Run TinyGo resource calls in a fresh OPFS root.
	root := "opfs-chrome-resource-echo-loop-" + time.Now().Format("150405.000000000")
	s.runWorker(t, workerArgs{
		scenario:   "resource-echo-loop",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 128),
		batch:      envIntDefault(t, resourceReadChunkEnv, 4096),
	})
}

// TestOpfsChromeTinyGoLargeWriteReadList checks large raw OPFS files under TinyGo.
func TestOpfsChromeTinyGoLargeWriteReadList(t *testing.T) {
	// Require the selected browser profile and TinyGo compiler for the worker scenario.
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo OPFS helper ABI", tinyGoEnv)
	}

	// Open a browser session for the selected OPFS worker scenario.
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Run large TinyGo file writes and readback in a fresh OPFS root.
	root := s.resetRoot(t, "opfs-chrome-large-write-read-list-")
	s.runWorker(t, workerArgs{
		scenario:   "large-write-read-list",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      1,
	})
}

// TestOpfsChromeReadAtHelperLoop checks worker offset reads and EOF semantics.
func TestOpfsChromeReadAtHelperLoop(t *testing.T) {
	// Open a browser session for the selected OPFS worker scenario.
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Run the offset-read helper repeatedly in a fresh OPFS root.
	root := s.resetRoot(t, "opfs-chrome-read-at-helper-")
	s.runWorker(t, workerArgs{
		scenario:   "read-at-helper-loop",
		root:       root,
		iterations: 64,
	})
}

// TestOpfsChromeRunWorkersScriptWatchdogReportsHangingWorker requires useful progress in timeout failures.
func TestOpfsChromeRunWorkersScriptWatchdogReportsHangingWorker(t *testing.T) {
	// Open a browser session for the selected OPFS worker scenario.
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Verify a hanging worker reports its scenario, identity, and last progress.
	_, err := s.evalWorkersScript(t, `async ({ worker }) => {
  window.__opfsChromeWorkerWatchdog.record({
    scenario: worker.scenario,
    worker: worker.worker,
    phase: 'test-hang',
  })
  await new Promise(() => {})
}`, map[string]any{
		"worker": mapSingleWorkerArg(workerArgs{
			scenario: "watchdog-hang",
			root:     "watchdog-hang",
			worker:   7,
			workers:  1,
		}),
	}, 50*time.Millisecond)
	if err == nil {
		t.Fatal("expected hanging worker script watchdog error")
	}
	msg := err.Error()
	for _, want := range []string{
		"opfs worker script timeout",
		"scenario=watchdog-hang",
		"worker=7",
		"phase=test-hang",
		"browser exception=none",
	} {
		if !strings.Contains(msg, want) {
			t.Fatalf("watchdog error missing %q: %s", want, msg)
		}
	}
}

// TestOpfsChromeFileLockSerializesWorkers requires file locking to preserve every counter increment.
func TestOpfsChromeFileLockSerializesWorkers(t *testing.T) {
	// Open a browser session for the selected OPFS worker scenario.
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Initialize a fresh OPFS counter before concurrent increments.
	root := s.resetRoot(t, "opfs-chrome-lock-")
	s.runWorker(t, workerArgs{
		scenario: "counter-init",
		root:     root,
	})

	// Run locked counter writers and verify every increment survives.
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
		})
	}
	s.runWorkers(t, args)
	s.runWorker(t, workerArgs{
		scenario:   "counter-verify",
		root:       root,
		workers:    workers,
		iterations: iterations,
		batch:      1,
	})
}

// TestOpfsChromeFileLockQueuedWorkersProgressAfterRelease checks queued writers resume after release.
func TestOpfsChromeFileLockQueuedWorkersProgressAfterRelease(t *testing.T) {
	// Open a browser session for the selected OPFS worker scenario.
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Initialize a fresh OPFS counter before queuing writers.
	root := s.resetRoot(t, "opfs-chrome-lock-queued-")
	s.runWorker(t, workerArgs{
		scenario: "counter-init",
		root:     root,
	})

	// Queue counter writers behind a holder and verify their increments after release.
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
		})
	}
	s.runBlockedLockWorkers(t, holder, args)
	s.runWorker(t, workerArgs{
		scenario:   "counter-verify",
		root:       root,
		workers:    workers,
		iterations: iterations,
		batch:      1,
	})
}

// TestOpfsChromeFileLockQueuedWorkersProgressAfterHolderTermination checks crash-released lock progress.
func TestOpfsChromeFileLockQueuedWorkersProgressAfterHolderTermination(t *testing.T) {
	// Open a browser session for the selected OPFS worker scenario.
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Initialize a fresh OPFS counter before terminating its lock holder.
	root := s.resetRoot(t, "opfs-chrome-lock-terminated-")
	s.runWorker(t, workerArgs{
		scenario: "counter-init",
		root:     root,
	})

	// Queue counter writers and verify their increments after holder termination.
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
		})
	}
	s.runTerminatedLockHolderWorkers(t, holder, args)
	s.runWorker(t, workerArgs{
		scenario:   "counter-verify",
		root:       root,
		workers:    workers,
		iterations: iterations,
		batch:      1,
	})
}

// TestOpfsChromeWebLockIfAvailable checks nonblocking lock outcomes while a holder is active.
func TestOpfsChromeWebLockIfAvailable(t *testing.T) {
	// Open a browser session for the selected OPFS worker scenario.
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Initialize a fresh OPFS counter before probing lock availability.
	root := s.resetRoot(t, "opfs-chrome-lock-if-available-")
	s.runWorker(t, workerArgs{
		scenario: "counter-init",
		root:     root,
	})

	// Verify the Web Lock is unavailable during the hold and available after release.
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

// TestOpfsChromeWebLockCancellation checks queued Web Lock cancellation.
func TestOpfsChromeWebLockCancellation(t *testing.T) {
	// Open a browser session for the selected OPFS worker scenario.
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Initialize a fresh OPFS counter before cancelling a queued lock request.
	root := s.resetRoot(t, "opfs-chrome-lock-cancel-")
	s.runWorker(t, workerArgs{
		scenario: "counter-init",
		root:     root,
	})

	// Probe queued lock cancellation while the counter holder remains active.
	s.runHeldLockCheck(t, workerArgs{
		scenario: "counter-hold",
		root:     root,
	}, workerArgs{
		scenario: "counter-timeout-lock",
		root:     root,
	})
}

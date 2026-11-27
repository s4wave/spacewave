package chrometest

import (
	"os"
	"strings"
	"testing"
)

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
	// Open a browser session for the selected OPFS worker scenario.
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Read the copy-walk concurrency and input size for the probe.
	concurrency := envIntDefault(t, "OPFS_COPYWALK_CONCURRENCY", 16)
	inputBytes := envIntDefault(t, "OPFS_COPYWALK_BLOCKS", 64*1024)

	// Run the copy-walk probe in a fresh OPFS root and report its duration.
	root := s.resetRoot(t, "opfs-chrome-copywalk-")
	res := s.runWorker(t, workerArgs{
		scenario:   "copy-walk-wrapper-concurrency",
		root:       root,
		iterations: inputBytes,
		batch:      concurrency,
	})
	t.Logf("copy-walk-wrapper concurrency=%d inputBytes=%d durationMs=%d",
		concurrency, inputBytes, res.durationMS)
}

// TestOpfsChromeUnixFSStorageAmplification bounds actual disk growth for a large UnixFS upload.
func TestOpfsChromeUnixFSStorageAmplification(t *testing.T) {
	// Require the browser profile selected for the OPFS scenario.
	requireChromeProfile(t, chromeSmoke)

	// Open a browser session for the selected OPFS worker scenario.
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Measure OPFS usage before and after uploading the UnixFS workload.
	const sourceBytes = 68056093
	root := s.resetRoot(t, "opfs-chrome-unixfs-storage-amplification-")
	before := s.readStorageSnapshot(t)
	s.runWorker(t, workerArgs{
		scenario:   "world-resource-large-unixfs-write",
		root:       root,
		iterations: sourceBytes,
		batch:      262144,
	})
	after := s.readStorageSnapshot(t)

	// Verify UnixFS storage growth stays within the amplification and quota limits.
	opfsGrowth := after.opfsBytes - before.opfsBytes
	usageGrowth := after.usage - before.usage
	if opfsGrowth <= 0 {
		t.Fatalf("OPFS growth = %d, want positive", opfsGrowth)
	}
	if opfsGrowth*100 > sourceBytes*120 {
		t.Fatalf("UnixFS OPFS amplification = %.6f, want at most 1.20", float64(opfsGrowth)/sourceBytes)
	}
	if usageGrowth*100 > sourceBytes*120 {
		t.Fatalf("UnixFS origin usage amplification = %.6f, want at most 1.20", float64(usageGrowth)/sourceBytes)
	}
	if after.quota <= after.usage {
		t.Fatalf("storage estimate quota=%d usage=%d, want positive headroom", after.quota, after.usage)
	}

	// Report the UnixFS workload and measured storage amplification.
	t.Logf(
		"unixfs storage estimate: source=%d quota=%d usage_before=%d usage_after=%d usage_growth=%d opfs_before=%d opfs_after=%d opfs_growth=%d files_before=%d files_after=%d persisted_before=%t persisted_after=%t opfs_amplification=%.6f usage_amplification=%.6f",
		sourceBytes,
		after.quota,
		before.usage,
		after.usage,
		usageGrowth,
		before.opfsBytes,
		after.opfsBytes,
		opfsGrowth,
		before.files,
		after.files,
		before.persisted,
		after.persisted,
		float64(opfsGrowth)/float64(sourceBytes),
		float64(usageGrowth)/float64(sourceBytes),
	)
}

// TestOpfsChromeVolumeRuntimeSlice checks interrupted initialization, durable remount, and explicit deletion.
func TestOpfsChromeVolumeRuntimeSlice(t *testing.T) {
	// Open a browser session for the selected OPFS worker scenario.
	requireChromeProfile(t, chromeStress)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Clear a fresh OPFS root before interrupting its format-marker creation.
	root := s.resetRoot(t, "opfs-chrome-volume-")

	// Simulate termination after creating the first format-marker entry.
	_, err := s.page.Evaluate(`async (name) => {
	  const root = await navigator.storage.getDirectory()
	  const test = await root.getDirectoryHandle(name, { create: true })
	  const volume = await test.getDirectoryHandle('volume', { create: true })
	  await volume.getFileHandle('.spacewave-opfs-format', { create: true })
	}`, root)
	if err != nil {
		t.Fatal(err)
	}

	// Verify volume writes, remount readback, and explicit deletion after interruption.
	s.runWorker(t, workerArgs{
		scenario: "volume-runtime-write",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "volume-runtime-verify",
		root:     root,
	})
	s.runWorker(t, workerArgs{
		scenario: "volume-runtime-delete-verify",
		root:     root,
	})
}

// TestOpfsChromeWorldInitUnixFS checks world initialization through the product volume.
func TestOpfsChromeWorldInitUnixFS(t *testing.T) {
	// Open a browser session for the selected OPFS worker scenario.
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Initialize a World and UnixFS in a fresh OPFS root.
	root := s.resetRoot(t, "opfs-chrome-world-unixfs-")
	s.runWorker(t, workerArgs{
		scenario: "world-init-unixfs",
		root:     root,
	})
}

// TestOpfsChromeWorldCrashRecovery checks that a World recovers at its last commit.
func TestOpfsChromeWorldCrashRecovery(t *testing.T) {
	// Open a browser session for the selected OPFS worker scenario.
	requireChromeProfile(t, chromeSmoke)
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Verify World recovery at the last commit in a fresh OPFS root.
	root := s.resetRoot(t, "opfs-chrome-world-crash-")
	s.runWorker(t, workerArgs{
		scenario: "world-crash-recovery",
		root:     root,
	})
}

// TestOpfsChromeTinyGoWorldLargeUnixFSUpload checks large world file write and readback under TinyGo.
func TestOpfsChromeTinyGoWorldLargeUnixFSUpload(t *testing.T) {
	// Require the selected browser profile and TinyGo compiler for the worker scenario.
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo UnixFS large-upload path", tinyGoEnv)
	}

	// Open a browser session for the selected OPFS worker scenario.
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Upload and read back a large UnixFS file in a fresh OPFS World.
	root := s.resetRoot(t, "opfs-chrome-world-large-unixfs-")
	s.runWorker(t, workerArgs{
		scenario:   "world-large-unixfs-upload",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
	})
}

// TestOpfsChromeTinyGoWorldResourceLargeUnixFSUpload checks large resource uploads under TinyGo.
func TestOpfsChromeTinyGoWorldResourceLargeUnixFSUpload(t *testing.T) {
	// Require the selected browser profile and TinyGo compiler for the worker scenario.
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo UnixFS resource large-upload path", tinyGoEnv)
	}

	// Open a browser session for the selected OPFS worker scenario.
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Upload a large UnixFS file through the World resource in a fresh OPFS root.
	root := s.resetRoot(t, "opfs-chrome-world-resource-large-unixfs-")
	s.runWorker(t, workerArgs{
		scenario:   "world-resource-large-unixfs-upload",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      envInt(t, resourceReadChunkEnv),
	})
}

// TestOpfsChromeTinyGoWorldResourceDirectUploadTreeLargeUnixFSUpload isolates direct UploadTree from RPC.
func TestOpfsChromeTinyGoWorldResourceDirectUploadTreeLargeUnixFSUpload(t *testing.T) {
	// Require the selected browser profile and TinyGo compiler for the worker scenario.
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo direct UploadTree path", tinyGoEnv)
	}

	// Open a browser session for the selected OPFS worker scenario.
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Exercise direct UploadTree with a large UnixFS file in a fresh OPFS root.
	root := s.resetRoot(t, "opfs-chrome-world-resource-direct-upload-tree-large-unixfs-")
	s.runWorker(t, workerArgs{
		scenario:   "world-resource-direct-upload-tree-large-unixfs-upload",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      envInt(t, resourceReadChunkEnv),
	})
}

// TestOpfsChromeTinyGoWorldControllerResourceLargeUnixFSUpload checks controller-backed resource uploads.
func TestOpfsChromeTinyGoWorldControllerResourceLargeUnixFSUpload(t *testing.T) {
	// Require the selected browser profile and TinyGo compiler for the worker scenario.
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo controller bucket large-upload path", tinyGoEnv)
	}

	// Open a browser session for the selected OPFS worker scenario.
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Upload a large UnixFS file through the controller-backed resource.
	root := s.resetRoot(t, "opfs-chrome-world-controller-resource-large-unixfs-")
	s.runWorker(t, workerArgs{
		scenario:   "world-controller-resource-large-unixfs-upload",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      envInt(t, resourceReadChunkEnv),
	})
}

// TestOpfsChromeTinyGoWorldCloudOverlayResourceLargeUnixFSUpload checks the dirty-tracking overlay upload path.
func TestOpfsChromeTinyGoWorldCloudOverlayResourceLargeUnixFSUpload(t *testing.T) {
	// Require the selected browser profile and TinyGo compiler for the worker scenario.
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo cloud-overlay large-upload path", tinyGoEnv)
	}

	// Open a browser session for the selected OPFS worker scenario.
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Upload a large UnixFS file through the dirty-tracking overlay.
	root := s.resetRoot(t, "opfs-chrome-world-cloud-overlay-resource-large-unixfs-")
	s.runWorker(t, workerArgs{
		scenario:   "world-cloud-overlay-resource-large-unixfs-upload",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      envInt(t, resourceReadChunkEnv),
	})
}

// TestOpfsChromeTinyGoWorldCloudSyncResourceLargeUnixFSUpload checks packing concurrent with an upload.
func TestOpfsChromeTinyGoWorldCloudSyncResourceLargeUnixFSUpload(t *testing.T) {
	// Require the selected browser profile and TinyGo compiler for the worker scenario.
	requireChromeProfile(t, chromeSmoke)
	if os.Getenv(tinyGoEnv) != "1" && !strings.EqualFold(os.Getenv(tinyGoEnv), "true") {
		t.Skipf("set %s=1 to exercise the TinyGo cloud-sync large-upload path", tinyGoEnv)
	}

	// Open a browser session for the selected OPFS worker scenario.
	h := newChromeHarness(t)
	s := h.newSession(t)
	defer s.close(t)

	// Upload a large UnixFS file while the World sync path packs blocks.
	root := s.resetRoot(t, "opfs-chrome-world-cloud-sync-resource-large-unixfs-")
	s.runWorker(t, workerArgs{
		scenario:   "world-cloud-sync-resource-large-unixfs-upload",
		root:       root,
		iterations: envIntDefault(t, largeSizeEnv, 68056093),
		batch:      envInt(t, resourceReadChunkEnv),
	})
}

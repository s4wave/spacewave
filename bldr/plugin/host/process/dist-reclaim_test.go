//go:build !js

package plugin_host_process

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	"github.com/s4wave/spacewave/net/hash"
)

// TestDistCheckoutsReclaimsUnused removes only checkouts no execution uses,
// in the background and launch modes.
func TestDistCheckoutsReclaimsUnused(t *testing.T) {
	for _, mode := range []distReclaimMode{distReclaimBackground, distReclaimLaunch} {
		t.Run(strconv.Itoa(int(mode)), func(t *testing.T) {
			// Sync three checkouts and release the stale one.
			host := newTestReclaimHost(t, mode)
			ctx := t.Context()
			paths := make(map[string]string)
			for _, version := range []string{"running", "stale", "new"} {
				paths[version] = acquireTestCheckout(t, host, "colors", version)
			}
			host.dists.release(testArtifactID(t, "colors", "stale"))

			// Leave an interrupted removal, a staged entrypoint and an unknown entry.
			manifestsDir := host.dists.manifestsDir("colors")
			trash := filepath.Join(manifestsDir, distTrashPrefix+"left")
			writeDiskFile(t, filepath.Join(trash, "entrypoint"), []byte("left"))
			staged := filepath.Join(manifestsDir, ".entrypoint-staged")
			writeDiskFile(t, staged, []byte("staged"))
			unknown := filepath.Join(manifestsDir, "unknown", "file")
			writeDiskFile(t, unknown, []byte("unknown"))

			// A pass must remove the stale checkout and the trash alone.
			host.dists.reclaim(ctx, "colors")
			assertPathMissing(t, paths["stale"])
			assertPathMissing(t, trash)
			assertFileContents(t, filepath.Join(paths["running"], "entrypoint"), "running")
			assertFileContents(t, filepath.Join(paths["new"], "entrypoint"), "new")
			assertFileContents(t, staged, "staged")
			assertFileContents(t, unknown, "unknown")
			assertNoTrash(t, manifestsDir)
		})
	}
}

// TestDistCheckoutsKeepsCheckoutAcquiredAfterListing keeps a checkout that an
// execution acquires between the reclaimer's listing and its retirement.
func TestDistCheckoutsKeepsCheckoutAcquiredAfterListing(t *testing.T) {
	// Sync and release a checkout, then list it as the reclaimer does.
	host := newTestReclaimHost(t, distReclaimBackground)
	path := acquireTestCheckout(t, host, "colors", "relaunched")
	artifactID := testArtifactID(t, "colors", "relaunched")
	host.dists.release(artifactID)
	names := []string{filepath.Base(path)}

	// A launch acquires the listed checkout before the pass retires it.
	if err := host.dists.acquire(t.Context(), artifactID); err != nil {
		t.Fatal(err)
	}
	host.dists.synced(t.Context(), "colors", false)
	host.dists.reclaimListed(t.Context(), "colors", names)
	assertFileContents(t, filepath.Join(path, "entrypoint"), "relaunched")
	host.dists.release(artifactID)

	// A launch of a root the reclaimer has claimed waits for the claim to drop.
	host.dists.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		host.dists.claimed[artifactID] = struct{}{}
	})
	waitCtx, waitCancel := context.WithTimeout(t.Context(), 20*time.Millisecond)
	defer waitCancel()
	if err := host.dists.acquire(waitCtx, artifactID); err == nil {
		t.Fatal("acquire of a claimed root returned before the claim dropped")
	}
	acquired := make(chan error, 1)
	go func() { acquired <- host.dists.acquire(t.Context(), artifactID) }()
	host.dists.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		delete(host.dists.claimed, artifactID)
		broadcast()
	})
	if err := <-acquired; err != nil {
		t.Fatal(err)
	}
	host.dists.synced(t.Context(), "colors", false)
	host.dists.release(artifactID)
}

// TestDistCheckoutsOffKeepsCheckouts keeps every checkout when reclaim is off,
// and turns reclaim off for an unknown setting.
func TestDistCheckoutsOffKeepsCheckouts(t *testing.T) {
	// An unknown setting must select off.
	if mode, err := parseDistReclaimMode("sometimes"); err == nil || mode != distReclaimOff {
		t.Fatalf("unknown setting = %v, %v; want off and an error", mode, err)
	}

	// A synced launch must queue no pass and keep the released checkout.
	host := newTestReclaimHost(t, distReclaimOff)
	path := acquireTestCheckout(t, host, "colors", "old")
	host.dists.release(testArtifactID(t, "colors", "old"))
	acquireTestCheckout(t, host, "colors", "new")
	if len(host.dists.pending) != 0 {
		t.Fatalf("pending = %v, want none", host.dists.pending)
	}
	assertFileContents(t, filepath.Join(path, "entrypoint"), "old")
}

// TestDistCheckoutsNeverReclaimsInUse races launches against the background
// reclaimer and fails if any running execution loses its checkout's files.
func TestDistCheckoutsNeverReclaimsInUse(t *testing.T) {
	// Run the background reclaimer.
	host := newTestReclaimHost(t, distReclaimBackground)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	host.dists.setContext(ctx)

	// Each worker launches its own roots, so every launch queues a pass that
	// can reach every other worker's checkouts.
	const workers, launches, roots = 4, 40, 3
	var wg sync.WaitGroup
	errs := make(chan error, workers)
	for w := range workers {
		wg.Go(func() {
			for i := range launches {
				version := "w" + strconv.Itoa(w) + "-r" + strconv.Itoa(i%roots)
				if err := runTestExecution(ctx, host, version); err != nil {
					errs <- err
					return
				}
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}

	// After the reclaimer stops, one pass must leave no checkout or trash.
	if exited, _ := host.dists.rc.SetRoutine(nil); exited != nil {
		<-exited
	}
	host.dists.reclaim(t.Context(), "colors")
	dirents, err := os.ReadDir(host.dists.manifestsDir("colors"))
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range dirents {
		if ent.IsDir() {
			t.Fatalf("unused checkout %s kept after the final pass", ent.Name())
		}
	}
}

// runTestExecution acquires and syncs a checkout, then reads it repeatedly
// while a running process would, before releasing it.
func runTestExecution(ctx context.Context, host *ProcessHost, version string) error {
	// Acquire and sync the checkout as ExecutePlugin does.
	root, err := hash.Sum(hash.RecommendedHashType, []byte(version))
	if err != nil {
		return err
	}
	artifactID := bldr_plugin.PluginArtifactID("colors", root.MarshalString())
	dist, err := newDistHandle(ctx, map[string][]byte{"entrypoint": []byte(version)})
	if err != nil {
		return err
	}
	defer dist.Release()
	if err := host.dists.acquire(ctx, artifactID); err != nil {
		return err
	}
	defer host.dists.release(artifactID)
	path, err := host.syncPluginDist(ctx, "colors", root.MarshalString(), "entrypoint", dist)
	host.dists.synced(ctx, "colors", err == nil)
	if err != nil {
		return err
	}

	// The checkout must keep its files while the reference is held.
	for range 20 {
		body, err := os.ReadFile(filepath.Join(path, "entrypoint"))
		if err != nil {
			return err
		}
		if string(body) != version {
			return os.ErrInvalid
		}
		runtime.Gosched()
	}
	return nil
}

// newTestReclaimHost builds a process host whose checkouts use mode.
func newTestReclaimHost(t *testing.T, mode distReclaimMode) *ProcessHost {
	t.Helper()
	host := newTestProcessHost(t)
	host.dists = newDistCheckouts(host.le, host.distDir, mode)
	return host
}

// acquireTestCheckout acquires and syncs a checkout whose entrypoint holds
// version, keeping its reference, and returns its directory.
func acquireTestCheckout(t *testing.T, host *ProcessHost, pluginID, version string) string {
	t.Helper()
	root := testRoot(t, version)
	dist := newTestDistHandle(t, map[string][]byte{"entrypoint": []byte(version)})
	defer dist.Release()
	if err := host.dists.acquire(t.Context(), bldr_plugin.PluginArtifactID(pluginID, root)); err != nil {
		t.Fatal(err)
	}
	path, err := host.syncPluginDist(t.Context(), pluginID, root, "entrypoint", dist)
	host.dists.synced(t.Context(), pluginID, err == nil)
	if err != nil {
		t.Fatal(err)
	}
	return path
}

// testArtifactID returns the artifact ID of the test root for version.
func testArtifactID(t *testing.T, pluginID, version string) string {
	t.Helper()
	return bldr_plugin.PluginArtifactID(pluginID, testRoot(t, version))
}

// testRoot returns a manifest root derived from version.
func testRoot(t *testing.T, version string) string {
	t.Helper()
	root, err := hash.Sum(hash.RecommendedHashType, []byte(version))
	if err != nil {
		t.Fatal(err)
	}
	return root.MarshalString()
}

// assertNoTrash fails if a trash directory remains in manifestsDir.
func assertNoTrash(t *testing.T, manifestsDir string) {
	t.Helper()
	dirents, err := os.ReadDir(manifestsDir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range dirents {
		if ent.IsDir() && strings.HasPrefix(ent.Name(), distTrashPrefix) {
			t.Fatalf("trash %s remains", ent.Name())
		}
	}
}

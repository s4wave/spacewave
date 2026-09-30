//go:build !js

package plugin_host_process

import (
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestProcessHostImmutableDist preserves files loaded later by an older executable.
func TestProcessHostImmutableDist(t *testing.T) {
	// Sync two immutable revisions of one plugin.
	host := newTestProcessHost(t)
	paths := make(map[string]string)
	for _, version := range []string{"old", "new"} {
		root, err := hash.Sum(hash.RecommendedHashType, []byte(version))
		if err != nil {
			t.Fatal(err)
		}
		dist := newTestDistHandle(t, map[string][]byte{
			"entrypoint": []byte(version), "shared.dat": []byte(version + " data"),
		})
		defer dist.Release()
		paths[version], err = host.syncPluginDist(t.Context(), "colors", root.MarshalString(), "entrypoint", dist)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Each revision must keep its own directory and contents.
	if paths["old"] == paths["new"] {
		t.Fatal("different immutable revisions share a distribution directory")
	}
	for version, path := range paths {
		assertFileContents(t, filepath.Join(path, "entrypoint"), version)
		assertFileContents(t, filepath.Join(path, "shared.dat"), version+" data")
	}
}

// TestProcessHostPrunesUnusedManifestDists removes only checkouts no
// executing instance uses.
func TestProcessHostPrunesUnusedManifestDists(t *testing.T) {
	// Acquire and sync three manifest dists.
	host := newTestProcessHost(t)
	paths := make(map[string]string)
	releases := make(map[string]func())
	for _, version := range []string{"running", "stale", "new"} {
		root, err := hash.Sum(hash.RecommendedHashType, []byte(version))
		if err != nil {
			t.Fatal(err)
		}
		dist := newTestDistHandle(t, map[string][]byte{"entrypoint": []byte(version)})
		defer dist.Release()
		releases[version] = host.acquirePluginDist("colors", root.MarshalString())
		paths[version], err = host.syncPluginDist(t.Context(), "colors", root.MarshalString(), "entrypoint", dist)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Pruning must remove only the released checkout.
	releases["stale"]()
	host.pruneUnusedPluginDists("colors")
	assertPathMissing(t, paths["stale"])
	assertFileContents(t, filepath.Join(paths["running"], "entrypoint"), "running")
	assertFileContents(t, filepath.Join(paths["new"], "entrypoint"), "new")
	releases["running"]()
	releases["new"]()
}

func TestProcessHostSyncReplacesExecutableInode(t *testing.T) {
	// Open the old executable before syncing the new dist.
	host := newTestProcessHost(t)
	entrypoint := filepath.Join(host.pluginDistDir("sample"), "entrypoint")
	writeDiskFile(t, entrypoint, []byte("old executable"))
	old, err := os.Open(entrypoint)
	if err != nil {
		t.Fatal(err)
	}
	defer old.Close()

	// Sync the new dist over the old executable.
	dist := newTestDistHandle(t, map[string][]byte{
		"entrypoint": []byte("new executable"),
	})
	defer dist.Release()
	if _, err := host.syncPluginDist(t.Context(), "sample", "", "entrypoint", dist); err != nil {
		t.Fatal(err)
	}

	// The file must be replaced, not rewritten in place.
	assertFileContents(t, entrypoint, "new executable")
	retained, err := io.ReadAll(old)
	if err != nil {
		t.Fatal(err)
	}
	if string(retained) != "old executable" {
		t.Fatalf("previous executable changed: %q", retained)
	}

	// The replaced executable must stay executable.
	info, err := os.Stat(entrypoint)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o755 {
		t.Fatalf("executable permissions: %v", info.Mode())
	}
}

func TestProcessHostInvalidatePluginDistPreservesPluginState(t *testing.T) {
	// Write a dist file and a state file for the plugin.
	ctx := t.Context()
	host := newTestProcessHost(t)
	pluginID := "spacewave-app"

	// Write the fixture files into the dist and state dirs.
	distFile := filepath.Join(host.pluginDistDir(pluginID), "old.txt")
	stateFile := filepath.Join(host.pluginStateDir(pluginID), "state.txt")
	writeDiskFile(t, distFile, []byte("old dist"))
	writeDiskFile(t, stateFile, []byte("state data"))

	// Invalidate the plugin dist.
	if err := host.InvalidatePluginDist(ctx, pluginID); err != nil {
		t.Fatal(err.Error())
	}

	// The dist must be gone and the state preserved.
	assertPathMissing(t, host.pluginDistDir(pluginID))
	assertFileContents(t, stateFile, "state data")

	// The package status must report the invalidation.
	status := singlePackageStatus(t, host, pluginID)
	if status.Materialized {
		t.Fatalf("materialized = true after invalidation: %#v", status)
	}
	if !status.Invalidated || status.LastAction != "invalidate" || status.LastError != "" {
		t.Fatalf("unexpected invalidation status: %#v", status)
	}
}

func TestProcessHostSyncPluginDistRebuildsSelectedDistAndPreservesState(t *testing.T) {
	// Write a stale dist file and a state file for the plugin.
	ctx := t.Context()
	host := newTestProcessHost(t)
	pluginID := "spacewave-app"

	// Write the fixture files into the dist and state dirs.
	oldDistFile := filepath.Join(host.pluginDistDir(pluginID), "stale.txt")
	stateFile := filepath.Join(host.pluginStateDir(pluginID), "state.txt")
	writeDiskFile(t, oldDistFile, []byte("stale dist"))
	writeDiskFile(t, stateFile, []byte("state data"))

	// Sync the selected dist over the stale one.
	selectedDist := newTestDistHandle(t, map[string][]byte{
		"entrypoint":       []byte("#!/bin/sh\n"),
		"assets/app.mjs":   []byte("export const selected = true\n"),
		"selected-version": []byte("manifest-2\n"),
	})
	defer selectedDist.Release()
	distDir, err := host.syncPluginDist(ctx, pluginID, "", "entrypoint", selectedDist)
	if err != nil {
		t.Fatal(err.Error())
	}

	// The dist must be rebuilt and the state preserved.
	assertPathMissing(t, oldDistFile)
	assertFileContents(t, filepath.Join(distDir, "selected-version"), "manifest-2\n")
	assertFileContents(t, filepath.Join(distDir, "assets/app.mjs"), "export const selected = true\n")
	assertFileContents(t, stateFile, "state data")

	// The package status must report the sync.
	status := singlePackageStatus(t, host, pluginID)
	if !status.Materialized || status.Invalidated || status.LastAction != "sync" || status.LastError != "" {
		t.Fatalf("unexpected sync status: %#v", status)
	}
	if status.DistDir != host.pluginDistDir(pluginID) {
		t.Fatalf("dist dir = %q, want %q", status.DistDir, host.pluginDistDir(pluginID))
	}

	// The published snapshot must carry the same status.
	publishedSnapshot := host.GetPackageStatusCtr().GetValue()
	if publishedSnapshot == nil {
		t.Fatal("missing published package status snapshot")
	}
	published := singlePackageStatusFromSnapshot(t, publishedSnapshot.Packages, pluginID)
	if !published.Materialized || published.DistDir != host.pluginDistDir(pluginID) {
		t.Fatalf("unexpected published package status: %#v", published)
	}
}

func singlePackageStatus(t *testing.T, host *ProcessHost, pluginID string) PluginPackageStatus {
	// Mark the helper and snapshot the statuses.
	t.Helper()

	// Return the status entry for the plugin id.
	statuses := host.PackageStatusSnapshot()
	for _, status := range statuses {
		if status.PluginID == pluginID {
			return status
		}
	}
	t.Fatalf("missing package status for %s: %#v", pluginID, statuses)
	return PluginPackageStatus{}
}

func singlePackageStatusFromSnapshot(t *testing.T, statuses []PluginPackageStatus, pluginID string) PluginPackageStatus {
	// Mark the helper and scan the snapshot statuses.
	t.Helper()

	// Return the status entry for the plugin id.
	for _, status := range statuses {
		if status.PluginID == pluginID {
			return status
		}
	}
	t.Fatalf("missing package status for %s: %#v", pluginID, statuses)
	return PluginPackageStatus{}
}

func newTestProcessHost(t *testing.T) *ProcessHost {
	// Mark the helper and build the host.
	t.Helper()

	// Create the state and dist directories.
	stateDir := filepath.Join(t.TempDir(), "state")
	distDir := filepath.Join(t.TempDir(), "dist")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err.Error())
	}
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatal(err.Error())
	}

	// Construct the process host.
	host, err := NewProcessHost(logrus.NewEntry(logrus.New()), stateDir, distDir)
	if err != nil {
		t.Fatal(err.Error())
	}
	return host
}

func newTestDistHandle(t *testing.T, files map[string][]byte) *unixfs.FSHandle {
	// Mark the helper and build the handle.
	t.Helper()

	// Build the in-memory filesystem handle.
	ctx := t.Context()
	rootRef, err := unixfs.NewFSHandle(unixfs_billy.NewBillyFSCursor(memfs.New(), ""))
	if err != nil {
		t.Fatal(err.Error())
	}
	bfs := unixfs_billy.NewBillyFS(ctx, rootRef, "", time.Now())

	// Write each fixture file into the filesystem.
	for path, body := range files {
		if err := billy_util.WriteFile(bfs, path, body, 0o644); err != nil {
			rootRef.Release()
			t.Fatal(err.Error())
		}
	}
	return rootRef
}

func writeDiskFile(t *testing.T, path string, body []byte) {
	t.Helper()

	// Create the parent directory and write the file.
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err.Error())
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err.Error())
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()

	// The path must be missing for a not-exists error.
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s exists, want missing", path)
	} else if !os.IsNotExist(err) {
		t.Fatal(err.Error())
	}
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()

	// The file contents must equal the expected value.
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err.Error())
	}
	if string(body) != want {
		t.Fatalf("%s = %q, want %q", path, string(body), want)
	}
}

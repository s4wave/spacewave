//go:build !js

package plugin_host_process

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/go-git/go-billy/v6/memfs"
	billy_util "github.com/go-git/go-billy/v6/util"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	"github.com/sirupsen/logrus"
)

func TestProcessHostInvalidatePluginDistPreservesPluginState(t *testing.T) {
	ctx := t.Context()
	host := newTestProcessHost(t)
	pluginID := "spacewave-app"

	distFile := filepath.Join(host.pluginDistDir(pluginID), "old.txt")
	stateFile := filepath.Join(host.pluginStateDir(pluginID), "state.txt")
	writeDiskFile(t, distFile, []byte("old dist"))
	writeDiskFile(t, stateFile, []byte("state data"))

	if err := host.InvalidatePluginDist(ctx, pluginID); err != nil {
		t.Fatal(err.Error())
	}

	assertPathMissing(t, host.pluginDistDir(pluginID))
	assertFileContents(t, stateFile, "state data")

	status := singlePackageStatus(t, host, pluginID)
	if status.Materialized {
		t.Fatalf("materialized = true after invalidation: %#v", status)
	}
	if !status.Invalidated || status.LastAction != "invalidate" || status.LastError != "" {
		t.Fatalf("unexpected invalidation status: %#v", status)
	}
}

func TestProcessHostSyncPluginDistRebuildsSelectedDistAndPreservesState(t *testing.T) {
	ctx := t.Context()
	host := newTestProcessHost(t)
	pluginID := "spacewave-app"

	oldDistFile := filepath.Join(host.pluginDistDir(pluginID), "stale.txt")
	stateFile := filepath.Join(host.pluginStateDir(pluginID), "state.txt")
	writeDiskFile(t, oldDistFile, []byte("stale dist"))
	writeDiskFile(t, stateFile, []byte("state data"))

	selectedDist := newTestDistHandle(t, map[string][]byte{
		"entrypoint":       []byte("#!/bin/sh\n"),
		"assets/app.mjs":   []byte("export const selected = true\n"),
		"selected-version": []byte("manifest-2\n"),
	})
	defer selectedDist.Release()

	distDir, err := host.syncPluginDist(ctx, pluginID, selectedDist)
	if err != nil {
		t.Fatal(err.Error())
	}

	assertPathMissing(t, oldDistFile)
	assertFileContents(t, filepath.Join(distDir, "selected-version"), "manifest-2\n")
	assertFileContents(t, filepath.Join(distDir, "assets/app.mjs"), "export const selected = true\n")
	assertFileContents(t, stateFile, "state data")

	status := singlePackageStatus(t, host, pluginID)
	if !status.Materialized || status.Invalidated || status.LastAction != "sync" || status.LastError != "" {
		t.Fatalf("unexpected sync status: %#v", status)
	}
	if status.DistDir != host.pluginDistDir(pluginID) {
		t.Fatalf("dist dir = %q, want %q", status.DistDir, host.pluginDistDir(pluginID))
	}
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
	t.Helper()
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
	t.Helper()
	for _, status := range statuses {
		if status.PluginID == pluginID {
			return status
		}
	}
	t.Fatalf("missing package status for %s: %#v", pluginID, statuses)
	return PluginPackageStatus{}
}

func newTestProcessHost(t *testing.T) *ProcessHost {
	t.Helper()

	stateDir := filepath.Join(t.TempDir(), "state")
	distDir := filepath.Join(t.TempDir(), "dist")
	if err := os.MkdirAll(stateDir, 0o755); err != nil {
		t.Fatal(err.Error())
	}
	if err := os.MkdirAll(distDir, 0o755); err != nil {
		t.Fatal(err.Error())
	}

	host, err := NewProcessHost(logrus.NewEntry(logrus.New()), stateDir, distDir)
	if err != nil {
		t.Fatal(err.Error())
	}
	return host
}

func newTestDistHandle(t *testing.T, files map[string][]byte) *unixfs.FSHandle {
	t.Helper()

	ctx := t.Context()
	rootRef, err := unixfs.NewFSHandle(unixfs_billy.NewBillyFSCursor(memfs.New(), ""))
	if err != nil {
		t.Fatal(err.Error())
	}
	bfs := unixfs_billy.NewBillyFS(ctx, rootRef, "", time.Now())
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

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err.Error())
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err.Error())
	}
}

func assertPathMissing(t *testing.T, path string) {
	t.Helper()

	if _, err := os.Stat(path); err == nil {
		t.Fatalf("%s exists, want missing", path)
	} else if !os.IsNotExist(err) {
		t.Fatal(err.Error())
	}
}

func assertFileContents(t *testing.T, path, want string) {
	t.Helper()

	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err.Error())
	}
	if string(body) != want {
		t.Fatalf("%s = %q, want %q", path, string(body), want)
	}
}

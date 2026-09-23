package npm

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
)

// TestEnsureBunAddIsolatesDownloads keeps concurrent target installs independent
// while serving the same package from a local registry substitute.
func TestEnsureBunAddIsolatesDownloads(t *testing.T) {
	// Serve a complete npm archive without relying on a public registry.
	var archive bytes.Buffer
	compressed := gzip.NewWriter(&archive)
	packed := tar.NewWriter(compressed)
	manifest := []byte(`{"name":"bldr-cache-fixture","version":"1.0.0"}`)
	if err := packed.WriteHeader(&tar.Header{
		Name: "package/package.json",
		Mode: 0o644,
		Size: int64(len(manifest)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := packed.Write(manifest); err != nil {
		t.Fatal(err)
	}
	if err := packed.Close(); err != nil {
		t.Fatal(err)
	}
	if err := compressed.Close(); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Resolve the pinned package through npm metadata before its tarball fetch.
		if r.URL.Path != "/fixture.tgz" {
			metadata := `{"name":"bldr-cache-fixture","dist-tags":{"latest":"1.0.0"},"versions":{"1.0.0":{"name":"bldr-cache-fixture","version":"1.0.0","dist":{"tarball":` + strconv.Quote("http://"+r.Host+"/fixture.tgz") + `}}}}`
			w.Header().Set("Content-Type", "application/json")
			http.ServeContent(w, r, "package.json", time.Time{}, bytes.NewReader([]byte(metadata)))
			return
		}

		// Return the archive for every target's independent download.
		http.ServeContent(w, r, "fixture.tgz", time.Time{}, bytes.NewReader(archive.Bytes()))
	}))
	t.Cleanup(server.Close)

	// Require each concurrent install to work with an unusable global cache.
	blockedCache := filepath.Join(t.TempDir(), "global-cache")
	if err := os.WriteFile(blockedCache, []byte("unavailable"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BUN_INSTALL_CACHE_DIR", blockedCache)
	for _, target := range []string{"arm64", "amd64"} {
		t.Run(target, func(t *testing.T) {
			t.Parallel()

			// Install the same archive through the real cached dependency helper.
			dir := t.TempDir()
			le := logrus.NewEntry(logrus.New())
			if err := EnsureBunAdd(t.Context(), le, "", dir, "bldr-cache-fixture@1.0.0", "NPM_CONFIG_REGISTRY="+server.URL); err != nil {
				t.Fatal(err)
			}

			// Both package materialization and private cache population must complete.
			if _, err := os.Stat(filepath.Join(dir, "node_modules", "bldr-cache-fixture", "package.json")); err != nil {
				t.Fatal(err)
			}
			cached, err := os.ReadDir(filepath.Join(dir, ".bun-cache"))
			if err != nil {
				t.Fatal(err)
			}
			if len(cached) == 0 {
				t.Fatal("download cache is empty")
			}
		})
	}
}

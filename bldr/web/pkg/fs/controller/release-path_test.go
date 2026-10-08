package web_pkg_fs_controller

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"
	"time"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/s4wave/spacewave/bldr/core"
	bldr_plugin "github.com/s4wave/spacewave/bldr/plugin"
	web_pkg_http "github.com/s4wave/spacewave/bldr/web/pkg/http"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_access "github.com/s4wave/spacewave/db/unixfs/access"
	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/sirupsen/logrus"
)

// TestPinnedPackageRedirect preserves the provider manifest in the module URL,
// including scoped package paths and the request query.
func TestPinnedPackageRedirect(t *testing.T) {
	// Register immutable package files on a real controller bus.
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	le := logrus.NewEntry(logrus.New())
	b, _, err := core.NewCoreBus(ctx, le)
	if err != nil {
		t.Fatal(err)
	}
	defer b.Close()

	// Build the immutable package filesystem and its manifest binding.
	root, err := hash.Sum(hash.RecommendedHashType, []byte("release"))
	if err != nil {
		t.Fatal(err)
	}
	artifact := bldr_plugin.PluginArtifactID("spacewave-web", root.MarshalString())
	fsID := bldr_plugin.PluginAssetsFsId(artifact)

	// Wrap package files in the UnixFS handle used by production lookups.
	cursor, err := unixfs_iofs.NewFSCursor(fstest.MapFS{
		"v/b/pkg/@s4wave/web/index.mjs": &fstest.MapFile{Data: []byte("export {}")},
	})
	if err != nil {
		t.Fatal(err)
	}
	handle, err := unixfs.NewFSHandle(cursor)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Release()

	// Attach the pinned files to the bus.
	files := unixfs_access.NewControllerWithHandle(le, b,
		controller.NewInfo("test/files", Version, "package files"), []string{fsID}, handle)
	releaseFiles, err := b.AddController(ctx, files, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releaseFiles()

	// Publish the package through the production filesystem getter.
	packages, err := NewController(le, b, &Config{
		UnixfsId: fsID, UnixfsPrefix: "v/b/pkg", WebPkgIdList: []string{"@s4wave/web"},
	})
	if err != nil {
		t.Fatal(err)
	}
	releasePackages, err := b.AddController(ctx, packages, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer releasePackages()

	// The unpinned entry redirects before any relative imports are resolved.
	req := httptest.NewRequest(http.MethodGet, "/b/pkg/@s4wave/web/index.mjs?mode=web", nil).WithContext(ctx)
	rec := httptest.NewRecorder()
	web_pkg_http.NewServer(le, b, false).ServeWebModuleHTTP("@s4wave/web/index.mjs", rec, req)
	want := "/b/pa/" + artifact + "/v/b/pkg/@s4wave/web/index.mjs?mode=web"
	if rec.Code != http.StatusTemporaryRedirect || rec.Header().Get("Location") != want || rec.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("redirect: status=%d headers=%v, want %s", rec.Code, rec.Header(), want)
	}
}

package bldr

import (
	"context"
	"testing"
	"testing/fstest"

	unixfs_iofs "github.com/s4wave/spacewave/db/unixfs/iofs"
	"github.com/sirupsen/logrus"
)

// TestDistSourcesFSCursor tests the web sources FSCursor build for errors.
func TestDistSourcesFSCursor(t *testing.T) {
	// Validate the embedded source tree and its top-level cursor.
	ifs, err := unixfs_iofs.NewFSCursor(DistSources)
	if err != nil {
		t.Fatal(err.Error())
	}
	if len(ifs.GetPath()) != 0 {
		t.Fail()
	}

	// Build the normalized source cursor used by the runtime.
	ifs = BuildDistSourcesFSCursor()
	if ifs == nil {
		t.Fatal("error in BuildDistSourcesFSCursor")
	}
	if len(ifs.GetPath()) != 0 {
		t.Fail()
	}

	// Prepare a context and logger for the embedded filesystem handle.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// fsRoot := unixfs.NewFS(ctx, le, fs, nil)
	// handle, err := fsRoot.AddRootReference(ctx)

	// Build the filesystem handle whose embedded paths the test validates.
	handle := BuildDistSourcesFSHandle(ctx, le)
	defer handle.Release()

	// Check the embedded filesystem's required source paths.
	// Check the embedded filesystem's required source paths.
	// check the fs handle mechanics via fstest
	ioFs := unixfs_iofs.NewFS(ctx, handle)
	err = fstest.TestFS(
		ioFs,
		"web/bldr/binary.ts",
		"web/bldr/web-runtime.ts",
		"web/electron/main/index.ts",
		"web/bldr-react/WebView.tsx",
		"devtool/web/entrypoint/startup-trace_js.go",
	)
	if err != nil {
		t.Fatal(err.Error())
	}
}

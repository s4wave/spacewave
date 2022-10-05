package bldr

import (
	"context"
	"testing"
	"testing/fstest"

	unixfs_iofs "github.com/aperturerobotics/hydra/unixfs/iofs"
	"github.com/sirupsen/logrus"
)

// TestWebSourcesFSCursor tests the web sources FSCursor build for errors.
func TestWebSourcesFSCursor(t *testing.T) {
	ifs, err := unixfs_iofs.NewFSCursor(WebSources)
	if err != nil {
		t.Fatal(err.Error())
	}
	ifs = BuildWebSourcesFSCursor()
	if ifs == nil {
		t.Fatal("error in BuildWebSourcesFSCursor")
	}
	if len(ifs.GetPath()) != 0 {
		t.Fail()
	}

	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	// fsRoot := unixfs.NewFS(ctx, le, fs, nil)
	// handle, err := fsRoot.AddRootReference(ctx)

	handle := BuildWebSourcesFSHandle(ctx, le)
	defer handle.Release()

	// check the fs handle mechanics via fstest
	ioFs := unixfs_iofs.NewFS(ctx, handle)
	err = fstest.TestFS(
		ioFs,
		"web/bldr/binary.ts",
		"web/bldr/web-runtime.ts",
		"web/electron/main/index.ts",
		"web/bldr-react/web-view.tsx",
	)
	if err != nil {
		t.Fatal(err.Error())
	}
}

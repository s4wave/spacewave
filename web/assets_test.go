package bldr_web

import (
	"context"
	"testing"
	"testing/fstest"

	"github.com/aperturerobotics/hydra/unixfs"
	unixfs_iofs "github.com/aperturerobotics/hydra/unixfs/iofs"
	"github.com/sirupsen/logrus"
)

// TestWebSourcesFSCursor tests the web sources FSCursor build for errors.
func TestWebSourcesFSCursor(t *testing.T) {
	fs, err := unixfs_iofs.NewFSCursor(WebSources)
	if err != nil {
		t.Fatal(err.Error())
	}
	fs = BuildWebSourcesFSCursor()
	if fs == nil {
		t.Fatal("error in BuildWebSourcesFSCursor")
	}
	if len(fs.GetPath()) != 0 {
		t.Fail()
	}

	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)

	fsRoot := unixfs.NewFS(ctx, le, fs, nil)
	handle, err := fsRoot.AddRootReference(ctx)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer handle.Release()

	// check the fs handle mechanics via fstest
	ifs := unixfs_iofs.NewFS(ctx, handle)
	err = fstest.TestFS(
		ifs,
		"bldr/binary.ts",
		"bldr/web-runtime.ts",
		"electron/main/index.ts",
		"bldr-react/web-view.tsx",
	)
	if err != nil {
		t.Fatal(err.Error())
	}
}

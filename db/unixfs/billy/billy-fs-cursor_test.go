package unixfs_billy_test

import (
	"context"
	"testing"

	"github.com/go-git/go-billy/v6/memfs"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
	unixfs_e2e "github.com/s4wave/spacewave/db/unixfs/e2e"
)

func TestBillyFSCursor(t *testing.T) {
	// Create the root of the fs to avoid a "not found" error.
	bfs := memfs.New()
	if err := bfs.MkdirAll("./", 0o755); err != nil {
		t.Fatal(err.Error())
	}

	// Retain a Billy cursor for the filesystem contract checks.
	fsc := unixfs_billy.NewBillyFSCursor(bfs, "")
	defer fsc.Release()

	// Retain a UnixFS handle for the filesystem contract checks.
	fsh, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsh.Release()

	// Exercise the UnixFS contract through the Billy cursor.
	ctx := context.Background()
	if err := unixfs_e2e.TestUnixFS(ctx, fsh); err != nil {
		t.Fatal(err.Error())
	}
}

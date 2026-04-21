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
	// we have to create the root of the fs or we get "not found"
	bfs := memfs.New()
	if err := bfs.MkdirAll("./", 0o755); err != nil {
		t.Fatal(err.Error())
	}

	fsc := unixfs_billy.NewBillyFSCursor(bfs, "")
	defer fsc.Release()

	fsh, err := unixfs.NewFSHandle(fsc)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsh.Release()

	ctx := context.Background()
	if err := unixfs_e2e.TestUnixFS(ctx, fsh); err != nil {
		t.Fatal(err.Error())
	}
}

package unixfs_billy_test

import (
	"context"
	"testing"
	"time"

	"github.com/go-git/go-billy/v6"
	"github.com/go-git/go-billy/v6/memfs"
	"github.com/s4wave/spacewave/db/unixfs"
	unixfs_billy "github.com/s4wave/spacewave/db/unixfs/billy"
)

// TestBillyFSCursorMemfsChmod proves chmod succeeds on a memfs-backed billy
// cursor, the RAM writable-root upper. memfs implements billy.Chmod but not the
// full billy.Change interface; asserting billy.Change here returned
// ErrNotSupported, which the v86fs setattr path collapses to EIO and which broke
// apt on the default --root-mode=ram. The shared e2e TestUnixFS swallows
// ErrNotSupported, so it could not catch this; this test asserts the chmod
// actually applies.
func TestBillyFSCursorMemfsChmod(t *testing.T) {
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
	ts := time.Date(2023, time.January, 1, 12, 0, 0, 0, time.UTC)
	if err := fsh.Mknod(ctx, true, []string{"chmod.txt"}, unixfs.NewFSCursorNodeType_File(), 0, ts); err != nil {
		t.Fatalf("create file: %v", err)
	}

	fhandle, err := fsh.Lookup(ctx, "chmod.txt")
	if err != nil {
		t.Fatalf("lookup file: %v", err)
	}

	if err := fhandle.SetPermissions(ctx, 0o600, ts); err != nil {
		if err == billy.ErrNotSupported {
			t.Fatal("SetPermissions returned ErrNotSupported on a memfs upper: the RAM writable root cannot chmod, so the guest sees EIO")
		}
		t.Fatalf("SetPermissions: %v", err)
	}

	fhandle, err = fsh.Lookup(ctx, "chmod.txt")
	if err != nil {
		t.Fatalf("re-lookup file: %v", err)
	}
	info, err := fhandle.GetFileInfo(ctx)
	if err != nil {
		t.Fatalf("get file info: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Fatalf("chmod not applied on memfs upper: got %o want %o", got, 0o600)
	}
}

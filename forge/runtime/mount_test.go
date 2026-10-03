package forge_runtime

import (
	"testing"

	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
	"github.com/sirupsen/logrus"
)

func TestV86WorkdirMountAttachOnceFlushAndRelease(t *testing.T) {
	// Start the World engine and writable Workdir relay testbed.
	ctx, eng, _ := newTestbed(t)
	le := logrus.NewEntry(logrus.New())
	server := unixfs_v86fs.NewServer(le, nil)
	handle, wtb, err := buildTestWorkdirHandle(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

	// Construct and attach the writable Workdir mount.
	mount, err := NewV86WorkdirMount(eng, server, "workdir", "/workspace", handle)
	if err != nil {
		t.Fatal(err)
	}
	if err := mount.Attach(); err != nil {
		t.Fatal(err)
	}
	if err := mount.Attach(); err == nil {
		t.Fatal("expected second attach to be rejected: one attempt mounts its workdir once")
	}

	// Check the registered Workdir mount name and guest path.
	mounts := server.ListMounts()
	if len(mounts) != 1 || mounts[0].Name != "workdir" || mounts[0].Path != "/workspace" {
		t.Fatalf("unexpected mounts: %+v", mounts)
	}

	// Check the Workdir durability fence for the current mount state.
	if err := mount.Flush(ctx); err != nil {
		t.Fatal(err)
	}

	// Release the Workdir mount and check that guest access is revoked.
	if err := mount.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if len(server.ListMounts()) != 0 {
		t.Fatal("release must revoke guest access by removing the mount")
	}

	// Check the Workdir durability fence for the current mount state.
	if err := mount.Flush(ctx); err == nil {
		t.Fatal("expected flush after release to fail")
	}
}

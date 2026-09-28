package forge_runtime

import (
	"testing"

	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
	"github.com/sirupsen/logrus"
)

func TestV86WorkdirMountAttachOnceFlushAndRelease(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	le := logrus.NewEntry(logrus.New())
	server := unixfs_v86fs.NewServer(le, nil)
	handle, wtb, err := buildTestWorkdirHandle(ctx, t)
	if err != nil {
		t.Fatal(err)
	}
	defer wtb.Release()

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
	mounts := server.ListMounts()
	if len(mounts) != 1 || mounts[0].Name != "workdir" || mounts[0].Path != "/workspace" {
		t.Fatalf("unexpected mounts: %+v", mounts)
	}
	if err := mount.Flush(ctx); err != nil {
		t.Fatal(err)
	}
	if err := mount.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if len(server.ListMounts()) != 0 {
		t.Fatal("release must revoke guest access by removing the mount")
	}
	if err := mount.Flush(ctx); err == nil {
		t.Fatal("expected flush after release to fail")
	}
}

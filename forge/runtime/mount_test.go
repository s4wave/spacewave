package forge_runtime

import (
	"testing"

	unixfs_v86fs "github.com/s4wave/spacewave/db/unixfs/v86fs"
	forge_lib_docker "github.com/s4wave/spacewave/forge/lib/docker"
	"github.com/sirupsen/logrus"
)

func TestApplyDockerWorkdirBindAddsSingleEntry(t *testing.T) {
	conf := &forge_lib_docker.Config{Image: "img"}
	if err := ApplyDockerWorkdirBind(conf, "/run/workdirs/exec-1", "/workspace"); err != nil {
		t.Fatal(err)
	}
	if len(conf.GetMounts()) != 1 {
		t.Fatalf("expected one mount, got %+v", conf.GetMounts())
	}
	m := conf.GetMounts()[0]
	if m.GetHostPath() != "/run/workdirs/exec-1" || m.GetContainerPath() != "/workspace" || m.GetReadOnly() {
		t.Fatalf("unexpected mount: %+v", m)
	}
	if err := ApplyDockerWorkdirBind(conf, "", "/x"); err == nil {
		t.Fatal("expected empty host path to be rejected")
	}
}

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

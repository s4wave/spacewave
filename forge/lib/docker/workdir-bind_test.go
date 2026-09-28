package forge_lib_docker

import "testing"

// TestApplyWorkdirBind adds only the requested host mount.
func TestApplyWorkdirBind(t *testing.T) {
	conf := &Config{Image: "img"}
	if err := ApplyWorkdirBind(conf, "/run/workdirs/exec-1", "/workspace"); err != nil {
		t.Fatal(err)
	}
	if len(conf.GetMounts()) != 1 {
		t.Fatalf("mount count = %d, want 1", len(conf.GetMounts()))
	}
	mount := conf.GetMounts()[0]
	if mount.GetHostPath() != "/run/workdirs/exec-1" || mount.GetContainerPath() != "/workspace" || mount.GetReadOnly() {
		t.Fatalf("unexpected mount: %+v", mount)
	}
	if err := ApplyWorkdirBind(conf, "", "/x"); err == nil {
		t.Fatal("empty host path was accepted")
	}
}

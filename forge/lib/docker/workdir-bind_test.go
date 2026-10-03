package forge_lib_docker

import "testing"

// TestApplyWorkdirBind adds only the requested host mount.
func TestApplyWorkdirBind(t *testing.T) {
	// Apply a host work directory bind to the container configuration.
	conf := &Config{Image: "img"}
	if err := ApplyWorkdirBind(conf, "/run/workdirs/exec-1", "/workspace"); err != nil {
		t.Fatal(err)
	}

	// Verify the bind mount paths and writable mode.
	if len(conf.GetMounts()) != 1 {
		t.Fatalf("mount count = %d, want 1", len(conf.GetMounts()))
	}
	mount := conf.GetMounts()[0]
	if mount.GetHostPath() != "/run/workdirs/exec-1" || mount.GetContainerPath() != "/workspace" || mount.GetReadOnly() {
		t.Fatalf("unexpected mount: %+v", mount)
	}

	// Verify that an empty host path cannot create a bind mount.
	if err := ApplyWorkdirBind(conf, "", "/x"); err == nil {
		t.Fatal("empty host path was accepted")
	}
}

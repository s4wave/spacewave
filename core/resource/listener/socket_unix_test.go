//go:build !windows && !js

package resource_listener

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListenProtectedUnixProtectsParentAndSocket(t *testing.T) {
	if err := os.MkdirAll(".tmp", 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(".tmp", "sock-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	parent := filepath.Join(root, "state")
	sock := filepath.Join(parent, "daemon.sock")
	lis, err := ListenProtectedUnix(sock, true)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	parentInfo, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	if got := parentInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("parent mode = %04o", got)
	}
	socketInfo, err := os.Stat(sock)
	if err != nil {
		t.Fatal(err)
	}
	if got := socketInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket mode = %04o", got)
	}
}

func TestListenProtectedUnixRefusesLooseExplicitParent(t *testing.T) {
	if err := os.MkdirAll(".tmp", 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(".tmp", "sock-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	parent := filepath.Join(root, "shared")
	if err := os.Mkdir(parent, 0o755); err != nil {
		t.Fatal(err)
	}
	sock := filepath.Join(parent, "daemon.sock")
	if _, err := ListenProtectedUnix(sock, false); err == nil {
		t.Fatal("expected loose parent refusal")
	}
	info, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Fatalf("parent changed to %04o", got)
	}
}

func TestListenProtectedUnixClosesOnSocketProtectionFailure(t *testing.T) {
	if err := os.MkdirAll(".tmp", 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(".tmp", "sock-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(root) })
	parent := filepath.Join(root, "state")
	sock := filepath.Join(parent, "daemon.sock")
	lis, err := listenProtectedUnix(sock, true, func(string, os.FileMode) error {
		return os.ErrPermission
	})
	if err == nil {
		if lis != nil {
			lis.Close()
		}
		t.Fatal("expected socket protection failure")
	}
	if lis != nil {
		t.Fatal("returned listener after protection failure")
	}
	if _, statErr := os.Stat(sock); !os.IsNotExist(statErr) {
		t.Fatalf("socket remains after protection failure: %v", statErr)
	}
}

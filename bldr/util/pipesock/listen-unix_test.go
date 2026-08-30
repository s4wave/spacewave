//go:build !windows

package pipesock

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
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	parent := filepath.Join(root, "state")
	sock := filepath.Join(parent, "debug.sock")
	lis, err := ListenProtectedUnix(sock)
	if err != nil {
		t.Fatal(err)
	}
	defer lis.Close()
	parentInfo, err := os.Stat(parent)
	if err != nil {
		t.Fatal(err)
	}
	if got := parentInfo.Mode().Perm(); got != 0o700 {
		t.Fatalf("parent mode = %04o, want 0700", got)
	}
	socketInfo, err := os.Stat(sock)
	if err != nil {
		t.Fatal(err)
	}
	if got := socketInfo.Mode().Perm(); got != 0o600 {
		t.Fatalf("socket mode = %04o, want 0600", got)
	}
}

func TestListenProtectedUnixClosesOnProtectionFailure(t *testing.T) {
	if err := os.MkdirAll(".tmp", 0o700); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(".tmp", "sock-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	parent := filepath.Join(root, "state")
	sock := filepath.Join(parent, "debug.sock")
	lis, err := listenProtectedUnix(sock, func(string, os.FileMode) error {
		return os.ErrPermission
	})
	if err == nil {
		if lis != nil {
			_ = lis.Close()
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

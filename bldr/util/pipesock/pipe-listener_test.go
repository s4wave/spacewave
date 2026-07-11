//go:build !windows

package pipesock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

func TestListenUsesShortUniquePrivateRootsAndCleansUp(t *testing.T) {
	ownerDir := filepath.Join(t.TempDir(), strings.Repeat("deep-checkout-segment-", 12), "sub", "vite")
	le := logrus.New().WithField("test", t.Name())

	first, err := Listen(le, ownerDir, "vite-abcd-1234")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Listen(le, ownerDir, "vite-abcd-1234")
	if err != nil {
		_ = first.Close()
		t.Fatal(err)
	}

	if first.GetRootDir() == second.GetRootDir() {
		t.Fatalf("concurrent listeners shared root %q", first.GetRootDir())
	}
	for _, listener := range []*PipeListener{first, second} {
		if len(listener.GetPath()) > maxSocketPathLength {
			t.Errorf("socket path is %d bytes: %q", len(listener.GetPath()), listener.GetPath())
		}
		if strings.HasPrefix(listener.GetPath(), ownerDir) {
			t.Errorf("socket path inherited deep owner prefix: %q", listener.GetPath())
		}
		info, err := os.Stat(listener.GetRootDir())
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o700 {
			t.Errorf("pipe root mode = %o, want 700", info.Mode().Perm())
		}
	}

	firstRoot := first.GetRootDir()
	secondRoot := second.GetRootDir()
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	if err := second.Close(); err != nil {
		t.Fatal(err)
	}
	for _, root := range []string{firstRoot, secondRoot} {
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Errorf("pipe root still exists after close: %q", root)
		}
	}
}

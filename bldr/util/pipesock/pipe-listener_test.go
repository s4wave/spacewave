//go:build !windows

package pipesock

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestListenUsesShortUniquePrivateRootsAndCleansUp checks that each listener
// gets a short private root that Close removes.
func TestListenUsesShortUniquePrivateRootsAndCleansUp(t *testing.T) {
	// Choose an owner directory deep enough to overflow a socket path.
	ownerDir := filepath.Join(t.TempDir(), strings.Repeat("deep-checkout-segment-", 12), "sub", "vite")
	le := logrus.New().WithField("test", t.Name())

	// Open two listeners for the same owner and name.
	first, err := Listen(le, ownerDir, "vite-abcd-1234")
	if err != nil {
		t.Fatal(err)
	}
	second, err := Listen(le, ownerDir, "vite-abcd-1234")
	if err != nil {
		_ = first.Close()
		t.Fatal(err)
	}

	// Each listener has its own short private root.
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

	// Closing each listener removes its root.
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

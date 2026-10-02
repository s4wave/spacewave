package keyfile

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sirupsen/logrus"
)

// TestOpenOrWritePrivKeyStatError tests that an unexpected stat error is
// returned instead of a nil key with nil error.
func TestOpenOrWritePrivKeyStatError(t *testing.T) {
	// Create a scratch directory for the key path.
	dir := t.TempDir()

	// A regular file where a directory component is expected makes os.Stat
	// fail with ENOTDIR, which is not os.IsNotExist.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(blocker, "nested", "key.pem")

	// Opening the key must report the stat error.
	le := logrus.NewEntry(logrus.New())
	privKey, err := OpenOrWritePrivKey(le, path)
	if err == nil {
		t.Fatalf("expected error for unreadable path, got key=%v", privKey)
	}
}

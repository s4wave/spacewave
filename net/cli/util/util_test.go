package cliutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	util_ulid "github.com/aperturerobotics/util/ulid"
)

func TestRunULID(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "ulid.txt")
	a := &UtilArgs{OutPath: outPath}
	if err := a.RunULID(nil); err != nil {
		t.Fatal(err)
	}

	dat, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	id := strings.TrimSpace(string(dat))
	if len(id) != util_ulid.EncodedSize {
		t.Fatalf("expected ULID length %d, got %d", util_ulid.EncodedSize, len(id))
	}
	if _, err := util_ulid.ParseULID(id); err != nil {
		t.Fatalf("expected valid ULID, got error: %v", err)
	}
}

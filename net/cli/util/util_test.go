package cliutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	util_ulid "github.com/aperturerobotics/util/ulid"
)

func TestRunULID(t *testing.T) {
	// Write a generated ULID to a temporary output file.
	outPath := filepath.Join(t.TempDir(), "ulid.txt")
	a := &UtilArgs{OutPath: outPath}
	if err := a.RunULID(nil); err != nil {
		t.Fatal(err)
	}

	// Read the output and validate its encoded value.
	dat, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatal(err)
	}

	// Confirm the generated text has the canonical ULID shape.
	id := strings.TrimSpace(string(dat))
	if len(id) != util_ulid.EncodedSize {
		t.Fatalf("expected ULID length %d, got %d", util_ulid.EncodedSize, len(id))
	}
	if _, err := util_ulid.ParseULID(id); err != nil {
		t.Fatalf("expected valid ULID, got error: %v", err)
	}
}

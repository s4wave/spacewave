//go:build !js

package spacewave_cli

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// TestDebugVolumeHas checks that the lookup reports a stored block present and
// an unstored one absent, that it skips blank lines and comments, and that it
// refuses an ID it cannot parse.
func TestDebugVolumeHas(t *testing.T) {
	// Open a fresh volume.
	ctx := t.Context()
	path := filepath.Join(t.TempDir(), "volume.s4wave")
	le := logrus.NewEntry(logrus.New())
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}

	// Store one block, and build the ref of another left unstored.
	stored, _, err := vol.PrepareOwnedBlock(ctx, "space", []byte("stored"), nil)
	if err != nil {
		t.Fatal(err)
	}
	unstored, err := block.BuildBlockRef([]byte("unstored"), nil)
	if err != nil {
		t.Fatal(err)
	}

	// Stop the volume, which one process opens once, and look both up.
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}
	ids := strings.Join([]string{"# holes", stored.MarshalString(), "", unstored.MarshalString()}, "\n")
	var out bytes.Buffer
	present, absent, err := runDebugVolumeHas(ctx, path, strings.NewReader(ids), &out)
	if err != nil {
		t.Fatal(err)
	}
	if present != 1 || absent != 1 {
		t.Fatalf("got %d present and %d absent, want 1 and 1", present, absent)
	}
	want := "present " + stored.MarshalString() + "\nabsent " + unstored.MarshalString() + "\n"
	if out.String() != want {
		t.Fatalf("got output %q, want %q", out.String(), want)
	}

	// An ID that is not a block ref fails with its line.
	_, _, err = runDebugVolumeHas(ctx, path, strings.NewReader("not-a-ref"), &out)
	if err == nil || !strings.Contains(err.Error(), "line 1") {
		t.Fatalf("got %v, want an error naming line 1", err)
	}
}

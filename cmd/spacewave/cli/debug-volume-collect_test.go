//go:build !js

package spacewave_cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/s4wave/spacewave/db/block"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// TestDebugVolumeCollect replaces a bucket head on a fresh volume, then checks
// that collection without frees keeps the replaced head and that collection
// with frees deletes it and keeps the live head, and that volume-state reads
// the stored frees switch each left without changing the file. It then runs
// the writer beside passes with frees and checks that it committed, released
// its scratch root, and left the live head stored.
func TestDebugVolumeCollect(t *testing.T) {
	// Write a head and replace it, so the first head is garbage.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	path := filepath.Join(t.TempDir(), "volume.s4wave")
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	put := func(data string, prev *block.BlockRef) *block.BlockRef {
		ref, err := block.BuildBlockRef([]byte(data), nil)
		if err != nil {
			t.Fatal(err)
		}
		entries := []*block.PutBatchEntry{{Ref: ref, Data: []byte(data), RefsKnown: true}}
		if err := vol.SetBucketRoots(ctx, "fill", entries, []block.NamedRoot{{Name: "head", Ref: ref, Previous: prev}}); err != nil {
			t.Fatal(err)
		}
		return ref
	}
	old := put("old head", nil)
	live := put("live head", old)
	vol.Close()

	// stored reports which of the two heads the stopped volume holds.
	stored := func() (bool, bool) {
		vol, err := openStoppedVolume(ctx, le, path, volumeReadOnly)
		if err != nil {
			t.Fatal(err)
		}
		defer vol.Close()
		oldOK, err := vol.GetBlockExists(ctx, old)
		if err != nil {
			t.Fatal(err)
		}
		liveOK, err := vol.GetBlockExists(ctx, live)
		if err != nil {
			t.Fatal(err)
		}
		return oldOK, liveOK
	}

	// frees reads the stored frees switch read-only, leaving the file's bytes
	// as they were.
	frees := func() bool {
		before, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		st, _, err := readVolumeState(ctx, le, path)
		if err != nil {
			t.Fatal(err)
		}
		after, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(before, after) {
			t.Fatal("reading the state changed the volume file")
		}
		return st.State.GetFrees()
	}

	// Without frees the passes net the logs and delete nothing.
	rep, err := collectVolume(ctx, le, path, &store_kvtx.Config{CollectorGraceDur: "1ns"}, 0, volumeLoadOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.res.Freed != 0 {
		t.Fatalf("freed %d without frees", rep.res.Freed)
	}
	if oldOK, liveOK := stored(); !oldOK || !liveOK {
		t.Fatalf("without frees: old head stored %v, live head stored %v", oldOK, liveOK)
	}
	if frees() {
		t.Fatal("state reads frees on after a collection without frees")
	}

	// With frees the replaced head goes and the live head stays.
	rep, err = collectVolume(ctx, le, path, &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"}, 0, volumeLoadOpts{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.res.Freed == 0 {
		t.Fatalf("freed nothing with frees in %d passes: %+v", rep.passes, rep.res)
	}
	if oldOK, liveOK := stored(); oldOK || !liveOK {
		t.Fatalf("with frees: old head stored %v, live head stored %v", oldOK, liveOK)
	}
	if !frees() {
		t.Fatal("state reads frees off after a collection with frees")
	}

	// A writer beside the passes commits, then releases its scratch root.
	load := volumeLoadOpts{rate: 50, window: 300 * time.Millisecond}
	rep, err = collectVolume(ctx, le, path, &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"}, 0, load)
	if err != nil {
		t.Fatal(err)
	}
	if rep.load == nil || len(rep.load.latencies) == 0 {
		t.Fatalf("the writer committed nothing: %+v", rep.load)
	}
	if _, liveOK := stored(); !liveOK {
		t.Fatal("the live head is gone after the load")
	}
	vol, err = openStoppedVolume(ctx, le, path, volumeReadOnly)
	if err != nil {
		t.Fatal(err)
	}
	defer vol.Close()
	head, err := vol.GetBucketRoot(ctx, volumeLoadBucket, volumeLoadRoot)
	if err != nil {
		t.Fatal(err)
	}
	if head != nil {
		t.Fatalf("the load's scratch root still holds %s", head.MarshalString())
	}
}

// TestDebugVolumeHolders reads a stopped temporary Volume through the command and leaves its file unchanged.
func TestDebugVolumeHolders(t *testing.T) {
	// Store a parent whose marked child list references one child.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	path := filepath.Join(t.TempDir(), "holders.s4wave")
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := vol.Close(); err != nil {
			t.Error(err)
		}
	})
	child, err := block.BuildBlockRef([]byte("child"), nil)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := block.BuildBlockRef([]byte("parent"), nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := []*block.PutBatchEntry{
		{Ref: child, Data: []byte("child"), RefsKnown: true},
		{Ref: parent, Data: []byte("parent"), Refs: []*block.BlockRef{child}, RefsKnown: true},
	}
	if err := vol.SetBucketRoots(ctx, "holders", entries, []block.NamedRoot{{Name: "head", Ref: parent}}); err != nil {
		t.Fatal(err)
	}
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	// Include tab-separated notes and use the real flag parser for two levels.
	refs := filepath.Join(t.TempDir(), "refs.tsv")
	if err := os.WriteFile(refs, []byte(child.MarshalString()+"\tinspect this child\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	app := &cli.App{Writer: &output, Commands: []*cli.Command{newDebugVolumeHoldersCommand()}}
	if err := app.RunContext(ctx, []string{"spacewave", "volume-holders", "--refs", refs, "--up", "2", path}); err != nil {
		t.Fatal(err)
	}

	// Confirm the parent's structural flags and the child relationship in its TSV row.
	want := strings.Join([]string{"1", parent.MarshalString(), child.MarshalString(), "1", "0",
		"true", "true", "true", "false", "false", "false", "false", ""}, "\t") + "\n"
	if !strings.Contains(output.String(), want) {
		t.Fatalf("parent row missing: %q", output.String())
	}
	if !strings.Contains(output.String(), "# level 1: 1 rows; 1 held by holders") || !strings.Contains(output.String(), "# level 2: 0 rows") {
		t.Fatalf("holder summary missing: %q", output.String())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(before, after) {
		t.Fatal("holder inspection changed the stopped Volume file")
	}
}

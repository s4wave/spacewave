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
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_gc "github.com/s4wave/spacewave/db/block/gc"
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

// TestDebugVolumeWriteOff runs the write-off command on a stopped volume whose
// live head lists a block the store lost with its list: the command records
// the hole, refuses a list that names a stored block without writing any hole
// off, and writes off the lost block when the list names only it.
func TestDebugVolumeWriteOff(t *testing.T) {
	// Store a head under a root, and list a child under it that was never
	// stored, as a volume that lost the child's bytes and list reads.
	ctx := t.Context()
	le := logrus.NewEntry(logrus.New())
	dir := t.TempDir()
	path := filepath.Join(dir, "volume.s4wave")
	vol, err := volume_s4db.NewVolume(ctx, le, &volume_s4db.Config{Path: path})
	if err != nil {
		t.Fatal(err)
	}
	head, err := block.BuildBlockRef([]byte("head"), nil)
	if err != nil {
		t.Fatal(err)
	}
	lost, err := block.BuildBlockRef([]byte("lost child"), nil)
	if err != nil {
		t.Fatal(err)
	}
	entries := []*block.PutBatchEntry{{Ref: head, Data: []byte("head"), RefsKnown: true}}
	if err := vol.SetBucketRoots(ctx, "fill", entries, []block.NamedRoot{{Name: "head", Ref: head}}); err != nil {
		t.Fatal(err)
	}
	writeRecords(t, vol, func(rec *block_gc.Records) error {
		return rec.PutChildren(ctx, head, []*block.BlockRef{lost})
	})
	if err := vol.Close(); err != nil {
		t.Fatal(err)
	}

	// holes reads the recorded and the open holes of the stopped volume.
	holes := func() (int, int) {
		st, _, err := readVolumeState(ctx, le, path)
		if err != nil {
			t.Fatal(err)
		}
		return st.Holes, st.OpenHoles
	}
	refsFile := func(name string, refs ...*block.BlockRef) string {
		var lines []string
		for _, ref := range refs {
			lines = append(lines, ref.MarshalString()+"\tnote")
		}
		file := filepath.Join(dir, name)
		if err := os.WriteFile(file, []byte(strings.Join(lines, "\n")), 0o600); err != nil {
			t.Fatal(err)
		}
		return file
	}

	// listed checks that the holes output holds the lost block alone, in
	// the given state.
	listing := filepath.Join(dir, "holes.tsv")
	listed := func(state string) {
		t.Helper()
		data, err := os.ReadFile(listing)
		if err != nil {
			t.Fatal(err)
		}
		if want := lost.MarshalString() + "\t" + state + "\n"; string(data) != want {
			t.Fatalf("holes output %q, want %q", data, want)
		}
	}

	// A list that names no block, or a stored block beside the hole, writes
	// off nothing. The audit still records the hole, and the refused run
	// still lists it.
	if _, err := runVolumeBlockCLI(t, "volume-write-off", path); err == nil {
		t.Fatal("the write-off ran with neither a list nor a holes output")
	}
	if _, err := runVolumeBlockCLI(t, "volume-write-off", "--refs", refsFile("none.tsv"), path); err == nil {
		t.Fatal("the write-off accepted an empty list")
	}
	if recorded, open := holes(); recorded != 0 || open != 0 {
		t.Fatalf("an empty list left %d holes, %d open; want none recorded", recorded, open)
	}
	_, err = runVolumeBlockCLI(t, "volume-write-off", "--refs", refsFile("stored.tsv", lost, head), "--holes-out", listing, path)
	if err == nil || !errors.Is(err, block_gc.ErrNotHole) || !strings.Contains(err.Error(), head.MarshalString()) {
		t.Fatalf("a write-off naming a stored block: %v", err)
	}
	if recorded, open := holes(); recorded != 1 || open != 1 {
		t.Fatalf("the refused write-off left %d holes, %d open; want 1 and 1", recorded, open)
	}
	listed("open")

	// The holes output alone lists the hole and writes none off, and its
	// rows are a list the write-off reads.
	if err := os.Remove(listing); err != nil {
		t.Fatal(err)
	}
	if _, err := runVolumeBlockCLI(t, "volume-write-off", "--holes-out", listing, path); err != nil {
		t.Fatal(err)
	}
	listed("open")
	if recorded, open := holes(); recorded != 1 || open != 1 {
		t.Fatalf("the listing left %d holes, %d open; want 1 and 1", recorded, open)
	}
	fromListing := filepath.Join(dir, "from-listing.tsv")
	if err := os.Link(listing, fromListing); err != nil {
		t.Fatal(err)
	}
	if _, err := runVolumeBlockCLI(t, "volume-write-off", "--refs", fromListing, "--holes-out", listing, path); err == nil {
		t.Fatal("the write-off overwrote its own list")
	}

	// The list of the lost block alone writes it off, on a first and a
	// second run.
	want := map[string]string{
		"Holes under a live root": "1", "Holes the audit recorded": "0", "Holes written off": "1",
		"Named holes that ended first": "0", "Recorded holes": "1", "Holes not written off": "0",
	}
	for range 2 {
		report, err := runVolumeBlockCLI(t, "volume-write-off", "--refs", refsFile("lost.tsv", lost), "--holes-out", listing, path)
		if err != nil {
			t.Fatal(err)
		}
		listed("written-off")
		for line := range strings.SplitSeq(strings.TrimSpace(report), "\n") {
			label, value, _ := strings.Cut(line, ":")
			if count, ok := want[label]; ok && strings.TrimSpace(value) != count {
				t.Fatalf("write-off row %q, want %s:\n%s", line, count, report)
			}
		}
		for label := range want {
			if !strings.Contains(report, label+":") {
				t.Fatalf("write-off report lacks %q:\n%s", label, report)
			}
		}
		if recorded, open := holes(); recorded != 1 || open != 0 {
			t.Fatalf("the write-off left %d holes, %d open; want 1 and 0", recorded, open)
		}
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
		"true", "true", "true", "false", "false", "false", "false", "", "", ""}, "\t") + "\n"
	if !strings.Contains(output.String(), want) {
		t.Fatalf("parent row missing: %q", output.String())
	}
	root := strings.Join([]string{"root", parent.MarshalString(), "", "", "", "", "", "", "", "", "", "", "", "holders", "head"}, "\t") + "\n"
	if !strings.Contains(output.String(), root) || !strings.Contains(output.String(), "# roots: 1 rows\n") {
		t.Fatalf("root row or summary missing: %q", output.String())
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

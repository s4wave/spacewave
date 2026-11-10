//go:build !js

package spacewave_cli

import (
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	volume_s4db "github.com/s4wave/spacewave/db/volume/s4db"
	"github.com/sirupsen/logrus"
)

// TestDebugVolumeCollect replaces a bucket head on a fresh volume, then checks
// that collection without frees keeps the replaced head and that collection
// with frees deletes it and keeps the live head.
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
		vol, err := openStoppedVolume(ctx, le, path)
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

	// Without frees the passes net the journal and delete nothing.
	res, _, err := collectVolume(ctx, le, path, &store_kvtx.Config{CollectorGraceDur: "1ns"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Freed != 0 {
		t.Fatalf("freed %d without frees", res.Freed)
	}
	if oldOK, liveOK := stored(); !oldOK || !liveOK {
		t.Fatalf("without frees: old head stored %v, live head stored %v", oldOK, liveOK)
	}

	// With frees the replaced head goes and the live head stays.
	res, passes, err := collectVolume(ctx, le, path, &store_kvtx.Config{CollectorFrees: true, CollectorGraceDur: "1ns"})
	if err != nil {
		t.Fatal(err)
	}
	if res.Freed == 0 {
		t.Fatalf("freed nothing with frees in %d passes: %+v", passes, res)
	}
	if oldOK, liveOK := stored(); oldOK || !liveOK {
		t.Fatalf("with frees: old head stored %v, live head stored %v", oldOK, liveOK)
	}
}

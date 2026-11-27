//go:build darwin || linux || windows

package packmove_test

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/block/store/s3/s3test"
	kvtx_prefixer "github.com/s4wave/spacewave/db/kvtx/prefixer"
	"github.com/s4wave/spacewave/db/s4db"
	"github.com/s4wave/spacewave/db/volume/packmove"
	"github.com/sirupsen/logrus"
)

// findUnpacked indexes the bucket's packs into the index file, as a fresh
// process does, and writes the volume's blocks they lack to listPath.
func findUnpacked(ctx context.Context, t *testing.T, client *block_store_s3.Client, indexPath, src, listPath string) packmove.UnpackedProgress {
	// Index every pack from the bucket.
	t.Helper()
	idx, err := s4db.Open(indexPath, s4db.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer idx.Close()
	packs := block_store_s3.NewPackStore(logrus.NewEntry(logrus.New()), client, s3test.BucketName, packPrefix, &block_store_s3.PackStoreOptions{
		KeepPacks: true,
		Index:     kvtx_prefixer.NewPrefixer(idx, block_store_s3.PackIndexPrefix(worldStoreID)),
	})
	defer packs.Close()
	if err := packs.BuildIndex(ctx, 2, nil); err != nil {
		t.Fatal(err)
	}

	// List the volume's blocks no pack holds.
	db, err := s4db.Open(src, s4db.Options{ReadOnly: true})
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	out, err := os.Create(listPath)
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	p, err := packmove.FindUnpacked(ctx, db, newKeys(t), packs, out, nil)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

// readList returns the keys of a key list.
func readList(t *testing.T, path string) map[string]bool {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	keys := map[string]bool{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		key, err := hex.DecodeString(sc.Text())
		if err != nil {
			t.Fatal(err)
		}
		keys[string(key)] = true
	}
	return keys
}

// TestPackListFill packs the blocks of a stopped volume that the bucket's
// packs lack, with a killed upload resumed, and checks that a fresh diff then
// finds only the block whose data fails its hash.
func TestPackListFill(t *testing.T) {
	// Lay out the files and the test bucket.
	ctx := t.Context()
	dir := t.TempDir()
	src, logPath := filepath.Join(dir, "archive.s4db"), filepath.Join(dir, "fill.log")
	_, client := s3test.New(t)
	packs := newPackStore(client)
	defer packs.Close()

	// Fill the volume: World blocks with references, a block of another
	// store, and a block that fails its hash.
	f := openFixture(t, src, s4db.Options{})
	world := f.putWorld("base", 120)
	settings := f.put(f.other, "settings")
	rotted := f.putBad("rotted")
	f.close()

	// The bucket already holds the first half of the World.
	var held []*block.PutBatchEntry
	for i, ref := range world[:60] {
		held = append(held, &block.PutBatchEntry{Ref: ref, Data: blockData("base-" + itoa(i)), RefsKnown: true})
	}
	if _, err := packs.PutBlockBatch(ctx, held); err != nil {
		t.Fatal(err)
	}

	// The diff lists the other half, the other store's block and the rotted
	// one, and nothing the bucket holds.
	first := findUnpacked(ctx, t, client, filepath.Join(dir, "index-1.s4db"), src, filepath.Join(dir, "missing-1"))
	listed := readList(t, filepath.Join(dir, "missing-1"))
	if first.Missing != 62 || uint64(len(listed)) != first.Missing || first.Examined != 122 {
		t.Fatalf("first diff %+v, %d listed", first, len(listed))
	}
	for _, ref := range world[:60] {
		key, _ := ref.MarshalKey()
		if listed[string(key)] {
			t.Fatalf("a held block is listed")
		}
	}

	// Pack the list in small packs. The first run dies after one upload.
	packList := func(packer packmove.BlockPacker) (packmove.PackProgress, error) {
		db, err := s4db.Open(src, s4db.Options{ReadOnly: true})
		if err != nil {
			t.Fatal(err)
		}
		defer db.Close()
		return packmove.PackList(ctx, packmove.PackConfig{
			Source:       db,
			Keys:         newKeys(t),
			WorldStoreID: worldStoreID,
			Packer:       packer,
			LogPath:      logPath,
			ListPath:     filepath.Join(dir, "missing-1"),
			PackBytes:    8 * blockSize,
		})
	}
	if _, err := packList(&stopAfterPut{PackStore: packs}); err == nil {
		t.Fatal("the killed run succeeded")
	}
	done, err := packList(packs)
	if err != nil {
		t.Fatal(err)
	}
	if done.Blocks != 61 || done.Bad != 1 {
		t.Fatalf("pack %+v", done)
	}

	// A fresh index of the bucket finds only the rotted block missing.
	second := findUnpacked(ctx, t, client, filepath.Join(dir, "index-2.s4db"), src, filepath.Join(dir, "missing-2"))
	rottedKey, _ := rotted.MarshalKey()
	if left := readList(t, filepath.Join(dir, "missing-2")); second.Missing != 1 || !left[string(rottedKey)] {
		t.Fatalf("second diff %+v", second)
	}

	// Every good block reads from the packs with its data and references.
	reader := newPackStore(client)
	defer reader.Close()
	for i, ref := range world {
		stored, err := reader.GetStoredBlock(ctx, ref)
		if err != nil || stored == nil || !bytes.Equal(stored.Data, blockData("base-"+itoa(i))) {
			t.Fatalf("block %d: %v", i, err)
		}
		if i >= 60 && (len(stored.Refs) != 1 || !stored.Refs[0].EqualsRef(world[i-1])) {
			t.Fatalf("block %d refs %v", i, stored.Refs)
		}
	}
	if stored, err := reader.GetStoredBlock(ctx, settings); err != nil || stored == nil {
		t.Fatalf("settings block: %v", err)
	}
}

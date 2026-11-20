package unixfs_world_testbed

import (
	"bytes"
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

// TestFolderLinkSync links a folder, then syncs it again as a restart does:
// the unchanged folder links no file and touches no index entry, and a file
// edited in place is the only one linked again, replacing its Drive node
// without reading the old contents, which the World's store cannot serve.
func TestFolderLinkSync(t *testing.T) {
	// Start the World and the Drive.
	ctx := context.Background()
	le := logrus.NewEntry(logrus.New())
	htb, err := hydra_testbed.NewTestbed(ctx, le)
	if err != nil {
		t.Fatal(err.Error())
	}
	wtb, err := world_testbed.NewTestbed(htb)
	if err != nil {
		t.Fatal(err.Error())
	}
	fsHandle, err := InitTestbed(wtb, objKey, true)
	if err != nil {
		t.Fatal(err.Error())
	}
	defer fsHandle.Release()

	// Fill a folder with small files in nested directories and one large file.
	folder := t.TempDir()
	const smallFiles = 200
	for i := range smallFiles {
		dir := filepath.Join(folder, fmt.Sprintf("d%d", i%10), "sub")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err.Error())
		}
		data := bytes.Repeat([]byte{byte(i)}, 100+i*50)
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%d", i)), data, 0o644); err != nil {
			t.Fatal(err.Error())
		}
	}
	big := make([]byte, 8<<20)
	_, _ = rand.Read(big)
	bigPath := filepath.Join(folder, "big.bin")
	if err := os.WriteFile(bigPath, big, 0o644); err != nil {
		t.Fatal(err.Error())
	}

	// sync runs one sync through a new link, as a restart does, and returns
	// the files it linked and the index transactions it opened.
	kv := kvtx_kvtest.NewCountingStore(store_kvtx_inmem.NewStore())
	idx := block_store_filesegment.NewIndex(kv)
	sync := func() (uint64, uint64) {
		t.Helper()
		link, err := unixfs_world.NewFolderLink(wtb.Engine, idx, objKey, folder, []string{"linked", "folder"})
		if err != nil {
			t.Fatal(err.Error())
		}
		before := kv.Counts().Txns
		var linked uint64
		err = link.Sync(ctx, func(files, _ uint64) error {
			linked = files
			return nil
		})
		if err != nil {
			t.Fatal(err.Error())
		}
		return linked, kv.Counts().Txns - before
	}

	// The first sync links every file.
	if linked, _ := sync(); linked != smallFiles+1 {
		t.Fatalf("first sync linked %d files, want %d", linked, smallFiles+1)
	}

	// A restart over the unchanged folder links nothing and reads no index.
	if linked, txns := sync(); linked != 0 || txns != 0 {
		t.Fatalf("restart linked %d files with %d index transactions, want none", linked, txns)
	}

	// Overwrite 1 MiB inside the large file: only it is linked again.
	edit := make([]byte, 1<<20)
	_, _ = rand.Read(edit)
	f, err := os.OpenFile(bigPath, os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err.Error())
	}
	if _, err := f.WriteAt(edit, 3<<20); err != nil {
		t.Fatal(err.Error())
	}
	if err := f.Close(); err != nil {
		t.Fatal(err.Error())
	}
	later := time.Now().Add(time.Second)
	if err := os.Chtimes(bigPath, later, later); err != nil {
		t.Fatal(err.Error())
	}
	if linked, _ := sync(); linked != 1 {
		t.Fatalf("sync after the edit linked %d files, want 1", linked)
	}

	// The Drive now matches the folder again.
	if linked, txns := sync(); linked != 0 || txns != 0 {
		t.Fatalf("sync after relinking linked %d files with %d index transactions, want none", linked, txns)
	}
}

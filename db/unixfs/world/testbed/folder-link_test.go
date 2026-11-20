package unixfs_world_testbed

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	unixfs_block "github.com/s4wave/spacewave/db/unixfs/block"
	unixfs_world "github.com/s4wave/spacewave/db/unixfs/world"
	"github.com/s4wave/spacewave/db/world"
	world_testbed "github.com/s4wave/spacewave/db/world/testbed"
	"github.com/sirupsen/logrus"
)

// TestFolderLinkSync links a folder, then syncs it again as a restart does:
// the unchanged folder links no file and touches no index entry, and a file
// edited in place is the only one linked again, replacing its Drive node
// without reading the old contents, which the World's store cannot serve.
// Files and directories removed from the folder leave the Drive, an entry
// that changed between file and directory changes type, and a targeted sync
// of the directories a watcher names finds the changes in them.
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

	// A restart over the unchanged folder links nothing and reads the index
	// only for the root's identity.
	if linked, txns := sync(); linked != 0 || txns != 1 {
		t.Fatalf("restart linked %d files with %d index transactions, want only the root check", linked, txns)
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
	if linked, txns := sync(); linked != 0 || txns != 1 {
		t.Fatalf("sync after relinking linked %d files with %d index transactions, want only the root check", linked, txns)
	}

	// kind returns the type of the Drive entry at the folder-relative path,
	// read from a fresh World snapshot: the test's handle follows the World
	// asynchronously.
	kind := func(rel string) string {
		t.Helper()
		rtx, err := wtb.Engine.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err.Error())
		}
		defer rtx.Discard()
		fsType, _, err := unixfs_world.LookupFsType(ctx, rtx, objKey)
		if err != nil {
			t.Fatal(err.Error())
		}
		obj, _, err := rtx.GetObject(ctx, objKey)
		defer world.ReleaseObjectState(obj)
		if err != nil {
			t.Fatal(err.Error())
		}
		out := "dir"
		_, _, err = unixfs_world.AccessUnixfsObject(ctx, obj, false, fsType, func(root *unixfs_block.FSTree) error {
			node := root
			for _, name := range append([]string{"linked", "folder"}, strings.Split(rel, "/")...) {
				child, dirent, err := node.LookupFollowDirent(name)
				if err != nil {
					return err
				}
				switch {
				case dirent == nil:
					out = "none"
					return nil
				case !dirent.GetIsDirectory():
					out = "file"
				}
				node = child
			}
			return nil
		}, nil)
		if err != nil {
			t.Fatal(err.Error())
		}
		return out
	}
	expect := func(want map[string]string) {
		t.Helper()
		for rel, k := range want {
			if got := kind(rel); got != k {
				t.Fatalf("%s is %s in the Drive, want %s", rel, got, k)
			}
		}
	}
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// Remove a file and a directory, turn a directory into a file and a file
	// into a directory: the sync links only the two new files.
	must(os.Remove(filepath.Join(folder, "d1", "sub", "f1")))
	must(os.RemoveAll(filepath.Join(folder, "d2")))
	must(os.RemoveAll(filepath.Join(folder, "d3", "sub")))
	must(os.WriteFile(filepath.Join(folder, "d3", "sub"), []byte("now a file"), 0o644))
	must(os.Remove(bigPath))
	must(os.MkdirAll(filepath.Join(bigPath, "inner"), 0o755))
	must(os.WriteFile(filepath.Join(bigPath, "inner", "x"), []byte("inside"), 0o644))
	if linked, _ := sync(); linked != 2 {
		t.Fatalf("sync after the removals linked %d files, want 2", linked)
	}
	expect(map[string]string{
		"d1/sub/f1":       "none",
		"d1/sub/f11":      "file",
		"d2":              "none",
		"d3/sub":          "file",
		"big.bin":         "dir",
		"big.bin/inner/x": "file",
	})

	// A targeted sync finds a new file, a new nested directory synced from
	// its nearest ancestor the Drive holds, and a removed file.
	must(os.WriteFile(filepath.Join(folder, "d4", "new.txt"), []byte("new"), 0o644))
	must(os.MkdirAll(filepath.Join(folder, "d5", "sub", "deep"), 0o755))
	must(os.WriteFile(filepath.Join(folder, "d5", "sub", "deep", "n.txt"), []byte("deep"), 0o644))
	must(os.Remove(filepath.Join(folder, "d6", "sub", "f6")))
	link, err := unixfs_world.NewFolderLink(wtb.Engine, idx, objKey, folder, []string{"linked", "folder"})
	must(err)
	var linked uint64
	must(link.SyncDirs(ctx, []string{"d4", "d5/sub/deep", "d6/sub"}, func(files, _ uint64) error {
		linked = files
		return nil
	}))
	if linked != 2 {
		t.Fatalf("targeted sync linked %d files, want 2", linked)
	}
	expect(map[string]string{
		"d4/new.txt":        "file",
		"d5/sub/deep/n.txt": "file",
		"d6/sub/f6":         "none",
	})
	if linked, txns := sync(); linked != 0 || txns != 1 {
		t.Fatalf("full sync after the targeted sync linked %d files with %d index transactions, want only the root check", linked, txns)
	}

	// A stand-in root on another file system, here an emptied folder whose
	// recorded identity differs, removes nothing; with the identity back,
	// the sync resumes and removes what the folder lacks.
	real, found, err := idx.FolderRoot(ctx, folder)
	must(err)
	if !found {
		t.Fatal("the first sync recorded no root identity")
	}
	must(idx.SetFolderRoot(ctx, folder, real+1))
	ents, err := os.ReadDir(folder)
	must(err)
	for _, e := range ents {
		must(os.RemoveAll(filepath.Join(folder, e.Name())))
	}
	link, err = unixfs_world.NewFolderLink(wtb.Engine, idx, objKey, folder, []string{"linked", "folder"})
	must(err)
	if err := link.Sync(ctx, nil); !errors.Is(err, unixfs_world.ErrFolderRootChanged) {
		t.Fatalf("sync of the stand-in root returned %v, want ErrFolderRootChanged", err)
	}
	if err := link.SyncDirs(ctx, []string{"d4"}, nil); !errors.Is(err, unixfs_world.ErrFolderRootChanged) {
		t.Fatalf("targeted sync of the stand-in root returned %v, want ErrFolderRootChanged", err)
	}
	expect(map[string]string{"d4/new.txt": "file", "d3/sub": "file"})
	must(idx.SetFolderRoot(ctx, folder, real))
	must(link.Sync(ctx, nil))
	expect(map[string]string{"d4": "none", "big.bin": "none"})
}

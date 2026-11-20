package unixfs_world_testbed

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"

	block_store_filesegment "github.com/s4wave/spacewave/db/block/store/filesegment"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	store_kvtx_inmem "github.com/s4wave/spacewave/db/store/kvtx/inmem"
	hydra_testbed "github.com/s4wave/spacewave/db/testbed"
	"github.com/s4wave/spacewave/db/unixfs"
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
	idx := block_store_filesegment.NewIndex(kv, nil)
	sync := func() (uint64, uint64) {
		t.Helper()
		link, err := unixfs_world.NewFolderLink(wtb.Engine, idx, objKey, folder, []string{"linked", "folder"}, false)
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
	link, err := unixfs_world.NewFolderLink(wtb.Engine, idx, objKey, folder, []string{"linked", "folder"}, false)
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

	// Reindex links a file whose bytes changed under its size and
	// modification time, which a stat walk cannot see, and syncs the
	// directory of a path that is gone.
	stalePath := filepath.Join(folder, "d7", "sub", "f7")
	staleInfo, err := os.Stat(stalePath)
	must(err)
	staleData, err := os.ReadFile(stalePath)
	must(err)
	staleData[0] ^= 0xff
	must(os.WriteFile(stalePath, staleData, 0o644))
	must(os.Chtimes(stalePath, staleInfo.ModTime(), staleInfo.ModTime()))
	if linked, _ := sync(); linked != 0 {
		t.Fatalf("stat walk linked %d files changed under their stat, want 0", linked)
	}
	link, err = unixfs_world.NewFolderLink(wtb.Engine, idx, objKey, folder, []string{"linked", "folder"}, false)
	must(err)
	linked = 0
	must(link.Reindex(ctx, []string{"d7/sub/f7", "d7/sub/gone"}, func(files, _ uint64) error {
		linked = files
		return nil
	}))
	if linked != 1 {
		t.Fatalf("reindex linked %d files, want 1", linked)
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
	link, err = unixfs_world.NewFolderLink(wtb.Engine, idx, objKey, folder, []string{"linked", "folder"}, false)
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

// TestFolderLinkTwoWay pins the remote write of a two-way link: a member's
// write replaces a file holding its base, and one that meets an edit on disk
// keeps both versions, the edit at the path and the member's under its
// conflict name (MemberWriteKept, RemoteReplacesOnlyBase). The next sync
// publishes the edit over the member's version, which it compared, and the
// conflict copy as a new file.
func TestFolderLinkTwoWay(t *testing.T) {
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
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// Link a folder of two files both ways.
	folder := t.TempDir()
	must(os.WriteFile(filepath.Join(folder, "a.txt"), []byte("owner a"), 0o644))
	must(os.WriteFile(filepath.Join(folder, "b.txt"), []byte("owner b"), 0o644))
	idx := block_store_filesegment.NewIndex(store_kvtx_inmem.NewStore(), nil)
	dest := []string{"linked", "folder"}
	link, err := unixfs_world.NewFolderLink(wtb.Engine, idx, objKey, folder, dest, true)
	must(err)
	must(link.Sync(ctx, nil))

	// A member writes both files while the owner edits b.txt on disk.
	rtx, err := wtb.Engine.NewTransaction(ctx, false)
	must(err)
	fsType, _, err := unixfs_world.LookupFsType(ctx, rtx, objKey)
	rtx.Discard()
	must(err)
	member := unixfs_world.NewBatchFSWriter(world.NewEngineWorldState(wtb.Engine, true), objKey, fsType, "")
	for _, name := range []string{"a.txt", "b.txt"} {
		data := []byte("member " + name)
		must(member.AddFile(ctx, dest, name, unixfs.NewFSCursorNodeType_File(), int64(len(data)), bytes.NewReader(data), 0o644, time.Now()))
	}
	must(member.Commit(ctx))
	member.Release()
	edited := time.Now().Add(time.Minute)
	must(os.WriteFile(filepath.Join(folder, "b.txt"), []byte("owner b2"), 0o644))
	must(os.Chtimes(filepath.Join(folder, "b.txt"), edited, edited))

	// The sync applies the write over the base and keeps both versions of
	// the conflict, then publishes the owner's edit and the conflict copy.
	var linked uint64
	must(link.Sync(ctx, func(files, _ uint64) error {
		linked = files
		return nil
	}))
	if n, err := link.Unapplied(); n != 0 {
		t.Fatalf("%d remote changes left unapplied: %v", n, err)
	}
	read := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(folder, name))
		must(err)
		return string(data)
	}
	if got := read("a.txt"); got != "member a.txt" {
		t.Fatalf("a.txt holds %q, want the member's write", got)
	}
	if got := read("b.txt"); got != "owner b2" {
		t.Fatalf("b.txt holds %q, want the owner's edit", got)
	}
	copies, err := filepath.Glob(filepath.Join(folder, "b.txt.conflict-*"))
	must(err)
	if len(copies) != 1 || read(filepath.Base(copies[0])) != "member b.txt" {
		t.Fatalf("conflict copies %v, want one holding the member's write", copies)
	}
	if linked != 2 {
		t.Fatalf("sync published %d files, want the edit and the conflict copy", linked)
	}

	// The folder and the Drive agree: the next sync applies and links nothing.
	linked = 0
	must(link.Sync(ctx, func(files, _ uint64) error {
		linked = files
		return nil
	}))
	if linked != 0 {
		t.Fatalf("second sync linked %d files, want 0", linked)
	}
}

func TestFolderLinkStageRecovery(t *testing.T) {
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
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err.Error())
		}
	}

	// Link a folder of three files both ways.
	folder := t.TempDir()
	put := func(name, data string, mtime time.Time) {
		t.Helper()
		must(os.WriteFile(filepath.Join(folder, name), []byte(data), 0o644))
		must(os.Chtimes(filepath.Join(folder, name), mtime, mtime))
	}
	owned := time.Unix(1_700_000_000, 0)
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		put(name, "owner "+name, owned)
	}
	idx := block_store_filesegment.NewIndex(store_kvtx_inmem.NewStore(), nil)
	dest := []string{"linked", "folder"}
	link, err := unixfs_world.NewFolderLink(wtb.Engine, idx, objKey, folder, dest, true)
	must(err)
	must(link.Sync(ctx, nil))

	// A member writes all three files.
	rtx, err := wtb.Engine.NewTransaction(ctx, false)
	must(err)
	fsType, _, err := unixfs_world.LookupFsType(ctx, rtx, objKey)
	rtx.Discard()
	must(err)
	member := unixfs_world.NewBatchFSWriter(world.NewEngineWorldState(wtb.Engine, true), objKey, fsType, "")
	written := time.Unix(1_800_000_000, 0)
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		data := []byte("member " + name)
		must(member.AddFile(ctx, dest, name, unixfs.NewFSCursorNodeType_File(), int64(len(data)), bytes.NewReader(data), 0o644, written))
	}
	must(member.Commit(ctx))
	member.Release()

	// The node crashed in each place a staged file outlives: after the
	// exchange of a.txt, which displaced its base, and of b.txt, which
	// displaced an owner's edit; and before the exchange of c.txt, holding
	// the member's fetched version, and a later fetch of c.txt cut short.
	put("a.txt", "member a.txt", written)
	put(".spacewave-stage-0000000a-a.txt", "owner a.txt", owned)
	put("b.txt", "member b.txt", written)
	put(".spacewave-stage-0000000b-b.txt", "owner b2", time.Now())
	put(".spacewave-stage-0000000c-c.txt", "member c.txt", written)
	put(".spacewave-fetch-0000000d-c.txt", "memb", time.Now())

	// The restarted node removes the versions the Drive holds and the cut
	// fetch, keeps the edit under its conflict name and publishes it, and
	// applies c.txt again.
	link, err = unixfs_world.NewFolderLink(wtb.Engine, idx, objKey, folder, dest, true)
	must(err)
	var linked uint64
	count := func(files, _ uint64) error {
		linked = files
		return nil
	}
	must(link.Sync(ctx, count))
	if linked != 1 {
		t.Fatalf("sync published %d files, want the conflict copy", linked)
	}
	if n, err := link.Unapplied(); n != 0 {
		t.Fatalf("%d remote changes left unapplied: %v", n, err)
	}
	stages, err := filepath.Glob(filepath.Join(folder, ".spacewave-*"))
	must(err)
	if len(stages) != 0 {
		t.Fatalf("staged files left: %v", stages)
	}
	read := func(name string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(folder, name))
		must(err)
		return string(data)
	}
	for _, name := range []string{"a.txt", "b.txt", "c.txt"} {
		if got := read(name); got != "member "+name {
			t.Fatalf("%s holds %q, want the member's write", name, got)
		}
	}
	copies, err := filepath.Glob(filepath.Join(folder, "*.conflict-*"))
	must(err)
	if len(copies) != 1 || !strings.HasPrefix(filepath.Base(copies[0]), "b.txt.conflict-") || read(filepath.Base(copies[0])) != "owner b2" {
		t.Fatalf("conflict copies %v, want one of b.txt holding the owner's edit", copies)
	}

	// The folder and the Drive agree: the next sync applies and links nothing.
	linked = 0
	must(link.Sync(ctx, count))
	if linked != 0 {
		t.Fatalf("second sync linked %d files, want 0", linked)
	}
}

// TestFolderLinkApplyRaces tries to make a two-way link's apply path lose,
// expose or invent a version. An owner's edit races each step of a member's
// write and delete: before the check, between the check and the exchange,
// through a handle still open on the displaced file, between the check and
// the move to the trash, and before the rename that never replaces. An edit
// in place keeps the base's size and modification time, which the stat check
// cannot see. And the node crashes at each step, its sync frozen there, and
// restarts. Each case ends with every version that must survive at its path,
// under a conflict name or in the trash with its entry, holding its own
// bytes, and no other file in the folder (NoLostVersion, MemberWriteKept,
// RemoteReplacesOnlyBase, TrashOnlyDeleted).
func TestFolderLinkApplyRaces(t *testing.T) {
	// Start the World and the Drive, with a trash of the test's own.
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
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err.Error())
		}
	}
	dataHome, home := t.TempDir(), t.TempDir()
	t.Setenv("XDG_DATA_HOME", dataHome)
	t.Setenv("HOME", home)
	trashFiles := filepath.Join(dataHome, "Trash", "files")
	if runtime.GOOS == "darwin" {
		trashFiles = filepath.Join(home, ".Trash")
		must(os.Mkdir(trashFiles, 0o700))
	}
	rtx, err := wtb.Engine.NewTransaction(ctx, false)
	must(err)
	fsType, _, err := unixfs_world.LookupFsType(ctx, rtx, objKey)
	rtx.Discard()
	must(err)

	// The versions: the base and its same-size edit, the member's write and
	// an owner's file with its size, an edit, and a new file.
	const (
		base    = "owner base"
		coarse  = "owner EDIT"
		member  = "member wrote"
		mimic   = "owner twelve"
		edit    = "owner edited it"
		created = "owner new"
		handled = "OWNER base"
	)
	owned, written := time.Unix(1_700_000_000, 0), time.Unix(1_800_000_000, 0)
	put := func(path, data string, mtime time.Time) {
		must(os.WriteFile(path, []byte(data), 0o644))
		must(os.Chtimes(path, mtime, mtime))
	}

	// The owner's actions, each on the file at path.
	editIt := func(path string) { put(path, edit, time.Now()) }
	recreate := func(path string) { put(path, created, time.Now()) }
	remove := func(path string) { must(os.Remove(path)) }
	coarseEdit := func(path string) {
		f, err := os.OpenFile(path, os.O_WRONLY, 0)
		must(err)
		_, err = f.WriteAt([]byte(coarse), 0)
		must(err)
		must(f.Close())
		must(os.Chtimes(path, owned, owned))
	}

	type action = func(path string)
	type races = map[unixfs_world.ApplyStep]action
	none := unixfs_world.ApplyStep(-1)
	cases := []struct {
		// name names the case.
		name string
		// del makes the member delete the file instead of writing it.
		del bool
		// handle opens the file before the sync, and writes into it, through
		// the handle, at the exchange; keep restores its stat too.
		handle, keep bool
		// before acts on the file after the member's change, before the sync.
		before action
		// race acts on the file at each step.
		race races
		// crash freezes the sync at the step, after its race, and restarts.
		crash unixfs_world.ApplyStep
		// deny makes the displaced file unreadable after the exchange, failing
		// its check, then readable again for a second pass.
		deny bool
		// at, conflicts and trashed are the versions that end at the path,
		// under conflict names and in the trash.
		at        string
		conflicts []string
		trashed   []string
	}{
		{name: "edit before the check", race: races{unixfs_world.ApplyStaged: editIt}, crash: none, at: edit, conflicts: []string{member}},
		{name: "edit before the exchange", race: races{unixfs_world.ApplyChecked: editIt}, crash: none, at: member, conflicts: []string{edit}},
		{name: "edit through a handle after the exchange", handle: true, crash: none, at: member, conflicts: []string{handled}},
		{name: "same-stat edit before the check", before: coarseEdit, crash: none, at: member, conflicts: []string{coarse}},
		{name: "same-stat edit before the exchange", race: races{unixfs_world.ApplyChecked: coarseEdit}, crash: none, at: member, conflicts: []string{coarse}},
		{name: "same-stat edit through a handle after the exchange", handle: true, keep: true, crash: none, at: member, conflicts: []string{handled}},
		{name: "other bytes under the member's stat", before: func(path string) { put(path, mimic, written) }, crash: none, at: mimic, conflicts: []string{member}},
		{name: "file created before the restore", before: remove, race: races{unixfs_world.ApplyRestoring: recreate}, crash: none, at: created, conflicts: []string{member}},
		{name: "delete", del: true, crash: none, trashed: []string{base}},
		{name: "edit before the delete's check", del: true, before: editIt, crash: none, at: edit},
		{name: "edit before the move to the trash", del: true, race: races{unixfs_world.ApplyChecked: editIt}, crash: none, at: edit},
		{name: "same-stat edit before the move to the trash", del: true, race: races{unixfs_world.ApplyChecked: coarseEdit}, crash: none, at: coarse},
		{name: "edit trashed, then the path recreated", del: true, race: races{unixfs_world.ApplyChecked: editIt, unixfs_world.ApplyTrashed: recreate}, crash: none, at: created, conflicts: []string{edit}},
		{name: "check of an exchanged edit fails", race: races{unixfs_world.ApplyChecked: editIt}, deny: true, crash: none, at: member, conflicts: []string{edit}},
		{name: "crash after the fetch", crash: unixfs_world.ApplyStaged, at: member},
		{name: "crash after the check", crash: unixfs_world.ApplyChecked, at: member},
		{name: "crash after the exchange", crash: unixfs_world.ApplySwapped, at: member},
		{name: "crash after an edit was exchanged", race: races{unixfs_world.ApplyChecked: editIt}, crash: unixfs_world.ApplySwapped, at: member, conflicts: []string{edit}},
		{name: "crash after a same-stat edit was exchanged", race: races{unixfs_world.ApplyChecked: coarseEdit}, crash: unixfs_world.ApplySwapped, at: member, conflicts: []string{coarse}},
		{name: "crash before the restore", before: remove, crash: unixfs_world.ApplyRestoring, at: member},
		{name: "crash after the delete's check", del: true, crash: unixfs_world.ApplyChecked, trashed: []string{base}},
		{name: "crash after the move to the trash", del: true, crash: unixfs_world.ApplyTrashed, trashed: []string{base}},
		{name: "crash after an edit moved to the trash", del: true, race: races{unixfs_world.ApplyChecked: editIt}, crash: unixfs_world.ApplyTrashed, trashed: []string{edit}},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			must := func(err error) {
				t.Helper()
				if err != nil {
					t.Fatal(err.Error())
				}
			}

			// Link a folder of one file both ways, named for the case so
			// the shared trash tells the cases apart.
			folder := t.TempDir()
			name := fmt.Sprintf("c%02d.txt", i)
			path := filepath.Join(folder, name)
			put(path, base, owned)
			idx := block_store_filesegment.NewIndex(store_kvtx_inmem.NewStore(), nil)
			dest := []string{"races", fmt.Sprintf("c%02d", i)}
			link, err := unixfs_world.NewFolderLink(wtb.Engine, idx, objKey, folder, dest, true)
			must(err)
			must(link.Sync(ctx, nil))

			// The member writes or deletes the file, then the owner acts.
			writer := unixfs_world.NewBatchFSWriter(world.NewEngineWorldState(wtb.Engine, true), objKey, fsType, "")
			if tc.del {
				must(writer.Remove(dest, name))
			} else {
				must(writer.AddFile(ctx, dest, name, unixfs.NewFSCursorNodeType_File(), int64(len(member)), strings.NewReader(member), 0o644, written))
			}
			must(writer.Commit(ctx))
			writer.Release()
			if tc.before != nil {
				tc.before(path)
			}
			race := tc.race
			stageMode := func(mode os.FileMode) {
				stages, err := filepath.Glob(filepath.Join(folder, ".spacewave-stage-*"))
				must(err)
				for _, stage := range stages {
					must(os.Chmod(stage, mode))
				}
			}
			if tc.deny {
				race = maps.Clone(race)
				if race == nil {
					race = races{}
				}
				race[unixfs_world.ApplySwapped] = func(string) { stageMode(0) }
			}
			if tc.handle {
				h, err := os.OpenFile(path, os.O_WRONLY, 0)
				must(err)
				defer h.Close()
				race = races{unixfs_world.ApplySwapped: func(string) {
					_, err := h.WriteAt([]byte(handled[:5]), 0)
					must(err)
					if tc.keep {
						stages, err := filepath.Glob(filepath.Join(folder, ".spacewave-stage-*"))
						must(err)
						for _, stage := range stages {
							must(os.Chtimes(stage, owned, owned))
						}
					}
				}}
			}

			// Apply the change with the races, and, for a crash, freeze the
			// sync at its step and restart the node.
			frozen := make(chan struct{})
			link.SetApplyHook(func(step unixfs_world.ApplyStep, at string) {
				if at != path {
					return
				}
				if act := race[step]; act != nil {
					act(at)
				}
				if step == tc.crash {
					close(frozen)
					select {}
				}
			})
			switch {
			case tc.deny:
				// The failed check leaves the change for the next pass.
				must(link.Sync(ctx, nil))
				if n, _ := link.Unapplied(); n == 0 {
					t.Fatal("the failed check applied the change")
				}
				stageMode(0o644)
				must(link.Sync(ctx, nil))
			case tc.crash == none:
				must(link.Sync(ctx, nil))
			default:
				go func() { _ = link.Sync(ctx, nil) }()
				<-frozen
				link, err = unixfs_world.NewFolderLink(wtb.Engine, idx, objKey, folder, dest, true)
				must(err)
				must(link.Sync(ctx, nil))
			}
			if n, err := link.Unapplied(); n != 0 {
				t.Fatalf("%d remote changes left unapplied: %v", n, err)
			}

			// The path, the conflict names and nothing else hold the versions.
			read := func(path string) string {
				t.Helper()
				data, err := os.ReadFile(path)
				must(err)
				return string(data)
			}
			ents, err := os.ReadDir(folder)
			must(err)
			var at string
			var conflicts []string
			for _, e := range ents {
				switch {
				case e.Name() == name:
					at = read(path)
				case strings.HasPrefix(e.Name(), name+".conflict-"):
					conflicts = append(conflicts, read(filepath.Join(folder, e.Name())))
				default:
					t.Fatalf("folder holds %s, which no version names", e.Name())
				}
			}
			if at != tc.at {
				t.Fatalf("path holds %q, want %q", at, tc.at)
			}
			slices.Sort(conflicts)
			if !slices.Equal(conflicts, tc.conflicts) {
				t.Fatalf("conflict copies hold %q, want %q", conflicts, tc.conflicts)
			}

			// The trash holds the versions it should, each with its entry.
			trashed, err := filepath.Glob(filepath.Join(trashFiles, strings.TrimSuffix(name, ".txt")+"*"))
			must(err)
			var got []string
			for _, f := range trashed {
				got = append(got, read(f))
				if runtime.GOOS != "linux" {
					continue
				}
				info := read(filepath.Join(dataHome, "Trash", "info", filepath.Base(f)+".trashinfo"))
				if !strings.Contains(info, "\nPath="+path+"\n") {
					t.Fatalf("trash entry %q does not name %s", info, path)
				}
			}
			if !slices.Equal(got, tc.trashed) {
				t.Fatalf("trash holds %q, want %q", got, tc.trashed)
			}
			if runtime.GOOS == "linux" {
				infos, err := filepath.Glob(filepath.Join(dataHome, "Trash", "info", strings.TrimSuffix(name, ".txt")+"*"))
				must(err)
				if len(infos) != len(tc.trashed) {
					t.Fatalf("trash entries %v, want one per trashed version", infos)
				}
			}
		})
	}
}

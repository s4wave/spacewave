//go:build !js && !wasip1

package main

import (
	"bytes"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/packahead"
	"github.com/s4wave/spacewave/db/packfile/writer"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	recovery "github.com/s4wave/spacewave/db/world/recovery"
	"github.com/s4wave/spacewave/net/hash"
)

// tornTail is the garbage a crash left past the open log's last batch.
var tornTail = bytes.Repeat([]byte{0xee}, 29)

// makeTierFixture moves three fixture blocks out of SQLite: the live child
// into a finalized local pack, the refused body and the live body into an open
// local log with a torn tail, and the live body again, beside one corrupt
// value, into a retained remote pack. It returns the fixture, its declared
// pack directories and the number of pack files they hold.
func makeTierFixture(t *testing.T) (*fixture, []string, uint64) {
	t.Helper()
	ctx := t.Context()
	f := makeFixture(t)
	moved := []*block.BlockRef{f.liveChild, f.refused.GetRootRef(), f.liveBody.GetRootRef()}
	values := takeBlocks(t, f.source.GetPath(), moved)
	encode := func(ref *block.BlockRef) packahead.Block {
		value, err := block.EncodeBlockObject(values[ref.MarshalString()], nil, false)
		if err != nil {
			t.Fatal(err)
		}
		return packahead.Block{Key: packfile.BlockKey(ref.GetHash()), Value: value}
	}

	// Write the local log with the store's own writer; its next Open
	// finalizes the first pack, and the second stays open.
	local := filepath.Join(t.TempDir(), "packahead")
	for _, batch := range [][]*block.BlockRef{{f.liveChild}, {f.refused.GetRootRef(), f.liveBody.GetRootRef()}} {
		log, err := packahead.Open(ctx, local, packahead.Options{})
		if err != nil {
			t.Fatal(err)
		}
		var blocks []packahead.Block
		for _, ref := range batch {
			blocks = append(blocks, encode(ref))
		}
		if err := log.Append(ctx, blocks); err != nil {
			t.Fatal(err)
		}
		log.Close()
	}
	entries, err := os.ReadDir(local)
	if err != nil {
		t.Fatal(err)
	}
	var packs uint64
	torn := false
	for _, entry := range entries {
		open, finalized := packahead.ClassifyName(entry.Name())
		if !open && !finalized {
			continue
		}
		packs++
		path := filepath.Join(local, entry.Name())
		if info, err := os.Stat(path); err != nil || !open || info.Size() == 0 {
			continue
		}
		torn = true
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := file.Write(tornTail); err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
	}
	if !torn {
		t.Fatal("fixture left no open local log")
	}

	// Write a retained remote pack holding a duplicate and a corrupt value.
	remote := filepath.Join(t.TempDir(), "remote")
	if err := os.Mkdir(remote, 0o700); err != nil {
		t.Fatal(err)
	}
	file, err := os.Create(filepath.Join(remote, "retained.kvf"))
	if err != nil {
		t.Fatal(err)
	}
	pending := []struct {
		h    *hash.Hash
		data []byte
	}{{f.liveBody.GetRootRef().GetHash(), values[f.liveBody.GetRootRef().MarshalString()]}, {f.liveChild.GetHash(), []byte("not the live child")}}
	if _, err := writer.PackBlocks(file, func() (*hash.Hash, *block.StoredBlock, error) {
		if len(pending) == 0 {
			return nil, nil, nil
		}
		next := pending[0]
		pending = pending[1:]
		return next.h, &block.StoredBlock{Data: next.data}, nil
	}); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	packs++

	// Declare both tiers; every count but locations and the pack rows holds.
	f.source.Packs = []*recovery.PackSource{{Path: local, Tier: recovery.PackTier_PACK_TIER_LOCAL}, {Path: remote, Tier: recovery.PackTier_PACK_TIER_REMOTE}}
	f.expected.PhysicalLocations -= 2
	f.expected.PackFiles = packs
	f.expected.UnreadTailBytes = uint64(len(tornTail))
	f.expected.CorruptValues = 1
	return f, []string{local, remote}, packs
}

// takeBlocks deletes the refs' rows from both fixture tables and returns
// their bytes by ref string.
func takeBlocks(t *testing.T, path string, refs []*block.BlockRef) map[string][]byte {
	t.Helper()
	db, err := sql.Open(sqliteDriver(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	want := map[string]bool{}
	for _, ref := range refs {
		want[ref.MarshalString()] = true
	}
	rows, err := db.Query("SELECT key, value FROM volume")
	if err != nil {
		t.Fatal(err)
	}
	values := map[string][]byte{}
	var keys [][]byte
	prefix := store_kvkey.NewDefaultKVKey().GetBlockFullPrefix()
	for rows.Next() {
		var key, value []byte
		if err := rows.Scan(&key, &value); err != nil {
			t.Fatal(err)
		}
		encoded, ok := bytes.CutPrefix(key, prefix)
		ref := &block.BlockRef{}
		if !ok || ref.UnmarshalVT(encoded) != nil || !want[ref.MarshalString()] {
			continue
		}
		values[ref.MarshalString()] = value
		keys = append(keys, key)
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		t.Fatal(err)
	}
	if len(keys) != len(refs) {
		t.Fatal("fixture lacks a block to move")
	}
	for _, table := range []string{"volume", "duplicate"} {
		for _, key := range keys {
			if _, err := db.Exec("DELETE FROM "+table+" WHERE key = ?", key); err != nil {
				t.Fatal(err)
			}
		}
	}
	return values
}

// treeStamp records the bytes and metadata of each source path and, for a
// directory, of every entry in it.
func treeStamp(t *testing.T, paths []string) map[string][]any {
	t.Helper()
	stamp := map[string][]any{}
	for _, path := range paths {
		stamp[path] = sourceStamp(t, path)
		entries, err := os.ReadDir(path)
		if err != nil {
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			stamp[path] = append(stamp[path], data)
			continue
		}
		for _, entry := range entries {
			child := filepath.Join(path, entry.Name())
			data, _ := os.ReadFile(child)
			info, err := os.Lstat(child)
			if err != nil {
				t.Fatal(err)
			}
			stamp[child] = []any{info.Mode(), info.Size(), info.ModTime(), data}
		}
	}
	return stamp
}

// changedPaths counts the source paths whose stamp moved, appeared or went.
func changedPaths(t *testing.T, before map[string][]any) int {
	t.Helper()
	after := treeStamp(t, rootPaths(before))
	changed := 0
	for path, stamp := range before {
		if !reflect.DeepEqual(stamp, after[path]) {
			changed++
		}
	}
	for path := range after {
		if _, ok := before[path]; !ok {
			changed++
		}
	}
	return changed
}

// rootPaths returns the stamped paths whose parent was not stamped.
func rootPaths(stamp map[string][]any) []string {
	var roots []string
	for path := range stamp {
		if _, ok := stamp[filepath.Dir(path)]; !ok {
			roots = append(roots, path)
		}
	}
	return roots
}

// TestRecoveryTiers counts the fixture with blocks in a finalized local pack,
// an open log with a torn tail and a retained remote pack, fresh and resumed
// after a failure between packs.
func TestRecoveryTiers(t *testing.T) {
	// The tier counts match the all-SQLite counts but for the moved locations.
	f, dirs, _ := makeTierFixture(t)
	stamp := treeStamp(t, append([]string{f.source.GetPath()}, dirs...))
	scratch := filepath.Join(t.TempDir(), "tiers.sqlite")
	var output bytes.Buffer
	if err := runCount(t.Context(), &output, f.source, scratch, countOptions()); err != nil {
		t.Fatal(err)
	}
	fresh, err := recovery.ReadResult(scratch)
	if err != nil {
		t.Fatal(err)
	}
	if !fresh.GetCounts().EqualVT(f.expected) {
		t.Fatalf("tier count mismatch\ngot: %v\nwant: %v", fresh.GetCounts(), f.expected)
	}
	gaps := strings.Join(fresh.GetGaps(), "\n")
	for _, want := range []string{"bytes past their last valid record", "s4db stores are not read", "paylog BlockDomain values", "remote catalogs are not read"} {
		if !strings.Contains(gaps, want) {
			t.Fatalf("report omitted gap %q", want)
		}
	}
	if strings.Contains(gaps, "no local pack log was declared") || strings.Contains(gaps, "no retained remote packs") {
		t.Fatal("declared tiers still reported as gaps")
	}
	if !strings.Contains(output.String(), "Corrupt pack values") || !strings.Contains(output.String(), "local pack log "+dirs[0]) {
		t.Fatal("report omitted pack rows or tier coverage")
	}
	if changed := changedPaths(t, stamp); changed != 0 {
		t.Fatalf("scan changed %d source paths", changed)
	}
	t.Log(output.String())

	// A failure after the first pack resumes to the same result.
	resumed := filepath.Join(t.TempDir(), "resumed.sqlite")
	injected := errors.New("synthetic pack read failure")
	opts := countOptions()
	var read uint64
	fired := false
	opts.Committed = func(result *recovery.Result) error { read = result.GetCounts().GetPackFiles(); return nil }
	opts.BeforeRead = func() error {
		if read == 1 && !fired {
			fired = true
			return injected
		}
		return nil
	}
	if err := runCount(t.Context(), io.Discard, f.source, resumed, opts); !errors.Is(err, injected) {
		t.Fatalf("injected pack failure: %v", err)
	}
	if err := runCount(t.Context(), io.Discard, f.source, resumed, countOptions()); err != nil {
		t.Fatal(err)
	}
	result, err := recovery.ReadResult(resumed)
	if err != nil {
		t.Fatal(err)
	}
	if !result.GetCounts().EqualVT(fresh.GetCounts()) || !reflect.DeepEqual(result.GetGaps(), fresh.GetGaps()) {
		t.Fatal("resumed tier count differs from fresh")
	}
}

// TestRecoveryReadOnlyPacks tries to make the scan write to its pack sources:
// a torn open log that the store's own opener would truncate and finalize,
// read-only files and directories, a link entry, an unreadable pack, and a
// scratch file placed inside a pack directory. The sources must not change.
func TestRecoveryReadOnlyPacks(t *testing.T) {
	// Scratch inside a captured pack directory is refused before any read.
	f, dirs, packs := makeTierFixture(t)
	if _, err := recovery.NewScanner(t.Context(), f.source, filepath.Join(dirs[0], "scratch.sqlite"), countOptions()); err == nil {
		t.Fatal("scratch inside a pack directory was accepted")
	}

	// Add a link entry and a pack with no index, then freeze every source.
	if err := os.Symlink(f.source.GetPath(), filepath.Join(dirs[1], "link.kvf")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dirs[1], "unindexed.kvf"), []byte("no kvfile index here"), 0o600); err != nil {
		t.Fatal(err)
	}
	var frozen []string
	for _, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.Type().IsRegular() {
				frozen = append(frozen, filepath.Join(dir, entry.Name()))
			}
		}
		frozen = append(frozen, dir)
	}
	t.Cleanup(func() {
		for _, path := range frozen {
			_ = os.Chmod(path, 0o700)
		}
	})
	for _, path := range frozen {
		if err := os.Chmod(path, 0o500); err != nil {
			t.Fatal(err)
		}
	}
	stamp := treeStamp(t, append([]string{f.source.GetPath()}, dirs...))

	// Count, then prove every byte, size, inode and time of the sources held.
	scratch := filepath.Join(t.TempDir(), "frozen.sqlite")
	if err := runCount(t.Context(), io.Discard, f.source, scratch, countOptions()); err != nil {
		t.Fatal(err)
	}
	result, err := recovery.ReadResult(scratch)
	if err != nil {
		t.Fatal(err)
	}
	count := result.GetCounts()
	if count.GetPackFiles() != packs+1 || count.GetUnreadablePacks() != 1 || count.GetUnreadTailBytes() != uint64(len(tornTail)) || count.GetPhysicalBlocks() != f.expected.GetPhysicalBlocks() {
		t.Fatalf("frozen tier counts: %v", count)
	}
	gaps := strings.Join(result.GetGaps(), "\n")
	if !strings.Contains(gaps, "not regular files are not read") || !strings.Contains(gaps, "no readable index") {
		t.Fatal("link entry or unreadable pack was not reported as a gap")
	}
	if changed := changedPaths(t, stamp); changed != 0 {
		t.Fatalf("scan changed %d source paths", changed)
	}
}

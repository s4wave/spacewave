//go:build js

package metashard

import (
	"context"
	"strings"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
	"github.com/s4wave/spacewave/db/volume/js/opfs/pagestore"
)

func newTestMetaShard(t *testing.T, name string) *MetaShard {
	t.Helper()
	if !opfs.SyncAvailable() {
		t.Skip("sync access handles not available")
	}
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, name, true)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = opfs.DeleteEntry(root, name, true)
	})
	ms, err := NewMetaShard(dir, name, 0)
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func reopenTestMetaShard(t *testing.T, name string) *MetaShard {
	t.Helper()
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, name, false)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := NewMetaShard(dir, name, 0)
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func putMetaValue(t *testing.T, ms *MetaShard, key, value string) {
	t.Helper()
	if err := ms.WriteTx(func(tree *pagestore.Tree) error {
		return tree.Put([]byte(key), []byte(value))
	}); err != nil {
		t.Fatal(err)
	}
}

func TestMetaShardReadSnapshotIsolation(t *testing.T) {
	ms := newTestMetaShard(t, "test-metashard-snapshot")
	store := NewMetaStore(ms)
	ctx := context.Background()

	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("k"), []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	readTx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()

	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Set(ctx, []byte("k"), []byte("v2")); err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	val, found, err := readTx.Get(ctx, []byte("k"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "v1" {
		t.Fatalf("snapshot read got found=%v val=%q want v1", found, val)
	}

	liveTx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer liveTx.Discard()
	val, found, err = liveTx.Get(ctx, []byte("k"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "v2" {
		t.Fatalf("live read got found=%v val=%q want v2", found, val)
	}
}

func TestMetaShardWriteTxMultipleMutations(t *testing.T) {
	ms := newTestMetaShard(t, "test-metashard-multi-mutation")
	store := NewMetaStore(ms)
	ctx := context.Background()

	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("k1"), []byte("v1")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("k2"), []byte("v2")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	readTx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()

	val, found, err := readTx.Get(ctx, []byte("k1"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "v1" {
		t.Fatalf("k1 got found=%v val=%q want v1", found, val)
	}

	val, found, err = readTx.Get(ctx, []byte("k2"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "v2" {
		t.Fatalf("k2 got found=%v val=%q want v2", found, val)
	}
}

func TestMetaShardRecoveryBeforeSuperblockFlip(t *testing.T) {
	ms := newTestMetaShard(t, "test-metashard-before-flip")
	putMetaValue(t, ms, "k", "v1")

	hookErr := errors.New("boom-before-flip")
	ms.testHook = func(stage string) error {
		if stage == "after-page-close" {
			return hookErr
		}
		return nil
	}
	err := ms.WriteTx(func(tree *pagestore.Tree) error {
		return tree.Put([]byte("k"), []byte("v2"))
	})
	if !errors.Is(err, hookErr) {
		t.Fatalf("expected hook err, got %v", err)
	}

	reopened := reopenTestMetaShard(t, "test-metashard-before-flip")
	val, found, err := reopened.Get([]byte("k"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "v1" {
		t.Fatalf("reopened value got found=%v val=%q want v1", found, val)
	}
}

func TestMetaShardRecoveryAfterSuperblockFlip(t *testing.T) {
	ms := newTestMetaShard(t, "test-metashard-after-flip")
	putMetaValue(t, ms, "k", "v1")

	hookErr := errors.New("boom-after-flip")
	ms.testHook = func(stage string) error {
		if stage == "after-superblock-write" {
			return hookErr
		}
		return nil
	}
	err := ms.WriteTx(func(tree *pagestore.Tree) error {
		return tree.Put([]byte("k"), []byte("v2"))
	})
	if !errors.Is(err, hookErr) {
		t.Fatalf("expected hook err, got %v", err)
	}

	reopened := reopenTestMetaShard(t, "test-metashard-after-flip")
	val, found, err := reopened.Get([]byte("k"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "v2" {
		t.Fatalf("reopened value got found=%v val=%q want v2", found, val)
	}
}

func TestMetaShardCorruptNewestSuperblockFallsBack(t *testing.T) {
	ms := newTestMetaShard(t, "test-metashard-corrupt-super")
	putMetaValue(t, ms, "k", "v1")
	putMetaValue(t, ms, "k", "v2")

	f, err := opfs.CreateSyncFile(ms.dir, "super-b")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteAt([]byte("corrupt"), 0); err != nil {
		t.Fatal(err)
	}
	f.Flush()
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := reopenTestMetaShard(t, "test-metashard-corrupt-super")
	val, found, err := reopened.Get([]byte("k"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "v1" {
		t.Fatalf("fallback value got found=%v val=%q want v1", found, val)
	}
}

func TestMetaShardMissingPagesFileReturnsReadError(t *testing.T) {
	ms := newTestMetaShard(t, "test-metashard-missing-pages")
	putMetaValue(t, ms, "k", "v1")

	if err := opfs.DeleteFile(ms.dir, "pages.dat"); err != nil {
		t.Fatal(err)
	}

	reopened := reopenTestMetaShard(t, "test-metashard-missing-pages")
	_, _, err := reopened.Get([]byte("k"))
	if err == nil {
		t.Fatal("expected read error")
	}
	if !strings.Contains(err.Error(), "open page file for read") {
		t.Fatalf("expected missing pages.dat read error, got %v", err)
	}
}

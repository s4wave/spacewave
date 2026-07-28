//go:build js

package metashard

import (
	"bytes"
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
	ms, err := NewMetaShard(dir, name, 0, nil)
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
	ms, err := NewMetaShard(dir, name, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	return ms
}

func openSecondTestMetaShard(t *testing.T, name string) *MetaShard {
	t.Helper()
	root, err := opfs.GetRoot()
	if err != nil {
		t.Fatal(err)
	}
	dir, err := opfs.GetDirectory(root, name, false)
	if err != nil {
		t.Fatal(err)
	}
	ms, err := NewMetaShard(dir, name, 0, nil)
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

func assertMetaValue(t *testing.T, ms *MetaShard, key, want string) {
	t.Helper()
	val, found, err := ms.Get([]byte(key))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != want {
		t.Fatalf("%s got found=%v val=%q want %q", key, found, val, want)
	}
}

func TestMetaStoreLargeValue(t *testing.T) {
	ms := newTestMetaShard(t, "test-metastore-large-value")
	store := NewMetaStore(ms)
	ctx := context.Background()
	key := []byte("pack_bloom/aa/test-pack")
	large := bytes.Repeat([]byte("b"), pagestore.DefaultPageSize+2048)

	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, key, large); err != nil {
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

	got, found, err := readTx.Get(ctx, key)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, large) {
		t.Fatalf("Get large: found=%v got %d bytes want %d", found, len(got), len(large))
	}

	reopened := reopenTestMetaShard(t, "test-metastore-large-value")
	got, found, err = reopened.Get(key)
	if err != nil {
		t.Fatal(err)
	}
	if !found || !bytes.Equal(got, large) {
		t.Fatalf("reopened Get large: found=%v got %d bytes want %d", found, len(got), len(large))
	}
}

func TestMetaStoreReadTxDoesNotBlockWrites(t *testing.T) {
	ms := newTestMetaShard(t, "test-metastore-read-tx-does-not-block-writes")
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

	// A read transaction left open must not stall a later write. Holding the
	// shared metadata lock for the transaction lifetime would queue this
	// commit behind a lock nothing releases until Discard.
	readTx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	val, found, err := readTx.Get(ctx, []byte("k"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "v1" {
		t.Fatalf("first read got found=%v val=%q want v1", found, val)
	}

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

	// The commit landed, which is what this test is for. The open transaction
	// already served a read from the previous generation, so it refuses to
	// serve one from this generation rather than mixing the two.
	if _, _, err := readTx.Get(ctx, []byte("k")); !errors.Is(err, ErrGenerationChanged) {
		t.Fatalf("read after commit err = %v, want ErrGenerationChanged", err)
	}

	// A fresh transaction sees the commit.
	nextTx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer nextTx.Discard()
	val, found, err = nextTx.Get(ctx, []byte("k"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "v2" {
		t.Fatalf("fresh read after commit got found=%v val=%q want v2", found, val)
	}
}

func TestMetaStoreReadTxRefusesToStraddleGenerations(t *testing.T) {
	ms := newTestMetaShard(t, "test-metastore-read-tx-straddle")
	store := NewMetaStore(ms)
	ctx := context.Background()

	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Set(ctx, []byte("a"), []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Set(ctx, []byte("b"), []byte("1")); err != nil {
		t.Fatal(err)
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// A caller reading a multi-key object reads "a" from the first generation.
	readTx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	val, _, err := readTx.Get(ctx, []byte("a"))
	if err != nil {
		t.Fatal(err)
	}
	if string(val) != "1" {
		t.Fatalf("a = %q, want 1", val)
	}

	// Another agent rewrites both keys between the two reads.
	second, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := second.Set(ctx, []byte("a"), []byte("2")); err != nil {
		t.Fatal(err)
	}
	if err := second.Set(ctx, []byte("b"), []byte("2")); err != nil {
		t.Fatal(err)
	}
	if err := second.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Serving b=2 here would hand the caller a=1 with b=2, which no generation
	// ever held.
	if _, _, err := readTx.Get(ctx, []byte("b")); !errors.Is(err, ErrGenerationChanged) {
		t.Fatalf("b err = %v, want ErrGenerationChanged", err)
	}
	if err := readTx.ScanPrefix(ctx, nil, func(_, _ []byte) error { return nil }); !errors.Is(err, ErrGenerationChanged) {
		t.Fatalf("scan err = %v, want ErrGenerationChanged", err)
	}
}

func TestMetaShardReadsRevalidateOnlyWhenTheGenerationMoves(t *testing.T) {
	ms := newTestMetaShard(t, "test-metashard-read-revalidation")
	store := NewMetaStore(ms)
	ctx := context.Background()

	writeTx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"k1", "k2", "k3", "k4"} {
		if err := writeTx.Set(ctx, []byte(key), []byte("v")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeTx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Validating a superblock walks the whole tree, so doing it per read makes
	// a run of M point reads cost M tree walks. Nothing has committed between
	// these reads, so the state loaded by the first one is still current.
	readTx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer readTx.Discard()
	for _, key := range []string{"k1", "k2", "k3", "k4"} {
		if _, _, err := readTx.Get(ctx, []byte(key)); err != nil {
			t.Fatal(err)
		}
	}
	before := ms.revalidations

	for _, key := range []string{"k1", "k2", "k3", "k4"} {
		if _, _, err := readTx.Get(ctx, []byte(key)); err != nil {
			t.Fatal(err)
		}
	}
	if after := ms.revalidations; after != before {
		t.Fatalf("reads over an unchanged shard revalidated %d times, want 0", after-before)
	}

	// A commit does have to be picked up.
	nextWrite, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := nextWrite.Set(ctx, []byte("k1"), []byte("v2")); err != nil {
		t.Fatal(err)
	}
	if err := nextWrite.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	freshTx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer freshTx.Discard()
	val, found, err := freshTx.Get(ctx, []byte("k1"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "v2" {
		t.Fatalf("k1 after commit got found=%v val=%q want v2", found, val)
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

func TestMetaShardWriteTxRefreshesStaleSecondInstance(t *testing.T) {
	ms1 := newTestMetaShard(t, "test-metashard-stale-second-instance")
	ms2 := openSecondTestMetaShard(t, "test-metashard-stale-second-instance")

	putMetaValue(t, ms1, "k1", "v1")
	putMetaValue(t, ms2, "k2", "v2")

	reopened := reopenTestMetaShard(t, "test-metashard-stale-second-instance")
	assertMetaValue(t, reopened, "k1", "v1")
	assertMetaValue(t, reopened, "k2", "v2")
}

func TestMetaShardReadRefreshesStaleSecondInstance(t *testing.T) {
	ms1 := newTestMetaShard(t, "test-metashard-stale-second-instance-read")
	ms2 := openSecondTestMetaShard(t, "test-metashard-stale-second-instance-read")

	putMetaValue(t, ms1, "k1", "v1")
	assertMetaValue(t, ms2, "k1", "v1")
}

func TestMetaStoreReadTxRefreshesStaleSecondInstance(t *testing.T) {
	ms1 := newTestMetaShard(t, "test-metastore-stale-second-instance-read")
	ms2 := openSecondTestMetaShard(t, "test-metastore-stale-second-instance-read")
	store := NewMetaStore(ms2)
	ctx := context.Background()

	putMetaValue(t, ms1, "k1", "v1")

	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	val, found, err := tx.Get(ctx, []byte("k1"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "v1" {
		t.Fatalf("read tx got found=%v val=%q want v1", found, val)
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

func TestMetaShardNewestSuperblockWithZeroRootFallsBack(t *testing.T) {
	ms := newTestMetaShard(t, "test-metashard-zero-root")
	putMetaValue(t, ms, "k", "v1")
	putMetaValue(t, ms, "k", "v2")

	var sbBuf [pagestore.SuperblockSize]byte
	if err := readSuper(ms.dir, "super-b", sbBuf[:]); err != nil {
		t.Fatal(err)
	}
	sb, err := pagestore.DecodeSuperblock(sbBuf[:])
	if err != nil {
		t.Fatal(err)
	}
	f, err := opfs.CreateSyncFile(ms.dir, "pages.dat")
	if err != nil {
		t.Fatal(err)
	}
	zeroPage := make([]byte, pagestore.DefaultPageSize)
	if _, err := f.WriteAt(zeroPage, int64(sb.RootPage)*pagestore.DefaultPageSize); err != nil {
		t.Fatal(err)
	}
	f.Flush()
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	reopened := reopenTestMetaShard(t, "test-metashard-zero-root")
	val, found, err := reopened.Get([]byte("k"))
	if err != nil {
		t.Fatal(err)
	}
	if !found || string(val) != "v1" {
		t.Fatalf("fallback value got found=%v val=%q want v1", found, val)
	}
}

func TestMetaShardBothSuperblocksWithZeroRootsResets(t *testing.T) {
	name := "test-metashard-both-zero-roots"
	ms := newTestMetaShard(t, name)
	putMetaValue(t, ms, "k", "v1")
	putMetaValue(t, ms, "k", "v2")
	zeroSuperblockRoots(t, ms)

	reopened := reopenTestMetaShard(t, name)
	_, found, err := reopened.Get([]byte("k"))
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected corrupt metashard reset to drop old metadata")
	}

	putMetaValue(t, reopened, "after-reset", "ok")
	assertMetaValue(t, reopenTestMetaShard(t, name), "after-reset", "ok")
}

// TestMetaShardResetGenerationUsesProcessFloor covers both draws the epoch can
// produce. The first case sets a floor low enough that a random epoch almost
// always clears it, and the second sets the highest epoch there is, so no draw
// can clear it and the floor has to carry the generation itself.
func TestMetaShardResetGenerationUsesProcessFloor(t *testing.T) {
	for _, tc := range []struct {
		name  string
		epoch uint64
	}{
		{name: "ordinary", epoch: 1},
		{name: "highest-epoch", epoch: 0xFFFFFFFF},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ms := newTestMetaShard(t, "test-metashard-reset-generation-floor-"+tc.name)
			putMetaValue(t, ms, "k", "v1")

			before := tc.epoch<<generationEpochShift | 1
			ms.generation = before
			if err := ms.resetCommittedStateLocked(); err != nil {
				t.Fatal(err)
			}

			putMetaValue(t, ms, "after-reset", "ok")
			after := ms.Generation()
			if after <= before {
				t.Fatalf("generation %d after reset did not exceed prior %d", after, before)
			}
		})
	}
}

func TestMetaStoreReadTxRecoversCorruptSnapshot(t *testing.T) {
	name := "test-metastore-read-tx-recovers-corrupt-snapshot"
	ms := newTestMetaShard(t, name)
	putMetaValue(t, ms, "k", "v1")
	putMetaValue(t, ms, "k", "v2")

	store := NewMetaStore(ms)
	tx, err := store.NewTransaction(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	zeroSuperblockRoots(t, ms)

	_, found, err := tx.Get(context.Background(), []byte("k"))
	if err != nil {
		t.Fatal(err)
	}
	if found {
		t.Fatal("expected corrupt read transaction recovery to reset metadata")
	}

	putMetaValue(t, ms, "after-reset", "ok")
	assertMetaValue(t, reopenTestMetaShard(t, name), "after-reset", "ok")
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

func zeroSuperblockRoots(t *testing.T, ms *MetaShard) {
	t.Helper()
	zeroSuperblockRoot(t, ms, "super-a")
	zeroSuperblockRoot(t, ms, "super-b")
}

func zeroSuperblockRoot(t *testing.T, ms *MetaShard, slot string) {
	t.Helper()
	var sbBuf [pagestore.SuperblockSize]byte
	if err := readSuper(ms.dir, slot, sbBuf[:]); err != nil {
		t.Fatal(err)
	}
	sb, err := pagestore.DecodeSuperblock(sbBuf[:])
	if err != nil {
		t.Fatal(err)
	}
	if sb.RootPage == pagestore.InvalidPage {
		return
	}
	f, err := opfs.CreateSyncFile(ms.dir, "pages.dat")
	if err != nil {
		t.Fatal(err)
	}
	zeroPage := make([]byte, pagestore.DefaultPageSize)
	if _, err := f.WriteAt(zeroPage, int64(sb.RootPage)*pagestore.DefaultPageSize); err != nil {
		t.Fatal(err)
	}
	f.Flush()
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}

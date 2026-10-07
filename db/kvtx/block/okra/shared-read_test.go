package kvtx_block_okra

import (
	"bytes"
	"context"
	"errors"
	"strconv"
	"sync"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/tx"
)

func TestTransactionsShareDecodedPages(t *testing.T) {
	// Publish a fixture tree deep enough to span several pages.
	ctx := context.Background()
	store := newOkraTestStore()
	fixture := newOkraFixture(t, ctx, store, 4096)
	rootRef, _ := writeOkraFixture(t, ctx, store, fixture)

	// Share one decoded-block cache scope across every transaction.
	cache := block.NewDecodedBlockCache()
	defer cache.Close()

	// readAll reads every fixture key in a read-only transaction.
	readAll := func() (block.ReadCounterSnapshot, error) {
		// Open a read-only Okra tree on the shared cache.
		readCtx, counter := block.WithReadCounter(ctx)
		btx, cursor := block.NewTransaction(store, nil, rootRef, nil)
		btx.SetDecodedBlockCache(cache)
		btx.SetReadOnly()
		okraTx, err := NewTx(readCtx, cursor, nil, false, nil)
		if err != nil {
			return block.ReadCounterSnapshot{}, err
		}
		defer okraTx.Discard()

		// Read each key and compare it with the fixture value.
		for i, key := range fixture.keys {
			value, found, err := okraTx.Get(readCtx, key)
			if err != nil {
				return block.ReadCounterSnapshot{}, err
			}
			if !found || !bytes.Equal(value, fixture.values[i]) {
				return block.ReadCounterSnapshot{}, errors.New("fixture value mismatch at " + string(key))
			}
		}

		// Require the read-only transaction to reject writes.
		if _, _, err := btx.Write(readCtx, false); !errors.Is(err, tx.ErrNotWrite) {
			return block.ReadCounterSnapshot{}, errors.New("read-only write did not fail with ErrNotWrite")
		}
		return counter.Snapshot(), nil
	}

	// writeKeys rewrites a slice of keys on the same cache and commits. A
	// writer that changed a shared page in place would corrupt later reads of
	// the fixture root.
	writeKeys := func(round int) (block.ReadCounterSnapshot, error) {
		// Open a writable Okra tree on the shared cache.
		writeCtx, counter := block.WithReadCounter(ctx)
		btx, cursor := block.NewTransaction(store, nil, rootRef, nil)
		btx.SetDecodedBlockCache(cache)
		okraTx, err := NewTx(writeCtx, cursor, btx, true, nil)
		if err != nil {
			return block.ReadCounterSnapshot{}, err
		}

		// Overwrite every 64th key, then publish the new tree.
		for i := round; i < len(fixture.keys); i += 64 {
			if err := okraTx.Set(writeCtx, fixture.keys[i], []byte("round-"+strconv.Itoa(round))); err != nil {
				okraTx.Discard()
				return block.ReadCounterSnapshot{}, err
			}
		}
		return counter.Snapshot(), okraTx.Commit(writeCtx)
	}

	// Warm the cache with one read so later readers hit shared pages.
	if _, err := readAll(); err != nil {
		t.Fatal(err)
	}
	cache.Wait()

	// Run concurrent readers beside writers that decode the same pages.
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for round := range 4 {
		wg.Go(func() {
			_, err := readAll()
			errs <- err
		})
		wg.Go(func() {
			_, err := writeKeys(round)
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}

	// Require a warm reader to take shared pages without cloning them. Only
	// the Okra root, which is not shareable, may be cloned.
	snapshot, err := readAll()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.DecodedBlockCacheHitCount < 2 {
		t.Fatalf("decoded-block cache hits = %d, want at least 2", snapshot.DecodedBlockCacheHitCount)
	}
	if snapshot.DecodedBlockCloneCount > 1 {
		t.Fatalf("decoded-block clones = %d, want at most 1", snapshot.DecodedBlockCloneCount)
	}

	// Require a warm writer to take shared pages without cloning them.
	snapshot, err = writeKeys(5)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.DecodedBlockCacheHitCount < 2 {
		t.Fatalf("writer decoded-block cache hits = %d, want at least 2", snapshot.DecodedBlockCacheHitCount)
	}
	if snapshot.DecodedBlockCloneCount > 1 {
		t.Fatalf("writer decoded-block clones = %d, want at most 1", snapshot.DecodedBlockCloneCount)
	}

	// Require the fixture root to read unchanged after every writer.
	if _, err := readAll(); err != nil {
		t.Fatal(err)
	}
}

func TestWriteCopiesSharedPage(t *testing.T) {
	// Publish a fixture tree on one decoded-block cache scope.
	ctx := context.Background()
	store := newOkraTestStore()
	fixture := newOkraFixture(t, ctx, store, 4096)
	rootRef, _ := writeOkraFixture(t, ctx, store, fixture)
	cache := block.NewDecodedBlockCache()
	defer cache.Close()

	// readFirst reads the first fixture key in a read-only transaction.
	readFirst := func() error {
		// Open a read-only Okra tree on the shared cache.
		btx, cursor := block.NewTransaction(store, nil, rootRef, nil)
		btx.SetDecodedBlockCache(cache)
		btx.SetReadOnly()
		okraTx, err := NewTx(ctx, cursor, nil, false, nil)
		if err != nil {
			return err
		}
		defer okraTx.Discard()

		// Compare the value with the fixture.
		value, found, err := okraTx.Get(ctx, fixture.keys[0])
		if err != nil {
			return err
		}
		if !found || !bytes.Equal(value, fixture.values[0]) {
			return errors.New("fixture value mismatch")
		}
		return nil
	}

	// Warm the cache so the writer takes the shared leaf page.
	if err := readFirst(); err != nil {
		t.Fatal(err)
	}
	cache.Wait()

	// Point the first value at another block through its cursor and write.
	btx, cursor := block.NewTransaction(store, nil, rootRef, nil)
	btx.SetDecodedBlockCache(cache)
	okraTx, err := NewTx(ctx, cursor, btx, true, nil)
	if err != nil {
		t.Fatal(err)
	}
	valueCursor, err := okraTx.GetCursorAtKey(ctx, fixture.keys[0])
	if err != nil {
		t.Fatal(err)
	}
	valueCursor.SetRefAtCursor(fixture.refs[1], true)
	if _, _, err := btx.Write(ctx, false); err != nil {
		t.Fatal(err)
	}

	// Require the shared page to keep the fixture value.
	if err := readFirst(); err != nil {
		t.Fatal(err)
	}
}

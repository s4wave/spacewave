package store_kvtx_inmem

import (
	"bytes"
	"context"
	"testing"
	"testing/synctest"

	"github.com/s4wave/spacewave/db/kvtx"
	store_kvkey "github.com/s4wave/spacewave/db/store/kvkey"
	store_kvtx "github.com/s4wave/spacewave/db/store/kvtx"
	kvtx_vlogger "github.com/s4wave/spacewave/db/store/kvtx/vlogger"
	store_test "github.com/s4wave/spacewave/db/store/test"
	"github.com/sirupsen/logrus"
)

// TestKVTxStore tests a key/value transaction store on top of inmem.
func TestKVTxStore(t *testing.T) {
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}
	ktx := store_kvtx.NewKVTx(
		kvkey,
		kvtx_vlogger.NewVLogger(le, NewStore()),
		nil,
	).(*store_kvtx.KVTx)
	if err := store_test.TestAll(ctx, ktx); err != nil {
		t.Fatal(err.Error())
	}
}

func TestWriteIteratorSortsAddedKeys(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	for _, key := range [][]byte{
		[]byte("m"),
		[]byte("a"),
		[]byte("z"),
	} {
		if err := tx.Set(ctx, key, []byte("value")); err != nil {
			t.Fatal(err)
		}
	}

	iter := tx.Iterate(ctx, nil, true, false)
	defer iter.Close()
	if err := iter.Seek(nil); err != nil {
		t.Fatal(err)
	}
	var got [][]byte
	for iter.Valid() {
		got = append(got, bytes.Clone(iter.Key()))
		iter.Next()
	}
	if err := iter.Err(); err != nil && err != kvtx.ErrDiscarded {
		t.Fatal(err)
	}

	want := [][]byte{
		[]byte("a"),
		[]byte("m"),
		[]byte("z"),
	}
	if len(got) != len(want) {
		t.Fatalf("got %d keys, want %d", len(got), len(want))
	}
	for idx := range want {
		if !bytes.Equal(got[idx], want[idx]) {
			t.Fatalf("key[%d] = %q, want %q", idx, got[idx], want[idx])
		}
	}
}

// TestWriteWaiterCancelReleasesReaders proves a write tx cancelled mid-wait
// releases its reader-admission block: subsequent readers and writers proceed.
// A leaked writeWaiting registration would starve them, which synctest detects
// as a bubble deadlock.
func TestWriteWaiterCancelReleasesReaders(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		store := NewStore()

		// Hold a reader so a writer must register as waiting.
		rtx, err := store.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}

		// A writer whose ctx is already cancelled registers writeWaiting in the
		// first lock hold, then exits via ctx.Done on the first select.
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		if _, err := store.NewTransaction(cctx, true); err != context.Canceled {
			t.Fatalf("cancelled write tx: got %v, want context.Canceled", err)
		}

		// The cancelled waiter must have released its block: a new reader is
		// admitted immediately. Under the leak this blocks forever (deadlock).
		rtx2, err := store.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		rtx.Discard()
		rtx2.Discard()

		// A writer must also proceed once all readers are gone.
		wtx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		wtx.Discard()
	})
}

// TestWriteWaiterCancelPreservesPeerBlock proves cancelling one of two waiting
// writers decrements the waiter count by exactly one rather than clearing it:
// the surviving writer keeps its exclusivity and readers stay blocked until it
// finishes. A blind clear would admit a reader while the peer still waits.
func TestWriteWaiterCancelPreservesPeerBlock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		store := NewStore()

		// Hold a reader so both writers register as waiting.
		rtx, err := store.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}

		// Surviving writer: never cancelled, acquires once readers clear.
		survivor := make(chan kvtx.Tx, 1)
		go func() {
			tx, err := store.NewTransaction(ctx, true)
			if err != nil {
				survivor <- nil
				return
			}
			survivor <- tx
		}()

		// Cancelled writer: registers, then is cancelled mid-wait.
		cctx, cancel := context.WithCancel(ctx)
		cancelErr := make(chan error, 1)
		go func() {
			_, err := store.NewTransaction(cctx, true)
			cancelErr <- err
		}()

		// Both writers are now durably blocked: writeWaiting == 2.
		synctest.Wait()

		cancel()
		synctest.Wait()
		if err := <-cancelErr; err != context.Canceled {
			t.Fatalf("cancelled write tx: got %v, want context.Canceled", err)
		}

		// A new reader must stay blocked: the surviving waiter still holds the
		// admission block (writeWaiting == 1). A blind clear would admit it.
		readerTx := make(chan kvtx.Tx, 1)
		go func() {
			tx, err := store.NewTransaction(ctx, false)
			if err != nil {
				readerTx <- nil
				return
			}
			readerTx <- tx
		}()
		synctest.Wait()
		select {
		case <-readerTx:
			t.Fatal("reader admitted while a writer is still waiting")
		default:
		}

		// Release the initial reader; the surviving writer acquires exclusively.
		rtx.Discard()
		synctest.Wait()
		stx := <-survivor
		if stx == nil {
			t.Fatal("surviving writer failed to acquire")
		}

		// The reader stays blocked while the writer holds the store.
		select {
		case <-readerTx:
			t.Fatal("reader admitted while the writer holds the store")
		default:
		}

		// Finishing the writer admits the waiting reader.
		stx.Discard()
		synctest.Wait()
		rtx2 := <-readerTx
		if rtx2 == nil {
			t.Fatal("reader failed to acquire after writer finished")
		}
		rtx2.Discard()
	})
}

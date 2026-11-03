package store_kvtx_inmem

import (
	"bytes"
	"context"
	"strconv"
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
	// Prepare logging and the key store for the in-memory transaction adapter.
	ctx := context.Background()
	log := logrus.New()
	log.SetLevel(logrus.DebugLevel)
	le := logrus.NewEntry(log)
	kvkey, err := store_kvkey.NewKVKey(store_kvkey.DefaultConfig())
	if err != nil {
		t.Fatal(err.Error())
	}

	// Exercise the store contract through the in-memory transaction adapter.
	ktx := store_kvtx.NewKVTx(
		kvkey,
		kvtx_vlogger.NewVLogger(le, NewStore()),
		nil,
	).(*store_kvtx.KVTx)
	if err := store_test.TestAll(ctx, ktx); err != nil {
		t.Fatal(err.Error())
	}
}

// TestWriteIteratorSortsAddedKeys tests that a write transaction iterates its
// added keys in sorted order.
func TestWriteIteratorSortsAddedKeys(t *testing.T) {
	// Open a write transaction for keys inserted out of order.
	ctx := context.Background()
	store := NewStore()
	tx, err := store.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Insert transaction keys in an order different from their sort order.
	for _, key := range [][]byte{
		[]byte("m"),
		[]byte("a"),
		[]byte("z"),
	} {
		if err := tx.Set(ctx, key, []byte("value")); err != nil {
			t.Fatal(err)
		}
	}

	// Collect keys from the sorted transaction iterator.
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

	// Verify that the transaction iterator returned every key in sorted order.
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

// TestIteratorKeepsEntryAcrossCancel tests that a canceled context ends
// iteration at the next move, not under the entry Next reached: a reader that
// saw Next return true reads that entry's key.
func TestIteratorKeepsEntryAcrossCancel(t *testing.T) {
	// Store one key.
	store := NewStore()
	wtx, err := store.NewTransaction(t.Context(), true)
	if err != nil {
		t.Fatal(err)
	}
	if err := wtx.Set(t.Context(), []byte("a"), []byte("value")); err != nil {
		t.Fatal(err)
	}
	if err := wtx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}

	// Iterate it in a read transaction under a context canceled after Next
	// reaches the key.
	ctx, cancel := context.WithCancel(t.Context())
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	iter := tx.Iterate(ctx, nil, true, false)
	defer iter.Close()
	if !iter.Next() {
		t.Fatalf("Next found no key: %v", iter.Err())
	}
	cancel()

	// The entry stays readable, and the next move ends with the cancel.
	if key := iter.Key(); !bytes.Equal(key, []byte("a")) {
		t.Fatalf("key after cancel = %q, want %q", key, "a")
	}
	if iter.Next() || iter.Valid() {
		t.Fatal("iterator moved under a canceled context")
	}
	if err := iter.Err(); err != context.Canceled {
		t.Fatalf("Err = %v, want context.Canceled", err)
	}
}

// TestWriteWaiterCancelReleasesReaders covers cancellation while the writer
// owns its FIFO turn but waits for existing readers to drain.
func TestWriteWaiterCancelReleasesReaders(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Hold a reader so the first writer owns a turn but cannot begin writing.
		ctx := context.Background()
		store := NewStore()
		rtx, err := store.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer rtx.Discard()

		// Cancel the writer only after it is durably waiting on the reader drain.
		cctx, cancel := context.WithCancel(ctx)
		cancelErr := make(chan error, 1)
		go func() {
			_, err := store.NewTransaction(cctx, true)
			cancelErr <- err
		}()
		synctest.Wait()
		if !store.WritersWaiting() {
			t.Fatal("draining writer did not report its registration")
		}
		cancel()
		synctest.Wait()
		if err := <-cancelErr; err != context.Canceled {
			t.Fatalf("cancelled write tx: got %v, want context.Canceled", err)
		}

		// A surviving reader may overlap another reader once the writer cancels.
		if store.WritersWaiting() {
			t.Fatal("cancelled writer kept its registration")
		}
		rtx2, err := store.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		rtx.Discard()
		rtx2.Discard()

		// The cancelled drain waiter also released its FIFO turn for later writers.
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
		// Create the store for competing writer and reader transactions.
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

		// Verify that cancelling one writer releases only its waiting registration.
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

		// Queue a realistic writer fanout in known arrival order behind the holder.
		const peers = 64
		order := make(chan int, peers+1)
		finished := make(chan error, peers)
		for i := range peers {
			go func() {
				// Each peer stages a value, then finishes by Commit or Discard.
				tx, err := store.NewTransaction(ctx, true)
				if err != nil {
					finished <- err
					return
				}
				defer tx.Discard()
				if err := tx.Set(ctx, []byte(strconv.Itoa(i)), []byte("peer")); err != nil {
					finished <- err
					return
				}
				order <- i
				if i%2 == 0 {
					finished <- tx.Commit(ctx)
				} else {
					tx.Discard()
					finished <- nil
				}
			}()
			synctest.Wait()
		}
		if !store.WritersWaiting() {
			t.Fatal("queued writer peers were not visible")
		}

		// The releasing caller must queue its reacquisition behind every peer.
		if err := stx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		stx.Discard()
		last, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		defer last.Discard()
		order <- peers
		if store.WritersWaiting() {
			t.Fatal("admitted peers kept waiting registrations")
		}

		// Readers stay excluded through the final turn, then regain admission.
		select {
		case <-readerTx:
			t.Fatal("reader entered before the queued writer turns finished")
		default:
		}
		last.Discard()
		synctest.Wait()
		for range peers {
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
		}

		// All queued writers precede the reacquirer without relying on elapsed time.
		for want := range peers + 1 {
			if got := <-order; got != want {
				t.Fatalf("writer admission[%d]=%d, want FIFO order", want, got)
			}
		}
		rtx2 := <-readerTx
		if rtx2 == nil {
			t.Fatal("reader failed to acquire after writer finished")
		}

		// The final reader sees only the half of peer writes that committed.
		size, err := rtx2.Size(ctx)
		if err != nil || size != peers/2 {
			t.Fatalf("committed peer keys=%d, error=%v, want %d", size, err, peers/2)
		}
		rtx2.Discard()
	})
}

// TestIteratorSeekReportsCanceled tests that Seek on an iterator whose context
// was canceled reports the cancellation.
func TestIteratorSeekReportsCanceled(t *testing.T) {
	// Open an iterator on a context canceled before the seek.
	ctx, cancel := context.WithCancel(context.Background())
	tx, err := NewStore().NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	iter := tx.Iterate(ctx, nil, true, false)
	defer iter.Close()
	cancel()

	// Verify that Seek returns the cancellation instead of nil.
	if err := iter.Seek(nil); err != context.Canceled {
		t.Fatalf("Seek returned %v, want %v", err, context.Canceled)
	}
}

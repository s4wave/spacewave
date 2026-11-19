package store_kvtx_inmem

import (
	"bytes"
	"context"
	"math/rand/v2"
	"slices"
	"strconv"
	"sync"
	"testing"
	"testing/synctest"

	"github.com/s4wave/spacewave/db/block"
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

// TestWriterAdmission covers the store's admission: a reader never waits for
// a writer and reads the tree published when it opened; a cancelled queued
// writer leaves the queue; foreground writers take the turn in arrival order,
// ahead of a background writer that queued before them.
func TestWriterAdmission(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		// Hold the writer turn with a staged key.
		ctx := context.Background()
		store := NewStore()
		holder, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := holder.Set(ctx, []byte("held"), []byte("v")); err != nil {
			t.Fatal(err)
		}

		// A reader opens at once and does not see the holder's key, before or
		// after the holder commits.
		before, err := store.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer before.Discard()

		// A cancelled queued writer leaves the queue and no turn behind.
		cctx, cancel := context.WithCancel(ctx)
		cancelErr := make(chan error, 1)
		go func() {
			_, err := store.NewTransaction(cctx, true)
			cancelErr <- err
		}()
		synctest.Wait()
		if !store.WritersWaiting() {
			t.Fatal("queued writer was not visible")
		}
		cancel()
		if err := <-cancelErr; err != context.Canceled {
			t.Fatalf("cancelled write tx: got %v, want context.Canceled", err)
		}
		if store.WritersWaiting() {
			t.Fatal("cancelled writer kept its registration")
		}

		// Queue a background writer, then foreground peers in known order.
		const peers = 16
		order := make(chan int, peers+1)
		finished := make(chan error, peers+1)
		queue := func(i int, ctx context.Context) {
			go func() {
				// Each writer stages a value, then finishes by Commit or Discard.
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
					finished <- nil
				}
			}()
			synctest.Wait()
		}
		queue(peers, block.WithBackground(ctx))
		for i := range peers {
			queue(i, ctx)
		}
		if err := holder.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		synctest.Wait()
		for range peers + 1 {
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
		}
		for want := range peers + 1 {
			if got := <-order; got != want {
				t.Fatalf("writer admission[%d]=%d, want foreground FIFO, then background", want, got)
			}
		}

		// The old reader still reads its snapshot; a new one sees the holder's
		// key and the committed half of the peers, and the background writer.
		if _, found, err := before.Get(ctx, []byte("held")); err != nil || found {
			t.Fatalf("old reader saw a later commit: found=%v err=%v", found, err)
		}
		after, err := store.NewTransaction(ctx, false)
		if err != nil {
			t.Fatal(err)
		}
		defer after.Discard()
		size, err := after.Size(ctx)
		if want := uint64(1 + peers/2 + 1); err != nil || size != want {
			t.Fatalf("committed keys=%d, error=%v, want %d", size, err, want)
		}
	})
}

// TestReadsDuringCommits tests that readers and iterators running while a
// writer commits each see one committed tree whole: every key, all at the
// value of the same commit. Run it under -race, which catches a published
// tree a commit still changes.
func TestReadsDuringCommits(t *testing.T) {
	// Seed the keys at commit zero.
	const keys, commits = 64, 200
	ctx := t.Context()
	store := NewStore()
	write := func(gen int) {
		tx, err := store.NewTransaction(ctx, true)
		if err != nil {
			t.Error(err)
			return
		}
		defer tx.Discard()
		for k := range keys {
			if err := tx.Set(ctx, []byte("k"+strconv.Itoa(k)), []byte(strconv.Itoa(gen))); err != nil {
				t.Error(err)
				return
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Error(err)
		}
	}
	write(0)

	// Commit every key again and again while readers read.
	done := make(chan struct{})
	go func() {
		defer close(done)
		for gen := 1; gen <= commits; gen++ {
			write(gen)
		}
	}()
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-done:
					return
				default:
				}
				readSnapshot(t, store, keys)
			}
		})
	}
	wg.Wait()
}

// readSnapshot reads every key of store by Get and by an iterator in one read
// transaction and fails t unless all hold one value.
func readSnapshot(t *testing.T, store *Store, keys int) {
	// Get the first key, then check every other key against it.
	ctx := t.Context()
	tx, err := store.NewTransaction(ctx, false)
	if err != nil {
		t.Error(err)
		return
	}
	defer tx.Discard()
	want, found, err := tx.Get(ctx, []byte("k0"))
	if err != nil || !found {
		t.Errorf("get k0: found=%v err=%v", found, err)
		return
	}
	iter := tx.Iterate(ctx, []byte("k"), false, false)
	defer iter.Close()
	if err := iter.Seek(nil); err != nil {
		t.Error(err)
		return
	}
	n := 0
	for ; iter.Valid(); iter.Next() {
		got, err := iter.Value()
		if err != nil || !bytes.Equal(got, want) {
			t.Errorf("key %q at %q (%v), k0 at %q in one read transaction", iter.Key(), got, err, want)
			return
		}
		n++
	}
	if err := iter.Err(); err != nil || n != keys {
		t.Errorf("iterated %d keys, want %d: %v", n, keys, err)
	}
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

// TestIteratorMergeMatchesModel walks a write transaction's merged view of
// committed, added, replaced and deleted keys, forward and reverse, under
// random prefixes and seeks, and checks it against a sorted model. Writes
// made while an iterator is open stay invisible to it.
func TestIteratorMergeMatchesModel(t *testing.T) {
	ctx := context.Background()
	rng := rand.New(rand.NewPCG(1, 2))
	key := func() []byte { return []byte{byte(rng.IntN(4)), byte(rng.IntN(4)), byte(rng.IntN(4))} }
	for round := range 50 {
		s := NewStore()
		model := make(map[string]string)
		tx, err := s.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for range 30 {
			k := key()
			if err := tx.Set(ctx, k, []byte("c")); err != nil {
				t.Fatal(err)
			}
			model[string(k)] = "c"
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		tx, err = s.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for range 30 {
			k := key()
			if rng.IntN(2) == 0 {
				_ = tx.Delete(ctx, k)
				delete(model, string(k))
			} else {
				_ = tx.Set(ctx, k, []byte("a"))
				model[string(k)] = "a"
			}
		}
		for step := range 20 {
			late := "late" + strconv.Itoa(step)
			prefix := key()[:rng.IntN(3)]
			if rng.IntN(4) == 0 {
				prefix = append(prefix, 0xff)
			}
			reverse := rng.IntN(2) == 0
			var want []string
			for k := range model {
				if bytes.HasPrefix([]byte(k), prefix) {
					want = append(want, k)
				}
			}
			slices.Sort(want)
			if reverse {
				slices.Reverse(want)
			}
			it := tx.Iterate(ctx, prefix, true, reverse)
			from := 0
			if rng.IntN(2) == 0 {
				k := key()
				if err := it.Seek(k); err != nil {
					t.Fatal(err)
				}
				for from < len(want) && (!reverse && want[from] < string(k) || reverse && want[from] > string(k)) {
					from++
				}
			} else if !it.Next() && len(want) != 0 {
				t.Fatalf("round %d: empty iterator, want %d keys", round, len(want))
			}
			// A write while the iterator is open must not show up in it.
			_ = tx.Set(ctx, append(bytes.Clone(prefix), 9), []byte(late))
			model[string(append(bytes.Clone(prefix), 9))] = late
			var got []string
			for ; it.Valid(); it.Next() {
				val, _ := it.Value()
				if string(val) == late {
					t.Fatalf("round %d: iterator saw a write made after it opened", round)
				}
				got = append(got, string(it.Key()))
			}
			it.Close()
			if !slices.Equal(got, want[from:]) {
				t.Fatalf("round %d prefix %x reverse %v: got %x want %x", round, prefix, reverse, got, want[from:])
			}
		}
		tx.Discard()
	}
}

//go:build darwin || linux || windows

package s4db

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"hash/fnv"
	"maps"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/volume/device"
)

// TestCleaner walks the cleaner through a file that fills, loses values in
// random order, and keeps taking writes. Compact empties the sparse value
// chunks, so the value chunks end within an eighth of the live bytes and the
// file shrinks; the moves count in no flow; the space matches the file and
// the values read back after reopening; the background cleaner shrinks the
// value chunks again by itself; and Compact gathers a thinned index spread
// over many chunks into a few.
func TestCleaner(t *testing.T) {
	// Open without the background cleaner.
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "clean.s4wave")
	opts := Options{RelocateBudget: -1, CheckpointMin: 256 << 10, CheckpointMax: 1 << 20, Families: [][]byte{[]byte("k/")}}
	db := openTest(t, path, opts)

	// write sets or deletes keys, and usage reads the file's use.
	r := rand.New(rand.NewPCG(3, 4))
	m := make(model)
	large, small := largeValues(r), smallValues(r, 64)
	write := func(name string, keys []int, val func() []byte) {
		t.Helper()
		setKeys(t, db, m, name, keys, val)
	}
	usage := func() Usage {
		t.Helper()
		u, err := db.Usage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		return u
	}

	// tight reports whether the value chunks hold at most an eighth more
	// than the live values, plus the two streams' open chunks.
	tight := func(u Usage) bool {
		return u.ValueChunkBytes <= u.ValueBytes+u.ValueBytes/8+2*chunkSize
	}

	// Fill 2,000 keys, delete 40% in another order, and rewrite a fifth.
	write("k/%05d", r.Perm(2000), large)
	keys := r.Perm(2000)
	write("k/%05d", keys[:800], nil)
	write("k/%05d", keys[800:1200], large)
	before := usage()
	if tight(before) {
		t.Fatalf("value chunks already tight before cleaning: %+v", before)
	}

	// Compact cleans: the value chunks tighten, the file shrinks to them,
	// the index and the log, and the flows do not change.
	flows := db.Flows()
	if err := db.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	after := usage()
	t.Logf("before %+v, after %+v", before, after)
	if !tight(after) || after.FileBytes > after.ValueChunkBytes+after.IndexChunkBytes+after.LogBytes+2*chunkSize {
		t.Fatalf("compact left value chunks %d for %d live, file %d from %d", after.ValueChunkBytes, after.ValueBytes, after.FileBytes, before.FileBytes)
	}
	if got := db.Flows(); !slices.Equal(got, flows) {
		t.Fatalf("flows after cleaning %v, want %v", got, flows)
	}
	checkSpace(t, db)
	m.check(t, db)

	// Reopened with the background cleaner, the values read back.
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	least := cleanMinSlack
	cleanMinSlack = 0
	defer func() { cleanMinSlack = least }()
	opts.RelocateBudget = 64 << 20
	db = openTest(t, path, opts)
	defer db.Close()
	m.check(t, db)

	// After another round of deletes, with rewrites going on, the cleaner
	// tightens the value chunks alone.
	write("k/%05d", keys[1200:1700], nil)
	rewrite := keys[1700:]
	for deadline := time.Now().Add(20 * time.Second); len(rewrite) != 0 || !tight(usage()); time.Sleep(20 * time.Millisecond) {
		// Keep writing while the cleaner runs.
		if time.Now().After(deadline) {
			t.Fatalf("background cleaner left %+v", usage())
		}
		n := min(32, len(rewrite))
		write("k/%05d", rewrite[:n], large)
		rewrite = rewrite[n:]
	}
	checkSpace(t, db)
	m.check(t, db)

	// An index spread over many chunks and then thinned leaves its pages
	// scattered; Compact gathers them into a few chunks and the file
	// shrinks.
	write("k/i%06d", r.Perm(40000), small)
	write("k/i%06d", r.Perm(40000)[:34000], nil)
	if err := db.Compact(ctx); err != nil {
		t.Fatal(err)
	}
	idx := usage()
	t.Logf("index %+v", idx)
	if idx.IndexChunkBytes > idx.IndexBytes+idx.IndexBytes/8+2*chunkSize {
		t.Fatalf("compact left index chunks %d for %d of pages", idx.IndexChunkBytes, idx.IndexBytes)
	}
	checkSpace(t, db)
	m.check(t, db)
}

// setKeys sets the keys of the form name numbered keys in db and m to values
// from val, or deletes them when val is nil, 64 to a durable commit.
func setKeys(t *testing.T, db *DB, m model, name string, keys []int, val func() []byte) {
	t.Helper()
	ctx := context.Background()
	for len(keys) != 0 {
		// Set or delete the next keys.
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for _, k := range keys[:min(64, len(keys))] {
			key := fmt.Sprintf(name, k)
			if val == nil {
				err = tx.Delete(ctx, []byte(key))
				delete(m, key)
			} else {
				v := val()
				err = tx.Set(ctx, []byte(key), v)
				m[key] = v
			}
			if err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		keys = keys[min(64, len(keys)):]
	}
}

// largeValues returns values of 2 to 30 KiB, stored in value chunks.
func largeValues(r *rand.Rand) func() []byte {
	return func() []byte {
		return bytes.Repeat([]byte{byte(r.Uint32())}, 2<<10+r.IntN(28<<10))
	}
}

// smallValues returns values of n bytes, at most InlineMax, stored in the
// index.
func smallValues(r *rand.Rand, n int) func() []byte {
	return func() []byte {
		return bytes.Repeat([]byte{byte(r.Uint32())}, n)
	}
}

// TestCleanerCrash crashes the device at each call of a Compact that empties
// value and index chunks, loses power, and checks that recovery keeps every
// value, which Compact does not change, and a consistent space that later
// commits and a reopen keep.
func TestCleanerCrash(t *testing.T) {
	ctx := context.Background()
	opts := Options{RelocateBudget: -1, CheckpointMin: 64 << 10, CheckpointMax: 256 << 10}
	for k := 0; ; k++ {
		// Fill a file with value and index chunks to empty, every commit
		// durable.
		r := rand.New(rand.NewPCG(5, 6))
		mem := device.NewMemory()
		db := openDevice(t, mem, opts)
		m := make(model)
		setKeys(t, db, m, "k/%05d", r.Perm(400), largeValues(r))
		setKeys(t, db, m, "k/i%05d", r.Perm(12000), smallValues(r, 400))
		setKeys(t, db, m, "k/%05d", r.Perm(400)[:240], nil)
		setKeys(t, db, m, "k/i%05d", r.Perm(12000)[:10000], nil)
		before, err := db.Usage(ctx)
		if err != nil {
			t.Fatal(err)
		}

		// Crash at Compact's call k, until Compact makes fewer.
		calls := mem.Calls()
		mem.CrashAfter(k)
		err = db.Compact(ctx)
		if err == nil {
			// Uncrashed, Compact leaves the file tight.
			after, err := db.Usage(ctx)
			if err != nil {
				t.Fatal(err)
			}
			t.Logf("compact finished at k %d in %d calls: %+v to %+v", k, mem.Calls()-calls, before, after)
			if after.FreeBytes != 0 || after.FileBytes >= before.FileBytes || after.ValueChunkBytes > after.ValueBytes+after.ValueBytes/8+2*chunkSize {
				t.Fatalf("compact left %+v from %+v", after, before)
			}
		}
		_ = db.Close()
		mem.PowerLoss(r)

		// Recovery keeps every value, and later commits use its space.
		db = openDevice(t, mem, opts)
		if got := contents(t, db); !maps.EqualFunc(got, m, bytes.Equal) {
			t.Fatalf("crash at call %d: recovered %d keys, want %d", k, len(got), len(m))
		}
		checkSpace(t, db)
		for range 20 {
			randomCommit(t, r, db, m, 2000)
		}
		checkSpace(t, db)
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db = openDevice(t, mem, opts)
		m.check(t, db)
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		if err == nil {
			return
		}
	}
}

// TestCleanerRace runs the background cleaner, with no least slack, over a
// file left sparse by deletes, while two writers set and delete their own
// keys and four readers read random keys four times in one snapshot, 100 ms
// apart, across dozens of commits. Every value names its key and checks itself, so a reader that gets a value
// from a chunk freed and reused under its snapshot fails. Once the writers
// stop, the cleaner tightens the value chunks, and the values read back after
// a reopen.
func TestCleanerRace(t *testing.T) {
	// Open with the background cleaner.
	ctx := context.Background()
	least := cleanMinSlack
	cleanMinSlack = 0
	defer func() { cleanMinSlack = least }()
	path := filepath.Join(t.TempDir(), "race.s4wave")
	opts := Options{RelocateBudget: 64 << 20, CheckpointMin: 256 << 10, CheckpointMax: 1 << 20}
	db := openTest(t, path, opts)

	// value returns a value for key that a reader can check alone: the key,
	// the writer's sequence, and 2 to 30 KiB of a byte drawn from both.
	const writers, keys = 2, 1000
	value := func(key string, seq uint64) []byte {
		v := binary.BigEndian.AppendUint64([]byte(key), seq)
		return append(v, bytes.Repeat([]byte{filler(key, seq)}, 2<<10+int(seq*7919%(28<<10)))...)
	}
	check := func(key string, v []byte) error {
		if len(v) < len(key)+8 || string(v[:len(key)]) != key {
			return fmt.Errorf("%s: value names another key", key)
		}
		seq := binary.BigEndian.Uint64(v[len(key):])
		if !bytes.Equal(v, value(key, seq)) {
			return fmt.Errorf("%s: value of sequence %d is damaged", key, seq)
		}
		return nil
	}

	// apply commits the sets and deletes, nil values, in next and records
	// them in m.
	apply := func(m, next model) error {
		// Commit the changes.
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			return err
		}
		defer tx.Discard()
		for key, v := range next {
			if v == nil {
				err = tx.Delete(ctx, []byte(key))
			} else {
				err = tx.Set(ctx, []byte(key), v)
			}
			if err != nil {
				return err
			}
		}
		if err := tx.Commit(ctx); err != nil {
			return err
		}

		// Record them once committed.
		for key, v := range next {
			if v == nil {
				delete(m, key)
			} else {
				m[key] = v
			}
		}
		return nil
	}

	// Fill each writer's keys and delete 60% of them, so most value chunks
	// are sparse when the writers and readers start.
	models := make([]model, writers)
	for w := range writers {
		models[w] = make(model)
		r := rand.New(rand.NewPCG(7, uint64(w)))
		for _, del := range []bool{false, true} {
			order := r.Perm(keys)
			if del {
				order = order[:keys*6/10]
			}
			for len(order) != 0 {
				next := make(model)
				for _, k := range order[:min(64, len(order))] {
					key := fmt.Sprintf("k/w%d/%05d", w, k)
					next[key] = nil
					if !del {
						next[key] = value(key, 0)
					}
				}
				if err := apply(models[w], next); err != nil {
					t.Fatal(err)
				}
				order = order[min(64, len(order)):]
			}
		}
	}

	// Writers set and delete their own keys until stop closes.
	stop, done := make(chan struct{}), make(chan struct{})
	var wg sync.WaitGroup
	for w := range writers {
		wg.Go(func() {
			r := rand.New(rand.NewPCG(9, uint64(w)))
			for seq := uint64(1); ; seq++ {
				select {
				case <-stop:
					return
				default:
				}
				next := make(model)
				for range 16 {
					key := fmt.Sprintf("k/w%d/%05d", w, r.IntN(keys))
					next[key] = nil
					if r.IntN(5) >= 2 {
						next[key] = value(key, seq)
					}
				}
				if err := apply(models[w], next); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}

	// Readers read random keys twice in one snapshot until done closes.
	var rg sync.WaitGroup
	for i := range 4 {
		rg.Go(func() {
			r := rand.New(rand.NewPCG(8, uint64(i)))
			for {
				select {
				case <-done:
					return
				default:
				}
				tx, err := db.NewTransaction(ctx, false)
				if err != nil {
					t.Error(err)
					return
				}
				read := make(map[string][]byte)
				names := make([]string, 32)
				for i := range names {
					names[i] = fmt.Sprintf("k/w%d/%05d", r.IntN(writers), r.IntN(keys))
				}
				for range 4 {
					for _, key := range names {
						v, ok, err := tx.Get(ctx, []byte(key))
						if err == nil && ok {
							err = check(key, v)
						}
						if was, seen := read[key]; err == nil && seen && !bytes.Equal(was, v) {
							err = fmt.Errorf("%s: snapshot changed", key)
						}
						if err != nil {
							t.Error(err)
							tx.Discard()
							return
						}
						read[key] = v
					}
					time.Sleep(100 * time.Millisecond)
				}
				tx.Discard()
			}
		})
	}

	// Write for four seconds, then let the cleaner tighten the value chunks
	// while the readers go on.
	time.Sleep(4 * time.Second)
	close(stop)
	wg.Wait()
	for deadline := time.Now().Add(20 * time.Second); ; time.Sleep(20 * time.Millisecond) {
		u, err := db.Usage(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if u.ValueChunkBytes <= u.ValueBytes+u.ValueBytes/8+2*chunkSize {
			t.Logf("tight: %+v", u)
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("background cleaner left %+v", u)
		}
	}
	close(done)
	rg.Wait()

	// The space matches the file, and every value reads back after a
	// reopen.
	m := make(model)
	for _, w := range models {
		maps.Copy(m, w)
	}
	checkSpace(t, db)
	m.check(t, db)
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	db = openTest(t, path, opts)
	defer db.Close()
	m.check(t, db)
}

// filler returns the filler byte of key's value at seq.
func filler(key string, seq uint64) byte {
	h := fnv.New32a()
	h.Write([]byte(key))
	return byte(h.Sum32() + uint32(seq))
}

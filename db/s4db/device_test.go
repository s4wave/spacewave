package s4db

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"math/rand/v2"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
	kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	"github.com/s4wave/spacewave/db/volume/device"
)

// deviceName is the database file on a test device.
const deviceName = "db.s4wave"

// openDevice opens the database on dev.
func openDevice(t *testing.T, dev device.Device, opts Options) *DB {
	t.Helper()
	db, err := OpenDevice(context.Background(), dev, deviceName, opts)
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// TestDeviceKvtx runs the shared store conformance suite on a device.
func TestDeviceKvtx(t *testing.T) {
	db := openDevice(t, device.NewMemory(), Options{})
	defer db.Close()
	if err := kvtest.TestAll(context.Background(), db); err != nil {
		t.Fatal(err)
	}
}

// TestDevicePowerLoss crashes a device at a random call during random
// commits and flushes, loses power, and checks that recovery keeps every
// durable commit, keeps a prefix of the ordered commits after it, and that
// later commits and a clean reopen keep it all.
func TestDevicePowerLoss(t *testing.T) {
	opts := Options{CacheBytes: 4 << 20, memtable: 96 << 10}
	seeds := uint64(32)
	if testing.Short() {
		seeds = 4
	}
	for seed := range seeds {
		// Commit a history, then crash at a random call during later
		// commits. Recovery may land on any state from the last durable
		// commit to the one in flight.
		r := rand.New(rand.NewPCG(seed, 9))
		mem := device.NewMemory()
		db := openDevice(t, mem, opts)
		m := make(model)
		allowed := []model{maps.Clone(m)}
		arm := 20 + r.IntN(60)
		for i := 0; ; i++ {
			if i == arm {
				mem.CrashAfter(r.IntN(48))
			}
			durable, err := commitRandom(r, db, m, 2000)
			if durable && err == nil {
				allowed = allowed[:0]
			}
			allowed = append(allowed, maps.Clone(m))
			if err != nil {
				break
			}
		}
		_ = db.Close()
		mem.PowerLoss(r)

		// Recover, find the allowed state, and keep committing on it.
		db = openDevice(t, mem, opts)
		got := contents(t, db)
		i := len(allowed) - 1
		for i >= 0 && !maps.EqualFunc(got, allowed[i], bytes.Equal) {
			i--
		}
		if i < 0 {
			t.Fatalf("seed %d: recovered %d keys, matching none of %d allowed states", seed, len(got), len(allowed))
		}
		for range 30 {
			randomCommit(t, r, db, got, 2000)
		}
		got.check(t, db)
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}

		// A clean reopen keeps it all.
		db = openDevice(t, mem, opts)
		got.check(t, db)
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestDeviceChurn overwrites a small key set for long enough that flushes,
// merges and cleaner steps all run, crashes at a random device call, and
// checks that recovery lands on an allowed state and the file keeps working.
func TestDeviceChurn(t *testing.T) {
	// A small budget and a key set the memtable cannot hold keep every
	// background job busy.
	opts := Options{CacheBytes: 4 << 20, memtable: 32 << 10}
	seeds := uint64(8)
	if testing.Short() {
		seeds = 2
	}
	for seed := range seeds {
		// Churn, arming the crash late in the history.
		r := rand.New(rand.NewPCG(seed, 11))
		mem := device.NewMemory()
		db := openDevice(t, mem, opts)
		m := make(model)
		allowed := []model{maps.Clone(m)}
		arm := 300 + r.IntN(600)
		for i := 0; ; i++ {
			if i == arm {
				mem.CrashAfter(r.IntN(48))
			}
			durable, err := commitRandom(r, db, m, 3000)
			if durable && err == nil {
				allowed = allowed[:0]
			}
			allowed = append(allowed, maps.Clone(m))
			if err != nil {
				break
			}
		}
		_ = db.Close()
		mem.PowerLoss(r)

		// Recover to an allowed state, keep going, and reopen cleanly.
		db = openDevice(t, mem, opts)
		got := contents(t, db)
		i := len(allowed) - 1
		for i >= 0 && !maps.EqualFunc(got, allowed[i], bytes.Equal) {
			i--
		}
		if i < 0 {
			t.Fatalf("seed %d: recovered %d keys, matching none of %d allowed states", seed, len(got), len(allowed))
		}
		for range 50 {
			randomCommit(t, r, db, got, 3000)
		}
		got.check(t, db)
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
		db = openDevice(t, mem, opts)
		got.check(t, db)
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// failFlushDevice is a device whose next flushing call fails, once armed,
// without reaching the device, as a drive that reports a write error does.
type failFlushDevice struct {
	*device.Memory
	// armed fails the next call with flush set.
	armed atomic.Bool
}

// errFlushFault is the failure failFlushDevice injects.
var errFlushFault = errors.New("injected flush failure")

// Write fails a flushing call once armed, and otherwise writes to memory.
func (d *failFlushDevice) Write(ctx context.Context, writes []device.Write, flush bool) error {
	if flush && d.armed.Swap(false) {
		return errFlushFault
	}
	return d.Memory.Write(ctx, writes, flush)
}

// putKey commits key with a value derived from it, durably.
func putKey(db *DB, key string) error {
	return putKeyCtx(context.Background(), db, key)
}

// putKeyCtx commits key with a value derived from it, durably, under ctx.
func putKeyCtx(ctx context.Context, db *DB, key string) error {
	tx, err := db.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	if err := tx.Set(ctx, []byte(key), []byte("v-"+key)); err != nil {
		tx.Discard()
		return err
	}
	return tx.Commit(ctx)
}

// hasKey reads whether a new transaction finds key.
func hasKey(db *DB, key string) (bool, error) {
	ctx := context.Background()
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		return false, err
	}
	defer tx.Discard()
	_, found, err := tx.Get(ctx, []byte(key))
	return found, err
}

// TestDeviceFailedFlushStops fails one durable flush and checks that the
// handle then shows the failed commit to no one: every new transaction,
// commit and Sync fails with ErrFlushFailed without a device call, so no
// later flush claims the failed commit durable, and recovery after a power
// loss keeps every durable commit. With fail-stop off, the old behavior returns: the failed commit
// reads as stored though a power loss drops it.
func TestDeviceFailedFlushStops(t *testing.T) {
	ctx := context.Background()
	r := rand.New(rand.NewPCG(7, 7))

	// Fail the flush of a durable commit after one that succeeded.
	mem := device.NewMemory()
	dev := &failFlushDevice{Memory: mem}
	db := openDevice(t, dev, Options{})
	if err := putKey(db, "kept"); err != nil {
		t.Fatal(err)
	}
	dev.armed.Store(true)
	if err := putKey(db, "failed"); !errors.Is(err, errFlushFault) || !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("commit with a failed flush returned %v", err)
	}

	// The handle refuses every new read, write and flush, without calling
	// the device, so no later flush claims the failed commit durable.
	calls := mem.Calls()
	if _, err := hasKey(db, "failed"); !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("read after a failed flush returned %v", err)
	}
	if err := putKey(db, "after"); !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("commit after a failed flush returned %v", err)
	}
	if err := db.Sync(ctx); !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("Sync after a failed flush returned %v", err)
	}
	if got := mem.Calls(); got != calls {
		t.Fatalf("failed handle made %d device calls", got-calls)
	}
	if err := db.Close(); !errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("Close after a failed flush returned %v", err)
	}

	// A power loss keeps the durable commit. Close issued the failed
	// commit's collected writes without a flush, so recovery may keep it or
	// not, and the refused commit is gone.
	mem.PowerLoss(r)
	db = openDevice(t, mem, Options{})
	for key, want := range map[string]bool{"kept": true, "after": false} {
		if got, err := hasKey(db, key); err != nil || got != want {
			t.Fatalf("after the power loss %q found=%v err=%v, want %v", key, got, err, want)
		}
	}
	if err := putKey(db, "reopened"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	// With fail-stop off, the failed commit reads as stored, and a power
	// loss drops it.
	kvtx.FailStopOff = true
	t.Cleanup(func() { kvtx.FailStopOff = false })
	mem = device.NewMemory()
	dev = &failFlushDevice{Memory: mem}
	db = openDevice(t, dev, Options{})
	dev.armed.Store(true)
	if err := putKey(db, "failed"); !errors.Is(err, errFlushFault) || errors.Is(err, kvtx.ErrFlushFailed) {
		t.Fatalf("commit with fail-stop off returned %v", err)
	}
	if got, err := hasKey(db, "failed"); err != nil || !got {
		t.Fatalf("with fail-stop off the failed commit found=%v err=%v", got, err)
	}
	mem.CrashAfter(0)
	_ = db.Close()
	mem.PowerLoss(r)
	db = openDevice(t, mem, Options{})
	defer db.Close()
	if got, err := hasKey(db, "failed"); err != nil || got {
		t.Fatalf("with fail-stop off the power loss kept the failed commit: found=%v err=%v", got, err)
	}
}

// gateFlushStorage is a database file whose next durable flush, once
// armed, waits for the test to release it with an error or nil. It sits
// below the fail-stop wrapper and holds no device lock while it waits, so
// other commits keep appending, as they do during a slow fdatasync.
type gateFlushStorage struct {
	storage
	// armed holds the next durable flush.
	armed atomic.Bool
	// entered receives once the held flush arrives.
	entered chan struct{}
	// release ends the held flush with its error, or nil to run it.
	release chan error
}

// flushDurable holds the flush once armed, and otherwise runs it.
func (s *gateFlushStorage) flushDurable() error {
	if s.armed.Swap(false) {
		s.entered <- struct{}{}
		if err := <-s.release; err != nil {
			return err
		}
	}
	return s.storage.flushDurable()
}

// TestDeviceCancelledCommitWaits cancels a durable commit while it waits on
// another commit's flush, then fails that flush and cuts the power. The
// cancelled commit is published by then, so it must not return before its
// flush ends: a commit that returned an error while its write was visible
// would be read back as stored, as the local provider's settle does, and the
// power cut drops it. Every commit that returned without an error must
// survive, and one that failed must not read back as stored.
func TestDeviceCancelledCommitWaits(t *testing.T) {
	r := rand.New(rand.NewPCG(5, 5))
	mem := device.NewMemory()
	gate := &gateFlushStorage{entered: make(chan struct{}), release: make(chan error)}
	db := openStorage(t, mem, func(s storage) storage {
		gate.storage = s
		return gate
	})
	if err := putKey(db, "kept"); err != nil {
		t.Fatal(err)
	}

	// result is a commit's error and, when it failed, what a read of its
	// key found at once, as a caller that settles a failed write reads.
	type result struct {
		commit, read error
		found        bool
	}
	commitAndRead := func(ctx context.Context, key string, out chan<- result) {
		res := result{commit: putKeyCtx(ctx, db, key)}
		if res.commit != nil {
			res.found, res.read = hasKey(db, key)
		}
		out <- res
	}

	// Hold commit a's flush at the device, once a is published.
	gate.armed.Store(true)
	aDone := make(chan result, 1)
	go commitAndRead(context.Background(), "a", aDone)
	<-gate.entered
	seqA := db.cur.Load().seq

	// Commit b joins a's flush once published, and is cancelled there.
	ctx, cancel := context.WithCancel(context.Background())
	bDone := make(chan result, 1)
	go commitAndRead(ctx, "b", bDone)
	err := db.bcast.Wait(context.Background(), func(_ func(), _ func() <-chan struct{}) (bool, error) {
		return db.cur.Load().seq > seqA, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	cancel()

	// A commit that returns on cancellation does so at once. Give it the
	// chance before the flush ends, when its write still reads as stored.
	results := map[string]result{}
	select {
	case results["b"] = <-bDone:
	case <-time.After(100 * time.Millisecond):
	}

	// Cut the power under the held flush and fail it.
	mem.CrashAfter(0)
	gate.release <- errFlushFault
	results["a"] = <-aDone
	if _, ok := results["b"]; !ok {
		results["b"] = <-bDone
	}
	for key, res := range results {
		if res.commit != nil && res.read == nil && res.found {
			t.Fatalf("commit %s returned %v but reads back as stored", key, res.commit)
		}
	}
	_ = db.Close()

	// Recovery keeps every commit that returned without an error.
	mem.PowerLoss(r)
	db = openDevice(t, mem, Options{})
	defer db.Close()
	for _, key := range []string{"kept", "a", "b"} {
		if res, ok := results[key]; ok && res.commit != nil {
			continue
		}
		if found, err := hasKey(db, key); err != nil || !found {
			t.Fatalf("commit %s returned without an error but after the power cut found=%v err=%v", key, found, err)
		}
	}
}

// recordStorage counts the calls that change or read a database's file,
// below its fail-stop wrapper.
type recordStorage struct {
	storage
	// reads, writes and truncates count ReadAt, WriteAt and truncate calls.
	reads, writes, truncates atomic.Int64
	// flushes counts flushDurable calls, and readsAtFlush the reads made
	// before the first.
	flushes, readsAtFlush atomic.Int64
	// evicts counts evict calls, and readsAtEvict and flushesAtEvict the
	// reads and flushes made before the first.
	evicts, readsAtEvict, flushesAtEvict atomic.Int64
}

// flushDurable counts the flush and the reads made before the first.
func (s *recordStorage) flushDurable() error {
	if s.flushes.Add(1) == 1 {
		s.readsAtFlush.Store(s.reads.Load())
	}
	return s.storage.flushDurable()
}

// ReadAt counts the read and passes it on.
func (s *recordStorage) ReadAt(p []byte, off int64) (int, error) {
	s.reads.Add(1)
	return s.storage.ReadAt(p, off)
}

// WriteAt counts the write and passes it on.
func (s *recordStorage) WriteAt(p []byte, off int64) (int, error) {
	s.writes.Add(1)
	return s.storage.WriteAt(p, off)
}

// truncate counts the truncation and passes it on.
func (s *recordStorage) truncate(n int64) error {
	s.truncates.Add(1)
	return s.storage.truncate(n)
}

// evict counts the eviction and the reads and flushes made before the first.
func (s *recordStorage) evict() error {
	if s.evicts.Add(1) == 1 {
		s.readsAtEvict.Store(s.reads.Load())
		s.flushesAtEvict.Store(s.flushes.Load())
	}
	return s.storage.evict()
}

// openStorage opens the database file of dev through the storage wrap
// returns for it.
func openStorage(t *testing.T, dev device.Device, wrap func(storage) storage) *DB {
	t.Helper()
	h, err := device.OpenHandle(context.Background(), dev, deviceName)
	if err != nil {
		t.Fatal(err)
	}
	db, err := open(wrap(deviceFile{h}), Options{})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

// openRecorded opens the database file of dev through a recordStorage.
func openRecorded(t *testing.T, dev device.Device) (*DB, *recordStorage) {
	t.Helper()
	rec := &recordStorage{}
	db := openStorage(t, dev, func(s storage) storage {
		rec.storage = s
		return rec
	})
	return db, rec
}

// TestFailedHandleWritesNothing checks that a writable open flushes the
// file and then drops its cached pages before recovery reads it, and that a
// handle whose flush failed sends the device nothing more, not even the
// writes it held at the failure when it closes: the next open would read
// them as stored.
func TestFailedHandleWritesNothing(t *testing.T) {
	mem := device.NewMemory()
	dev := &failFlushDevice{Memory: mem}
	db, rec := openRecorded(t, dev)
	if n, before := rec.flushes.Load(), rec.readsAtFlush.Load(); n == 0 || before != 0 {
		t.Fatalf("writable open flushed %d times after %d reads, want a flush before any read", n, before)
	}
	if n, reads, flushes := rec.evicts.Load(), rec.readsAtEvict.Load(), rec.flushesAtEvict.Load(); n != 1 || reads != 0 || flushes != 1 {
		t.Fatalf("writable open evicted %d times after %d reads and %d flushes, want once after the flush and before any read", n, reads, flushes)
	}

	// Fail a durable flush. Its first device call fails, so the handle
	// still holds the commit's writes.
	dev.armed.Store(true)
	if err := putKey(db, "failed"); err == nil {
		t.Fatal("commit with a failed flush returned nil")
	}

	// Nothing reaches the device after it, through close either.
	calls := mem.Calls()
	_ = db.Close()
	if n := mem.Calls() - calls; n != 0 {
		t.Fatalf("failed handle made %d device calls after the failure", n)
	}
}

// TestFailedFlushReopen fails a flush, closes the handle and reopens it, then
// cuts the power under the new handle. A commit the reopened handle reads as
// stored must survive the power cut: a reader takes what it reads after a
// reopen for durable.
func TestFailedFlushReopen(t *testing.T) {
	for seed := range uint64(24) {
		r := rand.New(rand.NewPCG(seed, seed))
		mem := device.NewMemory()
		dev := &failFlushDevice{Memory: mem}
		db := openDevice(t, dev, Options{})
		if err := putKey(db, "kept"); err != nil {
			t.Fatal(err)
		}
		dev.armed.Store(true)
		if err := putKey(db, "failed"); err == nil {
			t.Fatal("commit with a failed flush returned nil")
		}
		_ = db.Close()

		// Reopen, read the failed commit, and cut the power under the
		// open handle.
		db = openDevice(t, mem, Options{})
		read, err := hasKey(db, "failed")
		if err != nil {
			t.Fatal(err)
		}
		mem.CrashAfter(0)
		_ = db.Close()
		mem.PowerLoss(r)

		// What the reopened handle read survives, and so does the
		// durable commit.
		db = openDevice(t, mem, Options{})
		after, err := hasKey(db, "failed")
		if err != nil {
			t.Fatal(err)
		}
		kept, err := hasKey(db, "kept")
		_ = db.Close()
		if err != nil || !kept {
			t.Fatalf("seed %d: durable commit after the power cut found=%v err=%v", seed, kept, err)
		}
		if read && !after {
			t.Fatalf("seed %d: the reopened handle read the failed commit, and the power cut dropped it", seed)
		}
	}
}

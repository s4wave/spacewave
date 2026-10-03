//go:build !js

package engine

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/kvtx"
	"golang.org/x/sync/semaphore"
)

// diskBackend exercises actual durable files while sharing named test locks.
type diskBackend struct {
	// root is an isolated temporary directory.
	root string
	// mtx protects lock creation and I/O counters.
	mtx sync.Mutex
	// locks coordinate every engine opened against this fixture.
	locks map[string]*semaphore.Weighted
	// reads counts backend file reads for bounded-open checks.
	reads int
	// payloadReads counts physical payload-file visits.
	payloadReads int
	// afterPayloadRead injects one concurrent change after immutable bytes are copied.
	afterPayloadRead func()
	// writeGate can pause payload publication while allowing immutable reads.
	writeGate <-chan struct{}
	// writeStarted announces an attempted payload publication.
	writeStarted chan struct{}
	// failAfter injects a failed durable write after this many successful writes.
	failAfter int
	// written counts bytes published by Write.
	written int64
	// files counts files published by Write.
	files int
	// kindBytes counts written bytes by file kind, the name before its first
	// hyphen.
	kindBytes map[string]int64
	// unsynced skips flushes for setup writes whose durability is irrelevant.
	unsynced bool
	// crashAfter crashes the backend after this many further Write and Remove
	// calls succeed; negative never crashes.
	crashAfter int
	// crashed rejects every later Write and Remove, as after a process crash.
	crashed bool
	// mutations counts Write and Remove calls, including rejected ones.
	mutations int
}

// errCrashed reports a Write or Remove after an injected crash.
var errCrashed = errors.New("injected crash")

// newDiskBackend creates an isolated durable fixture.
func newDiskBackend(t *testing.T) *diskBackend {
	t.Helper()
	return &diskBackend{
		root:       t.TempDir(),
		locks:      make(map[string]*semaphore.Weighted),
		failAfter:  -1,
		crashAfter: -1,
		kindBytes:  make(map[string]int64),
	}
}

// crash counts one Write or Remove and reports whether the injected crash
// rejects it, and whether this call is the one the crash interrupted. The
// caller holds mtx.
func (d *diskBackend) crash() (rejected, interrupted bool) {
	// Count backend mutations and enforce the injected crash boundary.
	d.mutations++
	if d.crashed {
		return true, false
	}
	if d.crashAfter == 0 {
		d.crashed = true
		return true, true
	}
	if d.crashAfter > 0 {
		d.crashAfter--
	}
	return false, false
}

// restart ends an injected crash so a new engine can recover the files.
func (d *diskBackend) restart() {
	d.mtx.Lock()
	d.crashed, d.crashAfter = false, -1
	d.mtx.Unlock()
}

// Read reads one immutable file or range from disk.
func (d *diskBackend) Read(ctx context.Context, name string, offset int64, length int) ([]byte, error) {
	// Honor cancellation before reading the durable fixture.
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	// Track backend reads and retain any payload-read injection.
	d.mtx.Lock()
	d.reads++
	var callback func()
	if strings.HasPrefix(name, "pack-") {
		d.payloadReads++
		callback = d.afterPayloadRead
		d.afterPayloadRead = nil
	}
	d.mtx.Unlock()
	if callback != nil {
		defer callback()
	}

	// Read the immutable fixture file or its requested byte range.
	f, err := os.Open(filepath.Join(d.root, name))
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if length == readAll {
		return io.ReadAll(io.LimitReader(f, maxPublicationBytes+1))
	}
	data := make([]byte, length)
	_, err = f.ReadAt(data, offset)
	return data, err
}

// Write flushes a complete file and atomically replaces its directory entry.
func (d *diskBackend) Write(ctx context.Context, name string, data []byte) error {
	// Honor cancellation and wait for gated payload publication.
	if err := ctx.Err(); err != nil {
		return err
	}
	if strings.HasPrefix(name, "pack-") && d.writeGate != nil {
		select {
		case d.writeStarted <- struct{}{}:
		default:
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-d.writeGate:
		}
	}

	// Allocate crash and failure boundaries across concurrent immutable writes.
	// A crash interrupting the first creation of a file leaves an empty entry,
	// as the Backend contract allows.
	d.mtx.Lock()
	if rejected, interrupted := d.crash(); rejected {
		d.mtx.Unlock()
		path := filepath.Join(d.root, name)
		if _, err := os.Stat(path); interrupted && errors.Is(err, os.ErrNotExist) {
			_ = os.WriteFile(path, nil, 0o600)
		}
		return errCrashed
	}
	if d.failAfter == 0 {
		d.mtx.Unlock()
		return errors.New("injected write failure")
	}
	if d.failAfter > 0 {
		d.failAfter--
	}

	// Record the published file bytes and snapshot the durability policy.
	d.written += int64(len(data))
	d.files++
	kind, _, _ := strings.Cut(name, "-")
	d.kindBytes[kind] += int64(len(data))
	unsynced := d.unsynced
	d.mtx.Unlock()

	// Write and optionally flush the replacement file before publication.
	f, err := os.CreateTemp(d.root, "write-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if !unsynced {
		if err := f.Sync(); err != nil {
			_ = f.Close()
			return err
		}
	}

	// Publish the replacement directory entry after closing the file.
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(f.Name(), filepath.Join(d.root, name)); err != nil {
		return err
	}
	if unsynced {
		return nil
	}

	// Flush the containing directory to make the replacement durable.
	dir, err := os.Open(d.root)
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}

// Remove idempotently deletes a fixture file.
func (d *diskBackend) Remove(ctx context.Context, name string) error {
	// Honor cancellation before removing a durable fixture file.
	if err := ctx.Err(); err != nil {
		return err
	}

	// Reject removal after the fixture crash boundary.
	d.mtx.Lock()
	rejected, _ := d.crash()
	d.mtx.Unlock()
	if rejected {
		return errCrashed
	}

	// Delete the fixture entry while treating absence as success.
	err := os.Remove(filepath.Join(d.root, name))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

// Lock acquires a context-aware fair shared or exclusive fixture lock.
func (d *diskBackend) Lock(ctx context.Context, name string, exclusive bool) (func(), error) {
	// Resolve the shared semaphore for the named fixture lock.
	const capacity = 1 << 30
	d.mtx.Lock()
	lock := d.locks[name]
	if lock == nil {
		lock = semaphore.NewWeighted(capacity)
		d.locks[name] = lock
	}
	d.mtx.Unlock()

	// Acquire the requested shared or exclusive lock weight.
	weight := int64(1)
	if exclusive {
		weight = capacity
	}
	if err := lock.Acquire(ctx, weight); err != nil {
		return nil, err
	}
	return func() { lock.Release(weight) }, nil
}

// TestDurableIndexReopen proves splits, overwrites, tombstones, and bounded open.
func TestDurableIndexReopen(t *testing.T) {
	// Open a durable engine for index reopen checks.
	ctx := t.Context()
	d := newDiskBackend(t)
	e, err := Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}

	// Populate enough index records to force branching.
	value := bytes.Repeat([]byte("v"), 8192)
	for batch := range 12 {
		var records []*Record
		for i := range 128 {
			key := []byte(strconv.Itoa(100000 + batch*128 + i))
			records = append(records, &Record{Key: key, Value: value})
		}
		if err := e.Apply(ctx, records); err != nil {
			t.Fatal(err)
		}
	}

	// Overwrite one key repeatedly and tombstone its neighbor.
	for round := range 9 {
		if err := e.Apply(ctx, []*Record{{Key: []byte("100020"), Value: []byte(strconv.Itoa(round))}, {Key: []byte("100021"), Deleted: true}}); err != nil {
			t.Fatal(err)
		}
	}

	// Reopen the index and require bounded startup reads.
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	d.reads = 0
	e, err = Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()
	if d.reads > 8 {
		t.Fatalf("open visited %d files", d.reads)
	}

	// Verify retained values, the latest overwrite, and the tombstone.
	for _, key := range []string{"100000", "100999", "101535"} {
		got, found, _, err := e.Get(ctx, []byte(key))
		if err != nil || !found || !bytes.Equal(got, value) {
			t.Fatalf("get %s: found=%t error=%v", key, found, err)
		}
	}
	got, found, _, err := e.Get(ctx, []byte("100020"))
	if err != nil || !found || string(got) != "8" {
		t.Fatalf("overwrite: %q %t %v", got, found, err)
	}
	if _, found, _, err := e.Get(ctx, []byte("100021")); err != nil || found {
		t.Fatalf("tombstone: %t %v", found, err)
	}
}

// TestPublicationCrashRecovery rejects partial output at every commit boundary.
func TestPublicationCrashRecovery(t *testing.T) {
	// Exercise recovery at each publication failure boundary.
	for boundary := range 6 {
		t.Run(strconv.Itoa(boundary), func(t *testing.T) {
			// Open a fresh durable engine for this failure boundary.
			ctx := t.Context()
			d := newDiskBackend(t)
			e, err := Open(ctx, d)
			if err != nil {
				t.Fatal(err)
			}

			// Interrupt publication and reopen the engine for recovery.
			d.failAfter = boundary
			commitErr := e.Apply(ctx, []*Record{{Key: []byte("key"), Value: []byte("value")}})
			d.failAfter = -1
			_ = e.Close()
			e, err = Open(ctx, d)
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()

			// Verify commit atomicity and require a successful recovery write.
			value, found, _, err := e.Get(ctx, []byte("key"))
			if err != nil || found != (commitErr == nil) || (found && string(value) != "value") {
				t.Fatalf("commit=%v reopened=%q found=%t error=%v", commitErr, value, found, err)
			}
			if err := e.Apply(ctx, []*Record{{Key: []byte("after"), Value: []byte("recovery")}}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

// TestReclamationProtectsReadersAndTerminates exercises quiescent progress.
func TestReclamationProtectsReadersAndTerminates(t *testing.T) {
	// Open a durable engine for reclamation checks.
	ctx := t.Context()
	d := newDiskBackend(t)
	e, err := Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	// Publish successive values to create reclaimable generations.
	for i := range 8 {
		if err := e.Apply(ctx, []*Record{{Key: []byte("key"), Value: bytes.Repeat([]byte{byte(i)}, 100000)}}); err != nil {
			t.Fatal(err)
		}
	}

	// Require a live snapshot to block reclamation.
	s, err := e.snapshot(ctx)
	if err != nil {
		t.Fatal(err)
	}
	timed, cancel := context.WithTimeout(ctx, 10*time.Millisecond)
	defer cancel()
	if _, err := e.Reclaim(timed); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("reclamation bypassed live reader: %v", err)
	}

	// Release the snapshot and require reclamation to finish.
	s.release()
	var count int
	for ; count < 100; count++ {
		progress, err := e.Reclaim(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !progress {
			break
		}
	}
	if count == 100 {
		t.Fatal("quiescent reclamation never completed")
	}

	// Verify reclamation preserves the latest published value.
	value, found, _, err := e.Get(ctx, []byte("key"))
	if err != nil || !found || len(value) != 100000 || value[0] != 7 {
		t.Fatalf("reclaimed current data: found=%t error=%v", found, err)
	}
}

// TestStaleTransactionCannotOverwriteNewGeneration proves validation across instances.
func TestStaleTransactionCannotOverwriteNewGeneration(t *testing.T) {
	// Open the engine that will create a stale transaction.
	ctx := t.Context()
	d := newDiskBackend(t)
	e, err := Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	// Open a competing engine over the same durable files.
	other, err := Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()

	// Create a transaction that observes the key before another publication.
	tx, err := e.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if _, _, err := tx.Get(ctx, []byte("key")); err != nil {
		t.Fatal(err)
	}

	// Publish the competing value and require the stale commit to fail.
	if err := other.Apply(ctx, []*Record{{Key: []byte("key"), Value: []byte("other")}}); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("key"), []byte("stale")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); !errors.Is(err, kvtx.ErrInvalidSnapshot) {
		t.Fatalf("stale commit: %v", err)
	}
}

// TestTransactionCursor preserves ordering, overlays, seeks, and snapshot reads.
func TestTransactionCursor(t *testing.T) {
	// Open a durable engine for cursor ordering checks.
	ctx := t.Context()
	e, err := Open(ctx, newDiskBackend(t))
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	// Populate ordered records across several committed batches.
	for batch := range 5 {
		tx, err := e.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		for j := range 100 {
			key := "p/" + strconv.Itoa(1000+batch*100+j)
			if err := tx.Set(ctx, []byte(key), bytes.Repeat([]byte(key), 1000)); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Create a cursor transaction with deletion and overwrite overlays.
	tx, err := e.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	if err := tx.Delete(ctx, []byte("p/1200")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("p/1250"), []byte("overlay")); err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("p/1250-extra"), []byte("inserted")); err != nil {
		t.Fatal(err)
	}

	// Verify forward and reverse cursors expose ordered overlay values.
	for _, reverse := range []bool{false, true} {
		it := tx.Iterate(ctx, []byte("p/12"), true, reverse)
		var previous []byte
		var count int
		for it.Next() {
			key := it.Key()
			if !bytes.HasPrefix(key, []byte("p/12")) || string(key) == "p/1200" {
				t.Fatalf("invalid cursor key %q", key)
			}
			if previous != nil {
				comparison := bytes.Compare(previous, key)
				if (!reverse && comparison >= 0) || (reverse && comparison <= 0) {
					t.Fatalf("cursor ordering %q %q reverse=%t", previous, key, reverse)
				}
			}
			if string(key) == "p/1250" {
				value, err := it.Value()
				if err != nil || string(value) != "overlay" {
					t.Fatalf("overlay %q %v", value, err)
				}
			}
			previous = bytes.Clone(key)
			count++
		}
		if err := it.Err(); err != nil || count != 100 {
			t.Fatalf("prefix count=%d reverse=%t error=%v", count, reverse, err)
		}
		if err := it.Seek([]byte("p/1250")); err != nil || string(it.Key()) != "p/1250" {
			t.Fatalf("seek: key=%q error=%v", it.Key(), err)
		}
		it.Close()
	}

	// Commit the cursor transaction after validating its seeks.
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Callback writes preserve the scan snapshot and can reuse its protection.
	read, err := e.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer read.Discard()
	written := false
	err = read.ScanPrefixKeys(ctx, []byte("p/"), func([]byte) error {
		if written {
			return nil
		}
		written = true
		return e.Apply(ctx, []*Record{{Key: []byte("new"), Value: []byte("generation")}})
	})
	if err != nil {
		t.Fatalf("scan across publication: %v", err)
	}
}

// TestPackIndexAndQuiescentReclamation separates index work from payload work.
func TestPackIndexAndQuiescentReclamation(t *testing.T) {
	// Open a durable engine for pack index and reclamation checks.
	ctx := t.Context()
	d := newDiskBackend(t)
	e, err := Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	// Build and publish a batch of immutable blocks.
	s := &packStore{engine: e}
	var entries []*block.PutBatchEntry
	for j := range 32 {
		data := bytes.Repeat([]byte{byte(j)}, 8192)
		ref, err := block.BuildBlockRef(data, nil)
		if err != nil {
			t.Fatal(err)
		}
		entries = append(entries, &block.PutBatchEntry{Ref: ref, Data: data})
	}
	if err := s.PutBlockBatch(ctx, entries); err != nil {
		t.Fatal(err)
	}

	// Require existence and stat queries to use the index without payload reads.
	for _, entry := range entries {
		if exists, err := s.GetBlockExists(ctx, entry.Ref); err != nil || !exists {
			t.Fatalf("existence: %t %v", exists, err)
		}
		if stat, err := s.StatBlock(ctx, entry.Ref); err != nil || stat == nil || stat.Size != int64(len(entry.Data)) {
			t.Fatalf("stat: %v %v", stat, err)
		}
	}
	if d.payloadReads != 0 {
		t.Fatalf("existence/stat visited %d payload files", d.payloadReads)
	}

	// Preserve duplicate detection before removing the other blocks.
	if _, existed, err := s.PutBlock(ctx, entries[0].Data, nil); err != nil || !existed {
		t.Fatalf("duplicate: %t %v", existed, err)
	}
	for _, entry := range entries[1:] {
		if err := s.RmBlock(ctx, entry.Ref); err != nil {
			t.Fatal(err)
		}
	}

	// Clean the sparse pack and finish its pending reclamation.
	if progress, err := e.CleanPack(ctx); err != nil || !progress {
		t.Fatalf("pack clean: %t %v", progress, err)
	}
	for j := range 100 {
		progress, err := e.Reclaim(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if !progress {
			break
		}
		if j == 99 {
			t.Fatal("retirement did not finish")
		}
	}

	// Measure surviving pack bytes after quiescent reclamation.
	files, err := os.ReadDir(d.root)
	if err != nil {
		t.Fatal(err)
	}
	var packBytes int64
	for _, file := range files {
		if strings.HasPrefix(file.Name(), "pack-") {
			stat, err := file.Info()
			if err != nil {
				t.Fatal(err)
			}
			packBytes += stat.Size()
		}
	}
	if packBytes > 8300 {
		t.Fatalf("quiescent deleted payload bytes remain: %d", packBytes)
	}

	// Verify the remaining block statistics and payload.
	count, size, err := e.BlockStats(ctx)
	if err != nil || count != 1 || size != 8192 {
		t.Fatalf("stats: %d %d %v", count, size, err)
	}
	data, found, err := s.GetBlock(ctx, entries[0].Ref)
	if err != nil || !found || !bytes.Equal(data, entries[0].Data) {
		t.Fatalf("live payload after cleaning: %t %v", found, err)
	}
}

// TestRelocationDoesNotResurrectDeletedOrReinsertedBlocks tests conditional moves.
func TestRelocationDoesNotResurrectDeletedOrReinsertedBlocks(t *testing.T) {
	// Check relocation with both deletion and reinsertion during pack reading.
	for _, reinsert := range []bool{false, true} {
		t.Run(strconv.FormatBool(reinsert), func(t *testing.T) {
			// Open a durable engine for the relocation race.
			ctx := t.Context()
			d := newDiskBackend(t)
			e, err := Open(ctx, d)
			if err != nil {
				t.Fatal(err)
			}
			defer e.Close()

			// Publish two blocks and retire the first to make the pack sparse.
			s := &packStore{engine: e}
			first, err := block.BuildBlockRef([]byte("first"), nil)
			if err != nil {
				t.Fatal(err)
			}
			second, err := block.BuildBlockRef([]byte("second"), nil)
			if err != nil {
				t.Fatal(err)
			}
			if err := s.PutBlockBatch(ctx, []*block.PutBatchEntry{{Ref: first, Data: []byte("first")}, {Ref: second, Data: []byte("second")}}); err != nil {
				t.Fatal(err)
			}
			if err := s.RmBlock(ctx, first); err != nil {
				t.Fatal(err)
			}

			// Change the second block while cleaning holds its copied payload.
			d.afterPayloadRead = func() {
				if err := s.RmBlock(ctx, second); err != nil {
					t.Fatal(err)
				}
				if reinsert {
					if _, _, err := s.PutBlock(ctx, []byte("second"), nil); err != nil {
						t.Fatal(err)
					}
				}
			}

			// Verify cleaning honors the current block membership.
			if progress, err := e.CleanPack(ctx); err != nil || !progress {
				t.Fatalf("conditional clean: %t %v", progress, err)
			}
			data, found, err := s.GetBlock(ctx, second)
			if err != nil || found != reinsert || (found && string(data) != "second") {
				t.Fatalf("relocated stale extent: %q %t %v", data, found, err)
			}
		})
	}
}

// TestBufferedFencePublishesPrecedingWrites proves local visibility and durability.
func TestBufferedFencePublishesPrecedingWrites(t *testing.T) {
	// Open a durable engine with payload publication held at a gate.
	ctx := t.Context()
	d := newDiskBackend(t)
	gate := make(chan struct{})
	d.writeGate = gate
	d.writeStarted = make(chan struct{}, 1)
	e, err := Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	defer e.Close()

	// Buffer a block and verify local visibility before publication.
	s := NewBlockStore(ctx, e, 0)
	defer s.Close()
	ref, existed, err := s.PutBlock(ctx, []byte("pending content"), nil)
	if err != nil || existed {
		t.Fatalf("admission: %t %v", existed, err)
	}
	if data, found, err := s.GetBlock(ctx, ref); err != nil || !found || string(data) != "pending content" {
		t.Fatalf("read-through: %q %t %v", data, found, err)
	}

	// Wait for publication to reach the fixture gate.
	select {
	case <-d.writeStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("writeback did not start")
	}

	// Start a durability fence and require it to wait for publication.
	fenced := make(chan error, 1)
	go func() {
		ok, err := s.Sync(ctx)
		if err == nil && !ok {
			err = errors.New("missing durability fence")
		}
		fenced <- err
	}()
	select {
	case err := <-fenced:
		t.Fatalf("fence returned before publication: %v", err)
	case <-time.After(10 * time.Millisecond):
	}

	// Resume publication and require the durability fence to finish.
	close(gate)
	if err := <-fenced; err != nil {
		t.Fatal(err)
	}

	// Reopen the durable engine and verify the fenced payload.
	other, err := Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	read := &packStore{engine: other}
	if data, found, err := read.GetBlock(ctx, ref); err != nil || !found || string(data) != "pending content" {
		t.Fatalf("fenced reopen: %q %t %v", data, found, err)
	}

	// Delete and synchronously reinsert the block for the reopened reader.
	if err := s.RmBlock(ctx, ref); err != nil {
		t.Fatal(err)
	}
	if _, existed, err := s.PutBlock(ctx, []byte("pending content"), &block.PutOpts{Sync: true}); err != nil || existed {
		t.Fatalf("reinsertion after deletion: %t %v", existed, err)
	}
	if found, err := read.GetBlockExists(ctx, ref); err != nil || !found {
		t.Fatalf("reinserted durable block: %t %v", found, err)
	}
}

// TestCloseCancelsQueuedPublication proves shutdown joins the writer and recovers.
func TestCloseCancelsQueuedPublication(t *testing.T) {
	// Open a durable engine with publication held at a fixture gate.
	ctx := t.Context()
	d := newDiskBackend(t)
	d.writeGate = make(chan struct{})
	d.writeStarted = make(chan struct{}, 1)
	e, err := Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}

	// Buffer a block and wait for its publication attempt.
	s := NewBlockStore(ctx, e, 0)
	ref, _, err := s.PutBlock(ctx, []byte("canceled pending data"), nil)
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-d.writeStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("writeback did not start")
	}

	// Close the buffered store and reject reads after shutdown.
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.GetBlock(ctx, ref); !errors.Is(err, ErrClosed) {
		t.Fatalf("closed store served pending bytes: %v", err)
	}

	// Reopen the engine and require canceled publication to remain absent.
	if err := e.Close(); err != nil {
		t.Fatal(err)
	}
	other, err := Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	defer other.Close()
	read := &packStore{engine: other}
	if found, err := read.GetBlockExists(ctx, ref); err != nil || found {
		t.Fatalf("canceled publication became durable: %t %v", found, err)
	}
}

// TestMissingCommittedRootsNeverInitializeEmpty proves data-loss detection.
func TestMissingCommittedRootsNeverInitializeEmpty(t *testing.T) {
	// Publish a saved record in a durable engine.
	ctx := t.Context()
	d := newDiskBackend(t)
	e, err := Open(ctx, d)
	if err != nil {
		t.Fatal(err)
	}
	if err := e.Apply(ctx, []*Record{{Key: []byte("saved"), Value: []byte("data")}}); err != nil {
		t.Fatal(err)
	}

	// Remove both committed roots and require corruption detection.
	_ = e.Close()
	if err := d.Remove(ctx, "root-0"); err != nil {
		t.Fatal(err)
	}
	if err := d.Remove(ctx, "root-1"); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(ctx, d); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("lost roots opened as empty: %v", err)
	}
}

// Subscribe leaves this disk fixture on the durable missed-hint path.
func (d *diskBackend) Subscribe() (<-chan struct{}, func()) { return nil, func() {} }

// Notify deliberately drops hints so tests exercise authoritative reads.
func (d *diskBackend) Notify(uint64) {}

// Close owns no fixture-wide resources.
func (d *diskBackend) Close() error { return nil }

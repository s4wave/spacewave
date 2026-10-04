package logindex

import (
	"bytes"
	"context"
	"strconv"
	"testing"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/kvtx"
	kvtx_kvtest "github.com/s4wave/spacewave/db/kvtx/kvtest"
	"github.com/s4wave/spacewave/db/volume/device"
)

// TestKV runs the kvtx conformance tests.
func TestKV(t *testing.T) {
	i, err := Open(t.Context(), device.NewMemory(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()
	if err := kvtx_kvtest.TestAll(t.Context(), i); err != nil {
		t.Fatal(err)
	}
}

// TestCheckpointReopen commits enough to checkpoint several times, reopens,
// and checks every key and that replaced files are gone.
func TestCheckpointReopen(t *testing.T) {
	// Open a foreground checkpointing index on an in-memory device.
	ctx := t.Context()
	d := device.NewMemory()
	opts := Options{CheckpointBytes: 256, Foreground: true}
	i, err := Open(ctx, d, opts)
	if err != nil {
		t.Fatal(err)
	}

	// Commit sets and deletes.
	const n = 200
	for k := range n {
		tx, err := i.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte("key-"+strconv.Itoa(k)), []byte(strconv.Itoa(k))); err != nil {
			t.Fatal(err)
		}
		if k%3 == 0 && k != 0 {
			if err := tx.Delete(ctx, []byte("key-"+strconv.Itoa(k-1))); err != nil {
				t.Fatal(err)
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}

	// Close the index so the final checkpoint is written.
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen and check the keys.
	i, err = Open(ctx, d, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()

	// Verify the manifest generation advanced across checkpoints.
	if i.man.gen < 2 {
		t.Fatalf("manifest generation %d, want at least 2", i.man.gen)
	}

	// Read every key back from the reopened index.
	// Read every key back from the reopened index.
	tx, err := i.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for k := range n {
		_, found, err := tx.Get(ctx, []byte("key-"+strconv.Itoa(k)))
		if err != nil {
			t.Fatal(err)
		}
		deleted := (k+1)%3 == 0 && k+1 < n
		if found == deleted {
			t.Fatalf("key-%d found %v", k, found)
		}
	}

	// Only the manifest, its checkpoint, and its logs remain.
	files, err := d.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if c, ok := parseFile(checkpointPrefix, f.Name); ok && c != i.man.checkpoint {
			t.Fatalf("replaced checkpoint %s remains", f.Name)
		}
		if l, ok := parseFile(logPrefix, f.Name); ok && l < i.man.log {
			t.Fatalf("replaced log %s remains", f.Name)
		}
	}
}

// TestBackgroundCheckpoint commits while background checkpoints run and a
// reader walks snapshots, then reopens and checks every key.
func TestBackgroundCheckpoint(t *testing.T) {
	// Open a background checkpointing index on an in-memory device.
	ctx := t.Context()
	d := device.NewMemory()
	opts := Options{CheckpointBytes: 256}
	i, err := Open(ctx, d, opts)
	if err != nil {
		t.Fatal(err)
	}

	// Walk snapshots until the commits finish; each one is sorted and whole.
	const n = 500
	done := make(chan struct{})
	walked := make(chan error, 1)
	go func() {
		for {
			select {
			case <-done:
				walked <- nil
				return
			default:
			}
			tx, err := i.NewTransaction(ctx, false)
			if err != nil {
				walked <- err
				return
			}
			var prev []byte
			err = tx.ScanPrefixKeys(ctx, nil, func(key []byte) error {
				if bytes.Compare(prev, key) >= 0 {
					return errors.Errorf("key %q after %q", key, prev)
				}
				prev = key
				return nil
			})
			tx.Discard()
			if err != nil {
				walked <- err
				return
			}
		}
	}()

	// Commit one key per transaction.
	for k := range n {
		tx, err := i.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte("key-"+strconv.Itoa(k)), []byte(strconv.Itoa(k))); err != nil {
			t.Fatal(err)
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	close(done)

	// Stop the snapshot walker and wait for it to finish cleanly.
	if err := <-walked; err != nil {
		t.Fatal(err)
	}

	// Close the index so the final checkpoint is written.
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopen and count the keys.
	i, err = Open(ctx, d, opts)
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()

	// Reopen the index and count every committed key.
	// Reopen the index and count every committed key.
	tx, err := i.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	size, err := tx.Size(ctx)
	if err != nil || size != n {
		t.Fatalf("size %d, err %v, want %d", size, err, n)
	}
}

// atomicDevice is an in-memory device that reports atomic flushes and counts
// them.
type atomicDevice struct {
	*device.Memory
	// flushes counts the flushing writes.
	flushes int
}

// Write counts the flush and writes to memory.
func (d *atomicDevice) Write(ctx context.Context, writes []device.Write, flush bool) error {
	if flush {
		d.flushes++
	}
	return d.Memory.Write(ctx, writes, flush)
}

// FlushesAtomically reports atomic flushes.
func (d *atomicDevice) FlushesAtomically() bool {
	return true
}

// TestAtomicFlush checks that a commit after an ordered commit flushes an
// atomic device once, and that both commits survive a reopen.
func TestAtomicFlush(t *testing.T) {
	// Commit one ordered and one durable transaction.
	ctx := t.Context()
	d := &atomicDevice{Memory: device.NewMemory()}
	i, err := Open(ctx, d, Options{})
	if err != nil {
		t.Fatal(err)
	}
	for _, ordered := range []bool{true, false} {
		tx, err := i.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		key := []byte(strconv.FormatBool(ordered))
		if err := tx.Set(ctx, key, key); err != nil {
			t.Fatal(err)
		}
		if ordered {
			err = kvtx.CommitOrdered(ctx, tx)
		} else {
			err = tx.Commit(ctx)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := i.Close(); err != nil {
		t.Fatal(err)
	}
	if d.flushes != 1 {
		t.Fatalf("flushed %d times, want 1", d.flushes)
	}

	// Reopen and read both keys.
	r, err := Open(ctx, d, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	tx, err := r.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()
	for _, key := range []string{"true", "false"} {
		if _, found, err := tx.Get(ctx, []byte(key)); err != nil || !found {
			t.Fatalf("get %s: found %v, err %v", key, found, err)
		}
	}
}

// TestWaitDurable checks that ordered commits defer the device flush to one
// WaitDurable call.
func TestWaitDurable(t *testing.T) {
	// Open the index on a device that counts flushes.
	ctx := t.Context()
	d := &atomicDevice{Memory: device.NewMemory()}
	i, err := Open(ctx, d, Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer i.Close()

	// Commit two ordered transactions without a flush.
	for _, key := range []string{"a", "b"} {
		tx, err := i.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := tx.Set(ctx, []byte(key), []byte(key)); err != nil {
			t.Fatal(err)
		}
		if err := kvtx.CommitOrdered(ctx, tx); err != nil {
			t.Fatal(err)
		}
	}
	if d.flushes != 0 {
		t.Fatalf("ordered commits flushed %d times, want 0", d.flushes)
	}

	// Wait for durability with one flush.
	if err := i.WaitDurable(ctx); err != nil {
		t.Fatal(err)
	}
	if d.flushes != 1 {
		t.Fatalf("flushed %d times, want 1", d.flushes)
	}
}

// _ is a type assertion
var _ device.AtomicFlusher = (*atomicDevice)(nil)

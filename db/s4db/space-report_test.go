//go:build darwin || linux || windows

package s4db

import (
	"bytes"
	"context"
	"fmt"
	"path/filepath"
	"runtime"
	"testing"
)

// TestSpaceReport checks that the report's chunk counts match the usage and
// make up the file, that freed values leave whole free chunks below live ones,
// that the live values end inside the file, and that the handle's own slot
// shows.
func TestSpaceReport(t *testing.T) {
	// Fill a file with free chunks below live values.
	ctx := context.Background()
	opts := Options{RelocateBudget: -1, SingleProcess: true}
	db := openTest(t, filepath.Join(t.TempDir(), "report.s4wave"), opts)
	defer db.Close()
	fillSlack(t, ctx, db)

	// Read the usage and the report of the same state.
	u, err := db.Usage(ctx)
	if err != nil {
		t.Fatal(err)
	}
	r, err := db.SpaceReport(ctx)
	if err != nil {
		t.Fatal(err)
	}

	// The chunks of each kind make up the file and match the usage, and the
	// fill classes count every value and index chunk.
	c := r.Chunks
	if c.Free == 0 || c.Free*chunkSize != u.FreeBytes || c.Value*chunkSize != u.ValueChunkBytes {
		t.Fatalf("chunks %+v, usage %+v", c, u)
	}
	if (c.Free+c.Log+c.Index+c.Value)*chunkSize != u.FileBytes {
		t.Fatalf("chunks %+v do not make up the file of %d bytes", c, u.FileBytes)
	}
	var values, index int64
	for i := range r.ValueFill {
		values, index = values+r.ValueFill[i], index+r.IndexFill[i]
	}
	if values != c.Value || index != c.Index {
		t.Fatalf("fill classes %v %v, chunks %+v", r.ValueFill, r.IndexFill, c)
	}

	// The live values end inside the file.
	if r.ValueEndBytes == 0 || r.ValueEndBytes > u.FileBytes {
		t.Fatalf("ValueEndBytes = %d, file %d", r.ValueEndBytes, u.FileBytes)
	}

	// The handle's own slot is listed as live.
	var own bool
	for _, s := range r.Slots {
		own = own || (s.Own && s.Live && s.Slot == db.slot)
	}
	if !own {
		t.Fatalf("slots %+v lack the handle's own", r.Slots)
	}

	// On Unix the file system reports what it allocates, and only free
	// chunks not punched yet hold any of it.
	if runtime.GOOS == "windows" {
		return
	}
	if r.DiskBytes <= 0 || r.DiskBytes > u.FileBytes {
		t.Fatalf("DiskBytes = %d, file %d", r.DiskBytes, u.FileBytes)
	}
	if r.FreeDiskBytes < 0 || r.FreeDiskBytes > r.UnpunchedBytes {
		t.Fatalf("FreeDiskBytes = %d, unpunched %d", r.FreeDiskBytes, r.UnpunchedBytes)
	}
}

// fillSlack writes two generations of 512 values of 5000 bytes, deletes the
// first, and checkpoints and releases twice, so the chunks of the first
// generation are free below the second's. It returns the remaining contents.
func fillSlack(t *testing.T, ctx context.Context, db *DB) model {
	// Write or delete one generation of 512 keys in batches of 16.
	t.Helper()
	m := make(model)
	put := func(gen int, del bool) {
		for i := 0; i < 512; i += 16 {
			tx, err := db.NewTransaction(ctx, true)
			if err != nil {
				t.Fatal(err)
			}
			for j := i; j < i+16; j++ {
				key := fmt.Sprintf("g%d/%04d", gen, j)
				if del {
					err = tx.Delete(ctx, []byte(key))
					delete(m, key)
				} else {
					val := bytes.Repeat([]byte{byte(gen), byte(j)}, 2500)
					err = tx.Set(ctx, []byte(key), val)
					m[key] = val
				}
				if err != nil {
					tx.Discard()
					t.Fatal(err)
				}
			}
			if err := tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
		}
	}
	put(0, false)
	put(1, false)
	put(0, true)

	// Checkpoint twice, so the deleted values' chunks are released.
	w := db.w
	for range 2 {
		if err := w.lock(ctx); err != nil {
			t.Fatal(err)
		}
		err := w.checkpointDurable(ctx)
		if err == nil {
			err = w.releaseNow(ctx)
		}
		w.unlock(err)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Wait until the background punch returns its held chunks.
	w.punches.Wait()
	return m
}

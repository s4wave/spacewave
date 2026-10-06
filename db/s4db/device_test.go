package s4db

import (
	"bytes"
	"context"
	"maps"
	"math/rand/v2"
	"testing"

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
// commits, checkpoints, and compaction, loses power, and checks that
// recovery keeps every durable commit, keeps a prefix of the ordered
// commits after it, and leaves a consistent space that later commits use.
func TestDevicePowerLoss(t *testing.T) {
	opts := Options{CheckpointMin: 32 << 10, CheckpointMax: 128 << 10}
	seeds := uint64(64)
	if testing.Short() {
		seeds = 4
	}
	for seed := range seeds {
		// Commit a history, then crash at a random call during later
		// commits and a compaction. Recovery may land on any state from the
		// last durable commit to the one in flight.
		r := rand.New(rand.NewPCG(seed, 9))
		mem := device.NewMemory()
		db := openDevice(t, mem, opts)
		m := make(model)
		allowed := []model{maps.Clone(m)}
		arm, compact := 20+r.IntN(60), -1
		for i := 0; ; i++ {
			if i == arm {
				mem.CrashAfter(r.IntN(48))
				compact = i + 10
			}
			if i == compact {
				if err := db.Compact(context.Background()); err != nil {
					break
				}
				continue
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
		checkSpace(t, db)
		for range 30 {
			randomCommit(t, r, db, got, 2000)
		}
		checkSpace(t, db)
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

// contents returns every key and value in db.
func contents(t *testing.T, db *DB) model {
	// Open a read transaction.
	t.Helper()
	ctx := context.Background()
	tx, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Discard()

	// Copy every entry.
	m := make(model)
	if err := tx.ScanPrefix(ctx, nil, func(k, v []byte) error {
		m[string(k)] = bytes.Clone(v)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return m
}

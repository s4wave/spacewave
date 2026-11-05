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

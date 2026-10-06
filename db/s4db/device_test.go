package s4db

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"maps"
	"math/rand/v2"
	"testing"

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

// quotaDevice is a memory device that refuses a write or truncate leaving
// its files larger than limit bytes in total.
type quotaDevice struct {
	*device.Memory
	limit int64
}

// errQuota is the error of a refused write.
var errQuota = errors.New("quota exceeded")

// Write refuses ws when it grows the files past the limit.
func (q *quotaDevice) Write(ctx context.Context, ws []device.Write, flush bool) error {
	grow := make(map[string]int64)
	for _, w := range ws {
		grow[w.Name] = max(grow[w.Name], w.Offset+int64(len(w.Data)))
	}
	if err := q.check(ctx, grow); err != nil {
		return err
	}
	return q.Memory.Write(ctx, ws, flush)
}

// Truncate refuses to extend a file past the limit.
func (q *quotaDevice) Truncate(ctx context.Context, name string, size int64) error {
	if err := q.check(ctx, map[string]int64{name: size}); err != nil {
		return err
	}
	return q.Memory.Truncate(ctx, name, size)
}

// check fails when growing the named files to at least the given sizes
// exceeds the limit.
func (q *quotaDevice) check(ctx context.Context, grow map[string]int64) error {
	// Total the files at their grown sizes, including new ones.
	files, err := q.List(ctx)
	if err != nil {
		return err
	}
	var total int64
	for _, f := range files {
		total += max(f.Size, grow[f.Name])
		delete(grow, f.Name)
	}
	for _, size := range grow {
		total += size
	}
	if total > q.limit {
		return errQuota
	}
	return nil
}

// TestDeviceQuota fills a device past its quota with ordered commits and
// syncs, and checks that every commit reported done stays readable, that
// commits continue once space returns, and that a reopen keeps them all.
func TestDeviceQuota(t *testing.T) {
	ctx := context.Background()
	for limit := int64(1100 << 10); limit < 2600<<10; limit += 60 << 10 {
		// Commit 16 KiB values and sync every fourth until the quota
		// refuses a write.
		q := &quotaDevice{Memory: device.NewMemory(), limit: limit}
		db := openDevice(t, q, Options{})
		m := make(model)
		step := func(i int) error {
			// Set key i in a write transaction.
			k, v := fmt.Sprintf("k/%06d", i), bytes.Repeat([]byte{byte(i)}, 16<<10)
			tx, err := db.NewTransaction(ctx, true)
			if err != nil {
				return err
			}
			defer tx.Discard()
			if err := tx.Set(ctx, []byte(k), v); err != nil {
				return err
			}

			// Commit it ordered, then sync every fourth step.
			if err := tx.(kvtx.OrderedCommitTx).CommitOrdered(ctx); err != nil {
				return err
			}
			m[k] = v
			if i%4 == 3 {
				return db.Sync(ctx)
			}
			return nil
		}
		i := 0
		for ; ; i++ {
			err := step(i)
			if errors.Is(err, errQuota) {
				break
			}
			if err != nil {
				t.Fatalf("limit %d step %d: %v", limit, i, err)
			}
		}

		// The commits reported done read back, and later ones succeed once
		// the quota lifts.
		m.check(t, db)
		q.limit = 1 << 30
		for i++; i%8 != 0; i++ {
			if err := step(i); err != nil {
				t.Fatalf("limit %d step %d after the quota lifted: %v", limit, i, err)
			}
		}
		checkSpace(t, db)
		if err := db.Close(); err != nil {
			t.Fatal(err)
		}

		// A reopen keeps them all.
		db = openDevice(t, q, Options{})
		m.check(t, db)
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

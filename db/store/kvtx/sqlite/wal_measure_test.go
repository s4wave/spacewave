//go:build cgo

package store_kvtx_sqlite

import (
	"context"
	"database/sql"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/kvtx"
)

// TestWALMeasureCopy is scratch: the passive copy rate of a 256 MiB log.
func TestWALMeasureCopy(t *testing.T) {
	tp := newTempDBPath(t, "measure-*.sqlite")
	db, err := sql.Open("sqlite3", tp+"?_journal_mode=WAL&_synchronous=NORMAL")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, q := range []string{"PRAGMA wal_autocheckpoint=0", "CREATE TABLE kv (k BLOB PRIMARY KEY, v BLOB) WITHOUT ROWID"} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	value := make([]byte, 4096)
	for round := range 3 {
		start := time.Now()
		n := 0
		for i := 0; i < 4096; i++ {
			tx, _ := db.Begin()
			for range 16 {
				_, _ = tx.Exec("INSERT OR REPLACE INTO kv VALUES (?, ?)", []byte("key-"+strconv.Itoa(n%20000)), value)
				n++
			}
			_ = tx.Commit()
		}
		wrote := time.Since(start)
		info, _ := os.Stat(tp + "-wal")
		var busy, frames, copied int
		start = time.Now()
		_ = db.QueryRow("PRAGMA wal_checkpoint(PASSIVE)").Scan(&busy, &frames, &copied)
		took := time.Since(start)
		t.Logf("round %d: wrote %d MiB of log in %v (%.0f MiB/s); passive copied %d of %d frames in %v, %.0f frames/s, %.0f MiB/s", round, info.Size()>>20, wrote, float64(info.Size())/(1<<20)/wrote.Seconds(), copied, frames, took, float64(copied)/took.Seconds(), float64(copied)*4096/(1<<20)/took.Seconds())
		_ = db.QueryRow("PRAGMA wal_checkpoint(TRUNCATE)").Scan(&busy, &frames, &copied)
	}
}

// TestWALMeasureRate is scratch: the WAL peak and longest commit for an
// ordered writer throttled to each rate in MiB/s.
func TestWALMeasureRate(t *testing.T) {
	for _, rate := range []int{25, 50, 100} {
		ctx := context.Background()
		tp := newTempDBPath(t, "rate-*.sqlite")
		const bound = 16 << 20
		db, err := OpenWithPragmas(ctx, tp, "test_wal", Pragmas{WALCheckpointBytes: bound})
		if err != nil {
			t.Fatal(err)
		}
		value := make([]byte, 4096)
		var longest time.Duration
		var peak int64
		start := time.Now()
		n := 0
		for k := 0; time.Since(start) < 8*time.Second; k++ {
			if d := time.Until(start.Add(time.Duration(k) * 64 * time.Second / time.Duration(rate*1024))); d > 0 {
				time.Sleep(d)
			}
			began := time.Now()
			tx, _ := db.NewTransaction(ctx, true)
			for range 16 {
				_ = tx.Set(ctx, []byte("key-"+strconv.Itoa(n%20000)), value)
				n++
			}
			if err := tx.Commit(kvtx.WithOrderedCommitContext(ctx)); err != nil {
				t.Fatal(err)
			}
			longest = max(longest, time.Since(began))
			if info, err := os.Stat(tp + "-wal"); err == nil {
				peak = max(peak, info.Size())
			}
		}
		took := time.Since(start)
		t.Logf("rate %d MiB/s: achieved %.0f MiB/s, longest %v, peak %.2fx bound", rate, float64(n)*4096/(1<<20)/took.Seconds(), longest, float64(peak)/bound)
		_ = db.Close()
	}
}

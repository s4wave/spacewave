//go:build !js

package store_kvtx_sqlite

import (
	"context"
	crand "crypto/rand"
	"math/rand/v2"
	"os"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestSQLiteReadBurst measures read transactions while thousands run at once
// on one file, as every client reconnecting after a restart reads. It runs
// when SPACEWAVE_SQLITE_BURST_DB names the file, filled on first use.
func TestSQLiteReadBurst(t *testing.T) {
	path := os.Getenv("SPACEWAVE_SQLITE_BURST_DB")
	if path == "" {
		t.Skip("SPACEWAVE_SQLITE_BURST_DB names no file")
	}
	envInt := func(name string, fallback int) int {
		if value, err := strconv.Atoi(os.Getenv(name)); err == nil {
			return value
		}
		return fallback
	}
	readers := envInt("SPACEWAVE_SQLITE_BURST_READERS", 4000)
	keys := envInt("SPACEWAVE_SQLITE_BURST_KEYS", 600000)
	seconds := envInt("SPACEWAVE_SQLITE_BURST_SECONDS", 15)
	writeEvery := time.Duration(envInt("SPACEWAVE_SQLITE_BURST_WRITE_US", 2000)) * time.Microsecond
	think := time.Duration(envInt("SPACEWAVE_SQLITE_BURST_THINK_US", 20000)) * time.Microsecond

	ctx := context.Background()
	db, err := OpenWithPragmas(ctx, path, "world", Pragmas{MmapSize: 64 << 30})
	if err != nil {
		t.Fatal(err.Error())
	}
	defer db.Close()
	if idle := envInt("SPACEWAVE_SQLITE_BURST_IDLE", 0); idle != 0 {
		db.GetDB().SetMaxIdleConns(idle)
	}
	if open := envInt("SPACEWAVE_SQLITE_BURST_OPEN", 0); open != 0 {
		db.GetDB().SetMaxOpenConns(open)
	}
	key := func(i int) []byte { return []byte("k/" + strconv.Itoa(i)) }

	// Fill the file once.
	read, err := db.NewTransaction(ctx, false)
	if err != nil {
		t.Fatal(err.Error())
	}
	_, filled, err := read.Get(ctx, key(keys-1))
	read.Discard()
	if err != nil {
		t.Fatal(err.Error())
	}
	value := make([]byte, 5<<10)
	for start := 0; !filled && start < keys; start += 5000 {
		tx, err := db.NewTransaction(ctx, true)
		if err != nil {
			t.Fatal(err.Error())
		}
		for i := start; i < min(keys, start+5000); i++ {
			_, _ = crand.Read(value)
			if err := tx.Set(ctx, key(i), value); err != nil {
				t.Fatal(err.Error())
			}
		}
		if err := tx.Commit(ctx); err != nil {
			t.Fatal(err.Error())
		}
	}

	// Commit a few new rows at a steady rate, as the World's writes do.
	stop := make(chan struct{})
	var wg sync.WaitGroup
	var commits atomic.Int64
	wg.Go(func() {
		next := keys
		for {
			select {
			case <-stop:
				return
			case <-time.After(writeEvery):
			}
			tx, err := db.NewTransaction(ctx, true)
			if err != nil {
				t.Error(err.Error())
				return
			}
			for range 4 {
				_, _ = crand.Read(value)
				_ = tx.Set(ctx, key(next), value)
				next++
			}
			if err := tx.Commit(ctx); err != nil {
				t.Error(err.Error())
				return
			}
			commits.Add(1)
		}
	})

	// Run the readers: each reads four rows in one transaction, then
	// thinks a while, so connections come and go past the idle ones.
	var mu sync.Mutex
	var took []time.Duration
	var failed atomic.Int64
	deadline := time.Now().Add(time.Duration(seconds) * time.Second)
	for range readers {
		wg.Go(func() {
			var mine []time.Duration
			for time.Now().Before(deadline) {
				time.Sleep(time.Duration(rand.Int64N(int64(think) + 1)))
				start := time.Now()
				tx, err := db.NewTransaction(ctx, false)
				if err != nil {
					failed.Add(1)
					continue
				}
				for range 4 {
					if _, _, err := tx.Get(ctx, key(rand.IntN(keys))); err != nil {
						failed.Add(1)
						break
					}
				}
				tx.Discard()
				mine = append(mine, time.Since(start))
			}
			mu.Lock()
			took = append(took, mine...)
			mu.Unlock()
		})
	}
	time.Sleep(time.Until(deadline))
	close(stop)
	wg.Wait()

	slices.Sort(took)
	quantile := func(q float64) time.Duration { return took[int(q*float64(len(took)-1))] }
	stats := db.GetDB().Stats()
	t.Logf("readers %d: %d transactions, %d failed, %d commits; p50 %v p90 %v p99 %v p999 %v max %v; open %d, idle closed %d",
		readers, len(took), failed.Load(), commits.Load(),
		quantile(0.5), quantile(0.9), quantile(0.99), quantile(0.999), took[len(took)-1],
		stats.OpenConnections, stats.MaxIdleClosed)
}

//go:build darwin || linux || windows

package s4db

import (
	"context"
	"fmt"
	"math/rand/v2"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"testing"
	"time"
)

// BenchmarkCommitBesideCleaner times durable commits of four 4 KiB values
// on a file whose deletes left the cleaner a backlog below the urgent bound:
// alone, beside the background cleaner, and beside the cleaner while other
// processes keep every core busy; back to back, and paced 50 ms apart so the
// cleaner has quiet spells. It reports the commits' latency percentiles and
// the cleaner's rate, in value chunk bytes it emptied a second.
func BenchmarkCommitBesideCleaner(b *testing.B) {
	template := cleanerBacklog(b)
	for _, bc := range []struct {
		name   string
		budget int64
		burn   bool
		gap    time.Duration
	}{
		{"alone", -1, false, 0},
		{"cleaning", 0, false, 0},
		{"alone-paced", -1, false, 50 * time.Millisecond},
		{"cleaning-paced", 0, false, 50 * time.Millisecond},
		{"alone-paced-loaded", -1, true, 50 * time.Millisecond},
		{"cleaning-paced-loaded", 0, true, 50 * time.Millisecond},
	} {
		b.Run(bc.name, func(b *testing.B) {
			// Open a copy of the backlog, with the cores busy if asked.
			ctx := context.Background()
			if bc.burn {
				defer burnCores(b)()
			}
			db := openBacklog(b, template, bc.budget)
			defer db.Close()
			before, err := db.Usage(ctx)
			if err != nil {
				b.Fatal(err)
			}

			// Commit, timing each commit.
			val := make([]byte, 4<<10)
			lat := make([]time.Duration, 0, b.N)
			start := time.Now()
			b.ResetTimer()
			for i := range b.N {
				t0 := time.Now()
				tx, err := db.NewTransaction(ctx, true)
				if err != nil {
					b.Fatal(err)
				}
				for j := range 4 {
					if err := tx.Set(ctx, fmt.Appendf(nil, "c/%08d/%d", i, j), val); err != nil {
						b.Fatal(err)
					}
				}
				if err := tx.Commit(ctx); err != nil {
					b.Fatal(err)
				}
				lat = append(lat, time.Since(t0))
				time.Sleep(bc.gap)
			}
			b.StopTimer()

			// Report the percentiles and the cleaner's rate, counting the
			// value chunks the commits added.
			elapsed := time.Since(start)
			after, err := db.Usage(ctx)
			if err != nil {
				b.Fatal(err)
			}

			// Report.
			slices.Sort(lat)
			b.ReportMetric(float64(lat[len(lat)/2].Microseconds()), "p50-us")
			b.ReportMetric(float64(lat[len(lat)*99/100].Microseconds()), "p99-us")
			added := after.ValueBytes - before.ValueBytes
			emptied := before.ValueChunkBytes + added - after.ValueChunkBytes
			b.ReportMetric(float64(emptied)/(1<<20)/elapsed.Seconds(), "cleaned-MiB/s")
		})
	}
}

// BenchmarkCleanerIdle measures the cleaner's rate, in value chunk bytes it
// empties a second, with no foreground commits: on an idle host, and with
// other processes keeping every core busy. Each iteration is a second.
func BenchmarkCleanerIdle(b *testing.B) {
	template := cleanerBacklog(b)
	for _, bc := range []struct {
		name string
		burn bool
	}{{"idle", false}, {"loaded", true}} {
		b.Run(bc.name, func(b *testing.B) {
			// Open a copy of the backlog, with the cores busy if asked.
			ctx := context.Background()
			if bc.burn {
				defer burnCores(b)()
			}
			db := openBacklog(b, template, 0)
			defer db.Close()
			before, err := db.Usage(ctx)
			if err != nil {
				b.Fatal(err)
			}

			// Let the cleaner run, and report what it emptied.
			start := time.Now()
			time.Sleep(time.Duration(b.N) * time.Second)
			elapsed := time.Since(start)
			after, err := db.Usage(ctx)
			if err != nil {
				b.Fatal(err)
			}
			emptied := before.ValueChunkBytes - after.ValueChunkBytes
			b.ReportMetric(float64(emptied)/(1<<20)/elapsed.Seconds(), "cleaned-MiB/s")
		})
	}
}

// openBacklog opens a copy of the backlog at template with the cleaner's
// budget.
func openBacklog(b *testing.B, template string, budget int64) *DB {
	// Copy the backlog.
	b.Helper()
	path := filepath.Join(b.TempDir(), "bench.s4wave")
	data, err := os.ReadFile(template)
	if err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		b.Fatal(err)
	}

	// Open it.
	db, err := Open(path, Options{RelocateBudget: budget})
	if err != nil {
		b.Fatal(err)
	}
	return db
}

// cleanerBacklog writes a file of 3,000 values of 16 KiB and deletes 30% of
// them in random order, and returns its path.
func cleanerBacklog(b *testing.B) string {
	// Open the file without a cleaner.
	b.Helper()
	ctx := context.Background()
	path := filepath.Join(b.TempDir(), "backlog.s4wave")
	db, err := Open(path, Options{RelocateBudget: -1})
	if err != nil {
		b.Fatal(err)
	}

	// Fill it in ordered commits, then delete.
	r := rand.New(rand.NewPCG(1, 2))
	val := make([]byte, 16<<10)
	for _, del := range []bool{false, true} {
		keys := r.Perm(3000)
		if del {
			keys = keys[:900]
		}
		for len(keys) != 0 {
			tx, err := db.NewTransaction(ctx, true)
			if err != nil {
				b.Fatal(err)
			}
			for _, k := range keys[:min(64, len(keys))] {
				key := fmt.Appendf(nil, "k/%05d", k)
				if del {
					err = tx.Delete(ctx, key)
				} else {
					err = tx.Set(ctx, key, val)
				}
				if err != nil {
					b.Fatal(err)
				}
			}
			if err := tx.(*Tx).CommitOrdered(ctx); err != nil {
				b.Fatal(err)
			}
			keys = keys[min(64, len(keys)):]
		}
	}
	if err := db.Close(); err != nil {
		b.Fatal(err)
	}
	return path
}

// burnCores starts a busy child process per core and returns the function
// that kills them.
func burnCores(b *testing.B) func() {
	b.Helper()
	var cmds []*exec.Cmd
	for range runtime.NumCPU() {
		cmd := exec.Command(os.Args[0], "-test.run=^$")
		cmd.Env = append(os.Environ(), burnEnv+"=1")
		if err := cmd.Start(); err != nil {
			b.Fatal(err)
		}
		cmds = append(cmds, cmd)
	}
	return func() {
		for _, cmd := range cmds {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
		}
	}
}

//go:build !js

package paylog

import (
	"context"
	"math/rand/v2"
	"testing"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/volume/crashtest"
	"github.com/s4wave/spacewave/db/volume/device"
	"github.com/s4wave/spacewave/db/volume/logindex"
)

// crashEngines are the engines the crash test runs on. The log engine
// checkpoints often and in the foreground, so its device calls keep one order
// and the crash points cover its checkpoints.
var crashEngines = []engine{s4dbEngine, logEngine(logindex.Options{CheckpointBytes: 128, Foreground: true})}

// TestCrashRecovery runs the crash workload on the memory device with every
// crash engine.
func TestCrashRecovery(t *testing.T) {
	for _, e := range crashEngines {
		t.Run(e.name, func(t *testing.T) {
			crashtest.Run(t, crashtest.DurablePutBlocks, device.NewMemory, func(ctx context.Context, d *device.Memory) (crashtest.Target, error) {
				return e.openStore(ctx, d)
			})
		})
	}
}

// TestCrashMaintenance crashes segment cleaning at each mutating device call
// and checks that the recovered store keeps its blocks and statistics and
// finishes the cleaning.
func TestCrashMaintenance(t *testing.T) {
	for _, e := range crashEngines {
		t.Run(e.name, func(t *testing.T) {
			want := []string{"first block", "", "third block", ""}
			calls := 0
			for n := -1; n < calls; n++ {
				// Store four blocks and remove two after a reopen.
				ctx := t.Context()
				d := device.NewMemory()
				s := reopen(t, e, d, nil)
				refs := putBlocks(t, s, "first block", "other block", "third block", "forth block")
				s = reopen(t, e, d, s)
				for _, ref := range []*block.BlockRef{refs[1], refs[3]} {
					if err := s.RmBlock(ctx, ref); err != nil {
						t.Fatal(err)
					}
				}
				if _, err := s.Sync(ctx); err != nil {
					t.Fatal(err)
				}

				// Count the calls of an uncrashed cleaning on the first pass.
				if n < 0 {
					base := d.Calls()
					maintain(t, s)
					calls = d.Calls() - base
					_ = s.Close()
					continue
				}

				// Crash the cleaning, recover, and check the store.
				d.CrashAfter(n)
				if err := maintainErr(ctx, s); err == nil {
					t.Fatalf("crash %d: cleaning finished without crashing", n)
				}
				_ = s.Close()
				d.PowerLoss(rand.New(rand.NewPCG(uint64(n), 0))) //nolint:gosec
				s = reopen(t, e, d, nil)
				checkBlocks(t, s, refs, want...)
				checkStats(t, s, 2, 22)

				// Finish the cleaning.
				maintain(t, s)
				checkBlocks(t, s, refs, want...)
				files, err := d.List(ctx)
				if err != nil {
					t.Fatal(err)
				}
				for _, f := range files {
					if f.Name == "seg-1" {
						t.Fatalf("crash %d: segment 1 left after cleaning", n)
					}
				}
				_ = s.Close()
			}
			t.Logf("%d crash points", calls)
		})
	}
}

// maintain runs the two maintenance passes that empty and remove a segment.
func maintain(t *testing.T, s *Store) {
	t.Helper()
	if err := maintainErr(t.Context(), s); err != nil {
		t.Fatal(err)
	}
}

// maintainErr runs the two maintenance passes that empty and remove a
// segment.
func maintainErr(ctx context.Context, s *Store) error {
	if err := s.Maintenance(ctx); err != nil {
		return err
	}
	return s.Maintenance(ctx)
}

// TestCrashCollect crashes a Volume collection in the store's domain at each
// mutating device call and checks that the recovered Volume keeps the owned
// block and a second collection removes the orphan.
func TestCrashCollect(t *testing.T) {
	for _, e := range crashEngines {
		t.Run(e.name, func(t *testing.T) {
			calls := 0
			for n := -1; n < calls; n++ {
				// Publish two blocks, orphan the second, and sync.
				ctx := t.Context()
				d := device.NewMemory()
				s := reopen(t, e, d, nil)
				v := openVolume(t, s)
				keep, drop := publish(t, v, "keep"), publish(t, v, "drop")
				orphan(t, v, "drop", drop)
				if _, err := s.Sync(ctx); err != nil {
					t.Fatal(err)
				}

				// Count the calls of an uncrashed collection on the first pass.
				if n < 0 {
					base := d.Calls()
					if _, err := v.Collect(ctx); err != nil {
						t.Fatal(err)
					}
					calls = d.Calls() - base
					_ = v.Close()
					_ = s.Close()
					continue
				}

				// Crash the collection, recover, and collect again.
				d.CrashAfter(n)
				if _, err := v.Collect(ctx); err == nil {
					t.Fatalf("crash %d: collection finished without crashing", n)
				}
				_ = v.Close()
				_ = s.Close()
				d.PowerLoss(rand.New(rand.NewPCG(uint64(n), 1))) //nolint:gosec
				s = reopen(t, e, d, nil)
				v = openVolume(t, s)
				if _, err := v.Collect(ctx); err != nil {
					t.Fatalf("crash %d: %v", n, err)
				}
				checkCollected(t, s, v, keep, drop)
				_ = v.Close()
				_ = s.Close()
			}
			t.Logf("%d crash points", calls)
		})
	}
}

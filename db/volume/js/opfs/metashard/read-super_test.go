//go:build js

package metashard

import (
	"syscall/js"
	"testing"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/opfs"
)

// superReadDriver gates each slot independently to exercise overlapping reads.
type superReadDriver struct {
	// Driver supplies operations outside the two superblock reads.
	opfs.Driver
	// started transfers each read's slot name to the test.
	started chan string
	// results supplies the independently controlled result for each slot.
	results map[string]chan error
}

// ReadFile waits for the test to supply this slot's result.
func (d *superReadDriver) ReadFile(_ js.Value, name string) ([]byte, error) {
	d.started <- name
	return nil, <-d.results[name]
}

// TestReloadSuperblocksConcurrent verifies overlap and deterministic error precedence.
func TestReloadSuperblocksConcurrent(t *testing.T) {
	aErr := errors.New("slot a failed")
	bErr := errors.New("slot b failed")
	for _, tc := range []struct {
		name string
		aErr error
		bErr error
		want error
	}{
		{name: "both valid"},
		{name: "a fails", aErr: aErr, want: aErr},
		{name: "b fails", bErr: bErr, want: bErr},
		{name: "both fail", aErr: aErr, bErr: bErr, want: aErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// Hold both reads until their independent requests have arrived.
			driver := &superReadDriver{
				Driver:  opfs.DefaultDriver,
				started: make(chan string, 2),
				results: map[string]chan error{
					"super-a": make(chan error, 1),
					"super-b": make(chan error, 1),
				},
			}
			opfs.DefaultDriver = driver
			done := make(chan error, 1)
			completed := false
			t.Cleanup(func() {
				// Release blocked reads before restoring the process-wide driver.
				for _, result := range driver.results {
					select {
					case result <- aErr:
					default:
					}
				}
				if !completed {
					select {
					case <-done:
					case <-time.After(5 * time.Second):
						t.Error("superblock reads did not finish during cleanup")
					}
				}
				opfs.DefaultDriver = driver.Driver
			})
			shard := &MetaShard{stateLoaded: true}
			go func() { done <- shard.reloadCommittedStateLocked(false) }()

			// Serial reads cannot reach both gates without a result for the first.
			seen := make(map[string]bool, 2)
			for range 2 {
				select {
				case name := <-driver.started:
					seen[name] = true
				case <-time.After(5 * time.Second):
					t.Fatal("superblock reads did not overlap")
				}
			}
			if !seen["super-a"] || !seen["super-b"] {
				t.Fatalf("read slots = %v", seen)
			}

			// Deliver B first; failures must still follow the on-disk slot order.
			driver.results["super-b"] <- tc.bErr
			driver.results["super-a"] <- tc.aErr
			select {
			case err := <-done:
				completed = true
				if !errors.Is(err, tc.want) {
					t.Fatalf("reload error = %v, want %v", err, tc.want)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("superblock reads did not finish")
			}
		})
	}
}

package execution_controller

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	forge_execution "github.com/s4wave/spacewave/forge/execution"
	execution_tx "github.com/s4wave/spacewave/forge/execution/tx"
)

// TestLeaseSelfFences bounds execution by the last committed lease even when a
// renewal blocks, fails, or discovers that another claimant owns the execution.
func TestLeaseSelfFences(t *testing.T) {
	for _, tc := range []struct {
		name     string
		elapsed  time.Duration
		attempts int32
	}{
		{"transient", forge_execution.DefaultClaimLease - forge_execution.ClaimClockSkew, 2},
		{"blocked", forge_execution.DefaultClaimLease - forge_execution.ClaimClockSkew, 1},
		{"stale", forge_execution.DefaultClaimLease / 3, 1},
		{"reconstructed", 0, 1},
		{"renewed", forge_execution.DefaultClaimLease*4/3 - forge_execution.ClaimClockSkew, 3},
		{"completion", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				// Commit only the first renewal in the extension case.
				start := time.Now()
				var attempts atomic.Int32
				expiry := start.Add(forge_execution.DefaultClaimLease)
				if tc.name == "reconstructed" {
					expiry = start.Add(forge_execution.DefaultClaimLease / 3)
				}
				lease := NewLease(nil, expiry, forge_execution.DefaultClaimLease, func(ctx context.Context, _ time.Time) error {
					attempt := attempts.Add(1)
					switch tc.name {
					case "blocked":
						<-ctx.Done()
						return ctx.Err()
					case "stale", "reconstructed":
						return &execution_tx.StaleClaimEpochError{}
					case "renewed":
						if attempt == 1 {
							return nil
						}
					}
					return errors.New("renewal unavailable")
				})

				// Settlement fencing is terminal without waiting for another renewal.
				err := lease.Execute(t.Context(), func(ctx context.Context) error {
					if tc.name == "completion" {
						return &execution_tx.StaleClaimEpochError{}
					}
					<-ctx.Done()
					return ctx.Err()
				})
				if err != nil {
					t.Fatal(err)
				}

				// Fake time records the actual cancellation bound and retry count.
				if elapsed := time.Since(start); elapsed != tc.elapsed {
					t.Fatalf("execution lasted %s, want %s", elapsed, tc.elapsed)
				}
				if got := attempts.Load(); got != tc.attempts {
					t.Fatalf("renewed %d times, want %d", got, tc.attempts)
				}
			})
		})
	}
}

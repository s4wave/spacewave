package forge_runtime

import (
	"testing"
	"time"

	"github.com/pkg/errors"
)

func TestReserveRejectsExpiredUnsweptIdempotentReturn(t *testing.T) {
	ctx, eng, _ := newTestbed(t)
	admission := NewWorldRuntimeAdmission(eng, newTestStopper(), time.Minute)
	if _, err := admission.ObserveWorker(ctx, "worker/a", 2_000, 4<<30, []string{"docker"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	admission.SetTimeNow(func() time.Time { return now })
	if _, err := admission.Reserve(ctx, "worker/a", "exec/late", testRequest); err != nil {
		t.Fatal(err)
	}

	// Advance past the lease without running the sweep: the idempotent return
	// must reject instead of re-arming a dead lease.
	admission.SetTimeNow(func() time.Time { return now.Add(2 * time.Minute) })
	if _, err := admission.Reserve(ctx, "worker/a", "exec/late", testRequest); !errors.Is(err, ErrReservationExpired) {
		t.Fatalf("expected expired reservation error, got %v", err)
	}

	// The sweep releases once; the attempt stays terminal for its key.
	receipts, err := admission.ExpireLeases(ctx, now.Add(2*time.Minute))
	if err != nil || len(receipts) != 1 {
		t.Fatalf("expected one expiry receipt: %+v err=%v", receipts, err)
	}
	if _, err := admission.Reserve(ctx, "worker/a", "exec/late", testRequest); !errors.Is(err, ErrReservationTerminal) {
		t.Fatalf("expected terminal error after sweep, got %v", err)
	}
}

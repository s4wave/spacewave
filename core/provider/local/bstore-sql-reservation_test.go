package provider_local

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// TestSQLPublishReservations exercises the real publisher's bounded claims,
// canceled waits, one-use admission and cleanup before and after consumption.
func TestSQLPublishReservations(t *testing.T) {
	// Fill the count bound with claims, without admitting any publication.
	p, _, _ := newSQLPublishTest(t)
	var claims []*sqlReservation
	defer func() {
		for _, claim := range claims {
			claim.Release()
		}
	}()
	for range sqlPublishMaxPending {
		claim, err := p.reserve(t.Context(), 0)
		if err != nil {
			t.Fatal(err)
		}
		claims = append(claims, claim)
	}
	waitCanceled := func(size int64) {
		// An actual reservation wait must end with the caller's deadline.
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Millisecond)
		defer cancel()
		claim, err := p.reserve(ctx, size)
		if claim != nil || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatalf("full queue reservation = %v, %v", claim, err)
		}
	}
	waitCanceled(0)

	// Consume exactly one returned slot through the real head writer.
	claims[0].Release()
	claim, err := p.reserve(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	pub, _ := sqlPublicationFor(t, 1, nil, nil)
	ctx := block.WithPublicationReservation(t.Context(), claim)
	receipt, err := p.submit(ctx, pub)
	if err != nil {
		t.Fatal(err)
	}
	claim.Release()
	if err := receipt.Wait(t.Context()); err != nil {
		t.Fatal(err)
	}
	receipt.Release()

	// A completed publication cannot reuse its claim or retain unused slots.
	if _, err := p.submit(ctx, pub); err == nil {
		t.Fatal("reused a consumed reservation")
	}
	for _, held := range claims {
		held.Release()
	}

	// A byte claim holds the byte bound even though it holds only one slot.
	large, err := p.reserve(t.Context(), sqlPublishMaxPendingBytes)
	if err != nil {
		t.Fatal(err)
	}
	waitCanceled(1)
	large.Release()

	// Cancellation after acquiring capacity cannot admit a head or leak its claim.
	claim, err = p.reserve(t.Context(), 0)
	if err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := p.submit(block.WithPublicationReservation(canceled, claim), pub); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled admission = %v", err)
	}
	claim.Release()
	p.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if p.reserved != 0 || p.reservedBytes != 0 || p.pending != 0 {
			t.Errorf("capacity leaked: reserved=%d bytes=%d pending=%d", p.reserved, p.reservedBytes, p.pending)
		}
	})
}

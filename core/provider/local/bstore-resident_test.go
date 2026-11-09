package provider_local

import (
	"context"
	"testing"
	"time"

	account_settings "github.com/s4wave/spacewave/core/account/settings"
)

// TestResidentMarksApplyLive checks that SetResidentMarks lowers a mounted
// placed store's high mark while it runs, without a settings change: the
// eviction loop runs a pass at once, and again at the next change though its
// last pass fell short and left it waiting out evictShortWait. A placement
// change keeps the marks over it, and clearing them restores the
// placement's.
func TestResidentMarksApplyLive(t *testing.T) {
	// Place the store with no resident budget and wait for its upload.
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()
	p := setupPlacedStore(ctx, t)
	p.place(ctx, t, &account_settings.BlockStoreCache{SettleSeconds: 3600})
	bs := p.waitUpload(ctx, t, func(status UploadStatus) bool {
		return status.Policy.Settle == time.Hour && status.Pending == 0
	})
	rootSize, childSize := int64(len(p.blocks[p.rootRef])), int64(len(p.blocks[p.childRef]))

	// Count the eviction passes through the admit, which each pass asks
	// before its first batch.
	admitted := make(chan struct{}, 16)
	p.acc.SetEvictAdmit(func(_ context.Context, lastBytes int64) error {
		if lastBytes == 0 {
			admitted <- struct{}{}
		}
		return nil
	})
	waitPass := func(what string) {
		t.Helper()
		select {
		case <-admitted:
		case <-ctx.Done():
			t.Fatalf("no eviction pass after %s", what)
		}
	}

	// A high mark past the store records reads and runs no pass. Read the
	// root, so the next pass keeps it.
	p.acc.SetResidentMarks(ResidentMarks{HighBytes: 1 << 40})
	bs = p.waitUpload(ctx, t, func(status UploadStatus) bool { return status.Policy.ResidentHigh == 1<<40 })
	if _, found, err := bs.GetBlock(ctx, p.rootRef); err != nil || !found {
		t.Fatalf("read root = %v/%v", found, err)
	}

	// Lower the high mark below the store: a pass evicts the child and keeps
	// the read root, falling short of the mark.
	p.acc.SetResidentMarks(ResidentMarks{HighBytes: 1})
	waitPass("lowering the high mark")
	bs = p.waitUpload(ctx, t, func(status UploadStatus) bool {
		return status.Policy.ResidentHigh == 1 && status.EvictedBytes == childSize && status.ResidentBytes() == rootSize
	})

	// The short pass left the loop waiting out evictShortWait; a change of
	// the marks runs the next pass at once.
	p.acc.SetResidentMarks(ResidentMarks{HighBytes: 2})
	waitPass("changing the marks after a short pass")

	// A placement change keeps the marks over its settle age.
	p.place(ctx, t, &account_settings.BlockStoreCache{SettleSeconds: 1800})
	p.waitUpload(ctx, t, func(status UploadStatus) bool {
		return status.Policy.Settle == 30*time.Minute && status.Policy.ResidentHigh == 2
	})

	// Clearing the marks restores the placement's, which evict nothing.
	p.acc.SetResidentMarks(ResidentMarks{})
	p.waitUpload(ctx, t, func(status UploadStatus) bool {
		return status.Policy.ResidentHigh == 0 && status.Policy.ResidentLow == 0 && status.Policy.Settle == 30*time.Minute
	})
}

// TestResidentMarksApply checks how the marks set over a placement's policy.
func TestResidentMarksApply(t *testing.T) {
	base := uploadPolicy(&account_settings.BlockStoreCache{ResidentHighBytes: 1000, ResidentLowBytes: 800, MaxPendingBytes: 500})
	for _, c := range []struct {
		marks           ResidentMarks
		high, low, pend int64
	}{
		{ResidentMarks{}, 1000, 800, 500},
		{ResidentMarks{HighBytes: 100}, 100, 90, 500},
		{ResidentMarks{HighBytes: 100, LowBytes: 50, MaxPendingBytes: 40}, 100, 50, 40},
		{ResidentMarks{LowBytes: 900}, 1000, 900, 500},
		{ResidentMarks{HighBytes: 100, LowBytes: 200}, 100, 90, 500},
	} {
		got := c.marks.apply(base)
		if got.ResidentHigh != c.high || got.ResidentLow != c.low || got.MaxPendingBytes != c.pend {
			t.Fatalf("%+v over the placement = %d/%d/%d, want %d/%d/%d", c.marks, got.ResidentHigh, got.ResidentLow, got.MaxPendingBytes, c.high, c.low, c.pend)
		}
	}
}

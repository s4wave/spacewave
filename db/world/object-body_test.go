package world

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestGetObjectBodiesBatchPageWithholdsOversizedBody(t *testing.T) {
	// Prepare a body whose encoded entry alone exceeds the page budget.
	const byteBudget = 100
	body := make([]byte, byteBudget)
	keys := []string{"body/oversized", "body/small"}

	// Request a page holding the oversized body and a small one after it.
	page, consumed, err := getObjectBodiesBatchPage(
		context.Background(),
		keys,
		byteBudget,
		1,
		func(_ context.Context, key string) (*ObjectBody, error) {
			if key == "body/small" {
				return &ObjectBody{ObjectKey: key, Body: []byte("small"), Rev: 2, Exists: true}, nil
			}
			return &ObjectBody{ObjectKey: key, Body: body, Rev: 1, Exists: true}, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	// Require the oversized entry withheld with its size and the page to go on.
	if len(page) != 2 || consumed != 0 {
		t.Fatalf("page = %v, consumed = %d, want both keys answered", page, consumed)
	}
	withheld := page[0]
	if withheld.ObjectKey != keys[0] || !withheld.Exists || withheld.Rev != 1 || withheld.Body != nil || withheld.WithheldSize != byteBudget {
		t.Fatalf("withheld entry = %+v", withheld)
	}
	if string(page[1].Body) != "small" || page[1].WithheldSize != 0 {
		t.Fatalf("small entry = %+v", page[1])
	}
}

// TestGetObjectBodiesBatchPageOverlapsLookups runs pages of 16 and 160 keys
// with a lookahead of 16. Each lookup waits until 16 are in flight, so a pager
// that read one body at a time fails on the deadline; the larger page stops at
// its budget, and no lookup may still run once the page returns, since the
// caller releases the cursor the lookups read through.
func TestGetObjectBodiesBatchPageOverlapsLookups(t *testing.T) {
	const width = 16
	for _, n := range []int{width, 10 * width} {
		// Count lookups in flight and release them once width have started.
		var inflight, peak atomic.Int32
		full := make(chan struct{})
		var fullOnce sync.Once
		lookup := func(ctx context.Context, key string) (*ObjectBody, error) {
			defer inflight.Add(-1)
			for now, seen := inflight.Add(1), peak.Load(); now > seen && !peak.CompareAndSwap(seen, now); seen = peak.Load() {
			}
			if peak.Load() >= width {
				fullOnce.Do(func() { close(full) })
			}
			select {
			case <-full:
			case <-time.After(5 * time.Second):
				return nil, errors.New("lookups did not overlap")
			}
			return &ObjectBody{ObjectKey: key, Body: []byte("body"), Exists: true}, nil
		}

		// Page through every key; the larger set fills a budget of 100 bodies.
		keys := make([]string, n)
		for i := range keys {
			keys[i] = fmt.Sprintf("body/%04d", i)
		}
		budget := 100 * objectBodyEncodedSize(&ObjectBody{ObjectKey: keys[n-1], Body: []byte("body"), Exists: true})
		page, consumed, err := getObjectBodiesBatchPage(context.Background(), keys, budget, width, lookup)
		if err != nil {
			t.Fatalf("n=%d: %v", n, err)
		}

		// Require the bodies in key order, the budget's cut, and no lookup left.
		if running := inflight.Load(); running != 0 {
			t.Fatalf("n=%d: %d lookups outlived the page", n, running)
		}
		want := min(n, 100)
		if len(page) != want || (n > 100) != (consumed != 0) || (consumed != 0 && int(consumed) != want) {
			t.Fatalf("n=%d: page of %d, consumed %d, want %d", n, len(page), consumed, want)
		}
		for i, body := range page {
			if body.ObjectKey != keys[i] {
				t.Fatalf("n=%d: page[%d] = %q, want %q", n, i, body.ObjectKey, keys[i])
			}
		}
		if got := peak.Load(); got != width {
			t.Fatalf("n=%d: peak %d lookups in flight, want %d", n, got, width)
		}
	}
}

package packahead

import (
	"bytes"
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// TestAppendReserve refuses a batch that would leave less free disk than the
// reserve, writing none of it, while reads and the drain go on, and appends
// again once the space returns.
func TestAppendReserve(t *testing.T) {
	ctx := context.Background()
	var free atomic.Uint64
	free.Store(64 << 20)
	const reserve = 16 << 20
	l, err := Open(ctx, t.TempDir(), Options{
		MaxPackBlocks: 2,
		ReserveBytes:  reserve,
		FreeBytes:     func(string) (uint64, error) { return free.Load(), nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()

	// Above the reserve, a pack of two blocks fills and finalizes, and a
	// third opens the next pack.
	blocks := []Block{testBlock(t, 0, 4096), testBlock(t, 1, 4096), testBlock(t, 2, 4096), testBlock(t, 3, 4096)}
	for _, b := range blocks[:3] {
		if err := l.Append(ctx, []Block{b}); err != nil {
			t.Fatal(err)
		}
	}
	waitFinalized(t, l, 1)

	// Below the reserve, and just above it by less than the batch, the batch
	// is refused and nothing of it is written.
	for _, f := range []uint64{reserve - 1, reserve + 100} {
		free.Store(f)
		if err := l.Append(ctx, blocks[3:]); !errors.Is(err, ErrReserve) {
			t.Fatalf("free %d: append returned %v, want ErrReserve", f, err)
		}
		if l.Has(blocks[3].Key) {
			t.Fatalf("free %d: the refused block is held", f)
		}
	}

	// A block the Log holds writes nothing, so it appends under the reserve.
	if err := l.Append(ctx, blocks[:1]); err != nil {
		t.Fatalf("append of a held block under the reserve: %v", err)
	}

	// Reads go on under the reserve.
	for _, b := range blocks[:3] {
		got, ok, err := l.Get(b.Key)
		if err != nil || !ok || !bytes.Equal(got, b.Value) {
			t.Fatalf("read under the reserve: %v, %v", ok, err)
		}
	}

	// The drain uploads the finalized pack under the reserve.
	remote := &countRemote{listed: map[string]bool{}}
	d := NewDrain(l, DrainOptions{})
	runCtx, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	go func() { done <- d.Run(runCtx, remote) }()
	deadline := time.After(10 * time.Second)
	for {
		st, changed := d.Status()
		if st.Pending == 0 && st.Written >= 1 {
			break
		}
		select {
		case <-changed:
		case <-time.After(50 * time.Millisecond):
		case <-deadline:
			t.Fatalf("drain under the reserve did not finish: %+v", st)
		}
	}
	cancel()
	<-done

	// Once the space returns, the batch appends and reads.
	free.Store(64 << 20)
	if err := l.Append(ctx, blocks[3:]); err != nil {
		t.Fatalf("append after the space returned: %v", err)
	}
	if got, ok, err := l.Get(blocks[3].Key); err != nil || !ok || !bytes.Equal(got, blocks[3].Value) {
		t.Fatalf("read after the space returned: %v, %v", ok, err)
	}
}

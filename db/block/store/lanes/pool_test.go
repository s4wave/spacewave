package lanes

import (
	"context"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
)

// TestPool holds every lane, queues calls of each class, and checks the order
// the released lanes reach them: urgent, interactive, then refills within
// the cap, with a raised refill joining the interactive calls, a preferring
// waiter taking its lane, and a call that left giving its grant back.
func TestPool(t *testing.T) {
	ctx := context.Background()
	refill := block.WithReadClass(ctx, block.ReadRefill)
	urgent := block.WithReadClass(ctx, block.ReadUrgent)
	p := New([]string{"a", "a", "b"}, 1)

	// Refills hold at most the two lanes the reserve leaves them.
	r1 := take(t, p, refill, nil)
	r2 := take(t, p, refill, nil)
	if !r1.Capped || !r2.Capped {
		t.Fatal("refill leases are not capped")
	}
	held, _ := p.Held()
	if held != 2 {
		t.Fatalf("refills hold %d lanes; want 2", held)
	}
	blocked := wait(p, refill, nil)
	i1 := take(t, p, ctx, nil)
	if i1.Capped {
		t.Fatal("interactive lease is capped")
	}

	// Queue one call of each kind, plus a refill that is raised and one
	// interactive call that prefers lane b.
	raise := make(chan struct{})
	raised := wait(p, block.WithRaise(refill, raise), nil)
	inter := wait(p, ctx, nil)
	prefB := wait(p, ctx, func(s string) bool { return s == "b" })
	gone, leave := context.WithCancel(urgent)
	left := wait(p, gone, nil)
	urg := wait(p, urgent, nil)
	settle()
	close(raise)
	leave()
	settle()
	if err := recv(t, left).err; err == nil {
		t.Fatal("left call got a lane")
	}

	// The released lanes go to the urgent call, then the interactive calls in
	// order, except that lane b passes to the call preferring it.
	if r1.Lane != "b" || r2.Lane != "a" || i1.Lane != "a" {
		t.Fatalf("leases %q %q %q; want b a a", r1.Lane, r2.Lane, i1.Lane)
	}
	p.Put(r2)
	u := recv(t, urg)
	if u.lease.Lane != "a" {
		t.Fatalf("urgent got %q; want a", u.lease.Lane)
	}
	p.Put(r1)
	if got := recv(t, prefB); got.lease.Lane != "b" {
		t.Fatalf("preferring call got %q; want b", got.lease.Lane)
	}
	p.Put(i1)
	if got := recv(t, inter); got.lease.Lane != "a" {
		t.Fatalf("interactive got %q; want a", got.lease.Lane)
	}

	// The raised refill waits as interactive and takes the next lane ahead of
	// the capped refill, though refills now hold none.
	p.Put(u.lease)
	select {
	case got := <-raised:
		if got.lease.Capped {
			t.Fatal("raised refill lease is capped")
		}
		p.Put(got.lease)
	case <-blocked:
		t.Fatal("capped refill got a lane before the raised one")
	case <-time.After(time.Second):
		t.Fatal("raised refill got no lane")
	}
	if got := recv(t, blocked); !got.lease.Capped {
		t.Fatal("refill lease is not capped")
	}
}

// granted is the result of a waiting Take.
type granted struct {
	lease *Lease[string]
	err   error
}

// recv returns the result of a waiting Take that must arrive.
func recv(t *testing.T, ch <-chan granted) granted {
	t.Helper()
	select {
	case got := <-ch:
		return got
	case <-time.After(time.Second):
		t.Fatal("waiting call got no lane")
		return granted{}
	}
}

// take takes a lane that must be free.
func take(t *testing.T, p *Pool[string], ctx context.Context, prefer func(string) bool) *Lease[string] {
	t.Helper()
	l, err := p.Take(ctx, prefer)
	if err != nil {
		t.Fatal(err)
	}
	return l
}

// wait starts a Take and returns its result channel once it has queued.
func wait(p *Pool[string], ctx context.Context, prefer func(string) bool) <-chan granted {
	out := make(chan granted, 1)
	go func() {
		l, err := p.Take(ctx, prefer)
		out <- granted{l, err}
	}()
	settle()
	return out
}

// settle lets started goroutines reach their wait.
func settle() {
	time.Sleep(10 * time.Millisecond)
}

package cdn_bstore

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/core/cdn"
)

// testGrants issues grants of a fixed life on a fake clock.
type testGrants struct {
	mtx   sync.Mutex
	now   time.Time
	life  time.Duration
	err   error
	calls atomic.Int64
	// gate, when set, holds each issue until it closes.
	gate chan struct{}
}

// clock reads the fake clock.
func (s *testGrants) clock() time.Time {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.now
}

// advance moves the fake clock.
func (s *testGrants) advance(d time.Duration) {
	s.mtx.Lock()
	s.now = s.now.Add(d)
	s.mtx.Unlock()
}

// SpaceGrant issues a grant ending one life from now, or the set error.
func (s *testGrants) SpaceGrant(ctx context.Context, spaceID string) (*cdn.CdnSpaceGrant, error) {
	s.calls.Add(1)
	if s.gate != nil {
		<-s.gate
	}
	s.mtx.Lock()
	defer s.mtx.Unlock()
	if s.err != nil {
		return nil, s.err
	}
	return &cdn.CdnSpaceGrant{
		SpaceId:       spaceID,
		PackBaseUrl:   "https://bucket.example/file/b",
		Authorization: "token-" + s.now.String(),
		ExpiresAtMs:   s.now.Add(s.life).UnixMilli(),
	}, nil
}

// newTestKeeper returns a keeper over src's fake clock.
func newTestKeeper(src *testGrants) *GrantKeeper {
	k := NewGrantKeeper(src, "space")
	k.now = src.clock
	return k
}

// TestGrantKeeperRenews requires one renewal per three quarters of a grant's
// life, a valid grant kept through a failed renewal, and a refused grant
// replaced on the next read.
func TestGrantKeeperRenews(t *testing.T) {
	ctx := t.Context()
	src := &testGrants{now: time.UnixMilli(1_000_000), life: 100 * time.Second}
	k := newTestKeeper(src)

	// The first read fetches, and reads before the renewal point reuse it.
	first, err := k.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	src.advance(74 * time.Second)
	if g, _ := k.Current(ctx); g != first || src.calls.Load() != 1 {
		t.Fatalf("renewed early: %d calls", src.calls.Load())
	}

	// Past three quarters of its life, a read renews before expiry.
	src.advance(2 * time.Second)
	second, err := k.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if second == first || src.calls.Load() != 2 {
		t.Fatalf("did not renew at 76%% of life: %d calls", src.calls.Load())
	}

	// A failed renewal keeps the valid grant, and asks again only after the
	// retry delay.
	src.advance(80 * time.Second)
	src.err = errors.New("issuer down")
	if g, err := k.Current(ctx); err != nil || g != second {
		t.Fatalf("lost the valid grant on a failed renewal: %v", err)
	}
	calls := src.calls.Load()
	if g, _ := k.Current(ctx); g != second || src.calls.Load() != calls {
		t.Fatal("retried before the retry delay")
	}

	// Once the grant expires, the failure reaches the read.
	src.advance(30 * time.Second)
	if _, err := k.Current(ctx); err == nil {
		t.Fatal("served an expired grant")
	}

	// A grant the bucket refused is replaced on the next read.
	src.err = nil
	third, err := k.Current(ctx)
	if err != nil {
		t.Fatal(err)
	}
	k.refused(third)
	if g, _ := k.Current(ctx); g == third {
		t.Fatal("kept a refused grant")
	}
}

// TestGrantKeeperSharesRenewal requires concurrent reads with no valid grant
// to share one renewal, and reads holding a valid grant not to wait on it.
func TestGrantKeeperSharesRenewal(t *testing.T) {
	ctx := t.Context()
	src := &testGrants{now: time.UnixMilli(1_000_000), life: 100 * time.Second, gate: make(chan struct{})}
	k := newTestKeeper(src)

	// Eight reads with no grant wait on one renewal.
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			if _, err := k.Current(ctx); err != nil {
				t.Error(err)
			}
		})
	}
	for src.calls.Load() == 0 {
		time.Sleep(time.Millisecond)
	}
	close(src.gate)
	wg.Wait()
	if n := src.calls.Load(); n != 1 {
		t.Fatalf("%d renewals for one grant", n)
	}

	// While one read renews a due grant, the others get the valid one.
	first, _ := k.Current(ctx)
	src.gate = make(chan struct{})
	src.advance(80 * time.Second)
	renewed := make(chan *cdn.CdnSpaceGrant)
	go func() {
		g, _ := k.Current(ctx)
		renewed <- g
	}()
	for src.calls.Load() == 1 {
		time.Sleep(time.Millisecond)
	}
	if g, err := k.Current(ctx); err != nil || g != first {
		t.Fatalf("a read waited on the renewal: %v", err)
	}
	close(src.gate)
	if g := <-renewed; g == first {
		t.Fatal("the renewing read kept the old grant")
	}
}

// TestValidateGrant refuses a grant for another Space, with no base, or
// expired.
func TestValidateGrant(t *testing.T) {
	now := time.UnixMilli(1_000_000)
	ok := &cdn.CdnSpaceGrant{SpaceId: "s", PackBaseUrl: "https://b", ExpiresAtMs: now.Add(time.Minute).UnixMilli()}
	if err := ValidateGrant(ok, "s", now); err != nil {
		t.Fatal(err)
	}
	for name, g := range map[string]*cdn.CdnSpaceGrant{
		"space":   {SpaceId: "t", PackBaseUrl: "https://b", ExpiresAtMs: ok.ExpiresAtMs},
		"base":    {SpaceId: "s", ExpiresAtMs: ok.ExpiresAtMs},
		"expired": {SpaceId: "s", PackBaseUrl: "https://b", ExpiresAtMs: now.UnixMilli()},
	} {
		if ValidateGrant(g, "s", now) == nil {
			t.Errorf("accepted a grant with a bad %s", name)
		}
	}
}

package provider_local

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/s4wave/spacewave/db/block"
	dex_solicit "github.com/s4wave/spacewave/db/dex/solicit"
	"github.com/sirupsen/logrus"
	logtest "github.com/sirupsen/logrus/hooks/test"
)

// flakyPeers fails its first fails reads with err, or with a failed
// exchange when err is nil, then answers from blk, which nil makes a miss.
type flakyPeers struct {
	block.StoreOps
	fails int32
	err   error
	reads atomic.Int32
	blk   *block.StoredBlock
}

// GetStoredBlock fails, then answers.
func (f *flakyPeers) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if f.reads.Add(1) <= f.fails {
		if f.err != nil {
			return nil, f.err
		}
		return nil, errors.New("stream reset by the peer")
	}
	return f.blk, nil
}

// peerRead reads ref through a peerMissLog over peers.
func peerRead(t *testing.T, peers block.StoreOps) (*block.StoredBlock, time.Duration, error) {
	t.Helper()
	p := &peerMissLog{StoreOps: peers, le: logrus.NewEntry(logrus.New()), seen: newSeenRefs()}
	ref := block.NewBlockRef(nil)
	started := time.Now()
	blk, err := p.GetStoredBlock(context.Background(), ref)
	return blk, time.Since(started), err
}

// TestPeerReadRetriesFailures checks that a failed peer read asks again
// within the bound and returns the block a later read finds.
func TestPeerReadRetriesFailures(t *testing.T) {
	SetPeerReadWait(10 * time.Second)
	defer SetPeerReadWait(0)
	want := &block.StoredBlock{Data: []byte("served")}
	peers := &flakyPeers{fails: 3, blk: want}
	blk, _, err := peerRead(t, peers)
	if err != nil || blk != want {
		t.Fatalf("read: %v %v", blk, err)
	}
	if n := peers.reads.Load(); n != 4 {
		t.Fatalf("%d reads, want 4", n)
	}
}

// TestPeerReadMissIsFinal checks that a peer's answer that it lacks the block
// ends the read at once.
func TestPeerReadMissIsFinal(t *testing.T) {
	SetPeerReadWait(10 * time.Second)
	defer SetPeerReadWait(0)
	peers := &flakyPeers{}
	blk, took, err := peerRead(t, peers)
	if blk != nil || err != nil || took > time.Second || peers.reads.Load() != 1 {
		t.Fatalf("miss: %v %v after %v and %d reads", blk, err, took, peers.reads.Load())
	}
}

// TestPeerReadFailsUnavailable checks that a peer read failing past the bound
// fails with block.ErrUnavailable, not a miss.
func TestPeerReadFailsUnavailable(t *testing.T) {
	SetPeerReadWait(300 * time.Millisecond)
	defer SetPeerReadWait(0)
	peers := &flakyPeers{fails: 1 << 30}
	blk, took, err := peerRead(t, peers)
	if blk != nil || !errors.Is(err, block.ErrUnavailable) || took > 2*time.Second {
		t.Fatalf("away: %v %v after %v", blk, err, took)
	}
	if peers.reads.Load() < 2 {
		t.Fatalf("%d reads, want retries", peers.reads.Load())
	}
}

// hangingPeers answers nothing until ctx ends.
type hangingPeers struct {
	block.StoreOps
	asks atomic.Int32
}

// GetBlockExists waits for ctx.
func (h *hangingPeers) GetBlockExists(ctx context.Context, _ *block.BlockRef) (bool, error) {
	h.asks.Add(1)
	<-ctx.Done()
	return false, ctx.Err()
}

// GetBlockExistsBatch waits for ctx.
func (h *hangingPeers) GetBlockExistsBatch(ctx context.Context, _ []*block.BlockRef) ([]bool, error) {
	h.asks.Add(1)
	<-ctx.Done()
	return nil, ctx.Err()
}

// TestPeerExistsAsksNoPeer checks that an existence probe, such as a put's,
// reports the block absent at once without asking a peer that never answers.
func TestPeerExistsAsksNoPeer(t *testing.T) {
	peers := &hangingPeers{}
	p := &peerMissLog{StoreOps: peers, le: logrus.NewEntry(logrus.New()), seen: newSeenRefs()}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ref := block.NewBlockRef(nil)
	if found, err := p.GetBlockExists(ctx, ref); found || err != nil {
		t.Fatalf("exists: %v %v", found, err)
	}
	if found, err := p.GetBlockExistsBatch(ctx, []*block.BlockRef{ref, ref}); len(found) != 2 || found[0] || found[1] || err != nil {
		t.Fatalf("exists batch: %v %v", found, err)
	}
	if ctx.Err() != nil || peers.asks.Load() != 0 {
		t.Fatalf("asked the peers %d times", peers.asks.Load())
	}
}

// TestPeerReadDeferred checks that a read under block.WithPeerReads asks no
// peer and fails unavailable at once, that the PeerReads' fetch reads the
// peer, and that the next read answers from the fetched block.
func TestPeerReadDeferred(t *testing.T) {
	want := &block.StoredBlock{Data: []byte("served")}
	peers := &flakyPeers{blk: want}
	p := &peerMissLog{StoreOps: peers, le: logrus.NewEntry(logrus.New()), seen: newSeenRefs()}
	ref := block.NewBlockRef(nil)
	reads := block.NewPeerReads()
	ctx := block.WithPeerReads(context.Background(), reads)

	blk, err := p.GetStoredBlock(ctx, ref)
	if blk != nil || !errors.Is(err, block.ErrUnavailable) || peers.reads.Load() != 0 || !reads.Pending() {
		t.Fatalf("deferred read: %v %v, %d peer reads, pending %v", blk, err, peers.reads.Load(), reads.Pending())
	}
	ok, err := reads.Fetch(context.Background())
	if !ok || err != nil || peers.reads.Load() != 1 || reads.Pending() {
		t.Fatalf("fetch: %v %v, %d peer reads", ok, err, peers.reads.Load())
	}
	blk, err = p.GetStoredBlock(ctx, ref)
	if blk != want || err != nil || peers.reads.Load() != 1 {
		t.Fatalf("read after fetch: %v %v, %d peer reads", blk, err, peers.reads.Load())
	}
}

// countedPeerRead reads ref through a peerMissLog over peers that counts in a
// placement's peer read counts, and returns the placement's read status.
func countedPeerRead(t *testing.T, peers block.StoreOps) ReadStatus {
	t.Helper()
	p := &placementState{remote: &atomic.Pointer[openBackend]{}}
	src := newPeerMissSource(logrus.NewEntry(logrus.New()), func() block.StoreOps { return peers }, &p.peerReads)
	_, _ = src().GetStoredBlock(context.Background(), block.NewBlockRef(nil))
	return p.readStatus()
}

// TestPeerReadCountsServed checks that a block a peer serves counts once as
// served.
func TestPeerReadCountsServed(t *testing.T) {
	SetPeerReadWait(10 * time.Second)
	defer SetPeerReadWait(0)
	st := countedPeerRead(t, &flakyPeers{fails: 1, blk: &block.StoredBlock{Data: []byte("served")}})
	if st.PeerServed != 1 || st.PeerNotHeld != 0 || st.PeerFailed != (PeerFailedCauses{}) {
		t.Fatalf("counts: %+v", st)
	}
}

// TestPeerReadCountsNotHeld checks that the peers' answer that they lack the
// block counts once as not held.
func TestPeerReadCountsNotHeld(t *testing.T) {
	SetPeerReadWait(10 * time.Second)
	defer SetPeerReadWait(0)
	st := countedPeerRead(t, &flakyPeers{})
	if st.PeerServed != 0 || st.PeerNotHeld != 1 || st.PeerFailed != (PeerFailedCauses{}) {
		t.Fatalf("counts: %+v", st)
	}
}

// TestPeerReadCountsFailed checks that a read failing until the bound ends
// counts once as failed with its cause, however many times it asked.
func TestPeerReadCountsFailed(t *testing.T) {
	SetPeerReadWait(300 * time.Millisecond)
	defer SetPeerReadWait(0)
	peers := &flakyPeers{fails: 1 << 30}
	st := countedPeerRead(t, peers)
	if st.PeerServed != 0 || st.PeerNotHeld != 0 || st.PeerFailed != (PeerFailedCauses{Exchange: 1}) || peers.reads.Load() < 2 {
		t.Fatalf("counts: %+v after %d reads", st, peers.reads.Load())
	}
}

// TestPeerReadRemembersUnserved checks that a block the peers refused fails
// unavailable at once on the next read, asks no peer and counts as
// remembered, not failed, and that the memory ends after peerUnservedFor. A
// read that failed on an exchange, a blip, is not remembered: the next read
// asks again and is served.
func TestPeerReadRemembersUnserved(t *testing.T) {
	// Fail a block on a peer's own refusal, shaped as the fanout returns it.
	SetPeerReadWait(300 * time.Millisecond)
	defer SetPeerReadWait(0)
	refusal := fmt.Errorf("%w: no peer served block: %w", block.ErrUnavailable, fmt.Errorf("%w: tier away on the peer", dex_solicit.ErrPeerError))
	peers := &flakyPeers{fails: 1 << 30, err: refusal}
	p := &placementState{remote: &atomic.Pointer[openBackend]{}}
	src := newPeerMissSource(logrus.NewEntry(logrus.New()), func() block.StoreOps { return peers }, &p.peerReads)
	ref := block.NewBlockRef(nil)
	if _, err := src().GetStoredBlock(context.Background(), ref); !errors.Is(err, block.ErrUnavailable) {
		t.Fatalf("first read: %v", err)
	}
	asked := peers.reads.Load()

	// The next read answers from memory.
	started := time.Now()
	blk, err := src().GetStoredBlock(context.Background(), ref)
	st := p.readStatus()
	if blk != nil || !errors.Is(err, block.ErrUnavailable) || time.Since(started) > 100*time.Millisecond || peers.reads.Load() != asked {
		t.Fatalf("second read: %v %v after %v, %d peer reads, want %d", blk, err, time.Since(started), peers.reads.Load(), asked)
	}
	if st.PeerRemembered != 1 || st.PeerFailed != (PeerFailedCauses{PeerError: 1}) {
		t.Fatalf("counts: %+v", st)
	}
	mem := src().(*peerMissLog).unserved
	if !mem.holds(ref.MarshalString(), time.Now().Add(peerUnservedFor-time.Second)) || mem.holds(ref.MarshalString(), time.Now().Add(peerUnservedFor)) {
		t.Fatal("memory does not end after peerUnservedFor")
	}

	// A blip fails a read on its exchange, and the next read asks again and
	// is served.
	want := &block.StoredBlock{Data: []byte("served")}
	blip := &flakyPeers{fails: 1 << 30, blk: want}
	q := &placementState{remote: &atomic.Pointer[openBackend]{}}
	src = newPeerMissSource(logrus.NewEntry(logrus.New()), func() block.StoreOps { return blip }, &q.peerReads)
	if _, err := src().GetStoredBlock(context.Background(), ref); !errors.Is(err, block.ErrUnavailable) {
		t.Fatalf("blip read: %v", err)
	}
	blip.fails = 0
	if blk, err := src().GetStoredBlock(context.Background(), ref); err != nil || blk != want {
		t.Fatalf("read after the blip: %v %v", blk, err)
	}
	if st := q.readStatus(); st.PeerRemembered != 0 || st.PeerServed != 1 || st.PeerFailed != (PeerFailedCauses{Exchange: 1}) {
		t.Fatalf("blip counts: %+v", st)
	}

	// Only a tree of refusals is proof: a refusal beside a failed exchange,
	// a timeout or a closed session is not.
	reset := errors.New("stream reset by the peer")
	for _, tc := range []struct {
		err  error
		want bool
	}{
		{refusal, true},
		{fmt.Errorf("%w: no peer served block: %w", block.ErrUnavailable, errors.Join(refusal, reset)), false},
		{fmt.Errorf("%w: no peer served block: %w", block.ErrUnavailable, errors.Join(refusal, context.DeadlineExceeded)), false},
		{fmt.Errorf("%w: %w", block.ErrUnavailable, dex_solicit.ErrNoPeerSession), false},
		{reset, false},
	} {
		if got := peerRefused(tc.err); got != tc.want {
			t.Errorf("peerRefused(%v) = %v", tc.err, got)
		}
	}
}

// putStore takes a put, or fails it with err.
type putStore struct {
	block.StoreOps
	err error
}

// PutBlock takes the put or fails it.
func (s *putStore) PutBlock(context.Context, []byte, *block.PutOpts) (*block.BlockRef, bool, error) {
	return nil, s.err == nil, s.err
}

// TestPeerFillCounts checks that a peer fill counts only once it succeeds.
func TestPeerFillCounts(t *testing.T) {
	p := &placementState{remote: &atomic.Pointer[openBackend]{}}
	ref := block.NewBlockRef(nil)
	stored := &block.StoredBlock{Data: []byte("served"), RefsKnown: true}
	if err := p.peerFill(&putStore{err: errors.New("disk away")})(context.Background(), nil, ref, stored); err == nil {
		t.Fatal("failed fill reported no error")
	}
	if err := p.peerFill(&putStore{})(context.Background(), nil, ref, stored); err != nil {
		t.Fatalf("fill: %v", err)
	}
	if st := p.readStatus(); st.PeerFilled != 1 || st.PeerServed != 0 {
		t.Fatalf("counts: %+v", st)
	}
}

// causePeers fails each read the way its mode says, or serves blk in
// causeServe, and counts the reads.
type causePeers struct {
	block.StoreOps
	mode  atomic.Int32
	reads atomic.Int32
	blk   *block.StoredBlock
}

// The failure modes of causePeers, each shaped as the solicit store returns
// it.
const (
	// causeServe serves the block.
	causeServe int32 = iota
	// causePeerError answers with the fanout's error over two peers, one
	// that errored and one that timed out.
	causePeerError
	// causeTimeout waits for the read's bound and fails as the fanout does
	// with a request outstanding.
	causeTimeout
	// causeNoSession waits for the read's bound as the solicit store does
	// with no peer session, and fails as it does.
	causeNoSession
	// causeExchange fails as a reset stream does, with no answer from the
	// peer.
	causeExchange
)

// GetStoredBlock fails or serves by the mode.
func (c *causePeers) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	c.reads.Add(1)
	switch c.mode.Load() {
	case causePeerError:
		peerErr := fmt.Errorf("%w: tier away on the peer", dex_solicit.ErrPeerError)
		return nil, fmt.Errorf("%w: no peer served block: %w", block.ErrUnavailable, errors.Join(context.DeadlineExceeded, peerErr))
	case causeTimeout:
		<-ctx.Done()
		return nil, fmt.Errorf("%w: no peer served block: %w", block.ErrUnavailable, ctx.Err())
	case causeNoSession:
		<-ctx.Done()
		return nil, fmt.Errorf("%w: %w", dex_solicit.ErrNoPeerSession, ctx.Err())
	case causeExchange:
		return nil, fmt.Errorf("%w: no peer served block: %w", block.ErrUnavailable, errors.New("stream reset by the peer"))
	default:
		return c.blk, nil
	}
}

// TestPeerReadFailCauses tries to blur the failure causes: a peer error, a
// timeout, a missing session, a failed exchange and a caller that went away each count in their
// own cause, the canceled read in no cause, and only a failure warns, once
// per block with its cause and reader. After each failure the same block is
// still asked for again and served.
func TestPeerReadFailCauses(t *testing.T) {
	SetPeerReadWait(200 * time.Millisecond)
	defer SetPeerReadWait(0)
	for _, tc := range []struct {
		name   string
		mode   int32
		cancel bool
		want   ReadStatus
	}{
		{"peer_error", causePeerError, false, ReadStatus{PeerFailed: PeerFailedCauses{PeerError: 2}}},
		{"timeout", causeTimeout, false, ReadStatus{PeerFailed: PeerFailedCauses{Timeout: 2}}},
		{"no_session", causeNoSession, false, ReadStatus{PeerFailed: PeerFailedCauses{NoSession: 2}}},
		{"exchange", causeExchange, false, ReadStatus{PeerFailed: PeerFailedCauses{Exchange: 2}}},
		{"canceled", causeTimeout, true, ReadStatus{PeerCanceled: 2}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			logger, hook := logtest.NewNullLogger()
			p := &placementState{remote: &atomic.Pointer[openBackend]{}}
			want := &block.StoredBlock{Data: []byte("served")}
			peers := &causePeers{blk: want}
			src := newPeerMissSource(logrus.NewEntry(logger), func() block.StoreOps { return peers }, &p.peerReads)
			ref, err := block.BuildBlockRef([]byte(t.Name()), nil)
			if err != nil {
				t.Fatal(err)
			}

			// Fail the block twice, canceling the caller when the case says.
			peers.mode.Store(tc.mode)
			for range 2 {
				ctx, cancel := context.WithCancel(context.Background())
				if tc.cancel {
					time.AfterFunc(20*time.Millisecond, cancel)
				}
				blk, err := src().GetStoredBlock(ctx, ref)
				cancel()
				if blk != nil || err == nil {
					t.Fatalf("failed read: %v %v", blk, err)
				}
			}
			if st := p.readStatus(); st != tc.want {
				t.Fatalf("counts: %+v, want %+v", st, tc.want)
			}

			// Warn of a failure once, with its cause and reader.
			var warns []*logrus.Entry
			for _, e := range hook.AllEntries() {
				if e.Message == "peer read failed; the block stays unserved" {
					warns = append(warns, e)
				}
			}
			switch {
			case tc.cancel && len(warns) != 0:
				t.Fatalf("canceled reads warned %d times", len(warns))
			case !tc.cancel && len(warns) != 1:
				t.Fatalf("%d failure warnings, want 1", len(warns))
			case !tc.cancel:
				f := warns[0].Data
				if f["ref"] != ref.MarshalString() || f["cause"] != tc.name || f["root"] == "-" || f["site"] == "" {
					t.Fatalf("warning fields: %v", f)
				}
			}

			// Serve the same block on the next read, which asks again.
			asked := peers.reads.Load()
			peers.mode.Store(causeServe)
			blk, err := src().GetStoredBlock(context.Background(), ref)
			if blk != want || err != nil || peers.reads.Load() != asked+1 || p.readStatus().PeerServed != 1 {
				t.Fatalf("read after failures: %v %v, %d reads after %d", blk, err, peers.reads.Load(), asked)
			}
		})
	}
}

// TestPeerFailOnceBounded checks that the once-per-block set forgets its
// oldest ref past seenRefsCap, and that site names only main module frames.
func TestPeerFailOnceBounded(t *testing.T) {
	s := newSeenRefs()
	for i := range seenRefsCap + 1 {
		if !s.add(fmt.Sprint(i)) {
			t.Fatalf("ref %d counted as seen before it was added", i)
		}
	}
	if len(s.keys) != seenRefsCap || s.add(fmt.Sprint(seenRefsCap)) || !s.add("0") {
		t.Fatalf("set holds %d refs, want the newest %d", len(s.keys), seenRefsCap)
	}
	for fn, want := range map[string]bool{
		"example.com/app/backend.(*srv).read":   true,
		"example.com/app.Run":                   true,
		"main.main":                             true,
		"example.com/application/x.Read":        false,
		"github.com/aperturerobotics/util/x.Go": false,
	} {
		if inModule(fn, "example.com/app") != want {
			t.Errorf("inModule(%q) = %v, want %v", fn, !want, want)
		}
	}
}

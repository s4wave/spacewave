//go:build !js && !wasip1 && !windows

package main

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
)

// countingReader counts the bytes a fake server hands out.
type countingReader struct {
	r io.Reader
	n *atomic.Int64
}

// Read reads from r and counts the bytes read.
func (c *countingReader) Read(p []byte) (int, error) {
	n, err := c.r.Read(p)
	c.n.Add(int64(n))
	return n, err
}

// Close does nothing.
func (c *countingReader) Close() error { return nil }

// padServer returns each whole pack followed by pad extra bytes, counting
// the bytes it hands out.
type padServer struct {
	*fakeServer
	pad    int
	served atomic.Int64
}

// GetObject returns the pack and its padding.
func (p *padServer) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	rc, err := p.fakeServer.GetObject(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	data, _ := io.ReadAll(rc)
	data = append(data, make([]byte, p.pad)...)
	return &countingReader{r: bytes.NewReader(data), n: &p.served}, nil
}

// TestReviewOverRead serves a pack 8 MiB past its entry's size under a cap
// of that size: the rescue reads at most one byte past the size, fails the
// pack's refs as over-size and writes nothing.
func TestReviewOverRead(t *testing.T) {
	// Pad the only pack far past its size, with the cap at its size.
	src := newSource(t)
	refs := src.pack(4)
	_, size := onlyPack(t, src)
	out := t.TempDir()
	srv := &padServer{fakeServer: &fakeServer{dir: src.dir}, pad: 8 << 20}
	o := testOptions(src, writeList(t, out, refs), out)
	o.Fetch.CapBytes = uint64(size)

	// The read stops one byte past the size and fails the pack.
	res, err := run(context.Background(), o, srv)
	if err != nil {
		t.Fatal(err)
	}
	if served := srv.served.Load(); served > size+1 {
		t.Fatalf("served %d bytes of a pack of %d", served, size)
	}
	if res.Done != 0 || res.Failed != 4 || checkOutput(t, out, refs) != 0 {
		t.Fatalf("over-size pack: %+v", res)
	}
	l, err := os.ReadFile(o.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(l), "failed over-size") {
		t.Fatalf("ledger has no over-size refs:\n%s", l)
	}
}

// TestReviewZeroSizeEntry gives the only pack an index entry of size 0: the
// rescue does not take it as the pack's size, which would charge nothing,
// but asks SIZE, finds the pack past --max-pack-mib and fetches nothing.
func TestReviewZeroSizeEntry(t *testing.T) {
	// Rewrite the pack's index entry with size 0.
	src := newSource(t)
	refs := src.pack(4)
	id, _ := onlyPack(t, src)
	src.close()
	ctx := context.Background()
	tx, err := src.index.NewTransaction(ctx, true)
	if err != nil {
		t.Fatal(err)
	}
	e, err := block_store_s3.ReadIndexEntry(ctx, tx, id)
	if err != nil || e == nil {
		t.Fatal(err)
	}
	e.SizeBytes = 0
	v, err := e.MarshalVT()
	if err != nil {
		t.Fatal(err)
	}
	if err := tx.Set(ctx, []byte("e/"+id), v); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}

	// Under a cap of 1 and a guard of 1 byte nothing is read whole.
	out := t.TempDir()
	srv := &fakeServer{dir: src.dir}
	o := testOptions(src, writeList(t, out, refs), out)
	o.Fetch.CapBytes = 1
	o.MaxPackBytes = 1
	res, err := run(ctx, o, srv)
	if err != nil {
		t.Fatal(err)
	}
	if _, whole := srv.count(); whole != 0 || res.Sized != 1 || res.TooLarge != 4 || res.Bytes != 0 {
		t.Fatalf("zero-size entry: %d whole reads, %+v", whole, res)
	}
}

// stallServer hands out the first whole pack and then stalls before its end
// until the call's context ends, counting the bytes it hands out.
type stallServer struct {
	*fakeServer
	served  atomic.Int64
	once    sync.Once
	stalled chan struct{}
	calls   atomic.Int32
}

// stallReader reads r and then stalls at its end until ctx ends.
type stallReader struct {
	r       io.Reader
	n       *atomic.Int64
	ctx     context.Context
	stalled chan struct{}
	once    *sync.Once
}

// Read reads r, counting the bytes, and stalls at its end.
func (s *stallReader) Read(p []byte) (int, error) {
	// Count what r yields until its end.
	n, err := s.r.Read(p)
	s.n.Add(int64(n))
	if err != io.EOF {
		return n, err
	}

	// Signal the stall and wait for the call's end.
	s.once.Do(func() { close(s.stalled) })
	<-s.ctx.Done()
	return 0, s.ctx.Err()
}

// Close does nothing.
func (s *stallReader) Close() error { return nil }

// GetObject returns the pack, stalling before its end on the first call.
func (s *stallServer) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	// Read the pack from the fake server.
	rc, err := s.fakeServer.GetObject(ctx, bucket, key)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}

	// Stall the first call.
	if s.calls.Add(1) == 1 {
		return &stallReader{r: bytes.NewReader(data), n: &s.served, ctx: ctx, stalled: s.stalled, once: &s.once}, nil
	}
	return &countingReader{r: bytes.NewReader(data), n: &s.served}, nil
}

// TestReviewCrashLosesBytes crashes a run while a whole pack is in flight:
// the ledger as the disk holds it already charges the pack, so a run from
// it stops at the cap rather than reading the pack again past it.
func TestReviewCrashLosesBytes(t *testing.T) {
	// Run under a cap of one and a half packs until the pack is in flight.
	src := newSource(t)
	refs := src.pack(4)
	_, size := onlyPack(t, src)
	out := t.TempDir()
	srv := &stallServer{fakeServer: &fakeServer{dir: src.dir}, stalled: make(chan struct{})}
	o := testOptions(src, writeList(t, out, refs), out)
	o.Fetch.CapBytes = uint64(size) * 3 / 2
	o.Fetch.Timeout = 0
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		_, _ = run(ctx, o, srv)
	}()
	<-srv.stalled

	// Crash: keep the ledger as the disk holds it.
	crashed, err := os.ReadFile(o.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	<-done

	// The ledger as the crash left it charges the pack in flight.
	out2 := t.TempDir()
	o2 := testOptions(src, o.RefList, out2)
	o2.Fetch = o.Fetch
	if err := os.WriteFile(o2.Ledger, crashed, 0o644); err != nil {
		t.Fatal(err)
	}
	l, err := openLedger(o2.Ledger)
	if err != nil {
		t.Fatal(err)
	}
	charged := l.bytes
	if err := l.close(); err != nil {
		t.Fatal(err)
	}
	if charged < uint64(size) {
		t.Fatalf("ledger at the crash charges %d, the pack in flight is %d:\n%s", charged, size, crashed)
	}

	// A run from it stops at the cap.
	res, err := run(context.Background(), o2, srv)
	if err != nil || res.Stop != stopByteCap {
		t.Fatalf("run from the crashed ledger: %v, %+v", err, res)
	}
	if served := srv.served.Load(); uint64(served) > o.Fetch.CapBytes {
		t.Fatalf("served %d across runs past cap %d", served, o.Fetch.CapBytes)
	}
}

// TestReviewGrownListDropped finishes a pack for one of its refs, then adds
// the other three to the list: the next run fetches and records them.
func TestReviewGrownListDropped(t *testing.T) {
	// Finish the pack for its first ref.
	src := newSource(t)
	refs := src.pack(4)
	src.close()
	out := t.TempDir()
	o := testOptions(src, writeList(t, out, refs[:1]), out)
	if _, err := run(context.Background(), o, &fakeServer{dir: src.dir}); err != nil {
		t.Fatal(err)
	}

	// The grown list's new refs are fetched and stored.
	o.RefList = writeList(t, out, refs)
	srv := &fakeServer{dir: src.dir}
	res, err := run(context.Background(), o, srv)
	if err != nil {
		t.Fatal(err)
	}
	if calls, _ := srv.count(); calls != 1 || res.Done != 4 || checkOutput(t, out, refs) != 4 {
		t.Fatalf("grown list: %d requests, %+v, output holds %d of 4", calls, res, checkOutput(t, out, refs))
	}
}

package main

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/aperturerobotics/cli"
	"github.com/pkg/errors"
	block_packsweep_admin "github.com/s4wave/spacewave/db/block/packsweep/admin"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"golang.org/x/time/rate"
)

// admitEvery is how long one admission from the backend's gate covers the
// reads after it. A read starts at most this long after the gate last
// admitted, so a gate that closes holds every read within this time.
const admitEvery = time.Second

// paceBurst is the most bytes a read takes from the rate cap at once.
const paceBurst = 1 << 20

// pacingFlags binds the flags that pace a step's bulk reads of the store.
func (a *Args) pacingFlags() []cli.Flag {
	return []cli.Flag{
		&cli.Float64Flag{Name: "rate", Usage: "most MiB a second the step reads from the store, 0 for no cap", Destination: &a.RateMiB},
		&cli.StringFlag{Name: "gate-url", Usage: "the backend's pack sweep admin service, a ws or wss URL: each read waits until the backend admits it, as its own background passes wait; the Authorization header comes from " + adminAuthEnv, Destination: &a.GateURL},
	}
}

// pace wraps a store's reads in the rate cap and the backend's gate that
// the flags name, returning the store unchanged when they name neither.
func (a *Args) pace(ctx context.Context, store block_store_s3.ObjectStore) (block_store_s3.ObjectStore, func(), error) {
	// Refuse a negative cap, and leave an unpaced store alone.
	if a.RateMiB < 0 {
		return nil, nil, errors.New("--rate is negative")
	}
	if a.RateMiB == 0 && a.GateURL == "" && a.dialGate == nil {
		return store, func() {}, nil
	}

	// Cap the rate.
	p := &pacedStore{ObjectStore: store}
	if a.RateMiB > 0 {
		bytes := a.RateMiB * (1 << 20)
		p.limit = rate.NewLimiter(rate.Limit(bytes), min(paceBurst, max(int(bytes), 1)))
	}

	// Wait on the backend's gate.
	closeGate := func() {}
	if a.GateURL != "" || a.dialGate != nil {
		admit, closeAdmit, err := a.gate(ctx)
		if err != nil {
			return nil, nil, errors.Wrap(err, "--gate-url")
		}
		p.admit, closeGate = admit, closeAdmit
	}

	// Keep the version reads of a store that hides packs.
	if hider, ok := store.(block_store_s3.ObjectHider); ok {
		return pacedHider{pacedStore: p, hider: hider}, closeGate, nil
	}
	return p, closeGate, nil
}

// dialPacedStore reaches the store, paced as the flags say.
func (a *Args) dialPacedStore(ctx context.Context) (block_store_s3.ObjectStore, string, func(), error) {
	// Reach the store.
	objects, bucket, closeStore, err := a.dialStore()
	if err != nil {
		return nil, "", nil, err
	}

	// Pace its reads.
	paced, closePace, err := a.pace(ctx, objects)
	if err != nil {
		closeStore()
		return nil, "", nil, err
	}
	return paced, bucket, func() {
		closePace()
		closeStore()
	}, nil
}

// gate returns the wait on the backend's gate, through its admin service.
func (a *Args) gate(ctx context.Context) (func(ctx context.Context) error, func(), error) {
	// Use a test's gate.
	if a.dialGate != nil {
		return a.dialGate(ctx)
	}

	// Dial the service and wait on its Admit call.
	client, closeClient, err := dialAdminURL(ctx, a.GateURL)
	if err != nil {
		return nil, nil, err
	}
	return func(ctx context.Context) error {
		_, err := client.Admit(ctx, &block_packsweep_admin.AdmitRequest{})
		return err
	}, closeClient, nil
}

// pacedStore paces a store's reads: each waits on the backend's gate, at
// most every admitEvery, and the bytes read take from the rate cap.
// Listings and writes pass through.
//
// Its reads add no wait to a request: the gate's admission is the backend's
// own background passes' wait, which takes no lock a request holds, and the
// cap bounds the share of the network and disk the step takes.
type pacedStore struct {
	block_store_s3.ObjectStore
	// limit is the rate cap, nil for none.
	limit *rate.Limiter
	// admit waits on the backend's gate, nil for none.
	admit func(ctx context.Context) error

	// mtx guards admitted, and is held across an admission, so concurrent
	// reads wait on one call.
	mtx sync.Mutex
	// admitted is when the gate last admitted.
	admitted time.Time
}

// wait waits on the gate before a read, at most every admitEvery.
func (p *pacedStore) wait(ctx context.Context) error {
	// Pass while the last admission covers the read.
	if p.admit == nil {
		return nil
	}
	p.mtx.Lock()
	defer p.mtx.Unlock()
	if time.Since(p.admitted) < admitEvery {
		return nil
	}

	// Wait for the gate.
	if err := p.admit(ctx); err != nil {
		return errors.Wrap(err, "backend gate")
	}
	p.admitted = time.Now()
	return nil
}

// take takes n bytes from the rate cap, a burst at a time.
func (p *pacedStore) take(ctx context.Context, n int) error {
	if p.limit == nil {
		return nil
	}
	for n > 0 {
		step := min(n, p.limit.Burst())
		if err := p.limit.WaitN(ctx, step); err != nil {
			return err
		}
		n -= step
	}
	return nil
}

// GetObject waits on the gate, then opens the object with its body paced.
func (p *pacedStore) GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error) {
	if err := p.wait(ctx); err != nil {
		return nil, err
	}
	body, err := p.ObjectStore.GetObject(ctx, bucket, key)
	if err != nil || p.limit == nil {
		return body, err
	}
	return &pacedBody{ReadCloser: body, ctx: ctx, p: p}, nil
}

// GetObjectRange waits on the gate, reads the range, and takes its bytes
// from the cap.
func (p *pacedStore) GetObjectRange(ctx context.Context, bucket, key string, off int64, length int) ([]byte, error) {
	if err := p.wait(ctx); err != nil {
		return nil, err
	}
	data, err := p.ObjectStore.GetObjectRange(ctx, bucket, key, off, length)
	if err != nil {
		return nil, err
	}
	return data, p.take(ctx, len(data))
}

// Conns reports the store's connections, as the pack store sizes its entry
// reads by them, or zero when it does not say.
func (p *pacedStore) Conns() int {
	if w, ok := p.ObjectStore.(interface{ Conns() int }); ok {
		return w.Conns()
	}
	return 0
}

// pacedBody takes each read's bytes from the cap.
type pacedBody struct {
	io.ReadCloser
	// ctx bounds the waits.
	ctx context.Context
	// p is the store.
	p *pacedStore
}

// Read reads, then waits for the bytes read.
func (b *pacedBody) Read(buf []byte) (int, error) {
	n, err := b.ReadCloser.Read(buf)
	if werr := b.p.take(b.ctx, n); werr != nil && err == nil {
		err = werr
	}
	return n, err
}

// pacedHider is a pacedStore over a store that hides packs, keeping its
// hidden reads, paced, and its hides.
type pacedHider struct {
	*pacedStore
	// hider is the store.
	hider block_store_s3.ObjectHider
}

// HideObjectVersion hides the object, unpaced.
func (h pacedHider) HideObjectVersion(ctx context.Context, bucket, key string) (block_store_s3.HiddenObject, error) {
	return h.hider.HideObjectVersion(ctx, bucket, key)
}

// UnhideObjectVersion restores the version, unpaced.
func (h pacedHider) UnhideObjectVersion(ctx context.Context, bucket, key, version string) error {
	return h.hider.UnhideObjectVersion(ctx, bucket, key, version)
}

// HiddenObjectVersion returns the hidden object's version, unpaced.
func (h pacedHider) HiddenObjectVersion(ctx context.Context, bucket, key string) (string, error) {
	return h.hider.HiddenObjectVersion(ctx, bucket, key)
}

// GetObjectVersionRange waits on the gate, reads the version's range, and
// takes its bytes from the cap.
func (h pacedHider) GetObjectVersionRange(ctx context.Context, bucket, key, version string, off int64, length int) ([]byte, error) {
	if err := h.wait(ctx); err != nil {
		return nil, err
	}
	data, err := h.hider.GetObjectVersionRange(ctx, bucket, key, version, off, length)
	if err != nil {
		return nil, err
	}
	return data, h.take(ctx, len(data))
}

// Compile-time interface assertions.
var (
	_ block_store_s3.ObjectStore = (*pacedStore)(nil)
	_ block_store_s3.ObjectHider = pacedHider{}
)

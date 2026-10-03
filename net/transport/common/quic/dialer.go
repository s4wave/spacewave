package transport_quic

import (
	"context"

	"github.com/aperturerobotics/util/promise"
	"github.com/s4wave/spacewave/net/peer"
)

// Dialer represents an ongoing attempt to dial an address.
type Dialer struct {
	// t is the transport
	t *Transport
	// rootCtx is the root context
	rootCtx context.Context
	// ctx is the dialer ctx
	ctx context.Context
	// ctxCancel cancels ctx
	// called when execute() exits
	ctxCancel context.CancelFunc
	// peerID is the peer id
	// can be empty to indicate any peer
	peerID peer.ID
	// addr is the address
	addr string
	// result is the result promise
	result *promise.Promise[*Link]
}

// NewDialer constructs a new dialer.
func NewDialer(
	rctx context.Context,
	t *Transport,
	peerID peer.ID,
	addr string,
) (*Dialer, error) {
	d := &Dialer{
		t:       t,
		rootCtx: rctx,
		peerID:  peerID,
		addr:    addr,
		result:  promise.NewPromise[*Link](),
	}
	d.ctx, d.ctxCancel = context.WithCancel(rctx) //nolint:gosec // cancel stored on Dialer and called by Execute
	return d, nil
}

// Execute executes the dialer, yielding a Link.
func (d *Dialer) Execute() {
	// Retire the address dialer and its context when execution ends.
	ctx := d.ctx
	defer d.ctxCancel()
	defer func() {
		d.t.mtx.Lock()
		if odl, odlOk := d.t.dialers[d.addr]; odlOk && odl == d {
			delete(d.t.dialers, d.addr)
		}
		d.t.mtx.Unlock()
	}()

	// Dial the remote QUIC address with its peer identity in the log.
	le := d.t.le.WithField("remote-addr", d.addr)
	if d.peerID != "" {
		le = le.WithField("remote-peer", d.peerID.String())
	}
	le.Debug("quic: dialing peer")
	rconn, _, err := d.t.dialFn(ctx, d.addr)
	if err != nil {
		le.WithError(err).Warn("quic: failed to dial peer")
		d.result.SetResult(nil, err)
		return
	}

	// Publish the link created from the dialed QUIC session.
	d.result.SetResult(d.t.HandleSession(ctx, rconn))
}

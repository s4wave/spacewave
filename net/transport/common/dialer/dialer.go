package dialer

import (
	"context"
	"errors"
	"time"

	bo "github.com/aperturerobotics/util/backoff/cbackoff"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/sirupsen/logrus"
)

// Dialer manages a transport dialer.
type Dialer struct {
	// le is the logger
	le *logrus.Entry
	// tptDialer is the transport dialer.
	tptDialer TransportDialer
	// backoff is the dialer backoff
	backoff bo.BackOff
	// peerID is the peer id
	peerID peer.ID
	// address is the dial address
	address string
}

// NewDialer constructs a new Dialer
func NewDialer(
	le *logrus.Entry,
	tptDialer TransportDialer,
	opts *DialerOpts,
	peerID peer.ID,
	address string,
) *Dialer {
	return &Dialer{
		le:        le.WithFields(logrus.Fields{"dial-peer-id": peerID.String(), "dial-peer-addr": address}),
		tptDialer: tptDialer,
		backoff:   opts.GetBackoff().Construct(),
		peerID:    peerID,
		address:   address,
	}
}

// GetLogger returns the dialer logger.
func (d *Dialer) GetLogger() *logrus.Entry {
	return d.le
}

// Execute executes the dialer, with backoff.
func (d *Dialer) Execute(ctx context.Context) (link.Link, error) {
	for {
		d.le.Debug("attempting to dial peer")
		lnk, fatal, err := d.tptDialer.DialPeer(ctx, d.peerID, d.address)
		if err == nil {
			d.backoff.Reset()
			return lnk, nil
		}
		if ctx.Err() != nil {
			return nil, context.Canceled
		}

		bo := d.backoff.NextBackOff()
		if fatal {
			d.le.WithError(err).Warn("dialer errored fatally")
			return nil, err
		}

		d.le.
			WithError(err).
			WithField("backoff", bo.String()).
			Warn("dialer errored")

		if bo == -1 {
			return nil, errors.New("dial backoff max duration exceeded")
		}

		select {
		case <-ctx.Done():
			return nil, context.Canceled
		case <-time.After(bo):
		}
	}
}

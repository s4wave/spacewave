package transport_controller

import (
	"context"
	"io"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/sirupsen/logrus"
)

// establishedLink holds state for an established link.
type establishedLink struct {
	// le is the log entry
	le *logrus.Entry
	// c is the transport controller
	c *Controller
	// lnk is the link.
	lnk link.Link
	// mlnk is the mounted link
	mlnk link.MountedLink
	// tpt is the transport
	tpt transport.Transport
	// di is the directive instance
	di directive.Instance
	// cancel closes any goroutines related to the link
	cancel context.CancelFunc
}

// newEstablishedLink constructs the new EstablishedLink object.
// The EstablishLink directive is fulfilled on the controller bus.
func newEstablishedLink(
	le *logrus.Entry,
	rctx context.Context,
	b bus.Bus,
	lnk link.Link,
	mlnk link.MountedLink,
	tpt transport.Transport,
	ctrl *Controller,
) (*establishedLink, error) {
	// Construct EstablishLink directive.
	// The controller will match the directive to this link.
	// Note: the directive has an UnrefDisposeDur assigned: minimum hold-open time.
	di, dir, err := b.AddDirective(
		link.NewEstablishLinkWithPeer(lnk.GetLocalPeer(), lnk.GetRemotePeer()),
		nil,
	)
	if err != nil {
		return nil, err
	}

	// Start the close goroutine
	ctx, ctxCancel := context.WithCancel(rctx)
	el := &establishedLink{
		le:     le.WithField("peer-id", lnk.GetRemotePeer().String()),
		lnk:    lnk,
		mlnk:   mlnk,
		tpt:    tpt,
		di:     di,
		cancel: ctxCancel,
		c:      ctrl,
	}
	di.AddDisposeCallback(func() {
		go lnk.Close()
		ctxCancel()
	})
	go el.acceptStreamPump(ctx)

	// Remove the directive instance immediately.
	//
	// The directive will enter the UnrefDisposeDur hold-open timer if there
	// were no other references, and AddDisposeCallback above will be called
	// only when the directive is removed after the dispose timeout.
	dir.Release()

	return el, nil
}

func (e *establishedLink) acceptStreamPump(ctx context.Context) {
	// accept streams
	lnk, ctrl := e.lnk, e.c
	defer func() {
		// close the directive instance early if there are no non-weak refs.
		// this skips the hold-open (unref dispose dur) timer.
		_ = e.di.CloseIfUnreferenced(false)
		e.cancel()
		lnk.Close()
	}()

	for {
		strm, strmOpts, err := lnk.AcceptStream()
		if err != nil {
			if err != context.Canceled && err != io.EOF {
				select {
				case <-ctx.Done():
					// don't log if the context was canceled
				default:
					e.le.WithError(err).Warn("link accept stream errored")
				}
			}
			return
		}

		if strm != nil {
			go ctrl.HandleIncomingStream(ctx, e.tpt, lnk, strm, strmOpts)
		}
	}
}

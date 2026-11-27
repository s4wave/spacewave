package transport_controller

import (
	"context"
	"io"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/stream"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/sirupsen/logrus"
)

// registeredLink is an immutable index entry for a mounted link.
// The session owns stream dispatch and the directive demand lifetime.
type registeredLink struct {
	// lnk supplies connection identity to the controller's indexes.
	lnk link.Link
	// mlnk supplies protocol stream operations to link demand resolvers.
	mlnk link.MountedLink
	// session runs accepted streams and holds their link demand.
	session *establishedLink
}

// establishedLink accepts streams while its link demand remains live.
type establishedLink struct {
	// le is the log entry.
	le *logrus.Entry
	// handleIncomingStream dispatches each accepted stream.
	handleIncomingStream func(context.Context, transport.Transport, link.Link, stream.Stream, stream.OpenOpts)
	// lnk is the link.
	lnk link.Link
	// tpt is the transport.
	tpt transport.Transport
	// di is the directive instance.
	di directive.Instance
	// cancel stops the goroutines of the link and drops its directive dispose
	// callback. It is safe to call more than once.
	cancel func()
}

// newEstablishedLink constructs the new EstablishedLink object.
// The EstablishLink directive is fulfilled on the controller bus.
func newEstablishedLink(
	le *logrus.Entry,
	rctx context.Context,
	b bus.Bus,
	lnk link.Link,
	tpt transport.Transport,
	handleIncomingStream func(context.Context, transport.Transport, link.Link, stream.Stream, stream.OpenOpts),
) (*establishedLink, error) {
	// Construct an EstablishLink directive.
	// The controller will match the directive to this link.
	// Note: the directive has an UnrefDisposeDur assigned: minimum hold-open time.
	di, dir, err := b.AddDirective(
		rctx,
		link.NewEstablishLinkWithPeer(lnk.GetLocalPeer(), lnk.GetRemotePeer()),
		nil,
	)
	if err != nil {
		return nil, err
	}

	// Close the link when the directive is disposed. The directive outlives the
	// link while anyone holds a link to the peer, so cancel drops the callback:
	// a callback left registered keeps the closed link and its connection
	// reachable for as long as the directive lives.
	ctx, ctxCancel := context.WithCancel(rctx)
	relDispose := di.AddDisposeCallback(func() {
		go lnk.Close()
		ctxCancel()
	})
	el := &establishedLink{
		le:  le.WithField("peer-id", lnk.GetRemotePeer().String()),
		lnk: lnk,
		tpt: tpt,
		di:  di,
		cancel: func() {
			relDispose()
			ctxCancel()
		},
		handleIncomingStream: handleIncomingStream,
	}
	go el.acceptStreamPump(ctx)

	// Remove the directive instance immediately.
	//
	// The directive will enter the UnrefDisposeDur hold-open timer if there
	// were no other references, and AddDisposeCallback above will be called
	// only when the directive is removed after the dispose timeout.
	dir.Release()

	return el, nil
}

// acceptStreamPump accepts streams on the established link until it closes,
// dispatching each incoming stream to the controller. It releases the link
// when the accept loop exits.
func (e *establishedLink) acceptStreamPump(ctx context.Context) {
	// accept streams
	lnk := e.lnk
	defer func() {
		// close the directive instance early if there are no non-weak refs.
		// Closing the directive skips the hold-open (unref dispose dur)
		// timer.
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
			go e.handleIncomingStream(ctx, e.tpt, lnk, strm, strmOpts)
		}
	}
}

// releaseDemand stops accepted streams and releases unreferenced peer demand.
// hasNextLink keeps the demand alive while the controller replaces a link.
func (e *establishedLink) releaseDemand(hasNextLink bool) {
	// Cancel the stream context and release its disposal callback.
	e.cancel()

	// End unused demand immediately when this peer has no replacement link.
	if !hasNextLink {
		_ = e.di.CloseIfUnreferenced(false)
	}
}

// closeLinkAsync closes the physical link outside the controller's index lock.
func (e *establishedLink) closeLinkAsync() {
	go func() {
		_ = e.lnk.Close()
	}()
}

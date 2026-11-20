//go:build !js

package camera

import (
	"context"
	"errors"
	"io"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/core/frametrack"
	"github.com/s4wave/spacewave/core/testpattern"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/link"
	"github.com/sirupsen/logrus"
)

// server serves a shared camera track for one controller execution.
type server struct {
	// ctx ends every control stream when the camera stops.
	ctx context.Context
	// le reports failures on incoming streams.
	le *logrus.Entry
	// b is the Session bus that owns the incoming links.
	b bus.Bus
	// track fans one source out to admitted viewers.
	track *frametrack.Track
}

// HandleDirective offers the camera handler only for its control protocol.
func (s *server) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(link.HandleMountedStream)
	if !ok || dir.HandleMountedStreamProtocolID() != ControlProtocolID {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]link.MountedStreamHandler{s}), nil)
}

// HandleMountedStream serves the viewer asynchronously on the mounted link.
func (s *server) HandleMountedStream(ctx context.Context, ms link.MountedStream) error {
	go s.serve(ctx, ms)
	return nil
}

// serve admits one peer and holds its subscription until the control stream ends.
func (s *server) serve(linkCtx context.Context, ms link.MountedStream) {
	// End stream work when either the camera or the link stops.
	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()
	stopOnLinkEnd := context.AfterFunc(linkCtx, cancel)
	defer stopOnLinkEnd()

	// Refuse peers outside the Device's account before reading their requests.
	control, err := transport.AuthorizeAccountSessionPeer(ctx, s.b, ms)
	stopClose := context.AfterFunc(ctx, func() { _ = control.Close() })
	defer func() {
		if stopClose() {
			_ = control.Close()
		}
	}()
	if err != nil {
		if ctx.Err() == nil {
			s.le.WithError(err).WithField("remote-peer", ms.GetPeerID()).Warn("camera peer refused")
		}
		return
	}

	// Decode the subscription and retain the link used for its group streams.
	subscription, err := frametrack.ReadSubscription(control)
	if err != nil {
		return
	}
	_, ref, err := s.b.AddDirective(ctx, link.NewEstablishLinkWithPeer(ms.GetLink().GetLocalPeer(), ms.GetPeerID()), nil)
	if err != nil {
		return
	}
	defer ref.Release()

	// Each JPEG is its own group, so the sender window holds one frame.
	release, err := s.track.Subscribe(ctx, ms, subscription, 1)
	if err != nil {
		return
	}
	defer release()

	// This control stream carries one request. Extra input and EOF both end it.
	var extra [1]byte
	_, _ = io.ReadFull(control, extra[:])
}

// serveTestPattern runs one source and publishes its handler for that lifetime.
func serveTestPattern(ctx context.Context, le *logrus.Entry, b bus.Bus) error {
	// Bind the source and control streams to this controller execution.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	source := testpattern.NewJPEGSource()
	s := &server{ctx: ctx, le: le, b: b, track: frametrack.NewTrack(source)}

	// Publish the handler while the track runs and withdraw it on return.
	release, err := b.AddHandler(s)
	if err != nil {
		return errors.Join(err, source.Close())
	}
	defer release()
	return s.track.Run(ctx)
}

// _ is a type assertion.
var (
	_ directive.Handler         = (*server)(nil)
	_ link.MountedStreamHandler = (*server)(nil)
)

//go:build !js

// Package frametrack_server serves frame tracks on a Device Session.
package frametrack_server

import (
	"context"
	"errors"
	"io"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/core/frametrack"
	"github.com/s4wave/spacewave/core/transport"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/sirupsen/logrus"
)

// Server serves a shared frame track for one controller execution.
type Server struct {
	// ctx ends every control stream when the source stops.
	ctx context.Context
	// le reports failures on incoming streams.
	le *logrus.Entry
	// b is the Session bus that owns the incoming links.
	b bus.Bus
	// track fans one source out to admitted viewers.
	track *frametrack.Track
	// protocol identifies this track's control stream.
	protocol protocol.ID
	// window bounds each viewer's pending frames, including the active write.
	window int
	// input receives input from live subscriptions. Nil makes the track view-only.
	input func(*frametrack.Input)
}

// Serve runs the track and publishes its handler on the Device's Session bus.
// It takes ownership of source. Input may be called concurrently by viewers.
func Serve(
	ctx context.Context,
	le *logrus.Entry,
	b bus.Bus,
	id protocol.ID,
	source frametrack.Source,
	window int,
	input func(*frametrack.Input),
) error {
	// Bind the source and control streams to this controller execution.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	s := &Server{
		ctx: ctx, le: le, b: b, track: frametrack.NewTrack(source),
		protocol: id, window: window, input: input,
	}

	// Publish the handler while the track runs and withdraw it on return.
	release, err := b.AddHandler(s)
	if err != nil {
		return errors.Join(err, source.Close())
	}
	defer release()
	return s.track.Run(ctx)
}

// HandleDirective offers the track handler only for its control protocol.
func (s *Server) HandleDirective(_ context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir, ok := di.GetDirective().(link.HandleMountedStream)
	if !ok || dir.HandleMountedStreamProtocolID() != s.protocol {
		return nil, nil
	}
	return directive.R(directive.NewValueResolver([]link.MountedStreamHandler{s}), nil)
}

// HandleMountedStream serves the viewer asynchronously on the mounted link.
func (s *Server) HandleMountedStream(ctx context.Context, ms link.MountedStream) error {
	// The subscription and its ordered input share a reliable stream.
	if ms.GetOpenOpts().Unreliable {
		return errors.New("frame control requires a reliable stream")
	}

	// Leave mounted stream dispatch free to admit more viewers.
	go s.serve(ctx, ms)
	return nil
}

// serve admits one peer and holds its subscription until the control stream ends.
func (s *Server) serve(linkCtx context.Context, ms link.MountedStream) {
	// End stream work when either the source or the link stops.
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
			s.le.WithError(err).WithField("remote-peer", ms.GetPeerID()).Warn("frame track peer refused")
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

	// Start sending groups only after the peer and request are admitted.
	release, err := s.track.Subscribe(ctx, ms, subscription, s.window)
	if err != nil {
		return
	}
	defer release()

	// View-only subscriptions end on extra data or EOF.
	if s.input == nil {
		var extra [1]byte
		_, _ = io.ReadFull(control, extra[:])
		return
	}

	// Apply only complete input events from the live subscription.
	for {
		input, err := frametrack.ReadInput(control)
		if err != nil || ctx.Err() != nil {
			return
		}
		s.input(input)
	}
}

// _ is a type assertion.
var (
	_ directive.Handler         = (*Server)(nil)
	_ link.MountedStreamHandler = (*Server)(nil)
)

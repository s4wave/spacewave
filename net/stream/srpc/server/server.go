package stream_srpc_server

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/starpc/srpc"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/sirupsen/logrus"
)

// RegisterFn is a callback to register apis to a mux.
type RegisterFn func(mux srpc.Mux) error

// Server handles incoming streams for a peer id.
type Server struct {
	// b is the bus
	b bus.Bus
	// le is the logger
	le *logrus.Entry
	// info is the controller info
	info *controller.Info
	// protocolIDs is list of protocol id to listen on.
	// cannot be empty
	protocolIDs []protocol.ID
	// peerIDs is the list of local peer ids to listen on
	// if empty, allows any
	peerIDs []string
	// disableEstablishLink disables adding an establish link directive
	disableEstablishLink bool

	// mux is the srpc mux
	mux srpc.Mux
	// server is the srpc server
	server *srpc.Server
}

// NewServer constructs a common srpc controller.
// If peerIDs and/or domainIDs are empty, matches any.
func NewServer(
	b bus.Bus,
	le *logrus.Entry,
	info *controller.Info,
	registerFns []RegisterFn,
	protocolIDs []protocol.ID,
	peerIDs []string,
	disableEstablishLink bool,
) (*Server, error) {
	mux := srpc.NewMux()
	for _, rf := range registerFns {
		if err := rf(mux); err != nil {
			return nil, err
		}
	}

	server := srpc.NewServer(mux)
	return &Server{
		b:           b,
		le:          le,
		info:        info,
		protocolIDs: protocolIDs,
		peerIDs:     peerIDs,

		mux:                  mux,
		server:               server,
		disableEstablishLink: disableEstablishLink,
	}, nil
}

// NewServerWithMux constructs a server with a pre-built mux. Use this when
// the mux needs a fallback invoker or custom setup that RegisterFn cannot
// express.
func NewServerWithMux(
	b bus.Bus,
	le *logrus.Entry,
	info *controller.Info,
	mux srpc.Mux,
	protocolIDs []protocol.ID,
	peerIDs []string,
	disableEstablishLink bool,
) *Server {
	return &Server{
		b:                    b,
		le:                   le,
		info:                 info,
		protocolIDs:          protocolIDs,
		peerIDs:              peerIDs,
		mux:                  mux,
		server:               srpc.NewServer(mux),
		disableEstablishLink: disableEstablishLink,
	}
}

// GetControllerInfo returns information about the controller.
func (s *Server) GetControllerInfo() *controller.Info {
	return s.info
}

// Execute executes the controller goroutine.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (s *Server) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
func (s *Server) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case link.HandleMountedStream:
		return s.ResolveHandleMountedStream(ctx, di, d)
	}

	return nil, nil
}

// ResolveHandleMountedStream resolves a HandleMountedStream directive.
func (s *Server) ResolveHandleMountedStream(
	ctx context.Context,
	di directive.Instance,
	dir link.HandleMountedStream,
) ([]directive.Resolver, error) {
	inProtocol := dir.HandleMountedStreamProtocolID()
	var match bool
	if slices.Contains(s.protocolIDs, inProtocol) {
		match = true
	}
	if !match {
		return nil, nil
	}

	inPeerID := dir.HandleMountedStreamLocalPeerID()
	inPeerIDString := inPeerID.String()
	if len(s.peerIDs) != 0 {
		match = false
		for _, pid := range s.peerIDs {
			if pid == inPeerIDString {
				match = true
			}
		}
		if !match {
			return nil, nil
		}
	}

	return directive.Resolvers(newMountedStreamResolver(s)), nil
}

// HandleMountedStream handles an incoming mounted stream.
// Any returned error indicates the stream should be closed.
// This function should return as soon as possible, and start
// additional goroutines to manage the lifecycle of the stream.
// Typically EstablishLink is asserted in HandleMountedStream.
func (s *Server) HandleMountedStream(ctx context.Context, ms link.MountedStream) error {
	// keep the link open
	var elRef directive.Reference
	var err error
	if !s.disableEstablishLink {
		_, elRef, err = s.b.AddDirective(
			link.NewEstablishLinkWithPeer(ms.GetLink().GetLocalPeer(), ms.GetPeerID()),
			nil,
		)
		if err != nil {
			return err
		}
	}
	go func() {
		strm := ms.GetStream()
		sctx := link.WithMountedStreamContext(ctx, ms)
		s.server.HandleStream(sctx, strm)
		strm.Close()
		if elRef != nil {
			elRef.Release()
		}
	}()
	return nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (s *Server) Close() error {
	return nil
}

// _ is a type assertion
var (
	_ link.MountedStreamHandler = ((*Server)(nil))
	_ controller.Controller     = ((*Server)(nil))
)

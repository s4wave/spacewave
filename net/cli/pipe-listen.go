package cli

import (
	"context"
	"os"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/blang/semver/v4"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
)

// runListen runs the pipe command in listen/server mode.
func (a *PipeArgs) runListen(ctx context.Context) error {
	// Setup daemon with UDP transport
	d, cleanup, err := a.setupDaemon(ctx, a.ListenAddr, nil)
	if err != nil {
		return err
	}
	defer cleanup()

	b := d.GetControllerBus()
	peerID := d.GetNodePeerID()

	// Print peer ID to stderr for client to use
	a.logStatus("Peer ID: %s", peerID.String())
	a.logStatus("Listening on %s", a.ListenAddr)

	// Create stream channel for accepting incoming streams
	streamCh := make(chan link.MountedStream, 1)

	// Create and register the accept handler controller
	acceptHandler := newPipeAcceptController(
		b,
		protocol.ID(a.ProtocolID),
		peerID,
		streamCh,
	)

	// Add the handler to the bus
	_, err = b.AddController(ctx, acceptHandler, nil)
	if err != nil {
		return errors.Wrap(err, "add accept handler")
	}

	// Wait for incoming stream
	select {
	case <-ctx.Done():
		return ctx.Err()
	case mstrm := <-streamCh:
		a.logStatus("Accepted connection from %s", mstrm.GetPeerID().String())

		// Pipe stream to stdin/stdout
		return pipeStream(mstrm.GetStream(), os.Stdin, os.Stdout)
	}
}

// pipeAcceptController handles incoming streams for the pipe command.
type pipeAcceptController struct {
	bus        bus.Bus
	protocolID protocol.ID
	localPeer  peer.ID
	streamCh   chan<- link.MountedStream
}

// pipeAcceptControllerID is the controller ID.
const pipeAcceptControllerID = "bifrost/pipe/accept"

// pipeAcceptControllerVersion is the controller version.
var pipeAcceptControllerVersion = semver.MustParse("0.0.1")

// newPipeAcceptController creates a new pipe accept controller.
func newPipeAcceptController(
	b bus.Bus,
	protocolID protocol.ID,
	localPeer peer.ID,
	streamCh chan<- link.MountedStream,
) *pipeAcceptController {
	return &pipeAcceptController{
		bus:        b,
		protocolID: protocolID,
		localPeer:  localPeer,
		streamCh:   streamCh,
	}
}

// GetControllerInfo returns information about the controller.
func (c *pipeAcceptController) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		pipeAcceptControllerID,
		pipeAcceptControllerVersion,
		"pipe accept controller",
	)
}

// Execute executes the controller.
func (c *pipeAcceptController) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective handles directives.
func (c *pipeAcceptController) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	hms, ok := dir.(link.HandleMountedStream)
	if !ok {
		return nil, nil
	}

	// Check if protocol matches
	if hms.HandleMountedStreamProtocolID() != c.protocolID {
		return nil, nil
	}

	// Check if local peer matches (if set)
	if c.localPeer != peer.ID("") {
		if hms.HandleMountedStreamLocalPeerID() != c.localPeer {
			return nil, nil
		}
	}

	// Return resolver that provides our stream handler
	return directive.Resolvers(&pipeStreamResolver{
		streamCh: c.streamCh,
	}), nil
}

// Close closes the controller.
func (c *pipeAcceptController) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*pipeAcceptController)(nil)

// pipeStreamResolver resolves HandleMountedStream directives.
type pipeStreamResolver struct {
	streamCh chan<- link.MountedStream
}

// Resolve resolves the directive.
func (r *pipeStreamResolver) Resolve(
	ctx context.Context,
	handler directive.ResolverHandler,
) error {
	handler.AddValue(link.MountedStreamHandler(&pipeStreamHandler{
		streamCh: r.streamCh,
	}))
	return nil
}

// _ is a type assertion
var _ directive.Resolver = (*pipeStreamResolver)(nil)

// pipeStreamHandler handles incoming mounted streams.
type pipeStreamHandler struct {
	streamCh chan<- link.MountedStream
}

// HandleMountedStream handles an incoming stream.
func (h *pipeStreamHandler) HandleMountedStream(
	ctx context.Context,
	ms link.MountedStream,
) error {
	select {
	case h.streamCh <- ms:
		return nil
	default:
		// Channel full, reject stream
		return errors.New("already handling a stream")
	}
}

// _ is a type assertion
var _ link.MountedStreamHandler = (*pipeStreamHandler)(nil)

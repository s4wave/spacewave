package stream_listening

import (
	"context"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	ma "github.com/aperturerobotics/go-multiaddr"
	manet "github.com/aperturerobotics/go-multiaddr/net"
	"github.com/aperturerobotics/util/ioproxy"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	"github.com/sirupsen/logrus"
)

// Controller implements the listening controller. The controller listens on a
// multiaddress and forwards incoming connections as streams to the target peer
// with the configured protocol ID attached.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// conf is the config
	conf *Config
	// bus is the controller bus
	bus bus.Bus

	// listenMa is the listen multiaddr
	listenMa ma.Multiaddr
	// localPeerID is the local peer id
	localPeerID peer.ID
	// remotePeerID is the remote peer id
	remotePeerID peer.ID
	// protocolID is the protocol ID to use
	protocolID protocol.ID
}

// NewController constructs a new listening controller.
func NewController(
	le *logrus.Entry,
	conf *Config,
	bus bus.Bus,
) (*Controller, error) {
	listenMa, err := conf.ParseListenMultiaddr()
	if err != nil {
		return nil, err
	}

	localPeerID, err := conf.ParseLocalPeerID()
	if err != nil {
		return nil, err
	}

	remotePeerID, err := conf.ParseRemotePeerID()
	if err != nil {
		return nil, err
	}

	pid := protocol.ID(conf.GetProtocolId())
	if err := pid.Validate(); err != nil {
		return nil, err
	}

	return &Controller{
		le:       le.WithField("listen-addr", listenMa.String()),
		conf:     conf,
		bus:      bus,
		listenMa: listenMa,

		localPeerID:  localPeerID,
		remotePeerID: remotePeerID,
		protocolID:   pid,
	}, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"listening controller",
	)
}

// Execute executes the listening controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	le := c.le

	// Listen on the multiaddress
	listener, err := manet.Listen(c.listenMa)
	if err != nil {
		return err
	}
	defer listener.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- c.acceptPump(ctx, listener)
	}()

	le.Debug("listening on multiaddr")
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	}
}

// acceptPump accepts incoming connections.
func (c *Controller) acceptPump(ctx context.Context, listener manet.Listener) error {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return err
		}

		c.le.Debug("accepted connection")
		go c.handleConn(ctx, conn)
	}
}

// openStreamTimeout is the amount of time to wait for the stream to be opened.
var openStreamTimeout = time.Second * 10

// handleConn handles a connection.
func (c *Controller) handleConn(ctx context.Context, conn manet.Conn) {
	openCtx, openCtxCancel := context.WithTimeout(ctx, openStreamTimeout)
	defer openCtxCancel()

	opts := stream.OpenOpts{}
	mstrm, rel, err := link.OpenStreamWithPeerEx(
		openCtx,
		c.bus,
		c.protocolID,
		c.localPeerID,
		c.remotePeerID,
		c.conf.GetTransportId(),
		opts,
	)
	if err != nil {
		conn.Close()
		// c.le.WithError(err).Warn("unable to open stream to handle conn")
		return
	}

	strm := mstrm.GetStream()
	ioproxy.ProxyStreams(conn, strm, func() {
		rel()
		conn.Close()
	})
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))

package stream_api_accept

import (
	"context"
	"slices"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	stream_api_rpc "github.com/s4wave/spacewave/net/stream/api/rpc"
	"github.com/sirupsen/logrus"
)

// Controller accepts HandleMountedStream via waiting RPC calls and streams data
// over the request and response streams.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// conf is the config
	conf *Config
	// bus is the controller bus
	bus bus.Bus

	// localPeerID is the local peer id
	localPeerID peer.ID
	// remotePeerIDs are the acceptable remote peer id
	remotePeerIDs []peer.ID
	// protocolID is the protocol ID to use
	protocolID protocol.ID

	// rpcCh is the rpc channel
	rpcCh chan *queuedRPC
}

// NewController constructs a new accept controller.
func NewController(
	le *logrus.Entry,
	conf *Config,
	bus bus.Bus,
) (*Controller, error) {
	localPeerID, err := conf.ParseLocalPeerID()
	if err != nil {
		return nil, err
	}

	var remotePeerIDs []peer.ID
	for _, pid := range conf.GetRemotePeerIds() {
		pi, err := peer.IDB58Decode(pid)
		if err != nil {
			return nil, err
		}

		remotePeerIDs = append(remotePeerIDs, pi)
	}

	pid := protocol.ID(conf.GetProtocolId())
	if err := pid.Validate(); err != nil {
		return nil, err
	}

	return &Controller{
		le:    le,
		bus:   bus,
		conf:  conf,
		rpcCh: make(chan *queuedRPC),

		localPeerID:   localPeerID,
		remotePeerIDs: remotePeerIDs,
		protocolID:    pid,
	}, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"accept controller",
	)
}

// Execute executes the accept controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	// HandleMountedStream handler.
	if d, ok := dir.(link.HandleMountedStream); ok {
		return c.resolveHandleMountedStream(ctx, di, d)
	}

	return nil, nil
}

// queuedRPC is a queued RPC
type queuedRPC struct {
	rpc    stream_api_rpc.RPC
	doneCb func(err error)
}

// AttachRPC attaches a RPC call to the controller.
func (c *Controller) AttachRPC(rpc stream_api_rpc.RPC) error {
	ctx := rpc.Context()
	errCh := make(chan error, 1)

	if err := rpc.Send(&stream_api_rpc.Data{
		State: stream_api_rpc.StreamState_StreamState_ESTABLISHING,
	}); err != nil {
		return err
	}

	select {
	case <-ctx.Done():
		return ctx.Err()
	case c.rpcCh <- &queuedRPC{
		rpc: rpc,
		doneCb: func(err error) {
			select {
			case errCh <- err:
			default:
			}
		},
	}:
		return <-errCh
	}
}

// resolveHandleMountedStream resolves a HandleMountedStream directive by dialing a target.
func (c *Controller) resolveHandleMountedStream(
	ctx context.Context,
	di directive.Instance,
	dir link.HandleMountedStream,
) ([]directive.Resolver, error) {
	if c.protocolID != dir.HandleMountedStreamProtocolID() {
		return nil, nil
	}
	if localPeerID := c.localPeerID; localPeerID != peer.ID("") {
		if lid := dir.HandleMountedStreamLocalPeerID(); lid != localPeerID {
			c.le.Debugf(
				"incoming stream %s != filtered %s",
				lid.String(),
				localPeerID.String(),
			)
			return nil, nil
		}
	}
	if len(c.remotePeerIDs) != 0 {
		remoteID := dir.HandleMountedStreamRemotePeerID()
		var found bool
		if slices.Contains(c.remotePeerIDs, remoteID) {
			found = true
		}
		if !found {
			c.le.Debugf(
				"incoming stream %s != filtered %v",
				remoteID.String(),
				c.conf.GetRemotePeerIds(),
			)
			return nil, nil
		}
	}

	return directive.Resolvers(c), nil
}

// Resolve resolves the values, emitting them to the handler.
func (c *Controller) Resolve(ctx context.Context, handler directive.ResolverHandler) error {
	var rpc *queuedRPC
	select {
	case <-ctx.Done():
		return ctx.Err()
	case rpc = <-c.rpcCh:
	}

	h, err := NewMountedStreamHandler(c.le, c.bus, rpc)
	if err != nil {
		rpc.doneCb(err)
		return err
	}

	handler.AddValue(link.MountedStreamHandler(h))
	return nil
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))

// _ is a type assertion
var _ directive.Resolver = ((*Controller)(nil))

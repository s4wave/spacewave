package spacewave_launcher_gossip

import (
	"context"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/keyed"
	spacewave_launcher "github.com/s4wave/spacewave/core/provider/spacewave/launcher"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/protocol"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// ProtocolID is the link solicitation protocol for DistConfig gossip.
const ProtocolID = protocol.ID("spacewave/launcher/dist-config-gossip/1")

// ControllerID is the controller ID.
const ControllerID = "spacewave/launcher/gossip"

// Version is the version of this controller.
var Version = controller.MustParseVersion("0.0.1")

// Controller gossips the launcher's signed DistConfig with every peer linked
// on its bus. It solicits ProtocolID only once a launcher is reachable, so a
// process without a launcher opens no gossip streams.
type Controller struct {
	// le is the logger.
	le *logrus.Entry
	// b is the bus carrying the links and the launcher service.
	b bus.Bus
	// launcher mirrors and updates the local launcher.
	launcher *busLauncher
	// streams runs one exchange per matched stream.
	streams *keyed.Keyed[link_solicit.SolicitMountedStream, struct{}]
}

// NewController constructs a new DistConfig gossip controller.
func NewController(le *logrus.Entry, b bus.Bus) *Controller {
	c := &Controller{
		le: le,
		b:  b,
		launcher: &busLauncher{
			InfoWatcher: spacewave_launcher.NewInfoWatcher(le, b),
			b:           b,
		},
	}
	c.streams = keyed.NewKeyed(c.newStreamRoutine)
	return c
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(ControllerID, Version, "launcher dist config gossip")
}

// Execute executes the controller.
func (c *Controller) Execute(ctx context.Context) error {
	// Wait for a reachable launcher.
	c.launcher.SetContext(ctx)
	for {
		info, changed := c.launcher.Snapshot()
		if info != nil {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-changed:
		}
	}

	// Run one exchange per matched stream while its link lasts.
	c.streams.SetContext(ctx, false)
	defer c.streams.ClearContext()
	_, solicitRef, err := c.b.AddDirective(
		link_solicit.NewSolicitProtocol(ProtocolID, nil, "", 0),
		directive.NewTypedCallbackHandler(
			func(v directive.TypedAttachedValue[link_solicit.SolicitMountedStream]) {
				c.streams.SetKey(v.GetValue(), true)
			},
			func(v directive.TypedAttachedValue[link_solicit.SolicitMountedStream]) {
				c.streams.RemoveKey(v.GetValue())
			},
			nil, nil,
		),
	)
	if err != nil {
		return err
	}
	defer solicitRef.Release()

	<-ctx.Done()
	return ctx.Err()
}

// newStreamRoutine constructs the exchange routine for a matched stream.
func (c *Controller) newStreamRoutine(sms link_solicit.SolicitMountedStream) (keyed.Routine, struct{}) {
	return func(ctx context.Context) error {
		c.runStream(ctx, sms)
		return nil
	}, struct{}{}
}

// runStream claims a matched stream and runs the exchange until it ends.
func (c *Controller) runStream(ctx context.Context, sms link_solicit.SolicitMountedStream) {
	// Claim the stream unless another handler already holds it.
	ms, taken, err := sms.AcceptMountedStream()
	if err != nil || taken {
		return
	}

	// Exchange configs with the remote peer until the link or ctx ends.
	le := c.le.WithField("remote-peer", ms.GetPeerID().String())
	sess := stream_packet.NewSession(ms.GetStream(), maxMessageSize)
	err = exchange(ctx, le, sess, c.launcher)
	if err != nil && ctx.Err() == nil {
		le.WithError(err).Debug("dist config gossip ended")
	}
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(context.Context, directive.Instance) ([]directive.Resolver, error) {
	return nil, nil
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = (*Controller)(nil)

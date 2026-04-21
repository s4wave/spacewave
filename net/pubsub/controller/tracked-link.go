package pubsub_controller

import (
	"context"

	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/pubsub"
	"github.com/s4wave/spacewave/net/stream"
	"github.com/sirupsen/logrus"
)

type trackedLink struct {
	c         *Controller
	tpl       pubsub.PeerLinkTuple
	lnk       link.MountedLink
	le        *logrus.Entry
	ctxCancel context.CancelFunc
}

// trackLink executes tracking the link.
func (t *trackedLink) trackLink(ctx context.Context) error {
	// decide which side opens stream using whoever's peer id is greater
	// this is deterministic enough, as long as everyone uses the same
	// String() implementation.
	if t.lnk.GetLocalPeer().String() > t.lnk.GetRemotePeer().String() {
		return nil
	}
	t.le.Debug("link tracking starting")
	ps, err := t.c.GetPubSub(ctx)
	if err != nil {
		return err
	}

	mtStrm, err := t.lnk.OpenMountedStream(ctx, t.c.protocolID, stream.OpenOpts{})
	if err != nil {
		return err
	}

	t.le.WithField("protocol-id", mtStrm.GetProtocolID()).
		Info("pubsub stream opened (by us)")
	ps.AddPeerStream(t.tpl, true, mtStrm)
	return nil
}

package psecho

import (
	"context"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/pubsub"
	"github.com/aperturerobotics/hydra/block"
	"github.com/libp2p/go-libp2p-core/crypto"
)

// handleIncomingMessage passes an incoming message to other handlers.
func (c *Controller) handleIncomingMessage(
	ctx context.Context,
	m pubsub.Message,
	privKey crypto.PrivKey,
) {
	if !m.GetAuthenticated() || m.GetFrom().MatchesPrivateKey(privKey) {
		return
	}
	localPeer, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return
	}

	var msg PubSubMessage
	if err := (&msg).UnmarshalBlock(m.GetData()); err != nil {
		c.le.
			WithField("remote-peer-id", m.GetFrom().Pretty()).
			WithError(err).
			Warn("cannot parse message")
		return
	}

	// TODO
	from := m.GetFrom()
	msg.LogFields(c.le).
		WithField("remote-peer-id", from.Pretty()).
		Debug("received incoming pubsub message")
	if len(msg.GetWantRefs()) != 0 || msg.GetWantEmpty() {
		var checkList []*block.BlockRef
		c.mtx.Lock()
		rpeer, ok := c.remotePeers[from]
		if msg.GetWantEmpty() {
			if ok {
				delete(c.remotePeers, from)
			}
		} else {
			if !ok {
				rpeer = newRemotePeer(c, localPeer, from)
				c.remotePeers[from] = rpeer
			}
			checkList = rpeer.pushWantedRefs(msg.GetWantRefs())
		}
		c.mtx.Unlock()
		if len(checkList) != 0 {
			select {
			case <-ctx.Done():
				return
			case c.syncWantCheckCh <- &syncCheckList{peer: from, refs: checkList}:
			}
		}
	}
}

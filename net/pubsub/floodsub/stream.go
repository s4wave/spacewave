package floodsub

import (
	"context"
	"io"

	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/pubsub"
	"github.com/s4wave/spacewave/net/pubsub/util/pubmessage"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// streamHandler is a remote floodsub peer with a stream.
type streamHandler struct {
	tpl       pubsub.PeerLinkTuple
	m         *FloodSub
	initiator bool
	stream    *stream_packet.Session
	le        *logrus.Entry
	packetCh  chan *Packet
	peerID    peer.ID

	ctx       context.Context
	ctxCancel context.CancelFunc
}

// writePacket writes a packet.
func (s *streamHandler) writePacket(pkt *Packet) {
	select {
	case s.packetCh <- pkt:
	case <-s.ctx.Done():
	}
}

// executeSession executes the stream session.
func (s *streamHandler) executeSession() error {
	ctx := s.ctx
	defer s.stream.Close()
	defer s.ctxCancel()
	go s.readPump(ctx)

	// le := s.le.WithField("initiator", s.initiator)
	// le.Info("executing session")
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case pkt := <-s.packetCh:
			if err := s.stream.SendMsg(pkt); err != nil {
				return err
			}
		}
	}
}

// processPacket processes the incoming packet.
func (s *streamHandler) processPacket(msg *Packet) {
	if subs := msg.GetSubscriptions(); len(subs) != 0 {
		s.handleSubscriptions(subs)
	}
	if pubs := msg.GetPublish(); len(pubs) != 0 {
		s.handlePublish(pubs)
	}
}

// handlePublish handles incoming published packets
func (s *streamHandler) handlePublish(pkts []*peer.SignedMsg) {
	for _, pkt := range pkts {
		pktInner, _, _, err := pubmessage.ExtractAndVerify(pkt)
		if err != nil {
			s.le.WithError(err).Warnf(
				"invalid message from peer id: %q",
				pkt.GetFromPeerId(),
			)
			continue
		}
		chid := pktInner.GetChannel()
		s.m.mtx.Lock()
		_, chOk := s.m.channels[chid]
		s.m.mtx.Unlock()
		if !chOk {
			s.le.Warnf("received message for non-subscribed channel %s", chid)
			continue
		}
		s.m.handleValidMessage(s.ctx, s.peerID, pkt, pktInner)
	}
}

// handleSubscriptions processes subscription packet data
func (s *streamHandler) handleSubscriptions(subs []*SubscriptionOpts) {
	s.m.mtx.Lock()
	defer s.m.mtx.Unlock()

	for _, sub := range subs {
		chid := sub.GetChannelId()
		if chid == "" {
			continue
		}
		le := s.le.WithField("channel-id", chid)
		if sub.GetSubscribe() {
			cm, ok := s.m.peerChannels[chid]
			if !ok {
				cm = make(map[pubsub.PeerLinkTuple]struct{})
				s.m.peerChannels[chid] = cm
			}
			if _, ok := cm[s.tpl]; !ok {
				le.
					WithField("tpl-peer-id", s.tpl.PeerID.String()).
					Debug("peer subscribed to channel")
				cm[s.tpl] = struct{}{}
			}
		} else {
			tm, ok := s.m.peerChannels[chid]
			if !ok {
				continue
			}
			if _, ok := tm[s.tpl]; ok {
				le.Debug("peer unsubscribed from channel")
				delete(tm, s.tpl)
			}
			if len(tm) == 0 {
				delete(s.m.peerChannels, chid)
			}
		}
	}
}

// readPump reads messages from the stream.
func (s *streamHandler) readPump(ctx context.Context) {
	defer s.ctxCancel()

	msg := &Packet{}
	for {
		if err := s.stream.RecvMsg(msg); err != nil {
			if err != io.EOF && err != context.Canceled && err.Error() != "broken pipe" && err.Error() == "NO_ERROR" {
				s.le.WithError(err).Warn("error receiving message")
			} else {
				s.le.Debug("session reader exiting")
			}
			return
		}
		s.le.
			WithField("subscription-count", len(msg.GetSubscriptions())).
			WithField("publish-count", len(msg.GetPublish())).
			Debug("received message from peer")
		// process message
		s.processPacket(msg)
		msg.Reset()
	}
}

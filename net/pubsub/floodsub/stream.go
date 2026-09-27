package floodsub

import (
	"context"
	"io"
	"sync"

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

	// subMtx guards pending and announced subscription state.
	subMtx sync.Mutex
	// subWake wakes this stream's sole writer for coalesced state changes.
	subWake chan struct{}
	// pending contains only desired states that differ from announced.
	pending map[string]bool
	// announced records subscription changes already taken by the writer.
	announced map[string]bool
}

// queueSubscriptions coalesces state without waiting for the peer's writer.
func (s *streamHandler) queueSubscriptions(changes []*SubscriptionOpts) {
	s.subMtx.Lock()
	if s.pending == nil {
		s.pending = make(map[string]bool)
		s.announced = make(map[string]bool)
	}
	for _, change := range changes {
		id, subscribe := change.GetChannelId(), change.GetSubscribe()
		if s.announced[id] == subscribe {
			delete(s.pending, id)
			continue
		}
		s.pending[id] = subscribe
	}
	s.subMtx.Unlock()
	select {
	case s.subWake <- struct{}{}:
	default:
	}
}

// takeSubscriptions transfers the next ordered state change to the sole writer.
func (s *streamHandler) takeSubscriptions() *Packet {
	s.subMtx.Lock()
	defer s.subMtx.Unlock()
	if len(s.pending) == 0 {
		return nil
	}
	packet := &Packet{Subscriptions: make([]*SubscriptionOpts, 0, len(s.pending))}
	for id, subscribe := range s.pending {
		packet.Subscriptions = append(packet.Subscriptions, &SubscriptionOpts{ChannelId: id, Subscribe: subscribe})
		if subscribe {
			s.announced[id] = true
		} else {
			delete(s.announced, id)
		}
		delete(s.pending, id)
	}
	return packet
}

// tryWritePacket queues a packet without blocking.
// Returns false if the session is closed or its queue is full.
func (s *streamHandler) tryWritePacket(pkt *Packet) bool {
	select {
	case <-s.ctx.Done():
		return false
	default:
	}
	select {
	case s.packetCh <- pkt:
		return true
	default:
		return false
	}
}

// executeSession executes the stream session.
func (s *streamHandler) executeSession() error {
	ctx := s.ctx
	// Closing the transport interrupts a blocked write as well as the reader.
	stop := context.AfterFunc(ctx, func() { s.stream.Close() })
	defer stop()
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		s.readPump(ctx)
	}()
	defer func() {
		s.ctxCancel()
		s.stream.Close()
		<-readDone
	}()

	for {
		// Subscription changes use the same writer, preserving order under backpressure.
		if packet := s.takeSubscriptions(); packet != nil {
			if err := s.stream.SendMsg(packet); err != nil {
				return err
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-s.subWake:
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
	defer s.m.peerChanges.HoldLock(func(notify func(), _ func() <-chan struct{}) { notify() })

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
			if err != io.EOF && err != context.Canceled && err.Error() != "NO_ERROR" {
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

package floodsub

import (
	"context"
	"sync"
	"time"

	"github.com/patrickmn/go-cache"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/pubsub"
	pubmessage "github.com/s4wave/spacewave/net/pubsub/util/pubmessage"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// maxMessageSize constrains the message buffer allocation size.
// currently set to 2MB
const maxMessageSize = 2000000

const (
	FloodSubID = protocol.ID("bifrost/floodsub")
)

// FloodSub implements the FloodSub router.
//
// TODO bind to a specific peer
type FloodSub struct {
	// conf is the config
	conf *Config
	// le is the logger
	le *logrus.Entry
	// handler is the pubsub handler
	handler pubsub.PubSubHandler
	// wakeCh wakes the execute loop
	wakeCh chan struct{}
	// publishCh is for publishing messages
	publishCh chan *publishChMsg
	// seenMessages is a map with recently seen messages
	seenMessages *cache.Cache

	mtx sync.Mutex
	// peers are the complete set of executing remote peer streams that we can
	// use to contact next-hop peers. this is the "working set" of peers.
	peers map[pubsub.PeerLinkTuple]*streamHandler
	// channels are active local channel subscriptions.
	// key: channel ID
	channels map[string]map[*subscription]struct{}
	// peerChannels tracks which topics each peer is subscribed to
	peerChannels map[string]map[pubsub.PeerLinkTuple]struct{}

	// incSessions are sessions that were just added and haven't been executed yet.
	// if !streamHandler.initiator -> push to peers map when executing
	incSessions []*streamHandler
}

// publishChMsg is a message queued for publishing
type publishChMsg struct {
	msg         *peer.SignedMsg
	prevHopPeer peer.ID
	channelID   string
}

// NewFloodSub constructs a new FloodSub PubSub router.
func NewFloodSub(
	ctx context.Context,
	le *logrus.Entry,
	handler pubsub.PubSubHandler,
	cc *Config,
) (pubsub.PubSub, error) {
	return &FloodSub{
		le:      le,
		conf:    cc,
		handler: handler,

		wakeCh:       make(chan struct{}, 1),
		peers:        make(map[pubsub.PeerLinkTuple]*streamHandler),
		channels:     make(map[string]map[*subscription]struct{}),
		publishCh:    make(chan *publishChMsg, 16),
		seenMessages: cache.New(120*time.Second, 30*time.Second),

		peerChannels: make(map[string]map[pubsub.PeerLinkTuple]struct{}),
	}, nil
}

// Execute executes the PubSub routines.
func (m *FloodSub) Execute(ctx context.Context) error {
	m.le.Debug("floodsub starting")
	// re-evaluate at most once every 100ms
	evalTimer := time.NewTicker(time.Millisecond * 100)
	defer evalTimer.Stop()

	pubbedChannels := make(map[string]struct{})
	for {
		var initSet []*SubscriptionOpts
		m.mtx.Lock()
		for i := range m.incSessions {
			s := m.incSessions[i]
			nctx, nctxCancel := context.WithCancel(ctx)
			s.ctx = nctx
			s.ctxCancel = nctxCancel
			// if !s.initiator {
			if initSet == nil {
				initSet = make([]*SubscriptionOpts, 0, len(m.channels))
				for chid := range m.channels {
					initSet = append(initSet, &SubscriptionOpts{
						ChannelId: chid,
						Subscribe: true,
					})
				}
			}
			s.packetCh <- &Packet{Subscriptions: initSet}
			// }
			go func() {
				err := s.executeSession()
				if err != nil && err != context.Canceled {
					s.le.WithError(err).Warn("session exited with error")
				}
				// if s.initiator {
				m.mtx.Lock()
				if m.peers[s.tpl] == s {
					delete(m.peers, s.tpl)
				}
				m.mtx.Unlock()
				// }
			}()
			// if s.initiator {
			m.peers[s.tpl] = s
			// }
		}
		m.incSessions = nil
		m.mtx.Unlock() // intentional mtx hold-break
		initSet = nil

		var xmitPeers []*streamHandler
		var subChanges []*SubscriptionOpts
		m.mtx.Lock()
		// sweep empty channels
		for chid, chm := range m.channels {
			if len(chm) == 0 {
				if _, ok := pubbedChannels[chid]; ok {
					// cleanup no-ref subscription
					// inform peers we no longer need the channel
					subChanges = append(subChanges, &SubscriptionOpts{
						ChannelId: chid,
						Subscribe: false,
					})
					delete(pubbedChannels, chid)
				}
				delete(m.channels, chid)
				m.le.WithField("channel-id", chid).Info("unsubscribed from channel")
			} else if _, ok := pubbedChannels[chid]; !ok {
				pubbedChannels[chid] = struct{}{}
				subChanges = append(subChanges, &SubscriptionOpts{
					ChannelId: chid,
					Subscribe: true,
				})
			}
		}

		if len(subChanges) != 0 {
			xmitPeers = make([]*streamHandler, 0, len(m.peers))
			for _, p := range m.peers {
				if p.ctx != nil {
					xmitPeers = append(xmitPeers, p)
				}
			}
		}
		m.mtx.Unlock()
		for _, p := range xmitPeers { // xmitPeers is usually nil
			p.writePacket(&Packet{Subscriptions: subChanges})
		}

		var woken bool
		for !woken {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case pubMsg := <-m.publishCh:
				m.execPublish(pubMsg.prevHopPeer, pubMsg)
			case <-m.wakeCh:
				woken = true
			}
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-evalTimer.C:
		}
	}
}

// execPublish executes publishing a message
func (m *FloodSub) execPublish(prevHopPeerID peer.ID, pubMsg *publishChMsg) {
	pkt := &Packet{
		Publish: []*peer.SignedMsg{
			pubMsg.msg,
		},
	}
	chid := pubMsg.channelID
	tosend := make(map[pubsub.PeerLinkTuple]struct{})
	m.mtx.Lock()
	if peerChannels, ok := m.peerChannels[chid]; ok {
		for p := range peerChannels {
			tosend[p] = struct{}{}
		}
	}
	for pid := range tosend {
		pidString := pid.PeerID.String()
		if pidString == pubMsg.msg.GetFromPeerId() ||
			pid.PeerID == prevHopPeerID {
			continue
		}

		peer, ok := m.peers[pid]
		if ok {
			peer.writePacket(pkt)
		}
	}
	m.mtx.Unlock()
}

// AddSubscription adds a channel subscription, returning a subscription handle.
func (m *FloodSub) AddSubscription(ctx context.Context, privKey crypto.PrivKey, channelID string) (pubsub.Subscription, error) {
	if channelID == "" {
		return nil, errors.New("channel id must be specified")
	}

	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, err
	}

	ns := &subscription{
		ctx:       ctx,
		m:         m,
		channelID: channelID,
		handlers:  make(map[*subscriptionHandler]struct{}),
		privKey:   privKey,
		peerID:    peerID,
	}
	m.mtx.Lock()
	subs := m.channels[channelID]
	if subs == nil {
		subs = make(map[*subscription]struct{})
		defer m.wake()
		m.channels[channelID] = subs
		m.le.
			WithField("channel-id", channelID).
			WithField("sub-peer-id", peerID.String()).
			Info("subscribed to channel")
	}
	subs[ns] = struct{}{}
	m.mtx.Unlock()
	return ns, nil
}

// AddPeerStream adds a negotiated peer stream.
// The pubsub should communicate over the stream.
func (m *FloodSub) AddPeerStream(
	tpl pubsub.PeerLinkTuple,
	initiator bool,
	mstrm link.MountedStream,
) {
	le := m.le.WithField("peer", tpl.PeerID.String())
	sh := &streamHandler{
		m:      m,
		le:     le,
		tpl:    tpl,
		peerID: mstrm.GetPeerID(),

		packetCh:  make(chan *Packet, 32),
		stream:    stream_packet.NewSession(mstrm.GetStream(), maxMessageSize),
		initiator: initiator,
	}
	m.mtx.Lock()
	// if !initiator {
	if e, ok := m.peers[tpl]; ok {
		if e.ctxCancel != nil {
			e.ctxCancel()
		}
	}
	m.peers[tpl] = sh
	// }
	m.incSessions = append(m.incSessions, sh)
	m.mtx.Unlock()
	m.wake()
}

// Publish writes to the channel with a private key.
func (m *FloodSub) Publish(
	ctx context.Context,
	channelID string,
	privKey crypto.PrivKey,
	data []byte,
) error {
	pht := m.conf.GetPublishHashType()
	if pht == hash.HashType_HashType_UNKNOWN {
		pht = hash.HashType_HashType_SHA256
	}

	msg, inner, err := pubmessage.NewPubMessage(channelID, privKey, pht, data)
	if err != nil {
		return err
	}

	pid, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return err
	}

	m.handleValidMessage(ctx, pid, msg, inner)
	return nil
}

// Close closes the pubsub.
func (m *FloodSub) Close() {
	m.mtx.Lock()
	for pid, s := range m.peers {
		if s.ctxCancel != nil {
			s.ctxCancel()
		}
		delete(m.peers, pid)
	}
	for _, s := range m.incSessions {
		if s.stream != nil {
			s.stream.Close()
		}
	}
	m.incSessions = nil
	m.mtx.Unlock()
}

// handleValidMessage handles a valid message, repeating it to other peers and handlers.
func (m *FloodSub) handleValidMessage(
	ctx context.Context,
	prevHopPeer peer.ID,
	pkt *peer.SignedMsg,
	pktInner *pubmessage.PubMessageInner,
) {
	channelID := pktInner.GetChannel()
	msgId := pkt.ComputeMessageID()
	if _, ok := m.seenMessages.Get(msgId); ok {
		return
	}
	m.seenMessages.Set(msgId, pkt, 0)

	pid, err := peer.IDB58Decode(pkt.GetFromPeerId())
	if err != nil {
		return
	}
	msg := pubmessage.NewMessage(pid, pktInner)
	m.mtx.Lock()
	subs := m.channels[channelID]
	for sub := range subs {
		ss := sub
		go func() {
			ss.mtx.Lock()
			for s := range ss.handlers {
				s.cb(msg)
			}
			ss.mtx.Unlock()
		}()
	}
	m.mtx.Unlock()
	select {
	case m.publishCh <- &publishChMsg{
		msg:         pkt,
		channelID:   channelID,
		prevHopPeer: prevHopPeer,
	}:
	case <-ctx.Done():
	}
}

// wake wakes the controller
func (m *FloodSub) wake() {
	select {
	case m.wakeCh <- struct{}{}:
	default:
	}
}

/*
func shufflePeers(peers []pubsub.PeerLinkTuple) {
	for i := range peers {
		j := rand.Intn(i + 1)
		peers[i], peers[j] = peers[j], peers[i]
	}
}

// getPeers returns peers for a channel
func (m *FloodSub) getPeers(
	channel string,
	count int,
	filter func(pubsub.PeerLinkTuple) bool,
) []pubsub.PeerLinkTuple {
	tmap, ok := m.peerChannels[channel]
	if !ok {
		return nil
	}

	peers := make([]pubsub.PeerLinkTuple, 0, len(tmap))
	for p := range tmap {
		if filter(p) {
			peers = append(peers, p)
		}
	}

	shufflePeers(peers)
	if count > 0 && len(peers) > count {
		peers = peers[:count]
	}

	return peers
}

// peerListToMap converts a slice to a map
func peerListToMap(peers []pubsub.PeerLinkTuple) map[pubsub.PeerLinkTuple]struct{} {
	pmap := make(map[pubsub.PeerLinkTuple]struct{})
	for _, p := range peers {
		pmap[p] = struct{}{}
	}
	return pmap
}

func peerMapToList(peers map[pubsub.PeerLinkTuple]struct{}) []pubsub.PeerLinkTuple {
	plst := make([]pubsub.PeerLinkTuple, 0, len(peers))
	for p := range peers {
		plst = append(plst, p)
	}
	return plst
}
*/

// _ is a type assertion
var _ pubsub.PubSub = ((*FloodSub)(nil))

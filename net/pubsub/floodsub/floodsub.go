package floodsub

import (
	"context"
	"sync"
	"time"

	"github.com/aperturerobotics/util/broadcast"
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
	"golang.org/x/sync/errgroup"
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
	// seenMessages tracks recently seen message IDs until their expiry.
	// guarded by mtx
	seenMessages map[string]time.Time

	// peerChanges wakes subscription-readiness consumers under mtx.
	peerChanges broadcast.Broadcast
	mtx         sync.Mutex
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
		seenMessages: make(map[string]time.Time),

		peerChannels: make(map[string]map[pubsub.PeerLinkTuple]struct{}),
	}, nil
}

// Execute reconciles topology at known deadlines while continuously handling publications.
func (m *FloodSub) Execute(ctx context.Context) error {
	m.le.Debug("floodsub starting")
	var sessions errgroup.Group
	defer func() {
		m.Close()
		sessions.Wait()
	}()

	// Timers are armed only for pending changes and known message expirations.
	reconcile := time.NewTimer(time.Hour)
	reconcile.Stop()
	defer reconcile.Stop()
	expire := time.NewTimer(time.Hour)
	expire.Stop()
	defer expire.Stop()
	var reconcileCh, expireCh <-chan time.Time
	published := make(map[string]struct{})
	m.reconcile(ctx, published, &sessions)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case pubMsg := <-m.publishCh:
			m.execPublish(pubMsg.prevHopPeer, pubMsg)
			if expireCh == nil {
				expire.Reset(seenMessageTTL)
				expireCh = expire.C
			}
		case <-m.wakeCh:
			if reconcileCh == nil {
				reconcile.Reset(100 * time.Millisecond)
				reconcileCh = reconcile.C
			}
		case <-reconcileCh:
			reconcileCh = nil
			m.reconcile(ctx, published, &sessions)
		case <-expireCh:
			expireCh = nil
			if next := m.expireSeen(time.Now()); !next.IsZero() {
				expire.Reset(time.Until(next))
				expireCh = expire.C
			}
		}
	}
}

// reconcile starts pending streams and publishes coalesced subscription changes.
// The Execute goroutine owns published and sessions.
func (m *FloodSub) reconcile(ctx context.Context, published map[string]struct{}, sessions *errgroup.Group) {
	m.mtx.Lock()
	defer m.mtx.Unlock()

	// Queue initial subscriptions before each stream's writer starts.
	for _, s := range m.incSessions {
		if m.peers[s.tpl] != s {
			s.stream.Close()
			continue
		}
		s.ctx, s.ctxCancel = context.WithCancel(ctx)
		initial := make([]*SubscriptionOpts, 0, len(m.channels))
		for id, refs := range m.channels {
			if len(refs) != 0 {
				initial = append(initial, &SubscriptionOpts{ChannelId: id, Subscribe: true})
			}
		}
		s.queueSubscriptions(initial)
		sessions.Go(func() error {
			if err := s.executeSession(); err != nil && !errors.Is(err, context.Canceled) {
				s.le.WithError(err).Debug("session exited")
			}
			m.mtx.Lock()
			if m.peers[s.tpl] == s {
				delete(m.peers, s.tpl)
				m.removePeerChannelsLocked(s.tpl)
			}
			m.mtx.Unlock()
			return nil
		})
	}
	m.incSessions = nil

	// Retain only the final desired state for each peer; no peer queue is awaited.
	var changes []*SubscriptionOpts
	for id, refs := range m.channels {
		_, announced := published[id]
		if len(refs) == 0 {
			if announced {
				changes = append(changes, &SubscriptionOpts{ChannelId: id})
				delete(published, id)
			}
			delete(m.channels, id)
			continue
		}
		if !announced {
			published[id] = struct{}{}
			changes = append(changes, &SubscriptionOpts{ChannelId: id, Subscribe: true})
		}
	}
	if len(changes) == 0 {
		return
	}
	for _, peer := range m.peers {
		if peer.ctx != nil {
			peer.queueSubscriptions(changes)
		}
	}
}

// expireSeen removes expired message IDs and returns the next known deadline.
func (m *FloodSub) expireSeen(now time.Time) time.Time {
	m.mtx.Lock()
	defer m.mtx.Unlock()
	var next time.Time
	for id, deadline := range m.seenMessages {
		if !deadline.After(now) {
			delete(m.seenMessages, id)
			continue
		}
		if next.IsZero() || deadline.Before(next) {
			next = deadline
		}
	}
	return next
}

// execPublish executes publishing a message
func (m *FloodSub) execPublish(prevHopPeerID peer.ID, pubMsg *publishChMsg) {
	pkt := &Packet{
		Publish: []*peer.SignedMsg{
			pubMsg.msg,
		},
	}
	chid := pubMsg.channelID
	fromPeerID := pubMsg.msg.GetFromPeerId()
	var targets []*streamHandler
	m.mtx.Lock()
	for pid := range m.peerChannels[chid] {
		if pid.PeerID.String() == fromPeerID || pid.PeerID == prevHopPeerID {
			continue
		}
		// skip peers whose session has not started executing yet
		if peer, ok := m.peers[pid]; ok && peer.ctx != nil {
			targets = append(targets, peer)
		}
	}
	m.mtx.Unlock()

	for _, peer := range targets {
		if !peer.tryWritePacket(pkt) {
			peer.le.
				WithField("channel-id", chid).
				Debug("dropped publish to slow peer")
		}
	}
}

// removePeerChannelsLocked removes a peer from all peer channel subscriptions.
// Expects m.mtx to be held.
func (m *FloodSub) removePeerChannelsLocked(tpl pubsub.PeerLinkTuple) {
	for chid, cm := range m.peerChannels {
		delete(cm, tpl)
		if len(cm) == 0 {
			delete(m.peerChannels, chid)
		}
	}
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
		subWake:   make(chan struct{}, 1),
		stream:    stream_packet.NewSession(mstrm.GetStream(), maxMessageSize),
		initiator: initiator,
	}
	m.mtx.Lock()

	if e, ok := m.peers[tpl]; ok {
		if e.ctxCancel != nil {
			e.ctxCancel()
		}
	}
	m.peers[tpl] = sh

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

	// Record the message ID and report whether it was already seen.
	msgId := pkt.ComputeMessageID()
	m.mtx.Lock()
	now := time.Now()
	deadline, seen := m.seenMessages[msgId]
	seen = seen && deadline.After(now)
	if !seen {
		m.seenMessages[msgId] = now.Add(seenMessageTTL)
	}
	m.mtx.Unlock()
	if seen {
		return
	}

	pid, err := peer.IDB58Decode(pkt.GetFromPeerId())
	if err != nil {
		return
	}
	msg := pubmessage.NewMessage(pid, pktInner)

	// Snapshot the subscriptions, then deliver to each handler set.
	// Handlers must not block, so they run on the calling goroutine.
	m.mtx.Lock()
	subs := make([]*subscription, 0, len(m.channels[channelID]))
	for sub := range m.channels[channelID] {
		subs = append(subs, sub)
	}
	m.mtx.Unlock()
	for _, ss := range subs {
		ss.mtx.Lock()
		for s := range ss.handlers {
			s.cb(msg)
		}
		ss.mtx.Unlock()
	}
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

// _ is a type assertion
var _ pubsub.PubSub = (*FloodSub)(nil)

// WaitForPeerSubscription waits until a peer announces channelID.
// It observes the router's subscription state without polling or publishing probes.
func (m *FloodSub) WaitForPeerSubscription(ctx context.Context, channelID string, id peer.ID) error {
	for {
		m.mtx.Lock()
		var wait <-chan struct{}
		m.peerChanges.HoldLock(func(_ func(), getWait func() <-chan struct{}) { wait = getWait() })
		found := false
		for tuple := range m.peerChannels[channelID] {
			if tuple.PeerID == id {
				found = true
				break
			}
		}
		m.mtx.Unlock()
		if found {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-wait:
		}
	}
}

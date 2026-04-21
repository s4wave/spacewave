package floodsub

import (
	"context"
	"sync"

	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/pubsub"
)

// subscription implements the pubsub subscription handle.
type subscription struct {
	// ctx is the context for publishing
	ctx context.Context
	// relOnce guards release
	relOnce sync.Once
	// m is the floodsub instance
	m *FloodSub
	// channelID is the channel identifier
	channelID string
	// privKey is the private key for the sub
	privKey crypto.PrivKey
	// peerID is the peer ID for the sub
	peerID peer.ID

	// mtx guards handlers
	mtx sync.Mutex
	// handlers are the active handlers
	handlers map[*subscriptionHandler]struct{}
}

// GetPeerId returns the peer ID for this subscription derived from private key.
func (s *subscription) GetPeerId() peer.ID {
	return s.peerID
}

// GetChannelId returns the channel id.
func (s *subscription) GetChannelId() string {
	return s.channelID
}

// Publish writes to the channel.
func (s *subscription) Publish(data []byte) error {
	return s.m.Publish(s.ctx, s.channelID, s.privKey, data)
}

// AddHandler adds a callback that is called with each received message.
// The callback should not block.
// Returns a remove function.
// The handler(s) are also removed when the subscription is released.
func (s *subscription) AddHandler(cb func(m pubsub.Message)) func() {
	sh := &subscriptionHandler{cb: cb}
	s.mtx.Lock()
	s.handlers[sh] = struct{}{}
	s.mtx.Unlock()
	relOnce := sync.Once{}
	return func() {
		relOnce.Do(func() {
			s.mtx.Lock()
			delete(s.handlers, sh)
			s.mtx.Unlock()
		})
	}
}

// Release releases the handle.
func (s *subscription) Release() {
	s.mtx.Lock()
	for h := range s.handlers {
		delete(s.handlers, h)
	}
	s.mtx.Unlock()
	s.relOnce.Do(func() {
		chid := s.channelID
		s.m.mtx.Lock()
		subs := s.m.channels[chid]
		if subs != nil {
			delete(subs, s)
		}
		if len(subs) == 0 {
			defer s.m.wake()
		}
		s.m.mtx.Unlock()
	})
}

// _ is a type assertion
var _ pubsub.Subscription = ((*subscription)(nil))

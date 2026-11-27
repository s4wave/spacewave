package frametrack

import (
	"context"
	"crypto/rand"
	"sync"

	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/stream"
)

// SubscriptionRegistry routes incoming groups on a viewer's Session bus.
// The registry can be installed with the Session bus's AddHandler method.
type SubscriptionRegistry struct {
	// localPeer is the Session identity that receives the groups.
	localPeer peer.ID

	// mtx guards the peer map and each peer's subscriptions.
	mtx sync.Mutex
	// peers holds the subscriptions for each Device with an active viewer.
	peers map[peer.ID]*peerSubscriptions
}

// peerSubscriptions holds a Device's subscriptions under the registry lock.
type peerSubscriptions struct {
	// ctx lasts until the Device has no subscriptions in this registry.
	ctx context.Context
	// cancel closes streams for this peer that are still reading their headers.
	cancel context.CancelFunc
	// byID routes the Device's group headers to their subscription.
	byID map[string]viewerSubscription
}

// viewerSubscription holds one viewer's lifetime and group consumer.
type viewerSubscription struct {
	// ctx ends when the viewer releases its subscription.
	ctx context.Context
	// readGroup consumes a group until it ends or ctx is canceled.
	readGroup func(context.Context, *GroupReader)
}

// NewSubscriptionRegistry creates the group resolver for a viewer's Session.
func NewSubscriptionRegistry(localPeer peer.ID) *SubscriptionRegistry {
	return &SubscriptionRegistry{
		localPeer: localPeer,
		peers:     make(map[peer.ID]*peerSubscriptions),
	}
}

// Register prepares a new subscription to send on the Device's control stream.
// The viewer holds the control stream and its link until ctx ends or release is
// called. The reader callback may run concurrently for different groups. It
// reads the group before returning, when the registry closes the stream.
func (r *SubscriptionRegistry) Register(
	ctx context.Context,
	control link.MountedStream,
	readGroup func(context.Context, *GroupReader),
) (subscription *Subscription, release func(), err error) {
	// Bind a fresh ID to the peer authenticated on the control stream.
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	device := control.GetPeerID()
	if device == "" || readGroup == nil {
		return nil, nil, errors.New("frame subscription requires a Device peer and reader")
	}
	id := rand.Text()
	subCtx, cancel := context.WithCancel(ctx)

	// Keep header reads alive while the Device has at least one subscription.
	r.mtx.Lock()
	set := r.peers[device]
	if set == nil {
		peerCtx, cancelPeer := context.WithCancel(context.Background())
		set = &peerSubscriptions{
			ctx: peerCtx, cancel: cancelPeer, byID: make(map[string]viewerSubscription),
		}
		r.peers[device] = set
	}
	set.byID[id] = viewerSubscription{ctx: subCtx, readGroup: readGroup}
	r.mtx.Unlock()

	// Retire the peer's header readers once its last subscription goes away.
	context.AfterFunc(subCtx, func() {
		// Keep the Device registered while another subscription can receive groups.
		r.mtx.Lock()
		delete(set.byID, id)
		if len(set.byID) != 0 {
			r.mtx.Unlock()
			return
		}

		// Remove the empty registry entry before interrupting its header readers.
		delete(r.peers, device)
		r.mtx.Unlock()
		set.cancel()
	})
	return &Subscription{SubscriptionId: id}, cancel, nil
}

// HandleDirective resolves the group protocol for this viewer's Session peer.
func (r *SubscriptionRegistry) HandleDirective(_ context.Context, instance directive.Instance) ([]directive.Resolver, error) {
	// Match the directive's protocol and local identity.
	dir, ok := instance.GetDirective().(link.HandleMountedStream)
	if !ok || dir.HandleMountedStreamProtocolID() != GroupProtocolID ||
		dir.HandleMountedStreamLocalPeerID() != r.localPeer {
		return nil, nil
	}

	// Resolve the shared handler, which checks live subscriptions at admission.
	return directive.Resolvers(directive.NewValueResolver([]link.MountedStreamHandler{r})), nil
}

// HandleMountedStream accepts reliable groups from subscribed Devices.
func (r *SubscriptionRegistry) HandleMountedStream(ctx context.Context, ms link.MountedStream) error {
	// Reject unsolicited peers before reading their headers.
	if ms.GetOpenOpts().Unreliable {
		return errors.New("frame groups require a reliable stream")
	}
	r.mtx.Lock()
	set := r.peers[ms.GetPeerID()]
	r.mtx.Unlock()
	if set == nil {
		return errors.New("frame group peer has no live subscription")
	}

	// Read the header off the transport's stream dispatch path.
	go r.readGroup(ctx, set, ms.GetStream())
	return nil
}

// readGroup routes an incoming group and closes its stream when any owner ends.
func (r *SubscriptionRegistry) readGroup(ctx context.Context, set *peerSubscriptions, strm stream.Stream) {
	// Stop the header read if the link or Device's last subscription ends.
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	defer context.AfterFunc(set.ctx, cancel)()
	stopClose := context.AfterFunc(ctx, func() {
		// Closing the stream releases a blocked reader.
		_ = strm.Close()
	})
	defer func() {
		if stopClose() {
			// Release the stream once the header or group reader returns.
			_ = strm.Close()
		}
	}()

	// Resolve the claimed ID within the authenticated Device's subscriptions.
	group, err := NewGroupReader(strm)
	if err != nil {
		return
	}
	r.mtx.Lock()
	sub, ok := group.resolveSubscription(set)
	r.mtx.Unlock()
	if !ok || sub.ctx.Err() != nil {
		return
	}

	// Bind the admitted reader to this subscription's own lifetime.
	defer context.AfterFunc(sub.ctx, cancel)()
	if ctx.Err() == nil && sub.ctx.Err() == nil {
		sub.readGroup(ctx, group)
	}
}

// resolveSubscription looks up a claim within this peer's live subscriptions.
// The caller holds the registry lock.
func (s *peerSubscriptions) resolveSubscription(id string) (viewerSubscription, bool) {
	sub, ok := s.byID[id]
	return sub, ok
}

// _ is a type assertion.
var (
	_ directive.Handler         = (*SubscriptionRegistry)(nil)
	_ link.MountedStreamHandler = (*SubscriptionRegistry)(nil)
)

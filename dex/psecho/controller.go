package psecho

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/aperturerobotics/bifrost/link"
	"github.com/aperturerobotics/bifrost/peer"
	"github.com/aperturerobotics/bifrost/pubsub"
	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/hydra/block"
	bucket_lookup "github.com/aperturerobotics/hydra/bucket/lookup"
	"github.com/aperturerobotics/hydra/dex"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/blang/semver"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
)

// xmitWantlistDur is the time to wait between soliciting the wantlist.
var xmitWantlistDur = time.Second * time.Duration(3)

// xmitWantlistSize is the max number of refs per xmission
// at 32 bytes each, 64 should be a good cap
var xmitWantlistSize = 64

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "hydra/dex/psecho/1"

// Controller is the pubsub echo controller.
//
// The execute routine manages the local want list, and communicating changes to
// the network. When a directive to look up a block arrives, the LookupBlock
// call is issued with a context and a desired block ref. This pushes the
// reference and a handle to the Execute routine. All waiters for a given
// reference are processed together once.
type Controller struct {
	// le is the root logger
	le *logrus.Entry
	// b is the controller bus
	b bus.Bus
	// cc is the configuration
	cc *Config
	// wakeCh wakes the execute routine
	wakeCh chan struct{}

	// mtx guards below fields
	mtx sync.Mutex
	// waiters contains the current block waiter set
	// key: block ref as a string
	waiters map[string]*desiredBlockWaiter
	// remotePeers contains remote peer states
	remotePeers map[peer.ID]*remotePeer
	// syncWantCheckCh pushes a wantlist for checking to initiate a sync session
	syncWantCheckCh chan *syncCheckList
	// rxBlockCh contains incoming blocks received from peers
	rxBlockCh chan *rxBlock
	// cState contains the cState object
	// set when the controller becomes ready
	cState *ccontainer.CContainer[*cState]
}

// cState contains information about the controller that is resolved at start
type cState struct {
	// ctx is the root context
	ctx context.Context
	// peerID is the resolved peer id
	peerID peer.ID
	// peerPriv is the resolved peer private key
	peerPriv crypto.PrivKey
}

// NewController constructs a new node controller.
func NewController(le *logrus.Entry, b bus.Bus, cc *Config) (*Controller, error) {
	if channelID := cc.GetPubsubChannel(); channelID != "" {
		le = le.WithField("pubsub-channel", channelID)
	}

	return &Controller{
		le: le,
		b:  b,
		cc: cc,

		waiters:         make(map[string]*desiredBlockWaiter),
		remotePeers:     make(map[peer.ID]*remotePeer),
		wakeCh:          make(chan struct{}, 1),
		syncWantCheckCh: make(chan *syncCheckList, 15),
		rxBlockCh:       make(chan *rxBlock),
		cState:          ccontainer.NewCContainer[*cState](nil),
	}, nil
}

// Execute executes the given controller.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// c.concurrent.Execute() -> is a stub, no need to call it.
	subCtx, subCtxCancel := context.WithCancel(ctx)
	defer subCtxCancel()

	peerID, err := c.cc.ParsePeerID()
	if err != nil {
		return err
	}

	pr, prRef, err := peer.GetPeerWithID(ctx, c.b, peerID)
	if err != nil {
		return errors.Wrapf(err, "get peer with id %s", peerID.Pretty())
	}
	privKey, err := pr.GetPrivKey(ctx)
	if err != nil {
		return err
	}
	peerID = pr.GetPeerID()
	prRef.Release()

	setCState := &cState{
		ctx:      ctx,
		peerID:   peerID,
		peerPriv: privKey,
	}
	c.cState.SetValue(setCState)
	defer c.cState.SwapValue(func(val *cState) *cState {
		if val == setCState {
			val = nil
		}
		return val
	})

	// Subscribe to the pubsub channel.
	channelID := c.cc.GetPubsubChannel()
	psChVal, psChRef, err := bus.ExecOneOff(
		ctx,
		c.b,
		pubsub.NewBuildChannelSubscription(channelID, privKey),
		false,
		subCtxCancel,
	)
	if err != nil {
		return err
	}
	defer psChRef.Release()

	psCh, ok := psChVal.GetValue().(pubsub.BuildChannelSubscriptionValue)
	if !ok {
		return errors.New("build channel subscription returned unexpected value")
	}

	// add pubsub handler
	relHandler := psCh.AddHandler(func(m pubsub.Message) {
		c.handleIncomingMessage(subCtx, m, privKey)
	})
	defer relHandler()

	// defer cleanup of all waiters
	defer func() {
		c.mtx.Lock()
		for id, w := range c.waiters {
			w.err = context.Canceled
			close(w.doneCh)
			delete(c.waiters, id)
		}
		c.mtx.Unlock()
	}()

	buildLookup := func() (bucket_lookup.Lookup, func(), error) {
		bv, bvRef, err := bus.ExecOneOff(
			subCtx,
			c.b,
			bucket_lookup.NewBuildBucketLookup(c.cc.GetBucketId()),
			false,
			nil,
		)
		if err != nil {
			return nil, nil, err
		}
		lv, ok := bv.GetValue().(bucket_lookup.BuildBucketLookupValue)
		if !ok {
			bvRef.Release()
			return nil, nil, errors.New("build bucket lookup returned unknown value")
		}
		lk, err := lv.GetLookup(subCtx)
		if err != nil {
			bvRef.Release()
			return nil, nil, err
		}
		if lk == nil {
			bvRef.Release()
		}
		return lk, bvRef.Release, nil
	}

	// outer message to send to network
	var psOut PubSubMessage
	// send a advertisement of wantlist every N seconds
	wantlistTicker := time.NewTicker(xmitWantlistDur)
	defer wantlistTicker.Stop()
	var nextWantlistMin int
	// inner loop
	for {
		select {
		case <-subCtx.Done():
			return subCtx.Err()
		case rxb := <-c.rxBlockCh: // wait for incoming blocks
			rxRef := rxb.ref
			var wasWaiting func()
			rxRefStr := rxRef.MarshalString()
			c.mtx.Lock()
			for wid, waiter := range c.waiters {
				if waiter.ref.EqualsRef(rxRef) {
					waiter.data = rxb.data
					waiter.err = nil
					prevWasWaiting := wasWaiting
					wasWaiting = func() {
						if prevWasWaiting != nil {
							prevWasWaiting()
						}
						close(waiter.doneCh)
					}
					delete(c.waiters, wid)
					break
				}
			}
			for _, rpeer := range c.remotePeers {
				if _, ok := rpeer.wantedRefs[rxRefStr]; ok {
					if _, ok := rpeer.cachedRefs[rxRefStr]; !ok {
						rpeer.cachedRefs[rxRefStr] = rxb.data
						c.triggerRpeerSyncSession(subCtx, rpeer)
					}
				}
			}
			c.mtx.Unlock()
			// wasWaiting is a function chain that releases the waiters resolved above.
			if wasWaiting != nil {
				// build bucket handle
				lk, lkRel, err := buildLookup()
				if err != nil {
					wasWaiting()
					// TODO: possibly handle better
					return err
				}
				if lk == nil {
					// GetLookup may return nil if the bucket config is not known
					// just assume we don't have the blocks in this case
					wasWaiting()
					continue
				}
				// TODO: assert that PutBlock hash is equal to expected?
				_, _, err = lk.PutBlock(subCtx, rxb.data, &block.PutOpts{
					HashType: rxRef.GetHash().GetHashType(),
				})
				lkRel()
				wasWaiting()
				if err != nil {
					c.le.WithError(err).Warn("unable to put block to lookup handle")
				}
			} else {
				// nothing was waiting for the block, ignore it.
				continue
			}
		case wantList := <-c.syncWantCheckCh:
			// process sync checklist
			// first, quick check to ensure the peer still wants blocks
			c.mtx.Lock()
			lp, hasPeer := c.remotePeers[wantList.peer]
			c.mtx.Unlock()
			if !hasPeer {
				continue
			}
			// build bucket handle
			le := lp.le()
			le.WithField("ref", wantList.refs[0].MarshalString()).
				Debug("looking up refs for peer")
			lk, lkRel, err := buildLookup()
			if err != nil {
				// TODO: possibly handle better
				return err
			}
			if lk == nil {
				// GetLookup may return nil if the bucket config is not known
				// just assume we don't have the blocks in this case
				continue
			}
			for _, want := range wantList.refs {
				dat, ok, err := lk.LookupBlock(subCtx, want, bucket_lookup.WithLocalOnly())
				if err != nil {
					c.le.WithError(err).Warn("error looking up block")
					continue
				}
				if !ok {
					le.WithField("ref", want.MarshalString()).Debug("block not found")
					continue
				}

				le.WithField("ref", want.MarshalString()).Debug("block found")
				wantStr := want.MarshalString()
				c.mtx.Lock()
				rpeer, rpeerOk := c.remotePeers[wantList.peer]
				if !rpeerOk {
					c.mtx.Unlock()
					break
				}
				_, wasWanted := rpeer.wantedRefs[wantStr]
				if wasWanted {
					rpeer.cachedRefs[wantStr] = dat
					le.WithField("ref", want.MarshalString()).Debug("triggering sync session")
					c.triggerRpeerSyncSession(subCtx, rpeer)
				}
				c.mtx.Unlock()
				if wasWanted {
					break
				}
			}
			lkRel()
		case <-c.wakeCh:
		case <-wantlistTicker.C:
			nextWantlistMin = xmitWantlistSize
		}

		// scan wait list
		c.mtx.Lock()
		for id, w := range c.waiters {
			if w.refcount <= 0 {
				if w.xmit {
					// TODO: determine if the data was fetched or not
					psOut.ClearRefs = append(psOut.ClearRefs, w.ref)
				}
				close(w.doneCh)
				delete(c.waiters, id)
				continue
			}
			if !w.xmit || len(psOut.WantRefs) < nextWantlistMin {
				psOut.WantRefs = append(psOut.WantRefs, w.ref)
				w.xmit = true
			}
		}
		if len(c.waiters) == 0 &&
			(len(psOut.ClearRefs) != 0 || len(psOut.HaveRefs) != 0) {
			psOut.WantEmpty = true
			psOut.ClearRefs = nil
			psOut.HaveRefs = nil
		}
		c.mtx.Unlock()

		// send message to pubsub if needed
		nextWantlistMin = 0
		if len(psOut.GetClearRefs()) != 0 ||
			len(psOut.GetHaveRefs()) != 0 ||
			len(psOut.GetWantRefs()) != 0 || psOut.WantEmpty {
			data, err := psOut.MarshalBlock()
			if err != nil {
				return err
			}
			psOut.LogFields(c.le).Debug("sent pubsub message")
			psOut.Reset()
			if err := psCh.Publish(data); err != nil {
				return errors.Wrap(err, "publish message")
			}
		}
	}
}

// HandleMountedStream handles an incoming mounted stream.
// Any returned error indicates the stream should be closed.
// This function should return as soon as possible, and start
// additional goroutines to manage the lifecycle of the stream.
// Typically EstablishLink is asserted in HandleMountedStream.
func (c *Controller) HandleMountedStream(
	ctx context.Context,
	ms link.MountedStream,
) error {
	if mpid := ms.GetProtocolID(); mpid != syncProtocolID {
		return errors.Errorf(
			"expected protocol id %s but got %s",
			syncProtocolID,
			mpid,
		)
	}
	oo := ms.GetOpenOpts()
	if !oo.Encrypted || !oo.Reliable {
		return errors.New("expected stream to be encrypted and reliable")
	}

	// assert establish link to hold the link open
	_, lnkRef, err := c.b.AddDirective(
		link.NewEstablishLinkWithPeer(ms.GetLink().GetLocalPeer(), ms.GetPeerID()),
		nil,
	)
	if err != nil {
		return err
	}

	cs, err := c.getCState(ctx)
	if err != nil {
		lnkRef.Release()
		return err
	}

	from := ms.GetPeerID()

	// Incoming sync session
	// This occurs when we have some wanted blocks to receive.

	// need to build a remotePeer
	c.mtx.Lock()
	rpeer, ok := c.remotePeers[from]
	if !ok {
		rpeer = newRemotePeer(c, cs.peerID, from)
		c.remotePeers[from] = rpeer
	}
	rpeer.incSyncSessions++
	c.mtx.Unlock()

	go func() {
		if err := rpeer.executeIncomingSyncSession(cs.ctx, ms); err != nil {
			if err != context.Canceled && err != io.EOF {
				rpeer.le().WithError(err).Warn("error handling incoming sync session")
			}
		}
		lnkRef.Release()
		ms.GetStream().Close()
		c.mtx.Lock()
		if rpeer.incSyncSessions > 0 {
			rpeer.incSyncSessions--
		}
		if mp, mpOk := c.remotePeers[from]; mpOk && mp == rpeer {
			if rpeer.incSyncSessions == 0 &&
				len(rpeer.wantedRefs) == 0 &&
				rpeer.syncCtxCancel == nil {
				delete(c.remotePeers, from)
			}
		}
		c.mtx.Unlock()
	}()

	return nil
}

// wakeExecute wakes the execute loop
func (c *Controller) wakeExecute() {
	select {
	case c.wakeCh <- struct{}{}:
	default:
	}
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any exceptional errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case dex.LookupBlockFromNetwork:
		return directive.R(c.resolveLookupBlockFromNetwork(ctx, di, d))
	case link.HandleMountedStream:
		return directive.R(c.resolveHandleMountedStream(ctx, di, d))
	}
	return nil, nil
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"psecho pub-sub data exchange controller",
	)
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// triggerRpeerSyncSession triggers a sync session for a remote peer.
func (c *Controller) triggerRpeerSyncSession(subCtx context.Context, rpx *remotePeer) {
	rpx.triggerSyncSession(subCtx, func() {
		c.mtx.Lock()
		if rpeer := c.remotePeers[rpx.id]; rpeer == rpx {
			if rpeer.syncCtxCancel != nil {
				rpeer.syncCtxCancel()
				rpeer.syncCtxCancel = nil
			}
			if len(rpeer.wantedRefs) == 0 && rpeer.incSyncSessions <= 0 {
				delete(c.remotePeers, rpx.id)
			}
		}
		c.mtx.Unlock()
	})
}

// getCState returns the controller state
func (c *Controller) getCState(ctx context.Context) (*cState, error) {
	return c.cState.WaitValue(ctx, nil)
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))

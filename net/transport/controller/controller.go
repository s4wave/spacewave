package transport_controller

import (
	"context"
	"io"
	"time"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/keyed"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/link"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	"github.com/s4wave/spacewave/net/tptaddr"
	"github.com/s4wave/spacewave/net/transport"
	"github.com/s4wave/spacewave/net/transport/common/dialer"
	"github.com/sirupsen/logrus"
)

// streamEstablishTimeout is the max time to wait for a stream header.
var streamEstablishTimeout = time.Second * 5

// streamHandleTimeout is the max time to wait for a HandleMountedStream handler.
var streamHandleTimeout = time.Minute * 10

// streamEstablishMaxPacketSize is the maximum stream establish header size
var streamEstablishMaxPacketSize uint64 = 100000

// Constructor constructs a transport with common parameters.
type Constructor func(
	ctx context.Context,
	le *logrus.Entry,
	pkey crypto.PrivKey,
	handler transport.TransportHandler,
) (transport.Transport, error)

// ResolvePeerDialer is a function to resolve an address for a peer.
// Called when resolving EstablishLink.
// Return nil, nil to indicate not found or unavailable.
type ResolvePeerDialer func(
	ctx context.Context,
	le *logrus.Entry,
	pkey crypto.PrivKey,
	peerID peer.ID,
) (*dialer.DialerOpts, error)

// NewResolvePeerDialerWithStaticPeerMap builds a new ResolvePeerDialer from a peer map.
func NewResolvePeerDialerWithStaticPeerMap(spm map[string]*dialer.DialerOpts) ResolvePeerDialer {
	if spm == nil {
		return nil
	}

	return func(
		ctx context.Context,
		le *logrus.Entry,
		pkey crypto.PrivKey,
		peerID peer.ID,
	) (*dialer.DialerOpts, error) {
		return spm[peerID.String()], nil
	}
}

// Controller implements a common transport controller.
type Controller struct {
	// le is the logger
	le *logrus.Entry
	// bus is the controller bus
	bus bus.Bus
	// ctor is the constructor
	ctor Constructor
	// info is the controller info
	info *controller.Info
	// lookupPeerID is the peer id to lookup on the bus
	// may be empty
	lookupPeerID peer.ID
	// verbose enables verbose logs
	verbose bool

	// linkDialers tracks ongoing dial attempts
	// when a link is closed (removed from links) the associated dialer is restarted (if any).
	linkDialers *keyed.KeyedRefCount[linkDialerKey, *linkDialer]

	// bcast guards below fields
	bcast broadcast.Broadcast
	// execCtx is the controller execute context
	// nil until resolved
	execCtx context.Context
	// peerID is the local peer id.
	// empty until tpt is constructed
	peerID peer.ID
	// tpt is the transport
	// nil until resolved
	tpt transport.Transport
	// links is the set of active links, keyed by link uuid
	links map[uint64]*establishedLink
	// linksByPeerID is the set of links keyed by peer id
	linksByPeerID map[peer.ID][]*establishedLink
}

// NewController constructs a new transport controller.
func NewController(
	le *logrus.Entry,
	bus bus.Bus,
	info *controller.Info,
	peerID peer.ID,
	verbose bool,
	ctor Constructor,
) *Controller {
	c := &Controller{
		le:           le,
		bus:          bus,
		ctor:         ctor,
		info:         info,
		lookupPeerID: peerID,
		verbose:      verbose,

		links:         make(map[uint64]*establishedLink),
		linksByPeerID: make(map[peer.ID][]*establishedLink),
	}
	c.linkDialers = keyed.NewKeyedRefCount(c.buildLinkDialer)
	return c
}

// GetControllerID returns the controller ID.
func (c *Controller) GetControllerID() string {
	return c.info.GetId()
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return c.info.Clone()
}

// GetPeerLinks returns all links with the peer.
func (c *Controller) GetPeerLinks(peerID peer.ID) []link.Link {
	var lnks []link.Link
	c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		for _, lnk := range c.links {
			if lnk.lnk.GetRemotePeer() == peerID {
				lnks = append(lnks, lnk.lnk)
			}
		}
	})
	return lnks
}

// Execute executes the transport controller and the transport.
// Returning nil ends execution.
// Returning an error triggers a retry with backoff.
func (c *Controller) Execute(ctx context.Context) error {
	// Acquire a handle to the node.
	localPeer, _, localPeerRef, err := peer.GetPeerWithID(ctx, c.bus, c.lookupPeerID, false, nil)
	if err != nil {
		return err
	}

	// Get the priv key and release the peer
	privKey, err := localPeer.GetPrivKey(ctx)
	localPeerRef.Release()
	if err != nil {
		return err
	}

	localPeerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return err
	}

	// Construct the transport
	handler := newTransportHandler(ctx, c)
	tpt, err := c.ctor(
		ctx,
		c.le,
		privKey,
		handler,
	)
	if err != nil {
		return err
	}
	defer tpt.Close()

	// store the transport
	handler.tpt.SetResult(tpt, nil)

	if c.verbose {
		c.le.Debug("executing transport")
	}
	execCtx, execCtxCancel := context.WithCancel(ctx)
	defer execCtxCancel()

	// set hadles
	c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
		c.execCtx = execCtx
		c.peerID = localPeerID
		c.tpt = tpt
		broadcast()
	})

	// clear on exit
	defer func() {
		c.bcast.HoldLock(func(broadcast func(), getWaitCh func() <-chan struct{}) {
			c.execCtx = nil
			c.peerID = ""
			c.tpt = nil
			for _, link := range c.links {
				c.flushEstablishedLink(link, true)
			}
			broadcast()
		})
	}()

	// start link dialers
	c.linkDialers.SetContext(execCtx, true)
	defer c.linkDialers.ClearContext()

	// execute transport routine
	err = tpt.Execute(execCtx)
	if err != nil {
		return err
	}

	// Transport exited w/o an error
	<-ctx.Done()
	return context.Canceled
}

// GetTransport returns the controlled transport.
// This may be nil until the transport is constructed.
func (c *Controller) GetTransport(ctx context.Context) (transport.Transport, error) {
	var tpt transport.Transport
	err := c.bcast.Wait(ctx, func(_ func(), _ func() <-chan struct{}) (bool, error) {
		tpt = c.tpt
		return tpt != nil && c.peerID != "", nil
	})
	return tpt, err
}

// HandleDirective asks if the handler can resolve the directive.
// If it can, it returns a resolver. If not, returns nil.
// Any unexpected errors are returned for logging.
// It is safe to add a reference to the directive during this call.
func (c *Controller) HandleDirective(ctx context.Context, di directive.Instance) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case link.EstablishLinkWithPeer:
		return c.resolveEstablishLink(ctx, di, d)
	case transport.LookupTransport:
		return c.resolveLookupTransport(ctx, d)
	case tptaddr.DialTptAddr:
		return c.resolveDialTptAddr(ctx, di, d)
	}

	return nil, nil
}

// HandleIncomingStream handles an incoming stream from a link. It negotiates
// the protocol for the stream, acquires a handler for the protocol, and hands
// the stream to the protocol handler, then returns.
//
// rctx is the link Context, which is canceled when the link is closed.
func (c *Controller) HandleIncomingStream(
	rctx context.Context,
	tpt transport.Transport,
	lnk link.Link,
	strm stream.Stream,
	strmOpts stream.OpenOpts,
) {
	// Assert EstablishLink to keep the stream open during the header exchange.
	_, elRef, err := c.bus.AddDirective(
		link.NewEstablishLinkWithPeer(lnk.GetLocalPeer(), lnk.GetRemotePeer()),
		nil,
	)
	if err == nil {
		defer elRef.Release()
	}

	handleDeadline := time.Now().Add(streamHandleTimeout)
	readDeadline := time.Now().Add(streamEstablishTimeout)
	_ = strm.SetReadDeadline(readDeadline)

	// process stream establish header;
	streamEst, err := readStreamEstablishHeader(strm)
	if err != nil {
		if err != io.EOF && err != context.Canceled {
			c.le.WithError(err).Warn("unable to read stream establish header")
		}
		strm.Close()
		return
	}
	_ = strm.SetDeadline(time.Time{})

	// received stream establish header, now, create handlestream directive
	pid := protocol.ID(streamEst.GetProtocolId())
	if err := pid.Validate(); err != nil {
		c.le.
			WithError(err).
			WithField("stream-protocol-id", streamEst.GetProtocolId()).
			Warn("failed to validate protocol id")
		strm.Close()
		return
	}

	var mlnk link.MountedLink = newMountedLink(c, c.tpt, lnk)
	var mstrm link.MountedStream = newMountedStream(strm, strmOpts, pid, mlnk)

	// bus is the controller bus
	le := c.loggerForLink(lnk).WithField("protocol-id", pid)
	if c.verbose {
		le.Debug("accepted stream")
	}

	dir := link.NewHandleMountedStream(pid, lnk.GetLocalPeer(), mstrm.GetPeerID())

	handleMsCtx, handleMsCtxCancel := context.WithDeadline(rctx, handleDeadline)
	dval, _, dref, err := bus.ExecOneOff(handleMsCtx, c.bus, dir, nil, nil)
	handleMsCtxCancel()
	if err != nil {
		le.WithError(err).Warn("error retrieving stream handler for stream")
		strm.Close()
		return
	}
	defer dref.Release()

	mhnd, ok := dval.GetValue().(link.MountedStreamHandler)
	if !ok {
		c.le.
			WithError(err).
			WithField("protocol-id", pid).
			Warn("stream handler retrieved is not a link.MountedStreamHandler")
		strm.Close()
		return
	}

	if err := mhnd.HandleMountedStream(rctx, mstrm); err != nil {
		c.le.
			WithError(err).
			WithField("protocol-id", pid).
			Warn("stream handler returned an error")
		strm.Close()
		return
	}

	// stream is now handled by the handler.
}

// DialPeerAddr pushes a new dialer dialing a peer at an address.
// Waits for the transport to be constructed.
// Waits for the link to be established and returns the link.
// If the transport is not a TransportDialer, returns ErrNotTransportDialer.
func (c *Controller) DialPeerAddr(ctx context.Context, peerID peer.ID, opts *dialer.DialerOpts) (link.Link, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}
	if err := peerID.Validate(); err != nil {
		return nil, err
	}

	tpt, err := c.GetTransport(ctx)
	if err != nil {
		return nil, err
	}

	tptDialer, ok := tpt.(dialer.TransportDialer)
	if !ok {
		return nil, dialer.ErrNotTransportDialer
	}
	_ = tptDialer

	ref, dial, _ := c.linkDialers.AddKeyRef(linkDialerKey{peerID: peerID, dialAddress: opts.GetAddress()})
	defer ref.Release()

	_ = dial.opts.SetResult(opts, nil)

	return dial.lnk.WaitValue(ctx, nil)
}

// flushEstablishedLink closes an established link and cleans it up.
// mtx is locked by caller
func (c *Controller) flushEstablishedLink(el *establishedLink, hasNextLink bool) {
	le := c.loggerForLink(el.lnk)
	le.Info("link lost/closed")

	delete(c.links, el.lnk.GetUUID())

	peerID := el.lnk.GetRemotePeer()
	peerLinks := c.linksByPeerID[peerID]
	for i, plnk := range peerLinks {
		if plnk == el {
			if len(peerLinks) == 1 {
				peerLinks = nil
			} else {
				peerLinks[i] = peerLinks[len(peerLinks)-1]
				peerLinks[len(peerLinks)-1] = nil
				peerLinks = peerLinks[:len(peerLinks)-1]
			}
			break
		}
	}
	if len(peerLinks) == 0 {
		delete(c.linksByPeerID, peerID)
	} else {
		c.linksByPeerID[peerID] = peerLinks
	}

	el.cancel()

	// close the directive if unreferenced (skipping unref dispose dir)
	if !hasNextLink {
		_ = el.di.CloseIfUnreferenced(false)
	}

	// clear lnk from any dialers that were resolved with it.
	c.linkDialers.RestartAllRoutines(func(lk linkDialerKey, ld *linkDialer) bool {
		if lk.peerID != peerID {
			return false
		}
		if ld.lnk.GetValue() != el.lnk {
			return false
		}

		// clear the lnk value and restart if we dont already have a new link.
		ld.lnk.SetValue(nil)
		return !hasNextLink
	})

	// Call close on the link on a separate goroutine.
	go func() {
		_ = el.lnk.Close()
	}()
}

// loggerForLink wraps a logger with fields identifying the link.
func (c *Controller) loggerForLink(lnk link.Link) *logrus.Entry {
	return c.le.
		WithField("link-uuid", lnk.GetUUID()).
		WithField("link-peer", lnk.GetRemotePeer().String()).
		WithField("tpt-uuid", lnk.GetTransportUUID())
}

// Close releases any resources used by the controller.
// Error indicates any issue encountered releasing.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ transport.Controller = ((*Controller)(nil))

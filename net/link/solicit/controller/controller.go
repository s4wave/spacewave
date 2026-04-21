package link_solicit_controller

import (
	"bytes"
	"context"
	"encoding/hex"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/keyed"
	"github.com/blang/semver/v4"
	"github.com/s4wave/spacewave/net/link"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = semver.MustParse("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "bifrost/link/solicit"

// ControlProtocolID is the protocol ID for the solicitation control stream.
const ControlProtocolID = protocol.ID("bifrost/solicit")

// SolicitStreamPrefix is the protocol ID prefix for solicited streams.
const SolicitStreamPrefix = "solicit:"

// maxMessageSize is the max message size for packet session.
const maxMessageSize = 256 * 32 * 2 // ~16KB, enough for 256 hashes

// Controller is the solicitation controller.
type Controller struct {
	le        *logrus.Entry
	maxHashes uint32

	bcast broadcast.Broadcast
	// linkRoutines manages per-link control stream goroutines.
	linkRoutines *keyed.Keyed[uint64, struct{}]
	// solicitations tracks active SolicitProtocol directives.
	// guarded by bcast
	solicitations map[*solicitState]struct{}
	// links tracks active link states.
	// guarded by bcast
	links map[uint64]*linkState // key: link UUID
}

// solicitState tracks a single SolicitProtocol directive resolver.
type solicitState struct {
	dir     link_solicit.SolicitProtocol
	handler directive.ResolverHandler
}

// linkState tracks per-link solicitation state.
type linkState struct {
	le           *logrus.Entry
	ml           link.MountedLink
	sessionID    []byte
	localIsLower bool

	// guarded by Controller.bcast
	remoteHashes [][]byte
	matched      map[string]struct{} // hex hash -> already matched
}

// NewController constructs a new solicitation controller.
func NewController(le *logrus.Entry, conf *Config) (*Controller, error) {
	c := &Controller{
		le:            le,
		maxHashes:     conf.GetMaxHashesOrDefault(),
		solicitations: make(map[*solicitState]struct{}),
		links:         make(map[uint64]*linkState),
	}
	c.linkRoutines = keyed.NewKeyed(c.buildLinkRoutine,
		keyed.WithExitLogger[uint64, struct{}](le),
	)
	return c, nil
}

// buildLinkRoutine constructs the keyed routine for a link UUID.
func (c *Controller) buildLinkRoutine(uuid uint64) (keyed.Routine, struct{}) {
	var ls *linkState
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ls = c.links[uuid]
	})
	if ls == nil || !ls.localIsLower {
		return nil, struct{}{}
	}
	return func(ctx context.Context) error {
		return c.initiateControlStream(ctx, ls)
	}, struct{}{}
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	c.le.Debug("solicitation controller running")
	c.linkRoutines.SetContext(ctx, true)
	return nil
}

// HandleDirective asks if the handler can resolve the directive.
func (c *Controller) HandleDirective(
	ctx context.Context,
	di directive.Instance,
) ([]directive.Resolver, error) {
	dir := di.GetDirective()
	switch d := dir.(type) {
	case link_solicit.SolicitProtocol:
		return c.handleSolicitProtocol(ctx, di, d)
	case link.HandleMountedStream:
		return c.handleMountedStream(ctx, di, d)
	case link.EstablishLinkWithPeer:
		return c.handleEstablishLink(ctx, di, d)
	}
	return nil, nil
}

// handleSolicitProtocol returns a resolver for a SolicitProtocol directive.
func (c *Controller) handleSolicitProtocol(
	_ context.Context,
	_ directive.Instance,
	d link_solicit.SolicitProtocol,
) ([]directive.Resolver, error) {
	return directive.Resolvers(directive.NewFuncResolver(func(
		rctx context.Context,
		rh directive.ResolverHandler,
	) error {
		ss := &solicitState{dir: d, handler: rh}
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			c.solicitations[ss] = struct{}{}
			broadcast()
		})

		defer func() {
			c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
				delete(c.solicitations, ss)
				broadcast()
			})
		}()

		rh.MarkIdle(true)
		<-rctx.Done()
		return nil
	})), nil
}

// handleMountedStream returns a resolver for HandleMountedStream directives
// matching bifrost/solicit or solicit:{hash} protocol IDs.
func (c *Controller) handleMountedStream(
	_ context.Context,
	_ directive.Instance,
	d link.HandleMountedStream,
) ([]directive.Resolver, error) {
	pid := d.HandleMountedStreamProtocolID()

	if pid == ControlProtocolID {
		handler := &controlStreamMountedHandler{c: c}
		return directive.Resolvers(
			directive.NewValueResolver([]link.MountedStreamHandler{handler}),
		), nil
	}

	if strings.HasPrefix(string(pid), SolicitStreamPrefix) {
		hashHex := string(pid)[len(SolicitStreamPrefix):]
		handler := &solicitedStreamMountedHandler{c: c, hashHex: hashHex}
		return directive.Resolvers(
			directive.NewValueResolver([]link.MountedStreamHandler{handler}),
		), nil
	}

	return nil, nil
}

// handleEstablishLink watches EstablishLinkWithPeer for link values.
func (c *Controller) handleEstablishLink(
	_ context.Context,
	di directive.Instance,
	_ link.EstablishLinkWithPeer,
) ([]directive.Resolver, error) {
	ref := di.AddReference(
		directive.NewTypedCallbackHandler[link.MountedLink](
			func(v directive.TypedAttachedValue[link.MountedLink]) {
				c.addLink(v.GetValue())
			},
			func(v directive.TypedAttachedValue[link.MountedLink]) {
				c.removeLink(v.GetValue().GetLinkUUID())
			},
			nil, nil,
		),
		true,
	)
	di.AddDisposeCallback(func() {
		ref.Release()
	})
	return nil, nil
}

// addLink registers a new link for solicitation.
func (c *Controller) addLink(ml link.MountedLink) {
	uuid := ml.GetLinkUUID()
	localPeer := ml.GetLocalPeer()
	remotePeer := ml.GetRemotePeer()

	sessionID := link_solicit.ComputeSessionID(localPeer, remotePeer)
	isLower := localPeer < remotePeer

	le := c.le.WithField("link-uuid", uuid).
		WithField("remote-peer", remotePeer.String()).
		WithField("is-lower", isLower)

	ls := &linkState{
		le:           le,
		ml:           ml,
		sessionID:    sessionID,
		localIsLower: isLower,
		matched:      make(map[string]struct{}),
	}

	var added bool
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if _, exists := c.links[uuid]; exists {
			return
		}
		c.links[uuid] = ls
		added = true
		broadcast()
	})
	if !added {
		return
	}

	le.Debug("link added for solicitation")
	c.linkRoutines.SetKey(uuid, true)
}

// removeLink removes a link from solicitation tracking.
func (c *Controller) removeLink(uuid uint64) {
	var ls *linkState
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		ls = c.links[uuid]
		delete(c.links, uuid)
		if ls != nil {
			broadcast()
		}
	})

	if ls != nil {
		ls.le.Debug("link removed from solicitation")
		c.linkRoutines.RemoveKey(uuid)
	}
}

// initiateControlStream opens the bifrost/solicit control stream on a link.
// Only called by the lower peer ID side via keyed routine.
func (c *Controller) initiateControlStream(ctx context.Context, ls *linkState) error {
	ms, err := ls.ml.OpenMountedStream(ctx, ControlProtocolID, stream.OpenOpts{})
	if err != nil {
		ls.le.WithError(err).Warn("failed to open control stream")
		return err
	}

	sess := stream_packet.NewSession(ms.GetStream(), maxMessageSize)
	c.runControlStream(ctx, ls, sess)
	return nil
}

// getSolicitEntries returns the current set of solicit entries
// for a given link, filtering by peer and transport constraints.
// Caller must hold bcast lock.
func (c *Controller) getSolicitEntries(ml link.MountedLink) []link_solicit.SolicitEntry {
	remotePeer := ml.GetRemotePeer()
	transportUUID := ml.GetTransportUUID()

	var entries []link_solicit.SolicitEntry
	for ss := range c.solicitations {
		if pid := ss.dir.SolicitProtocolPeerID(); len(pid) != 0 && pid != remotePeer {
			continue
		}
		if tid := ss.dir.SolicitProtocolTransportID(); tid != 0 && tid != transportUUID {
			continue
		}
		entries = append(entries, link_solicit.SolicitEntry{
			ProtocolID: ss.dir.SolicitProtocolID(),
			Context:    ss.dir.SolicitProtocolContext(),
		})
	}
	return entries
}

// computeHashes computes sorted hashes for the given entries.
func (c *Controller) computeHashes(ls *linkState, entries []link_solicit.SolicitEntry) [][]byte {
	if len(entries) == 0 {
		return nil
	}
	hashes := link_solicit.ComputeProtocolHashes(ls.sessionID, entries)
	if len(hashes) > int(c.maxHashes) {
		hashes = hashes[:c.maxHashes]
	}
	return hashes
}

// resolveMatch finds SolicitProtocol directives that match a given hash
// on a link and emits SolicitMountedStream values.
func (c *Controller) resolveMatch(ls *linkState, hashBytes []byte, ms link.MountedStream) {
	hashHex := hex.EncodeToString(hashBytes)

	var matches []*solicitState
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for ss := range c.solicitations {
			if pid := ss.dir.SolicitProtocolPeerID(); len(pid) != 0 && pid != ls.ml.GetRemotePeer() {
				continue
			}
			if tid := ss.dir.SolicitProtocolTransportID(); tid != 0 && tid != ls.ml.GetTransportUUID() {
				continue
			}

			h := link_solicit.ComputeProtocolHash(
				ls.sessionID,
				ss.dir.SolicitProtocolID(),
				ss.dir.SolicitProtocolContext(),
			)
			if bytes.Equal(h, hashBytes) {
				matches = append(matches, ss)
			}
		}
	})

	for _, ss := range matches {
		sms := link_solicit.NewSolicitMountedStream(ms)
		if _, ok := ss.handler.AddValue(sms); ok {
			ls.le.WithField("hash", hashHex).Debug("emitted SolicitMountedStream value")
		}
	}
}

// openSolicitedStream opens a solicited stream for a matched hash.
func (c *Controller) openSolicitedStream(ctx context.Context, ls *linkState, hashBytes []byte) {
	hashHex := hex.EncodeToString(hashBytes)
	pid := protocol.ID(SolicitStreamPrefix + hashHex)

	ms, err := ls.ml.OpenMountedStream(ctx, pid, stream.OpenOpts{})
	if err != nil {
		ls.le.WithError(err).WithField("hash", hashHex).Warn("failed to open solicited stream")
		return
	}

	c.resolveMatch(ls, hashBytes, ms)
}

// handleIncomingSolicitedStream routes an incoming solicited stream.
func (c *Controller) handleIncomingSolicitedStream(
	hashHex string,
	ms link.MountedStream,
) {
	hashBytes, err := hex.DecodeString(hashHex)
	if err != nil {
		c.le.WithError(err).Warn("invalid solicited stream hash")
		ms.GetStream().Close()
		return
	}

	lnk := ms.GetLink()
	uuid := lnk.GetLinkUUID()

	var ls *linkState
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ls = c.links[uuid]
	})

	if ls == nil {
		c.le.Warn("solicited stream for unknown link")
		ms.GetStream().Close()
		return
	}

	c.resolveMatch(ls, hashBytes, ms)
}

// runControlStream manages the control stream exchange for a link.
//
// The loop follows the standard broadcast wait pattern: all state is
// read atomically with the wait channel so that any broadcast firing
// after the lock release is guaranteed to wake the select.
func (c *Controller) runControlStream(
	ctx context.Context,
	ls *linkState,
	sess *stream_packet.Session,
) {
	le := ls.le.WithField("phase", "control-stream")
	le.Debug("control stream started")
	defer sess.Close()

	// Read incoming exchanges in a goroutine.
	remoteCh := make(chan [][]byte, 1)
	go func() {
		defer close(remoteCh)
		for {
			var msg link_solicit.SolicitationExchange
			if err := sess.RecvMsg(&msg); err != nil {
				le.WithError(err).Debug("control stream read ended")
				return
			}
			hashes := msg.GetProtocolHashes()
			if len(hashes) > int(c.maxHashes) {
				hashes = hashes[:c.maxHashes]
			}
			select {
			case remoteCh <- hashes:
			case <-ctx.Done():
				return
			}
		}
	}()

	var localHashes [][]byte
	for {
		// Snapshot all state and wait channel atomically.
		// Any broadcast after this lock release closes ch.
		var ch <-chan struct{}
		var entries []link_solicit.SolicitEntry
		var rh [][]byte
		var linkRemoved bool
		c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			ch = getWaitCh()
			if _, ok := c.links[ls.ml.GetLinkUUID()]; !ok {
				linkRemoved = true
				return
			}
			entries = c.getSolicitEntries(ls.ml)
			rh = ls.remoteHashes
		})
		if linkRemoved {
			return
		}

		// Compute hashes outside the lock (pure computation).
		newHashes := c.computeHashes(ls, entries)

		// Send exchange if local hashes changed.
		if !slices.EqualFunc(localHashes, newHashes, bytes.Equal) {
			le.WithField("hash-count", len(newHashes)).Debug("sending exchange")
			localHashes = newHashes
			if err := c.sendExchange(sess, localHashes); err != nil {
				le.WithError(err).Debug("failed to send exchange")
				return
			}
		}

		// Evaluate matches if we have remote hashes.
		if rh != nil {
			c.evaluateMatches(ctx, ls, localHashes, rh)
		}

		select {
		case <-ctx.Done():
			return
		case remoteHashes, ok := <-remoteCh:
			if !ok {
				return
			}
			le.WithField("remote-hash-count", len(remoteHashes)).Debug("received remote exchange")
			c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				ls.remoteHashes = remoteHashes
			})
			c.evaluateMatches(ctx, ls, localHashes, remoteHashes)
		case <-ch:
		}
	}
}

// evaluateMatches finds the intersection and opens streams for new matches.
func (c *Controller) evaluateMatches(
	ctx context.Context,
	ls *linkState,
	localHashes, remoteHashes [][]byte,
) {
	matches := link_solicit.FindMatchingHashes(localHashes, remoteHashes)
	ls.le.WithField("local-count", len(localHashes)).
		WithField("remote-count", len(remoteHashes)).
		WithField("match-count", len(matches)).
		Debug("evaluated matches")
	for _, h := range matches {
		hashHex := hex.EncodeToString(h)
		var exists bool
		c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			_, exists = ls.matched[hashHex]
			if !exists {
				ls.matched[hashHex] = struct{}{}
			}
		})
		if exists {
			continue
		}

		if ls.localIsLower {
			go c.openSolicitedStream(ctx, ls, h)
		}
	}
}

// sendExchange sends a SolicitationExchange message.
func (c *Controller) sendExchange(sess *stream_packet.Session, hashes [][]byte) error {
	msg := &link_solicit.SolicitationExchange{
		ProtocolHashes: hashes,
	}
	return sess.SendMsg(msg)
}

// GetControllerInfo returns information about the controller.
func (c *Controller) GetControllerInfo() *controller.Info {
	return controller.NewInfo(
		ControllerID,
		Version,
		"link solicitation controller",
	)
}

// Close releases any resources used by the controller.
func (c *Controller) Close() error {
	return nil
}

// _ is a type assertion
var _ controller.Controller = ((*Controller)(nil))

package link_solicit_controller

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"math"
	"slices"
	"strings"
	"sync"

	"github.com/aperturerobotics/controllerbus/controller"
	"github.com/aperturerobotics/controllerbus/directive"
	"github.com/aperturerobotics/util/broadcast"
	"github.com/aperturerobotics/util/keyed"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/link"
	link_solicit "github.com/s4wave/spacewave/net/link/solicit"
	"github.com/s4wave/spacewave/net/protocol"
	"github.com/s4wave/spacewave/net/stream"
	stream_packet "github.com/s4wave/spacewave/net/stream/packet"
	"github.com/sirupsen/logrus"
)

// Version is the version of the controller implementation.
var Version = controller.MustParseVersion("0.0.1")

// ControllerID is the ID of the controller.
const ControllerID = "bifrost/link/solicit"

// ControlProtocolID is the protocol ID for the solicitation control stream.
const ControlProtocolID = protocol.ID("bifrost/solicit")

// SolicitStreamPrefix is the protocol ID prefix for solicited streams.
const SolicitStreamPrefix = "solicit:"

// solicitAcceptByte is written by the receiver of a solicited stream once it
// accepts the stream's offer pair. The opener publishes the stream only after
// reading it, because its view of the remote incarnation may be stale.
const solicitAcceptByte = 1

// solicitationIncarnationSize is the number of random bytes in an offer incarnation.
const solicitationIncarnationSize = 16

const (
	// encodedProtocolHashSize bounds one repeated 32-byte hash field.
	encodedProtocolHashSize = 34
	// encodedOfferSize bounds one repeated offer with hash and incarnation.
	encodedOfferSize = 54
	// exchangeMetadataSize bounds capability and generation fields.
	exchangeMetadataSize = 32
)

// Controller is the solicitation controller.
type Controller struct {
	// le is the logger for the controller.
	le *logrus.Entry
	// maxHashes is the maximum number of solicit hashes sent per control stream.
	maxHashes uint32

	// bcast guards solicitation registration, link state, and pending opens.
	bcast broadcast.Broadcast
	// linkRoutines manages per-link control stream goroutines.
	linkRoutines *keyed.Keyed[uint64, struct{}]
	// openRoutines manages solicited stream openings outside control loops.
	openRoutines *keyed.Keyed[solicitationOpenKey, struct{}]
	// solicitations tracks active SolicitProtocol directives, guarded by bcast.
	solicitations map[*solicitState]struct{}
	// links indexes active link states by UUID, guarded by bcast.
	links map[uint64]*linkState
	// opens owns pending solicited stream openings, guarded by bcast.
	opens map[solicitationOpenKey]solicitationOpen
	// settledMtx serializes settlement reports so the last report carries
	// the latest state.
	settledMtx sync.Mutex
}

// solicitState tracks a single SolicitProtocol directive resolver.
type solicitState struct {
	// dir is the continuous solicitation directive represented by this state.
	dir link_solicit.SolicitProtocol
	// handler receives the stream produced for each bilateral incarnation.
	handler directive.ResolverHandler
	// incarnation distinguishes this lifetime from equivalent successors.
	incarnation []byte
	// disposed prevents publication after disposal, guarded by Controller.bcast.
	disposed bool
}

// linkState tracks per-link solicitation state.
type linkState struct {
	// le is the logger for this link.
	le *logrus.Entry
	// ml is the mounted link being solicited over.
	ml link.MountedLink
	// sessionID is the derived control-stream session ID.
	sessionID []byte
	// localIsLower records whether our peer ID sorts below the remote's.
	localIsLower bool
	// refCount counts active users of this state, guarded by Controller.bcast.
	refCount int

	// controlStreams counts running control streams, guarded by Controller.bcast.
	controlStreams int
	// controlEnded records that a control stream ended or failed to open,
	// guarded by Controller.bcast. Until then the link is still negotiating.
	controlEnded bool
	// localExchange is the offer generation this side last sent on the current
	// control stream. It is guarded by Controller.bcast.
	localExchange *solicitationExchange
	// remoteExchange is the peer's most recently advertised offer generation
	// on the current control stream. It is guarded by Controller.bcast.
	remoteExchange *solicitationExchange
	// matched suppresses duplicate streams for each bilateral incarnation pair
	// and records whether the pair's stream was delivered or refused. It is
	// guarded by Controller.bcast.
	matched map[string]bool
}

// controlStreamLocalSnapshot captures one local offer-set observation.
type controlStreamLocalSnapshot struct {
	// linkRemoved indicates that this link state no longer owns its UUID.
	linkRemoved bool
	// offers contains the current local directive incarnations for the link.
	offers []solicitationOffer
}

// solicitationOffer binds stable discovery identity to one directive lifetime.
type solicitationOffer struct {
	// protocolID identifies the requested application protocol.
	protocolID protocol.ID
	// context distinguishes independent uses of the same protocol.
	context []byte
	// hash is the stable link-scoped discovery identity.
	hash []byte
	// incarnation distinguishes equivalent offers across directive lifetimes.
	incarnation []byte
}

// solicitationExchange is one complete control-stream offer generation.
type solicitationExchange struct {
	// hashes carries stable discovery identities for legacy peers.
	hashes [][]byte
	// offers carries incarnation-bound identities for upgraded peers.
	offers []solicitationOffer
	// supportsOfferIncarnations selects the incarnation-aware match contract.
	supportsOfferIncarnations bool
	// generation changes whenever this side's offer set changes.
	generation uint64
	// acknowledgedGeneration is the latest remote generation observed.
	acknowledgedGeneration uint64
}

// solicitationMatch binds a stable hash to both participating offer lifetimes.
type solicitationMatch struct {
	// hash is the stable discovery identity shared by both peers.
	hash []byte
	// localIncarnation identifies the local offer lifetime.
	localIncarnation []byte
	// remoteIncarnation identifies the remote offer lifetime.
	remoteIncarnation []byte
	// incarnated distinguishes upgraded matches from legacy hash-only matches.
	incarnated bool
}

// solicitationOpenKey identifies one pending open on one physical link.
type solicitationOpenKey struct {
	// linkUUID identifies the physical link that owns the pending open.
	linkUUID uint64
	// match is the canonical bilateral incarnation key.
	match string
}

// solicitationOpen contains the state needed by a one-shot keyed open routine.
type solicitationOpen struct {
	// ls owns the physical link used to open the stream.
	ls *linkState
	// match binds the stream to the current bilateral offer lifetimes.
	match solicitationMatch
}

// NewController constructs a new solicitation controller.
func NewController(le *logrus.Entry, conf *Config) (*Controller, error) {
	// Allocate the shared state before constructing its keyed routines.
	c := &Controller{
		le:            le,
		maxHashes:     conf.GetMaxHashesOrDefault(),
		solicitations: make(map[*solicitState]struct{}),
		links:         make(map[uint64]*linkState),
		opens:         make(map[solicitationOpenKey]solicitationOpen),
	}

	// Keep control streams and one-shot opens under their keyed lifetimes.
	c.linkRoutines = keyed.NewKeyed(c.buildLinkRoutine,
		keyed.WithExitLogger[uint64, struct{}](le),
	)
	c.openRoutines = keyed.NewKeyed(
		c.buildOpenRoutine,
		keyed.WithExitLogger[solicitationOpenKey, struct{}](le),
		keyed.WithExitCb(func(
			key solicitationOpenKey,
			_ keyed.Routine,
			_ struct{},
			_ error,
		) {
			c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
				delete(c.opens, key)
			})
			c.openRoutines.RemoveKey(key)
		}),
	)
	return c, nil
}

// maxExchangeMessageSize returns a packet bound for the configured maximum
// stable hashes and incarnated offers, including exchange metadata.
func maxExchangeMessageSize(maxHashes uint32) uint32 {
	entrySize := max(encodedOfferSize, encodedProtocolHashSize)
	size := uint64(maxHashes)*uint64(entrySize) + exchangeMetadataSize
	if size > math.MaxUint32 {
		return math.MaxUint32
	}
	return uint32(size)
}

// buildLinkRoutine constructs the keyed routine for a link UUID.
func (c *Controller) buildLinkRoutine(uuid uint64) (keyed.Routine, struct{}) {
	// Snapshot the link state under the broadcast lock.
	var ls *linkState
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ls = c.links[uuid]
	})
	if ls == nil || !ls.localIsLower {
		return nil, struct{}{}
	}

	// Run the control stream only from the lower peer side.
	return func(ctx context.Context) error {
		return c.initiateControlStream(ctx, ls)
	}, struct{}{}
}

// buildOpenRoutine constructs a one-shot solicited stream opening routine.
func (c *Controller) buildOpenRoutine(key solicitationOpenKey) (keyed.Routine, struct{}) {
	// Snapshot the registered opening while its metadata remains available.
	var open solicitationOpen
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		open = c.opens[key]
	})
	if open.ls == nil {
		return nil, struct{}{}
	}

	// Run one opening; the keyed exit callback retires its metadata.
	return func(ctx context.Context) error {
		c.openSolicitedStream(ctx, open.ls, open.match)
		return nil
	}, struct{}{}
}

// Execute executes the controller goroutine.
func (c *Controller) Execute(ctx context.Context) error {
	c.le.Debug("solicitation controller running")
	c.linkRoutines.SetContext(ctx, true)
	c.openRoutines.SetContext(ctx, true)

	// Report settlement whenever solicitation state changes.
	for {
		var waitCh <-chan struct{}
		c.bcast.HoldLock(func(_ func(), getWaitCh func() <-chan struct{}) {
			waitCh = getWaitCh()
		})
		c.reportSettled()
		select {
		case <-ctx.Done():
			return nil
		case <-waitCh:
		}
	}
}

// reportSettled marks each solicitation resolver idle exactly while it has
// settled on every link.
func (c *Controller) reportSettled() {
	// Serialize reports so the last one published carries the latest state.
	c.settledMtx.Lock()
	defer c.settledMtx.Unlock()

	// Snapshot settlement under the state lock.
	type report struct {
		handler directive.ResolverHandler
		settled bool
	}
	var reports []report
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for ss := range c.solicitations {
			reports = append(reports, report{
				handler: ss.handler,
				settled: c.solicitationSettledLocked(ss),
			})
		}
	})

	// Publish outside the state lock because the bus may call back into it.
	for _, r := range reports {
		r.handler.MarkIdle(r.settled)
	}
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
	di directive.Instance,
	d link_solicit.SolicitProtocol,
) ([]directive.Resolver, error) {
	// Bind equivalent protocol demand to this directive's lifetime.
	incarnation := make([]byte, solicitationIncarnationSize)
	if _, err := rand.Read(incarnation); err != nil {
		return nil, errors.Wrap(err, "generate solicitation incarnation")
	}

	ss := &solicitState{dir: d, incarnation: incarnation}
	di.AddDisposeCallback(func() {
		c.removeSolicitation(ss)
	})

	// Publish the offer only while its resolver can receive streams.
	return directive.Resolvers(directive.NewFuncResolver(func(
		rctx context.Context,
		rh directive.ResolverHandler,
	) error {
		// Register the incarnation while its directive remains active.
		c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
			if ss.disposed {
				return
			}
			ss.handler = rh
			c.solicitations[ss] = struct{}{}
			broadcast()
		})
		defer c.removeSolicitation(ss)

		// Execute reports idle while every link has settled this offer.
		<-rctx.Done()
		return nil
	})), nil
}

// solicitationSettledLocked reports whether ss has settled on every link it
// may be offered on. Caller must hold bcast lock.
func (c *Controller) solicitationSettledLocked(ss *solicitState) bool {
	for _, ls := range c.links {
		if ss.offeredOn(ls.ml) && !c.linkSettledLocked(ls, ss) {
			return false
		}
	}
	return true
}

// linkSettledLocked reports whether ss needs no further work on ls: both
// peers acknowledged an exchange that carries its offer, and every stream that
// exchange matched was delivered or refused. A link with no running control
// stream since one ended has nothing left to negotiate. Caller must hold bcast
// lock.
func (c *Controller) linkSettledLocked(ls *linkState, ss *solicitState) bool {
	// Settle a link that is not negotiating.
	if ls.controlStreams == 0 {
		return ls.controlEnded
	}

	// Wait for both peers to observe the exchange carrying this offer.
	local, remote := ls.localExchange, ls.remoteExchange
	if local == nil || remote == nil || !exchangesAcknowledged(local, remote) {
		return false
	}
	idx := slices.IndexFunc(local.offers, func(offer solicitationOffer) bool {
		return bytes.Equal(offer.incarnation, ss.incarnation)
	})
	if idx < 0 {
		// An offer beyond the hash limit is never sent.
		return len(local.offers) >= int(c.maxHashes)
	}

	// Wait for every stream matched to this offer.
	hash := local.offers[idx].hash
	for _, match := range findSolicitationMatches(local, remote) {
		if !bytes.Equal(match.hash, hash) ||
			(match.incarnated && !bytes.Equal(match.localIncarnation, ss.incarnation)) {
			continue
		}
		if !ls.matched[encodeSolicitationMatch(ls, match)] {
			return false
		}
	}
	return true
}

// offeredOn reports whether the peer and transport constraints of ss admit ml.
func (ss *solicitState) offeredOn(ml link.MountedLink) bool {
	if pid := ss.dir.SolicitProtocolPeerID(); len(pid) != 0 && pid != ml.GetRemotePeer() {
		return false
	}
	if tid := ss.dir.SolicitProtocolTransportID(); tid != 0 && tid != ml.GetTransportUUID() {
		return false
	}
	return true
}

// removeSolicitation synchronously and idempotently withdraws an offer.
func (c *Controller) removeSolicitation(ss *solicitState) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		ss.disposed = true
		if _, exists := c.solicitations[ss]; !exists {
			return
		}
		delete(c.solicitations, ss)
		broadcast()
	})
}

// handleMountedStream returns a resolver for HandleMountedStream directives
// matching bifrost/solicit or solicit:{hash} protocol IDs.
func (c *Controller) handleMountedStream(
	_ context.Context,
	_ directive.Instance,
	d link.HandleMountedStream,
) ([]directive.Resolver, error) {
	// Route control and solicited protocol IDs to their handlers.
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
	// Track mounted links through typed add/remove callbacks.
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
	// Snapshot link identity and derive the canonical session state.
	uuid := ml.GetLinkUUID()
	localPeer := ml.GetLocalPeer()
	remotePeer := ml.GetRemotePeer()
	sessionID := link_solicit.ComputeSessionID(localPeer, remotePeer)
	isLower := localPeer < remotePeer
	le := c.le.WithField("link-uuid", uuid).
		WithField("remote-peer", remotePeer.String()).
		WithField("is-lower", isLower)

	// Build the per-link solicitation state.
	ls := &linkState{
		le:           le,
		ml:           ml,
		sessionID:    sessionID,
		localIsLower: isLower,
		matched:      make(map[string]bool),
	}

	// Publish the link once while retaining references for duplicates.
	var added bool
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if existing := c.links[uuid]; existing != nil {
			existing.refCount++
			return
		}
		ls.refCount = 1
		c.links[uuid] = ls
		added = true
		broadcast()
	})

	// Withdraw settlement before the link's discoverer continues, so a read
	// that follows the link waits for its negotiation. A duplicate registration
	// may overtake the first, so it reports too.
	c.reportSettled()

	// Ignore duplicate link registrations.
	if !added {
		return
	}

	// Start the keyed control routine for a newly published link.
	le.Debug("link added for solicitation")
	c.linkRoutines.SetKey(uuid, true)
}

// removeLink removes a link from solicitation tracking.
func (c *Controller) removeLink(uuid uint64) {
	var ls *linkState

	// Decrement the link reference and remove it when the count reaches zero.
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		ls = c.links[uuid]
		if ls != nil {
			ls.refCount--
			if ls.refCount > 0 {
				ls = nil
				return
			}
			delete(c.links, uuid)
			broadcast()
		}
	})

	// Stop the keyed routine after removing the final link reference.
	if ls != nil {
		ls.le.Debug("link removed from solicitation")
		c.linkRoutines.RemoveKey(uuid)
		c.retireLinkOpens(uuid)
	}
}

// retireLinkOpens removes pending metadata and cancels every open for a link.
func (c *Controller) retireLinkOpens(uuid uint64) {
	// Retire pending metadata under the same lock as link removal.
	keys := make(map[solicitationOpenKey]struct{})
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for key := range c.opens {
			if key.linkUUID != uuid {
				continue
			}
			delete(c.opens, key)
			keys[key] = struct{}{}
		}
	})

	// Include openings registered concurrently with metadata retirement.
	for _, key := range c.openRoutines.GetKeys() {
		if key.linkUUID == uuid {
			keys[key] = struct{}{}
		}
	}
	for key := range keys {
		c.openRoutines.RemoveKey(key)
	}
}

// initiateControlStream opens the bifrost/solicit control stream on a link.
// Only called by the lower peer ID side via keyed routine.
func (c *Controller) initiateControlStream(ctx context.Context, ls *linkState) error {
	// Open the control stream and run its exchange loop.
	ms, err := ls.ml.OpenMountedStream(ctx, ControlProtocolID, stream.OpenOpts{})
	if err != nil {
		ls.le.WithError(err).Warn("failed to open control stream")
		c.endControlStream(ls, false)
		return err
	}

	// Run the exchange loop over a packet session on the stream.
	sess := stream_packet.NewSession(ms.GetStream(), maxExchangeMessageSize(c.maxHashes))
	c.runControlStream(ctx, ls, sess)
	return nil
}

// getSolicitationOffers returns the current offers for a link, filtering by
// peer and transport constraints.
// Caller must hold bcast lock.
func (c *Controller) getSolicitationOffers(ml link.MountedLink) []solicitationOffer {
	var offers []solicitationOffer
	for ss := range c.solicitations {
		if !ss.offeredOn(ml) {
			continue
		}
		offers = append(offers, solicitationOffer{
			protocolID:  ss.dir.SolicitProtocolID(),
			context:     ss.dir.SolicitProtocolContext(),
			incarnation: ss.incarnation,
		})
	}
	return offers
}

// watchControlStreamLocalSnapshots watches the local solicit snapshot for a
// link and sends each distinct snapshot to send. Caller must not hold bcast.
func (c *Controller) watchControlStreamLocalSnapshots(
	ctx context.Context,
	ls *linkState,
	send func(*controlStreamLocalSnapshot) error,
) error {
	return broadcast.WatchBroadcastWithEqual(
		ctx,
		&c.bcast,
		func() *controlStreamLocalSnapshot {
			return c.snapshotControlStreamLocalLocked(ls)
		},
		send,
		controlStreamLocalSnapshotsEqual,
	)
}

// snapshotControlStreamLocalLocked snapshots the local solicit entries for a
// link, or marks the link removed. Caller must hold bcast lock.
func (c *Controller) snapshotControlStreamLocalLocked(ls *linkState) *controlStreamLocalSnapshot {
	if !c.controlStreamLinkActiveLocked(ls) {
		return &controlStreamLocalSnapshot{linkRemoved: true}
	}
	return &controlStreamLocalSnapshot{
		offers: cloneSolicitationOffers(c.getSolicitationOffers(ls.ml)),
	}
}

// currentControlStreamLocalSnapshot returns the current local solicit
// snapshot under the bcast lock.
func (c *Controller) currentControlStreamLocalSnapshot(ls *linkState) *controlStreamLocalSnapshot {
	locked := c.bcast.Lock()
	defer locked.Unlock()

	return c.snapshotControlStreamLocalLocked(ls)
}

// currentControlStreamRemoteExchange returns the remote exchange reported by a
// link, or true when the link is no longer active.
func (c *Controller) currentControlStreamRemoteExchange(
	ls *linkState,
) (*solicitationExchange, bool) {
	locked := c.bcast.Lock()
	defer locked.Unlock()

	if !c.controlStreamLinkActiveLocked(ls) {
		return nil, true
	}
	return cloneSolicitationExchange(ls.remoteExchange), false
}

// setControlStreamRemoteExchange stores the remote exchange reported by a link
// and returns false when the link is no longer active.
func (c *Controller) setControlStreamRemoteExchange(
	ls *linkState,
	exchange *solicitationExchange,
) bool {
	// Record the exchange only for the active link state.
	locked := c.bcast.Lock()
	defer locked.Unlock()
	if !c.controlStreamLinkActiveLocked(ls) {
		return false
	}
	ls.remoteExchange = cloneSolicitationExchange(exchange)
	locked.Broadcast()
	return true
}

// setControlStreamLocalExchange records the exchange this side sent on a link.
func (c *Controller) setControlStreamLocalExchange(
	ls *linkState,
	exchange *solicitationExchange,
) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		ls.localExchange = exchange
		broadcast()
	})
}

// beginControlStream starts negotiation on a link from empty exchanges.
func (c *Controller) beginControlStream(ls *linkState) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		ls.controlStreams++
		ls.localExchange, ls.remoteExchange = nil, nil
		broadcast()
	})
}

// endControlStream records that a control stream ended. started reports
// whether beginControlStream counted it.
func (c *Controller) endControlStream(ls *linkState, started bool) {
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if started {
			ls.controlStreams--
		}
		ls.controlEnded = true
		broadcast()
	})
}

// controlStreamLinkActiveLocked returns true if ls is still the tracked
// state for its link UUID. Caller must hold bcast lock.
func (c *Controller) controlStreamLinkActiveLocked(ls *linkState) bool {
	return c.links[ls.ml.GetLinkUUID()] == ls
}

// controlStreamLocalSnapshotsEqual returns true if two local snapshots hold
// the same link-removed flag and offers.
func controlStreamLocalSnapshotsEqual(a, b *controlStreamLocalSnapshot) bool {
	if a == nil || b == nil {
		return a == b
	}
	if a.linkRemoved != b.linkRemoved {
		return false
	}
	return solicitationOffersEqual(a.offers, b.offers)
}

// cloneSolicitationOffers deep-clones and canonically sorts offers.
func cloneSolicitationOffers(offers []solicitationOffer) []solicitationOffer {
	if len(offers) == 0 {
		return nil
	}
	out := make([]solicitationOffer, len(offers))
	for i, offer := range offers {
		out[i] = solicitationOffer{
			protocolID:  offer.protocolID,
			context:     slices.Clone(offer.context),
			hash:        slices.Clone(offer.hash),
			incarnation: slices.Clone(offer.incarnation),
		}
	}
	slices.SortFunc(out, func(a, b solicitationOffer) int {
		if cmp := bytes.Compare(a.hash, b.hash); cmp != 0 {
			return cmp
		}
		if a.protocolID != b.protocolID {
			return strings.Compare(string(a.protocolID), string(b.protocolID))
		}
		if cmp := bytes.Compare(a.context, b.context); cmp != 0 {
			return cmp
		}
		return bytes.Compare(a.incarnation, b.incarnation)
	})
	return out
}

// solicitationOffersEqual returns true if two offer lists match pairwise.
func solicitationOffersEqual(a, b []solicitationOffer) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].protocolID != b[i].protocolID ||
			!bytes.Equal(a[i].context, b[i].context) ||
			!bytes.Equal(a[i].hash, b[i].hash) ||
			!bytes.Equal(a[i].incarnation, b[i].incarnation) {
			return false
		}
	}
	return true
}

// cloneSolicitationExchange deep-clones an exchange.
func cloneSolicitationExchange(exchange *solicitationExchange) *solicitationExchange {
	if exchange == nil {
		return nil
	}
	return &solicitationExchange{
		hashes:                    cloneHashes(exchange.hashes),
		offers:                    cloneSolicitationOffers(exchange.offers),
		supportsOfferIncarnations: exchange.supportsOfferIncarnations,
		generation:                exchange.generation,
		acknowledgedGeneration:    exchange.acknowledgedGeneration,
	}
}

// cloneHashes deep-clones a hash list.
func cloneHashes(hashes [][]byte) [][]byte {
	if len(hashes) == 0 {
		return nil
	}
	out := make([][]byte, len(hashes))
	for i, hash := range hashes {
		out[i] = slices.Clone(hash)
	}
	return out
}

// computeExchange computes the stable hashes and incarnated offers sent for a link.
func (c *Controller) computeExchange(
	ls *linkState,
	offers []solicitationOffer,
) *solicitationExchange {
	// Derive and order the bounded discovery set for this physical link.
	for i := range offers {
		offers[i].hash = link_solicit.ComputeProtocolHash(
			ls.sessionID,
			offers[i].protocolID,
			offers[i].context,
		)
	}
	offers = cloneSolicitationOffers(offers)
	if len(offers) > int(c.maxHashes) {
		offers = offers[:c.maxHashes]
	}

	// Retain stable hashes for the initial legacy-compatible capability probe.
	hashes := make([][]byte, len(offers))
	for i := range offers {
		hashes[i] = slices.Clone(offers[i].hash)
	}
	return &solicitationExchange{
		hashes:                    hashes,
		offers:                    offers,
		supportsOfferIncarnations: true,
	}
}

// matchSolicitations returns the active local incarnations bound to the
// current remote incarnation of match.
func (c *Controller) matchSolicitations(
	ls *linkState,
	match solicitationMatch,
) []*solicitState {
	var matches []*solicitState
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		if !c.controlStreamLinkActiveLocked(ls) ||
			!remoteExchangeContainsMatch(ls, match) {
			return
		}
		for ss := range c.solicitations {
			if !ss.offeredOn(ls.ml) {
				continue
			}

			h := link_solicit.ComputeProtocolHash(
				ls.sessionID,
				ss.dir.SolicitProtocolID(),
				ss.dir.SolicitProtocolContext(),
			)
			if !bytes.Equal(h, match.hash) {
				continue
			}
			if match.incarnated && !bytes.Equal(ss.incarnation, match.localIncarnation) {
				continue
			}
			matches = append(matches, ss)
		}
	})
	return matches
}

// emitMatch publishes ms to each matched solicitation and reports whether any
// accepted it. Call without the broadcast lock because directive callbacks may
// re-enter the controller.
func emitMatch(
	ls *linkState,
	match solicitationMatch,
	ms link.MountedStream,
	matches []*solicitState,
) bool {
	var emitted bool
	for _, ss := range matches {
		sms := link_solicit.NewSolicitMountedStream(ms)
		if _, ok := ss.handler.AddValue(sms); ok {
			emitted = true
			ls.le.WithField("hash", hex.EncodeToString(match.hash)).
				Debug("emitted SolicitMountedStream value")
		}
	}
	return emitted
}

// remoteExchangeContainsMatch reports whether the remote side still advertises
// the incarnation bound into match. Caller must hold the broadcast lock.
func remoteExchangeContainsMatch(ls *linkState, match solicitationMatch) bool {
	if !match.incarnated {
		return ls.remoteExchange == nil ||
			!ls.remoteExchange.supportsOfferIncarnations
	}
	if ls.remoteExchange == nil || !ls.remoteExchange.supportsOfferIncarnations {
		return false
	}
	for _, offer := range ls.remoteExchange.offers {
		if bytes.Equal(offer.hash, match.hash) &&
			bytes.Equal(offer.incarnation, match.remoteIncarnation) {
			return true
		}
	}
	return false
}

// openSolicitedStream opens a stream bound to one bilateral offer pair.
func (c *Controller) openSolicitedStream(
	ctx context.Context,
	ls *linkState,
	match solicitationMatch,
) {
	// Resolve the match however the open ends.
	defer c.resolveMatch(ls, match)

	// Open a protocol bound to the exact bilateral offer pair.
	pid := protocol.ID(SolicitStreamPrefix + encodeSolicitationMatch(ls, match))
	ms, err := ls.ml.OpenMountedStream(ctx, pid, stream.OpenOpts{})
	if err != nil {
		ls.le.WithError(err).WithField("hash", hex.EncodeToString(match.hash)).
			Warn("failed to open solicited stream")
		return
	}

	// Wait for the receiver to accept the pair; it closes stale pairs.
	strm := ms.GetStream()
	stopClose := context.AfterFunc(ctx, func() { _ = strm.Close() })
	var accept [1]byte
	_, err = io.ReadFull(strm, accept[:])
	stopClose()
	if err != nil || accept[0] != solicitAcceptByte {
		ls.le.WithError(err).WithField("hash", hex.EncodeToString(match.hash)).
			Debug("solicited stream not accepted")
		_ = strm.Close()
		return
	}

	// A retired offer cannot retain a newly accepted stream.
	matches := c.matchSolicitations(ls, match)
	if !emitMatch(ls, match, ms, matches) {
		_ = strm.Close()
	}
}

// encodeSolicitationMatch returns the stream protocol suffix for a match.
func encodeSolicitationMatch(ls *linkState, match solicitationMatch) string {
	hashHex := hex.EncodeToString(match.hash)
	if !match.incarnated {
		return hashHex
	}
	lower, higher := match.localIncarnation, match.remoteIncarnation
	if !ls.localIsLower {
		lower, higher = higher, lower
	}
	return hashHex + ":" + hex.EncodeToString(lower) + ":" + hex.EncodeToString(higher)
}

// decodeSolicitationMatch parses a stream protocol suffix for the receiving side.
func decodeSolicitationMatch(ls *linkState, encoded string) (solicitationMatch, error) {
	// Decode the stable protocol hash shared with legacy peers.
	parts := strings.Split(encoded, ":")
	if len(parts) != 1 && len(parts) != 3 {
		return solicitationMatch{}, errors.New("invalid solicitation match field count")
	}
	hash, err := hex.DecodeString(parts[0])
	if err != nil || len(hash) != link_solicit.HashSize {
		return solicitationMatch{}, errors.New("invalid solicitation protocol hash")
	}
	match := solicitationMatch{hash: hash}
	if len(parts) == 1 {
		return match, nil
	}

	// Restore local and remote incarnations from canonical peer order.
	lower, err := hex.DecodeString(parts[1])
	if err != nil || len(lower) != solicitationIncarnationSize {
		return solicitationMatch{}, errors.New("invalid lower solicitation incarnation")
	}
	higher, err := hex.DecodeString(parts[2])
	if err != nil || len(higher) != solicitationIncarnationSize {
		return solicitationMatch{}, errors.New("invalid higher solicitation incarnation")
	}
	match.incarnated = true
	match.localIncarnation, match.remoteIncarnation = higher, lower
	if ls.localIsLower {
		match.localIncarnation, match.remoteIncarnation = lower, higher
	}
	return match, nil
}

// handleIncomingSolicitedStream routes an incoming solicited stream.
func (c *Controller) handleIncomingSolicitedStream(
	encodedMatch string,
	ms link.MountedStream,
) {
	// Locate the tracked physical link before interpreting the stream match.
	uuid := ms.GetLink().GetLinkUUID()
	var ls *linkState
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		ls = c.links[uuid]
	})
	if ls == nil {
		c.le.Warn("solicited stream for unknown link")
		ms.GetStream().Close()
		return
	}

	// Reject malformed or retired offer pairs before publishing the stream.
	match, err := decodeSolicitationMatch(ls, encodedMatch)
	if err != nil {
		c.le.WithError(err).Warn("invalid solicited stream match")
		ms.GetStream().Close()
		return
	}

	// Select the live local incarnations bound to the pair.
	defer c.resolveMatch(ls, match)
	matches := c.matchSolicitations(ls, match)
	if len(matches) == 0 {
		ms.GetStream().Close()
		return
	}

	// Accept the pair before publishing so the opener never holds a stream
	// this side rejected.
	strm := ms.GetStream()
	if _, err := strm.Write([]byte{solicitAcceptByte}); err != nil {
		c.le.WithError(err).Debug("failed to accept solicited stream")
		strm.Close()
		return
	}
	if !emitMatch(ls, match, ms, matches) {
		strm.Close()
	}
}

// resolveMatch records that the stream of a matched pair was delivered or
// refused. Recording an unregistered pair suppresses a later duplicate open.
func (c *Controller) resolveMatch(ls *linkState, match solicitationMatch) {
	matchKey := encodeSolicitationMatch(ls, match)
	c.bcast.HoldLock(func(broadcast func(), _ func() <-chan struct{}) {
		if !c.controlStreamLinkActiveLocked(ls) || ls.matched[matchKey] {
			return
		}
		ls.matched[matchKey] = true
		broadcast()
	})
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
	// Keep both watchers and packet reads within the control stream lifetime.
	le := ls.le.WithField("phase", "control-stream")
	le.Debug("control stream started")
	defer sess.Close()
	c.beginControlStream(ls)
	defer c.endControlStream(ls, true)
	watchCtx, cancelWatch := context.WithCancel(ctx)
	defer cancelWatch()

	// Read incoming exchanges in a goroutine.
	remoteCh := make(chan *solicitationExchange, 1)
	go func() {
		defer close(remoteCh)
		for {
			var msg link_solicit.SolicitationExchange
			if err := sess.RecvMsg(&msg); err != nil {
				le.WithError(err).Debug("control stream read ended")
				return
			}
			exchange := c.decodeExchange(&msg)
			select {
			case remoteCh <- exchange:
			case <-watchCtx.Done():
				return
			}
		}
	}()

	// Observe local offer changes without blocking their registration callbacks.
	localCh := make(chan struct{}, 1)
	localErrCh := make(chan error, 1)
	go func() {
		defer close(localErrCh)
		err := c.watchControlStreamLocalSnapshots(
			watchCtx,
			ls,
			func(*controlStreamLocalSnapshot) error {
				select {
				case localCh <- struct{}{}:
					return nil
				case <-watchCtx.Done():
					return watchCtx.Err()
				}
			},
		)
		if err != nil && watchCtx.Err() == nil {
			localErrCh <- err
		}
	}()

	// Keep generation accounting in the single exchange loop.
	var localExchange *solicitationExchange
	var peerSupportsOfferIncarnations bool
	sendLocalExchange := func(exchange *solicitationExchange) bool {
		// Send the exchange and record it as this side's current offer.
		le.WithField("hash-count", len(exchange.hashes)).Debug("sending exchange")
		if err := c.sendExchange(sess, exchange, peerSupportsOfferIncarnations); err != nil {
			le.WithError(err).Debug("failed to send exchange")
			return false
		}
		localExchange = exchange
		c.setControlStreamLocalExchange(ls, exchange)
		return true
	}
	handleLocalWake := func() bool {
		// Snapshot the current local and remote offers.
		snap := c.currentControlStreamLocalSnapshot(ls)
		if snap.linkRemoved {
			return false
		}
		remoteExchange, linkRemoved := c.currentControlStreamRemoteExchange(ls)
		if linkRemoved {
			return false
		}

		// Advance the local generation only when its offer set changes.
		newExchange := c.computeExchange(ls, snap.offers)
		newExchange.generation = 1
		if localExchange != nil {
			newExchange.generation = localExchange.generation
			newExchange.acknowledgedGeneration = localExchange.acknowledgedGeneration
			if !solicitationOfferSetsEqual(localExchange, newExchange) {
				newExchange.generation++
			}
		}

		// Withdraw retired opens before publishing and matching the new set.
		c.pruneRetiredMatches(ls, newExchange, remoteExchange)
		if !solicitationExchangesEqual(localExchange, newExchange) {
			if !sendLocalExchange(newExchange) {
				return false
			}
		}
		if remoteExchange != nil {
			c.evaluateMatches(ctx, ls, localExchange, remoteExchange)
		}
		return true
	}

	// Send the capability probe before processing the peer's exchanges.
	select {
	case <-ctx.Done():
		return
	case err, ok := <-localErrCh:
		if ok && err != nil {
			le.WithError(err).Debug("local solicitation watch ended")
		}
		return
	case <-localCh:
		if !handleLocalWake() {
			return
		}
	}

	// Serialize offer publication, peer acknowledgements, and match admission.
	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-localErrCh:
			if ok && err != nil {
				le.WithError(err).Debug("local solicitation watch ended")
				return
			}
			localErrCh = nil
		case <-localCh:
			if !handleLocalWake() {
				return
			}
		case remoteExchange, ok := <-remoteCh:
			if !ok {
				return
			}
			le.WithField("remote-hash-count", len(remoteExchange.hashes)).
				Debug("received remote exchange")
			if !c.setControlStreamRemoteExchange(ls, remoteExchange) {
				return
			}
			c.pruneRetiredMatches(ls, localExchange, remoteExchange)
			// The capability probe carried hashes without incarnations. Publish
			// the complete offers as a new generation so its acknowledgement
			// proves the peer can accept the stream's incarnation pair.
			upgraded := !peerSupportsOfferIncarnations && remoteExchange.supportsOfferIncarnations
			peerSupportsOfferIncarnations = remoteExchange.supportsOfferIncarnations
			if remoteExchange.supportsOfferIncarnations &&
				localExchange.supportsOfferIncarnations &&
				(upgraded || localExchange.acknowledgedGeneration != remoteExchange.generation) {
				acknowledged := cloneSolicitationExchange(localExchange)
				if upgraded {
					acknowledged.generation++
				}
				acknowledged.acknowledgedGeneration = remoteExchange.generation
				if !sendLocalExchange(acknowledged) {
					return
				}
			}
			c.evaluateMatches(ctx, ls, localExchange, remoteExchange)
		}
	}
}

// pruneRetiredMatches removes incarnated suppression and pending opens after
// either offer leaves the current bilateral set. Legacy hash suppression stays
// for the lifetime of the physical link.
func (c *Controller) pruneRetiredMatches(
	ls *linkState,
	local, remote *solicitationExchange,
) {
	// Compute the bilateral set that still permits incarnation-bound openings.
	active := make(map[string]struct{})
	if local != nil && remote != nil &&
		local.supportsOfferIncarnations && remote.supportsOfferIncarnations {
		for _, match := range findOfferPairs(local.offers, remote.offers) {
			active[encodeSolicitationMatch(ls, match)] = struct{}{}
		}
	}

	// Retire suppression and pending opens for pairs outside that set.
	var retired []solicitationOpenKey
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		for matchKey := range ls.matched {
			if strings.Count(matchKey, ":") != 2 {
				continue
			}
			if _, exists := active[matchKey]; exists {
				continue
			}
			delete(ls.matched, matchKey)
			openKey := solicitationOpenKey{
				linkUUID: ls.ml.GetLinkUUID(),
				match:    matchKey,
			}
			if _, exists := c.opens[openKey]; exists {
				delete(c.opens, openKey)
				retired = append(retired, openKey)
			}
		}
	})

	// Cancel keyed work outside the state lock.
	for _, key := range retired {
		c.openRoutines.RemoveKey(key)
	}
}

// evaluateMatches finds the intersection and opens each bilateral incarnation once.
func (c *Controller) evaluateMatches(
	ctx context.Context,
	ls *linkState,
	local, remote *solicitationExchange,
) {
	// Only mutually acknowledged offer generations may admit streams.
	if local == nil || remote == nil {
		return
	}
	matches := findSolicitationMatches(local, remote)
	ls.le.WithField("local-count", len(local.hashes)).
		WithField("remote-count", len(remote.hashes)).
		WithField("match-count", len(matches)).
		Debug("evaluated matches")

	// Register each pair once and open it from the lower peer.
	for _, match := range matches {
		matchKey := encodeSolicitationMatch(ls, match)
		var exists, linkActive bool
		c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
			linkActive = c.controlStreamLinkActiveLocked(ls)
			if !linkActive {
				return
			}
			_, exists = ls.matched[matchKey]
			if !exists {
				ls.matched[matchKey] = false
				if ls.localIsLower {
					key := solicitationOpenKey{
						linkUUID: ls.ml.GetLinkUUID(),
						match:    matchKey,
					}
					c.opens[key] = solicitationOpen{ls: ls, match: match}
				}
			}
		})
		if !linkActive || exists {
			continue
		}

		if ls.localIsLower {
			key := solicitationOpenKey{
				linkUUID: ls.ml.GetLinkUUID(),
				match:    matchKey,
			}
			c.startOpenRoutine(key)
		}
	}
}

// startOpenRoutine registers an opening and removes the keyed entry if its
// metadata or link was retired before registration completed.
func (c *Controller) startOpenRoutine(key solicitationOpenKey) {
	// Register the keyed routine before checking for concurrent retirement.
	c.openRoutines.SetKey(key, true)

	// Remove work whose metadata or physical link no longer exists.
	var retained bool
	c.bcast.HoldLock(func(_ func(), _ func() <-chan struct{}) {
		open, exists := c.opens[key]
		retained = exists && c.links[key.linkUUID] == open.ls
	})
	if !retained {
		c.openRoutines.RemoveKey(key)
	}
}

// sendExchange sends a SolicitationExchange message.
func (c *Controller) sendExchange(
	sess *stream_packet.Session,
	exchange *solicitationExchange,
	sendOffers bool,
) error {
	// Probe using legacy hashes until the peer advertises incarnation support.
	msg := &link_solicit.SolicitationExchange{
		ProtocolHashes:            exchange.hashes,
		SupportsOfferIncarnations: true,
		Generation:                exchange.generation,
		AcknowledgedGeneration:    exchange.acknowledgedGeneration,
	}

	// Send complete offers within the negotiated packet bound after upgrade.
	if sendOffers {
		msg.ProtocolHashes = nil
		msg.Offers = make([]*link_solicit.SolicitationOffer, len(exchange.offers))
		for i, offer := range exchange.offers {
			msg.Offers[i] = &link_solicit.SolicitationOffer{
				ProtocolHash: slices.Clone(offer.hash),
				Incarnation:  slices.Clone(offer.incarnation),
			}
		}
	}
	return sess.SendMsg(msg)
}

// decodeExchange validates and normalizes a received exchange.
func (c *Controller) decodeExchange(
	msg *link_solicit.SolicitationExchange,
) *solicitationExchange {
	// Normalize and bound the legacy hash set.
	hashes := cloneHashes(msg.GetProtocolHashes())
	if len(hashes) > int(c.maxHashes) {
		hashes = hashes[:c.maxHashes]
	}
	link_solicit.SortHashes(hashes)

	// Validate incarnation pairs before deriving their stable discovery hashes.
	offers := make([]solicitationOffer, 0, len(msg.GetOffers()))
	for _, wireOffer := range msg.GetOffers() {
		if len(wireOffer.GetProtocolHash()) != link_solicit.HashSize ||
			len(wireOffer.GetIncarnation()) != solicitationIncarnationSize {
			continue
		}
		offers = append(offers, solicitationOffer{
			hash:        slices.Clone(wireOffer.GetProtocolHash()),
			incarnation: slices.Clone(wireOffer.GetIncarnation()),
		})
	}
	offers = cloneSolicitationOffers(offers)
	if len(offers) > int(c.maxHashes) {
		offers = offers[:c.maxHashes]
	}
	if len(hashes) == 0 && msg.GetSupportsOfferIncarnations() {
		hashes = make([][]byte, len(offers))
		for i := range offers {
			hashes[i] = slices.Clone(offers[i].hash)
		}
	}
	return &solicitationExchange{
		hashes:                    hashes,
		offers:                    offers,
		supportsOfferIncarnations: msg.GetSupportsOfferIncarnations(),
		generation:                msg.GetGeneration(),
		acknowledgedGeneration:    msg.GetAcknowledgedGeneration(),
	}
}

// solicitationExchangesEqual reports whether exchanges advertise identical state.
func solicitationExchangesEqual(a, b *solicitationExchange) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.supportsOfferIncarnations == b.supportsOfferIncarnations &&
		a.generation == b.generation &&
		a.acknowledgedGeneration == b.acknowledgedGeneration &&
		slices.EqualFunc(a.hashes, b.hashes, bytes.Equal) &&
		solicitationOffersEqual(a.offers, b.offers)
}

// solicitationOfferSetsEqual reports whether exchanges advertise the same offers.
func solicitationOfferSetsEqual(a, b *solicitationExchange) bool {
	return slices.EqualFunc(a.hashes, b.hashes, bytes.Equal) &&
		solicitationOffersEqual(a.offers, b.offers)
}

// findSolicitationMatches returns incarnated matches when both peers support
// them and stable-hash matches for legacy peers.
func findSolicitationMatches(
	local, remote *solicitationExchange,
) []solicitationMatch {
	if !exchangesAcknowledged(local, remote) {
		return nil
	}

	// Legacy peers suppress matches by stable hash for the physical link lifetime.
	if !local.supportsOfferIncarnations || !remote.supportsOfferIncarnations {
		hashes := link_solicit.FindMatchingHashes(local.hashes, remote.hashes)
		matches := make([]solicitationMatch, len(hashes))
		for i, hash := range hashes {
			matches[i] = solicitationMatch{hash: hash}
		}
		return matches
	}
	return findOfferPairs(local.offers, remote.offers)
}

// exchangesAcknowledged reports whether the exchanges may admit matches.
// Legacy peers do not acknowledge generations; upgraded peers must each have
// observed the other's complete offer generation.
func exchangesAcknowledged(local, remote *solicitationExchange) bool {
	if !local.supportsOfferIncarnations || !remote.supportsOfferIncarnations {
		return true
	}
	return local.acknowledgedGeneration == remote.generation &&
		remote.acknowledgedGeneration == local.generation
}

// findOfferPairs returns every matching stable hash bound to both incarnations.
func findOfferPairs(local, remote []solicitationOffer) []solicitationMatch {
	var matches []solicitationMatch
	for _, localOffer := range local {
		for _, remoteOffer := range remote {
			if !bytes.Equal(localOffer.hash, remoteOffer.hash) {
				continue
			}
			matches = append(matches, solicitationMatch{
				hash:              slices.Clone(localOffer.hash),
				localIncarnation:  slices.Clone(localOffer.incarnation),
				remoteIncarnation: slices.Clone(remoteOffer.incarnation),
				incarnated:        true,
			})
		}
	}
	return matches
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
var _ controller.Controller = (*Controller)(nil)

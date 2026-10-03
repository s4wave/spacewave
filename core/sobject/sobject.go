package sobject

import (
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/refcount"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ProcessOpsFunc applies decoded operations to the state of a non-World
// shared object. Replay calls it with one operation at a time.
// If rawNextStateData is nil, the state is unchanged.
type ProcessOpsFunc = func(
	ctx context.Context,
	snap SharedObjectStateSnapshot,
	currentStateData []byte,
	ops []*SOOperationInner,
) (rawNextStateData *[]byte, opResults []*SOOperationResult, err error)

// SharedObject is the shared object handle interface.
//
// This is the interface exposed by the provider on the "client side."
type SharedObject interface {
	// GetBus returns the bus used by the session.
	GetBus() bus.Bus

	// GetPeerID returns the peer ID attached to this SharedObject handle.
	GetPeerID() peer.ID

	// GetSharedObjectID returns the shared object id.
	GetSharedObjectID() string

	// GetBlockStore returns the block store mounted along with the SharedObject.
	GetBlockStore() bstore.BlockStore

	// AccessLocalStateStore accesses a kvtx ops for a local state store with the given ID.
	// This state store is stored along with the local SharedObject state.
	AccessLocalStateStore(ctx context.Context, storeID string, released func()) (kvtx.Store, func(), error)

	// GetSharedObjectState returns a snapshot of the shared object state.
	GetSharedObjectState(ctx context.Context) (SharedObjectStateSnapshot, error)

	// AccessSharedObjectState adds a reference to the state and returns the state container.
	// Returns a release function. Accepts a function that is called if the Watchable becomes invalid.
	AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[SharedObjectStateSnapshot], func(), error)

	// QueueOperation signs op as the local peer and adds it to the operation
	// set. Returns the local operation id once the local state holds it.
	QueueOperation(ctx context.Context, op []byte) (string, error)
}

// PublicationRetention is implemented by providers that publish local state
// asynchronously. Its store retains graph-completion proofs for the mounted
// block store; World consumers fence all dependencies before accepting a write.
type PublicationRetention interface {
	AccessPublicationRetention(context.Context) (kvtx.Store, func(), error)
}

// OrderedQueue is implemented by a SharedObject whose operation queue write
// shares one ordered store with its block store: the queue write never becomes
// durable before an earlier block write. A writer may then queue an operation
// after writing its blocks without a block store Sync in between.
type OrderedQueue interface {
	// QueueOrdersBlockWrites reports whether queueing orders block writes.
	QueueOrdersBlockWrites() bool
}

// QueueOrdersBlockWrites reports whether so's queue write is ordered after its
// earlier block writes.
func QueueOrdersBlockWrites(so SharedObject) bool {
	ordered, ok := so.(OrderedQueue)
	return ok && ordered.QueueOrdersBlockWrites()
}

// orderedOperationKey marks a context whose queued operation may be accepted
// with an ordered commit.
type orderedOperationKey struct{}

// WithOrderedOperation marks operations queued with ctx as allowed to be
// accepted with an ordered commit: applied when QueueOperation returns, and
// durable at the provider's next durability point. Providers that do not
// support this ignore the mark.
func WithOrderedOperation(ctx context.Context) context.Context {
	return context.WithValue(ctx, orderedOperationKey{}, true)
}

// OrderedOperation reports whether ctx carries WithOrderedOperation.
func OrderedOperation(ctx context.Context) bool {
	ordered, _ := ctx.Value(orderedOperationKey{}).(bool)
	return ordered
}

// SharedObjectHealthAccessor exposes SharedObject health directly from a mounted object.
type SharedObjectHealthAccessor interface {
	// AccessSharedObjectHealth adds a reference to SharedObject health and returns the state container.
	// Returns a release function. Accepts a function that is called if the Watchable becomes invalid.
	AccessSharedObjectHealth(ctx context.Context, released func()) (ccontainer.Watchable[*SharedObjectHealth], func(), error)
}

// ReplayReporter is implemented by a SharedObject that shows the results of
// its device's replay in its health: rejected edits and a wrong checkpoint.
// The World engine reports them after each replay it installs.
type ReplayReporter interface {
	// SetRejectedEdits replaces the rejected edits in the health.
	SetRejectedEdits(edits []*SORejectedEdit)
	// SetCheckpointMismatch replaces the checkpoint mismatch in the health.
	SetCheckpointMismatch(mismatch *SOCheckpointMismatch)
}

// InviteHost is an optional interface on SharedObject implementations that
// support invite creation and management. Both local and spacewave providers
// implement this.
type InviteHost interface {
	InviteMutator

	// GetSOHost returns the SOHost for invite operations.
	GetSOHost() *SOHost
	// GetPrivKey returns the private key for signing invite messages.
	GetPrivKey() crypto.PrivKey
	// GetProviderID returns the provider identifier for the invite message.
	GetProviderID() string
}

// JoinRequestHost is an optional interface on SharedObject implementations
// that hold the join requests queued by invites requiring approval.
type JoinRequestHost interface {
	// GetJoinRequestsCtr returns the pending join requests, at most one per peer.
	GetJoinRequestsCtr() ccontainer.Watchable[*SOJoinRequestList]
	// RemoveJoinRequest removes the pending request of peerID.
	RemoveJoinRequest(ctx context.Context, peerID string) error
}

// InviteMutator mutates shared-object invite state.
type InviteMutator interface {
	// CreateSOInviteOp creates a new invite and returns the signed invite message.
	CreateSOInviteOp(
		ctx context.Context,
		ownerPrivKey crypto.PrivKey,
		providerID string,
		terms *SOInvite,
	) (*SOInviteMessage, error)
	// RevokeInvite revokes an invite by ID.
	RevokeInvite(ctx context.Context, signerPrivKey crypto.PrivKey, inviteID string) error
	// IncrementInviteUses increments the use counter for an invite by ID.
	IncrementInviteUses(ctx context.Context, signerPrivKey crypto.PrivKey, inviteID string) error
}

// AccessLocalStateStoreFunc implements AccessLocalStateStore.
type AccessLocalStateStoreFunc func(ctx context.Context, storeID string, released func()) (kvtx.Store, func(), error)

// NewLocalStateStoreRefcount retains the selected local store while references remain.
// The access callback supplies the store and its release function.
func NewLocalStateStoreRefcount(
	storeID string,
	access AccessLocalStateStoreFunc,
) *refcount.RefCount[kvtx.Store] {
	return refcount.NewRefCount(nil, false, nil, nil, func(ctx context.Context, released func()) (kvtx.Store, func(), error) {
		return access(ctx, storeID, released)
	})
}

// Validate checks bootstrap structure or a retained configuration, including a terminal empty audience.
// A nonempty configuration must keep an OWNER so later changes remain authorizable.
func (c *SharedObjectConfig) Validate() error {
	// Pinned operations of removed authors outlive the final departure too.
	if err := validateAuthorHeads("removed_authors", c.GetRemovedAuthors()); err != nil {
		return err
	}

	// The roster drops each peer once, in order.
	for i, peerID := range c.GetRosterDroppedPeerIds() {
		if _, err := parsePeerIDField(peerID); err != nil {
			return errors.Wrapf(err, "roster_dropped_peer_ids[%d]", i)
		}
		if i > 0 && strings.Compare(c.GetRosterDroppedPeerIds()[i-1], peerID) >= 0 {
			return errors.New("roster_dropped_peer_ids must be strictly sorted")
		}
	}

	// A signed history head can retain the final departure; an empty bootstrap cannot grant authority.
	participants := c.GetParticipants()
	if len(participants) == 0 {
		if len(c.GetConfigChainHash()) != 32 {
			return ErrEmptyParticipants
		}
		return nil
	}
	if len(participants) > MaxParticipants {
		return ErrMaxCountExceeded
	}

	// Each remaining peer has one unambiguous role in this configuration.
	seenPeerIDs := make(map[string]struct{}, len(participants))
	var hasOwner bool
	for i, participant := range participants {
		if err := participant.Validate(); err != nil {
			return errors.Wrapf(err, "participants[%d]", i)
		}
		ppID := participant.GetPeerId()
		if _, ok := seenPeerIDs[ppID]; ok {
			return errors.Errorf("participants[%d]: duplicate peer id: %v", i, ppID)
		}
		seenPeerIDs[ppID] = struct{}{}
		hasOwner = hasOwner || IsOwner(participant.GetRole())
	}

	// Remaining participants need an owner to authorize any later change.
	if !hasOwner {
		return ErrNoOwner
	}
	return nil
}

// AdmitsOperation reports whether replay under this config applies the
// operation at nonce in peerID's chain: the author participates, or a removal
// pinned an operation at or after nonce.
func (c *SharedObjectConfig) AdmitsOperation(peerID string, nonce uint64) bool {
	if slices.ContainsFunc(c.GetParticipants(), func(p *SOParticipantConfig) bool { return p.GetPeerId() == peerID }) {
		return true
	}
	i, ok := slices.BinarySearchFunc(c.GetRemovedAuthors(), peerID, func(a *SOOperationPosition, id string) int {
		return strings.Compare(a.GetPeerId(), id)
	})
	return ok && nonce <= c.GetRemovedAuthors()[i].GetNonce()
}

// Writers returns every participant that can write operations, in
// participant order.
func (c *SharedObjectConfig) Writers() []string {
	var writers []string
	for _, p := range c.GetParticipants() {
		if CanWriteOps(p.GetRole()) {
			writers = append(writers, p.GetPeerId())
		}
	}
	return writers
}

// TrimRoster returns the trimming roster: every writer the roster does not
// drop, in participant order.
func (c *SharedObjectConfig) TrimRoster() []string {
	return slices.DeleteFunc(c.Writers(), func(peerID string) bool {
		return slices.Contains(c.GetRosterDroppedPeerIds(), peerID)
	})
}

// Checkpointer returns the owner that checkpoints the stable point: the first
// owner on the trimming roster, in participant order, or empty when none is.
// One checkpointer keeps owners from signing different checkpoints at one
// height.
func (c *SharedObjectConfig) Checkpointer() string {
	for _, p := range c.GetParticipants() {
		if IsOwner(p.GetRole()) && !slices.Contains(c.GetRosterDroppedPeerIds(), p.GetPeerId()) {
			return p.GetPeerId()
		}
	}
	return ""
}

// NewSOOperationLocalID constructs a new randomized local ID for a op.
func NewSOOperationLocalID() string {
	return ulid.NewULID()
}

// ParseSOOperationLocalID parses and validates the local id is the correct format.
func ParseSOOperationLocalID(id string) (ulid.ULID, error) {
	return ulid.ParseULID(id)
}

// UnmarshalInner unmarshals and verifies the SOOperationInner.
func (op *SOOperation) UnmarshalInner() (*SOOperationInner, error) {
	inner := &SOOperationInner{}
	if err := inner.UnmarshalVT(op.GetInner()); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal inner data")
	}
	if err := inner.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid inner data")
	}
	return inner, nil
}

// Validate performs cursory checks on the SOOperation.
func (op *SOOperation) Validate() error {
	if len(op.GetInner()) == 0 {
		return ErrEmptyInnerData
	}
	if len(op.GetInner()) > MaxInnerDataSize {
		return ErrMaxSizeExceeded
	}

	if err := op.GetSignature().Validate(); err != nil {
		return err
	}

	// Unmarshal and validate the inner data
	_, err := op.UnmarshalInner()
	if err != nil {
		return err
	}

	return nil
}

// Validate performs cursory checks on the SOOperationInner.
func (i *SOOperationInner) Validate() error {
	// The author, local ID, and sequence identify the operation.
	if _, err := i.ParsePeerID(); err != nil {
		return err
	}
	if _, err := ParseSOOperationLocalID(i.GetLocalId()); err != nil {
		return err
	}
	if i.GetNonce() == 0 {
		return ErrInvalidNonce
	}

	// The payload is bounded. An acknowledgment carries none.
	if len(i.GetOpData()) > MaxInnerDataSize {
		return ErrMaxSizeExceeded
	}

	// The links place the operation in its chain.
	return i.validateLinks()
}

// IsAcknowledgment reports whether the operation is an acknowledgment: it
// applies nothing and records that its author has built on every operation it
// names.
func (i *SOOperationInner) IsAcknowledgment() bool {
	return len(i.GetOpData()) == 0
}

// parsePeerIDField parses a peer id string from a proto field. Returns
// peer.ErrEmptyPeerID when empty so callers do not need to repeat the check.
func parsePeerIDField(peerID string) (peer.ID, error) {
	if len(peerID) == 0 {
		return "", peer.ErrEmptyPeerID
	}
	return confparse.ParsePeerID(peerID)
}

// ParsePeerID parses the peer ID.
func (r *SOOperationRef) ParsePeerID() (peer.ID, error) {
	return parsePeerIDField(r.GetPeerId())
}

// Validate validates the SOOperationRef.
func (r *SOOperationRef) Validate() error {
	if _, err := r.ParsePeerID(); err != nil {
		return err
	}
	if r.GetNonce() == 0 {
		return ErrInvalidNonce
	}
	return nil
}

// ParsePeerID parses the peer ID.
func (i *SOOperationInner) ParsePeerID() (peer.ID, error) {
	return parsePeerIDField(i.GetPeerId())
}

// ParsePeerID parses the peer ID.
func (g *SOGrant) ParsePeerID() (peer.ID, error) {
	return parsePeerIDField(g.GetPeerId())
}

// Validate performs cursory checks on the SOGrant.
func (g *SOGrant) Validate() error {
	if _, err := g.ParsePeerID(); err != nil {
		return err
	}
	if len(g.GetInnerData()) == 0 {
		return ErrEmptyInnerData
	}
	if len(g.GetInnerData()) > MaxInnerDataSize {
		return ErrMaxSizeExceeded
	}
	return g.GetSignature().Validate()
}

// Validate performs cursory checks on the SOGrantInner.
func (g *SOGrantInner) Validate() error {
	if g.GetTransformConf().GetEmpty() {
		return ErrEmptyTransformConfig
	}
	if g.GetTransformConf().SizeVT() > MaxBlockRefSize {
		return ErrMaxSizeExceeded
	}
	return g.GetTransformConf().Validate()
}

// BuildSOOperationResult constructs a new SOOperationResult.
// If success is false, errorDetails must be non-nil.
func BuildSOOperationResult(
	peerID string,
	nonce uint64,
	success bool,
	errorDetails *SOOperationRejectionErrorDetails,
) *SOOperationResult {
	result := &SOOperationResult{
		OpRef: &SOOperationRef{
			PeerId: peerID,
			Nonce:  nonce,
		},
	}
	if success {
		result.Body = &SOOperationResult_Success{
			Success: true,
		}
	} else {
		result.Body = &SOOperationResult_ErrorDetails{
			ErrorDetails: errorDetails,
		}
	}
	return result
}

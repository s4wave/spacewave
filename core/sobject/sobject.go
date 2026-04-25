package sobject

import (
	"context"
	"slices"
	"strings"

	"github.com/aperturerobotics/controllerbus/bus"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/ccontainer"
	"github.com/aperturerobotics/util/refcount"
	"github.com/aperturerobotics/util/scrub"
	"github.com/aperturerobotics/util/ulid"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/bstore"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/net/crypto"
	"github.com/s4wave/spacewave/net/hash"
	"github.com/s4wave/spacewave/net/peer"
	"github.com/s4wave/spacewave/net/util/confparse"
)

// ProcessOpsFunc is a function which processes operations against a state.
// cb is called with the state snapshot and the decoded inner state.
// If rawNextStateData is nil, no changes will be applied to the state (no-op).
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

	// GetSharedObjectState returns an snapshot of the shared object state.
	GetSharedObjectState(ctx context.Context) (SharedObjectStateSnapshot, error)

	// AccessSharedObjectState adds a reference to the state and returns the state container.
	// Returns a release function. Accepts a function that is called if the Watchable becomes invalid.
	AccessSharedObjectState(ctx context.Context, released func()) (ccontainer.Watchable[SharedObjectStateSnapshot], func(), error)

	// QueueOperation applies an operation to the shared object op queue.
	// Returns after the operation is applied to the local queue.
	// Returns the local operation id.
	QueueOperation(ctx context.Context, op []byte) (string, error)

	// WaitOperation waits for the operation to be confirmed or rejected by the provider.
	// Returns the current state nonce (greater than or equal to the nonce when the op was applied).
	// After ClearOperation has been called, this will return success even for failed ops!
	// If the operation was rejected, returns 0, true, error.
	// Any other error returns 0, false, error
	WaitOperation(ctx context.Context, localID string) (uint64, bool, error)

	// ClearOperationResult clears the operation state.
	// No-op if the operation was successfully applied or ClearOperationResult was already called.
	// Be sure to call this after checking WaitOperation (not before).
	ClearOperationResult(ctx context.Context, localID string) error

	// ProcessOperations processes operations as a validator.
	// The ops should be processed in the order they are provided.
	// The results must be a subset of ops (but does not need to have all ops).
	//
	// If watch is set, waits for ops to be queued, then calls cb. Does not return.
	// If watch is unset, if there are no available ops, returns immediately.
	//
	// cb is called with the state snapshot and the decoded inner state.
	// If rawNextStateData is nil, no changes will be applied to the state (no-op).
	ProcessOperations(ctx context.Context, watch bool, cb ProcessOpsFunc) error
}

// SharedObjectHealthAccessor exposes SharedObject health directly from a mounted object.
type SharedObjectHealthAccessor interface {
	// AccessSharedObjectHealth adds a reference to SharedObject health and returns the state container.
	// Returns a release function. Accepts a function that is called if the Watchable becomes invalid.
	AccessSharedObjectHealth(ctx context.Context, released func()) (ccontainer.Watchable[*SharedObjectHealth], func(), error)
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

// InviteMutator mutates shared-object invite state.
type InviteMutator interface {
	// CreateSOInviteOp creates a new invite and returns the signed invite message.
	CreateSOInviteOp(
		ctx context.Context,
		ownerPrivKey crypto.PrivKey,
		role SOParticipantRole,
		providerID string,
		targetPeerID string,
		maxUses uint32,
		expiresAt *timestamppb.Timestamp,
	) (*SOInviteMessage, error)
	// RevokeInvite revokes an invite by ID.
	RevokeInvite(ctx context.Context, signerPrivKey crypto.PrivKey, inviteID string) error
	// IncrementInviteUses increments the use counter for an invite by ID.
	IncrementInviteUses(ctx context.Context, signerPrivKey crypto.PrivKey, inviteID string) error
}

// AccessLocalStateStoreFunc implements AccessLocalStateStore.
type AccessLocalStateStoreFunc func(ctx context.Context, storeID string, released func()) (kvtx.Store, func(), error)

// NewLocalStateStoreRefcount constructs a refcount with the LocalStateStore access func.
//
// ctx, target and targetErr can be empty
//
// keepUnref sets if the value should be kept if there are zero references.
// resolver is the resolver function
// returns the value and a release function
// call the released callback if the value is no longer valid.
func NewLocalStateStoreRefcount(
	storeID string,
	access AccessLocalStateStoreFunc,
) *refcount.RefCount[kvtx.Store] {
	return refcount.NewRefCount(nil, false, nil, nil, func(ctx context.Context, released func()) (kvtx.Store, func(), error) {
		return access(ctx, storeID, released)
	})
}

// Validate checks if the SharedObjectConfig is valid.
func (c *SharedObjectConfig) Validate() error {
	if len(c.GetParticipants()) == 0 {
		return ErrEmptyParticipants
	}
	if len(c.GetParticipants()) > MaxParticipants {
		return ErrMaxCountExceeded
	}

	seenPeerIDs := make(map[string]struct{})
	for i, participant := range c.GetParticipants() {
		if err := participant.Validate(); err != nil {
			return errors.Wrapf(err, "participants[%d]", i)
		}
		ppID := participant.GetPeerId()
		if _, ok := seenPeerIDs[ppID]; ok {
			return errors.Errorf("participants[%d]: duplicate peer id: %v", i, ppID)
		}
		seenPeerIDs[ppID] = struct{}{}
	}

	return nil
}

// BuildSOOperation constructs a new SOOperation for a writer or validator.
// The privKey must belong to a writer or validator participant.
// opDataEnc should be opData encoded with the root state transform.
func BuildSOOperation(
	sharedObjectID string,
	privKey crypto.PrivKey,
	opDataEnc []byte,
	opNonce uint64,
	opLocalID string,
) (*SOOperation, error) {
	// Get the peer ID from the private key
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get peer ID from private key")
	}
	peerIDStr := peerID.String()

	// Create the operation
	inner := &SOOperationInner{
		PeerId:  peerIDStr,
		LocalId: opLocalID,
		Nonce:   opNonce,
		OpData:  opDataEnc,
	}
	if err := inner.Validate(); err != nil {
		return nil, err
	}

	innerData, err := inner.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal operation inner data")
	}

	// Sign the operation
	encContext := BuildSOOperationSignatureContext(sharedObjectID, peerIDStr, opNonce, opLocalID)
	sig, err := peer.NewSignature(encContext, privKey, hash.HashType_HashType_BLAKE3, innerData, true)
	if err != nil {
		return nil, errors.Wrap(err, "failed to sign operation")
	}

	// Create the operation
	op := &SOOperation{
		Inner:     innerData,
		Signature: sig,
	}

	// Validate the operation
	if err := op.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid operation")
	}

	return op, nil
}

// BuildSOOperationRejection constructs a new SOOperationRejection.
// The privKey is used to sign the rejection.
// The sharedObjectID is used to build the signature context.
// The submitterPeerID is the peer ID of the operation submitter.
// The opNonce is the nonce of the rejected operation.
// The opLocalID is the local ID of the rejected operation, must match the original operation.
// The errorDetails are optional and may be nil.
func BuildSOOperationRejection(
	privKey crypto.PrivKey,
	sharedObjectID string,
	submitterPeerID peer.ID,
	opNonce uint64,
	opLocalID string,
	errorDetails *SOOperationRejectionErrorDetails,
) (*SOOperationRejection, error) {
	// Construct the inner rejection
	inner := &SOOperationRejectionInner{
		PeerId:  submitterPeerID.String(),
		OpNonce: opNonce,
		LocalId: opLocalID,
	}

	// Get the validator's peer ID
	validatorPeerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, err
	}

	// Add error details if provided
	errorDetailsData, err := errorDetails.MarshalVT()
	if err != nil {
		return nil, err
	}
	if len(errorDetailsData) != 0 {
		defer scrub.Scrub(errorDetailsData)
		submitterPubKey, err := submitterPeerID.ExtractPublicKey()
		if err != nil {
			return nil, err
		}

		encErrorDetails, err := peer.EncryptToPubKey(
			submitterPubKey,
			BuildSOOperationRejectionErrorDetailsContext(
				sharedObjectID,
				validatorPeerID.String(),
				submitterPeerID.String(),
				opNonce,
				opLocalID,
			),
			errorDetailsData,
		)
		if err != nil {
			return nil, err
		}
		inner.ErrorDetails = encErrorDetails
	}

	// Validate the rejection.
	if err := inner.Validate(); err != nil {
		return nil, err
	}

	// Marshal the inner rejection
	innerData, err := inner.MarshalVT()
	if err != nil {
		return nil, err
	}

	// Create the signature
	encContext := BuildSOOperationRejectionSignatureContext(sharedObjectID, validatorPeerID.String(), submitterPeerID.String(), opNonce, opLocalID)
	sig, err := peer.NewSignature(encContext, privKey, hash.RecommendedHashType, innerData, true)
	if err != nil {
		return nil, err
	}

	// Construct the SOOperationRejection
	rejection := &SOOperationRejection{
		Inner:     innerData,
		Signature: sig,
	}

	return rejection, nil
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
	if _, err := i.ParsePeerID(); err != nil {
		return err
	}

	if _, err := ParseSOOperationLocalID(i.GetLocalId()); err != nil {
		return err
	}

	if i.GetNonce() == 0 {
		return ErrInvalidNonce
	}

	if len(i.GetOpData()) == 0 {
		return ErrEmptyInnerData
	}
	if len(i.GetOpData()) > MaxInnerDataSize {
		return ErrMaxSizeExceeded
	}

	return nil
}

// ParsePeerID parses the peer ID.
func (r *SOOperationRef) ParsePeerID() (peer.ID, error) {
	if len(r.GetPeerId()) == 0 {
		return "", peer.ErrEmptyPeerID
	}
	return confparse.ParsePeerID(r.GetPeerId())
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
func (n *SOAccountNonce) ParsePeerID() (peer.ID, error) {
	if len(n.GetPeerId()) == 0 {
		return "", peer.ErrEmptyPeerID
	}
	return confparse.ParsePeerID(n.GetPeerId())
}

// ParsePeerID parses the peer ID.
func (i *SOOperationInner) ParsePeerID() (peer.ID, error) {
	if len(i.GetPeerId()) == 0 {
		return "", peer.ErrEmptyPeerID
	}
	return confparse.ParsePeerID(i.GetPeerId())
}

// Validate performs cursory checks on the SORoot.
// updateAccountNonce updates the account nonce if the new nonce is higher.
func (r *SORoot) updateAccountNonce(peerID string, nonce uint64) {
	for i, accNonce := range r.AccountNonces {
		if accNonce.GetPeerId() == peerID {
			if nonce > accNonce.GetNonce() {
				r.AccountNonces[i].Nonce = nonce
			}
			return
		}
	}
	r.AccountNonces = append(r.AccountNonces, &SOAccountNonce{
		PeerId: peerID,
		Nonce:  nonce,
	})
	slices.SortFunc(r.AccountNonces, func(a, b *SOAccountNonce) int {
		return strings.Compare(a.GetPeerId(), b.GetPeerId())
	})
}

func (r *SORoot) Validate() error {
	if r.GetInnerSeqno() == 0 {
		return ErrInvalidSeqno
	}
	if len(r.GetInner()) == 0 {
		return ErrEmptyInnerData
	}
	if len(r.GetInner()) > MaxInnerDataSize {
		return ErrMaxSizeExceeded
	}
	if len(r.GetValidatorSignatures()) == 0 {
		return ErrEmptyValidatorSignatures
	}
	if len(r.GetValidatorSignatures()) > MaxValidatorSignatures {
		return ErrMaxCountExceeded
	}

	// Validate account nonces are sorted and unique by peer ID
	var prevPeerID string
	for i, nonce := range r.GetAccountNonces() {
		// Parse and validate the peer ID
		if _, err := nonce.ParsePeerID(); err != nil {
			return errors.Wrapf(err, "account_nonces[%d]", i)
		}
		nPeerID := nonce.GetPeerId()
		if prevPeerID != "" {
			cmp := strings.Compare(prevPeerID, nPeerID)
			if cmp >= 0 {
				return errors.New("account nonces not sorted by peer_id or not unique")
			}
		}
		prevPeerID = nPeerID
	}

	// Validate validator signatures
	for i, sig := range r.GetValidatorSignatures() {
		if err := sig.Validate(); err != nil {
			return errors.Wrapf(err, "validator_signatures[%d]", i)
		}
	}

	return nil
}

// ApplyUpdatedState applies an updated state to the SORoot.
// Asserts that the previous (current) seqno is prevSeqno.
// Signs the updated state.
// Updates the SORoot in-place.
// innerData should be the transformed marshaled inner data object.
func (r *SORoot) ApplyUpdatedState(privKey crypto.PrivKey, sharedObjectID string, prevSeqno uint64, hashType hash.HashType, innerData []byte) error {
	if r.GetInnerSeqno() != prevSeqno {
		return ErrInvalidSeqno
	}

	nextSeqno := prevSeqno + 1
	r.InnerSeqno = nextSeqno
	r.ValidatorSignatures = nil
	r.Inner = innerData

	return r.SignInnerData(privKey, sharedObjectID, nextSeqno, hashType)
}

// BuildSignatureData builds the signature data that includes both inner and account nonces
func (r *SORoot) BuildSignatureData() ([]byte, error) {
	var signData []byte
	signData = append(signData, r.GetInner()...)
	for _, nonce := range r.GetAccountNonces() {
		nonceData, err := nonce.MarshalVT()
		if err != nil {
			return nil, err
		}
		signData = append(signData, nonceData...)
	}
	return signData, nil
}

// SignInnerData signs the inner data with a validator privKey.
// If the privKey was already used to sign the data, does nothing.
// seqno asserts that r.InnerSeqno matches the given value.
func (r *SORoot) SignInnerData(privKey crypto.PrivKey, sharedObjectID string, seqno uint64, hashType hash.HashType) error {
	pubKey := privKey.GetPublic()

	// check if already signed
	for _, sig := range r.GetValidatorSignatures() {
		sigPubKey, err := sig.ParsePubKey()
		if err != nil {
			return err
		}
		if pubKey.Equals(sigPubKey) {
			return nil
		}
	}

	// assert seqno
	innerData, innerSeqno := r.GetInner(), r.GetInnerSeqno()
	if len(innerData) == 0 {
		return ErrEmptyInnerData
	}
	if innerSeqno != seqno {
		return ErrInvalidSeqno
	}

	// build inner data
	signData, err := r.BuildSignatureData()
	if err != nil {
		return err
	}
	defer scrub.Scrub(signData)

	// sign
	encContext := BuildValidatorRootSignatureContext(sharedObjectID, seqno)
	sig, err := peer.NewSignature(encContext, privKey, hashType, signData, true)
	if err != nil {
		return err
	}

	r.ValidatorSignatures = append(r.ValidatorSignatures, sig)
	return nil
}

// Validate performs cursory checks on the SORootInner.
func (r *SORootInner) Validate() error {
	if r.GetSeqno() == 0 {
		return ErrInvalidSeqno
	}
	if len(r.GetStateData()) > MaxStateDataSize {
		return ErrMaxSizeExceeded
	}
	return nil
}

// ParsePeerID parses the peer ID.
func (g *SOGrant) ParsePeerID() (peer.ID, error) {
	if len(g.GetPeerId()) == 0 {
		return "", peer.ErrEmptyPeerID
	}
	return confparse.ParsePeerID(g.GetPeerId())
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

// Validate performs cursory checks on the SOOperationRejection.
func (r *SOOperationRejection) Validate() error {
	if len(r.GetInner()) == 0 {
		return ErrEmptyInnerData
	}
	if len(r.GetInner()) > MaxInnerDataSize {
		return ErrMaxSizeExceeded
	}

	if err := r.GetSignature().Validate(); err != nil {
		return peer.ErrSignatureInvalid
	}

	// Unmarshal and validate the inner data
	_, err := r.UnmarshalInner()
	if err != nil {
		return err
	}

	return nil
}

// UnmarshalInner unmarshals and verifies the SOOperationRejectionInner.
func (r *SOOperationRejection) UnmarshalInner() (*SOOperationRejectionInner, error) {
	inner := &SOOperationRejectionInner{}
	if err := inner.UnmarshalVT(r.GetInner()); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal inner data")
	}
	if err := inner.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid inner data")
	}
	return inner, nil
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

// ParsePeerID parses the peer ID.
func (i *SOOperationRejectionInner) ParsePeerID() (peer.ID, error) {
	if len(i.GetPeerId()) == 0 {
		return "", peer.ErrEmptyPeerID
	}
	return confparse.ParsePeerID(i.GetPeerId())
}

// Validate performs cursory checks on the SOOperationRejectionInner.
func (i *SOOperationRejectionInner) Validate() error {
	// Parse and validate the peer ID
	if _, err := i.ParsePeerID(); err != nil {
		return err
	}

	if i.GetOpNonce() == 0 {
		return ErrInvalidNonce
	}

	if len(i.GetErrorDetails()) > MaxErrorDetailsSize {
		return ErrMaxSizeExceeded
	}

	if _, err := ParseSOOperationLocalID(i.GetLocalId()); err != nil {
		return err
	}

	return nil
}

// ParsePeerID parses the peer ID for the SOPeerOpRejections.
func (r *SOPeerOpRejections) ParsePeerID() (peer.ID, error) {
	if len(r.GetPeerId()) == 0 {
		return "", peer.ErrEmptyPeerID
	}
	return confparse.ParsePeerID(r.GetPeerId())
}

// BuildSOClearOperationResult constructs a new SOClearOperationResult.
// The privKey must belong to the peer that submitted the original operation.
func BuildSOClearOperationResult(
	sharedObjectID string,
	privKey crypto.PrivKey,
	localID string,
) (*SOClearOperationResult, error) {
	// Get the peer ID from the private key
	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get peer ID from private key")
	}
	peerIDStr := peerID.String()

	// Create the inner message
	inner := &SOClearOperationResultInner{
		PeerId:  peerIDStr,
		LocalId: localID,
	}

	// Marshal the inner data
	innerData, err := inner.MarshalVT()
	if err != nil {
		return nil, errors.Wrap(err, "failed to marshal inner data")
	}

	// Sign the inner data
	sig, err := peer.NewSignature(
		BuildSOClearOperationResultSignatureContext(
			sharedObjectID,
			peerIDStr,
			localID,
		),
		privKey,
		hash.RecommendedHashType,
		innerData,
		true,
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to sign inner data")
	}

	// Create and return the clear operation
	return &SOClearOperationResult{
		Inner:     innerData,
		Signature: sig,
	}, nil
}

// UnmarshalInner unmarshals and verifies the SOClearOperationResultInner.
func (c *SOClearOperationResult) UnmarshalInner() (*SOClearOperationResultInner, error) {
	inner := &SOClearOperationResultInner{}
	if err := inner.UnmarshalVT(c.GetInner()); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal inner data")
	}
	if err := inner.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid inner data")
	}
	return inner, nil
}

// Validate performs cursory checks on the SOClearOperationResult.
func (c *SOClearOperationResult) Validate() error {
	if len(c.GetInner()) == 0 {
		return ErrEmptyInnerData
	}
	if len(c.GetInner()) > MaxInnerDataSize {
		return ErrMaxSizeExceeded
	}
	if err := c.GetSignature().Validate(); err != nil {
		return err
	}
	return nil
}

// Validate performs cursory checks on the SOClearOperationResultInner.
func (i *SOClearOperationResultInner) Validate() error {
	if _, err := i.ParsePeerID(); err != nil {
		return err
	}
	if _, err := ParseSOOperationLocalID(i.GetLocalId()); err != nil {
		return err
	}
	return nil
}

// ParsePeerID parses the peer ID.
func (i *SOClearOperationResultInner) ParsePeerID() (peer.ID, error) {
	if len(i.GetPeerId()) == 0 {
		return "", peer.ErrEmptyPeerID
	}
	return confparse.ParsePeerID(i.GetPeerId())
}

// Validate performs cursory checks on the QueuedSOOperation.
func (q *QueuedSOOperation) Validate() error {
	if _, err := ParseSOOperationLocalID(q.GetLocalId()); err != nil {
		return err
	}

	if len(q.GetOpData()) == 0 {
		return ErrEmptyInnerData
	}
	if len(q.GetOpData()) > MaxInnerDataSize {
		return ErrMaxSizeExceeded
	}

	return nil
}

// Validate performs cursory checks on the SOPeerOpRejections.
func (r *SOPeerOpRejections) Validate() error {
	_, err := r.ParsePeerID()
	if err != nil {
		return err
	}
	for i, rejection := range r.GetRejections() {
		if err := rejection.Validate(); err != nil {
			return errors.Wrapf(err, "rejection[%d]", i)
		}
	}
	return nil
}

// DecodeErrorDetails decodes the error details message.
//
// If the field is empty, returns nil, nil
func (i *SOOperationRejectionInner) DecodeErrorDetails(privKey crypto.PrivKey, sharedObjectID string, validatorPeerID peer.ID) (*SOOperationRejectionErrorDetails, error) {
	if len(i.GetErrorDetails()) == 0 {
		return nil, nil
	}

	privPeerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return nil, err
	}
	privPeerIDStr := privPeerID.String()

	if iPeerID := i.GetPeerId(); privPeerIDStr != iPeerID {
		return nil, errors.Errorf("unexpected peer id for error details: %s != expected %s", privPeerIDStr, iPeerID)
	}

	decData, err := peer.DecryptWithPrivKey(
		privKey,
		BuildSOOperationRejectionErrorDetailsContext(
			sharedObjectID,
			validatorPeerID.String(),
			privPeerIDStr,
			i.GetOpNonce(),
			i.GetLocalId(),
		),
		i.GetErrorDetails(),
	)
	if err != nil {
		return nil, err
	}
	defer scrub.Scrub(decData)

	errorDetails := &SOOperationRejectionErrorDetails{}
	if err := errorDetails.UnmarshalVT(decData); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal error details")
	}
	if len(errorDetails.GetErrorMsg()) == 0 {
		return nil, errors.New("error details message was non-zero length but had an empty error msg")
	}

	return errorDetails, nil
}

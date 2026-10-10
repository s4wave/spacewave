package sobject

import (
	"errors"

	"github.com/aperturerobotics/util/ulid"
)

var (
	// ErrEmptySharedObjectID is returned if the shared object id was empty.
	ErrEmptySharedObjectID = errors.New("shared object id cannot be empty")

	// ErrInvalidSharedObjectID is returned if the shared object id was invalid.
	ErrInvalidSharedObjectID = errors.New("invalid shared object id")

	// ErrSharedObjectExists is returned if the shared object already exists.
	ErrSharedObjectExists = errors.New("shared object with that id already exists")

	// ErrSharedObjectNotFound is returned if the sobject was not found.
	ErrSharedObjectNotFound = errors.New("shared object not found")

	// ErrInvalidSOParticipantRole is returned if the shared object participant role is invalid.
	ErrInvalidSOParticipantRole = errors.New("invalid shared object participant role")

	// ErrEmptyParticipants is returned if no participants are specified in the SharedObjectConfig.
	ErrEmptyParticipants = errors.New("empty shared object participants list")

	// ErrNoOwner is returned if a nonempty SharedObjectConfig has no OWNER.
	ErrNoOwner = errors.New("shared object participants have no owner")

	// ErrEmptyBodyType is returned if the sobject body type was empty.
	ErrEmptyBodyType = errors.New("empty shared object body type")

	// ErrUnsupportedBodyType identifies a body the mounted resource cannot serve.
	ErrUnsupportedBodyType = errors.New("unsupported shared object type")

	// ErrEmptyInnerData is returned if the inner data was empty.
	ErrEmptyInnerData = errors.New("empty inner data")

	// ErrConfigChainHeadMismatch is returned if a config change does not
	// extend the held config chain head. The writer read a stale head and may
	// rebuild the change against the current one.
	ErrConfigChainHeadMismatch = errors.New("config change previous_hash does not match current config_chain_hash")

	// ErrInvalidNonce is returned if the op nonce was unexpected.
	ErrInvalidNonce = errors.New("invalid shared object op nonce")

	// ErrCannotDecode is returned if our local peer cannot decode the inner data (no valid grant).
	ErrCannotDecode = errors.New("access denied: no valid grant for our peer")

	// ErrKeyEpochUnavailable is returned if the local peer holds no grant for
	// the key epoch an operation or checkpoint names. Replay stops instead of
	// deciding an outcome, so members never diverge on what they could read.
	ErrKeyEpochUnavailable = errors.New("no grant for the key epoch")

	// ErrNotParticipant is returned if the peer is not a participant in the shared object.
	ErrNotParticipant = errors.New("access denied: peer is not a participant")

	// ErrEmptyTransformConfig is returned if the transform config was empty.
	ErrEmptyTransformConfig = errors.New("transform config is required")

	// ErrMaxSizeExceeded is returned if a size limit is exceeded.
	ErrMaxSizeExceeded = errors.New("maximum size exceeded")

	// ErrStateTooLarge is returned if a snapshot of the state is larger than
	// one sync frame carries. The checkpointer shrinks the state by trimming the
	// operation set; a peer that cannot fetch it recovers by other means.
	ErrStateTooLarge = errors.New("shared object state exceeds the sync frame")

	// ErrMaxCountExceeded is returned if a count limit is exceeded.
	ErrMaxCountExceeded = errors.New("maximum count exceeded")

	// ErrInvalidLocalOpID is returned if the local op id is invalid.
	ErrInvalidLocalOpID = ulid.ErrInvalidULID

	// ErrRejectedOp is returned if the op was rejected.
	ErrRejectedOp = errors.New("rejected op")

	// ErrInvalidMeta is returned if the metadata is invalid.
	ErrInvalidMeta = errors.New("sobject: meta: invalid shared object metadata")

	// ErrSharedObjectRecoveryCredentialRequired is returned if recovery needs entity credentials.
	ErrSharedObjectRecoveryCredentialRequired = errors.New("shared object recovery requires entity credentials")

	// ErrResourceBlocked is matched by provider errors that block access to a
	// shared object until the user retries after the block is lifted.
	ErrResourceBlocked = errors.New("shared object resource is blocked")

	// ErrSharedObjectRecoveryEntityMismatch is returned if recovery material does not match the current entity.
	ErrSharedObjectRecoveryEntityMismatch = errors.New("shared object recovery entity mismatch")
)

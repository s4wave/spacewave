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

	// ErrEmptyBodyType is returned if the sobject body type was empty.
	ErrEmptyBodyType = errors.New("empty shared object body type")

	// ErrEmptyInnerData is returned if the inner data was empty.
	ErrEmptyInnerData = errors.New("empty inner data")

	// ErrInvalidSeqno is returned if the root seqno was unexpected.
	ErrInvalidSeqno = errors.New("invalid shared object root seqno")

	// ErrInvalidNonce is returned if the op nonce was unexpected.
	ErrInvalidNonce = errors.New("invalid shared object op nonce")

	// ErrEmptyValidatorSignatures is returned if there are no validator signatures.
	ErrEmptyValidatorSignatures = errors.New("at least one validator signature required")

	// ErrCannotDecode is returned if our local peer cannot decode the inner data (no valid grant).
	ErrCannotDecode = errors.New("access denied: no valid grant for our peer")

	// ErrNotParticipant is returned if the peer is not a participant in the shared object.
	ErrNotParticipant = errors.New("access denied: peer is not a participant")

	// ErrEmptyTransformConfig is returned if the transform config was empty.
	ErrEmptyTransformConfig = errors.New("transform config is required")

	// ErrMaxSizeExceeded is returned if a size limit is exceeded.
	ErrMaxSizeExceeded = errors.New("maximum size exceeded")

	// ErrMaxCountExceeded is returned if a count limit is exceeded.
	ErrMaxCountExceeded = errors.New("maximum count exceeded")

	// ErrInvalidLocalOpID is returned if the local op id is invalid.
	ErrInvalidLocalOpID = ulid.ErrInvalidULID

	// ErrRejectedOp is returned if the op was rejected.
	ErrRejectedOp = errors.New("rejected op")

	// ErrInvalidValidator is returned if the required validator peer is not in the set of signatures.
	ErrInvalidValidator = errors.New("required validator peer not in set of signatures")

	// ErrInvalidMeta is returned if the metadata is invalid.
	ErrInvalidMeta = errors.New("sobject: meta: invalid shared object metadata")

	// ErrSharedObjectRecoveryCredentialRequired is returned if recovery needs entity credentials.
	ErrSharedObjectRecoveryCredentialRequired = errors.New("shared object recovery requires entity credentials")

	// ErrSharedObjectRecoveryEntityMismatch is returned if recovery material does not match the current entity.
	ErrSharedObjectRecoveryEntityMismatch = errors.New("shared object recovery entity mismatch")
)

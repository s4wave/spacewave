package provider_spacewave

import "github.com/pkg/errors"

// ErrUnknownEntity is returned when the account exists but the
// provided credentials do not match any registered keypair.
var ErrUnknownEntity = errors.New("unknown entity: wrong credentials")

// ErrUnknownKeypair is returned when no account exists for the
// provided entity ID.
var ErrUnknownKeypair = errors.New("unknown keypair: account not found")

// ErrSharedObjectMetadataDeleted is returned when cached shared-object metadata is deleted.
var ErrSharedObjectMetadataDeleted = errors.New("shared object metadata deleted")

// ErrSpaceLinkNonceConsumed is returned when a SpaceLink approval tries to
// reuse a nonce already consumed by this provider account.
var ErrSpaceLinkNonceConsumed = errors.New("spacelink nonce already consumed")

// ErrSigningUnavailable indicates that a session client cannot sign requests
// until its signing capability is installed.
var ErrSigningUnavailable = errors.New("no private key or signing function configured for signing")

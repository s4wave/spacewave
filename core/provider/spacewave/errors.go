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

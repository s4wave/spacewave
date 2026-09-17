package kvtx

import "errors"

var (
	// errInvalidPublication rejects a malformed atomic publication request.
	errInvalidPublication = errors.New("invalid atomic publication")
	// errPublicationReadOnly rejects mutation through a validation overlay.
	errPublicationReadOnly = errors.New("publication validation store is read-only")
)

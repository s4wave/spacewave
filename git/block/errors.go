package git_block

import "errors"

var (
	// ErrReferenceNameEmpty is returned if the reference name was empty.
	ErrReferenceNameEmpty = errors.New("reference name is empty")
	// ErrReferenceNameInvalid is returned if the reference name was invalid.
	ErrReferenceNameInvalid = errors.New("reference name is invalid")
	// ErrReferenceTypeInvalid is returned if the reference name was invalid.
	ErrReferenceTypeInvalid = errors.New("reference type is invalid")
	// ErrReferenceHashEmpty is returned if the reference name was empty.
	ErrReferenceHashEmpty = errors.New("reference hash is empty")
	// ErrHashTypeInvalid is returned if the hash type is not sha1.
	ErrHashTypeInvalid = errors.New("hash type must be sha1")
	// ErrObjectTypeInvalid is returned if the hash type is not sha1.
	ErrObjectTypeInvalid = errors.New("object type must be set")
	// ErrSizeInvalid is returned if the hash type is not sha1.
	ErrSizeInvalid = errors.New("object size invalid")
	// ErrEmptyHash is returned if the hash was empty.
	ErrEmptyHash = errors.New("hash cannot be empty")
	// ErrHashMismatch is returned if the hash did not match.
	ErrHashMismatch = errors.New("hash mismatch")
)

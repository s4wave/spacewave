package git_lfs

import "github.com/pkg/errors"

var (
	// ErrInvalidOid is returned when an oid is not a SHA-256 hex digest.
	ErrInvalidOid = errors.New("git-lfs: oid must be 64 lowercase hex characters")
	// ErrOidMismatch is returned when downloaded bytes hash to another oid.
	ErrOidMismatch = errors.New("git-lfs: downloaded object does not match its oid")
	// ErrSizeMismatch is returned when a download has the wrong length.
	ErrSizeMismatch = errors.New("git-lfs: downloaded object has the wrong size")
)

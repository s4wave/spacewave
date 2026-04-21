package volume_rpc

import "errors"

var (
	// ErrUnknownVolumeID is returned if the volume id is not found.
	ErrUnknownVolumeID = errors.New("unknown volume id")
	// ErrPrivKeyUnavailable is returned if returning private keys is disabled.
	ErrPrivKeyUnavailable = errors.New("volume private key is unavailable")
)

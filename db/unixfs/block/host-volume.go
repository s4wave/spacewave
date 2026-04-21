package unixfs_block

import (
	"context"
	"errors"

	"github.com/s4wave/spacewave/db/block"
)

// ErrCannotModifyHostVolume is returned if an operation tries to modify a host volume.
var ErrCannotModifyHostVolume = errors.New("cannot modify host volume this way")

// NewFSHostVolume constructs a FSHostVolume with a ID.
func NewFSHostVolume(volumeID string) *FSHostVolume {
	return &FSHostVolume{
		VolumeId: volumeID,
	}
}

// NewFSHostVolumeBlock constructs a FSHostVolume as a Block.
func NewFSHostVolumeBlock() block.Block {
	return &FSHostVolume{}
}

// UnmarshalFSHostVolume unmarshals a filesystem node from a cursor.
// If empty, returns nil, nil
func UnmarshalFSHostVolume(ctx context.Context, bcs *block.Cursor) (*FSHostVolume, error) {
	return block.UnmarshalBlock[*FSHostVolume](ctx, bcs, NewFSHostVolumeBlock)
}

// Validate checks the HostVolume.
func (n *FSHostVolume) Validate() error {
	if n.GetVolumeId() == "" {
		return errors.New("volume id cannot be empty")
	}
	return nil
}

// MarshalBlock marshals the block to binary.
// This is the initial step of marshaling, before transformations.
func (n *FSHostVolume) MarshalBlock() ([]byte, error) {
	return n.MarshalVT()
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (n *FSHostVolume) UnmarshalBlock(data []byte) error {
	return n.UnmarshalVT(data)
}

// _ is a type assertion
var _ block.Block = ((*FSHostVolume)(nil))

package unixfs_block

import (
	"errors"

	"github.com/aperturerobotics/hydra/block"
	proto "google.golang.org/protobuf/proto"
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
func UnmarshalFSHostVolume(bcs *block.Cursor) (*FSHostVolume, error) {
	if bcs == nil {
		return nil, nil
	}
	blk, err := bcs.Unmarshal(NewFSHostVolumeBlock)
	if err != nil {
		return nil, err
	}
	if blk == nil {
		return nil, nil
	}
	bv, ok := blk.(*FSHostVolume)
	if !ok {
		return nil, block.ErrUnexpectedType
	}
	return bv, nil
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
	return proto.Marshal(n)
}

// UnmarshalBlock unmarshals the block to the object.
// This is the final step of decoding, after transformations.
func (n *FSHostVolume) UnmarshalBlock(data []byte) error {
	return proto.Unmarshal(data, n)
}

// _ is a type assertion
var _ block.Block = ((*FSHostVolume)(nil))

package transform_chksum

import (
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/pkg/errors"
)

// Chksum is the checksum step.
type Chksum struct {
	c *Config
}

// NewChksum constructs the checksum step.
func NewChksum(c *Config) (*Chksum, error) {
	if err := c.Validate(); err != nil {
		return nil, err
	}

	return &Chksum{c: c}, nil
}

// EncodeBlock encodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *Chksum) EncodeBlock(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	switch s.c.GetChksumType() {
	case ChksumType_ChksumType_UNKNOWN:
		fallthrough
	case ChksumType_ChksumType_CRC32:
		return EncodeCRC32(data)
	default:
		return nil, errors.Errorf(
			"unknown checksum type: %s",
			s.c.GetChksumType().String(),
		)
	}
}

// DecodeBlock decodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *Chksum) DecodeBlock(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, nil
	}

	switch s.c.GetChksumType() {
	case ChksumType_ChksumType_UNKNOWN:
		fallthrough
	case ChksumType_ChksumType_CRC32:
		return DecodeCRC32(data)
	default:
		return nil, errors.Errorf(
			"unknown checksum type: %s",
			s.c.GetChksumType().String(),
		)
	}
}

// _ is a type assertion
var _ block_transform.Step = ((*Chksum)(nil))

package transform_chksum

import (
	"encoding/binary"
	"github.com/aperturerobotics/hydra/block/transform"
	"github.com/pkg/errors"
	"hash/crc32"
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
		cs := crc32.ChecksumIEEE(data)
		b := make([]byte, 4)
		binary.LittleEndian.PutUint32(b, cs)
		return append(data, b...), nil
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
		if len(data) < 5 {
			return nil, errors.New("short data")
		}
		// get last 4 bytes
		b := data[len(data)-4:]
		data = data[:len(data)-4]
		cs := crc32.ChecksumIEEE(data)
		cse := binary.LittleEndian.Uint32(b)
		if cs != cse {
			return nil, errors.Errorf("checksum mismatch %v != %v (indicated)", cs, cse)
		}
		return data, nil
	default:
		return nil, errors.Errorf(
			"unknown checksum type: %s",
			s.c.GetChksumType().String(),
		)
	}
}

// _ is a type assertion
var _ block_transform.Step = ((*Chksum)(nil))

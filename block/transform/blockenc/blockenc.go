package transform_blockenc

import (
	"github.com/aperturerobotics/bifrost/util/blockcrypt"
	"github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/util/padding"
	"github.com/pkg/errors"
)

// BlockEnc is the BlockEnc encryption step.
type BlockEnc struct {
	crypt blockcrypt.Crypt
}

// NewBlockEnc constructs the snappy step.
func NewBlockEnc(c *Config) (*BlockEnc, error) {
	crypt, err := blockcrypt.BuildBlockCrypt(c.GetBlockCrypt(), c.GetKey())
	if err != nil {
		return nil, err
	}
	return &BlockEnc{crypt: crypt}, nil
}

// EncodeBlock encodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *BlockEnc) EncodeBlock(data []byte) ([]byte, error) {
	data = padding.PadInPlace(data)
	if s.crypt == nil {
		return data, nil
	}
	s.crypt.Encrypt(data, data)
	return data, nil
}

// DecodeBlock decodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *BlockEnc) DecodeBlock(data []byte) ([]byte, error) {
	if s.crypt == nil {
		return data, nil
	}

	s.crypt.Decrypt(data, data)
	if len(data)%32 != 0 {
		return nil, errors.Errorf("data length %d must be a multiple of 32", len(data))
	}
	var err error
	data, err = padding.UnpadInPlace(data)
	return data, err
}

// _ is a type assertion
var _ block_transform.Step = ((*BlockEnc)(nil))

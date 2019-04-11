package transform_blockenc

import (
	"github.com/aperturerobotics/bifrost/util/blockcrypt"
	"github.com/aperturerobotics/hydra/block/transform"
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
	if s.crypt == nil {
		return data, nil
	}
	s.crypt.Encrypt(data, data)
	return data, nil
}

// DecodeBlock decodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *BlockEnc) DecodeBlock(data []byte) ([]byte, error) {
	s.crypt.Decrypt(data, data)
	return data, nil
}

// _ is a type assertion
var _ block_transform.Step = ((*BlockEnc)(nil))

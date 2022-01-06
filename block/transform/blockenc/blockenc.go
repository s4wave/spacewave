package transform_blockenc

import (
	"sync"

	"github.com/aperturerobotics/bifrost/util/blockcrypt"
	block_transform "github.com/aperturerobotics/hydra/block/transform"
	"github.com/aperturerobotics/hydra/util/padding"
	"github.com/pkg/errors"
)

// BlockEnc is the BlockEnc encryption step.
type BlockEnc struct {
	// contains blockcrypt.Crypt
	cryptArena sync.Pool
}

// NewBlockEnc constructs the block enc step.
func NewBlockEnc(c *Config) (*BlockEnc, error) {
	crypt, err := blockcrypt.BuildBlockCrypt(c.GetBlockCrypt(), c.GetKey())
	if err != nil {
		return nil, err
	}
	enc := &BlockEnc{}
	enc.cryptArena = sync.Pool{
		New: func() interface{} {
			// note: we asserted this doesn't error above
			crypt, _ := blockcrypt.BuildBlockCrypt(c.GetBlockCrypt(), c.GetKey())
			return blockcrypt.Crypt(crypt)
		},
	}
	enc.cryptArena.Put(crypt)
	return enc, nil
}

// EncodeBlock encodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *BlockEnc) EncodeBlock(data []byte) ([]byte, error) {
	data = padding.PadInPlace(data)
	crypt := s.getCrypt()
	crypt.Encrypt(data, data)
	s.relCrypt(crypt)
	return data, nil
}

// DecodeBlock decodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *BlockEnc) DecodeBlock(data []byte) ([]byte, error) {
	crypt := s.getCrypt()
	crypt.Decrypt(data, data)
	s.relCrypt(crypt)
	if len(data)%32 != 0 {
		return nil, errors.Errorf("data length %d must be a multiple of 32", len(data))
	}
	var err error
	data, err = padding.UnpadInPlace(data)
	return data, err
}

// getCrypt gets a crypt from the pool.
func (s *BlockEnc) getCrypt() blockcrypt.Crypt {
	nv := s.cryptArena.Get()
	return nv.(blockcrypt.Crypt)
}

// relCrypt releases a crypt back to the pool.
func (s *BlockEnc) relCrypt(c blockcrypt.Crypt) {
	s.cryptArena.Put(c)
}

// _ is a type assertion
var _ block_transform.Step = ((*BlockEnc)(nil))

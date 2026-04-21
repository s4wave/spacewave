package transform_blockenc

import (
	"sync"

	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/util/blockenc"
)

// BlockEnc is the BlockEnc encryption step.
type BlockEnc struct {
	// contains blockenc.Method
	cryptArena sync.Pool
	alloc      blockenc.AllocFn
}

// NewBlockEnc constructs the block enc step.
func NewBlockEnc(c *Config) (*BlockEnc, error) {
	crypt, err := blockenc.BuildBlockEnc(c.GetBlockEnc(), c.GetKey())
	if err != nil {
		return nil, err
	}
	enc := &BlockEnc{alloc: blockenc.DefaultAllocFn()}
	enc.cryptArena = sync.Pool{
		New: func() any {
			// note: we asserted this doesn't error above
			crypt, _ := blockenc.BuildBlockEnc(c.GetBlockEnc(), c.GetKey())
			return crypt
		},
	}
	enc.cryptArena.Put(crypt)
	return enc, nil
}

// EncodeBlock encodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *BlockEnc) EncodeBlock(data []byte) ([]byte, error) {
	crypt := s.getCrypt()
	d, err := crypt.Encrypt(s.alloc, data)
	s.relCrypt(crypt)
	return d, err
}

// DecodeBlock decodes the block according to the config.
// May reuse the same byte slice if possible.
func (s *BlockEnc) DecodeBlock(data []byte) ([]byte, error) {
	crypt := s.getCrypt()
	out, err := crypt.Decrypt(s.alloc, data)
	s.relCrypt(crypt)
	return out, err
}

// getCrypt gets a crypt from the pool.
func (s *BlockEnc) getCrypt() blockenc.Method {
	nv := s.cryptArena.Get()
	return nv.(blockenc.Method)
}

// relCrypt releases a crypt back to the pool.
func (s *BlockEnc) relCrypt(c blockenc.Method) {
	s.cryptArena.Put(c)
}

// _ is a type assertion
var _ block_transform.Step = ((*BlockEnc)(nil))

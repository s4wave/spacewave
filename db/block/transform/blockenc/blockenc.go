package transform_blockenc

import (
	"sync"

	block_transform "github.com/s4wave/spacewave/db/block/transform"
	"github.com/s4wave/spacewave/db/util/blockenc"
)

// BlockEnc is the BlockEnc encryption step.
type BlockEnc struct {
	// cryptArena pools blockenc.Method instances. The pool is populated
	// lazily by its New func on the first Get that finds the pool empty.
	// sync.Pool may discard pooled items at any GC tick, so the pool floor
	// is best-effort; New must always be able to build a fresh Method.
	cryptArena sync.Pool
	alloc      blockenc.AllocFn
}

// NewBlockEnc constructs the block enc step.
func NewBlockEnc(c *Config) (*BlockEnc, error) {
	// Validate the config eagerly so the pool's New func can safely
	// discard the build error on subsequent calls.
	if _, err := blockenc.BuildBlockEnc(c.GetBlockEnc(), c.GetKey()); err != nil {
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

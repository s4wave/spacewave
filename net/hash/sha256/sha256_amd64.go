//go:build amd64 && gc && !purego

package sha256

import (
	"crypto/sha256"
	"encoding/binary"
	"hash"

	"golang.org/x/sys/cpu"
)

// useAVX selects the AVX code: crypto/sha256 has SHA-NI and AVX2 code, so it
// is used wherever AVX2 and BMI2 are, and its generic code is slower than AVX
// everywhere else.
var useAVX = cpu.X86.HasAVX && cpu.X86.HasSSSE3 && !(cpu.X86.HasAVX2 && cpu.X86.HasBMI2)

// blockAvx hashes message, a whole number of blocks, into h.
//
//go:noescape
func blockAvx(h []uint32, message []uint8)

// New returns a new hash.Hash computing the SHA-256 checksum.
func New() hash.Hash {
	if !useAVX {
		return sha256.New()
	}
	return newDigest()
}

// Sum256 returns the SHA-256 checksum of data.
func Sum256(data []byte) [Size]byte {
	if !useAVX {
		return sha256.Sum256(data)
	}
	var d digest
	d.Reset()
	_, _ = d.Write(data)
	return d.checkSum()
}

// digest is a running SHA-256 checksum over the AVX block function.
type digest struct {
	h   [8]uint32
	x   [BlockSize]byte
	nx  int
	len uint64
}

func newDigest() *digest {
	d := new(digest)
	d.Reset()
	return d
}

// Reset returns the digest to its initial state.
func (d *digest) Reset() {
	d.h = [8]uint32{0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19}
	d.nx = 0
	d.len = 0
}

// Size returns the checksum size.
func (d *digest) Size() int { return Size }

// BlockSize returns the block size.
func (d *digest) BlockSize() int { return BlockSize }

// Write adds p to the running checksum.
func (d *digest) Write(p []byte) (int, error) {
	nn := len(p)
	d.len += uint64(nn)
	if d.nx > 0 {
		n := copy(d.x[d.nx:], p)
		d.nx += n
		if d.nx < BlockSize {
			return nn, nil
		}
		blockAvx(d.h[:], d.x[:])
		d.nx = 0
		p = p[n:]
	}
	if n := len(p) &^ (BlockSize - 1); n > 0 {
		blockAvx(d.h[:], p[:n])
		p = p[n:]
	}
	if len(p) > 0 {
		d.nx = copy(d.x[:], p)
	}
	return nn, nil
}

// Sum appends the checksum of the data written so far to in, leaving the
// digest unchanged.
func (d *digest) Sum(in []byte) []byte {
	d0 := *d
	sum := d0.checkSum()
	return append(in, sum[:]...)
}

// checkSum pads the message and returns its checksum.
func (d *digest) checkSum() [Size]byte {
	length := d.len
	var tmp [BlockSize + 8]byte
	tmp[0] = 0x80
	rem := length % BlockSize
	pad := 56 - rem
	if rem >= 56 {
		pad = BlockSize + 56 - rem
	}
	binary.BigEndian.PutUint64(tmp[pad:], length<<3)
	_, _ = d.Write(tmp[:pad+8])

	var sum [Size]byte
	for i, word := range d.h {
		binary.BigEndian.PutUint32(sum[i*4:], word)
	}
	return sum
}

// _ checks that digest is a hash.Hash.
var _ hash.Hash = (*digest)(nil)

//go:build goscript

// Package blake3 provides Spacewave's BLAKE3 owner API.
package blake3

import (
	"hash"
	"io"
	"syscall/js"

	"github.com/pkg/errors"
)

const (
	blockSize = 64
	size256   = 32
	size512   = 64
)

type hasherMode uint8

const (
	hasherModeHash hasherMode = iota
	hasherModeKeyed
	hasherModeDeriveKey
)

// Digest captures a BLAKE3 output stream snapshot.
type Digest struct {
	h      *Hasher
	offset int64
}

// Hasher is a hash.Hash for BLAKE3.
type Hasher struct {
	mode    hasherMode
	size    int
	key     [32]byte
	context string
	buf     []byte
}

var _ hash.Hash = (*Hasher)(nil)

// DeriveKey derives key material for the context and input material.
func DeriveKey(context string, material []byte, out []byte) {
	sidecarDeriveKey(context, material, out)
}

// New returns a new unkeyed BLAKE3 hasher.
func New() *Hasher {
	return &Hasher{size: size256}
}

// NewDeriveKey returns a BLAKE3 key-derivation hasher for the context.
func NewDeriveKey(context string) *Hasher {
	return &Hasher{mode: hasherModeDeriveKey, size: size256, context: context}
}

// NewKeyed returns a keyed BLAKE3 hasher.
func NewKeyed(key []byte) (*Hasher, error) {
	if len(key) != size256 {
		return nil, errors.New("invalid key size")
	}
	h := &Hasher{mode: hasherModeKeyed, size: size256}
	copy(h.key[:], key)
	return h, nil
}

// Sum256 returns the first 256 bits of the unkeyed BLAKE3 digest.
func Sum256(data []byte) (sum [32]byte) {
	sidecarHash(data, sum[:])
	return sum
}

// Sum512 returns the first 512 bits of the unkeyed BLAKE3 digest.
func Sum512(data []byte) (sum [64]byte) {
	sidecarHash(data, sum[:])
	return sum
}

// Write implements hash.Hash. It never returns an error.
func (h *Hasher) Write(p []byte) (int, error) {
	h.buf = append(h.buf, p...)
	return len(p), nil
}

// WriteString writes a string without forcing callers to allocate first.
func (h *Hasher) WriteString(p string) (int, error) {
	h.buf = append(h.buf, p...)
	return len(p), nil
}

// Reset resets the hasher to its initial state.
func (h *Hasher) Reset() {
	h.buf = h.buf[:0]
}

// Clone returns a new hasher with the same state.
func (h *Hasher) Clone() *Hasher {
	clone := *h
	if len(h.buf) != 0 {
		clone.buf = append([]byte(nil), h.buf...)
	}
	return &clone
}

// Size returns the number of bytes appended by Sum.
func (h *Hasher) Size() int {
	return h.size
}

// BlockSize returns BLAKE3's natural block size.
func (h *Hasher) BlockSize() int {
	return blockSize
}

// Sum appends the current digest to b.
func (h *Hasher) Sum(b []byte) []byte {
	digest := make([]byte, h.size)
	h.sumInto(digest)
	return append(b, digest...)
}

// Digest returns an extendable-output digest snapshot.
func (h *Hasher) Digest() *Digest {
	return &Digest{h: h.Clone()}
}

// Read fills p from the digest stream.
func (d *Digest) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	maxInt := int64(^uint(0) >> 1)
	if d.offset < 0 || d.offset > maxInt-int64(len(p)) {
		return 0, errors.New("blake3 digest offset overflows int")
	}
	start := int(d.offset)
	buf := make([]byte, start+len(p))
	d.h.sumInto(buf)
	copy(p, buf[start:])
	d.offset += int64(len(p))
	return len(p), nil
}

// Seek sets the digest stream offset.
func (d *Digest) Seek(offset int64, whence int) (int64, error) {
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		offset += d.offset
	case io.SeekEnd:
		return 0, errors.New("seek from end not supported")
	default:
		return 0, errors.Errorf("invalid whence: %d", whence)
	}
	if offset < 0 {
		return 0, errors.New("seek before start")
	}
	d.offset = offset
	return offset, nil
}

func (h *Hasher) sumInto(out []byte) {
	switch h.mode {
	case hasherModeHash:
		sidecarHash(h.buf, out)
	case hasherModeKeyed:
		sidecarKeyedHash(h.key[:], h.buf, out)
	case hasherModeDeriveKey:
		sidecarDeriveKey(h.context, h.buf, out)
	}
}

func sidecarHash(data []byte, out []byte) {
	callSidecarBytes("hash", out, bytesToJS(data), len(out))
}

func sidecarKeyedHash(key []byte, data []byte, out []byte) {
	callSidecarBytes("keyedHash", out, bytesToJS(key), bytesToJS(data), len(out))
}

func sidecarDeriveKey(context string, material []byte, out []byte) {
	callSidecarBytes("deriveKey", out, context, bytesToJS(material), len(out))
}

func callSidecarBytes(name string, out []byte, args ...any) {
	result := js.Global().Get("BLDR_BLAKE3").Call(name, args...)
	js.CopyBytesToGo(out, result)
}

func bytesToJS(data []byte) js.Value {
	out := js.Global().Get("Uint8Array").New(len(data))
	js.CopyBytesToJS(out, data)
	return out
}

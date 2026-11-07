package writelog

import (
	"bytes"
	"strconv"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/net/hash"
)

// Kind is the kind of a line.
type Kind byte

// Line kinds.
const (
	// KindPut is a block the store accepted, including one it held.
	KindPut Kind = 'p'
	// KindRoot is a block a named root was set to.
	KindRoot Kind = 'r'
	// KindFill is a block read back from the remote.
	KindFill Kind = 'f'
	// kindEpoch opens a segment.
	kindEpoch Kind = 'e'
	// kindClose ends an epoch.
	kindClose Kind = 'c'
)

// Line is one record of the log.
type Line struct {
	// Seq numbers the line in its epoch, from one.
	Seq uint64
	// Kind is the line's kind: put, root or fill.
	Kind Kind
	// Hash is the block's hash.
	Hash *hash.Hash
}

// maxLineBytes bounds a record line: a sequence of up to 20 digits, the kind,
// the b58 hash of up to 64 bytes, and the separators.
const maxLineBytes = 20 + 3 + 88 + 1

// hashScratchBytes bounds the encoded hash a line holds without allocating.
const hashScratchBytes = 64

// b58Alphabet is the alphabet of hash.MarshalString.
const b58Alphabet = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// appendRecord appends the line of a record, encoding the hash in scratch.
func appendRecord(b []byte, seq uint64, kind Kind, h *hash.Hash, scratch *[hashScratchBytes]byte) []byte {
	// Write the sequence and the kind.
	b = strconv.AppendUint(b, seq, 10)
	b = append(b, ' ', byte(kind), ' ')

	// Encode the hash as hash.MarshalString does, without allocating when
	// it fits the scratch.
	var enc []byte
	if n := h.SizeVT(); n <= len(scratch) {
		if _, err := h.MarshalToSizedBufferVT(scratch[:n]); err == nil {
			enc = scratch[:n]
		}
	}
	if enc == nil {
		enc = h.MarshalDigest()
	}
	b = appendBase58(b, enc)
	return append(b, '\n')
}

// appendMarker appends an epoch or close line.
func appendMarker(b []byte, seq uint64, kind Kind, epoch string) []byte {
	b = strconv.AppendUint(b, seq, 10)
	b = append(b, ' ', byte(kind), ' ')
	b = append(b, epoch...)
	return append(b, '\n')
}

// b58Limb is 58^5, the base of a limb of appendBase58's number: a limb fits
// a uint32, and a limb times 2^32 plus a carry fits a uint64.
const b58Limb = 58 * 58 * 58 * 58 * 58

// appendBase58 appends src in base58, as the base58 package encodes it, for
// up to hashScratchBytes bytes without allocating. It converts four input
// bytes at a time into limbs of five digits.
func appendBase58(b, src []byte) []byte {
	// Write a '1' for each leading zero byte, or encode a longer value by the
	// package.
	if len(src) > hashScratchBytes {
		return append(b, b58.Encode(src)...)
	}
	zeros := 0
	for zeros < len(src) && src[zeros] == 0 {
		zeros++
	}
	for range zeros {
		b = append(b, b58Alphabet[0])
	}

	// Convert the rest to limbs, least significant first: a short chunk
	// first, so every later chunk is four bytes.
	var limbs [hashScratchBytes*138/100/5 + 2]uint32
	n := 0
	rest := src[zeros:]
	for len(rest) != 0 {
		k := len(rest) % 4
		if k == 0 {
			k = 4
		}
		var carry uint64
		for _, c := range rest[:k] {
			carry = carry<<8 | uint64(c)
		}
		shift := uint64(1) << (8 * k)
		for i := range n {
			v := uint64(limbs[i])*shift + carry
			limbs[i], carry = uint32(v%b58Limb), v/b58Limb //nolint:gosec // v%b58Limb fits a uint32.
		}
		for carry != 0 {
			limbs[n], carry = uint32(carry%b58Limb), carry/b58Limb //nolint:gosec // as above.
			n++
		}
		rest = rest[k:]
	}

	// Write the top limb without its leading zero digits, then five digits
	// for each other limb.
	for i := n - 1; i >= 0; i-- {
		var digits [5]byte
		v := limbs[i]
		for j := 4; j >= 0; j-- {
			digits[j] = b58Alphabet[v%58]
			v /= 58
		}
		d := digits[:]
		if i == n-1 {
			for len(d) > 1 && d[0] == b58Alphabet[0] {
				d = d[1:]
			}
		}
		b = append(b, d...)
	}
	return b
}

// parseLine parses one line without its newline.
func parseLine(b []byte) (seq uint64, kind Kind, rest []byte, err error) {
	// Split the sequence, the kind and the rest.
	num, tail, ok := bytes.Cut(b, []byte{' '})
	if !ok || len(tail) < 3 || tail[1] != ' ' {
		return 0, 0, nil, errors.Errorf("malformed log line %q", b)
	}

	// Parse the sequence.
	seq, err = strconv.ParseUint(string(num), 10, 64)
	if err != nil {
		return 0, 0, nil, errors.Wrapf(err, "log line %q", b)
	}
	return seq, Kind(tail[0]), tail[2:], nil
}

// parseHash parses a line's b58 hash.
func parseHash(b []byte) (*hash.Hash, error) {
	h := &hash.Hash{}
	if err := h.ParseFromB58(string(b)); err != nil {
		return nil, errors.Wrapf(err, "log line hash %q", b)
	}
	if err := h.Validate(); err != nil {
		return nil, errors.Wrapf(err, "log line hash %q", b)
	}
	return h, nil
}

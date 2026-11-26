package refwalk

import (
	"bytes"
	"slices"
	"strconv"
	"strings"

	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/net/hash"
)

// scanDepth bounds how deep ScanHits reads nested messages, well past the
// nesting of any block type.
const scanDepth = 32

// ScanHit is one hash a scan found.
type ScanHit struct {
	// Ref is the hash as a block ref.
	Ref *block.BlockRef
	// Path is the field numbers from the message down to the field holding
	// the hash, joined by dots.
	Path string
}

// ScanHits reads data as a protobuf message of unknown type and returns
// every hash it holds at any depth, each once, in the order the bytes hold
// them, with the path of the first field holding it: each length-delimited
// field whose bytes are exactly a hash.Hash of a known type with that type's
// digest length, which is how a block.BlockRef and every message built on it
// encode a ref. It reports false when data is not a well-formed message, as
// raw bytes are not.
//
// The result holds every ref a typed decode of the same bytes extracts, as
// long as the type encodes its refs as BlockRefs, and may hold more: a hash
// that is not a ref, or bytes that only read as a message. So it suits a
// reader that must not miss a ref and can afford a stranger, and it does not
// tell which type a child is.
func ScanHits(data []byte) ([]ScanHit, bool) {
	// Keep each hash once.
	var out []ScanHit
	seen := make(map[string]struct{})
	ok := scanMessage(data, scanDepth, nil, func(typ hash.HashType, digest []byte, path []uint64) {
		// Skip a hash already kept, and copy the digest out of data.
		key := scanKey(typ, digest)
		if _, dup := seen[key]; dup {
			return
		}
		seen[key] = struct{}{}
		ref := &block.BlockRef{Hash: &hash.Hash{HashType: typ, Hash: bytes.Clone(digest)}}
		out = append(out, ScanHit{Ref: ref, Path: scanPath(path)})
	})
	if !ok {
		return nil, false
	}
	return out, true
}

// scanPath joins field numbers with dots.
func scanPath(path []uint64) string {
	var b strings.Builder
	for i, field := range path {
		if i != 0 {
			b.WriteByte('.')
		}
		b.WriteString(strconv.FormatUint(field, 10))
	}
	return b.String()
}

// scanCompare scans data and returns the refs among refs the scan does not
// find, each once, and the hashes it finds that are not among refs.
func scanCompare(refs []*block.BlockRef, data []byte) (missed []*block.BlockRef, extra []ScanHit) {
	// Index the scanned hashes.
	scanned, _ := ScanHits(data)
	seen := make(map[string]struct{}, len(scanned))
	for _, hit := range scanned {
		seen[refKey(hit.Ref)] = struct{}{}
	}

	// Take each typed ref once, since a block may name one child twice, and
	// keep those the scan did not find.
	typed := make(map[string]struct{}, len(refs))
	for _, ref := range refs {
		key := refKey(ref)
		if _, dup := typed[key]; dup {
			continue
		}
		typed[key] = struct{}{}
		if _, ok := seen[key]; !ok {
			missed = append(missed, ref)
		}
	}

	// Keep the scanned hashes no typed ref names.
	for _, hit := range scanned {
		if _, ok := typed[refKey(hit.Ref)]; !ok {
			extra = append(extra, hit)
		}
	}
	return missed, extra
}

// refKey identifies a ref's hash as scanKey does.
func refKey(ref *block.BlockRef) string {
	return scanKey(ref.GetHash().GetHashType(), ref.GetHash().GetHash())
}

// scanKey identifies a hash by its type and digest.
func scanKey(typ hash.HashType, digest []byte) string {
	return string(rune(typ)) + string(digest)
}

// scanShapeMax bounds the outline ScanShape returns.
const scanShapeMax = 1200

// ScanShape outlines data as the scan reads it, with no content: each field
// as its number and wire type, a length-delimited field with its length, a
// hash as H and its type, a nested message in braces, and "!" where the
// bytes stop reading as a message. It says why a scan missed a ref.
func ScanShape(data []byte) string {
	var b strings.Builder
	scanShape(&b, data, scanDepth)
	return b.String()
}

// scanShape writes the outline of b's fields and reports whether b is
// well-formed.
func scanShape(out *strings.Builder, b []byte, depth int) bool {
	for len(b) != 0 {
		// Stop at the bound, and at a tag that does not read.
		if out.Len() > scanShapeMax {
			out.WriteString(" ...")
			return true
		}
		tag, n := scanVarint(b)
		if n == 0 || tag>>3 == 0 {
			out.WriteString(" !tag")
			return false
		}
		b = b[n:]
		out.WriteString(" " + strconv.FormatUint(tag>>3, 10))

		// Outline a scalar by its wire type.
		var skip int
		switch tag & 7 {
		case 0:
			_, skip = scanVarint(b)
			out.WriteString(":v")
		case 1:
			skip = 8
			out.WriteString(":f8")
		case 5:
			skip = 4
			out.WriteString(":f4")
		case 2:
		default:
			out.WriteString(":!wire" + strconv.FormatUint(tag&7, 10))
			return false
		}
		if tag&7 != 2 {
			if skip == 0 || len(b) < skip {
				out.WriteString("!")
				return false
			}
			b = b[skip:]
			continue
		}

		// Outline a length-delimited field by what its bytes read as.
		body, rest, ok := scanBytes(b)
		if !ok {
			out.WriteString(":L!")
			return false
		}
		b = rest
		out.WriteString(":L" + strconv.Itoa(len(body)))
		if typ, _, ok := scanHash(body); ok {
			out.WriteString("=H" + strconv.FormatInt(int64(typ), 10))
			continue
		}
		if depth == 0 || len(body) == 0 {
			continue
		}
		var nested strings.Builder
		if scanShape(&nested, body, depth-1) {
			out.WriteString("{" + nested.String() + " }")
		}
	}
	return true
}

// scanBytes reads a length-delimited field's bytes from the front of b and
// returns them with what follows them. It reports false when the length is
// malformed or runs past the end of b.
func scanBytes(b []byte) (body, rest []byte, ok bool) {
	size, n := scanVarint(b)
	if n == 0 || size > uint64(len(b)-n) { //nolint:gosec // n is at most len(b).
		return nil, nil, false
	}
	end := n + int(size) //nolint:gosec // size is at most len(b)-n.
	return b[n:end], b[end:], true
}

// scanMessage reads b as the fields of one message and calls found with each
// hash in it and the path of its field under path, descending at most depth
// levels. It reports false when b is not well-formed; hashes found before
// the malformed field were still passed to found, so a caller reading a
// nested field buffers them.
func scanMessage(b []byte, depth int, path []uint64, found func(typ hash.HashType, digest []byte, path []uint64)) bool {
	for len(b) != 0 {
		// Read the field's tag.
		tag, n := scanVarint(b)
		if n == 0 || tag>>3 == 0 {
			return false
		}
		b = b[n:]

		// Skip a scalar, and take the bytes of a length-delimited field.
		switch tag & 7 {
		case 0:
			_, n := scanVarint(b)
			if n == 0 {
				return false
			}
			b = b[n:]
			continue
		case 1:
			if len(b) < 8 {
				return false
			}
			b = b[8:]
			continue
		case 5:
			if len(b) < 4 {
				return false
			}
			b = b[4:]
			continue
		case 2:
		default:
			return false
		}
		body, rest, ok := scanBytes(b)
		if !ok {
			return false
		}
		b = rest

		// Report a hash, and otherwise the hashes of a nested message. A
		// field that is not a message, such as a string, reports nothing.
		at := append(slices.Clone(path), tag>>3)
		if typ, digest, ok := scanHash(body); ok {
			found(typ, digest, at)
			continue
		}
		if depth == 0 {
			continue
		}
		type hit struct {
			typ    hash.HashType
			digest []byte
			path   []uint64
		}
		var hits []hit
		nested := scanMessage(body, depth-1, at, func(typ hash.HashType, digest []byte, path []uint64) {
			hits = append(hits, hit{typ, digest, path})
		})
		if nested {
			for _, h := range hits {
				found(h.typ, h.digest, h.path)
			}
		}
	}
	return true
}

// scanHash reads b as exactly a hash.Hash: its type, then a digest of that
// type's length, and nothing more.
func scanHash(b []byte) (hash.HashType, []byte, bool) {
	// Read the two fields in the order every encoder writes them.
	if len(b) < 4 || b[0] != 0x08 || b[2] != 0x12 || int(b[3]) != len(b)-4 {
		return 0, nil, false
	}
	typ := hash.HashType(b[1])
	digest := b[4:]

	// Accept only a digest of the type's length.
	var want int
	switch typ {
	case hash.HashType_HashType_SHA256, hash.HashType_HashType_BLAKE3:
		want = 32
	case hash.HashType_HashType_SHA1:
		want = 20
	default:
		return 0, nil, false
	}
	return typ, digest, len(digest) == want
}

// scanVarint reads a varint from the start of b and returns it with its
// length, or a zero length when b holds none.
func scanVarint(b []byte) (uint64, int) {
	var v uint64
	for i := 0; i < len(b) && i < 10; i++ {
		v |= uint64(b[i]&0x7f) << (7 * i)
		if b[i] < 0x80 {
			return v, i + 1
		}
	}
	return 0, 0
}

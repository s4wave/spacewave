package identity

import (
	"crypto/sha256"
	"encoding/binary"
	"io"

	b58 "github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/packfile/writer"
)

const (
	// PackIDPrefix is the v1 packfile identifier prefix.
	PackIDPrefix = "pfv1_"
	// WriterVersionV1 is the v1 kvfile writer identity namespace.
	WriterVersionV1 = "kvfile-writer-v1"
)

const packIDDigestLen = 32

// BuildPackID builds a v1 packfile identifier for one resource-scoped pack.
func BuildPackID(resourceID string, result *writer.PackResult) (string, error) {
	if resourceID == "" {
		return "", errors.New("resource id is empty")
	}
	if result == nil {
		return "", errors.New("pack result is nil")
	}
	if len(result.SortedKeyDigest) != packIDDigestLen {
		return "", errors.New("sorted key digest is invalid")
	}
	if len(result.PackBytesDigest) != packIDDigestLen {
		return "", errors.New("pack bytes digest is invalid")
	}
	policyTag := result.PolicyTag
	if policyTag == "" {
		policyTag = writer.PolicyTag(writer.DefaultPolicy())
	}
	valueOrder := result.ValueOrderPolicy
	if valueOrder == "" {
		valueOrder = writer.ValueOrderIterator
	}
	h := sha256.New()
	writePart(h, []byte("spacewave-packfile-id-v1"))
	writePart(h, []byte(resourceID))
	writePart(h, []byte(WriterVersionV1))
	writePart(h, []byte(policyTag))
	writePart(h, []byte(valueOrder))
	writePart(h, result.SortedKeyDigest)
	writePart(h, result.PackBytesDigest)
	sum := h.Sum(nil)
	return PackIDPrefix + b58.Encode(sum), nil
}

// ValidatePackID validates the v1 packfile identifier shape.
func ValidatePackID(id string) error {
	if len(id) <= len(PackIDPrefix) || id[:len(PackIDPrefix)] != PackIDPrefix {
		return errors.New("pack id must start with pfv1_")
	}
	digest, err := b58.Decode(id[len(PackIDPrefix):])
	if err != nil {
		return errors.Wrap(err, "pack id suffix must be base58")
	}
	if len(digest) != packIDDigestLen {
		return errors.New("pack id suffix must decode to 32 bytes")
	}
	return nil
}

func writePart(h io.Writer, part []byte) {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(part))) //nolint:gosec // pack identity parts are bounded strings and fixed-size digests in the v1 framing.
	_, _ = h.Write(lenBuf[:])
	_, _ = h.Write(part)
}

package identity

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"sort"
	"strconv"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/core/provider/spacewave/packfile/writer"
)

const (
	// PackIDPrefix is the v1 packfile identifier prefix.
	PackIDPrefix = "pfv1_"
	// WriterVersionV1 is the v1 kvfile writer identity namespace.
	WriterVersionV1 = "kvfile-writer-v1"
	// ValueOrderIterator records that physical kvfile value order follows the
	// writer iterator order.
	ValueOrderIterator = "iterator"
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
		policyTag = PolicyTag(writer.DefaultPolicy())
	}
	valueOrder := result.ValueOrderPolicy
	if valueOrder == "" {
		valueOrder = ValueOrderIterator
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
	return PackIDPrefix + hex.EncodeToString(sum), nil
}

// DigestSortedKeys digests a pack's block keys independent of physical value
// order.
func DigestSortedKeys(keys [][]byte) []byte {
	sorted := make([][]byte, len(keys))
	for i, key := range keys {
		sorted[i] = append([]byte(nil), key...)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return string(sorted[i]) < string(sorted[j])
	})
	h := sha256.New()
	writePart(h, []byte("spacewave-packfile-key-digest-v1"))
	for _, key := range sorted {
		writePart(h, key)
	}
	return h.Sum(nil)
}

// PolicyTag returns the canonical v1 policy tag for a pack construction policy.
func PolicyTag(policy writer.Policy) string {
	return "max-bytes=" + strconv.FormatInt(policy.MaxPackBytes, 10) +
		";max-blocks=" + strconv.FormatUint(policy.MaxBlocksPerPack, 10) +
		";bloom-expected=" + strconv.FormatUint(policy.BloomExpectedBlocks, 10) +
		";bloom-fp=" + strconv.FormatFloat(policy.BloomFalsePositive, 'g', -1, 64) +
		";require-bloom=" + strconv.FormatBool(policy.RequireBloomFilter) +
		";require-count=" + strconv.FormatBool(policy.RequireBlockCount) +
		";require-created-at=" + strconv.FormatBool(policy.RequireCreatedAt)
}

// ValidatePackID validates the v1 packfile identifier shape.
func ValidatePackID(id string) error {
	if len(id) != len(PackIDPrefix)+packIDDigestLen*2 {
		return errors.New("pack id must be pfv1_ followed by 64 lowercase hex characters")
	}
	if id[:len(PackIDPrefix)] != PackIDPrefix {
		return errors.New("pack id must start with pfv1_")
	}
	for _, ch := range id[len(PackIDPrefix):] {
		if (ch >= '0' && ch <= '9') || (ch >= 'a' && ch <= 'f') {
			continue
		}
		return errors.New("pack id contains invalid character")
	}
	return nil
}

func writePart(h interface{ Write([]byte) (int, error) }, part []byte) {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(part)))
	_, _ = h.Write(lenBuf[:])
	_, _ = h.Write(part)
}

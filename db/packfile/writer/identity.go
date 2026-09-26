package writer

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"io"
	"slices"
	"strconv"
)

// ValueOrderIterator records that physical kvfile value order follows the
// writer iterator order.
const ValueOrderIterator = "iterator"

// DigestSortedKeys digests a pack's block keys independent of physical value
// order.
func DigestSortedKeys(keys [][]byte) []byte {
	sorted := make([][]byte, len(keys))
	for i, key := range keys {
		sorted[i] = bytes.Clone(key)
	}
	slices.SortFunc(sorted, bytes.Compare)
	h := sha256.New()
	writePart(h, []byte("spacewave-packfile-key-digest-v1"))
	for _, key := range sorted {
		writePart(h, key)
	}
	return h.Sum(nil)
}

// PolicyTag returns the canonical v1 policy tag for a pack construction policy.
func PolicyTag(policy Policy) string {
	return "max-bytes=" + strconv.FormatInt(policy.MaxPackBytes, 10) +
		";max-blocks=" + strconv.FormatUint(policy.MaxBlocksPerPack, 10) +
		";bloom-fp=" + strconv.FormatFloat(policy.BloomFalsePositive, 'g', -1, 64) +
		";require-bloom=" + strconv.FormatBool(policy.RequireBloomFilter) +
		";require-count=" + strconv.FormatBool(policy.RequireBlockCount) +
		";require-created-at=" + strconv.FormatBool(policy.RequireCreatedAt)
}

func writePart(h io.Writer, part []byte) {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(part))) //nolint:gosec // pack identity parts are bounded strings and fixed-size digests in the v1 framing.
	_, _ = h.Write(lenBuf[:])
	_, _ = h.Write(part)
}

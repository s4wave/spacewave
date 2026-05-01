package writer

import (
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"strconv"
)

const valueOrderIterator = "iterator"

func digestSortedKeys(keys [][]byte) []byte {
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

func policyTag(policy Policy) string {
	return "max-bytes=" + strconv.FormatInt(policy.MaxPackBytes, 10) +
		";max-blocks=" + strconv.FormatUint(policy.MaxBlocksPerPack, 10) +
		";bloom-expected=" + strconv.FormatUint(policy.BloomExpectedBlocks, 10) +
		";bloom-fp=" + strconv.FormatFloat(policy.BloomFalsePositive, 'g', -1, 64) +
		";require-bloom=" + strconv.FormatBool(policy.RequireBloomFilter) +
		";require-count=" + strconv.FormatBool(policy.RequireBlockCount) +
		";require-created-at=" + strconv.FormatBool(policy.RequireCreatedAt)
}

func writePart(h interface{ Write([]byte) (int, error) }, part []byte) {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(part)))
	_, _ = h.Write(lenBuf[:])
	_, _ = h.Write(part)
}

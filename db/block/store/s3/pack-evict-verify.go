//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"crypto/sha256"
	"slices"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
)

// Reasons a pack fails VerifyPack. They are a closed set, so they can label
// metrics.
const (
	// VerifyMissing is the reason when the bucket has no object for the pack.
	VerifyMissing = "missing"
	// VerifyOversize is the reason when the recorded size, or the object's
	// size, is past the bound VerifyPack reads.
	VerifyOversize = "oversize"
	// VerifyShort is the reason when the bucket returned fewer bytes than the
	// recorded size.
	VerifyShort = "short"
	// VerifyIndex is the reason when the index in the fetched bytes does not
	// parse, or an entry lies outside them or names no valid block key.
	VerifyIndex = "index"
	// VerifyHash is the reason when a value does not decode or hash to its
	// key, or the bytes differ from the SHA-256 recorded at upload.
	VerifyHash = "hash"
	// VerifyFetch is the reason when the fetch failed with another error.
	VerifyFetch = "fetch"
)

// verifySlackBytes is how far past its recorded size a pack's object may be
// before VerifyPack refuses it without reading.
const verifySlackBytes = 64 << 10

// verifyFailed returns a PackVerifyError.
func verifyFailed(reason string, err error) error {
	return &PackVerifyError{Reason: reason, Err: err}
}

// LocatePacks names the pack the local index places each block in, reading
// only the local index. Eviction groups blocks by these packs and verifies
// each one with VerifyPack; the location itself proves nothing.
func (s *PackStore) LocatePacks(ctx context.Context, hashes []*hash.Hash) ([]PackLocation, error) {
	// Locate only through the local index.
	if s.index == nil {
		return nil, errors.New("pack store has no local index to locate packs")
	}

	// Read every block's row in one snapshot.
	keys := make([][]byte, len(hashes))
	for i, h := range hashes {
		if h != nil {
			keys[i] = packfile.BlockKey(h)
		}
	}
	ids, err := s.index.locate(ctx, keys, make([]bool, len(keys)))
	if err != nil {
		return nil, err
	}

	// Size each named pack from its cached entry; a pack with no entry
	// locates nothing.
	uniq := slices.Compact(slices.Sorted(slices.Values(slices.DeleteFunc(slices.Clone(ids), func(id string) bool { return id == "" }))))
	entries, err := s.index.readEntries(ctx, uniq)
	if err != nil {
		return nil, err
	}
	sizes := make(map[string]int64, len(entries))
	for _, entry := range entries {
		sizes[entry.GetId()] = int64(entry.GetSizeBytes()) //nolint:gosec // pack sizes are far below MaxInt64.
	}
	out := make([]PackLocation, len(ids))
	for i, id := range ids {
		if size, ok := sizes[id]; ok && size > 0 {
			out[i] = PackLocation{ID: id, Size: size}
		}
	}
	return out, nil
}

// VerifyPack fetches pack id whole from the bucket and verifies every value
// it holds: it parses the pack's index from the fetched bytes, not from the
// local index or a cached tail, and hashes each value against the block key
// that index names. It reads with one range request of the recorded size,
// never through the read cache or the pack readers, so no local copy can
// answer for the bucket. An object larger than the recorded size plus
// verifySlackBytes, or a recorded size past the largest pack this store
// writes plus the slack, is refused before its body is read, so one call
// holds at most that many bytes. Returns every value of the pack, or a
// PackVerifyError when anything fails: a pack is good whole or not at all.
func (s *PackStore) VerifyPack(ctx context.Context, id string, size int64) ([]VerifiedValue, error) {
	// Refuse a recorded size no pack of this store reaches.
	limit := max(s.maxPackBytes, writer.DefaultMaxPackBytes) + verifySlackBytes
	if size <= 0 || size > limit {
		return nil, verifyFailed(VerifyOversize, errors.Errorf("recorded size %d", size))
	}
	key := s.prefix + packDir + id

	// Refuse an object past the recorded size before reading it, when the
	// bucket reports sizes.
	if statter, ok := s.client.(objectStatter); ok {
		stat, err := statter.HeadObject(ctx, s.bucket, key)
		switch {
		case errors.Is(err, ErrNotFound):
			return nil, verifyFailed(VerifyMissing, err)
		case err != nil:
			return nil, verifyFailed(VerifyFetch, err)
		case stat.Size > size+verifySlackBytes:
			return nil, verifyFailed(VerifyOversize, errors.Errorf("object size %d, recorded %d", stat.Size, size))
		}
	}

	// Fetch the recorded size from the bucket itself.
	data, err := s.client.GetObjectRange(ctx, s.bucket, key, 0, int(size))
	s.reads.downloaded(readWhole, len(data))
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, verifyFailed(VerifyMissing, err)
	case err != nil:
		return nil, verifyFailed(VerifyFetch, err)
	case int64(len(data)) != size:
		return nil, verifyFailed(VerifyShort, errors.Errorf("read %d of %d bytes", len(data), size))
	}

	// Match the SHA-256 recorded at upload, when the pack has one.
	var want []byte
	if s.index != nil {
		want, err = s.index.sum(ctx, id)
		if err != nil {
			return nil, verifyFailed("sum", err)
		}
	}
	if got := sha256.Sum256(data); want != nil && !bytes.Equal(got[:], want) {
		return nil, verifyFailed(VerifyHash, errors.New("pack bytes differ from the SHA-256 recorded at upload"))
	}

	// Parse the index from the fetched bytes.
	index, err := packfile.ReadIndex(bytes.NewReader(data), uint64(size))
	if err != nil {
		return nil, verifyFailed(VerifyIndex, err)
	}
	if len(index) == 0 {
		return nil, verifyFailed(VerifyIndex, errors.New("pack index lists no values"))
	}

	// Hash every value against its key.
	values := make([]VerifiedValue, 0, len(index))
	for _, ie := range index {
		off, n := ie.GetOffset(), ie.GetSize()
		if off > uint64(size) || n > uint64(size)-off {
			return nil, verifyFailed(VerifyIndex, errors.Errorf("value at %d+%d past %d bytes", off, n, size))
		}
		h, err := packfile.ParseBlockKey(ie.GetKey())
		if err != nil {
			return nil, verifyFailed(VerifyIndex, err)
		}
		stored, err := verifiedBlock(block.NewBlockRef(h), data[off:off+n])
		if err != nil {
			return nil, verifyFailed(VerifyHash, err)
		}
		values = append(values, VerifiedValue{Hash: h, Size: int64(len(stored.Data))})
	}
	return values, nil
}

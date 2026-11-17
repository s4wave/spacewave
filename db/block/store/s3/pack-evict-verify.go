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
	// VerifyEntry is the reason when the bucket's entry for the pack is
	// missing, does not read or decode, names another id or size, or has a
	// bloom filter that lacks a value the pack holds.
	VerifyEntry = "entry"
	// VerifyUnregistered is the reason when the pack is not in the plain
	// registered state: the local index lacks its entry, a compaction
	// superseded it, a reclaim trashed it, or a pack sweep holds it out of
	// the reads. It heals when the pack returns to the reads.
	VerifyUnregistered = "unregistered"
)

// The delete paths EvictHalt names.
const (
	// EvictHaltCompaction names compaction and reclaim, the delete paths of
	// a store that does not keep its packs.
	EvictHaltCompaction = "pack compaction and reclaim"
	// EvictHaltSweep names the pack sweep while it holds any pack.
	EvictHaltSweep = "pack sweep"
)

// verifySlackBytes is how far past its recorded size a pack's object may be
// before VerifyPack refuses it without reading.
const verifySlackBytes = 64 << 10

// verifyFailed returns a PackVerifyError.
func verifyFailed(reason string, err error) error {
	return &PackVerifyError{Reason: reason, Err: err}
}

// EvictHalt names a path by which the store itself could remove a pack an
// eviction pass verified, or returns "": compaction and reclaim on a store
// that does not keep its packs, and the pack sweep while it holds any pack.
// A pass drops nothing while it names one.
func (s *PackStore) EvictHalt() string {
	if !s.keepPacks {
		return EvictHaltCompaction
	}
	return s.sweepHalt()
}

// evictCounts reports whether pack entry can prove a block safe to drop.
//
// Invariant: only a pack in the plain registered state counts for eviction,
// as Registered reports it, with an entry no compaction superseded and no
// reclaim trashed. A pack out of that state is on its way out of the bucket,
// and a drop it proved would leave the block in no pack that stays.
// EvictLocate and VerifyPack both read this one rule. A pack held after the
// check is the sweep's to keep: once the local copy is gone, a read of the
// block reaches the held pack, which lifts it or records the miss that keeps
// it from removal.
func (s *PackStore) evictCounts(entry *packfile.PackfileEntry) bool {
	return entry.GetTrashedAt() == nil && entry.GetSupersededBy() == "" && s.Registered(entry.GetId())
}

// checkRegistered refuses pack id unless the local index holds its entry and
// the entry counts for eviction.
func (s *PackStore) checkRegistered(ctx context.Context, id string) error {
	// Without an index no pack counts.
	if s.index == nil {
		return verifyFailed(VerifyUnregistered, errors.New("pack store has no local index"))
	}

	// Read the pack's entry once the table loads, and apply the rule.
	if err := s.loadEntries(ctx); err != nil {
		return err
	}
	entries, err := s.index.readEntries(ctx, []string{id})
	if err != nil {
		return err
	}
	if len(entries) == 0 || !s.evictCounts(entries[0]) {
		return verifyFailed(VerifyUnregistered, errors.Errorf("pack %s is not registered", id))
	}
	return nil
}

// EvictLocate names the pack the local index places each block in, reading
// only the local index, or none for a block whose pack does not count for
// eviction. Eviction groups blocks by these packs and verifies each one with
// VerifyPack; the location itself proves nothing.
func (s *PackStore) EvictLocate(ctx context.Context, hashes []*hash.Hash) ([]PackLocation, error) {
	// Locate only through the local index, once its table loads.
	if s.index == nil {
		return nil, errors.New("pack store has no local index to locate packs")
	}
	if err := s.loadEntries(ctx); err != nil {
		return nil, err
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
		if !s.evictCounts(entry) {
			continue
		}
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
// that index names. It also reads the pack's entry from the bucket and checks
// that the entry names the pack at the recorded size and that its bloom
// filter holds every value.
//
// The pack must count for eviction, as evictCounts describes, both before
// the fetch and after it, so a pack the sweep held while it was read proves
// nothing.
//
// Invariant: eviction drops a block's last local copy only when a reader with
// no local index can find that block from the bucket alone. Such a reader
// finds packs by listing entries/ and testing each entry's bloom filter, so a
// pack whose bytes are sound but whose entry is missing or does not name a
// value hides that value from it. Any doubt about the entry refuses the pack,
// and a refused pack keeps every block.
//
// Cost per verified pack: two point reads of the local index, one GET of the
// entry object, a few KiB that grow with the pack's block count, then one
// HEAD where the backend reports sizes and one range GET of at most the
// recorded size plus verifySlackBytes and one byte, never through the read
// cache or the pack readers, so no local copy can answer for the bucket. An
// object larger than the recorded size plus verifySlackBytes is refused,
// before its body is read when the backend reports sizes, and a recorded size
// past the largest pack this store writes plus the slack is refused without a
// read, so one call holds at most the largest pack plus twice the slack and a
// byte: about 63.1 MiB. Returns every value of the pack, or a PackVerifyError when
// anything fails: a pack is good whole or not at all.
func (s *PackStore) VerifyPack(ctx context.Context, id string, size int64) ([]VerifiedValue, error) {
	// Refuse a recorded size no pack of this store reaches.
	limit := max(s.maxPackBytes, writer.DefaultMaxPackBytes) + verifySlackBytes
	if size <= 0 || size > limit {
		return nil, verifyFailed(VerifyOversize, errors.Errorf("recorded size %d", size))
	}
	if err := s.checkRegistered(ctx, id); err != nil {
		return nil, err
	}
	key := s.prefix + packDir + id

	// Read the pack's entry from the bucket first: it is small, and a pack a
	// listing reader cannot find is refused before its body is fetched.
	entry, err := s.readEntry(ctx, id)
	if err != nil {
		return nil, verifyFailed(VerifyEntry, err)
	}
	if entry.GetSizeBytes() != uint64(size) {
		return nil, verifyFailed(VerifyEntry, errors.Errorf("bucket entry names %d bytes, recorded %d", entry.GetSizeBytes(), size))
	}

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

	// Fetch the recorded size from the bucket itself, and as far as the
	// slack and one byte past it, so a backend that reports no sizes, such
	// as SFTP or FTPS, still shows an object past the recorded size plus the
	// slack, which is refused.
	data, err := s.client.GetObjectRange(ctx, s.bucket, key, 0, int(size+verifySlackBytes+1))
	s.reads.downloaded(readWhole, len(data))
	switch {
	case errors.Is(err, ErrNotFound):
		return nil, verifyFailed(VerifyMissing, err)
	case err != nil:
		return nil, verifyFailed(VerifyFetch, err)
	case int64(len(data)) > size+verifySlackBytes:
		return nil, verifyFailed(VerifyOversize, errors.Errorf("object past %d bytes, recorded %d", size+verifySlackBytes, size))
	case int64(len(data)) < size:
		return nil, verifyFailed(VerifyShort, errors.Errorf("read %d of %d bytes", len(data), size))
	}
	data = data[:size]

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
	keys := make([][]byte, 0, len(index))
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
		keys = append(keys, ie.GetKey())
	}

	// Every value must be one a listing reader finds through the entry.
	if err := checkEntryBloom(entry, keys); err != nil {
		return nil, verifyFailed(VerifyEntry, err)
	}

	// The pack must still count once its bytes are read.
	if err := s.checkRegistered(ctx, id); err != nil {
		return nil, err
	}
	return values, nil
}

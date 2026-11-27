//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"cmp"
	"crypto/sha256"
	"math"
	"path"
	"slices"
	"sort"
	"strings"
	"sync/atomic"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/s4wave/spacewave/net/hash"
)

// coldPack describes an immutable pack and its place in the ordered manifest.
type coldPack struct {
	// meta supplies the trusted bounds, size and index digest.
	meta *packfile.ColdPackEntry
	// key is the full pack object key.
	key string
	// filter rejects nonmembers before an index lookup.
	filter *bloom.Filter
	// gone permanently fences membership after an authoritative bucket miss.
	gone atomic.Bool
	// wrongObject fences a length or full-object checksum mismatch.
	wrongObject atomic.Bool
	// verifiedBytes records a full-object digest match for the immutable pack.
	verifiedBytes atomic.Bool
	// badPayload fences cached positive answers after disproving a known extent.
	badPayload atomic.Bool
}

// unusable reports whether observed bucket evidence rules out this pack.
func (p *coldPack) unusable() bool { return p.gone.Load() || p.wrongObject.Load() }

// coldInterval records a pack and the greatest last key at or before its
// position.
type coldInterval struct {
	// pack indexes the manifest, independent of interval ordering.
	pack uint16
	// maxLast is the inclusive upper bound for all earlier intervals and this one.
	maxLast []byte
}

// coldPackCatalog holds immutable metadata. The manifest never enters a
// PackStore catalog or its compaction, tail merge, reclaim or sweep machinery.
type coldPackCatalog struct {
	// packs numbers packs in manifest order for the promoted log.
	packs []coldPack
	// intervals orders the manifest's pack numbers by first key.
	intervals []coldInterval
	// promotionScope binds the log to the loaded manifest and bucket namespace.
	promotionScope [sha256.Size]byte
}

// newColdPackCatalog validates the manifest and prepares its interval search.
func newColdPackCatalog(prefix string, manifest *packfile.ColdPackManifest) (*coldPackCatalog, error) {
	// Check the manifest fits the log's pack-number field.
	entries := manifest.GetEntries()
	if len(entries) == 0 || len(entries) > 1<<16 {
		return nil, errors.New("cold manifest must hold between 1 and 65536 packs")
	}
	c := &coldPackCatalog{packs: make([]coldPack, len(entries)), intervals: make([]coldInterval, len(entries))}

	// Check every pack before publishing the manifest as a whole.
	for i, meta := range entries {
		// Accept only canonical relative directories and valid pack IDs.
		entry, subpath := meta.GetEntry(), meta.GetSubpath()
		if !coldPackSubpath(subpath) {
			return nil, errors.Errorf("invalid cold subpath %q", subpath)
		}
		if err := identity.ValidatePackID(entry.GetId()); err != nil {
			return nil, err
		}
		if i > 0 && compareColdPacks(entries[i-1], meta) >= 0 {
			return nil, errors.New("cold manifest packs must be ordered by subpath and ID, without duplicates")
		}

		// Require canonical SHA-256 keys and an index digest bound to a nonempty tail.
		if !coldPackKey(meta.GetFirstKey()) || !coldPackKey(meta.GetLastKey()) || bytes.Compare(meta.FirstKey, meta.LastKey) > 0 {
			return nil, errors.Errorf("invalid cold key bounds for %q", entry.Id)
		}
		if entry.BlockCount == 0 || entry.SizeBytes > math.MaxInt64 || meta.IndexTailBytes == 0 ||
			meta.IndexTailBytes >= entry.SizeBytes || len(meta.IndexSha256) != sha256.Size || len(meta.PackSha256) != sha256.Size {
			return nil, errors.Errorf("invalid cold index metadata for %q", entry.Id)
		}

		// Decode the writer's filter once and index the pack by its stable number.
		filter, err := coldEntryBloom(entry)
		if err != nil {
			return nil, err
		}
		c.packs[i] = coldPack{meta: meta, key: prefix + subpath + packDir + entry.Id, filter: filter}
		c.intervals[i].pack = uint16(i) //nolint:gosec // The manifest is bounded above.
	}

	// Prepare a prefix maximum so an overlap cannot hide a wider earlier interval.
	slices.SortStableFunc(c.intervals, func(a, b coldInterval) int {
		return bytes.Compare(c.packs[a.pack].meta.FirstKey, c.packs[b.pack].meta.FirstKey)
	})
	var last []byte
	for i := range c.intervals {
		// Record the upper bound that permits the reverse scan to stop.
		interval := &c.intervals[i]
		if candidate := c.packs[interval.pack].meta.LastKey; bytes.Compare(candidate, last) > 0 {
			last = candidate
		}
		interval.maxLast = last
	}
	return c, nil
}

// candidates yields every pack whose inclusive interval contains key, without
// inspecting bloom filters outside those intervals.
func (c *coldPackCatalog) candidates(key []byte, yield func(uint16) bool) {
	// Binary search for the inclusive upper edge, then prune by prefix maxima.
	upper := sort.Search(len(c.intervals), func(i int) bool {
		return bytes.Compare(c.packs[c.intervals[i].pack].meta.FirstKey, key) > 0
	})
	for _, interval := range slices.Backward(c.intervals[:upper]) {
		// Stop only when no earlier interval can contain the key.
		if bytes.Compare(interval.maxLast, key) < 0 {
			return
		}
		if bytes.Compare(c.packs[interval.pack].meta.LastKey, key) >= 0 && !yield(interval.pack) {
			return
		}
	}
}

// compareColdPacks orders pack numbers independently of their intervals.
func compareColdPacks(a, b *packfile.ColdPackEntry) int {
	// Sort by directory first, then by the immutable pack ID.
	if n := cmp.Compare(a.GetSubpath(), b.GetSubpath()); n != 0 {
		return n
	}
	return cmp.Compare(a.GetEntry().GetId(), b.GetEntry().GetId())
}

// coldPackSubpath accepts a canonical relative object directory or the cold prefix itself.
func coldPackSubpath(subpath string) bool {
	// Do not allow a manifest to escape the configured cold prefix.
	dir := strings.TrimSuffix(subpath, "/")
	return subpath == "" || subpath == dir+"/" && dir == path.Clean(dir) && dir != "." && dir != ".." &&
		!strings.HasPrefix(dir, "../") && !strings.ContainsAny(dir, "\\\x00") && !path.IsAbs(dir)
}

// coldPackKey reports whether key is the writer's canonical SHA-256 key encoding.
func coldPackKey(key []byte) bool {
	// Reject other hash algorithms and alternate protobuf encodings.
	h, err := packfile.ParseBlockKey(key)
	return err == nil && h.GetHashType() == hash.RecommendedHashType && bytes.Equal(key, packfile.BlockKey(h))
}

// coldEntryBloom parses a validated v1 pack bloom filter. The S3 pack writer
// uses writer.DefaultPolicy().NewBloomFilter (pack.go). The default 0.008
// target in writer/policy.go gives 10.05 bits/key and 7 probes at 26,316 keys
// (about 0.80% per-pack false positives, or about 0.024 candidate tails for
// an absent key covered by three independent intervals).
func coldEntryBloom(entry *packfile.PackfileEntry) (*bloom.Filter, error) {
	// Reject unsupported or malformed filters rather than trusting their negatives.
	var encoded bloom.BloomFilter
	if entry.GetBloomFormatVersion() != packfile.BloomFormatVersionV1 {
		return nil, errors.Errorf("cold pack %q needs a v1 bloom filter", entry.GetId())
	}
	if err := encoded.UnmarshalVT(entry.GetBloomFilter()); err != nil {
		return nil, errors.Wrapf(err, "decode bloom for cold pack %q", entry.Id)
	}
	if encoded.IsEmpty() || encoded.K > 64 || uint64(encoded.M) > uint64(len(encoded.GetBitSet().GetSet()))*64 {
		return nil, errors.Errorf("invalid bloom for cold pack %q", entry.Id)
	}
	return encoded.ToBloomFilter(), nil
}

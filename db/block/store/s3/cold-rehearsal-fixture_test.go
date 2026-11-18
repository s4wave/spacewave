//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"math/rand/v2"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
)

const (
	// rehearsalKeys is the motivating export's full key count.
	rehearsalKeys = 16975008
	// rehearsalPart is the count in a full part of the sorted export.
	rehearsalPart = 500000
	// rehearsalPackCount is the number of packs per part, including the final part.
	rehearsalPackCount = 19
	// rehearsalAbsent is the number of independently sampled missing digests.
	rehearsalAbsent = 100000
	// rehearsalPresent is the number of sampled refs distributed across the export.
	rehearsalPresent = 1000
	// rehearsalValue is the logical encoded block size, near the original mean.
	rehearsalValue = 4900
	// rehearsalData accounts for the one-byte protobuf tag and two-byte length.
	rehearsalData = rehearsalValue - 3
)

// rehearsalKey orders deterministic payloads by the first 64 bits of SHA-256.
// A prefix tie is rejected before packing, not treated as a duplicate key.
type rehearsalKey struct {
	// prefix orders the key within the complete key space.
	prefix uint64
	// seed regenerates the data whose hash defines the key.
	seed uint32
}

// rehearsalSample names a random present ref and its bucket object.
type rehearsalSample struct {
	// ref is the true SHA-256 of the written payload.
	ref *block.BlockRef
	// object is its complete bucket key.
	object string
	// seed regenerates the expected payload.
	seed uint32
}

// rehearsalFixture holds exact writer-generated tails, reconstructible body
// bytes and deterministic refs. It holds no full object copies on disk.
type rehearsalFixture struct {
	// bucket serves the synthetic range objects.
	bucket *rehearsalBucket
	// manifest carries the actual bloom and index tail produced by each writer.
	manifest *packfile.ColdPackManifest
	// catalog is the parsed manifest for the worker-free cache controls.
	catalog *coldPackCatalog
	// absent names 100,000 keys from an independent sample of the SHA-256 space.
	absent []*block.BlockRef
	// present names sample keys whose data were supplied to the pack writer.
	present []rehearsalSample
	// ordinals joins bucket object names to manifest positions.
	ordinals map[string]uint16
	// logicalBytes counts all bytes actually emitted by the writer.
	logicalBytes uint64
	// tailBytes counts raw exact index bytes kept by the fake bucket.
	tailBytes int
	// buildTime is the time to generate, sort and pack the synthetic fixture.
	buildTime time.Duration
}

// loadRehearsalFixture shares one immutable fixture between the test and benchmarks.
var loadRehearsalFixture = sync.OnceValues(buildRehearsalFixture)

// buildRehearsalFixture preserves 34 disjoint parts, 19 packs per part and at
// most three overlapping intervals per window. A contiguous window of three
// packs is distributed round-robin so the triple-overlap case is exercised.
func buildRehearsalFixture() (*rehearsalFixture, error) {
	// Create real content hashes and sort only their fixed-width permutation.
	start := time.Now()
	order := make([]rehearsalKey, rehearsalKeys)
	var data [rehearsalData]byte
	for i := range order {
		seed := uint32(i) //nolint:gosec // The fixture has fewer than 2^32 keys.
		digest := rehearsalDigest(data[:], seed)
		order[i] = rehearsalKey{prefix: binary.BigEndian.Uint64(digest[:]), seed: seed}
	}
	slices.SortFunc(order, func(a, b rehearsalKey) int { return cmp.Compare(a.prefix, b.prefix) })
	for i := 1; i < len(order); i++ {
		if order[i].prefix == order[i-1].prefix {
			return nil, errors.New("fixture needs a deterministic SHA-256 prefix tie breaker")
		}
	}

	// Draw uniform, independently seeded present and absent samples.
	rng := rand.New(rand.NewPCG(0xf1f2f3, 0xb1b2b3)) //nolint:gosec // Reproducible fixture, not cryptography.
	chosen := make(map[uint32]struct{}, rehearsalPresent)
	for len(chosen) < rehearsalPresent {
		chosen[order[rng.IntN(len(order))].seed] = struct{}{}
	}
	f := &rehearsalFixture{
		bucket: &rehearsalBucket{objects: make(map[string]*rehearsalObject)}, manifest: &packfile.ColdPackManifest{},
		absent: make([]*block.BlockRef, 0, rehearsalAbsent), present: make([]rehearsalSample, 0, rehearsalPresent),
	}
	for len(f.absent) < rehearsalAbsent {
		var digest [32]byte
		for i := range 4 {
			binary.BigEndian.PutUint64(digest[8*i:], rng.Uint64())
		}
		prefix := binary.BigEndian.Uint64(digest[:])
		if _, exists := slices.BinarySearchFunc(order, prefix, func(a rehearsalKey, b uint64) int { return cmp.Compare(a.prefix, b) }); !exists {
			f.absent = append(f.absent, block.NewBlockRef(hash.NewHash(hash.RecommendedHashType, digest[:])))
		}
	}

	// Recreate the pack windows within each disjoint, sorted export part.
	for part, start := 0, 0; start < len(order); part, start = part+1, start+rehearsalPart {
		end := min(len(order), start+rehearsalPart)
		if err := f.writePart(order[start:end], part, chosen); err != nil {
			return nil, err
		}
	}

	// Parse the actual catalog a cold mount will search and recover ordinals.
	slices.SortFunc(f.manifest.Entries, compareColdPacks)
	catalog, err := newColdPackCatalog("cold/", f.manifest)
	if err != nil {
		return nil, err
	}
	f.catalog = catalog
	f.ordinals = make(map[string]uint16, len(catalog.packs))
	for i := range catalog.packs {
		f.ordinals[catalog.packs[i].key] = uint16(i) //nolint:gosec // The manifest checked the ordinal range.
	}
	rng.Shuffle(len(f.present), func(i, j int) { f.present[i], f.present[j] = f.present[j], f.present[i] })
	f.buildTime = time.Since(start)
	return f, nil
}

// writePart packs consecutive three-pack windows, followed by the one pack left over.
func (f *rehearsalFixture) writePart(input []rehearsalKey, part int, chosen map[uint32]struct{}) error {
	// Compute balanced counts around 26,316 keys, except for the smaller final part.
	subpath := "X" + strconv.Itoa(part + 100)[1:] + "/"
	for first := 0; first < rehearsalPackCount; first += 3 {
		// Distribute canonical key ranks across one window, not across the whole part.
		width := min(3, rehearsalPackCount-first)
		lo := first * len(input) / rehearsalPackCount
		hi := (first + width) * len(input) / rehearsalPackCount
		for lane := range width {
			if err := f.writePack(input[lo:hi], lane, width, subpath, chosen); err != nil {
				return err
			}
		}
	}
	return nil
}

// writePack passes every payload through the real pack writer and retains its raw tail.
func (f *rehearsalFixture) writePack(input []rehearsalKey, lane, width int, subpath string, chosen map[uint32]struct{}) error {
	// The writer concatenates fixed-size values before emitting the index.
	count := (len(input) - lane + width - 1) / width
	object := &rehearsalObject{seeds: make([]uint32, 0, count), base: int64(count * rehearsalValue)}
	sink := &rehearsalTailWriter{base: object.base}
	sink.tail.Grow(count * 54)
	var samples []rehearsalSample
	var data [rehearsalData]byte
	i := lane

	// Hash, encode and write every synthetic block with normal production policy.
	result, err := writer.PackBlocks(sink, func() (*hash.Hash, *block.StoredBlock, error) {
		// Supply canonical keys and retain the seeds needed to reconstruct byte ranges.
		if i >= len(input) {
			return nil, nil, nil
		}
		record := input[i]
		i += width
		digest := rehearsalDigest(data[:], record.seed)
		h := hash.NewHash(hash.RecommendedHashType, digest[:])
		object.seeds = append(object.seeds, record.seed)
		if _, ok := chosen[record.seed]; ok {
			samples = append(samples, rehearsalSample{ref: block.NewBlockRef(h), seed: record.seed})
		}
		return h, &block.StoredBlock{Data: data[:], RefsKnown: true}, nil
	})
	if err != nil {
		return err
	}

	// Authenticate the exact tail, checking offsets against the fake bucket's recipe.
	object.tail = sink.tail.Bytes()
	entry, err := object.entry(result, count, subpath)
	if err != nil {
		return err
	}
	key := "cold/" + subpath + packDir + entry.Entry.Id
	f.bucket.objects[key] = object
	f.manifest.Entries = append(f.manifest.Entries, entry)
	f.logicalBytes += result.BytesWritten
	f.tailBytes += len(object.tail)
	for _, sample := range samples {
		sample.object = key
		f.present = append(f.present, sample)
	}
	return nil
}

// rehearsalDigest writes the seed into a fixed-size payload and hashes the full value.
func rehearsalDigest(data []byte, seed uint32) [sha256.Size]byte {
	// The checksum belongs to the body, not its encoded storage wrapper.
	binary.BigEndian.PutUint32(data[len(data)-4:], seed)
	return sha256.Sum256(data)
}

// rehearsalTailWriter keeps only the original index bytes after the known payload boundary.
type rehearsalTailWriter struct {
	// base is the first byte of the raw kvfile index.
	base int64
	// pos is the writer's current logical offset.
	pos int64
	// tail holds bytes written at or beyond the index boundary.
	tail bytes.Buffer
}

// Write counts the full writer stream and retains just its authenticated index.
func (w *rehearsalTailWriter) Write(data []byte) (int, error) {
	// Discard the reconstructible payload portion of this write.
	start := max(int64(0), w.base-w.pos)
	w.pos += int64(len(data))
	if start < int64(len(data)) {
		w.tail.Write(data[start:])
	}
	return len(data), nil
}

// rehearsalObject serves the exact writer bytes without writing 83 GB of payloads to disk.
type rehearsalObject struct {
	// seeds are the payloads in their physical writer order.
	seeds []uint32
	// base is the byte length of the payload region.
	base int64
	// tail is the original writer's exact raw index tail.
	tail []byte
}

// entry builds the manifest entry from the raw index and writer result.
func (o *rehearsalObject) entry(result *writer.PackResult, count int, subpath string) (*packfile.ColdPackEntry, error) {
	// Confirm the index starts where the payload recipe says it does.
	if int64(result.BytesWritten) != o.base+int64(len(o.tail)) || result.BlockCount != uint64(count) {
		return nil, errors.New("fixture writer size disagrees with the virtual bucket")
	}
	r, err := kvfile.BuildReaderWithIndexTail(o.tail, result.BytesWritten)
	if err != nil {
		return nil, err
	}
	if r.Size() != result.BlockCount {
		return nil, errors.New("fixture writer tail count mismatch")
	}
	first, err := r.ReadIndexEntry(0)
	if err != nil {
		return nil, err
	}
	last, err := r.ReadIndexEntry(result.BlockCount - 1)
	if err != nil {
		return nil, err
	}
	if first.Offset != 0 || last.Offset+last.Size != uint64(o.base) || last.Size != rehearsalValue {
		return nil, errors.New("fixture writer layout disagrees with reconstructible values")
	}

	// Record the real writer's bloom and stable content-derived pack identity.
	id, err := identity.BuildPackID("rehearsal", result)
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(o.tail)
	return &packfile.ColdPackEntry{
		Subpath: subpath, FirstKey: first.Key, LastKey: last.Key, IndexSha256: digest[:], IndexTailBytes: uint64(len(o.tail)),
		Entry: &packfile.PackfileEntry{Id: id, BlockCount: result.BlockCount, SizeBytes: result.BytesWritten,
			BloomFormatVersion: packfile.BloomFormatVersionV1, BloomFilter: result.BloomFilter},
	}, nil
}

// read returns precisely the requested bytes from reconstructed payloads and the original tail.
func (o *rehearsalObject) read(off int64, size int) ([]byte, error) {
	// Check the exact requested extent before allocating a response.
	end := off + int64(size)
	if off < 0 || size < 0 || end > o.base+int64(len(o.tail)) {
		return nil, errors.New("invalid rehearsal bucket range")
	}
	out := make([]byte, size)

	// Re-encode only the fixed-size block values touched by this range.
	var data [rehearsalData]byte
	for pos := off; pos < min(end, o.base); {
		base := pos / rehearsalValue * rehearsalValue
		binary.BigEndian.PutUint32(data[len(data)-4:], o.seeds[pos/rehearsalValue])
		value, err := block.EncodeBlockObject(data[:], nil, true)
		if err != nil || len(value) != rehearsalValue {
			return nil, errors.New("invalid reconstructed block value")
		}
		n := copy(out[pos-off:], value[pos-base:])
		pos += int64(n)
	}

	// Append the untouched index bytes the pack writer produced.
	if end > o.base {
		pos := max(off, o.base)
		copy(out[pos-off:], o.tail[pos-o.base:])
	}
	return out, nil
}

// rehearsalBucket is an in-process fake bucket with exact range accounting.
// Fixture backing objects are immutable; one mount-local delete flag simulates 404.
type rehearsalBucket struct {
	// objects maps complete bucket keys to writer objects.
	objects map[string]*rehearsalObject
	// removed is the optional test object that was deleted.
	removed atomic.Pointer[string]
	// requests counts requests, including a 404.
	requests atomic.Uint64
	// bytes counts bytes successfully returned.
	bytes atomic.Uint64
}

// GetObjectRange serves an exact range without network or credentials.
func (b *rehearsalBucket) GetObjectRange(ctx context.Context, _, key string, off int64, size int) ([]byte, error) {
	// Reject a canceled caller or the deleted object before reading bytes.
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	b.requests.Add(1)
	o := b.objects[key]
	if removed := b.removed.Load(); o == nil || removed != nil && *removed == key {
		return nil, ErrNotFound
	}

	// Reconstruct the wire response and count only bytes the range returned.
	data, err := o.read(off, size)
	b.bytes.Add(uint64(len(data)))
	return data, err
}

// _ is a type assertion.
var _ ColdPackRangeReader = (*rehearsalBucket)(nil)

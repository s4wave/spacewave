//go:build !tinygo

package block_store_s3

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/binary"
	stderrors "errors"
	"hash/crc32"
	"io"
	"maps"
	"math"
	"os"
	"slices"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/coord/filelock"
)

const (
	// coldPromotionMagic names the manifest-bound, full-key format.
	coldPromotionMagic = "SWCPLOG2"
	// coldPromotionHeaderBytes holds the magic and SHA-256 of the mount scope.
	coldPromotionHeaderBytes = len(coldPromotionMagic) + sha256.Size
	// coldPromotionBytes holds the full digest, compact extent and scoped CRC.
	coldPromotionBytes = 48
	// coldPromoteBatch bounds the number of records written in one sync step.
	coldPromoteBatch = 64
)

// coldExtent is one encoded block value in the manifest.
type coldExtent struct {
	// pack is its pack's ordinal in the manifest.
	pack uint16
	// off is the byte offset of the encoded block object.
	off uint64
	// size is the length of the encoded block object.
	size uint64
}

// flightKey names a complete range without using any shortened block identity.
func (e coldExtent) flightKey() string {
	// Format the range as delimited unsigned integers.
	return strconv.FormatUint(uint64(e.pack), 10) + ":" + strconv.FormatUint(e.off, 10) + ":" + strconv.FormatUint(e.size, 10)
}

// coldPromotion is the in-memory form of a record and its current proof.
type coldPromotion struct {
	// extent supplies the pack, offset and encoded length.
	extent coldExtent
	// dataSize is the decoded length, known only after body verification.
	dataSize uint32
	// digest names the full canonical wire key in the bound manifest.
	digest [sha256.Size]byte
	// verified means this mount has verified the body, not just recovered the key.
	verified bool
}

// coldPromotions owns a single append-only log and an in-memory read index.
// The promoter never holds its map lock across a write or fsync.
type coldPromotions struct {
	// store supplies the counters and error log.
	store *ColdPackStore
	// path names the dedicated, owned local log.
	path string
	// cap is the maximum number of physical records, not the map size.
	cap uint64
	// active admits offers only when a healthy writer owns the log.
	active atomic.Bool
	// promoteCh accepts bounded offers without blocking a read.
	promoteCh chan coldPromotion
	// mtx guards records. It never guards filesystem operations.
	mtx sync.RWMutex
	// records maps each full canonical key digest to its latest durable record.
	records map[[sha256.Size]byte]coldPromotion
	// recordCRC seeds record checksums with the header, set before publication.
	recordCRC uint32
}

// newColdPromotions creates the in-memory promotion index and queue.
func newColdPromotions(store *ColdPackStore, path string, cap int) *coldPromotions {
	return &coldPromotions{
		store: store, path: path, cap: uint64(cap), //nolint:gosec // The store constructor checked cap is non-negative.
		promoteCh: make(chan coldPromotion, 1024), records: make(map[[sha256.Size]byte]coldPromotion),
	}
}

// coldPromotionScope binds pack ordinals to the exact manifest and object names.
func coldPromotionScope(bucket, prefix string, manifest []byte) [sha256.Size]byte {
	// Frame both path components so no other bucket/prefix split can alias.
	h := sha256.New()
	var size [8]byte
	for _, part := range []string{bucket, prefix} {
		binary.BigEndian.PutUint64(size[:], uint64(len(part)))
		_, _ = h.Write(size[:])
		_, _ = h.Write([]byte(part))
	}
	_, _ = h.Write(manifest)
	return [sha256.Size]byte(h.Sum(nil))
}

// get copies the record for one exact canonical key without waiting.
func (p *coldPromotions) get(digest [sha256.Size]byte) (coldPromotion, bool) {
	// Do not wait behind a promoter publishing a durable batch.
	if !p.mtx.TryRLock() {
		return coldPromotion{}, false
	}
	defer p.mtx.RUnlock()
	record, found := p.records[digest]
	return record, found
}

// confirm publishes the verified body length for an existing record.
func (p *coldPromotions) confirm(digest [sha256.Size]byte, extent coldExtent, size uint32) {
	// Keep the newer record if the promoter replaced the hint mid-read.
	if !p.mtx.TryLock() {
		return
	}
	defer p.mtx.Unlock()
	if rec, ok := p.records[digest]; ok && rec.extent == extent {
		rec.dataSize = size
		rec.verified = true
		p.records[digest] = rec
	}
}

// offer submits only an authenticated canonical index hit with a verified body.
func (p *coldPromotions) offer(digest [sha256.Size]byte, extent coldExtent, size int64) {
	// Keep reading extents that cannot be represented in this log format.
	if !p.active.Load() || extent.off >= 1<<48 || extent.size > math.MaxUint32 || size < 0 || size > math.MaxUint32 {
		return
	}
	if old, ok := p.get(digest); ok && old.verified && old.extent == extent {
		return
	}

	// Above the disk cap the writer admits nothing, and accounts for the drop.
	if p.store.stats.promotedKeys.Load() >= p.cap {
		p.store.stats.promoteCapDropped.Add(1)
		return
	}
	rec := coldPromotion{extent: extent, dataSize: uint32(size), digest: digest, verified: true} //nolint:gosec // size passed the bounds check.
	select {
	case p.promoteCh <- rec:
	default:
		p.store.stats.promoteDropped.Add(1)
	}
}

// open validates and truncates the log before a catalog can expose the hints.
// Exclusive ownership and the byte cap apply even if two mounts are misconfigured
// to use the same file. Recovery creates no SQLite state or sidecar file.
func (p *coldPromotions) open(manifestFile string, catalog *coldPackCatalog) (*os.File, error) {
	// Promotion may be disabled; never open or create its path in that case.
	if p.path == "" || p.cap == 0 {
		return nil, nil
	}
	manifest, err := os.Stat(manifestFile)
	if err != nil {
		return nil, err
	}
	f, err := os.OpenFile(p.path, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o600)
	if err != nil {
		return nil, err
	}

	// Fence recovery and appends from other mounts of the same inode.
	if err := lockPromotionFile(f, manifest); err != nil {
		return nil, stderrors.Join(err, f.Close())
	}
	if err := p.recover(f, catalog); err != nil {
		clear(p.records)
		return nil, stderrors.Join(err, f.Close())
	}
	p.active.Store(true)
	return f, nil
}

// lockPromotionFile rejects invalid paths and takes ownership of the log inode.
func lockPromotionFile(f *os.File, manifest os.FileInfo) error {
	// Never let recovery truncate the manifest (including via link aliases).
	info, err := f.Stat()
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || os.SameFile(info, manifest) {
		return errors.New("promoted log must be a separate regular file")
	}
	locked, err := filelock.TryLockExclusive(f)
	if err == nil && !locked {
		return errors.New("promoted log already has a writer")
	}
	return err
}

// recover keeps only records in the current format and mount scope. A recovered
// hint cannot recover its wire identity from a body hash or a pack ordinal alone.
// A missing or different header discards the cache instead of migrating it.
func (p *coldPromotions) recover(f *os.File, catalog *coldPackCatalog) error {
	// Fence stale ordinals before decoding any record as a canonical key.
	reader := bufio.NewReader(f)
	var header, actual [coldPromotionHeaderBytes]byte
	copy(header[:], coldPromotionMagic)
	copy(header[len(coldPromotionMagic):], catalog.promotionScope[:])
	_, err := io.ReadFull(reader, actual[:])
	if err != nil && !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrUnexpectedEOF) {
		return err
	}
	p.recordCRC = crc32.ChecksumIEEE(header[:])
	if err != nil || actual != header {
		return replaceColdPromotionHeader(f, header)
	}

	// Read at most the configured bound, even with an oversized log.
	var raw [coldPromotionBytes]byte
	var kept uint64
	for kept < p.cap {
		// End the durable prefix on the first torn record.
		_, err := io.ReadFull(reader, raw[:])
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			break
		}
		if err != nil {
			return err
		}
		if crc32.Update(p.recordCRC, crc32.IEEETable, raw[:44]) != binary.BigEndian.Uint32(raw[44:]) {
			break
		}

		// Discard a record and its suffix if its extent cannot fit this pack.
		rec := decodeColdPromotion(raw)
		if !rec.extent.fits(catalog) {
			break
		}
		p.records[rec.digest] = rec
		kept++
	}

	// Truncate and fence the tail before appending, outside all request paths.
	info, err := f.Stat()
	if err != nil {
		return err
	}
	end := int64(uint64(coldPromotionHeaderBytes) + kept*coldPromotionBytes) // The constructor bounded the record count.
	if info.Size() != end {
		if err := f.Truncate(end); err != nil {
			return err
		}
		if err := f.Sync(); err != nil {
			return err
		}
	}
	p.store.stats.promotedKeys.Store(kept)
	return nil
}

// replaceColdPromotionHeader empties the dedicated log and commits its new scope.
func replaceColdPromotionHeader(f *os.File, header [coldPromotionHeaderBytes]byte) error {
	// Repair a missing, obsolete or torn header under the existing inode lock.
	if err := f.Truncate(0); err != nil {
		return err
	}
	if _, err := f.Write(header[:]); err != nil {
		return err
	}
	return f.Sync()
}

// fits reports whether the extent points entirely inside the pack's payloads.
func (e coldExtent) fits(catalog *coldPackCatalog) bool {
	// Use the immutable manifest's tail boundary, not offsets from the log.
	if int(e.pack) >= len(catalog.packs) || e.size == 0 || e.size > math.MaxInt {
		return false
	}
	meta := catalog.packs[e.pack].meta
	base := meta.Entry.SizeBytes - meta.IndexTailBytes
	return e.off < base && e.size <= base-e.off
}

// decodeColdPromotion decodes the fixed fields after recovery checks the CRC.
func decodeColdPromotion(raw [coldPromotionBytes]byte) coldPromotion {
	return coldPromotion{
		digest: [sha256.Size]byte(raw[:32]),
		extent: coldExtent{
			pack: binary.BigEndian.Uint16(raw[32:34]),
			off:  uint64(binary.BigEndian.Uint16(raw[34:36]))<<32 | uint64(binary.BigEndian.Uint32(raw[36:40])),
			size: uint64(binary.BigEndian.Uint32(raw[40:44])),
		},
	}
}

// encodeColdPromotion appends a fixed record with a scope-seeded checksum.
func encodeColdPromotion(dst []byte, rec coldPromotion, seed uint32) []byte {
	// Persist the whole canonical digest and the extent, not a decoded size.
	start := len(dst)
	dst = append(dst, rec.digest[:]...)
	dst = binary.BigEndian.AppendUint16(dst, rec.extent.pack)
	dst = binary.BigEndian.AppendUint16(dst, uint16(rec.extent.off>>32)) //nolint:gosec // offer bounds offsets to 48 bits.
	dst = binary.BigEndian.AppendUint32(dst, uint32(rec.extent.off))     //nolint:gosec // The low 32 bits of the 48-bit offset.
	dst = binary.BigEndian.AppendUint32(dst, uint32(rec.extent.size))    //nolint:gosec // offer bounds length to uint32.
	return binary.BigEndian.AppendUint32(dst, crc32.Update(seed, crc32.IEEETable, dst[start:]))
}

// run appends batches with one fsync each and yields between batches. Only the
// worker holds the log handle; reads and commits neither flush nor close it.
func (p *coldPromotions) run(ctx context.Context, f *os.File) {
	// Release the exclusive log on shutdown or any write failure.
	defer func() {
		p.active.Store(false)
		if err := f.Close(); err != nil {
			p.store.warn(ctx, err, "close promoted cold log")
		}
	}()
	for ctx.Err() == nil {
		// Wait only on the background worker's own queue.
		var first coldPromotion
		select {
		case <-ctx.Done():
			return
		case first = <-p.promoteCh:
		}

		// Append a short batch, stopping on a possible partial write. Recovery
		// will resume at the valid CRC prefix; a later batch cannot bury a tear.
		start := time.Now()
		if err := p.appendBatch(f, first); err != nil {
			p.store.warn(ctx, err, "append promoted cold log")
			return
		}
		if !coldPause(ctx, max(100*time.Millisecond, 2*time.Since(start))) {
			return
		}
	}
}

// appendBatch drains a bounded batch and publishes it only after fsync.
func (p *coldPromotions) appendBatch(f *os.File, first coldPromotion) error {
	// Filter duplicates and check physical capacity before writing any bytes.
	admitted := make(map[[sha256.Size]byte]coldPromotion, coldPromoteBatch)
	var buf [coldPromotionBytes * coldPromoteBatch]byte
	data := buf[:0]
	rec := first
	for i := range coldPromoteBatch {
		// Choose at most one extent per canonical key in this batch.
		key := rec.digest
		old, exists := p.get(key)
		if _, duplicate := admitted[key]; !duplicate && (!exists || !old.verified || old.extent != rec.extent) {
			if p.store.stats.promotedKeys.Load()+uint64(len(admitted)) < p.cap {
				admitted[key] = rec
				data = encodeColdPromotion(data, rec, p.recordCRC)
			} else {
				p.store.stats.promoteCapDropped.Add(1)
			}
		}

		// Do not consume an offer beyond the batch limit.
		if i+1 == coldPromoteBatch {
			break
		}
		select {
		case rec = <-p.promoteCh:
			continue
		default:
		}
		break
	}
	if len(admitted) == 0 {
		return nil
	}

	// One append and one fsync; nothing on the foreground path holds this file.
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}

	// Publish only records from the durable batch under a bounded map update.
	p.mtx.Lock()
	maps.Copy(p.records, admitted)
	p.mtx.Unlock()
	p.store.stats.promotedKeys.Add(uint64(len(admitted)))
	p.store.stats.lastNewKeyUnix.Store(time.Now().Unix())
	return nil
}

// warmPacks chooses unique recovered pack numbers before serving requests.
func (p *coldPromotions) warmPacks() []uint16 {
	// Preserve an order independent of map traversal and duplicate records.
	seen := make(map[uint16]struct{})
	for _, rec := range p.records {
		seen[rec.extent.pack] = struct{}{}
	}
	return slices.Sorted(maps.Keys(seen))
}

// coldPause yields a background slice without waiting past cancellation.
func coldPause(ctx context.Context, delay time.Duration) bool {
	// Use a cancelable timer rather than a sleep on the worker lifetime.
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

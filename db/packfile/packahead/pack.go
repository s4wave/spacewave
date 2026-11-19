package packahead

import (
	"bytes"
	"container/list"
	"crypto/sha256"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/go-kvfile"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block/bloom"
	"github.com/s4wave/spacewave/db/packfile/writer"
)

// openExt names a pack being written or left unfinalized.
const openExt = ".open"

// packExt names a finalized pack.
const packExt = ".pack"

// indexEntryReserve bounds the kvfile index bytes one entry costs beyond its
// key: the entry's tags, offset and size, its length varint and its position.
const indexEntryReserve = 32

// indexTrailerReserve bounds the kvfile index bytes beyond its entries.
const indexTrailerReserve = 64

// pack is one pack file. Its fields are guarded by the Log's bcast, except
// that the one writer holding writeMtx fills buf past written, and the
// maintainer reads buf, entries and written of a pack nothing writes.
type pack struct {
	// seq orders the pack among the Log's packs.
	seq uint64
	// path is the pack's current file name.
	path string
	// f is the pack file, open for the pack's life in the Log, so a rename
	// leaves every reader's handle valid.
	f *os.File
	// buf holds the pack's bytes [0, written) until it is finalized, and is
	// nil after: the open pack's hot cache.
	buf []byte
	// bufReaders pins buf against recycling while a reader copies its bytes.
	bufReaders atomic.Int32
	// written is the pack's length: every batch written to f.
	written int64
	// synced is the durable end of the batch data, excluding the index.
	// A finalized pack loaded at Open has no append waiters.
	synced int64
	// syncing is set while a writer runs the pack's fdatasync.
	syncing bool
	// err is the first failed write or sync. The pack takes no more batches,
	// and the maintainer recovers it from the file.
	err error
	// poisoned is set when a sync of the pack failed. A later sync may report
	// success for pages the failed one dropped, so the pack is never
	// finalized by this process: it stays unfinalized for the next Open to
	// recover from its batch checksums, and none of its values is durable to
	// a writer, which stores them again in a later pack.
	poisoned bool
	// entries are the values written, in write order, until finalized.
	entries []entry
	// keyBytes sums the keys of entries, for the index reserve.
	keyBytes int64
	// firstAt is when the pack's first batch was written.
	firstAt time.Time
	// state is the pack's place in its life.
	state packState
	// id names a finalized pack.
	id string
	// blocks is a finalized pack's value count.
	blocks int
	// bloom is a finalized pack's serialized bloom filter.
	bloom []byte
	// uploaded is set once the Log's remote holds the finalized pack.
	uploaded bool
	// removed tracks locations freed before upload. They remain readable
	// until upload and a put can revive them.
	removed map[keyTail]*list.Element
	// removals orders deferred locations for bounded invalidation batches.
	removals list.List
	// order links the pack in the rotated queue or the finalized list.
	order *list.Element
	// pending links this finalized pack in the upload queue, if pending.
	pending *list.Element
	// held links this pack among the packs holding unuploaded data.
	held *list.Element
	// evicting claims the pack for one unlink and table cleanup.
	evicting bool
	// renamed is what finalize made of the pack once its rename succeeded,
	// so a retry after a failed directory sync only syncs the directory. Only
	// the goroutine finalizing the pack uses it.
	renamed *finalized
	// dropped is set once dropEmpty closed the pack's file to remove it, so a
	// retry after a failed remove or directory sync does not recover it
	// again. Only the goroutine finalizing the pack uses it.
	dropped bool
}

// packState is a pack's place in its life.
type packState int

const (
	// packSpare is created and synced into the directory, awaiting the writer.
	packSpare packState = iota
	// packOpen takes the writer's batches.
	packOpen
	// packRotated takes no more batches and awaits finalizing.
	packRotated
	// packFinal is a finalized packfile.
	packFinal
	// packGone is an evicted pack: its file is unlinked and the table holds
	// none of its values.
	packGone
)

// openName returns the file name of pack seq while it is open.
func openName(seq uint64) string {
	return seqText(seq) + openExt
}

// finalName returns the file name of finalized pack seq named id.
func finalName(seq uint64, id string) string {
	return seqText(seq) + "-" + id + packExt
}

// seqText returns seq as 16 hex digits, so names sort in write order.
func seqText(seq uint64) string {
	s := strconv.FormatUint(seq, 16)
	return "0000000000000000"[len(s):] + s
}

// indexReserve returns the bytes the pack's index needs with n more entries
// whose keys sum to keyBytes more.
func (p *pack) indexReserve(n int, keyBytes int64) int64 {
	entries := int64(len(p.entries) + n)
	return p.keyBytes + keyBytes + entries*indexEntryReserve + indexTrailerReserve
}

// finalized is what finalize made of a pack.
type finalized struct {
	// id names the pack.
	id string
	// path is the pack's finalized file name.
	path string
	// size is the pack's length, its index included.
	size int64
	// dataEnd is the end of the batch data that the index follows.
	dataEnd int64
	// blocks is the pack's value count.
	blocks int
	// bloom is the pack's serialized bloom filter.
	bloom []byte
}

// finalize writes the sorted kvfile index after the pack's last batch, syncs
// it with opts.SyncData, and renames the pack to its finalized name in dir,
// syncing dir. data is the pack's bytes [0, written), from its buffer or its
// file. The pack must take no more batches.
func (p *pack) finalize(dir string, data []byte, opts Options) (*finalized, error) {
	// A pack an earlier try renamed needs only its directory sync.
	if p.renamed != nil {
		if err := dirSync(dir); err != nil {
			return nil, errors.Wrap(err, "sync pack directory")
		}
		return p.renamed, nil
	}

	// Index each key once, at its last copy: a block written again after a
	// free repeats its bytes, and a kvfile holds each key once.
	index := make([]*kvfile.IndexEntry, len(p.entries))
	for i, e := range p.entries {
		index[i] = &kvfile.IndexEntry{Key: e.key, Offset: uint64(e.off), Size: uint64(e.size)} //nolint:gosec // non-negative.
	}
	slices.SortStableFunc(index, func(a, b *kvfile.IndexEntry) int { return bytes.Compare(a.Key, b.Key) })
	kept := index[:0]
	for i, e := range index {
		if i+1 < len(index) && bytes.Equal(e.Key, index[i+1].Key) {
			continue
		}
		kept = append(kept, e)
	}
	index = kept

	// Write the index after the last batch and make the pack durable.
	var tail bytes.Buffer
	if _, err := kvfile.WriteIndex(&tail, index, uint64(len(data))); err != nil {
		return nil, errors.Wrap(err, "write pack index")
	}
	size := int64(len(data) + tail.Len())
	if size > opts.MaxPackBytes {
		return nil, errors.Errorf("pack %s of %d bytes exceeds %d", seqText(p.seq), size, opts.MaxPackBytes)
	}
	if _, err := p.f.WriteAt(tail.Bytes(), int64(len(data))); err != nil {
		return nil, errors.Wrap(err, "write pack index")
	}
	if err := opts.SyncData(p.f); err != nil {
		return nil, errors.Wrap(err, "sync pack index")
	}

	// Build the pack's bloom filter.
	policy := writer.DefaultPolicy()
	bf := policy.NewBloomFilter(uint64(len(index)))
	keys := make([][]byte, len(index))
	for i, e := range index {
		bf.Add(e.Key)
		keys[i] = e.Key
	}
	bloomBytes, err := bloom.NewBloom(bf).MarshalVT()
	if err != nil {
		return nil, err
	}

	// Name the pack from its digests.
	body := sha256.New()
	_, _ = body.Write(data)
	_, _ = body.Write(tail.Bytes())
	id, err := opts.PackID(&writer.PackResult{
		BloomFilter:      bloomBytes,
		BlockCount:       uint64(len(index)),
		BytesWritten:     uint64(size), //nolint:gosec // non-negative.
		SortedKeyDigest:  writer.DigestSortedKeys(keys),
		PackBytesDigest:  body.Sum(nil),
		PolicyTag:        writer.PolicyTag(policy),
		ValueOrderPolicy: writer.ValueOrderIterator,
	})
	if err != nil {
		return nil, errors.Wrap(err, "name pack")
	}

	// Rename the pack to its finalized name and make the rename durable. A
	// rename done is never tried again.
	path := filepath.Join(dir, finalName(p.seq, id))
	if err := os.Rename(p.path, path); err != nil {
		return nil, err
	}
	p.renamed = &finalized{id: id, path: path, size: size, dataEnd: int64(len(data)), blocks: len(index), bloom: bloomBytes}
	if err := dirSync(dir); err != nil {
		return nil, errors.Wrap(err, "sync pack directory")
	}
	return p.renamed, nil
}

// recoverFile reads an unfinalized pack, keeps the batches scan verifies at
// their original offsets, and truncates the tail after the last one. It syncs
// any repairs made by scan together with the truncation before returning.
func (p *pack) recoverFile(
	sync func(*os.File) error,
	scan func([]byte) ([]entry, int64, bool, error),
) ([]byte, []entry, error) {
	// Read the whole file: a pack is bounded by MaxPackBytes.
	info, err := p.f.Stat()
	if err != nil {
		return nil, nil, err
	}
	data := make([]byte, info.Size())
	if err := readAtFull(p.f, data, 0); err != nil {
		return nil, nil, errors.Wrap(err, "read pack")
	}

	// Keep the batches whose records and values both check, and drop the rest.
	entries, end, changed, err := scan(data)
	if err != nil {
		return nil, nil, err
	}
	if end < info.Size() {
		if err := p.f.Truncate(end); err != nil {
			return nil, nil, errors.Wrap(err, "truncate pack")
		}
		changed = true
	}

	// Reopening the old name must see the same verified boundaries.
	if changed {
		if err := sync(p.f); err != nil {
			return nil, nil, errors.Wrap(err, "sync recovered pack")
		}
	}
	return data[:end], entries, nil
}

// readAtFull fills buf from f at off. An io.ReaderAt may return io.EOF with a
// full read that ends at the end of the file, which the GoScript runtime's
// files do, so only a short read is an error.
func readAtFull(f io.ReaderAt, buf []byte, off int64) error {
	n, err := f.ReadAt(buf, off)
	if err == io.EOF && n == len(buf) {
		return nil
	}
	return err
}

// sealRecoveryGaps repairs only discarded batches before the last kept batch.
// It changes no kept record or value. The caller syncs the pack before making
// the recovered locations available.
func (p *pack) sealRecoveryGaps(data []byte, original, kept []entry, keptEnd int64) (bool, error) {
	// Walk only boundaries the writer recorded, including across adjacent gaps.
	var changed bool
	for len(original) != 0 {
		n, start, end := firstBatchExtent(original)
		original = original[n:]
		if end > keptEnd {
			break
		}
		if len(kept) != 0 && kept[0].batch == start {
			kept = kept[n:]
			continue
		}

		// Keep existing framing when startup can already skip the batch.
		// A previous attempt may have written its skip record before failing.
		if _, next, err := decodeBatch(data[:end], start); err != nil && next == end {
			changed = changed || err == errSkippedRecord
			continue
		}

		// Seal this gap without moving any later batch. The old bytes behind
		// the skip record remain opaque to startup and to the final index.
		rec := encodeSkipRecord(start, end)
		if _, err := p.f.WriteAt(rec[:], start); err != nil {
			return false, errors.Wrap(err, "write pack skip record")
		}
		copy(data[start:], rec[:])
		changed = true
	}
	return changed, nil
}

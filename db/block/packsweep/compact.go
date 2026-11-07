//go:build !tinygo

package packsweep

import (
	"bytes"
	"cmp"
	"context"
	"io"
	"slices"
	"time"

	"github.com/aperturerobotics/fastjson"
	"github.com/aperturerobotics/go-kvfile"
	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/kvtx"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
)

// Bounds of a new pack's bytes past its values, from the kvfile layout: an
// index entry is its key, two tagged varints, its size varint and its fixed
// position, and the index ends with the entry count.
const (
	// packEntryBytes is the most an index entry adds past its key.
	packEntryBytes = 33
	// packTrailerBytes is the index's fixed end.
	packTrailerBytes = 8
)

// CompactOptions configures Compact.
type CompactOptions struct {
	// CoverOptions reach the scratch store and the backend, and cover the
	// cycle from R before the victims' live keys are read.
	CoverOptions
	// Objects reads the victims and writes the new packs in the store's
	// bucket.
	Objects block_store_s3.ObjectStore
	// Bucket is the store's bucket.
	Bucket string
	// Prefix starts the store's object keys.
	Prefix string
	// MaxPackBytes bounds a new pack's file. Zero uses
	// writer.DefaultMaxPackBytes.
	MaxPackBytes int64
}

// CompactResult reports a compaction.
type CompactResult struct {
	// Blocked is why the cycle may not move a pack, empty when it may.
	Blocked string
	// Victims is the low-liveness packs this run read.
	Victims uint64
	// Gone is the victims whose files the store no longer has.
	Gone uint64
	// Packs is the new packs this run wrote and the backend adopted.
	Packs uint64
	// Blocks is the blocks those packs hold.
	Blocks uint64
	// Bytes is the bytes of their files.
	Bytes uint64
	// Resumed is the packs a killed run uploaded that this run adopted.
	Resumed uint64
	// Dropped is the packs a killed run recorded and never finished
	// uploading, deleted for this run to write again.
	Dropped uint64
}

// JSON describes the compaction.
func (r *CompactResult) JSON(a *fastjson.Arena) *fastjson.Value {
	return jsonObject(a,
		jsonField{"blocked", a.NewString(r.Blocked)},
		jsonField{"victims", jsonUint(a, r.Victims)},
		jsonField{"gone", jsonUint(a, r.Gone)},
		jsonField{"packs", jsonUint(a, r.Packs)},
		jsonField{"blocks", jsonUint(a, r.Blocks)},
		jsonField{"bytes", jsonUint(a, r.Bytes)},
		jsonField{"resumed", jsonUint(a, r.Resumed)},
		jsonField{"dropped", jsonUint(a, r.Dropped)},
	)
}

// Compact writes the live blocks of each pack judged low into new packs and
// has the backend adopt them, so the quarantine can take the victims. It
// pulls the log and walks W first, then reads each victim with no record
// once: its index, then its file in one pass, keeping each key in L or W that
// no adopted pack holds. The kept blocks fill new packs to MaxPackBytes in
// victim order, a pack crossing victims. Before each upload it records the
// pack's random id and victims, then uploads the file, then the entry, has
// the backend adopt the pack, and records where each key went. A run killed
// after the record adopts the pack when its entry is in the store, and
// otherwise deletes the file and writes the blocks again, so no block is lost
// and no pack written twice.
func Compact(ctx context.Context, opts *CompactOptions) (*CompactResult, error) {
	// Cover the cycle from R.
	_, blocked, err := cover(ctx, &opts.CoverOptions)
	if err != nil || blocked != "" {
		return &CompactResult{Blocked: blocked}, err
	}

	// Finish the packs a killed run recorded.
	res := &CompactResult{}
	if err := resumeCompaction(ctx, opts, res); err != nil {
		return nil, err
	}

	// Take the victims, and write their live blocks a pack at a time.
	victims, err := compactVictims(ctx, opts.Store)
	if err != nil {
		return nil, err
	}
	c := &compactor{opts: opts, res: res, src: &liveSource{ctx: ctx, opts: opts, victims: victims, res: res}}
	defer c.src.close()
	for {
		more, err := c.writePack(ctx)
		if err != nil || !more {
			return res, err
		}
	}
}

// compactVictims returns the packs judged low that hold no record, in id
// order.
func compactVictims(ctx context.Context, store kvtx.Store) ([]string, error) {
	// Read the records.
	records, err := readPackRecords(ctx, store)
	if err != nil {
		return nil, err
	}
	recorded := make(map[string]bool, len(records))
	for _, p := range records {
		recorded[p.ID] = true
	}

	// Take the low packs without one.
	var out []string
	err = Judgments(ctx, store, func(j *Judgment) error {
		if j.Verdict == VerdictLow && !recorded[j.ID] {
			out = append(out, j.ID)
		}
		return nil
	})
	return out, err
}

// resumeCompaction adopts each pack a killed run recorded whose entry is in
// the store, and drops the rest, deleting any file they left.
func resumeCompaction(ctx context.Context, opts *CompactOptions, res *CompactResult) error {
	// Read the packs recorded and not adopted.
	rows, err := readCompactPacks(ctx, opts.Store)
	if err != nil {
		return err
	}
	var adopt []AdoptedPack
	var adoptRows []*compactPack
	for _, row := range rows {
		if row.adopted {
			continue
		}

		// Drop a pack whose entry never landed.
		entry, err := readEntry(ctx, opts.Objects, opts.Bucket, opts.Prefix, row.id)
		if errors.Is(err, block_store_s3.ErrNotFound) {
			if err := dropCompactPack(ctx, opts, row.id); err != nil {
				return err
			}
			res.Dropped++
			continue
		}
		if err != nil {
			return err
		}

		// Read the keys of one whose entry did; its file is whole.
		ap, err := readIndexKeys(ctx, opts.Objects, opts.Bucket, opts.Prefix, entry)
		if err != nil {
			return err
		}
		adopt, adoptRows = append(adopt, ap), append(adoptRows, row)
	}

	// Adopt the rest a batch at a time, then place their keys.
	for start := 0; start < len(adopt); start += MaxBackendAdopt {
		end := min(start+MaxBackendAdopt, len(adopt))
		if err := opts.Backend.Adopt(ctx, adopt[start:end]); err != nil {
			return errors.Wrap(err, "adopt")
		}
		for i := start; i < end; i++ {
			if err := placePack(ctx, opts.Store, adoptRows[i], adopt[i].Keys); err != nil {
				return err
			}
			res.Resumed++
		}
	}
	return nil
}

// dropCompactPack deletes the file of a recorded pack whose entry never
// landed, then its record.
func dropCompactPack(ctx context.Context, opts *CompactOptions, id string) error {
	// Delete the file, which succeeds when it is missing.
	if err := opts.Objects.DeleteObject(ctx, opts.Bucket, block_store_s3.PackObjectKey(opts.Prefix, id)); err != nil {
		return errors.Wrap(err, "delete the unfinished pack "+id)
	}

	// Delete the record.
	tx, err := opts.Store.NewTransaction(ctx, true)
	if err != nil {
		return err
	}
	defer tx.Discard()
	if err := tx.Delete(ctx, []byte(compactPackPrefix+id)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// compactor writes the live blocks of the victims into new packs.
type compactor struct {
	// opts are the compaction's options.
	opts *CompactOptions
	// res is the compaction's result.
	res *CompactResult
	// src yields the live blocks.
	src *liveSource
}

// writePack fills one new pack from the victims, records it, uploads it and
// has the backend adopt it. It returns false once no live block remains.
func (c *compactor) writePack(ctx context.Context) (bool, error) {
	// Fill the pack to its bound.
	limit := c.opts.MaxPackBytes
	if limit <= 0 {
		limit = writer.DefaultMaxPackBytes
	}
	c.src.inPack = make(map[string]bool)
	fill := &packFill{src: c.src, budget: limit - packTrailerBytes}
	var buf bytes.Buffer
	result, err := writer.PackBlocks(&buf, fill.next)
	if err != nil || result.BlockCount == 0 {
		return false, err
	}
	if int64(buf.Len()) > limit {
		return false, errors.Errorf("new pack of %d bytes exceeds the %d byte bound", buf.Len(), limit)
	}

	// Record the pack's id and victims before its upload.
	entry := &packfile.PackfileEntry{
		Id:                 identity.NewPackID(),
		BloomFilter:        result.BloomFilter,
		BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount:         result.BlockCount,
		SizeBytes:          result.BytesWritten,
		CreatedAt:          timestamppb.New(time.Now().UTC()),
	}
	row := &compactPack{id: entry.GetId(), victims: fill.victims}
	if err := writeCompactPack(ctx, c.opts.Store, row); err != nil {
		return false, err
	}

	// Upload the file, then the entry, and have the backend adopt the pack.
	if err := uploadPack(ctx, c.opts, entry, buf.Bytes()); err != nil {
		return false, err
	}
	if err := c.opts.Backend.Adopt(ctx, []AdoptedPack{{Entry: entry, Keys: fill.keys}}); err != nil {
		return false, errors.Wrap(err, "adopt")
	}

	// Record where each key went.
	if err := placePack(ctx, c.opts.Store, row, fill.keys); err != nil {
		return false, err
	}
	c.res.Packs++
	c.res.Blocks += result.BlockCount
	c.res.Bytes += result.BytesWritten
	return true, nil
}

// uploadPack writes a new pack's file, then its entry, so a listed entry
// names a complete file.
func uploadPack(ctx context.Context, opts *CompactOptions, entry *packfile.PackfileEntry, data []byte) error {
	// Encode the entry.
	entryData, err := entry.MarshalVT()
	if err != nil {
		return err
	}

	// Write the file, then the entry.
	id := entry.GetId()
	if err := opts.Objects.PutObject(ctx, opts.Bucket, block_store_s3.PackObjectKey(opts.Prefix, id), data, "application/octet-stream"); err != nil {
		return errors.Wrap(err, "write the new pack "+id)
	}
	if err := opts.Objects.PutObject(ctx, opts.Bucket, block_store_s3.EntryObjectKey(opts.Prefix, id), entryData, "application/octet-stream"); err != nil {
		return errors.Wrap(err, "write the entry of the new pack "+id)
	}
	return nil
}

// packFill feeds one new pack the live blocks that fit its budget.
type packFill struct {
	// src yields the live blocks.
	src *liveSource
	// budget is the bytes left for the pack's values and index entries.
	budget int64
	// keys are the keys the pack holds.
	keys [][]byte
	// victims are the victims the pack's blocks came from, in order.
	victims []string
}

// next yields the next live block while it fits, leaving one that does not
// for the next pack; the first block always goes in.
func (f *packFill) next() (*hash.Hash, *block.StoredBlock, error) {
	// Take the next block, stopping at the end or the bound.
	lb, err := f.src.peek()
	if err != nil || lb == nil {
		return nil, nil, err
	}
	cost := int64(len(lb.key) + len(lb.value) + packEntryBytes)
	if cost > f.budget && len(f.keys) != 0 {
		return nil, nil, nil
	}
	f.src.take()

	// Decode it.
	h, err := packfile.ParseBlockKey(lb.key)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "pack %s holds a malformed key", lb.victim)
	}
	stored, err := block.DecodeBlockObject(lb.value)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "pack %s holds a malformed value", lb.victim)
	}

	// Add it to the pack.
	f.budget -= cost
	f.keys = append(f.keys, lb.key)
	f.src.inPack[string(Tail(lb.key))] = true
	if len(f.victims) == 0 || f.victims[len(f.victims)-1] != lb.victim {
		f.victims = append(f.victims, lb.victim)
	}
	return h, stored, nil
}

// liveBlock is a live block read from a victim.
type liveBlock struct {
	// victim is the pack it came from.
	victim string
	// key is its pack index key.
	key []byte
	// value is its encoded block object.
	value []byte
}

// liveSource reads the victims one at a time, each once: its index, then its
// file in one pass, yielding each kept key's value.
type liveSource struct {
	// opts are the compaction's options.
	opts *CompactOptions
	// res is the compaction's result.
	res *CompactResult
	// victims are the victims not yet opened.
	victims []string
	// inPack are the tails of the keys in the pack being written, which a
	// victim opened now does not write again.
	inPack map[string]bool
	// ctx is the compaction's context, for the reads.
	ctx context.Context
	// id is the open victim.
	id string
	// body is the open victim's file, read up to pos.
	body io.ReadCloser
	pos  uint64
	// entries are the open victim's kept keys, by offset.
	entries []*kvfile.IndexEntry
	// peeked is the block read and not yet taken.
	peeked *liveBlock
}

// peek returns the next live block without taking it, or nil at the end.
func (s *liveSource) peek() (*liveBlock, error) {
	// Return the block read last.
	if s.peeked != nil {
		return s.peeked, nil
	}

	// Open victims until one has a key left.
	for len(s.entries) == 0 {
		s.close()
		if len(s.victims) == 0 {
			return nil, nil
		}
		id := s.victims[0]
		s.victims = s.victims[1:]
		if err := s.open(id); err != nil {
			return nil, err
		}
	}

	// Read its next kept value.
	e := s.entries[0]
	s.entries = s.entries[1:]
	value, err := s.readValue(e)
	if err != nil {
		return nil, errors.Wrap(err, "read pack "+s.id)
	}
	s.peeked = &liveBlock{victim: s.id, key: e.GetKey(), value: value}
	return s.peeked, nil
}

// take consumes the block peek returned.
func (s *liveSource) take() {
	s.peeked = nil
}

// close closes the open victim's file.
func (s *liveSource) close() {
	if s.body != nil {
		_ = s.body.Close()
		s.body, s.entries = nil, nil
	}
}

// open reads victim id's index and the keys it keeps, and opens its file
// when it keeps any.
func (s *liveSource) open(id string) error {
	// Find the keys to keep.
	row, keep, err := s.keptTails(id)
	if err != nil || len(keep) == 0 {
		return err
	}

	// Read the victim's index and take the kept keys' entries by offset.
	entries, err := s.keptEntries(id, row, keep)
	if errors.Is(err, block_store_s3.ErrNotFound) {
		s.res.Gone++
		return nil
	}
	if err != nil {
		return err
	}

	// Open the file to read them in one pass.
	body, err := s.opts.Objects.GetObject(s.ctx, s.opts.Bucket, block_store_s3.PackObjectKey(s.opts.Prefix, id))
	if errors.Is(err, block_store_s3.ErrNotFound) {
		s.res.Gone++
		return nil
	}
	if err != nil {
		return errors.Wrap(err, "read pack "+id)
	}
	s.id, s.body, s.pos, s.entries = id, body, 0, entries
	s.res.Victims++
	return nil
}

// keptTails returns victim id's catalog row and the tails of its keys to keep: in
// L or W, sound, and in no adopted pack nor the pack being written.
func (s *liveSource) keptTails(id string) (catalogPack, map[string]bool, error) {
	// Open one read for the row and the members.
	ctx := s.ctx
	tx, err := s.opts.Store.NewTransaction(ctx, false)
	if err != nil {
		return catalogPack{}, nil, err
	}
	defer tx.Discard()

	// Read the victim's catalog row.
	data, found, err := tx.Get(ctx, []byte(catalogPackPrefix+id))
	if err == nil && !found {
		err = errors.Errorf("pack %s is not in the catalog", id)
	}
	if err != nil {
		return catalogPack{}, nil, err
	}
	row, err := unmarshalCatalogPack(data)
	if err != nil {
		return catalogPack{}, nil, err
	}

	// Keep each live, sound member no other pack holds.
	keep := make(map[string]bool)
	place := newPlacement(tx)
	prefix := []byte(catalogMemberPrefix + id + "/")
	err = tx.ScanPrefix(ctx, prefix, func(key, value []byte) error {
		tail := key[len(prefix):]
		ok, err := s.keeps(ctx, tx, place, tail, value)
		if ok {
			keep[string(tail)] = true
		}
		return err
	})
	return row, keep, err
}

// keeps reports whether a member of a victim is live, sound, and in no
// adopted pack nor the pack being written.
func (s *liveSource) keeps(ctx context.Context, tx kvtx.TxOps, place *placement, tail, value []byte) (bool, error) {
	// Skip a corrupt copy and one the pack being written holds.
	m, err := unmarshalMember(value)
	if err != nil || m.flags&memberCorrupt != 0 || s.inPack[string(tail)] {
		return false, err
	}

	// Keep a live key no adopted pack holds.
	live, err := isLive(ctx, tx, tail)
	if err != nil || !live {
		return false, err
	}
	placed, err := place.placed(ctx, tail)
	return !placed, err
}

// keptEntries reads victim id's index and returns the entries of the kept
// tails, by offset.
func (s *liveSource) keptEntries(id string, row catalogPack, keep map[string]bool) ([]*kvfile.IndexEntry, error) {
	// Read the index from the end of the file.
	ctx := s.ctx
	key := block_store_s3.PackObjectKey(s.opts.Prefix, id)
	read := func(ctx context.Context, off int64, length int) ([]byte, error) {
		return s.opts.Objects.GetObjectRange(ctx, s.opts.Bucket, key, off, length)
	}
	tail, err := packfile.ReadIndexTail(ctx, row.size, packfile.IndexTailReadSize(row.blocks), read)
	if err != nil {
		return nil, errors.Wrap(err, "read the index of pack "+id)
	}

	// Take the kept entries, by offset.
	var out []*kvfile.IndexEntry
	err = packfile.ScanIndexTail(tail, row.size, func(e *kvfile.IndexEntry) error {
		if keep[string(Tail(e.GetKey()))] {
			out = append(out, &kvfile.IndexEntry{Key: bytes.Clone(e.GetKey()), Offset: e.GetOffset(), Size: e.GetSize()})
		}
		return nil
	})
	if err != nil {
		return nil, errors.Wrap(err, "read the index of pack "+id)
	}
	slices.SortFunc(out, func(a, b *kvfile.IndexEntry) int { return cmp.Compare(a.GetOffset(), b.GetOffset()) })
	return out, nil
}

// readValue reads entry e's value from the open file, skipping the bytes
// before it.
func (s *liveSource) readValue(e *kvfile.IndexEntry) ([]byte, error) {
	// Skip to the value; the entries are by offset.
	if e.GetOffset() < s.pos {
		return nil, errors.New("overlapping values")
	}
	if _, err := io.CopyN(io.Discard, s.body, int64(e.GetOffset()-s.pos)); err != nil { //nolint:gosec // a pack is at most its bound.
		return nil, err
	}

	// Read it.
	value := make([]byte, e.GetSize())
	if _, err := io.ReadFull(s.body, value); err != nil {
		return nil, err
	}
	s.pos = e.GetOffset() + e.GetSize()
	return value, nil
}

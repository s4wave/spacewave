// Package packfetch copies a list of blocks from a read-only source into new
// packs in a bucket, holding nothing on the local disk.
//
// It is the outward-only fetcher of the World's move to B2: the new owner
// lists the blocks it lacks, a host that can read the old owner's packs reads
// exactly those blocks, and each pack it writes, with its entry beside it, is
// one a pack store adopts (see block_store_s3.PackStore.AdoptPack). It never
// deletes or overwrites an object: a resumed run reuses what an earlier run
// wrote, and stops when an object differs from what it would write.
package packfetch

import (
	"bytes"
	"cmp"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"slices"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/block/bloom"
	block_store_s3 "github.com/s4wave/spacewave/db/block/store/s3"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
	"golang.org/x/sync/errgroup"
)

// Object layout under Prefix, the pack store's own layout plus the markers.
const (
	// packDir holds the packs, as a PackStore names them.
	packDir = "packs/"
	// entryDir holds each pack's PackfileEntry, the manifest a store adopts.
	entryDir = "entries/"
	// markerDir holds, per batch of the list, the id its pack takes, written
	// before the pack so a resumed run names the pack the same.
	markerDir = "fetch/"
	// defaultBatchBlocks is the blocks one pack holds at most.
	defaultBatchBlocks = 4096
	// defaultReads is the source reads kept in flight.
	defaultReads = 8
)

// Source serves blocks with their refs, such as a pack store over the old
// owner's SFTP packs with exact reads. A nil block with no error is a miss.
type Source interface {
	GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error)
}

// Bucket is the target. It has no delete, so the fetcher cannot remove an
// object, and the fetcher puts only to keys a head found empty.
type Bucket interface {
	// PutObject writes the object.
	PutObject(ctx context.Context, bucket, key string, data []byte, contentType string) error
	// GetObject opens the object's body. The caller must Close it.
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	// HeadObject returns the object's size, or block_store_s3.ErrNotFound.
	HeadObject(ctx context.Context, bucket, key string) (*block_store_s3.ObjectStat, error)
}

// _ is a type assertion
var _ Bucket = (*block_store_s3.Client)(nil)

// Fetcher copies listed blocks into packs.
type Fetcher struct {
	// Source serves the blocks.
	Source Source
	// Bucket is the target.
	Bucket Bucket
	// BucketName names the target bucket.
	BucketName string
	// Prefix is the target pack store's object prefix, ending in "/".
	Prefix string
	// BatchBlocks is the most blocks in one pack; zero is 4096.
	BatchBlocks int
	// Reads is the source reads in flight; zero is 8.
	Reads int
	// OnPack is called with each pack's entry once its pack and entry are
	// written, for a live adopt. Optional.
	OnPack func(entry *packfile.PackfileEntry)
}

// Result reports a run.
type Result struct {
	// Packed is the packs this run wrote or finished.
	Packed int
	// Skipped is the batches an earlier run finished.
	Skipped int
	// Blocks is the blocks this run packed.
	Blocks int
	// Bytes is the pack bytes this run wrote.
	Bytes int64
	// Holes lists the blocks the source did not hold, in the batches this run
	// read.
	Holes []*block.BlockRef
}

// Run copies the blocks of refs. The list is deduplicated and sorted, then cut
// into batches, so the same list cuts into the same batches on every run, and
// a resumed run skips each batch whose entry an earlier run wrote.
func (f *Fetcher) Run(ctx context.Context, refs []*block.BlockRef) (*Result, error) {
	// Sort the refs by their pack key, dropping duplicates and empty refs.
	type item struct {
		key []byte
		ref *block.BlockRef
	}
	items := make([]item, 0, len(refs))
	for _, ref := range refs {
		if ref.GetEmpty() {
			continue
		}
		items = append(items, item{key: packfile.BlockKey(ref.GetHash()), ref: ref})
	}
	slices.SortFunc(items, func(a, b item) int { return bytes.Compare(a.key, b.key) })
	items = slices.CompactFunc(items, func(a, b item) bool { return bytes.Equal(a.key, b.key) })

	// Copy each batch in turn.
	res := &Result{}
	size := cmp.Or(f.BatchBlocks, defaultBatchBlocks)
	for batch := range slices.Chunk(items, size) {
		keys := make([][]byte, len(batch))
		batchRefs := make([]*block.BlockRef, len(batch))
		for i, it := range batch {
			keys[i], batchRefs[i] = it.key, it.ref
		}
		if err := f.runBatch(ctx, keys, batchRefs, res); err != nil {
			return res, err
		}
	}
	return res, nil
}

// runBatch copies one batch: it reads the batch's marker, or writes a new one
// naming a new pack id, then reads the blocks, then writes the pack and its
// entry unless they exist.
func (f *Fetcher) runBatch(ctx context.Context, keys [][]byte, refs []*block.BlockRef, res *Result) error {
	// Find the pack id an earlier run gave this batch, and skip a finished one.
	digest := sha256.New()
	for _, k := range keys {
		digest.Write(k)
		digest.Write([]byte{0})
	}
	markerKey := f.Prefix + markerDir + hex.EncodeToString(digest.Sum(nil))
	id, err := f.readObject(ctx, markerKey)
	if err != nil {
		return err
	}
	if id != nil {
		if err := identity.ValidatePackID(string(id)); err != nil {
			return errors.Wrapf(err, "marker %s", markerKey)
		}
		done, err := f.exists(ctx, f.Prefix+entryDir+string(id))
		if err != nil || done {
			if done {
				res.Skipped++
			}
			return err
		}
	}

	// Read the blocks, checking each against its ref.
	blocks := make([]*block.StoredBlock, len(refs))
	eg, ectx := errgroup.WithContext(ctx)
	eg.SetLimit(cmp.Or(f.Reads, defaultReads))
	for i, ref := range refs {
		eg.Go(func() error {
			// Read the block; a miss leaves its slot empty.
			stored, err := f.Source.GetStoredBlock(ectx, ref)
			if err != nil || stored == nil {
				return errors.Wrap(err, ref.MarshalString())
			}

			// Keep it only when its bytes match the ref.
			if err := ref.VerifyData(stored.Data, false); err != nil {
				return errors.Wrap(err, ref.MarshalString())
			}
			blocks[i] = stored
			return nil
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}

	// List the holes, and the blocks to pack.
	var packKeys [][]byte
	var packed []int
	for i, stored := range blocks {
		if stored == nil {
			res.Holes = append(res.Holes, refs[i])
			continue
		}
		packKeys = append(packKeys, keys[i])
		packed = append(packed, i)
	}
	if len(packed) == 0 {
		return nil
	}

	// Pack the blocks in key order, so a resumed batch builds the same bytes.
	var buf bytes.Buffer
	next := 0
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if next == len(packed) {
			return nil, nil, nil
		}
		i := packed[next]
		next++
		return refs[i].GetHash(), blocks[i], nil
	})
	if err != nil {
		return err
	}
	data := buf.Bytes()

	// Name the pack before writing it, so a kill leaves no pack a resume
	// cannot name.
	if id == nil {
		id = []byte(identity.NewPackID())
		if err := f.putNew(ctx, markerKey, id); err != nil {
			return err
		}
	}

	// Write the pack unless an earlier run wrote it.
	packKey := f.Prefix + packDir + string(id)
	if err := f.putNew(ctx, packKey, data); err != nil {
		return err
	}

	// Build the entry, with the bloom filter of the packed keys.
	bf := writer.DefaultPolicy().NewBloomFilter(uint64(len(packKeys)))
	for _, k := range packKeys {
		bf.Add(k)
	}
	bloomBytes, err := bloom.NewBloom(bf).MarshalVT()
	if err != nil {
		return err
	}
	entry := &packfile.PackfileEntry{
		Id:                 string(id),
		BloomFilter:        bloomBytes,
		BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount:         result.BlockCount,
		SizeBytes:          uint64(len(data)),
		CreatedAt:          timestamppb.New(time.Now().UTC()),
	}
	entryData, err := entry.MarshalVT()
	if err != nil {
		return err
	}

	// Write the entry, the manifest a store adopts, unless it exists.
	entryKey := f.Prefix + entryDir + string(id)
	if done, err := f.exists(ctx, entryKey); err != nil || done {
		return err
	}
	if err := f.Bucket.PutObject(ctx, f.BucketName, entryKey, entryData, "application/octet-stream"); err != nil {
		return errors.Wrap(err, "write entry")
	}

	// Count the pack and hand it to the adopt.
	res.Packed++
	res.Blocks += len(packed)
	res.Bytes += int64(len(data))
	if f.OnPack != nil {
		f.OnPack(entry)
	}
	return nil
}

// putNew writes data at key unless the object exists. An existing object with
// the same bytes is kept; one with other bytes stops the run.
func (f *Fetcher) putNew(ctx context.Context, key string, data []byte) error {
	// Read what the key holds.
	have, err := f.readObject(ctx, key)
	if err != nil {
		return err
	}
	if have != nil {
		if !bytes.Equal(have, data) {
			return errors.Errorf("object %s differs from what this run would write; not overwriting", key)
		}
		return nil
	}

	// Write it to the empty key.
	if err := f.Bucket.PutObject(ctx, f.BucketName, key, data, "application/octet-stream"); err != nil {
		return errors.Wrapf(err, "write %s", key)
	}
	return nil
}

// exists reports whether the bucket holds key.
func (f *Fetcher) exists(ctx context.Context, key string) (bool, error) {
	_, err := f.Bucket.HeadObject(ctx, f.BucketName, key)
	if errors.Is(err, block_store_s3.ErrNotFound) {
		return false, nil
	}
	return err == nil, err
}

// readObject returns the object at key, or nil when the bucket lacks it.
func (f *Fetcher) readObject(ctx context.Context, key string) ([]byte, error) {
	// Head the key first, so a miss costs no body.
	exists, err := f.exists(ctx, key)
	if err != nil || !exists {
		return nil, err
	}

	// Read the body.
	body, err := f.Bucket.GetObject(ctx, f.BucketName, key)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	return io.ReadAll(body)
}

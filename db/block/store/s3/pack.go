//go:build !tinygo

package block_store_s3

import (
	"bytes"
	"context"
	"io"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/aperturerobotics/protobuf-go-lite/types/known/timestamppb"
	"github.com/aperturerobotics/util/csync"
	"github.com/pkg/errors"
	"github.com/s4wave/spacewave/db/block"
	"github.com/s4wave/spacewave/db/packfile"
	"github.com/s4wave/spacewave/db/packfile/identity"
	packfile_store "github.com/s4wave/spacewave/db/packfile/store"
	"github.com/s4wave/spacewave/db/packfile/writer"
	"github.com/s4wave/spacewave/net/hash"
	"golang.org/x/sync/errgroup"
)

// batchConcurrency bounds the concurrent requests of one batch call.
const batchConcurrency = 16

const (
	// packDir holds the packfiles under the object prefix.
	packDir = "packs/"
	// entryDir holds one PackfileEntry per complete packfile.
	entryDir = "entries/"
)

// ErrPackImmutable is returned when removing a block from a PackStore.
var ErrPackImmutable = errors.New("bucket packfiles do not remove blocks")

// PackStore is a block store on an S3-compatible bucket that writes each batch
// of blocks as one packfile, so small blocks cost one request per batch
// instead of one each.
//
// A packfile is the object {prefix}packs/{id}: a kvfile of block objects, the
// bytes and refs of each block, keyed by block hash. The object
// {prefix}entries/{id} holds its PackfileEntry with the bloom filter readers
// use to find the pack of a block. The entry is written after the packfile, so
// a listed entry names a complete packfile.
//
// Reads use the entries this store wrote or listed. A lookup that finds
// nothing lists the entries again, which finds the packfiles other writers
// added since.
type PackStore struct {
	// client sends the signed requests.
	client *Client
	// bucket is the bucket holding the objects.
	bucket string
	// prefix precedes every object key.
	prefix string
	// packs reads blocks from the known packfiles.
	packs *packfile_store.PackfileStore

	// mtx guards entries.
	mtx sync.Mutex
	// entries are the known packfiles by id.
	entries map[string]*packfile.PackfileEntry

	// listMtx serializes entry listings.
	listMtx csync.Mutex
	// listings counts the entry listings started.
	listings atomic.Uint64
}

// NewPackStore builds a packfile block store on the bucket.
func NewPackStore(client *Client, bucket, prefix string) *PackStore {
	s := &PackStore{
		client:  client,
		bucket:  bucket,
		prefix:  prefix,
		entries: make(map[string]*packfile.PackfileEntry),
	}
	s.packs = packfile_store.NewPackfileStore(s.openPack, nil)
	return s
}

// Close releases the open packfile readers.
func (s *PackStore) Close() {
	s.packs.Close()
}

// GetHashType returns 0 to use the default hash type.
func (s *PackStore) GetHashType() hash.HashType {
	return 0
}

// GetSupportedFeatures returns the native feature bitmask for the store.
func (s *PackStore) GetSupportedFeatures() block.StoreFeature {
	return block.StoreFeature_STORE_FEATURE_UNKNOWN
}

// BeginReadOperation returns the store as the scoped read handle.
func (s *PackStore) BeginReadOperation(context.Context) (block.StoreOps, func(), error) {
	return s, func() {}, nil
}

// PutBlock writes one block as its own packfile.
func (s *PackStore) PutBlock(ctx context.Context, data []byte, opts *block.PutOpts) (*block.BlockRef, bool, error) {
	ref, err := block.BuildBlockRef(data, opts)
	if err != nil {
		return nil, false, err
	}
	entry := &block.PutBatchEntry{Ref: ref, Data: data, Refs: opts.GetRefs()}
	if err := s.PutBlockBatch(ctx, []*block.PutBatchEntry{entry}); err != nil {
		return nil, false, err
	}
	return ref, false, nil
}

// PutBlockBatch writes the batch as one packfile and its entry.
//
// The batch must fit one packfile: at most writer.DefaultMaxBlocksPerPack
// blocks and writer.DefaultMaxPackBytes bytes. Tombstones are rejected.
func (s *PackStore) PutBlockBatch(ctx context.Context, batch []*block.PutBatchEntry) error {
	var buf bytes.Buffer
	i := 0
	result, err := writer.PackBlocks(&buf, func() (*hash.Hash, *block.StoredBlock, error) {
		if i == len(batch) {
			return nil, nil, nil
		}
		entry := batch[i]
		i++
		if entry.Tombstone {
			return nil, nil, ErrPackImmutable
		}
		return entry.Ref.GetHash(), &block.StoredBlock{Data: entry.Data, Refs: entry.Refs, RefsKnown: true}, nil
	})
	if err != nil {
		return err
	}
	if result.BlockCount == 0 {
		return nil
	}
	if int64(buf.Len()) > writer.DefaultMaxPackBytes {
		return errors.Errorf("packfile of %d bytes exceeds the %d byte limit", buf.Len(), writer.DefaultMaxPackBytes)
	}

	id, err := identity.BuildPackID(s.bucket+"/"+s.prefix, result)
	if err != nil {
		return err
	}
	entry := &packfile.PackfileEntry{
		Id:                 id,
		BloomFilter:        result.BloomFilter,
		BloomFormatVersion: packfile.BloomFormatVersionV1,
		BlockCount:         result.BlockCount,
		SizeBytes:          result.BytesWritten,
		CreatedAt:          timestamppb.New(time.Now().UTC()),
	}
	entryData, err := entry.MarshalVT()
	if err != nil {
		return err
	}
	if err := s.client.PutObject(ctx, s.bucket, s.prefix+packDir+id, buf.Bytes(), "application/octet-stream"); err != nil {
		return errors.Wrap(err, "write packfile")
	}
	if err := s.client.PutObject(ctx, s.bucket, s.prefix+entryDir+id, entryData, "application/octet-stream"); err != nil {
		return errors.Wrap(err, "write packfile entry")
	}
	s.addEntries(entry)
	return nil
}

// GetBlock looks up a block in the store.
func (s *PackStore) GetBlock(ctx context.Context, ref *block.BlockRef) ([]byte, bool, error) {
	stored, err := s.GetStoredBlock(ctx, ref)
	return stored.GetData(), stored != nil, err
}

// GetStoredBlock returns the block with its refs. Returns nil if the block is
// not found.
func (s *PackStore) GetStoredBlock(ctx context.Context, ref *block.BlockRef) (*block.StoredBlock, error) {
	if ref.GetEmpty() {
		return nil, block.ErrEmptyBlockRef
	}
	var stored *block.StoredBlock
	err := s.lookup(ctx, func() (bool, error) {
		var err error
		stored, err = s.packs.GetStoredBlock(ctx, ref)
		return stored != nil, err
	})
	return stored, err
}

// GetBlockExists checks the packfiles for the block.
func (s *PackStore) GetBlockExists(ctx context.Context, ref *block.BlockRef) (bool, error) {
	var exists bool
	err := s.lookup(ctx, func() (bool, error) {
		var err error
		exists, err = s.packs.GetBlockExists(ctx, ref)
		return exists, err
	})
	return exists, err
}

// GetBlockExistsBatch checks the packfiles for each block.
func (s *PackStore) GetBlockExistsBatch(ctx context.Context, refs []*block.BlockRef) ([]bool, error) {
	var exists []bool
	err := s.lookup(ctx, func() (bool, error) {
		var err error
		exists, err = s.packs.GetBlockExistsBatch(ctx, refs)
		return err == nil && !slices.Contains(exists, false), err
	})
	return exists, err
}

// StatBlock returns the block's metadata, or nil if the block is not found.
func (s *PackStore) StatBlock(ctx context.Context, ref *block.BlockRef) (*block.BlockStat, error) {
	var stat *block.BlockStat
	err := s.lookup(ctx, func() (bool, error) {
		var err error
		stat, err = s.packs.StatBlock(ctx, ref)
		return stat != nil, err
	})
	return stat, err
}

// RmBlock returns ErrPackImmutable: packfiles are written once.
func (s *PackStore) RmBlock(context.Context, *block.BlockRef) error {
	return ErrPackImmutable
}

// Sync reports always-durable: each batch is written before PutBlockBatch
// returns.
func (s *PackStore) Sync(context.Context) (bool, error) {
	return true, nil
}

// lookup runs find against the known packfiles. When find reports a miss, it
// lists the entries, which finds the packfiles other writers added, and runs
// find again.
func (s *PackStore) lookup(ctx context.Context, find func() (bool, error)) error {
	listed := s.listings.Load()
	found, err := find()
	if found || err != nil {
		return err
	}
	if err := s.listEntries(ctx, listed); err != nil {
		return err
	}
	_, err = find()
	return err
}

// listEntries lists the packfile entries and reads the new ones, unless a
// listing started after the caller's count of listed already completed.
func (s *PackStore) listEntries(ctx context.Context, listed uint64) error {
	release, err := s.listMtx.Lock(ctx)
	if err != nil {
		return err
	}
	defer release()
	if s.listings.Load() != listed {
		return nil
	}
	s.listings.Add(1)

	var ids []string
	dir := s.prefix + entryDir
	err = s.client.ListObjects(ctx, s.bucket, dir, func(key string, _ int64) error {
		id := strings.TrimPrefix(key, dir)
		if !s.hasEntry(id) {
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil {
		return errors.Wrap(err, "list packfile entries")
	}

	entries := make([]*packfile.PackfileEntry, len(ids))
	eg, egCtx := errgroup.WithContext(ctx)
	eg.SetLimit(batchConcurrency)
	for i, id := range ids {
		eg.Go(func() error {
			entry, err := s.readEntry(egCtx, id)
			entries[i] = entry
			return err
		})
	}
	if err := eg.Wait(); err != nil {
		return err
	}
	s.addEntries(entries...)
	return nil
}

// readEntry reads the entry object of packfile id.
func (s *PackStore) readEntry(ctx context.Context, id string) (*packfile.PackfileEntry, error) {
	body, err := s.client.GetObject(ctx, s.bucket, s.prefix+entryDir+id)
	if err != nil {
		return nil, errors.Wrap(err, "read packfile entry")
	}
	data, err := io.ReadAll(body)
	_ = body.Close()
	if err != nil {
		return nil, errors.Wrap(err, "read packfile entry")
	}
	entry := &packfile.PackfileEntry{}
	if err := entry.UnmarshalVT(data); err != nil {
		return nil, errors.Wrap(err, "decode packfile entry "+id)
	}
	if entry.GetId() != id {
		return nil, errors.Errorf("packfile entry %s names packfile %s", id, entry.GetId())
	}
	return entry, nil
}

// hasEntry reports whether packfile id is known.
func (s *PackStore) hasEntry(id string) bool {
	s.mtx.Lock()
	defer s.mtx.Unlock()
	return s.entries[id] != nil
}

// addEntries adds known packfiles and publishes them to the reader.
func (s *PackStore) addEntries(entries ...*packfile.PackfileEntry) {
	s.mtx.Lock()
	for _, entry := range entries {
		s.entries[entry.GetId()] = entry
	}
	all := make([]*packfile.PackfileEntry, 0, len(s.entries))
	for _, entry := range s.entries {
		all = append(all, entry)
	}
	s.mtx.Unlock()
	s.packs.UpdateManifest(all)
}

// openPack opens a reader over the ranges of packfile id.
func (s *PackStore) openPack(id string, size int64) (*packfile_store.PackReader, error) {
	transport := &rangeTransport{client: s.client, bucket: s.bucket, key: s.prefix + packDir + id}
	return packfile_store.NewPackReader(id, size, transport, 0), nil
}

// rangeTransport reads byte ranges of one packfile object.
type rangeTransport struct {
	client *Client
	bucket string
	key    string
}

// Fetch reads bytes [off, off+length) of the object.
func (t *rangeTransport) Fetch(ctx context.Context, off int64, length int) ([]byte, error) {
	if length <= 0 {
		return nil, nil
	}
	return t.client.GetObjectRange(ctx, t.bucket, t.key, off, length)
}

// _ is a type assertion
var (
	_ block.StoreOps           = (*PackStore)(nil)
	_ packfile_store.Transport = (*rangeTransport)(nil)
)
